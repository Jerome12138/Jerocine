package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

/**
 * 直播按键决策(2026-11-07 用户拍板: ←→ 双击才换源, 单击只提示防误触)—— 纯逻辑 JVM 单测。
 */
public class LiveKeyPolicyTest {

    // ============================ onSourceKey(双击换源判定) ============================

    private static LiveKeyPolicy newPolicy() {
        return LiveKeyPolicy.create();
    }

    @Test
    public void sameDirectionWithinWindow_switches() {
        LiveKeyPolicy policy = newPolicy();
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 1_000L));
        assertEquals(LiveKeyPolicy.ZapVerdict.SWITCH,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 1_000L + 400L));
    }

    @Test
    public void exactWindowBoundary_switches() {
        // 恰好 700ms 间隔按「窗口内」判定(<=, 与 LiveKeyPolicy 自身 javadoc 同口径)。
        // 注意: 这与 RecoveryGate 的恰好边界惯例**相反** —— RecoveryGate.tryAcquire 以
        // >= 开新窗(恰到窗口边界视为新窗/窗外), 本类用 <=(恰好边界仍算窗内); 两者仅
        // synchronized + 单调时钟惯例同款, 边界口径并不对齐, 勿据此误改判定翻转边界行为。
        LiveKeyPolicy policy = newPolicy();
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 1_000L));
        assertEquals(LiveKeyPolicy.ZapVerdict.SWITCH, policy.onSourceKey(
                LiveKeyPolicy.SourceDir.NEXT, 1_000L + LiveKeyPolicy.SOURCE_ZAP_WINDOW_MS));
    }

    @Test
    public void beyondWindow_hintsAndResetsWindowStart() {
        LiveKeyPolicy policy = newPolicy();
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 1_000L));
        long secondAt = 1_000L + LiveKeyPolicy.SOURCE_ZAP_WINDOW_MS + 1;
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, secondAt));
        // 窗口起点已重置到第二次按键: 再隔恰好一个窗口 → SWITCH(若仍锚在 1000, 此刻已超窗两倍, 应为 HINT)
        assertEquals(LiveKeyPolicy.ZapVerdict.SWITCH, policy.onSourceKey(
                LiveKeyPolicy.SourceDir.NEXT, secondAt + LiveKeyPolicy.SOURCE_ZAP_WINDOW_MS));
    }

    @Test
    public void directionChangeResetsChain() {
        // 中途换向: 即便落在窗口内也不构成双击 —— 换向即重置, 同向第二次(首按方向)仍只提示
        LiveKeyPolicy policy = newPolicy();
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.PREV, 0L));
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 300L));
        // 第二次 PREV(与首按同向): 若换向未重置, 600-0=600ms <= 700 会被误判成 SWITCH
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.PREV, 600L));
        // 锚点已落在换向后的 PREV@600: 300ms 后同向再按, 双击成立
        assertEquals(LiveKeyPolicy.ZapVerdict.SWITCH,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.PREV, 900L));
    }

    @Test
    public void afterSwitch_stateCleared_nextPressHints() {
        LiveKeyPolicy policy = newPolicy();
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 1_000L));
        assertEquals(LiveKeyPolicy.ZapVerdict.SWITCH,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 1_100L));
        // SWITCH 已清状态: 紧接着一按只提示, 不当作又一发换源
        assertEquals(LiveKeyPolicy.ZapVerdict.HINT,
                policy.onSourceKey(LiveKeyPolicy.SourceDir.NEXT, 1_200L));
        // 新窗口从 1200 起算: 恰好一个窗口后再按, 又可换源
        assertEquals(LiveKeyPolicy.ZapVerdict.SWITCH, policy.onSourceKey(
                LiveKeyPolicy.SourceDir.NEXT, 1_200L + LiveKeyPolicy.SOURCE_ZAP_WINDOW_MS));
    }

    // ============================ stepChannel(切台步进) ============================

    @Test
    public void stepChannel_clampsAtBothEnds() {
        // 到头停住不回绕: 首台再 ← 停在 0, 末台再 → 停在 size-1
        assertEquals(0, LiveKeyPolicy.stepChannel(0, -1, 5));
        assertEquals(4, LiveKeyPolicy.stepChannel(4, 1, 5));
        // 大步幅同样钳到端点
        assertEquals(0, LiveKeyPolicy.stepChannel(1, -3, 5));
        assertEquals(4, LiveKeyPolicy.stepChannel(2, 5, 5));
        // 窗口内正常步进
        assertEquals(3, LiveKeyPolicy.stepChannel(2, 1, 5));
        assertEquals(1, LiveKeyPolicy.stepChannel(2, -1, 5));
    }

    @Test
    public void stepChannel_nonPositiveSize_returnsZero() {
        assertEquals(0, LiveKeyPolicy.stepChannel(0, 1, 0));
        assertEquals(0, LiveKeyPolicy.stepChannel(3, -1, -2));
    }

    // ============================ channelIndexForNewSource(换源保序号) ============================

    @Test
    public void channelIndexForNewSource_clamps() {
        assertEquals(2, LiveKeyPolicy.channelIndexForNewSource(2, 5));   // 范围内原样保留
        assertEquals(4, LiveKeyPolicy.channelIndexForNewSource(7, 5));   // 越末钳到 newSize-1
        assertEquals(0, LiveKeyPolicy.channelIndexForNewSource(-1, 5));  // 负值钳到 0
    }

    @Test
    public void channelIndexForNewSource_nonPositiveSize_returnsZero() {
        assertEquals(0, LiveKeyPolicy.channelIndexForNewSource(3, 0));
        assertEquals(0, LiveKeyPolicy.channelIndexForNewSource(3, -1));
    }
}
