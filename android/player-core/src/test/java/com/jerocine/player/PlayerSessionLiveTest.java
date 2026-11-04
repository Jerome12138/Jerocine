package com.jerocine.player;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import androidx.media3.common.Player;

import org.junit.Test;

import java.lang.reflect.Proxy;

/**
 * Q③ isLive 贯通 — 会话级直播判定(纯 JVM 单测, Host 占位同 PlayerSessionPrefetchTest)。
 *
 * 三条不变量:
 *   ① 旧载荷缺省: 无 isLive extra 时 {@code session.isLive} 必须是 false(向后兼容, 不能误伤 VOD);
 *   ② 载荷标志: TV/web 双入口的直播载荷带 isLive=true → isLiveNow() 恒真;
 *   ③ media3 兜底: 载荷没带标志时, 当前媒体项是直播(时间线)也算直播。
 */
public class PlayerSessionLiveTest {

    /** Player 接口的动态代理 fake: 只回答 isCurrentMediaItemLive, 不实例化 ExoPlayer。 */
    private static Player fakePlayer(final boolean mediaItemLive) {
        return (Player) Proxy.newProxyInstance(
                Player.class.getClassLoader(),
                new Class<?>[]{Player.class},
                (p, method, args) -> {
                    switch (method.getName()) {
                        case "isCurrentMediaItemLive": return mediaItemLive;
                        case "equals": return p == args[0];
                        case "hashCode": return System.identityHashCode(p);
                        case "toString": return "FakePlayer(live=" + mediaItemLive + ")";
                        default: return null;
                    }
                });
    }

    @Test
    public void legacyPayloadWithoutFlagDefaultsToNotLive() {
        PlayerSession s = PlayerSessionPrefetchTest.newSession();
        assertFalse("旧载荷无 isLive extra → 必须按非直播处理(向后兼容)", s.isLive);
        assertFalse(s.isLiveNow());
    }

    @Test
    public void payloadFlagMakesSessionLive() {
        PlayerSession s = PlayerSessionPrefetchTest.newSession();
        s.isLive = true;
        assertTrue(s.isLiveNow());
    }

    @Test
    public void mediaItemLiveIsEnoughEvenWithoutPayloadFlag() {
        // 时间线分支抽成 static 纯函数(session.player 是 ExoPlayer 具体类型, JVM 单测塞不了 fake)
        assertTrue(PlayerSession.isLiveState(false, fakePlayer(true)));
        assertFalse(PlayerSession.isLiveState(false, fakePlayer(false)));
        assertFalse(PlayerSession.isLiveState(false, null));
    }

    @Test
    public void payloadFlagWinsOverPlayerTimeline() {
        assertTrue(PlayerSession.isLiveState(true, fakePlayer(false)));
    }
}
