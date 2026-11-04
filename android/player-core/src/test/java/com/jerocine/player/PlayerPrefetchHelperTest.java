package com.jerocine.player;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

import java.util.Arrays;

/**
 * Q④ 直播短路预取 — 直播会话(载荷 isLive=true)下 canPrefetch 恒 false:
 * 直播没有"下一集", 且冻结滑窗里预取只会空转烧请求。
 * VOD 基线条件不变(否则等于顺手改了预取行为, 超出本批规格)。
 */
public class PlayerPrefetchHelperTest {

    /** 造一个"除直播标志外全部满足预取条件"的会话(过滤开 + 多源 playlist + 非本地)。 */
    private static PlayerSession vodLikeSession() {
        PlayerSession s = PlayerSessionPrefetchTest.newSession();
        s.adFilterOn = true;
        s.currentBypassFilter = false;
        s.currentRawUrls = Arrays.asList(
                "https://cdn.example.com/ep1.m3u8",
                "https://cdn.example.com/ep2.m3u8");
        return s;
    }

    @Test
    public void liveSessionNeverPrefetches() {
        PlayerSession s = vodLikeSession();
        s.isLive = true;
        PlayerPrefetchHelper h = new PlayerPrefetchHelper(s);
        assertFalse("直播会话: canPrefetch 必须恒 false", h.canPrefetch(0));
        assertFalse(h.canPrefetch(-1));
    }

    @Test
    public void vodWithSameConditionsStillPrefetches() {
        PlayerSession s = vodLikeSession();
        PlayerPrefetchHelper h = new PlayerPrefetchHelper(s);
        assertTrue("非直播: 其余条件满足时预取照旧(基线不变)", h.canPrefetch(0));
    }
}
