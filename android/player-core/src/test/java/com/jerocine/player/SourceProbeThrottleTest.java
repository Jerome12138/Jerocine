package com.jerocine.player;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * 进程级探测节流/熔断(buffer-perf 规格 D⑦/§3.4)纯逻辑测试。
 *
 * <p>规则: 距上次探测 &lt;30 分钟跳过; 连续 2 轮探测全败 → 进程内禁探至进程重启
 * (频率/UA 敏感源兜底, 豆瓣 418 反爬先例); 若出现成功则全败计数清零。
 * 时间为纯入参(单调时钟语义由调用方保证), 测试用任意毫秒刻度。
 */
public class SourceProbeThrottleTest {

    private static final long MIN = 60_000L;
    private static final long T30 = 30 * MIN;

    @Test
    public void coldStart_allowed() {
        assertTrue(new SourceProbeThrottle().shouldProbe(1000L));
    }

    @Test
    public void withinInterval_skipped() {
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, true);
        assertFalse("距上次探测 <30 分钟必须跳过", t.shouldProbe(T30 - 1));
    }

    @Test
    public void atIntervalBoundary_allowed() {
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, true);
        assertTrue(t.shouldProbe(T30));
    }

    @Test
    public void oneAllFailRound_stillAllowedAfterInterval() {
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, false);
        assertTrue("连续 1 轮全败不熔断", t.shouldProbe(T30));
    }

    @Test
    public void twoConsecutiveAllFailRounds_blockedAtAnyTime() {
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, false);
        t.recordRound(T30, false);
        assertFalse("熔断后任何时间都禁探", t.shouldProbe(2 * T30));
        assertFalse(t.shouldProbe(100 * T30));
        assertFalse(t.shouldProbe(0L)); // 连时钟回拨(极端入参)也不解禁
    }

    @Test
    public void successResetsConsecutiveFailCount() {
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, false);
        t.recordRound(T30, true);            // 成功 → 全败计数清零
        t.recordRound(2 * T30, false);       // 再败 = 连续仅 1 轮
        assertTrue(t.shouldProbe(3 * T30));
    }

    @Test
    public void allFailRounds_mustBeConsecutive() {
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, false);
        t.recordRound(T30, true);            // 中间成功打断「连续」
        t.recordRound(2 * T30, false);
        assertTrue(t.shouldProbe(3 * T30));
    }

    @Test
    public void successAfterBreaker_clearsCircuit() {
        // 熔断期间不再有新探测(shouldProbe 恒 false), 此用例防御: 万一出现成功
        // (如未来新增手动触发路径), 熔断按同一条 record 规则解除, 不留死锁态。
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, false);
        t.recordRound(T30, false);
        assertFalse(t.shouldProbe(2 * T30));
        t.recordRound(2 * T30, true);
        assertTrue(t.shouldProbe(3 * T30));
    }

    @Test
    public void failedRound_alsoThrottledByInterval() {
        // 全败轮与成功轮同享 30 分钟节流(探测本身即流量, 与成败无关)
        SourceProbeThrottle t = new SourceProbeThrottle();
        t.recordRound(0L, false);
        assertFalse(t.shouldProbe(T30 - 1));
    }
}
