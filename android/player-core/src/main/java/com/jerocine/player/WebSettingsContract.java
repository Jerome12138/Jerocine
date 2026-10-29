package com.jerocine.player;

import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Web 端播放设置快照契约(2026-10-18) —— 与前端 {@code web/src/composables/useWebSettings.ts}
 * 的 {@code webSettingsSnapshot()} 一一对应, 格式 {@code "adFilter=1;autoPlayNext=1"}。
 *
 * <p>为什么不用 JSON: 快照经 WebView evaluateJavascript 在 Java↔JS 两侧穿梭, 还要在
 * player-core 的 JVM 单测里解析(Android 的 org.json 在纯 JVM 单测不可用); 值全部来自
 * 固定白名单(布尔), 无需转义, {@code k=v;} 平铺格式是零依赖且两端各 ~20 行的方案。
 *
 * <p>消费方: {@code JcSettingsDrawer} 的「播放设置(Web)」组 —— web 壳里经
 * {@code window.__jcConfig} 钩子读写, 数据始终存 web localStorage(与 /settings 配置页同一份)。
 *
 * <p>历史(2026-10-28 下线): filterMode / relayCache 随服务端 m3u8 过滤/中转链路删除而移除。
 */
public final class WebSettingsContract {

    /** 广告过滤开关(布尔, "1"/"0") */
    public static final String KEY_AD_FILTER = "adFilter";
    /** 自动连播开关(布尔) */
    public static final String KEY_AUTO_PLAY_NEXT = "autoPlayNext";

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
}
