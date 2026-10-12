package com.jerocine.player;

import static org.junit.Assert.assertEquals;

import androidx.media3.common.Player;

import org.junit.Test;

/** {@link PlayerBufferToastPolicy} 纯逻辑单测 — 缓冲进度 toast 的起显/保持/收起决策(2026-10-11). */
public class PlayerBufferToastPolicyTest {

    @Test
    public void bufferingFromIdle_startsToast() {
        assertEquals(PlayerBufferToastPolicy.Action.START,
                PlayerBufferToastPolicy.onState(Player.STATE_BUFFERING, false, false));
    }

    @Test
    public void bufferingWhileShowing_keepsToast() {
        // STATE_BUFFERING 回调可能连发: 已在显则保持(外层不重置 tick)
        assertEquals(PlayerBufferToastPolicy.Action.KEEP,
                PlayerBufferToastPolicy.onState(Player.STATE_BUFFERING, true, false));
    }

    @Test
    public void readyStopsToast() {
        assertEquals(PlayerBufferToastPolicy.Action.STOP,
                PlayerBufferToastPolicy.onState(Player.STATE_READY, true, false));
    }

    @Test
    public void idleStopsToast() {
        assertEquals(PlayerBufferToastPolicy.Action.STOP,
                PlayerBufferToastPolicy.onState(Player.STATE_IDLE, true, false));
    }

    @Test
    public void nonBufferingWithoutToast_isNoop() {
        // 常态播放/暂停: 既没在显也不是缓冲, 不应触发任何 UI 动作
        assertEquals(PlayerBufferToastPolicy.Action.IGNORE,
                PlayerBufferToastPolicy.onState(Player.STATE_READY, false, false));
        assertEquals(PlayerBufferToastPolicy.Action.IGNORE,
                PlayerBufferToastPolicy.onState(Player.STATE_ENDED, false, false));
    }

    // ---- 广告过滤窗口抑制(2026-10-12): 起播/切集等过滤期间不显示"缓冲中 0%" ----

    @Test
    public void filteringBuffering_doesNotStartToast() {
        // 过滤窗口内即便处于 BUFFERING 也不起显 —— 此时百分比恒 0, 是噪音
        assertEquals(PlayerBufferToastPolicy.Action.IGNORE,
                PlayerBufferToastPolicy.onState(Player.STATE_BUFFERING, false, true));
    }

    @Test
    public void filteringWhileToastShowing_stopsToast() {
        // 若真缓冲的 toast 还在显, 而此刻进入了过滤窗口(切集): 收起让位给"广告过滤中"
        assertEquals(PlayerBufferToastPolicy.Action.STOP,
                PlayerBufferToastPolicy.onState(Player.STATE_BUFFERING, true, true));
    }

    @Test
    public void leavingFilteringBackToRealBuffering_startsToastAgain() {
        // 过滤完成、真正开始缓冲媒体时(episodeSwitching 归位) → 恢复原行为
        assertEquals(PlayerBufferToastPolicy.Action.START,
                PlayerBufferToastPolicy.onState(Player.STATE_BUFFERING, false, false));
    }
}
