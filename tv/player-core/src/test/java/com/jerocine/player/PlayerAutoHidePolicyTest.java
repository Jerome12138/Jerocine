package com.jerocine.player;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * 播放器控制面板自动收起策略 — 用户要求(2026-10-09):
 * 暂停和缓冲中面板不主动消失; 播放中维持 3s 自动收起。
 */
public class PlayerAutoHidePolicyTest {

    @Test
    public void paused_holdsVisible() {
        assertTrue(PlayerAutoHidePolicy.shouldHoldVisible(false, false));
        assertEquals(0, PlayerAutoHidePolicy.timeoutMs(false, false));
    }

    @Test
    public void buffering_holdsVisible_evenWhilePlayWhenReady() {
        // 缓冲中 playWhenReady 仍为 true — media3 默认照样收, 策略必须按 buffering 拦住
        assertTrue(PlayerAutoHidePolicy.shouldHoldVisible(true, true));
        assertEquals(0, PlayerAutoHidePolicy.timeoutMs(true, true));
    }

    @Test
    public void bufferingWhilePaused_holdsVisible() {
        assertTrue(PlayerAutoHidePolicy.shouldHoldVisible(true, false));
        assertEquals(0, PlayerAutoHidePolicy.timeoutMs(true, false));
    }

    @Test
    public void playing_autoHidesAfter3s() {
        assertFalse(PlayerAutoHidePolicy.shouldHoldVisible(false, true));
        assertEquals(3000, PlayerAutoHidePolicy.timeoutMs(false, true));
    }
}
