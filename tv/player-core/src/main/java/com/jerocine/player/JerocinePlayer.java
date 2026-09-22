package com.jerocine.player;

import okhttp3.OkHttpClient;

/**
 * 播放器入口 — 壳层唯一装配点.
 *
 * 壳层(WebView 壳 / TV 壳)在启动时注入三样东西, 播放器本身不硬编码任何业务/品牌信息:
 *   1. {@link #setCallback}        播放事件出口
 *   2. {@link #setMediaClient}     媒体 OkHttp(可带壳自己的 TLS 放宽策略, 老设备 CA 兼容)
 *   3. {@link #setDefaultProxyBase} 未显式传 proxyBase 时的兜底代理地址
 */
public class JerocinePlayer {

    private static volatile PlayerActivity.PlayerEventCallback sCallback;
    private static volatile OkHttpClient sMediaClient;
    private static volatile String sDefaultProxyBase = "";

    private JerocinePlayer() {}

    // ============================ 壳层装配 ============================

    public static void setCallback(PlayerActivity.PlayerEventCallback cb) {
        sCallback = cb;
    }

    /** 分发一次播放事件给壳层. */
    static void dispatch(String name, org.json.JSONObject payload) {
        PlayerActivity.PlayerEventCallback cb = sCallback;
        if (cb == null) return;
        try {
            cb.onPlayerEvent(name, payload);
        } catch (Exception ignore) {
        }
    }

    /**
     * 注入媒体请求用的 OkHttp(壳可带入自己的 TLS 策略 / UA / 日志).
     * 未注入时播放器自建一个默认客户端.
     */
    public static void setMediaClient(OkHttpClient client) {
        sMediaClient = client;
    }

    static OkHttpClient mediaClient() {
        return sMediaClient;
    }

    /** 未显式传 proxyBase 时的兜底代理地址(由壳注入, 播放器不硬编码域名). */
    public static void setDefaultProxyBase(String proxyBase) {
        sDefaultProxyBase = proxyBase == null ? "" : proxyBase;
    }

    static String defaultProxyBase() {
        return sDefaultProxyBase;
    }

    // ============================ 运行控制 ============================

    public static void stopCurrent() {
        PlayerActivity.stopRunningInstance();
    }

    public static void setSpeed(float speed) {
        PlayerActivity.setSpeedOnRunningInstance(speed);
    }
}
