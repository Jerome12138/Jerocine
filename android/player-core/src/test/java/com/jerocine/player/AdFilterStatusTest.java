package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

/**
 * 广告过滤角标六态纯逻辑测试(文案/色调须与前端 web 播放器一致).
 */
public class AdFilterStatusTest {

    /** 常用入参: 开关开 / 是 HLS / 非在途 / 未失败 / 已跑过 / 0 段. */
    private static AdFilterStatus of(
            boolean on, boolean hls,
            boolean filtering, boolean failed, boolean attempted, int count) {
        return AdFilterStatus.of(on, hls, filtering, failed, attempted, count);
    }

    @Test
    public void offShowsIdleBadgeInsteadOfHiding() {
        AdFilterStatus st = of(false, true, false, false, true, 0);
        assertEquals(AdFilterStatus.KIND_OFF, st.kind);
        assertEquals("过滤未开启", st.text);
        assertEquals(AdFilterStatus.Tone.GRAY, st.tone);
    }

    @Test
    public void nonHlsSourceIsUnsupported() {
        AdFilterStatus st = of(true, false, false, false, false, 0);
        assertEquals(AdFilterStatus.KIND_UNSUPPORTED, st.kind);
        assertEquals("该源无需过滤", st.text);
        assertEquals(AdFilterStatus.Tone.GRAY, st.tone);
    }

    /** 端侧过滤在途(无任何结果) → 「广告过滤中」蓝; 起播/切集窗口的第一帧就是真实状态. */
    @Test
    public void clientFilteringInFlightIsBusyBlue() {
        AdFilterStatus st = of(true, true, true, false, false, 0);
        assertEquals(AdFilterStatus.KIND_FILTERING, st.kind);
        assertEquals("广告过滤中", st.text);
        assertEquals(AdFilterStatus.Tone.BLUE, st.tone);
    }

    /** 有结果(已跑过)就不再显示"过滤中" —— 在途判定只覆盖结果落地之前. */
    @Test
    public void filteringFlagLosesToExistingResult() {
        AdFilterStatus clean = of(true, true, true, false, true, 0);
        assertEquals(AdFilterStatus.KIND_CLEAN, clean.kind);
        AdFilterStatus counted = of(true, true, true, false, true, 2);
        assertEquals(AdFilterStatus.KIND_FILTERED, counted.kind);
    }

    /** 失败必须压过"过滤中"(防御调用方误传: 已有结局必须胜出). */
    @Test
    public void failureWinsOverInFlight() {
        AdFilterStatus st = of(true, true, true, true, false, 0);
        assertEquals(AdFilterStatus.KIND_FAILED, st.kind);
        assertEquals("过滤失败", st.text);
        assertEquals(AdFilterStatus.Tone.GRAY, st.tone);
    }

    @Test
    public void filteredCarriesCountAndGreenTone() {
        AdFilterStatus st = of(true, true, false, false, true, 3);
        assertEquals(AdFilterStatus.KIND_FILTERED, st.kind);
        assertEquals("已过滤 3 段广告", st.text);
        assertEquals(3, st.count);
        assertEquals(AdFilterStatus.Tone.GREEN, st.tone);
    }

    @Test
    public void cleanWhenClientFilterRanWithoutHits() {
        AdFilterStatus st = of(true, true, false, false, true, 0);
        assertEquals(AdFilterStatus.KIND_CLEAN, st.kind);
        assertEquals("未检出广告", st.text);
        assertEquals(AdFilterStatus.Tone.GREEN, st.tone);
    }

    /** 优先级: 关闭过滤 > 非 HLS > 端侧在途 > 过滤失败 > 有结果 > 无检出. */
    @Test
    public void offWinsOverEverythingElse() {
        AdFilterStatus st = of(false, false, true, true, false, 9);
        assertEquals(AdFilterStatus.KIND_OFF, st.kind);
    }
}
