package com.jerocine.player;

import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;

/**
 * Web 端播放设置快照契约(2026-10-18) —— 与前端 {@code web/src/composables/useWebSettings.ts}
 * 的 {@code webSettingsSnapshot()} 一一对应, 格式 {@code "adFilter=1;filterMode=client;relayCache=0;autoPlayNext=1"}。
 *
 * <p>为什么不用 JSON: 快照经 WebView evaluateJavascript 在 Java↔JS 两侧穿梭, 还要在
 * player-core 的 JVM 单测里解析(Android 的 org.json 在纯 JVM 单测不可用); 值全部来自
 * 固定白名单(布尔/枚举), 无需转义, {@code k=v;} 平铺格式是零依赖且两端各 ~20 行的方案。
 *
 * <p>消费方: {@code JcSettingsDrawer} 的「播放设置(Web)」组 —— web 壳里经
 * {@code window.__jcConfig} 钩子读写, 数据始终存 web localStorage(与 /settings 配置页同一份)。
 */
public final class WebSettingsContract {

    /** 广告过滤开关(布尔, "1"/"0") */
    public static final String KEY_AD_FILTER = "adFilter";
    /** 过滤取源方式(client=端侧取源默认 / server=云端取源) */
    public static final String KEY_FILTER_MODE = "filterMode";
    /** 全量中转开关(布尔) */
    public static final String KEY_RELAY_CACHE = "relayCache";
    /** 自动连播开关(布尔) */
    public static final String KEY_AUTO_PLAY_NEXT = "autoPlayNext";

    public static final String FILTER_MODE_CLIENT = "client";
    public static final String FILTER_MODE_SERVER = "server";

    private WebSettingsContract() {}

    /**
     * 解析快照为键值表(容错: 忽略空段/无等号段/空白; 未知键原样保留, 读取方自行校验)。
     * {@code null}/空串返回空表 —— 调用方用 {@link #bool} 的默认值兜底。
     */
    public static Map<String, String> parse(String snapshot) {
        Map<String, String> out = new LinkedHashMap<>();
        if (snapshot == null) return out;
        for (String pair : snapshot.split(";")) {
            if (pair == null) continue;
            String p = pair.trim();
            int eq = p.indexOf('=');
            if (eq <= 0 || eq == p.length() - 1) continue;
            out.put(p.substring(0, eq).trim(), p.substring(eq + 1).trim());
        }
        return out;
    }

    /** 读布尔项: "1"/"true"=开, "0"/"false"=关, 缺失/非法=默认值。 */
    public static boolean bool(Map<String, String> snapshot, String key, boolean def) {
        String v = snapshot.get(key);
        if (v == null || v.isEmpty()) return def;
        if ("1".equals(v) || "true".equalsIgnoreCase(v)) return true;
        if ("0".equals(v) || "false".equalsIgnoreCase(v)) return false;
        return def;
    }

    /** 归一化取源方式: 仅 server 视为云端, 其余(含空/未知)一律归 client(默认)。 */
    public static String normalizeFilterMode(String mode) {
        return FILTER_MODE_SERVER.equalsIgnoreCase(mode == null ? "" : mode.trim())
                ? FILTER_MODE_SERVER : FILTER_MODE_CLIENT;
    }

    /** 取源方式循环切换(client ↔ server) —— 抽屉行点击用。 */
    public static String nextFilterMode(String mode) {
        return FILTER_MODE_SERVER.equals(normalizeFilterMode(mode))
                ? FILTER_MODE_CLIENT : FILTER_MODE_SERVER;
    }

    /** 取源方式的展示文案(与 /settings 配置页两档文案一致)。 */
    public static String filterModeLabel(String mode) {
        return FILTER_MODE_SERVER.equals(normalizeFilterMode(mode)) ? "云端取源" : "端侧取源";
    }

    /** 展示文案统一小写比较入口(防大小写漂移)。 */
    static boolean isServerMode(String mode) {
        return FILTER_MODE_SERVER.equals(normalizeFilterMode(mode).toLowerCase(Locale.ROOT));
    }
}
