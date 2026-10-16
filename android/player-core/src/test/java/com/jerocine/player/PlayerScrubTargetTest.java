package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

/**
 * 横滑刮擦目标位置计算(PlayerGestureHelper.computeScrubTarget)的纯逻辑测试.
 *
 * 不变量:
 *   ① 长片源按实际时长换算每像素毫秒数(durMs/width), 位移→目标位置;
 *   ② 速率下限 SCRUB_MIN_SPEED_MS_PER_PX(120s/屏): 短时长片源全屏横滑至少跨越约120s,
 *      不会因为源短而刮擦"纹丝不动";
 *   ③ 目标钳制在 [0, durMs-500];
 *   ④ 直播/未知时长(durMs<=0): 固定 250ms/px, 只钳下界不钳上界;
 *   ⑤ width<=0 保护(不除零).
 */
public class PlayerScrubTargetTest {

    /** 60s 起点, 1000px 宽, 1h 长片源: perPxMs=max(3600,120)=3600ms/px. */
    @Test
    public void forwardScrubScalesWithDuration() {
        long target = PlayerGestureHelper.computeScrubTarget(60_000L, 200f, 1000, 3_600_000L);
        assertEquals(60_000L + 200 * 3_600L, target); // +720s → 780s
    }

    @Test
    public void backwardScrubClampsToZero() {
        long target = PlayerGestureHelper.computeScrubTarget(60_000L, -100f, 1000, 3_600_000L);
        assertEquals(0L, target); // 60s - 360s < 0 → 0
    }

    @Test
    public void targetClampsToDurationMinus500ms() {
        long target = PlayerGestureHelper.computeScrubTarget(60_000L, 10_000f, 1000, 3_600_000L);
        assertEquals(3_600_000L - 500L, target);
    }

    /** 60s 短片源, 1000px: perPxMs=max(60,120)=120ms/px(速率下限生效). */
    @Test
    public void shortSourceUsesMinRateFloor() {
        long target = PlayerGestureHelper.computeScrubTarget(60_000L, 300f, 1000, 60_000L);
        // 60s + 300*120ms=36s → 96s, 但源只有 60s → 钳到 59.5s
        assertEquals(60_000L - 500L, target);
    }

    /** 直播(durMs=0): 固定 250ms/px, 只钳下界不上限. */
    @Test
    public void liveStreamUsesFixedRateAndNoUpperClamp() {
        assertEquals(25_000L, PlayerGestureHelper.computeScrubTarget(0L, 100f, 1000, 0L));
        // 负的未知时长(-1)与 0 同语义
        assertEquals(25_000L, PlayerGestureHelper.computeScrubTarget(0L, 100f, 1000, -1L));
    }

    @Test
    public void zeroOrNegativeWidthNeverDividesByZero() {
        // width=0 → 兜底 1px: perPxMs=durMs/1=1_000_000ms/px → 位移 1px 即跨 1000s,
        // 但被上限钳制到 dur-500(不会崩、不会越界)
        long target = PlayerGestureHelper.computeScrubTarget(0L, 1f, 0, 1_000_000L);
        assertEquals(1_000_000L - 500L, target);
    }

    @Test
    public void exactMidpointKeepBaseWhenNoMotion() {
        assertEquals(123_456L, PlayerGestureHelper.computeScrubTarget(123_456L, 0f, 800, 3_600_000L));
    }
}
