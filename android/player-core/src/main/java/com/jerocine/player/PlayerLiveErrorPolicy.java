package com.jerocine.player;

import androidx.media3.common.PlaybackException;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylistTracker;
import androidx.media3.exoplayer.source.BehindLiveWindowException;

/**
 * Q②/PB3(player-fix 2026-11-05) — 直播 behind-live/reset 类报错的兜底判定与防循环闸门。
 *
 * <p>media3 标准处方: 直播落后于边缘(seekable 窗口整体滚动出播放位置)时,
 * {@code seekToDefaultPosition() + prepare()} 重定位到当前滑窗边缘即可续播, 不该弹错退出。
 * 判定与限次都抽成纯逻辑, 供 {@link PlayerActivity#onPlayerError} 前置短路。
 */
public final class PlayerLiveErrorPolicy {

    /** 短窗内最多兜底次数: 一次 seek+prepare 足以重定位滑窗; 窗内再报即重试未生效, 止损走原报错路径。 */
    public static final int MAX_RECOVERS_PER_WINDOW = 1;
    /**
     * 防循环短窗时长: 覆盖 2~3 个常见 targetDuration(8~10s)。窗口内反复报
     * behind-live/reset 说明兜底没起效(死循环风险), 立即停; 超窗后允许再试(保可用)。
     */
    public static final long RECOVERY_WINDOW_MS = 30_000L;

    private PlayerLiveErrorPolicy() {
    }

    /**
     * 是否属于"直播落后于边缘"类可自愈错误:
     * ① {@code errorCode == ERROR_CODE_BEHIND_LIVE_WINDOW}; 或
     * ② cause 链上任一环是 {@link HlsPlaylistTracker.PlaylistResetException} /
     * {@link BehindLiveWindowException}。
     *
     * <p>{@link HlsPlaylistTracker.PlaylistStuckException} 刻意不在列(规格 Q② 只收
     * PlaylistReset/BehindLive): Stuck 是"快照冻结"类失败, seek+prepare 重抓清单治不了,
     * 兜底反而掩盖真问题 —— Q① 清单免缓存后它本就不该再出现, 保留报错可见性。
     */
    public static boolean isBehindLiveError(int errorCode, Throwable cause) {
        if (errorCode == PlaybackException.ERROR_CODE_BEHIND_LIVE_WINDOW) {
            return true;
        }
        for (Throwable t = cause; t != null; t = t.getCause()) {
            if (t instanceof HlsPlaylistTracker.PlaylistResetException
                    || t instanceof BehindLiveWindowException) {
                return true;
            }
        }
        return false;
    }

    /**
     * 短窗限次闸门 — 防兜底重试死循环。调用方以单调时钟
     * (如 {@code SystemClock.elapsedRealtime()})喂 {@code nowMs}。
     */
    public static final class RecoveryGate {

        private final int maxAttempts;
        private final long windowMs;
        private long windowStartMs = -1L;
        private int usedInWindow;

        public RecoveryGate(int maxAttempts, long windowMs) {
            this.maxAttempts = maxAttempts;
            this.windowMs = windowMs;
        }

        /**
         * true = 允许本次兜底(消耗一次额度; 首次/超窗则开新窗并重置额度)。
         * 窗口内额度用尽 → 拒绝; 恰到窗口边界视为新窗。
         */
        public synchronized boolean tryAcquire(long nowMs) {
            if (windowStartMs < 0 || nowMs - windowStartMs >= windowMs) {
                windowStartMs = nowMs;
                usedInWindow = 1;
                return true;
            }
            if (usedInWindow < maxAttempts) {
                usedInWindow++;
                return true;
            }
            return false;
        }
    }
}
