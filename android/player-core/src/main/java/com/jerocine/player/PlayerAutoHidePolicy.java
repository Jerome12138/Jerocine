package com.jerocine.player;

/**
 * 播放器控制面板自动收起策略 — 纯逻辑, JVM 可测。
 *
 * <p>背景(用户要求 2026-10-09): 暂停和缓冲中面板不主动消失。media3 PlayerView 的
 * 自动收起是"面板展示时排一个延时任务"实现 — 暂停时唤出也会在超时后被收起
 * (show() 排的任务没人取消), 缓冲中(playWhenReady 仍为 true)同样收起。
 * 因此按播放状态切超时, 由 PlayerActivity.updateControllerAutoHide 落到
 * {@code PlayerView.setControllerShowTimeoutMs} 上。
 */
public final class PlayerAutoHidePolicy {

    private PlayerAutoHidePolicy() {
    }

    /** 播放中面板自动收起间隔(media3 默认 3s, 显式声明便于调整)。 */
    public static final long AUTO_HIDE_MS = 3000L;

    /** 暂停或缓冲中 → 面板由用户全权控制, 不自动收(超时 0 = 只能手动 hide)。 */
    public static boolean shouldHoldVisible(boolean buffering, boolean playWhenReady) {
        return buffering || !playWhenReady;
    }

    /** 应传给 setControllerShowTimeoutMs 的值; 0 = 永不自动收。 */
    public static int timeoutMs(boolean buffering, boolean playWhenReady) {
        return shouldHoldVisible(buffering, playWhenReady) ? 0 : (int) AUTO_HIDE_MS;
    }
}
