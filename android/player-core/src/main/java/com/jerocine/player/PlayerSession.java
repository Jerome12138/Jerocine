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
import java.util.Collections;
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

        /** 切换当前集 本地 ⇄ 在线(底栏"切本地/切在线"按钮调用; 无本地副本时为 no-op). */
        void toggleLocalOnline();

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

    // ===== 本地优先播放(已下载集默认播本地缓存) =====
    /**
     * episode index -> 本地过滤后清单 URI(file://…/playlist.m3u8)。装载播放列表时
     * 重查一次下载业务表(见 refreshLocalDownloads), 只含**已完成且清单文件存在**的集。
     *
     * <p>2026-10-09 重构: 原独立的"离线播放模式"(offlinePlayback, 下载页播单集)已并入
     * 本表 —— 无影片上下文时PlayerActivity 会把单条 file:// 清单装进本表(见
     * {@link #isSingleLocalPlaylist}), 判定/文案/自愈/预取全部走
     * {@link PlayerModes#isEpisodePlayingLocal} 一个出口, 不再有第二套状态。
     */
    volatile java.util.Map<Integer, String> localEpisodePlaylists = java.util.Collections.emptyMap();
    /**
     * 本集在**别的源**有已完成下载(episode → 任务)。多源严格匹配下不参与自动本地优先
     * (不同源的集号/内容可能对不齐), 但要在更多菜单里可见可操作 —— 否则用户从历史
     * 恢复到另一个源时, 明明下载过却既不播本地、菜单也没有任何提示(2026-10-09 真机实锤:
     * 下载时 lz 源, 续播恢复成 bf 源 → 菜单无切换项, 用户以为功能坏了)。
     */
    volatile java.util.Map<Integer, com.jerocine.player.download.DownloadTask> otherSourceLocalEpisodes =
            java.util.Collections.emptyMap();
    /** 已弹过"别源已下载"提示的集(会话级, 每集只弹一次)。 */
    public final java.util.Set<Integer> otherSourceToastShown = newConcurrentKeySet();
    /** 用户显式切回在线的集(底栏「切本地/切在线」; 会话级偏好, 不落盘)。 */
    public final java.util.Set<Integer> preferOnlineIdx = newConcurrentKeySet();

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

    // ===== 广告过滤 =====
    volatile boolean adFilterOn = true;
    volatile List<String> currentRawUrls = new ArrayList<>();

    /**
     * ConcurrentHashMap.newKeySet() 的 API 21 等价物 —— newKeySet 是 API 24+,
     * Android 6 老设备(坚果投影)会 NoSuchMethodError(2026-10-15 lint 全量梳理)。
     */
    static <T> Set<T> newConcurrentKeySet() {
        return Collections.newSetFromMap(new ConcurrentHashMap<T, Boolean>());
    }
    volatile int pendingFilteredCount = 0;
    volatile boolean filterAttempted = false;
    volatile boolean filterFailed = false;
    boolean filterToastShownForEpisode = false;
    /**
     * 最近一次清单解析(FilterPlaylistParser.parse)的 URI — parse 在 loader 线程写,
     * 主线程 transition 回调读。切集 reset 的竞态防护: 主线程卡顿时下一集的解析可能
     * **先于** transition 回调完成, 此时 parse 写下的过滤状态就是本集的, 不能抹
     * (抹掉会让 READY 的状态提示误报"该源无需过滤"/丢失"已过滤 N 段", 2026-10-10 用户实测)。
     */
    volatile String lastParsedPlaylistUrl = "";

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
    /** 底部按钮行"上次选中"记忆(View id, 0=无): 进度条按 ↓ 恢复焦点用(2026-10-11 用户要求,
     * 有记忆恢复记忆按钮, 无记忆落行内第一个可见按钮, 不再恒落「更多」). */
    int lastBottomBtnId = 0;

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

    /** 设置当前源的"原始" m3u8 列表(供回退/遥测). */
    void setRawUrls(List<String> urls) {
        currentRawUrls = new ArrayList<>(urls);
    }

    /** 每集起播时重置本集过滤统计(供 STATE_READY 弹一次状态). */
    void resetFilterStateForEpisode() {
        // toast 门必须每集重置(即使下面竞态跳过了状态重置, 新集也要能弹自己的状态)
        filterToastShownForEpisode = false;
        // 竞态防护(2026-10-10): parse 跑在 loader 线程, 本方法在主线程 transition 回调里。
        // 主线程卡顿时, 下一集的清单解析可能**先于**本回调完成 —— parse 写下的 attempted/
        // pendingFilteredCount 就是本集的结果, 抹掉会让 READY 时误报"该源无需过滤"。
        // 判据: 最近一次解析的 URI == 当前媒体项 URI → 状态属于本集, 跳过重置。
        String cur = currentMediaUriOrNull();
        if (!lastParsedPlaylistUrl.isEmpty() && lastParsedPlaylistUrl.equals(cur)) {
            return;
        }
        lastParsedPlaylistUrl = "";
        pendingFilteredCount = 0;
        filterAttempted = false;
        filterFailed = false;
    }

    /** 当前媒体项 URI; player 未就绪/无项返回空串。 */
    private String currentMediaUriOrNull() {
        try {
            if (player != null && player.getCurrentMediaItem() != null
                    && player.getCurrentMediaItem().localConfiguration != null) {
                return String.valueOf(player.getCurrentMediaItem().localConfiguration.uri);
            }
        } catch (Exception ignore) {
            /* player 未就绪: 视为无 URI */
        }
        return "";
    }

    /** 把指定 source 的 episodes 装入 player; 从 startEpisodeIndex 开始, resumeMs 续播. */
    void loadSourceIntoPlayer(int sourceIdx, int startEpisodeIndex, long resumeMs) {
        if (sourceIdx < 0 || sourceIdx >= sourceList.size()) return;
        SourceData src = sourceList.get(sourceIdx);
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
        // 本地优先: 装载时重查一次已下载集(轻量查询, 每次装载只这一次)。
        // 单 URL 兼容模式无剧集概念, 不查。
        if (!bypassFilter) {
            refreshLocalDownloads(host.filmId(), currentSourceLabel());
        }
        setRawUrls(rawUrls);
        // 装载 = 换片/换源/开关切换: 上一份播放列表的预取清单全部作废(切集不走这里, 缓存能活过切集)
        invalidatePrefetch();
        List<MediaItem> items = new ArrayList<>(rawUrls.size());
        for (int i = 0; i < rawUrls.size(); i++) {
            String raw = rawUrls.get(i);
            items.add(MediaItem.fromUri(bypassFilter ? raw : playbackUriFor(i, raw)));
        }
        int safeStart = Math.max(0, Math.min(startIndex, items.size() - 1));
        introSkippedForCurrent = false;
        outroPromptShown = false;
        // 装载即"切集窗口"(2026-10-18 修): setMediaItems 首次装载**不触发** onMediaItemTransition,
        // 旧逻辑只靠切集路径 markEpisodeSwitching ⇒ 冷启动起播的过滤窗口里
        // isFilteringWindow 恒 false —— 「正在准备视频 · 广告过滤中」不显示、普通缓冲提示
        // 也无人抑制。STATE_READY 时统一复位, 本地集/过滤关时 isFilteringWindow 自然为 false。
        markEpisodeSwitching();
        player.setMediaItems(items, safeStart, Math.max(0L, resumeMs));
        player.prepare();
        player.setPlayWhenReady(true);
        host.updateTitleForCurrent();
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

    // ============================ 本地优先播放 ============================

    /**
     * 重查已下载集映射: filmId(+当前源) 的已完成任务, 清单文件存在的入表。
     * 每次装载播放列表调一次; 查询失败保持旧映射(在线播放不受影响)。
     *
     * <p>主线程轻量 SQLite 查询(任务表≤数百行), 换取装载期一次性的本地可用性判定;
     * 若将来表规模失控, 应改成异步预查 + 装载时用快照。
     */
    void refreshLocalDownloads(String filmId, String sourceKey) {
        final String TAG = "JcLocal";
        if (localPlayback || filmId == null || filmId.isEmpty()) {
            android.util.Log.i(TAG, "skip: localPlayback=" + localPlayback + " filmId=" + filmId);
            localEpisodePlaylists = java.util.Collections.emptyMap();
            otherSourceLocalEpisodes = java.util.Collections.emptyMap();
            return;
        }
        try {
            com.jerocine.player.download.DownloadEngine e =
                    com.jerocine.player.download.DownloadEngine.existing();
            if (e == null) {
                // 冷启动直接进播放器(继续观看)时引擎尚未初始化 —— 不查就是空表,
                // 本地优先整个失效(2026-10-08 真机实锤)。这里惰性初始化: 只建 DB/线程池,
                // 无下载任务时无副作用; 与下载服务(同进程单例)天然共享。
                try {
                    e = com.jerocine.player.download.DownloadEngine.get(context());
                    android.util.Log.i(TAG, "lazy init engine ok=" + (e != null));
                } catch (Exception ex) {
                    android.util.Log.w(TAG, "lazy init engine FAILED", ex);
                }
            }
            if (e == null) {
                android.util.Log.i(TAG, "engine null -> empty map");
                localEpisodePlaylists = java.util.Collections.emptyMap();
                otherSourceLocalEpisodes = java.util.Collections.emptyMap();
                return;
            }
            java.util.Map<Integer, String> m = new java.util.HashMap<>();
            java.util.Map<Integer, com.jerocine.player.download.DownloadTask> other =
                    new java.util.HashMap<>();
            java.util.List<com.jerocine.player.download.DownloadTask> tasks =
                    e.repository().listByFilm(filmId);
            for (com.jerocine.player.download.DownloadTask t : tasks) {
                android.util.Log.i(TAG, "task ep=" + t.episode + " state=" + t.state
                        + " srcKey=" + t.sourceKey + " (cur=" + sourceKey + ")"
                        + " cacheDir=" + t.cacheDir);
                if (t.state != com.jerocine.player.download.DownloadTask.STATE_COMPLETED) continue;
                if (t.cacheDir == null || t.cacheDir.isEmpty()) continue;
                java.io.File p = new java.io.File(t.cacheDir, "playlist.m3u8");
                if (!p.exists()) continue;
                // 多源模式严格匹配源 id, 避免播到另一条线路的缓存;
                // 单源模式(sourceList 空)拿不到 sourceKey → 接受该片任何源的已完成任务。
                if (sourceKey != null && !sourceKey.isEmpty() && !sourceKey.equals(t.sourceKey)) {
                    // putIfAbsent 是 API 24+ (Map default 方法), Android 6 会 NoSuchMethodError —— 用 containsKey+put
                    if (!other.containsKey(t.episode)) {
                        other.put(t.episode, t); // 别源已完成: 供更多菜单"换源播本地"
                    }
                    continue;
                }
                m.put(t.episode, "file://" + p.getAbsolutePath());
            }
            localEpisodePlaylists = m;
            otherSourceLocalEpisodes = other;
            android.util.Log.i(TAG, "matched " + m.size() + "/" + tasks.size()
                    + " local episodes, otherSource=" + other.size()
                    + " (filmId=" + filmId + " sourceKey=" + sourceKey + ")");
        } catch (Exception ex) {
            // 查询异常: 保持旧映射, 不影响本次装载的在线播放
            android.util.Log.w(TAG, "refresh FAILED", ex);
        }
    }

    /** 该集是否有可用的本地缓存(已完成下载且本地清单存在)。 */
    boolean isLocalEpisode(int idx) {
        return localEpisodePlaylists.containsKey(idx);
    }

    /** 本集是否在**别的源**有已完成下载(更多菜单"换源播本地"的判定)。 */
    boolean otherSourceHasEpisode(int idx) {
        return otherSourceLocalEpisodes.containsKey(idx);
    }

    /** 取本集在别源的下载任务(仅当 {@link #otherSourceHasEpisode} 为 true)。 */
    com.jerocine.player.download.DownloadTask otherSourceTask(int idx) {
        return otherSourceLocalEpisodes.get(idx);
    }

    /** 按 sourceKey 找 sourceList 下标; 找不到(源列表变动)返回 -1。 */
    int indexOfSourceKey(String key) {
        if (key == null || key.isEmpty()) return -1;
        for (int i = 0; i < sourceList.size(); i++) {
            if (key.equals(sourceList.get(i).id)) return i;
        }
        return -1;
    }

    /**
     * 播放 URI 决策(装载/重试统一入口):
     * 已下载集默认播本地(file:// 过滤后清单, 分片由 RoutingDataSource 从下载缓存取),
     * 用户在底栏/更多里切回在线(preferOnlineIdx)或无缓存时走在线链路(mediaUriFor)。
     */
    String playbackUriFor(int idx, String rawUrl) {
        String local = localEpisodePlaylists.get(idx);
        if (local != null && !preferOnlineIdx.contains(idx)) return local;
        return rawUrl;
    }

    /** 会话是否是"单条本地清单"(下载页无影片上下文播单集), 判据见 {@link PlayerModes}。 */
    boolean isSingleLocalPlaylist() {
        return PlayerModes.isSingleLocalPlaylist(currentBypassFilter, currentRawUrls);
    }

    /**
     * <b>统一判定: 第 idx 集现在是否在播本地</b> —— 角标/底栏按钮/线路自愈/预取/
     * 过滤提示全部只看这里, 禁止再散落自写 file:// 前缀判断以外的第二套状态。
     */
    public boolean isEpisodePlayingLocal(int idx) {
        return PlayerModes.isEpisodePlayingLocal(
                localPlayback, isSingleLocalPlaylist(), localEpisodePlaylists, preferOnlineIdx, idx);
    }

    /** 第 idx 集有无本地副本且存在在线替代(底栏切换按钮显隐); 单集本地/本地文件模式无在线可切。 */
    public boolean isEpisodeLocalAvailable(int idx) {
        return PlayerModes.isEpisodeLocalAvailable(
                localPlayback, isSingleLocalPlaylist(), localEpisodePlaylists, idx);
    }

    /**
     * 当前集在指定源是否已有已下载副本(换源弹窗标记用)。
     * 当前源看本地副本表; 其他源看别源任务表(otherSourceLocalEpisodes 只存与当前源不匹配的任务, 正好对口)。
     */
    public boolean isEpisodeDownloadedOnSource(int episode, int sourceIdx) {
        if (sourceIdx < 0 || sourceIdx >= sourceList.size()) return false;
        if (sourceIdx == currentSourceIndex) return localEpisodePlaylists.containsKey(episode);
        com.jerocine.player.download.DownloadTask t = otherSourceLocalEpisodes.get(episode);
        String key = sourceList.get(sourceIdx).id;
        return t != null && key != null && key.equals(t.sourceKey);
    }

    /** 用当前播放策略重建某集并恢复进度(线路切换 / 失败自愈). */
    void retryCurrentItem(int idx, String message) {
        if (player == null || idx < 0 || idx >= currentRawUrls.size()) return;
        final long pos = Math.max(0L, player.getCurrentPosition());
        final String raw = currentRawUrls.get(idx);
        new Handler(Looper.getMainLooper()).post(() -> {
            try {
                // 重建同一集不会触发 onMediaItemTransition ⇒ 必须在这里显式清过滤状态,
                // 否则上一轮残留的 filterFailed 会让角标一直显示"过滤失败"。
                // 顺带把 filterToastShownForEpisode 也复位 → 新链路 READY 后角标与中央提示都会按当前真实态刷新。
                resetFilterStateForEpisode();
                player.replaceMediaItem(idx, MediaItem.fromUri(playbackUriFor(idx, raw)));
                player.seekTo(idx, pos);
                player.prepare();
                player.play();
                host.showCenterToast(message, 2500);
            } catch (Exception ignore) {
            }
        });
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
