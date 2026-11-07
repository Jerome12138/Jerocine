package com.jerocine.player;

/**
 * 直播播放器按键决策(纯逻辑, 2026-11-07 用户拍板)。
 *
 * <p>2026-11-07 用户拍板: 直播 ←→ 双击才换源, 单击只提示防误触 ——
 * 单击 ← / → 只出提示不动源, {@link #SOURCE_ZAP_WINDOW_MS} 窗口内同向再按一次才真正执行换源;
 * 中途换向或超窗都视作新的单击(窗口起点随之重置到本次)。
 * 切台步进与换源落点也收在本类: 直播切台不回绕, 换源保留频道序号。
 *
 * <p>有状态部分(双击判定)与 {@link PlayerLiveErrorPolicy.RecoveryGate} 同款惯例:
 * synchronized 方法 + 调用方以单调时钟(如 {@code SystemClock.elapsedRealtime()})喂 {@code nowMs}。
 * 双击判定按直播会话持有实例(经 {@link #create()} 获取); 静态方法部分无状态, 可直接调。
 */
final class LiveKeyPolicy {

    /** 双击换源的判定窗口: 两次同向按键间隔不超过此值才算双击(恰好边界算窗口内)。 */
    public static final long SOURCE_ZAP_WINDOW_MS = 700L;

    /** ← / → 方向。 */
    enum SourceDir { PREV, NEXT }

    /**
     * 单次 ← / → 按键的判定结果。
     * HINT = 单击只提示不动作(防误触); SWITCH = 窗口内同向再按一次, 执行换源。
     */
    enum ZapVerdict { HINT, SWITCH }

    /** 上一次单击的方向; null = 无记录(尚未按过, 或 SWITCH 后已清空)。 */
    private SourceDir lastDir;

    /** 上一次单击的时刻; -1 = 无记录(哨兵, 与 RecoveryGate 的 windowStartMs 同款)。 */
    private long lastAtMs = -1L;

    private LiveKeyPolicy() {
    }

    /** 有状态实例的唯一入口(构造私有, 防散落 new; 调用方每个直播会话持一个)。 */
    static LiveKeyPolicy create() {
        return new LiveKeyPolicy();
    }

    /**
     * ← / → 换源判定: 同向且 {@code nowMs - lastAtMs <= SOURCE_ZAP_WINDOW_MS} → 清状态、放行换源;
     * 否则(首次 / 换向 / 超窗)只提示, 并记录本次(dir, nowMs)为新的窗口起点。
     */
    synchronized ZapVerdict onSourceKey(SourceDir dir, long nowMs) {
        if (lastDir != null && dir == lastDir && nowMs - lastAtMs <= SOURCE_ZAP_WINDOW_MS) {
            // 双击成立即清状态: 紧跟的第三按不当作又一发换源, 重新从单击提示起步
            lastDir = null;
            lastAtMs = -1L;
            return ZapVerdict.SWITCH;
        }
        lastDir = dir;
        lastAtMs = nowMs;
        return ZapVerdict.HINT;
    }

    /**
     * 直播切台步进: 返回 {@code index + delta}, 钳制到 {@code [0, size-1]}。
     *
     * <p>2026-11-07 用户拍板: 直播切台不回绕, 到头停住 ——
     * 首台再 ← 停在 0、末台再 → 停在 size-1, 不做环形回绕。
     */
    static int stepChannel(int index, int delta, int size) {
        if (size <= 0) return 0;
        int next = index + delta;
        if (next < 0) return 0;
        if (next > size - 1) return size - 1;
        return next;
    }

    /**
     * 换源后的频道序号: 保留当前频道序号, 钳制进新源频道表的 {@code [0, newSize-1]} 返回。
     *
     * <p>2026-11-07 用户拍板: 直播换源不改选台位置 —— 新源频道数可能更少, 越界钳到最后一项;
     * 与 VOD {@link PlaybackTarget#switchSource} 同思路(保留序号, 越界钳制)。
     */
    static int channelIndexForNewSource(int currentIndex, int newSize) {
        if (newSize <= 0) return 0;
        if (currentIndex < 0) return 0;
        if (currentIndex > newSize - 1) return newSize - 1;
        return currentIndex;
    }
}
