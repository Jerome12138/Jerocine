package com.jerocine.player;

import android.view.MotionEvent;
import android.view.View;

import androidx.media3.common.PlaybackParameters;
import androidx.media3.common.Player;
import androidx.media3.ui.PlayerView;

import java.util.Locale;

/**
 * 触屏手势 — 对齐 Web 端语义: 长按临时 2x / 横滑刮擦进度 / 轻扫 ±10s.
 *
 * 只读写 {@link PlayerSession} 与 Host, 与其它 helper 无互相引用.
 */
public class PlayerGestureHelper {

    private static final int GESTURE_LONG_PRESS_MS = 450;
    private static final int GESTURE_SWIPE_TRIGGER_PX = 12;
    private static final int GESTURE_FLICK_MS = 300;
    private static final int GESTURE_FLICK_PX = 60;
    private static final int GESTURE_FLICK_SEEK_SEC = 10;
    private static final int GESTURE_SEEK_INTERVAL_MS = 400;
    private static final long GESTURE_HINT_KEEP_MS = 60_000L;
    /** 横滑刮擦速率下限: 每像素至少 120ms ⇒ 全屏横滑最多前进约 120s(短时长片源也不会刮不动). */
    private static final float SCRUB_MIN_SPEED_MS_PER_PX = 120_000f;
    /** 直播/未知时长(durMs<=0)时的固定刮擦速率: 250ms/px. */
    private static final float SCRUB_LIVE_SPEED_MS_PER_PX = 250f;

    private final PlayerSession session;

    /** 手势模式: 0=无 1=长按2x 2=横滑刮擦 */
    private int gestureMode = 0;
    private float gestureStartX, gestureStartY;
    private long gestureStartTime;
    private long gestureSeekBaseMs, gestureTargetMs;
    private float gestureLastDx;
    private long gestureLastSeekAt;
    private float gestureTempRateBefore = 1f;
    /** 进度条(横滑刮擦时让 scrubber 跟手, 见 {@link #ensureScrubber}). */
    private androidx.media3.ui.DefaultTimeBar scrubber;

    private final Runnable gestureLongPressRunnable = new Runnable() {
        @Override
        public void run() {
            Player player = session.player;
            if (player == null || !player.getPlayWhenReady()
                    || player.getPlaybackState() == Player.STATE_BUFFERING) return;
            gestureMode = 1;
            gestureTempRateBefore = player.getPlaybackParameters().speed;
            player.setPlaybackParameters(new PlaybackParameters(2f));
            session.host().showCenterToast("2x 倍速中 ▶▶", GESTURE_HINT_KEEP_MS);
        }
    };

    public PlayerGestureHelper(PlayerSession session) {
        this.session = session;
    }

    /**
     * 取进度条(Media3 控制视图懒加载, 每次面板创建后都要重查).
     * 控制视图由 {@code pv.showController()} 同步 inflate, 故 showController 后即可拿到.
     */
    private void ensureScrubber() {
        if (scrubber == null) {
            PlayerView pv = session.host().playerView();
            if (pv != null) {
                scrubber = pv.findViewById(androidx.media3.ui.R.id.exo_progress);
            }
        }
    }

