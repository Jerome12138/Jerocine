package com.jerocine.player;

import android.content.Context;
import android.os.Handler;
import android.os.Looper;
import android.view.KeyEvent;

import androidx.media3.common.MediaItem;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.ui.PlayerView;

import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 播放会话 — helper 之间唯一的共享上下文.
 *
 * 抽取前 6 个 helper 通过 activity 互相直接读写对方字段(adFilter → source → adFilter → skip 成环),
 * 任何一处改动都要同时读 6 个文件. 现在把"跨 helper 的共享状态与操作"全部收敛到本类:
 *   - helper 之间互不引用, 只依赖 session;
 *   - PlayerActivity 只负责 UI 出口(Host)与视图装配, 不再持有播放状态;
 *   - 本类不持有任何 View —— 视图全部归 PlayerActivity, helper 经 Host 读写.
 */
public class PlayerSession {

    /** 壳内 UI 出口 — 由 PlayerActivity 实现, session/helper 通过它反馈界面与生命周期. */
    public interface Host {
        Context context();

        void showCenterToast(String msg, long durationMs);

        void showCenterIcon(int drawableRes, long durationMs);

        void showCenterIconPersistent(int drawableRes);

        void hideGestureToast();

        void finishPlayer();

        /** 按当前集刷新标题栏(片名/集数). */
        void updateTitleForCurrent();

        void emitEvent(String name, JSONObject payload);

        /** 当前影片 id(供事件回传), 无则空串. */
        String filmId();

        /** 切换广告过滤开关(由 AdFilterHelper 实现, 供底栏"过滤"按钮调用). */
        void toggleAdFilter();

        /** 刷新底栏"过滤"按钮左上角状态点(开=绿点/关=灰点). 文案恒定不变色. */
        void renderAdFilterSwitch(boolean on);

        /** 切换线路开关(直连 ⇄ 中转, 由 NetworkModeHelper 实现, 供底栏"中转"按钮调用). */
        void toggleNetworkMode();

        /** 刷新底栏"中转"按钮左上角状态点(开=绿点/关=灰点). 文案恒定为"中转". */
        void renderNetworkMode(boolean relay);

        /** 播放器视图 — 壳层装配的实例, helper 做面板显隐/取子控件时经此访问. */
        PlayerView playerView();

        /** 刷新广告过滤角标(五态文案 + 状态点色调); status 为 null 表示隐藏(尚无片源). */
        void renderAdFilterBadge(AdFilterStatus status);

        /** 刷新倍速角标文案. */
        void renderSpeedText(String label);

        /** 刷新底栏"倍速"按钮左上角状态点(播放倍速 ≠ 1.0 即开). */
        void renderSpeedDot(boolean on);

        /** 刷新底栏"跳过"按钮左上角状态点(片头/片尾跳过已启用即开). */
        void renderSkipDot(boolean on);

        /** 把按键事件交回 Activity 默认处理(焦点移动等). */
        boolean dispatchToSuper(KeyEvent event);
    }

    /** 多源模式下的单个片源(单源/单 URL 模式时 sourceList 为空). */
    public static class SourceData {
        public String id;
        public String name;
        /**
         * 服务端能否抓到这个源的清单(play 接口的 adFilterOk 透传)。
         * null=未测/老壳没传 → 按"可用"处理(保持旧行为); false=服务端抓不到 → 不包装代理。
         */
        public Boolean adFilterOk;
        public final ArrayList<String> urls = new ArrayList<>();
        public final ArrayList<String> titles = new ArrayList<>();
    }

    private final Host host;

    // ===== 播放器 =====
    ExoPlayer player;

    // ===== 本地文件播放(文件管理器「打开方式」/ 应用内 SAF 选文件) =====
    /** 本地模式: 不经 HLS 广告过滤链路, 用 DefaultMediaSourceFactory 自动识别 mp4/ts/m3u8. */
    boolean localPlayback = false;
    /** 本地文件 content:// / file:// URI. */
    String localUri = "";
    /** 本地文件显示名(文件名, 无则 fallback). */
    String localTitle = "";

