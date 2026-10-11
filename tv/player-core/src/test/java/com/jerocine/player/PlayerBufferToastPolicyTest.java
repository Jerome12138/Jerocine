package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import androidx.media3.common.Player;

import org.junit.Test;

/** {@link PlayerBufferToastPolicy} 纯逻辑单测 — 缓冲进度 toast 的起显/保持/收起决策(2026-10-11). */
public class PlayerBufferToastPolicyTest {

    @Test
    public void bufferingFromIdle_startsToast() {
        assertEquals(PlayerBufferToastPolicy.Action.START,
                PlayerBufferToastPolicy.onState(Player.STATE_BUFFERING, false));
    }

    @Test
    public void bufferingWhileShowing_keepsToast() {
        // STATE_BUFFERING 回调可能连发: 已在显则保持(外层不重置 tick)
        assertEquals(PlayerBufferToastPolicy.Action.KEEP,
                PlayerBufferToastPolicy.onState(Player.STATE_BUFFERING, true));
    }

    @Test
    public void readyStopsToast() {
        assertEquals(PlayerBufferToastPolicy.Action.STOP,
                PlayerBufferToastPolicy.onState(Player.STATE_READY, true));
    }

    @Test
    public void idleStopsToast() {
        assertEquals(PlayerBufferToastPolicy.Action.STOP,
                PlayerBufferToastPolicy.onState(Player.STATE_IDLE, true));
    }

    @Test
    public void nonBufferingWithoutToast_isNoop() {
        // 常态播放/暂停: 既没在显也不是缓冲, 不应触发任何 UI 动作
        assertEquals(PlayerBufferToastPolicy.Action.IGNORE,
                PlayerBufferToastPolicy.onState(Player.STATE_READY, false));
        assertEquals(PlayerBufferToastPolicy.Action.IGNORE,
                PlayerBufferToastPolicy.onState(Player.STATE_ENDED, false));
    }
}
