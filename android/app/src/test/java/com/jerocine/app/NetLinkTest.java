package com.jerocine.app;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import com.jerocine.player.data.NetLink;

import org.junit.Test;

/**
 * NetLink 纯决策函数的方向语义锁定(组件化二期 A5)。
 *
 * 两个消费方(web 壳 loadServer 与 ServerNavigationWebViewClient 降级计数)探测的是同一条链路,
 * 但 fail-safe 兜底方向必须各自可控、不可被"统一"掉:
 *  - fail-open(判定不了按"有网"兜底): WebShellActivity.isOnline —— 宁可放行去加载,
 *    不可把有网用户误拦在离线兜底页;
 *  - fail-closed(判定不了按"无网"兜底): 保守方向, 供需要"宁可不计数也不把链路抖动误计为
 *    WebView 能力不足"的场景(误计会把弱网用户**永久**踢进原生壳)。
 * 改错方向是弱网降级回归的头号风险 —— 本测试即闸门, 两方向语义都必须存在且锁定。
 */
public class NetLinkTest {

    @Test
    public void determinedConnected_isOnlineRegardlessOfDirection() {
        assertTrue(NetLink.isOnline(Boolean.TRUE, NetLink.FAIL_OPEN));
        assertTrue(NetLink.isOnline(Boolean.TRUE, NetLink.FAIL_CLOSED));
    }

    @Test
    public void determinedDisconnected_isOfflineRegardlessOfDirection() {
        assertFalse(NetLink.isOnline(Boolean.FALSE, NetLink.FAIL_OPEN));
        assertFalse(NetLink.isOnline(Boolean.FALSE, NetLink.FAIL_CLOSED));
    }

    @Test
    public void undetermined_failsOpen_whenDirectionIsFailOpen() {
        assertTrue(NetLink.isOnline(null, NetLink.FAIL_OPEN));
    }

    @Test
    public void undetermined_failsClosed_whenDirectionIsFailClosed() {
        assertFalse(NetLink.isOnline(null, NetLink.FAIL_CLOSED));
    }
}