    // ===== 离线播放(下载管理页「播放」, 本地过滤后清单 + 下载缓存区) =====
    /**
     * 离线模式: 播放的是**已下载**的分片(SimpleCache 里), 与在线播放的本质区别:
     * <ul>
     *   <li>清单已过滤过(下载时落库的 playlist.m3u8), 再送 /v1/m3u8/filter 纯属白付一次
     *       POST, 而且失败会触发 escalateToProxy → file:// 被包成 /m3u8/proxy?src=file://…
     *       → 服务端根本抓不到本地文件 → "清单代理失败, 已切换直连" 来回打转 ——
     *       2026-10-08 用户实锤"播放已下载视频一直走中转、直连逻辑"的根因;</li>
     *   <li>中转/直连自愈全部无意义: 分片要么在缓存里, 要么就得重新下载, 网络换线救不了;</li>
     *   <li>过滤/中转/换源/选集控件全部隐藏(单集离线, 这些开关没有指代对象)。</li>
     * </ul>
     * 由 PlayerActivity 检测 EXTRA_CACHE_DIR + file:// EXTRA_URL 置位(与 localPlayback 互斥)。
     */
    volatile boolean offlinePlayback = false;

    // ===== 片源 =====
    final ArrayList<SourceData> sourceList = new ArrayList<>();
    int currentSourceIndex = 0;
    /**
     * volatile: 预取线程({@code jc-prefetch} 池)会读它做URL 键匹配与下钻, 而主线程在
     * {@link #setRawUrls} 里整体重新赋值。缺 happens-before 时预取线程可能看到新引用
     * 但未完全可见的数组内容 → {@code get()} 越界(被静默 catch)或拿到 null, 表现为
     * "预取悄悄失效"。与下面各volatile 字段是同一类问题。
     */
    volatile List<String> playlistTitles = new ArrayList<>();
    /** 上一次的集索引 — onMediaItemTransition 里作为 fromIndex 回传(前端清上一集记忆). */
    int lastMediaItemIndex = 0;
    /** 本次装载是否走"单URL 兼容模式"(不做任何包装) — 重载时要沿用同一模式. 预取线程会读。 */
    volatile boolean currentBypassFilter = false;

    // ===== 广告过滤 / 线路 =====
    String proxyBase = "";
    volatile boolean adFilterOn = true;
    /**
     * 当前片源能否走服务端清单代理(来自 adFilterOk)。false ⇒ 清单由设备直连抓 +
     * 端侧混合过滤剔广告, 且"端侧失败升级代理"/"中转"都不可用(服务端抓不到, 包了也白搭)。
     * 只有显式 false 才为 false, 未测(null)沿用"可用"。
     */
    volatile boolean sourceProxyUsable = true;
    /**
     * 本片源端侧过滤已失败过一次 → 后续集直接用服务端代理。
     * 为什么需要: 端侧过滤是主路径, 若某源的大清单 POST 稳定超时(12s), 没有这个粘性偏好就会
     * **每集都白等一轮超时**再升级。换片/换源(startFromIntent)时重置。
     */
    volatile boolean sourcePreferProxy = false;
    volatile List<String> currentRawUrls = new ArrayList<>();
    /**
     * 三个"强制线路"集索引: **主线程写, 预取线程读**(读点在
     * {@link #mediaUriFor}, 它被 {@code PlayerPrefetchHelper} 的池线程调用)。
     *
     * <p>必须用并发集合: 写方会在 {@code onPlayerError} 里 {@code add}, 触发 HashMap 扩容;
     * 读方同时在 {@code contains}。非并发 HashSet 在 resize 与并发 get 交错时可能读到
     * 环形桶链(经典死循环)或返回错误结果 → 预取误判线路 → 白发一次注定不被消费的
     * 过滤请求, 或无谓放弃预取。
     */
    /** 代理失败的集 → 强制用原始地址. */
    final Set<Integer> forceRawIdx = ConcurrentHashMap.newKeySet();
    /** 端侧过滤失败的集 → 强制用服务端代理(proxyMedia=0)。 */
    final Set<Integer> forceProxyIdx = ConcurrentHashMap.newKeySet();
    /** 直连失败的集 → 强制全量中转(单集自愈; 与用户的"中转"开关无关). */
    final Set<Integer> forceRelayIdx = ConcurrentHashMap.newKeySet();
    /**
     * 用户的中转开关(默认关 = 不包代理, 走端侧混合过滤) — 全局生效, 持久化在 PlayerNetworkModeHelper.
     * 开 = 优先走服务端代理且分片全中转(proxyMedia=1)。
     * 与 forceRelayIdx 的差别: 这个是"用户意图", 那个是"单集自愈".
     */
    volatile boolean relayOn = false;
    volatile int pendingFilteredCount = 0;
    volatile boolean filterAttempted = false;
    volatile boolean filterFailed = false;
    volatile boolean filterProxyMissing = false;
    boolean filterToastShownForEpisode = false;

