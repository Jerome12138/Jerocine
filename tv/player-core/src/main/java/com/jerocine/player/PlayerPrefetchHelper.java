package com.jerocine.player;

import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;

/**
 * 下一集清单预取 — 端侧优先路径的加速器.
 *
 * 背景: 端侧优先下每集要过滤**两次**清单(master + 子表, 服务端 FilterText 只绝对化不改写子表),
 * 单次实测 1.4s(小清单)~5.1s(900+ 段大清单) ⇒ 切集白等 3~10s。清单本身很小(上行 ~25KB),
 * 提前抓取代价极低。
 *
 * 只做三件事:
 *   ① 进入片尾窗口后, 后台 GET 下一集的原始清单;
 *   ② POST /v1/m3u8/filter 端侧过滤, 结果写进 {@link PlayerSession} 的预取缓存;
 *   ③ 若是 master 表, 再下钻一层把子表也预热(否则切集仍会卡在子表那次过滤上)。
 *
 * **触发时机 = 距片尾/跳过片尾点 {@link #NEAR_OUTRO_LEAD_MS}(60s) 内**(与 web 播放页的
 * `PREFETCH_LEAD_S` 对齐, 见 {@link PlayerActivity#maybeRefreshPrefetchNearOutro()})。
 * 不做"起播就预取下一集": 预取结果 TTL 只有 10min, 而一集 30~45min ⇒ 起播时预取的那份到切集时
 * 必然过期, 对自动连播是白做; 而它唯一能覆盖的"用户 10min 内手动按下一集"窗口太窄,
 * 不值当每集多烧一次服务端过滤。宁可只保留**切集前那一刻一定新鲜**的这一路。
 *
 * 消费方是 {@link PlayerAdFilterHelper.FilterPlaylistParser}: 解析到清单时先查缓存, 命中就直接用,
 * 不再付 POST 的代价。**只预取"下一集会走端侧过滤"的情形** —— 本集走代理/中转、过滤关闭、
 * 单 URL 兼容模式都直接跳过(那些路径由服务端自己缓存, 端侧预取纯属白做)。
 *
 * 幂等 + 静默失败: 同一 URL 不重复在途; 预取失败当没预取, 切集时按原路径再付一次全价, 不影响播放。
 */
public class PlayerPrefetchHelper {

    /**
     * 与 PlayerActivity.buildCacheFactory 的 OkHttpDataSource UA 保持一致 ——
     * 有的源站按 UA 返回不同清单, 不一致会导致预取结果与实际播放的清单不是一回事。
     */
    private static final String USER_AGENT = "Jerocine/1.0 (Android TV)";
    /**
     * 接近片尾的提前量: 剩余时长 ≤ 片尾跳过秒数 + 这个值 时预取下一集.
     * 60s 与 web 播放页的 `PREFETCH_LEAD_S` 对齐; 比一次过滤(vs 最坏 5.1s)留足余量。
     */
    static final long NEAR_OUTRO_LEAD_MS = 60_000L;
    /** 最小重复间隔: 片尾窗口内进度轮询每 5s 一次, 刚做过(< 60s)就别再做 —— 否则会重复抓十几次. */
    private static final long REFRESH_MIN_AGE_MS = 60_000L;
    /** 下钻层数上限: master → 子表(再往下是分片, 不用预取). */
    private static final int MAX_CHILD_DEPTH = 1;
    /** 单个 master 最多预热几个子表(多码率源; AUTO 默认取靠前的几个). */
    private static final int MAX_CHILD_PLAYLISTS = 3;

    private final PlayerSession session;
    private final ExecutorService io = Executors.newSingleThreadExecutor(r -> {
        Thread t = new Thread(r, "jc-prefetch");
        t.setDaemon(true);
        return t;
    });
    /** 抓清单用与播放同一套网络栈(壳层可能为老设备放宽了 TLS), 但加总时限 —— 预取卡死不能占线程. */
    private final OkHttpClient client;
    /** 在途 URL(去重): 同一清单只跑一次. */
    private final Set<String> inflight = ConcurrentHashMap.newKeySet();

    public PlayerPrefetchHelper(PlayerSession session) {
        this.session = session;
        OkHttpClient injected = JerocinePlayer.mediaClient();
        OkHttpClient.Builder builder =
                (injected != null) ? injected.newBuilder() : new OkHttpClient.Builder();
        this.client = builder
                .connectTimeout(8, TimeUnit.SECONDS)
                .readTimeout(12, TimeUnit.SECONDS)
                .writeTimeout(12, TimeUnit.SECONDS)
                .callTimeout(20, TimeUnit.SECONDS)
                .retryOnConnectionFailure(true)
                .build();
    }

    /**
     * 进入片尾窗口后即可调用(主线程). 只有"下一集确实走端侧过滤 + 手上还没有新鲜结果"才会真的去抓,
     * 所以调用方可以每 5s 无脑调一次(见 {@link PlayerActivity#maybeRefreshPrefetchNearOutro()})。
     */
    void schedule() {
        int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
        if (!canPrefetch(idx)) return;
        String raw = rawUrlAt(idx + 1);
        if (raw == null) return;
        long age = session.prefetchedAgeMs(raw);
        if (age >= 0 && age < REFRESH_MIN_AGE_MS) return; // 刚预取过
        submit(raw, session.prefetchGeneration());
    }

