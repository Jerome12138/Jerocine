package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

/**
 * 高峰档参数(buffer-perf 规格 D⑥)纯逻辑测试。
 *
 * <p>参数表(§3.3):
 * <pre>
 *   非高峰在线(= 现状 = 本地恒定)  30_000 / 120_000 / 1_500 / 2_500 / back 30_000
 *   高峰在线                        60_000 / 150_000 / 3_000 / 5_000 / back 30_000
 * </pre>
 * 非高峰 golden 用例 = 「非高峰与现状完全一致」的机器可查凭据。
 */
public class PlayerLoadControlPolicyTest {

    private static final int STD_MIN = 30_000;
    private static final int STD_MAX = 120_000;
    private static final int STD_PLAYBACK = 1_500;
    private static final int STD_REBUFFER = 2_500;

    private static final int PEAK_MIN = 60_000;
    private static final int PEAK_MAX = 150_000;
    private static final int PEAK_PLAYBACK = 3_000;
    private static final int PEAK_REBUFFER = 5_000;

    /** golden: 非高峰在线 = 改造前 initPlayer 逐字现状(D⑥), 锁「非高峰零变化」保底。 */
    @Test
    public void onlineNonPeak_goldenLockedToCurrentValues() {
        PlayerLoadControlPolicy p = PlayerLoadControlPolicy.resolve(false, false);
        assertEquals(STD_MIN, p.minBufferMs());
        assertEquals(STD_MAX, p.maxBufferMs());
        assertEquals(STD_PLAYBACK, p.bufferForPlaybackMs());
        assertEquals(STD_REBUFFER, p.bufferForPlaybackAfterRebufferMs());
    }

    /** 高峰在线 = 放大一档(D⑥: min 30→60s / max 120→150s / 起播 1.5→3s / 重缓冲 2.5→5s)。 */
    @Test
    public void onlinePeak_scaledUp() {
        PlayerLoadControlPolicy p = PlayerLoadControlPolicy.resolve(false, true);
        assertEquals(PEAK_MIN, p.minBufferMs());
        assertEquals(PEAK_MAX, p.maxBufferMs());
        assertEquals(PEAK_PLAYBACK, p.bufferForPlaybackMs());
        assertEquals(PEAK_REBUFFER, p.bufferForPlaybackAfterRebufferMs());
    }

    /** 本地文件模式恒现状值(peak 真/假都不影响, D⑥)。 */
    @Test
    public void localMode_constantRegardlessOfPeak() {
        for (boolean peak : new boolean[]{false, true}) {
            PlayerLoadControlPolicy p = PlayerLoadControlPolicy.resolve(true, peak);
            assertEquals("local min", STD_MIN, p.minBufferMs());
            assertEquals("local max", STD_MAX, p.maxBufferMs());
            assertEquals("local playback", STD_PLAYBACK, p.bufferForPlaybackMs());
            assertEquals("local rebuffer", STD_REBUFFER, p.bufferForPlaybackAfterRebufferMs());
        }
    }

    /** backBuffer 30s 两档共用不动(D⑥: 与高峰正向卡顿正交)。 */
    @Test
    public void backBuffer_fixedAt30s_inAllTiers() {
        assertEquals(30_000, PlayerLoadControlPolicy.resolve(false, false).backBufferMs());
        assertEquals(30_000, PlayerLoadControlPolicy.resolve(false, true).backBufferMs());
        assertEquals(30_000, PlayerLoadControlPolicy.resolve(true, false).backBufferMs());
        assertEquals(30_000, PlayerLoadControlPolicy.resolve(true, true).backBufferMs());
    }
}