    // ===== 端侧预取缓存(下一集清单) =====
    /**
     * 预取条目有效期 — 与服务端 m3u8 缓存(10min)对齐. 过期即弃用, 切集时按原路径重来,
     * 不会拿一份放旧了的清单(源站清单可能带时效签名)。
     */
    static final long PREFETCH_TTL_MS = 10 * 60 * 1000L;
    /** 缓存条数上限(单条 10~100KB) — TV 端内存有限, 只留最近几集(master + 子表)。 */
    static final int PREFETCH_MAX_ENTRIES = 8;

    /**
     * 一条"已过滤清单"预取结果: key = 原始清单 URL. 由 {@link PlayerPrefetchHelper} 写入,
     * {@link PlayerAdFilterHelper}(解析器) 消费 —— 两者不能互相引用, 所以放这里做共享点。
     */
    public static class Prefetched {
        public final byte[] data;
        /** 该层清单被剔除的分片数(master 恒为 0, 广告在子表层)。 */
        public final int filteredCount;
        final long atMs;

        Prefetched(byte[] data, int filteredCount, long atMs) {
            this.data = data;
            this.filteredCount = filteredCount;
            this.atMs = atMs;
        }
    }

    private final java.util.Map<String, Prefetched> prefetched =
            new java.util.concurrent.ConcurrentHashMap<>();
    /**
     * 预取代际 — 装载/换源/开关切换(即 {@link #invalidatePrefetch()})后自增:
     * 在途的预取任务写完缓存前比对代际, 不一致就丢弃结果(否则换片后后台还在写上一部片的缓存)。
     */
    private final java.util.concurrent.atomic.AtomicInteger prefetchGen =
            new java.util.concurrent.atomic.AtomicInteger();

    // ===== 跳过片头/片尾 =====
    long skipIntroMs = PlayerSkipHelper.DEFAULT_SKIP_INTRO_MS;
    long skipOutroMs = PlayerSkipHelper.DEFAULT_SKIP_OUTRO_MS;
    /** 跳过总开关 — 关时即使 skipIntroMs/OutroMs > 0 也不执行. */
    boolean skipEnabled = false;
    boolean autoNext = true;
    boolean introSkippedForCurrent = false;
    boolean outroPromptShown = false;
    /** 正在切集(含自动连播): 期间抑制缓冲抖动带来的误判. */
    boolean episodeSwitching = false;

    public PlayerSession(Host host) {
        this.host = host;
    }

    public Host host() { return host; }

    public Context context() { return host.context(); }

    // ============================ 片源装载 ============================

