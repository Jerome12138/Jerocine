package com.jerocine.player;

import okhttp3.OkHttpClient;

/**
 * 播放器入口 — 壳层唯一装配点.
 *
 * 壳层只经两个句柄与播放器打交道, 播放器本身不硬编码任何业务/品牌信息:
 *   - {@link #config()}  注入三样: callback(播放事件出口) / mediaClient(媒体 OkHttp,
 *     可带壳层自己的 TLS 放宽策略, 老设备 CA 兼容) / defaultProxyBase(未显式传
 *     proxyBase 时的兜底代理地址);
 *   - {@link #control()} 控制正在播放的实例(停止 / 倍速)与读取播放结局.
 *
 * 这两个都是进程内单例(壳层是 Application 级注入, 实例控制是全局唯一);
 * 原先散落的 5 个 static 字段全部收进它们, 不再有"裸静态变量"。
 */
public class JerocinePlayer {

    /** 壳层注入点 — 集中成一个对象, 一眼看清"壳给了播放器什么". */
    public static final class Config {

        private volatile PlayerActivity.PlayerEventCallback callback;
        private volatile OkHttpClient mediaClient;
        private volatile String defaultProxyBase = "";

        private Config() {}

        /** 播放事件出口; 传 null 表示注销(壳层退出时清掉, 防止回调打到已销毁的界面). */
        public void setCallback(PlayerActivity.PlayerEventCallback cb) {
            callback = cb;
        }

        /** 媒体 OkHttp 客户端 — 播放器统一加超时, 这里只提供连接池 / TLS 策略 / UA. */
        public void setMediaClient(OkHttpClient client) {
            mediaClient = client;
        }

        /** 未显式传 proxyBase 时的兜底代理地址; null 视作空串. */
        public void setDefaultProxyBase(String proxyBase) {
            defaultProxyBase = proxyBase == null ? "" : proxyBase;
        }
    }

    private static final Config CONFIG = new Config();
    private static final PlayerControl CONTROL = PlayerControl.get();

    private JerocinePlayer() {}

    /** 壳层装配入口(进程内唯一). */
    public static Config config() {
        return CONFIG;
    }

    /** 运行控制入口(进程内唯一). */
    public static PlayerControl control() {
        return CONTROL;
    }

    // ============================ 内部使用 ============================

    /** 分发一次播放事件给壳层. */
    static void dispatch(String name, org.json.JSONObject payload) {
        PlayerActivity.PlayerEventCallback cb = CONFIG.callback;
        if (cb == null) return;
        try {
            cb.onPlayerEvent(name, payload);
        } catch (Exception ignore) {
        }
    }

    static OkHttpClient mediaClient() {
        return CONFIG.mediaClient;
    }

    static String defaultProxyBase() {
        return CONFIG.defaultProxyBase;
    }
}
