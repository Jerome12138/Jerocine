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
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

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

        /** 打开"播放控制"菜单(由 DialogHelper 实现, 供按键分发调用). */
        void showPlayMenu();

        /** 切换广告过滤开关(由 AdFilterHelper 实现, 供菜单调用). */
        void toggleAdFilter();

        /** 刷新线路角标与"线路"按钮文案(relay = 当前集是否走全量中转). */
        void renderNetworkMode(boolean relay);

        /** 播放器视图 — 壳层装配的实例, helper 做面板显隐/取子控件时经此访问. */
        PlayerView playerView();

        /** 刷新广告过滤角标; text 为 null 表示只改显隐(隐藏时不必刷文案). */
        void renderAdFilterBadge(boolean visible, String text);

        /** 刷新倍速角标文案. */
        void renderSpeedText(String label);

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

    // ===== 片源 =====
    final ArrayList<SourceData> sourceList = new ArrayList<>();
    int currentSourceIndex = 0;
    List<String> playlistTitles = new ArrayList<>();
    /** 上一次的集索引 — onMediaItemTransition 里作为 fromIndex 回传(前端清上一集记忆). */
    int lastMediaItemIndex = 0;

    // ===== 广告过滤 / 线路 =====
    String proxyBase = "";
    volatile boolean adFilterOn = true;
    List<String> currentRawUrls = new ArrayList<>();
    /** 代理失败的集 → 强制用原始地址. */
    final Set<Integer> forceRawIdx = new HashSet<>();
    /** 直连分片失败的集 → 强制全量中转. */
    final Set<Integer> forceRelayIdx = new HashSet<>();
    volatile int pendingFilteredCount = 0;
    volatile boolean filterAttempted = false;
    volatile boolean filterFailed = false;
    volatile boolean filterProxyMissing = false;
    boolean filterToastShownForEpisode = false;

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
        playlistTitles = (titles != null) ? new ArrayList<>(titles) : new ArrayList<>();
        setRawUrls(rawUrls);
        resetLineState();
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

    /** 重新装载当前集并保留进度 — 过滤开关/线路切换后调用. */
    void reloadCurrentSourceKeepPosition() {
        int idx = player != null ? player.getCurrentMediaItemIndex() : 0;
        long pos = player != null ? player.getCurrentPosition() : 0L;
        loadSourceIntoPlayer(currentSourceIndex, idx, pos);
    }

    // ============================ 线路 / 自愈 ============================

    String mediaUriFor(int idx, String rawUrl) {
        return PlayerUrls.buildPlayableUrl(
                rawUrl, adFilterOn, forceRawIdx.contains(idx), forceRelayIdx.contains(idx), proxyBase);
    }

    /** 当前集是否走全量中转. */
    boolean isRelay(int idx) {
        return forceRelayIdx.contains(idx) && !forceRawIdx.contains(idx);
    }

    /** 当前视频能否切换线路(仅"直连 CDN 的 m3u8 + 开关开启 + 有代理地址"). */
    boolean canSwitchNetworkMode(int idx) {
        if (!adFilterOn || proxyBase == null || proxyBase.isEmpty()) return false;
        if (idx < 0 || idx >= currentRawUrls.size()) return false;
        String raw = currentRawUrls.get(idx).toLowerCase(Locale.US);
        return raw.contains(".m3u8") && !raw.contains("/m3u8/proxy?");
    }

    /** 手动切换本集线路(直连 ⇄ 中转). */
    void toggleNetworkMode() {
        int idx = player != null ? player.getCurrentMediaItemIndex() : -1;
        if (!canSwitchNetworkMode(idx)) {
            host.showCenterToast("当前视频不支持线路切换", 1500);
            return;
        }
        boolean relay = !forceRelayIdx.contains(idx);
        forceRawIdx.remove(idx);
        if (relay) forceRelayIdx.add(idx); else forceRelayIdx.remove(idx);
        retryCurrentItem(idx, relay ? "已切换到中转" : "已切换到直连");
    }

    /** 用当前线路策略重建某集并恢复进度(线路切换 / 失败自愈). */
    void retryCurrentItem(int idx, String message) {
        if (player == null || idx < 0 || idx >= currentRawUrls.size()) return;
        final long pos = Math.max(0L, player.getCurrentPosition());
        final String raw = currentRawUrls.get(idx);
        new Handler(Looper.getMainLooper()).post(() -> {
            try {
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

    /** 刷新线路角标与"线路"按钮文案 — 经 Host 出 UI, 本类不再直接持有/写 View. */
    void updateNetworkModeUi() {
        int idx = player != null ? player.getCurrentMediaItemIndex() : -1;
        host.renderNetworkMode(isRelay(idx));
    }

    void markEpisodeSwitching() {
        episodeSwitching = true;
    }
}