    /** 当前片源 id(多源时 = sourceList 选中项 id), 供回传前端更新历史片源. */
    String currentSourceLabel() {
        if (currentSourceIndex >= 0 && currentSourceIndex < sourceList.size()) {
            return sourceList.get(currentSourceIndex).id;
        }
        return "";
    }

    /** 设置当前源的"原始" m3u8 列表(供代理包装/反馈/回退). */
    void setRawUrls(List<String> urls) {
        currentRawUrls = new ArrayList<>(urls);
    }

    /** 切源/切集时重置线路自愈状态. */
    void resetLineState() {
        forceRawIdx.clear();
        forceProxyIdx.clear();
        forceRelayIdx.clear();
    }

    /** 每集起播时重置本集过滤统计(供 STATE_READY 弹一次状态). */
    void resetFilterStateForEpisode() {
        pendingFilteredCount = 0;
        filterAttempted = false;
        filterFailed = false;
        filterProxyMissing = false;
        filterToastShownForEpisode = false;
    }

    /** 把指定 source 的 episodes 装入 player; 从 startEpisodeIndex 开始, resumeMs 续播. */
    void loadSourceIntoPlayer(int sourceIdx, int startEpisodeIndex, long resumeMs) {
        if (sourceIdx < 0 || sourceIdx >= sourceList.size()) return;
        SourceData src = sourceList.get(sourceIdx);
        // 跟随片源切换刷新"服务端能否代理": 服务端抓不到的源(如 bf/360)不能走代理, 也不能中转,
        // 只能"设备抓清单 + /v1/m3u8/filter 端侧过滤"。
        sourceProxyUsable = src.adFilterOk == null || src.adFilterOk;
        // 换源 = 换了一个片源, 上一源"端侧过滤不可靠"的结论不能继承。
        // 不清的话: 片源 A 有一次弱网超时 → sourcePreferProxy 永久为 true → 切到片源 B 后
        // PlayerPrefetchHelper.canPrefetch 的 `raw.equals(mediaUriFor(...))` 恒为 false
        // → **整个新片源的预取加速器被永久关掉**, 而 B 的端侧过滤其实完全正常。
        if (sourceIdx != currentSourceIndex) {
            sourcePreferProxy = false;
        }
        loadPlaylistIntoPlayer(src.urls, src.titles, startEpisodeIndex, resumeMs, false);
    }

    /**
     * 装载并起播一组地址 — 三种启动模式(多源 / 单源 playlist / 单 URL)的唯一出口.
     * 装载序列只此一份, 各模式只决定"装哪些地址、要不要走代理包装".
     *
     * @param rawUrls      原始地址列表(供线路切换与失败回退)
     * @param titles       每集标题, 可为 null
     * @param startIndex   起始集
     * @param resumeMs     续播位置
     * @param bypassFilter true = 直接用原始地址(单 URL 兼容模式, 不做代理包装)
     */
    void loadPlaylistIntoPlayer(List<String> rawUrls, List<String> titles,
                                int startIndex, long resumeMs, boolean bypassFilter) {
        if (player == null || rawUrls == null || rawUrls.isEmpty()) return;
        // playlistTitles 必须与 rawUrls **等长**: 它被多处当作"集数"基准 ——
        // PlayerActivity 角标 `共 N 集`、PlayerDialogHelper.showEpisodeDialog 的分段与条目。
        // 两个列表来自 Intent 的两个独立 extra, 壳层若传 20 个 URL 但只给 12 个标题,
        // 角标会显示"共 12 集"且最后 8 集在"选集"里根本选不到。缺失项补占位标题。
        List<String> aligned = new ArrayList<>(rawUrls.size());
        for (int i = 0; i < rawUrls.size(); i++) {
            String t = (titles != null && i < titles.size()) ? titles.get(i) : null;
            aligned.add((t == null || t.isEmpty()) ? "第 " + (i + 1) + " 集" : t);
        }
        playlistTitles = aligned;
        currentBypassFilter = bypassFilter;
        setRawUrls(rawUrls);
        resetLineState();
        // 装载 = 换片/换源/开关切换: 上一份播放列表的预取清单全部作废(切集不走这里, 缓存能活过切集)
        invalidatePrefetch();
        List<MediaItem> items = new ArrayList<>(rawUrls.size());
        for (int i = 0; i < rawUrls.size(); i++) {
            String raw = rawUrls.get(i);
            items.add(MediaItem.fromUri(bypassFilter ? raw : mediaUriFor(i, raw)));
        }
        int safeStart = Math.max(0, Math.min(startIndex, items.size() - 1));
        introSkippedForCurrent = false;
        outroPromptShown = false;
        player.setMediaItems(items, safeStart, Math.max(0L, resumeMs));
        player.prepare();
        player.setPlayWhenReady(true);
        host.updateTitleForCurrent();
        updateNetworkModeUi();
    }

