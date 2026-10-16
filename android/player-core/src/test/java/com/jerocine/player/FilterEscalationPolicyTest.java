package com.jerocine.player;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * 端侧过滤失败升级策略 — 用户实测(2026-10-10): 一集瞬时失败就整源粘性切代理,
 * 弱代理源"一集失败, 后面的集跟着失败"。改为连续两集失败才粘。
 */
public class FilterEscalationPolicyTest {

    @Test
    public void singleFailure_doesNotStickSource() {
        // 第一次失败: 只升级该集(forceProxyIdx), 整片源不粘 —— 下一集还会重试端侧
        assertFalse(FilterEscalationPolicy.shouldStickSource(1));
    }

    @Test
    public void consecutiveFailures_stickSource() {
        assertTrue(FilterEscalationPolicy.shouldStickSource(2));
        assertTrue(FilterEscalationPolicy.shouldStickSource(3));
        assertTrue(FilterEscalationPolicy.shouldStickSource(10));
    }

    @Test
    public void zeroOrNegative_neverSticks() {
        assertFalse(FilterEscalationPolicy.shouldStickSource(0));
        assertFalse(FilterEscalationPolicy.shouldStickSource(-1));
    }
}
