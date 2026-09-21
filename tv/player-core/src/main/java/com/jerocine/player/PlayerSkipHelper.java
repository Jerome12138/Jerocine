package com.jerocine.player;

import android.os.Handler;
import android.os.Looper;

import androidx.media3.common.Player;

/**
 * 跳过片头片尾 + 自动连播助手。
 */
public class PlayerSkipHelper {

    /** 跳片尾轮询间隔 */
    private static final long OUTRO_POLL_MS = 1000L;

    /** 跳片头/片尾不应用于过短的视频 */
    static final long MIN_DURATION_FOR_SKIP_MS = 5 * 60_000L;

    /** 默认值 — 跳过开关打开时若用户没改, 用这个 */
    static final long DEFAULT_SKIP_INTRO_MS = 90_000L;
    static final long DEFAULT_SKIP_OUTRO_MS = 60_000L;

    /** 倒数 5 分钟内不上报进度(web 层不写播放记忆), 避免重开续播到片尾。 */
    static final long NO_RECORD_TAIL_MS = 300_000L;

    private final PlayerActivity activity;
    private final Handler outroHandler = new Handler(Looper.getMainLooper());

    long skipIntroMs = DEFAULT_SKIP_INTRO_MS;
    long skipOutroMs = DEFAULT_SKIP_OUTRO_MS;
    boolean skipEnabled = false;
    boolean autoNext = true;
    boolean introSkippedForCurrent = false;
    boolean outroPromptShown = false;
    boolean episodeSwitching = false;

    private final Runnable outroWatcher = new Runnable() {
        @Override
        public void run() {
            try {
                if (activity.player != null && skipEnabled && skipOutroMs > 0 && autoNext
                        && activity.player.isPlaying() && !episodeSwitching) {
                    long duration = activity.player.getDuration();
                    long pos = activity.player.getCurrentPosition();
                    if (duration > MIN_DURATION_FOR_SKIP_MS && pos > 0 && activity.player.hasNextMediaItem()) {
                        long remaining = duration - pos;
                        if (!outroPromptShown && remaining <= skipOutroMs + 10000 && remaining > skipOutroMs) {
                            outroPromptShown = true;
                            activity.showCenterToast("10 秒后跳过片尾, 播放下一集", 1500);
                        }
                        if (remaining <= skipOutroMs) {
                            activity.showCenterToast("跳过片尾 → 下一集", 1500);
                            episodeSwitching = true;
                            activity.player.seekToNextMediaItem();
                        }
                    }
                }
            } finally {
                outroHandler.postDelayed(this, OUTRO_POLL_MS);
            }
        }
    };

    public PlayerSkipHelper(PlayerActivity activity) {
        this.activity = activity;
    }

    void startOutroWatcher() {
        outroHandler.postDelayed(outroWatcher, OUTRO_POLL_MS);
    }

    void stopOutroWatcher() {
        outroHandler.removeCallbacksAndMessages(null);
    }

    /**
     * STATE_READY 第一次到达时, 跳到 skipIntroMs (短视频不跳). 总开关关时直接退出.
     */
    void applySkipIntro() {
        if (activity.player == null || !skipEnabled || skipIntroMs <= 0) return;
        long duration = activity.player.getDuration();
        if (duration <= 0 || duration < MIN_DURATION_FOR_SKIP_MS) return;
        long currentPos = activity.player.getCurrentPosition();
        if (currentPos >= skipIntroMs) return;
        activity.player.seekTo(skipIntroMs);
        activity.showCenterToast("已跳过片头 " + (skipIntroMs / 1000) + "s", 1200);
    }

    /**
     * 当前是否处于"倒数 5 分钟不记进度"区间。
     */
    boolean inNoRecordTail() {
        try {
            if (activity.player == null) return false;
            long duration = activity.player.getDuration();
            if (duration <= NO_RECORD_TAIL_MS) return false;
            return duration - activity.player.getCurrentPosition() <= NO_RECORD_TAIL_MS;
        } catch (Exception e) {
            return false;
        }
    }

    /**
     * 跳片头/片尾设置 — 总开关 + ±10/±60 stepper, 改动即时生效并回写账号。
     */
    void showSkipSettingsDialog() {
        activity.dialogHelper.showSkipSettingsDialog();
    }
}