    /**
     * 重新装载当前集并保留进度 — 过滤开关/中转开关切换后调用.
     *
     * 两种模式都要覆盖: 多源(sourceList 非空)走 loadSourceIntoPlayer; 单源(v2 playlist /
     * 单 URL, sourceList 为空)原先会在这里早退 ⇒ **切换过滤/中转完全没反应**, 现按当前
     * 原始地址表原地重载(沿用本次的 bypassFilter 模式)。
     */
    void reloadCurrentSourceKeepPosition() {
        int idx = player != null ? player.getCurrentMediaItemIndex() : 0;
        long pos = player != null ? player.getCurrentPosition() : 0L;
        if (!sourceList.isEmpty()
                && currentSourceIndex >= 0 && currentSourceIndex < sourceList.size()) {
            loadSourceIntoPlayer(currentSourceIndex, idx, pos);
            return;
        }
        if (currentRawUrls.isEmpty()) return;
        loadPlaylistIntoPlayer(currentRawUrls, playlistTitles, idx, pos, currentBypassFilter);
    }

    // ============================ 线路 / 自愈 ============================

    String mediaUriFor(int idx, String rawUrl) {
        // 离线播放: 清单是本地 file://(已过滤), 任何代理包装都是"包一个服务端抓不到的地址"
        if (offlinePlayback) return rawUrl;
        // forceProxyIdx = 本集端侧过滤失败后的升级; sourcePreferProxy = 本片源端侧过滤"坏过"的粘性偏好
        boolean forceProxy = forceProxyIdx.contains(idx) || sourcePreferProxy;
        return PlayerUrls.buildPlayableUrl(
                rawUrl, adFilterOn, forceRawIdx.contains(idx), forceProxy, isRelay(idx),
                sourceProxyUsable, proxyBase);
    }

    /** 当前集是否走全量中转: 用户开关开着, 或本集被自愈标记(且没被强制回原始). */
    boolean isRelay(int idx) {
        if (forceRawIdx.contains(idx)) return false;
        return relayOn || forceRelayIdx.contains(idx);
    }

    /** 当前视频能否切换线路(仅"直连 CDN 的 m3u8 + 开关开启 + 有代理地址 + 服务端抓得到该源"). */
    boolean canSwitchNetworkMode(int idx) {
        // 离线播放没有"线路"概念: 分片在本地缓存, 中转/直连都救不了缺失的分片
        if (offlinePlayback) return false;
        if (!adFilterOn || !sourceProxyUsable || proxyBase == null || proxyBase.isEmpty()) return false;
        if (idx < 0 || idx >= currentRawUrls.size()) return false;
        String raw = currentRawUrls.get(idx).toLowerCase(Locale.US);
        return raw.contains(".m3u8") && !raw.contains("/m3u8/proxy?");
    }

    // 手动切换线路(直连 ⇄ 中转)不在这里 —— 它需要读/写 Context 落盘开关态,
    // 由 PlayerNetworkModeHelper.toggle() 实现, 经 Host.toggleNetworkMode() 转发。

