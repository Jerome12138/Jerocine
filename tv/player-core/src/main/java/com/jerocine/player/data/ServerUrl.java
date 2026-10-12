package com.jerocine.player.data;

import java.net.URI;

/**
 * 服务器地址工具 —— 单包双壳共用的**唯一一份**。
 *
 * 合并前这里有两份语义相邻却各写各的实现：原生壳 {@code SettingsModel.kt} 的
 * {@code normalizeServerUrl}（只做去空白/去尾斜杠）与 web 壳 {@code ServerNavigationPolicy}
 * 的 {@code hostOf / isInAppHost}（站内导航判定）。细则 §5.1 要求合并成 core 一份。
 *
 * 关于 {@link #isInAppHost} 的背景（2026-10-10 真机实测，原 web 壳注释）：
 * 远程加载的 SPA 里，Capacitor 的 {@code Bridge.launchIntent()} 只把 appUrl 同源或
 * {@code server.allowNavigation} 列出的 host 当作站内，其余一律丢给系统浏览器 ——
 * 于是任何指向线上域名的顶层导航都会被踢出 App（顶栏「刷新」按钮的 location.reload() 正好命中）。
 * 不改 {@code allowNavigation} 是因为那份配置还会被塞进 Capacitor 的 authorities，
 * 导致 WebViewLocalServer 去找 APK 内置 assets/public 里的同名旧文件、顶掉线上新产物。
 */
public final class ServerUrl {

    private ServerUrl() {}

    /**
     * 归一化用户填写的服务器地址：去首尾空白 + 去末尾斜杠。
     * 例：{@code " https://example.com/// "} → {@code "https://example.com"}
     */
    public static String normalize(String value) {
        if (value == null) return "";
        String s = value.trim();
        while (s.endsWith("/")) {
            s = s.substring(0, s.length() - 1);
        }
        return s;
    }

    /**
     * 从服务器地址里取出 host。用户可能填：
     *   https://jerocine.art  |  https://jerocine.art/  |  http://1.2.3.4:8080/x
     *   jerocine.art          |  1.2.3.4:8080          |  user@host:8080
     * 解析不出 host 时返回 null。
     */
    public static String hostOf(String serverUrl) {
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

    /**
     * urlHost 是否属于"当前服务器" ⇒ 顶层导航应留在 WebView 内。
     * 任一侧为空/空白 ⇒ false（保持 Capacitor 默认语义：当外部链接丢系统浏览器）。
     */
    public static boolean isInAppHost(String urlHost, String serverHost) {
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
}
