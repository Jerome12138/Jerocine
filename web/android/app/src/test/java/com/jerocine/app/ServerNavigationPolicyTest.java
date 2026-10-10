package com.jerocine.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * ServerNavigationPolicy 的 JVM 单测(本机可跑, 不需要设备/模拟器):
 *   cd web/android && ./gradlew :app:testDebugUnitTest
 *
 * 覆盖的真机回归: 顶栏「刷新」按钮(location.reload) 曾被 Capacitor 当外链丢给系统浏览器,
 * 修法就是"服务器 host 的顶层导航留在 WebView" —— 这条判定的边界都在这里钉住。
 */
public class ServerNavigationPolicyTest {

    // ============ isInAppHost ============

    @Test
    public void sameHost_isInApp() {
        assertTrue(ServerNavigationPolicy.isInAppHost("jerocine.art", "jerocine.art"));
    }

    @Test
    public void hostMatch_isCaseInsensitive_andTrimmed() {
        assertTrue(ServerNavigationPolicy.isInAppHost("Jerocine.ART", " jerocine.art "));
    }

    @Test
    public void otherHost_isNotInApp() {
        // 外链必须继续走系统浏览器 —— 别把整站判成站内
        assertFalse(ServerNavigationPolicy.isInAppHost("www.baidu.com", "jerocine.art"));
        assertFalse(ServerNavigationPolicy.isInAppHost("jerocine.art.evil.com", "jerocine.art"));
    }

    @Test
    public void suffixOrPrefixIsNotAMatch() {
        assertFalse(ServerNavigationPolicy.isInAppHost("art", "jerocine.art"));
        assertFalse(ServerNavigationPolicy.isInAppHost("jerocine.art.cn", "jerocine.art"));
    }

    @Test
    public void nullOrBlank_isNotInApp() {
        assertFalse(ServerNavigationPolicy.isInAppHost(null, "jerocine.art"));
        assertFalse(ServerNavigationPolicy.isInAppHost("jerocine.art", null));
        assertFalse(ServerNavigationPolicy.isInAppHost("", "jerocine.art"));
        assertFalse(ServerNavigationPolicy.isInAppHost("jerocine.art", "   "));
    }

    // ============ hostOf ============

    @Test
    public void hostOf_standardUrls() {
        assertEquals("jerocine.art", ServerNavigationPolicy.hostOf("https://jerocine.art"));
        assertEquals("jerocine.art", ServerNavigationPolicy.hostOf("https://jerocine.art/"));
        assertEquals("jerocine.art", ServerNavigationPolicy.hostOf("http://jerocine.art/index.html"));
        assertEquals("jerocine.art", ServerNavigationPolicy.hostOf("https://jerocine.art?a=1#b"));
    }

    @Test
    public void hostOf_withPort_stripsPort() {
        assertEquals("1.2.3.4", ServerNavigationPolicy.hostOf("http://1.2.3.4:8080/x"));
        assertEquals("jerocine.art", ServerNavigationPolicy.hostOf("https://jerocine.art:8443"));
    }

    @Test
    public void hostOf_withoutScheme_stillParses() {
        // 用户手填的朴素写法(没有 scheme)不能让 URI 解析抛错后整条判定失效
        assertEquals("jerocine.art", ServerNavigationPolicy.hostOf("jerocine.art"));
        assertEquals("1.2.3.4", ServerNavigationPolicy.hostOf("1.2.3.4:8080"));
        assertEquals("jerocine.art", ServerNavigationPolicy.hostOf("jerocine.art/path"));
    }

    @Test
    public void hostOf_blankOrNull() {
        assertNull(ServerNavigationPolicy.hostOf(null));
        assertNull(ServerNavigationPolicy.hostOf(""));
        assertNull(ServerNavigationPolicy.hostOf("   "));
    }

    @Test
    public void hostOf_thenIsInApp_roundTrip() {
        // 真实链路: prefs 里的 server_url → hostOf → 与页面 host 比对
        String host = ServerNavigationPolicy.hostOf("https://jerocine.art");
        assertTrue(ServerNavigationPolicy.isInAppHost("jerocine.art", host));
        assertFalse(ServerNavigationPolicy.isInAppHost("cdn.example.com", host));
    }
}