    /** 用当前线路策略重建某集并恢复进度(线路切换 / 失败自愈). */
    void retryCurrentItem(int idx, String message) {
        if (player == null || idx < 0 || idx >= currentRawUrls.size()) return;
        final long pos = Math.max(0L, player.getCurrentPosition());
        final String raw = currentRawUrls.get(idx);
        new Handler(Looper.getMainLooper()).post(() -> {
            try {
                // 重建同一集不会触发 onMediaItemTransition ⇒ 必须在这里显式清过滤状态,
                // 否则上一轮残留的 filterFailed 会让角标一直显示"过滤失败"(且盖掉正确的"服务端过滤中").
                // 顺带把 filterToastShownForEpisode 也复位 → 新链路 READY 后角标与中央提示都会按当前真实态刷新。
                resetFilterStateForEpisode();
                player.replaceMediaItem(idx, MediaItem.fromUri(mediaUriFor(idx, raw)));
                player.seekTo(idx, pos);
                player.prepare();
                player.play();
                updateNetworkModeUi();
                host.showCenterToast(message, 2500);
            } catch (Exception ignore) {
            }
        });
    }

    /** 刷新底栏"中转"按钮的状态点 — 经 Host 出 UI, 本类不再直接持有/写 View. */
    void updateNetworkModeUi() {
        host.renderNetworkMode(relayOn);
    }

    void markEpisodeSwitching() {
        episodeSwitching = true;
    }

    // ============================ 端侧预取缓存 ============================

    /** 取预取结果(命中且未过期); 无/已过期返回 null. 过期条目顺手清掉. */
    Prefetched takePrefetched(String url) {
        if (url == null) return null;
        Prefetched p = prefetched.get(url);
        if (p == null) return null;
        if (System.currentTimeMillis() - p.atMs > PREFETCH_TTL_MS) {
            prefetched.remove(url);
            return null;
        }
        return p;
    }

    /** 写入预取结果; 代际不符(期间已换片/换源)则丢弃. */
    void putPrefetched(String url, byte[] data, int filteredCount, int gen) {
        if (url == null || data == null || data.length == 0) return;
        if (gen != prefetchGen.get()) return;
        if (prefetched.size() >= PREFETCH_MAX_ENTRIES && !prefetched.containsKey(url)) {
            // 条目很少(≤8), 直接淘汰最旧的一条, 不值当上 LRU
            String oldestKey = null;
            long oldestAt = Long.MAX_VALUE;
            for (java.util.Map.Entry<String, Prefetched> e : prefetched.entrySet()) {
                if (e.getValue().atMs < oldestAt) {
                    oldestAt = e.getValue().atMs;
                    oldestKey = e.getKey();
                }
            }
            if (oldestKey != null) prefetched.remove(oldestKey);
        }
        prefetched.put(url, new Prefetched(data, filteredCount, System.currentTimeMillis()));
    }

    /**
     * 作废全部预取并抬代际(在途任务会自行放弃).
     * 调用点: 装载播放列表(换片/换源/切线路开关重载)时 —— 见 {@link #loadPlaylistIntoPlayer}。
     * 切集**不**走装载路径, 所以预取结果能活过切集、正好被下一集消费。
     */
    void invalidatePrefetch() {
        prefetched.clear();
        prefetchGen.incrementAndGet();
    }

    /** 当前预取代际 — 预取任务开始/写回时取快照比对. */
    int prefetchGeneration() {
        return prefetchGen.get();
    }

    /** 某 URL 的预取结果年龄(ms); 无缓存返回 -1 —— 供预取器判断"是否还新鲜、值不值得刷新". */
    long prefetchedAgeMs(String url) {
        if (url == null) return -1L;
        Prefetched p = prefetched.get(url);
        return (p == null) ? -1L : System.currentTimeMillis() - p.atMs;
    }
}
