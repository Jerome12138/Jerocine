package com.jerocine.player;

import org.junit.Test;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

/**
 * StallFailoverPolicy spec(P3 卡顿看门狗) — 与 web/src/utils/stallFailover.spec.ts 守卫矩阵同口径。
 */
public class StallFailoverPolicyTest {

    /** 合法基线: 缓冲超阈值、播放中、有候选源、没触发过 */
    private boolean fire(long bufferingMs, long sinceFailover, int count,
                         boolean playWhenReady, int otherSources, boolean switching) {
        return StallFailoverPolicy.shouldFailover(
                bufferingMs, sinceFailover, count, playWhenReady, otherSources, switching);
    }

    @Test
    public void fires_atThreshold_andNotBefore() {
        assertTrue(fire(StallFailoverPolicy.STALL_THRESHOLD_MS, Long.MAX_VALUE, 0, true, 2, false));
        assertFalse(fire(StallFailoverPolicy.STALL_THRESHOLD_MS - 1, Long.MAX_VALUE, 0, true, 2, false));
    }

    @Test
    public void paused_neverFires() {
        assertFalse(fire(StallFailoverPolicy.STALL_THRESHOLD_MS * 10, Long.MAX_VALUE, 0, false, 2, false));
    }

    @Test
    public void singleSource_neverFires() {
        assertFalse(fire(StallFailoverPolicy.STALL_THRESHOLD_MS * 10, Long.MAX_VALUE, 0, true, 0, false));
    }

    @Test
    public void switchWindow_orBufferInProgress_neverFires() {
        assertFalse(fire(StallFailoverPolicy.STALL_THRESHOLD_MS * 10, Long.MAX_VALUE, 0, true, 2, true));
    }

    @Test
    public void cappedPerEpisode() {
        assertTrue(fire(StallFailoverPolicy.STALL_THRESHOLD_MS, Long.MAX_VALUE,
                StallFailoverPolicy.MAX_PER_EPISODE - 1, true, 2, false));
        assertFalse(fire(StallFailoverPolicy.STALL_THRESHOLD_MS, Long.MAX_VALUE,
                StallFailoverPolicy.MAX_PER_EPISODE, true, 2, false));
    }

    @Test
    public void cooldownWindow() {
        assertFalse(fire(StallFailoverPolicy.STALL_THRESHOLD_MS,
                StallFailoverPolicy.COOLDOWN_MS - 1, 0, true, 2, false));
        assertTrue(fire(StallFailoverPolicy.STALL_THRESHOLD_MS,
                StallFailoverPolicy.COOLDOWN_MS, 0, true, 2, false));
    }
}
