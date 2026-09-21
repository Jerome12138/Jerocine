package com.jerocine.player;

import android.view.KeyEvent;
import android.view.View;

import androidx.media3.common.Player;

/**
 * 按键事件处理 — 遥控/键盘事件分发。
 */
public class PlayerKeyEventHelper {

    private static final long EPISODE_CONFIRM_MS = 2000L;
    private static final long BACK_CONFIRM_MS = 2000L;

    private final PlayerActivity activity;
    private long lastEpisodeKeyAt = 0L;
    private int lastEpisodeKeyCode = 0;
    private long lastBackAt = 0L;

    public PlayerKeyEventHelper(PlayerActivity activity) {
        this.activity = activity;
    }

    /** 进度键长按渐进步进 — 按 getDownTime() 起算的实际按住时长 (ms) 计算步幅 */
    private long seekStepFor(KeyEvent ev, boolean forward) {
        long heldMs = ev.getEventTime() - ev.getDownTime();
        long fwd;
        if (heldMs < 500)        fwd = 10_000L;
        else if (heldMs < 2000)  fwd = 30_000L;
        else if (heldMs < 5000)  fwd = 60_000L;
        else                     fwd = 180_000L;
        return forward ? fwd : fwd / 2;
    }

    public boolean dispatchKeyEvent(KeyEvent event) {
        if (event.getAction() != KeyEvent.ACTION_DOWN) {
            return activity.dispatchKeyEvent(event);
        }
        int code = event.getKeyCode();
        boolean controllerVisible = activity.playerView != null && activity.playerView.isControllerFullyVisible();
        boolean onSeekbar = isFocusOnSeekbar();

        switch (code) {
            case KeyEvent.KEYCODE_MENU:
            case KeyEvent.KEYCODE_INFO: {
                if (controllerVisible) {
                    activity.playerView.hideController();
                } else {
                    activity.dialogHelper.showPlayMenu();
                }
                return true;
            }
            case KeyEvent.KEYCODE_BACK: {
                if (controllerVisible) {
                    activity.playerView.hideController();
                    lastBackAt = 0L;
                    return true;
                }
                long now = System.currentTimeMillis();
                if (now - lastBackAt < BACK_CONFIRM_MS) {
                    lastBackAt = 0L;
                    activity.finish();
                } else {
                    lastBackAt = now;
                    activity.showCenterToast("再按一次返回退出播放", 1800);
                }
                return true;
            }
            case KeyEvent.KEYCODE_MEDIA_NEXT:
            case KeyEvent.KEYCODE_CHANNEL_UP:
                if (activity.player != null && activity.player.hasNextMediaItem()) {
                    activity.skipHelper.episodeSwitching = true;
                    activity.player.seekToNextMediaItem();
                    activity.showCenterToast("下一集", 600);
                }
                return true;
            case KeyEvent.KEYCODE_MEDIA_PREVIOUS:
            case KeyEvent.KEYCODE_CHANNEL_DOWN:
                if (activity.player != null && activity.player.hasPreviousMediaItem()) {
                    activity.skipHelper.episodeSwitching = true;
                    activity.player.seekToPreviousMediaItem();
                    activity.showCenterToast("上一集", 600);
                }
                return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:
            case KeyEvent.KEYCODE_SPACE:
                togglePlayPause();
                return true;
        }

        if (!controllerVisible) {
            switch (code) {
                case KeyEvent.KEYCODE_DPAD_LEFT:
                case KeyEvent.KEYCODE_MEDIA_REWIND:
                    seekRelative(-seekStepFor(event, false));
                    return true;
                case KeyEvent.KEYCODE_DPAD_RIGHT:
                case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD:
                    seekRelative(seekStepFor(event, true));
                    return true;
                case KeyEvent.KEYCODE_DPAD_UP:
                    return handleEpisodeKey(true, code);
                case KeyEvent.KEYCODE_DPAD_DOWN:
                    return handleEpisodeKey(false, code);
                case KeyEvent.KEYCODE_DPAD_CENTER:
                case KeyEvent.KEYCODE_ENTER:
                    togglePlayPause();
                    return true;
                default:
                    return activity.dispatchKeyEvent(event);
            }
        }

        switch (code) {
            case KeyEvent.KEYCODE_DPAD_LEFT:
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                if (onSeekbar) {
                    seekRelative(code == KeyEvent.KEYCODE_DPAD_LEFT
                            ? -seekStepFor(event, false)
                            : seekStepFor(event, true));
                    return true;
                }
                return activity.dispatchKeyEvent(event);
            case KeyEvent.KEYCODE_DPAD_UP:
            case KeyEvent.KEYCODE_DPAD_DOWN:
                return activity.dispatchKeyEvent(event);
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER:
                if (onSeekbar) {
                    togglePlayPause();
                    return true;
                }
                return activity.dispatchKeyEvent(event);
            default:
                return activity.dispatchKeyEvent(event);
        }
    }

    private boolean isFocusOnSeekbar() {
        if (activity.playerView == null) return false;
        View focus = activity.playerView.findFocus();
        if (focus == null) return false;
        return focus.getId() == androidx.media3.ui.R.id.exo_progress;
    }

    /** 切集 + 防误触: 1 下提示, 2s 内再按 1 下才生效. prev=true 上一集, false 下一集. */
    private boolean handleEpisodeKey(boolean prev, int code) {
        if (activity.player == null) {
            return true;
        }
        boolean has = prev ? activity.player.hasPreviousMediaItem() : activity.player.hasNextMediaItem();
        if (!has) {
            activity.showCenterToast(prev ? "已是第一集" : "已是最后一集", 800);
            return true;
        }
        long now = System.currentTimeMillis();
        if (lastEpisodeKeyCode == code && now - lastEpisodeKeyAt < EPISODE_CONFIRM_MS) {
            lastEpisodeKeyAt = 0; lastEpisodeKeyCode = 0;
            activity.skipHelper.episodeSwitching = true;
            if (prev) activity.player.seekToPreviousMediaItem();
            else activity.player.seekToNextMediaItem();
            activity.showCenterToast(prev ? "上一集" : "下一集", 600);
        } else {
            lastEpisodeKeyAt = now; lastEpisodeKeyCode = code;
            activity.showCenterToast(prev ? "再按 ▲ 切上一集" : "再按 ▼ 切下一集", 1500);
        }
        return true;
    }

    private void seekRelative(long deltaMs) {
        if (activity.player == null) return;
        long target = Math.max(0, activity.player.getCurrentPosition() + deltaMs);
        long duration = activity.player.getDuration();
        if (duration > 0) target = Math.min(target, duration);
        activity.player.seekTo(target);
        String dir = deltaMs > 0 ? "▶ +" : "◀ ";
        activity.showCenterToast(dir + (deltaMs / 1000) + "s", 800);
    }

    private void togglePlayPause() {
        if (activity.player == null) return;
        if (activity.player.getPlayWhenReady()) {
            activity.player.pause();
        } else {
            activity.player.play();
        }
    }
}
