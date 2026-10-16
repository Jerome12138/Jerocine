package com.jerocine.player;

import android.os.Handler;
import android.os.Looper;

/**
 * 跳过片头片尾 + 自动连播.
 *
 * 只读写 {@link PlayerSession} 的状态, 与其它 helper 无互相引用.
 */
public class PlayerSkipHelper {

    /** 跳片尾轮询间隔 */
    private static final long OUTRO_POLL_MS = 1000L;

    /** 跳片头/片尾不应用于过短的视频 */
    static final long MIN_DURATION_FOR_SKIP_MS = 5 * 60_000L;

    /** 默认值 — 跳过开关打开时若用户没改, 用这个 */
    static final long DEFAULT_SKIP_INTRO_MS = 90_000L;
    static final long DEFAULT_SKIP_OUTRO_MS = 60_000L;

    /** 倒数 5 分钟内不上报进度(前端不写播放记忆), 避免重开续播到片尾. */
    static final long NO_RECORD_TAIL_MS = 300_000L;

    private final PlayerSession session;
    private final Handler outroHandler = new Handler(Looper.getMainLooper());

    public PlayerSkipHelper(PlayerSession session) {
        this.session = session;
    }

    private final Runnable outroWatcher = new Runnable() {
        @Override
        public void run() {
            try {
                if (session.player != null && session.skipEnabled && session.skipOutroMs > 0
                        && session.autoNext && session.player.isPlaying() && !session.episodeSwitching) {
                    long duration = session.player.getDuration();
                    long pos = session.player.getCurrentPosition();
                    if (duration > MIN_DURATION_FOR_SKIP_MS && pos > 0
                            && session.player.hasNextMediaItem()) {
                        long remaining = duration - pos;
                        if (!session.outroPromptShown
                                && remaining <= session.skipOutroMs + 10000
                                && remaining > session.skipOutroMs) {
                            session.outroPromptShown = true;
                            session.host().showCenterToast("10 秒后跳过片尾, 播放下一集", 1500);
                        }
                        if (remaining <= session.skipOutroMs) {
                            session.host().showCenterToast("跳过片尾 → 下一集", 1500);
                            session.markEpisodeSwitching();
                            session.player.seekToNextMediaItem();
                        }
                    }
                }
            } finally {
                outroHandler.postDelayed(this, OUTRO_POLL_MS);
            }
        }
    };

    void startOutroWatcher() {
        outroHandler.postDelayed(outroWatcher, OUTRO_POLL_MS);
    }

    void stopOutroWatcher() {
        outroHandler.removeCallbacksAndMessages(null);
    }

    /** STATE_READY 第一次到达时, 跳到 skipIntroMs (短视频不跳). 总开关关时直接退出. */
    void applySkipIntro() {
        if (session.player == null || !session.skipEnabled || session.skipIntroMs <= 0) return;
        long duration = session.player.getDuration();
        if (duration <= 0 || duration < MIN_DURATION_FOR_SKIP_MS) return;
        long currentPos = session.player.getCurrentPosition();
        // 续播位置已超过片头就不再跳
        if (currentPos >= session.skipIntroMs) return;
        session.player.seekTo(session.skipIntroMs);
        session.host().showCenterToast("已跳过片头 " + (session.skipIntroMs / 1000) + "s", 1200);
    }

    /** 当前是否处于"倒数 5 分钟不记进度"区间. */
    boolean inNoRecordTail() {
        try {
            if (session.player == null) return false;
            long duration = session.player.getDuration();
            if (duration <= NO_RECORD_TAIL_MS) return false;
            return duration - session.player.getCurrentPosition() <= NO_RECORD_TAIL_MS;
        } catch (Exception e) {
            return false;
        }
    }
}
