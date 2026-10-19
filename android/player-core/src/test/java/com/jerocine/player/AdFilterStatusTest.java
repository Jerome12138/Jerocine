package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

/**
 * 广告过滤角标六态纯逻辑测试(文案/色调须与前端 web 播放器一致).
 */
public class AdFilterStatusTest {

    /** 常用入参: 开关开 / 有代理 / 是 HLS / 非 proxy 清单 / 非在途 / 未失败 / 已跑过 / 0 段. */
    private static AdFilterStatus of(
            boolean on, boolean proxyMissing, boolean hls, boolean viaServerProxy,
            boolean filtering, boolean failed, boolean attempted, int count) {
        return AdFilterStatus.of(on, proxyMissing, hls, viaServerProxy,
                filtering, failed, attempted, count);
    }

    @Test
    public void offShowsIdleBadgeInsteadOfHiding() {
        AdFilterStatus st = of(false, false, true, false, false, false, true, 0);
        assertEquals(AdFilterStatus.KIND_OFF, st.kind);
        assertEquals("过滤未开启", st.text);
        assertEquals(AdFilterStatus.Tone.GRAY, st.tone);
    }

    @Test
    public void nonHlsSourceIsUnsupported() {
        AdFilterStatus st = of(true, false, false, false, false, false, false, 0);
        assertEquals(AdFilterStatus.KIND_UNSUPPORTED, st.kind);
        assertEquals("该源无需过滤", st.text);
        assertEquals(AdFilterStatus.Tone.GRAY, st.tone);
    }

    @Test
    public void serverProxyPlaylistIsBusyBlue() {
        AdFilterStatus st = of(true, false, true, true, false, false, true, 0);
        assertEquals(AdFilterStatus.KIND_PROXY, st.kind);
        assertEquals("服务端过滤中", st.text);
        assertEquals(AdFilterStatus.Tone.BLUE, st.tone);
    }

    /** 端侧过滤在途(无任何结果) → 「广告过滤中」蓝; 起播/切集窗口的第一帧就是真实状态. */
    @Test
    public void clientFilteringInFlightIsBusyBlue() {
        AdFilterStatus st = of(true, false, true, false, true, false, false, 0);
        assertEquals(AdFilterStatus.KIND_FILTERING, st.kind);
        assertEquals("广告过滤中", st.text);
        assertEquals(AdFilterStatus.Tone.BLUE, st.tone);
    }

    /** 有结果(已跑过)就不再显示"过滤中" —— 在途判定只覆盖结果落地之前. */
    @Test
    public void filteringFlagLosesToExistingResult() {
        AdFilterStatus clean = of(true, false, true, false, true, false, true, 0);
        assertEquals(AdFilterStatus.KIND_CLEAN, clean.kind);
        AdFilterStatus counted = of(true, false, true, false, true, false, true, 2);
        assertEquals(AdFilterStatus.KIND_FILTERED, counted.kind);
    }

    @Test
    public void filteredCarriesCountAndGreenTone() {
        AdFilterStatus st = of(true, false, true, false, false, false, true, 3);
        assertEquals(AdFilterStatus.KIND_FILTERED, st.kind);
        assertEquals("已过滤 3 段广告", st.text);
        assertEquals(3, st.count);
        assertEquals(AdFilterStatus.Tone.GREEN, st.tone);
    }

    @Test
    public void cleanWhenClientFilterRanWithoutHits() {
        AdFilterStatus st = of(true, false, true, false, false, false, true, 0);
        assertEquals(AdFilterStatus.KIND_CLEAN, st.kind);
        assertEquals("未检出广告", st.text);
        assertEquals(AdFilterStatus.Tone.GREEN, st.tone);
    }

    @Test
    public void androidOnlyStates() {
        AdFilterStatus missing = of(true, true, true, false, false, false, false, 0);
        assertEquals(AdFilterStatus.KIND_INEFFECTIVE, missing.kind);
        assertEquals("过滤未生效", missing.text);
        assertEquals(AdFilterStatus.Tone.GRAY, missing.tone);

        AdFilterStatus failed = of(true, false, true, false, false, true, true, 0);
        assertEquals(AdFilterStatus.KIND_FAILED, failed.kind);
        assertEquals("过滤失败", failed.text);
        assertEquals(AdFilterStatus.Tone.GRAY, failed.tone);
    }

    /** 优先级: 关闭过滤 > 代理缺失 > 非 HLS > **服务端代理** > 端侧在途 > 过滤失败 > 有结果 > 无检出. */
    @Test
    public void offWinsOverEverythingElse() {
        AdFilterStatus st = of(false, true, false, false, true, true, false, 9);
        assertEquals(AdFilterStatus.KIND_OFF, st.kind);
    }

    @Test
    public void proxyMissingWinsOverFailureAndProxy() {
        AdFilterStatus st = of(true, true, true, true, true, true, true, 5);
        assertEquals(AdFilterStatus.KIND_INEFFECTIVE, st.kind);
    }

    /**
     * 当前清单确实走着服务端代理时, 残留的 filterFailed(上一集/上一轮端侧 POST 失败) 不该
     * 盖掉"服务端过滤中" —— 否则角标会一直误报"过滤失败"。
     */
    @Test
    public void serverProxyWinsOverStaleFailureFlag() {
        AdFilterStatus st = of(true, false, true, true, false, true, true, 0);
        assertEquals(AdFilterStatus.KIND_PROXY, st.kind);
        assertEquals(AdFilterStatus.Tone.BLUE, st.tone);
    }
}
