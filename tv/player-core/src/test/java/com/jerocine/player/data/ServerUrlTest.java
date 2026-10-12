package com.jerocine.player.data;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * com.jerocine.player.data.ServerUrl 的 JVM 单测(本机可跑, 不需要设备/模拟器):
 *   cd web/android && ./gradlew :player-core:testDebugUnitTest
 *
 * 覆盖的真机回归: 顶栏「刷新」按钮(location.reload) 曾被 Capacitor 当外链丢给系统浏览器,
 * 修法就是"服务器 host 的顶层导航留在 WebView" —— 这条判定的边界都在这里钉住。
 * 另覆盖 normalize 的归一边界(去空白 / 去尾斜杠)。
 */
public class ServerUrlTest {

    // ============ normalize ============

    @Test
    public void normalize_trimsAndStripsTrailingSlashes() {
        assertEquals("https://example.com", ServerUrl.normalize(" https://example.com/// "));
    }

    @Test
    public void normalize_keepsInnerPath() {
        assertEquals("http://1.2.3.4:8080/x", ServerUrl.normalize("http://1.2.3.4:8080/x/"));
    }

    @Test
    public void normalize_nullBecomesEmpty() {
        assertEquals("", ServerUrl.normalize(null));
    }

    // ============ isInAppHost ============

    @Test
    public void sameHost_isInApp() {
        assertTrue(ServerUrl.isInAppHost("jerocine.art", "jerocine.art"));
    }

    @Test
    public void hostMatch_isCaseInsensitive_andTrimmed() {
        assertTrue(ServerUrl.isInAppHost("Jerocine.ART", " jerocine.art "));
    }

    @Test
    public void otherHost_isNotInApp() {
        // 外链必须继续走系统浏览器 —— 别把整站判成站内
        assertFalse(ServerUrl.isInAppHost("www.baidu.com", "jerocine.art"));
        assertFalse(ServerUrl.isInAppHost("jerocine.art.evil.com", "jerocine.art"));
    }

    @Test
    public void suffixOrPrefixIsNotAMatch() {
        assertFalse(ServerUrl.isInAppHost("art", "jerocine.art"));
        assertFalse(ServerUrl.isInAppHost("jerocine.art.cn", "jerocine.art"));
    }

    @Test
    public void nullOrBlank_isNotInApp() {
        assertFalse(ServerUrl.isInAppHost(null, "jerocine.art"));
        assertFalse(ServerUrl.isInAppHost("jerocine.art", null));
        assertFalse(ServerUrl.isInAppHost("", "jerocine.art"));
        assertFalse(ServerUrl.isInAppHost("jerocine.art", "   "));
    }

    // ============ hostOf ============

    @Test
    public void hostOf_standardUrls() {
        assertEquals("jerocine.art", ServerUrl.hostOf("https://jerocine.art"));
        assertEquals("jerocine.art", ServerUrl.hostOf("https://jerocine.art/"));
        assertEquals("jerocine.art", ServerUrl.hostOf("http://jerocine.art/index.html"));
        assertEquals("jerocine.art", ServerUrl.hostOf("https://jerocine.art?a=1#b"));
    }

    @Test
    public void hostOf_withPort_stripsPort() {
        assertEquals("1.2.3.4", ServerUrl.hostOf("http://1.2.3.4:8080/x"));
        assertEquals("jerocine.art", ServerUrl.hostOf("https://jerocine.art:8443"));
    }

    @Test
    public void hostOf_withoutScheme_stillParses() {
        // 用户手填的朴素写法(没有 scheme)不能让 URI 解析抛错后整条判定失效
        assertEquals("jerocine.art", ServerUrl.hostOf("jerocine.art"));
        assertEquals("1.2.3.4", ServerUrl.hostOf("1.2.3.4:8080"));
        assertEquals("jerocine.art", ServerUrl.hostOf("jerocine.art/path"));
    }

    @Test
    public void hostOf_blankOrNull() {
        assertNull(ServerUrl.hostOf(null));
        assertNull(ServerUrl.hostOf(""));
        assertNull(ServerUrl.hostOf("   "));
    }

    @Test
    public void hostOf_thenIsInApp_roundTrip() {
        // 真实链路: prefs 里的 server_url → hostOf → 与页面 host 比对
        String host = ServerUrl.hostOf("https://jerocine.art");
        assertTrue(ServerUrl.isInAppHost("jerocine.art", host));
        assertFalse(ServerUrl.isInAppHost("cdn.example.com", host));
    }
}
