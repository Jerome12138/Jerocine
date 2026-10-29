package com.jerocine.player;

import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;

/**
 * 下一集清单预取 — 端侧优先路径的加速器.
 *
 * 背景: 端侧优先下每集有一次**在过滤窗口里白等**的媒体列表过滤(服务端 FilterText 只绝对化
 * 不改写子表; master 层已免 POST 直通, 见 PlayerUrls.hasMediaSegments),
 * 单次实测 1.4s(小清单)~5.1s(900+ 段大清单) ⇒ 切集白等 1~5s。清单本身很小(上行 ~25KB),
 * 提前抓取代价极低。
 *
 * 只做三件事:
 *   ① 进入片尾窗口后, 后台 GET 下一集的原始清单;
 *   ② POST /v1/m3u8/filter 端侧过滤(媒体列表才 POST, master 层免 POST 直通), 结果写进
 *      {@link PlayerSession} 的预取缓存;
 *   ③ 若是 master 表, 再下钻一层把子表也预热(否则切集仍会卡在子表那次过滤上);
 *      master 免 POST 后子表地址不再被服务端顺手绝对化, 由 {@link PlayerUrls#absolutize} 端侧解析。
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
    /** 在途 URL(去重): 同一清单只跑一次. (不用 newKeySet —— API 24+, 见 PlayerSession#newConcurrentKeySet) */
    private final Set<String> inflight = java.util.Collections.newSetFromMap(new ConcurrentHashMap<String, Boolean>());

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
        if (session.currentBypassFilter) return false;                                // 单 URL 兼容模式: 不过滤
        int next = currentIdx + 1;
        String raw = rawUrlAt(next);
        if (raw == null || !PlayerUrls.isM3u8(raw)) return false;
        // 下一集正在播本地(本地优先/单集本地) → 会直接播 file:// 本地清单, 端侧预取纯浪费
        return !session.isEpisodePlayingLocal(next);
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

    /** 后台线程: 抓原始清单 → 媒体列表才 POST 过滤(master 免 POST) → 写缓存; depth=0 再下钻一层子表. */
    private void prefetchOne(String url, int depth, int gen) {
        if (gen != session.prefetchGeneration()) return;
        byte[] raw = fetch(url);
        if (raw == null) return;
        String text = new String(raw, StandardCharsets.UTF_8);
        byte[] data = raw;
        int filteredCount = 0;
        if (PlayerUrls.hasMediaSegments(text)) {
            // 媒体列表(广告在这层)才值得过滤; master 层与播放路径 master-direct 一致免过滤
            PlayerSession.Prefetched done = filterLocal(url, raw);
            if (done == null) return;
            data = done.data;
            filteredCount = done.filteredCount;
        }
        // master 也照写缓存: 播放路径 master 层已免 POST 直通(不查缓存), 写它纯粹为了让
        // schedule() 的 REFRESH_MIN_AGE 节流照常工作 —— 不写的话片尾窗口会每 5s 重抓一次 master。
        session.putPrefetched(url, data, filteredCount, gen);
        if (depth >= MAX_CHILD_DEPTH) return;
        // 子表串行做(1~3 个): 与 master 共用一条预取线程, 不必再开并发.
        // 与 master 同批重做(不看是否已有缓存) —— 保证整棵树的过滤结果同一时间基准, 别一半新鲜一半旧.
        List<String> children = PlayerUrls.childPlaylistUrls(text, MAX_CHILD_PLAYLISTS);
        for (String child : children) {
            if (gen != session.prefetchGeneration()) return;
            // master 免 POST 后子表地址不再被服务端顺手绝对化 → 端侧自行解析;
            // 解析失败跳过该子表(退化为切集时现过滤, 只损失预取收益不影响正确性)
            String abs = PlayerUrls.absolutize(child, url);
            if (abs == null) continue;
            prefetchOne(abs, depth + 1, gen);
        }
    }

    /** GET 原始清单; 失败返回 null(静默). */
    private byte[] fetch(String url) {
        final String srcTag = AdFilterTrace.srcTag(url);
        long t0 = System.nanoTime();
        try {
            Request req = new Request.Builder().url(url).header("User-Agent", USER_AGENT).build();
            try (Response resp = client.newCall(req).execute()) {
                if (!resp.isSuccessful() || resp.body() == null) {
                    logTrace("prefetch-fetch", srcTag, "fail http=" + resp.code(), t0);
                    return null;
                }
                byte[] body = resp.body().bytes();
                logTrace("prefetch-fetch", srcTag, "ok http=" + resp.code() + " in=" + body.length, t0);
                return body;
            }
        } catch (Exception e) {
            logTrace("prefetch-fetch", srcTag, "fail ex=" + e.getClass().getSimpleName(), t0);
            return null;
        }
    }

    /** 本地过滤媒体清单(M3u8FilterCore), 返回"过滤后字节 + 剔除段数"; 异常返回 null. */
    private PlayerSession.Prefetched filterLocal(String srcUrl, byte[] raw) {
        final String srcTag = AdFilterTrace.srcTag(srcUrl);
        long t0 = System.nanoTime();
        try {
            M3u8FilterCore.Result r = M3u8FilterCore.rewriteM3u8(
                    new String(raw, StandardCharsets.UTF_8), srcUrl, true);
            logTrace("prefetch-filter", srcTag,
                    "ok out=" + r.getText().length() + " ads=" + r.getFiltered(), t0);
            return new PlayerSession.Prefetched(
                    r.getText().getBytes(StandardCharsets.UTF_8), r.getFiltered(),
                    System.currentTimeMillis());
        } catch (Exception e) {
            logTrace("prefetch-filter", srcTag, "fail ex=" + e.getClass().getSimpleName(), t0);
            return null;
        }
    }

    /** 埋点: 记一行带耗时的路径决策(phase 区分抓取与过滤; 格式见 {@link AdFilterTrace})。 */
    private static void logTrace(String phase, String srcTag, String decision, long startNanos) {
        android.util.Log.i(AdFilterTrace.TAG, AdFilterTrace.layer(
                phase, decision, (System.nanoTime() - startNanos) / 1_000_000L, srcTag));
    }
}