    /**
     * 整体消费 playerView 的触摸; 控制条/按钮等子控件会先吃掉自己的触摸, 到不了这里.
     * 普通单击(未形成任何手势) = 切换控制面板.
     * 横滑刮擦: 进入即显示上下控制 UI(面板), 进度条 scrubber 跟随手指移动, 抬指才落 seek.
     */
    boolean onPlayerTouch(View v, MotionEvent ev) {
        final PlayerView pv = session.host().playerView();
        if (session.player == null || pv == null) return false;
        switch (ev.getActionMasked()) {
            case MotionEvent.ACTION_DOWN: {
                gestureMode = 0;
                gestureStartX = ev.getX();
                gestureStartY = ev.getY();
                gestureStartTime = System.currentTimeMillis();
                gestureLastDx = 0;
                pv.removeCallbacks(gestureLongPressRunnable);
                pv.postDelayed(gestureLongPressRunnable, GESTURE_LONG_PRESS_MS);
                return true;
            }
            case MotionEvent.ACTION_POINTER_DOWN:
                cancelGesture();
                return true;
            case MotionEvent.ACTION_MOVE: {
                if (ev.getPointerCount() != 1) return true;
                float dx = ev.getX() - gestureStartX;
                float dy = ev.getY() - gestureStartY;
                if (gestureMode == 0) {
                    if (Math.abs(dx) > GESTURE_SWIPE_TRIGGER_PX && Math.abs(dx) > Math.abs(dy)) {
                        pv.removeCallbacks(gestureLongPressRunnable);
                        gestureMode = 2;
                        gestureSeekBaseMs = session.player.getCurrentPosition();
                        gestureTargetMs = gestureSeekBaseMs;
                        gestureLastSeekAt = 0L;
                        // 需求: 左右滑动时显示上下的控制 UI, 进度条随动
                        pv.showController();
                        ensureScrubber();
                    }
                    return true;
                }
                if (gestureMode != 2) return true;
                gestureLastDx = dx;
                long durMs = session.player.getDuration();
                gestureTargetMs = computeScrubTarget(gestureSeekBaseMs, dx, pv.getWidth(), durMs);
                if (scrubber != null) scrubber.setPosition(gestureTargetMs);
                long delta = gestureTargetMs - gestureSeekBaseMs;
                session.host().showCenterToast(
                        (delta >= 0 ? "快进 " : "快退 ") + fmtGestureMs(Math.abs(delta))
                                + " · " + fmtGestureMs(gestureTargetMs)
                                + " (" + (durMs > 0 ? Math.round(gestureTargetMs * 100.0 / durMs) : 0) + "%)",
                        GESTURE_HINT_KEEP_MS);
                long now = System.currentTimeMillis();
                if (now - gestureStartTime > GESTURE_FLICK_MS
                        && Math.abs(dx) > GESTURE_FLICK_PX
                        && now - gestureLastSeekAt > GESTURE_SEEK_INTERVAL_MS) {
                    gestureLastSeekAt = now;
                    session.player.seekTo(gestureTargetMs);
                }
                return true;
            }
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL: {
                pv.removeCallbacks(gestureLongPressRunnable);
                int mode = gestureMode;
                gestureMode = 0;
                if (mode == 1) {
                    session.player.setPlaybackParameters(new PlaybackParameters(gestureTempRateBefore));
                    session.host().hideGestureToast();
                    return true;
                }
                if (mode == 2) {
                    long dt = System.currentTimeMillis() - gestureStartTime;
                    if (dt < GESTURE_FLICK_MS && Math.abs(gestureLastDx) < GESTURE_FLICK_PX) {
                        long target = session.player.getCurrentPosition()
                                + (gestureLastDx >= 0 ? 1 : -1) * GESTURE_FLICK_SEEK_SEC * 1000L;
                        long dur = session.player.getDuration();
                        target = Math.max(0, target);
                        if (dur > 0) target = Math.min(target, dur);
                        session.player.seekTo(target);
                        if (scrubber != null) scrubber.setPosition(target);
                        session.host().showCenterToast(
                                (gestureLastDx >= 0 ? "▶ 快进 " : "◀ 快退 ")
                                        + GESTURE_FLICK_SEEK_SEC + "s", 800);
                    } else {
                        session.player.seekTo(gestureTargetMs);
                        if (scrubber != null) scrubber.setPosition(gestureTargetMs);
                        session.host().showCenterToast("已跳转 " + fmtGestureMs(gestureTargetMs), 800);
                    }
                    return true;
                }
                if (System.currentTimeMillis() - gestureStartTime < 250) {
                    if (pv.isControllerFullyVisible()) {
                        pv.hideController();
                    } else {
                        pv.showController();
                    }
                }
                return true;
            }
            default:
                return true;
        }
    }

    /** 手势中途取消(多指/异常): 长按恢复原速, 刮擦不落 seek. */
    void cancelGesture() {
        PlayerView pv = session.host().playerView();
        if (pv != null) {
            pv.removeCallbacks(gestureLongPressRunnable);
        }
        if (gestureMode == 1 && session.player != null) {
            session.player.setPlaybackParameters(new PlaybackParameters(gestureTempRateBefore));
        }
        gestureMode = 0;
        session.host().hideGestureToast();
    }

    /**
     * 横滑刮擦目标位置 — 纯计算, 便于单测.
     *
     * 基准位置 + 手指位移 × 每像素毫秒数, 钳制在 [0, durMs-500].
     * 每像素毫秒数下限 {@link #SCRUB_MIN_SPEED_MS_PER_PX}: 短时长片源也保证全屏横滑可跨越多集进度,
     * 不会因为源太短而刮擦"纹丝不动". 直播/未知时长(durMs<=0)用固定速率, 不上限钳制.
     */
    static long computeScrubTarget(long seekBaseMs, float dx, int widthPx, long durMs) {
        int width = Math.max(1, widthPx);
        float perPxMs = durMs > 0
                ? Math.max(durMs / (float) width, SCRUB_MIN_SPEED_MS_PER_PX / width)
                : SCRUB_LIVE_SPEED_MS_PER_PX;
        long limit = durMs > 0 ? durMs - 500 : Long.MAX_VALUE;
        return Math.min(Math.max(seekBaseMs + (long) (dx * perPxMs), 0L), Math.max(limit, 0L));
    }

    /** ms → m:ss (手势提示用) */
    static String fmtGestureMs(long ms) {
        long s = Math.max(0, ms) / 1000;
        return (s / 60) + ":" + String.format(Locale.US, "%02d", s % 60);
    }
}
