package com.jerocine.app;

import android.net.Uri;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;

import com.getcapacitor.Bridge;
import com.getcapacitor.BridgeWebViewClient;

/**
 * 继承 Capacitor 的 BridgeWebViewClient, **只加一条规则**:
 * 指向"当前服务器 host"的顶层导航(A 标签跳转 / location.href / location.reload)留在
 * WebView 内; 其余(其它域名的外链)仍按 Capacitor 默认交给系统浏览器打开。
 *
 * 判定逻辑抽在 {@link ServerNavigationPolicy}(纯逻辑 + JVM 单测), 这里只做 Android 侧的接线。
 *
 * 为什么不走 config 的 server.allowNavigation: 见 ServerNavigationPolicy 类注释 ——
 * 它会把线上 host 加进 Capacitor 的 authorities, 让内置的 assets/public 整站旧副本
 * 有机会顶替线上产物。这里只覆盖导航判定, 其它一律 super/launchIntent 原样透传,
 * 不改变 Capacitor 的任何既有能力(本地资源拦截、外链打开方式都不变)。
 */
class ServerNavigationWebViewClient extends BridgeWebViewClient {

    /** 取"当前服务器 host" —— 用户可改服务器地址(抽屉里的"重置服务器"), 故用回调而非常量 */
    interface ServerHostProvider {
        String currentServerHost();
    }

    private final Bridge bridge;
    private final ServerHostProvider hostProvider;

    ServerNavigationWebViewClient(Bridge bridge, ServerHostProvider hostProvider) {
        super(bridge);
        this.bridge = bridge;
        this.hostProvider = hostProvider;
    }

    /** API 24+ 走这个重载 */
    @Override
    public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
        Uri url = (request == null) ? null : request.getUrl();
        if (isInApp(url)) {
            return false; // 站内 ⇒ WebView 自己导航(刷新/跳转都留在这里)
        }
        return url != null && bridge.launchIntent(url);
    }

    /** API 24 以下的旧重载(与 Capacitor 一样保持"外链走 launchIntent"的语义) */
    @Override
    @SuppressWarnings("deprecation")
    public boolean shouldOverrideUrlLoading(WebView view, String url) {
        Uri uri = (url == null) ? null : Uri.parse(url);
        if (isInApp(uri)) {
            return false;
        }
        return uri != null && bridge.launchIntent(uri);
    }

    private boolean isInApp(Uri url) {
        if (url == null) {
            return false;
        }
        return ServerNavigationPolicy.isInAppHost(url.getHost(), hostProvider.currentServerHost());
    }
}
