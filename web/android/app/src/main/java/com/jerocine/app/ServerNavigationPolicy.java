package com.jerocine.app;

import java.net.URI;

/**
 * 「站内导航」判定的**纯逻辑**(不碰 View / Context / android.net.Uri) —— 单独成类是为了能在
 * 本机跑 JVM 单测(见 app/src/test/java/com/jerocine/app/ServerNavigationPolicyTest.java)。
 *
 * 背景(2026-10-10 真机实测): web 壳的 SPA 是**远程加载**的(MainActivity.loadServer →
 * wv.loadUrl("https://jerocine.art")), 而 Capacitor 的 Bridge.launchIntent() 只把
 *   - appUrl 同源(config 里 androidScheme=http ⇒ appUrl = http://localhost), 或
 *   - server.allowNavigation 里列出的 host
 * 当作站内; 其余一律 Intent.ACTION_VIEW 丢给系统浏览器。
 * 于是**任何指向线上域名的顶层导航**都会被踢出 App —— 顶栏「刷新」按钮的
 * location.reload() 正好命中(实测: 点一下前台就从 art.jerocine.app 变成 com.android.browser)。
 *
 * 为什么不改用 server.allowNavigation: 那份配置还会被塞进 Capacitor 的 `authorities`
 * ⇒ WebViewLocalServer 会去 APK 内置的 assets/public 里找同名文件, 而那是打包时的
 * **整站旧副本**(含 index.html / sw.js) —— 会把线上新产物顶掉。所以只覆盖"导航判定"这一处。
 */
final class ServerNavigationPolicy {

    private ServerNavigationPolicy() {}

    /**
     * urlHost 是否属于"当前服务器" ⇒ 顶层导航应留在 WebView 内。
     * 任一侧为空/空白 ⇒ false(保持 Capacitor 默认语义: 当外部链接丢系统浏览器)。
     */
    static boolean isInAppHost(String urlHost, String serverHost) {
        if (urlHost == null || serverHost == null) {
            return false;
        }
        String a = urlHost.trim();
        String b = serverHost.trim();
        if (a.isEmpty() || b.isEmpty()) {
            return false;
        }
        return a.equalsIgnoreCase(b);
    }

    /**
     * 从服务器地址里取出 host。用户可能填:
     *   https://jerocine.art  |  https://jerocine.art/  |  http://1.2.3.4:8080/x
     *   jerocine.art          |  1.2.3.4:8080          |  user@host:8080
     * 解析不出 host 时返回 null。
     */
    static String hostOf(String serverUrl) {
        if (serverUrl == null) {
            return null;
        }
        String s = serverUrl.trim();
        if (s.isEmpty()) {
            return null;
        }
        // 标准形式(带 scheme)优先 —— URI 能正确剥掉端口与路径
        try {
            String h = URI.create(s).getHost();
            if (h != null && !h.isEmpty()) {
                return h;
            }
        } catch (IllegalArgumentException ignored) {
            // 落到下面的容错解析(无 scheme 的朴素写法会让 URI 解析抛错)
        }
        // 容错: 手工剥 userinfo / path / query / port
        String hostPort = s;
        int slash = hostPort.indexOf('/');
        if (slash >= 0) {
            hostPort = hostPort.substring(0, slash);
        }
        int query = hostPort.indexOf('?');
        if (query >= 0) {
            hostPort = hostPort.substring(0, query);
        }
        int at = hostPort.lastIndexOf('@');
        if (at >= 0) {
            hostPort = hostPort.substring(at + 1);
        }
        int colon = hostPort.lastIndexOf(':');
        if (colon >= 0) {
            hostPort = hostPort.substring(0, colon);
        }
        String host = hostPort.trim();
        return host.isEmpty() ? null : host;
    }
}
