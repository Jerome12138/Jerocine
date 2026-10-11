package com.jerocine.app;

import android.content.Context;
import android.content.SharedPreferences;
import android.net.Uri;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;

import com.getcapacitor.Bridge;
import com.getcapacitor.BridgeWebViewClient;

/**
 * 继承 Capacitor 的 BridgeWebViewClient, 做两件事:
 *
 * 1. 导航规则: 指向"当前服务器 host"的顶层导航(A 标签 / location.href / location.reload)
 *    留在 WebView 内; 其余(其它域名的外链)仍按 Capacitor 默认交给系统浏览器打开。
 *    判定逻辑抽在 {@link ServerNavigationPolicy}(纯逻辑 + JVM 单测)。
 *
 * 2. 运行时降级兜底(P2.3): WebView 连续加载失败或渲染进程崩溃达到阈值后,
 *    写 SharedPreferences 强制原生模式(jc-shell-mode=native)并重启应用,
 *    让 EntryRouter 下次走 NativeShellActivity。避免低配设备 WebView 反复
 *    崩溃时用户卡在白屏。
 *
 * 为什么不走 config 的 server.allowNavigation: 见 ServerNavigationPolicy 类注释。
 */
class ServerNavigationWebViewClient extends BridgeWebViewClient {

    /** 取"当前服务器 host" —— 用户可改服务器地址(抽屉里的"重置服务器"), 故用回调而非常量 */
    interface ServerHostProvider {
        String currentServerHost();
    }

    /** WebView 连续失败达到阈值时的回调 —— 由 WebShellActivity 实现重启逻辑 */
    interface WebViewFallbackCallback {
        void onWebViewUnrecoverable();
    }

    /** 连续失败多少次后触发降级(主框架错误 + 渲染进程崩溃累计) */
    private static final int FAIL_THRESHOLD = 3;
    /** 成功加载后多久重置失败计数(ms) —— 给一个缓冲, 避免偶发错误被累计 */
    private static final long RESET_AFTER_SUCCESS_MS = 30_000L;

    private final Bridge bridge;
    private final ServerHostProvider hostProvider;
    private final WebViewFallbackCallback fallbackCallback;
    private final SharedPreferences prefs;

    private int consecutiveFailures = 0;
    private long lastSuccessTime = 0L;
    private boolean fallbackTriggered = false;

    ServerNavigationWebViewClient(Bridge bridge, ServerHostProvider hostProvider,
                                   WebViewFallbackCallback fallbackCallback,
                                   SharedPreferences prefs) {
        super(bridge);
        this.bridge = bridge;
        this.hostProvider = hostProvider;
        this.fallbackCallback = fallbackCallback;
        this.prefs = prefs;
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

    // ==================== 运行时降级兜底 ====================

    /** API 23+: 主框架加载失败时累计错误计数 */
    @Override
    public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
        super.onReceivedError(view, request, error);
        if (request == null || !request.isForMainFrame()) return;
        // 忽略一些常见的非致命错误码(如 favicon 404、取消加载)
        if (error != null) {
            int code = error.getErrorCode();
            if (code == ERROR_UNSUPPORTED_SCHEME || code == ERROR_FILE_NOT_FOUND) return;
        }
        recordFailure("mainFrame error: " + (error == null ? "?" : error.getErrorCode()));
    }

    /** API 23 以下的旧重载 */
    @Override
    @SuppressWarnings("deprecation")
    public void onReceivedError(WebView view, int errorCode, String description, String failingUrl) {
        super.onReceivedError(view, errorCode, description, failingUrl);
        if (errorCode == ERROR_UNSUPPORTED_SCHEME || errorCode == ERROR_FILE_NOT_FOUND) return;
        recordFailure("mainFrame error(legacy): " + errorCode);
    }

    /**
     * 渲染进程崩溃 —— WebView 最严重的失败。返回 true 表示应用自己处理(不杀进程),
     * 然后立即触发降级(即使没到阈值, 因为渲染进程崩溃本身就不可恢复)。
     */
    @Override
    public boolean onRenderProcessGone(WebView view, android.webkit.RenderProcessGoneDetail detail) {
        recordFailure("render process gone, didCrash=" + (detail != null && detail.didCrash()));
        // 渲染进程没了, 当前 WebView 已不可用; 直接触发降级, 不等阈值
        triggerFallback();
        return true;
    }

    /** 页面加载完成(progress>=100 时由 WebChromeClient 调) —— 重置失败计数 */
    void onPageLoadSuccess() {
        lastSuccessTime = System.currentTimeMillis();
        consecutiveFailures = 0;
    }

    private void recordFailure(String reason) {
        android.util.Log.w("JerocineWeb", "WebView failure: " + reason
                + " (consecutive=" + (consecutiveFailures + 1) + "/" + FAIL_THRESHOLD + ")");
        // 距上次成功超过缓冲期才累计; 刚成功过的偶发错误忽略
        if (System.currentTimeMillis() - lastSuccessTime < RESET_AFTER_SUCCESS_MS
                && consecutiveFailures == 0) {
            return;
        }
        consecutiveFailures++;
        if (consecutiveFailures >= FAIL_THRESHOLD) {
            triggerFallback();
        }
    }

    private void triggerFallback() {
        if (fallbackTriggered) return;
        fallbackTriggered = true;
        android.util.Log.w("JerocineWeb", "WebView unrecoverable, forcing native shell mode");
        // 写 prefs 强制原生模式 —— 与 DeviceProbe / 设置页壳模式开关共用同一键
        prefs.edit().putString(WebShellActivity.KEY_SHELL_MODE, "native").apply();
        if (fallbackCallback != null) {
            fallbackCallback.onWebViewUnrecoverable();
        }
    }

    private boolean isInApp(Uri url) {
        if (url == null) {
            return false;
        }
        return ServerNavigationPolicy.isInAppHost(url.getHost(), hostProvider.currentServerHost());
    }
}