    /** 播放器销毁: 停掉在途预取. */
    void shutdown() {
        io.shutdownNow();
    }

    // ============================ 内部 ============================

    /** 只有"下一集确实会走端侧过滤"才值得预取. */
    private boolean canPrefetch(int currentIdx) {
        if (!session.adFilterOn) return false;                                        // 过滤关着, 没人查缓存
        if (session.currentBypassFilter) return false;                                // 单 URL 兼容模式: 不包装也不过滤
        if (session.proxyBase == null || session.proxyBase.isEmpty()) return false;   // 端侧过滤也要服务端 /v1/m3u8/filter
        int next = currentIdx + 1;
        String raw = rawUrlAt(next);
        if (raw == null || !PlayerUrls.isM3u8(raw)) return false;
        // 下一集正在播本地(本地优先/单集本地) → 会直接播 file:// 本地清单, 端侧预取纯浪费一次过滤 POST
        if (session.isEpisodePlayingLocal(next)) return false;
        // 下一集实际会包代理/走中转(端侧不可用或已升级) → 清单由服务端抓并缓存, 端侧预取无意义
        return raw.equals(session.mediaUriFor(next, raw));
    }

    private String rawUrlAt(int idx) {
        if (idx < 0 || idx >= session.currentRawUrls.size()) return null;
        String url = session.currentRawUrls.get(idx);
        return (url == null || url.isEmpty()) ? null : url;
    }

    /**
     * 提交一次预取. 这里**不**看"是否已有缓存" —— 调用方({@link #schedule()})已按新鲜度节流过,
     * 而片尾那次的目的就是**刷新**掉 TTL 内但已不新鲜的旧结果。
     */
    private void submit(String url, int gen) {
        if (!inflight.add(url)) return; // 已在途
        if (io.isShutdown()) {
            inflight.remove(url);
            return;
        }
        io.execute(() -> {
            try {
                prefetchOne(url, 0, gen);
            } finally {
                inflight.remove(url);
            }
        });
    }

    /** 后台线程: 抓原始清单 → 端侧过滤 → 写缓存; depth=0 时再下钻一层子表. */
    private void prefetchOne(String url, int depth, int gen) {
        if (gen != session.prefetchGeneration()) return;
        byte[] raw = fetch(url);
        if (raw == null) return;
        PlayerSession.Prefetched done = filterViaServer(url, raw);
        if (done == null) return;
        // 预取与播放共用同一条端侧过滤链路: 成功即清失败连击(见 FilterEscalationPolicy)
        session.clientFilterFailStreak = 0;
        session.putPrefetched(url, done.data, done.filteredCount, gen);
        if (depth >= MAX_CHILD_DEPTH) return;
        // 子表串行做(1~3 个): 与 master 共用一条预取线程, 不必再开并发.
        // 与 master 同批重做(不看是否已有缓存) —— 保证整棵树的过滤结果同一时间基准, 别一半新鲜一半旧.
        List<String> children = PlayerUrls.childPlaylistUrls(
                new String(done.data, StandardCharsets.UTF_8), MAX_CHILD_PLAYLISTS);
        for (String child : children) {
            if (gen != session.prefetchGeneration()) return;
            prefetchOne(child, depth + 1, gen);
        }
    }

    /** GET 原始清单; 失败返回 null(静默). */
    private byte[] fetch(String url) {
        try {
            Request req = new Request.Builder().url(url).header("User-Agent", USER_AGENT).build();
            try (Response resp = client.newCall(req).execute()) {
                if (!resp.isSuccessful() || resp.body() == null) return null;
                return resp.body().bytes();
            }
        } catch (Exception e) {
            return null;
        }
    }

    /** POST 端侧过滤(/v1/m3u8/filter), 返回"过滤后字节 + 剔除段数"; 失败返回 null. */
    private PlayerSession.Prefetched filterViaServer(String srcUrl, byte[] raw) {
        String base = session.proxyBase;
        if (base == null || base.isEmpty()) return null;
        final String url;
        try {
            url = base + "/v1/m3u8/filter?src="
                    + java.net.URLEncoder.encode(srcUrl, "UTF-8");
        } catch (Exception e) {
            return null;
        }
        try {
            Request req = new Request.Builder().url(url)
                    .post(RequestBody.create(
                            MediaType.parse("application/vnd.apple.mpegurl"), raw))
                    .build();
            try (Response resp = client.newCall(req).execute()) {
                if (!resp.isSuccessful() || resp.body() == null) return null;
                int count = 0;
                String header = resp.header("X-Ad-Filtered");
                if (header != null) {
                    try {
                        count = Integer.parseInt(header.trim());
                    } catch (NumberFormatException ignore) {
                    }
                }
                byte[] out = resp.body().bytes();
                if (out.length == 0) return null;
                return new PlayerSession.Prefetched(out, count, System.currentTimeMillis());
            }
        } catch (Exception e) {
            return null;
        }
    }
}
