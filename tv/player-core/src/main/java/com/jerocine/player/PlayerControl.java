package com.jerocine.player;

/**
 * 播放运行控制句柄 — 唯一持有"当前播放实例"与"上次播放结局"的地方.
 *
 * 抽这个类之前, 这两样是散在 {@link PlayerActivity} 上的 static volatile 字段
 * (sCurrentInstance / sLastPlaybackFailed): 谁写、何时清、壳层能不能读全靠读代码,
 * 壳层还得直接引用 Activity 的静态成员. 现在收敛成一个显式句柄:
 *   - 生命周期唯一: attach / detach 只由 PlayerActivity 的 onCreate / onDestroy 调用;
 *   - 出口唯一: 壳层只经 {@link JerocinePlayer#control()} 访问, 不碰 Activity 静态成员;
 *   - 失败标志"读取即消费": 壳层用 {@link #consumePlaybackFailure()} 一次取回并清掉,
 *     不必自己置回 false(原先漏置会让下次回主界面重复通知前端).
 *
 * 注意: 进程级 SimpleCache 单例不放这里 —— 那是"进程内资源", 与"当前实例"无关,
 * 仍留在 {@link PlayerActivity} 作静态字段(见该处注释).
 */
public final class PlayerControl {

    private static final PlayerControl INSTANCE = new PlayerControl();

    private PlayerControl() {}

    static PlayerControl get() {
        return INSTANCE;
    }

    /** 当前存活的播放实例 — 强引用, 由 onDestroy 显式 detach. */
    private volatile PlayerActivity current;

    /** 上次播放是否以失败告终(报错且未能自愈). */
    private volatile boolean lastPlaybackFailed;

    // ============================ 生命周期(仅 PlayerActivity 调用) ============================

    void attach(PlayerActivity activity) {
        current = activity;
    }

    void detach(PlayerActivity activity) {
        if (current == activity) current = null;
    }

    /** 记一次播放失败结局, 等壳层回主界面时消费. */
    void markPlaybackFailed() {
        lastPlaybackFailed = true;
    }

    /** 自愈成功(切线路重试) → 撤销刚记下的失败结局. */
    void clearPlaybackFailure() {
        lastPlaybackFailed = false;
    }

    // ============================ 壳层出口 ============================

    /** 当前是否有正在播放的实例. */
    public boolean hasActive() {
        return current != null;
    }

    /** 关掉当前播放器; 无实例时静默返回. */
    public void stop() {
        PlayerActivity activity = current;
        if (activity != null) activity.runOnUiThread(activity::finish);
    }

    /** 改当前播放器倍速; 无实例时静默返回. */
    public void setSpeed(float speed) {
        PlayerActivity activity = current;
        if (activity != null) activity.runOnUiThread(() -> activity.applyPlaybackSpeed(speed));
    }

    /** 上次播放是否失败 — 读取即消费(取回结果的同时清掉标志). */
    public boolean consumePlaybackFailure() {
        boolean failed = lastPlaybackFailed;
        lastPlaybackFailed = false;
        return failed;
    }
}
