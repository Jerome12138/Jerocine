package com.jerocine.player;

import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;

/**
 * 本地文件播放判定与标题提取 — 纯 Java 逻辑(不依赖 android API 特性), 可直接单测.
 *
 * <p>文件管理器/SAF 传给播放器的 intent 形态: {@code ACTION_VIEW} + {@code data=content://...}.
 * 播放器据此切换本地模式: 不初始化 HLS 广告过滤链路, 改用 DefaultMediaSourceFactory
 * 按扩展名自动识别 mp4/ts/m3u8(本地分片相对路径由 media3 按文件所在目录解析).
 */
public final class LocalPlayback {

    /** intent action 是否本地文件打开(文件管理器 ACTION_VIEW). */
    public static boolean isLocalAction(String action) {
        return android.content.Intent.ACTION_VIEW.equals(action);
    }

    /**
     * 显示标题: 取 URI 最后一段路径(文件名), URL 解码, 去掉常见文件扩展名;
     * 空/异常回退 fallback.
     */
    public static String displayTitle(String lastPathSegment, String fallback) {
        String name = lastPathSegment;
        if (name == null) name = "";
        int slash = Math.max(name.lastIndexOf('/'), name.lastIndexOf('\\'));
        if (slash >= 0) name = name.substring(slash + 1);
        try {
            // URLDecoder 会把 '+' 解成空格 → 先转义为 %2B, 文件名里的 + 才能保留
            name = URLDecoder.decode(name.replace("+", "%2B"), StandardCharsets.UTF_8.name());
        } catch (Exception ignore) {
        }
        name = name.trim();
        if (name.isEmpty()) return fallback == null || fallback.isEmpty() ? "本地视频" : fallback;
        int dot = name.lastIndexOf('.');
        if (dot > 0) name = name.substring(0, dot);
        return name.isEmpty() ? (fallback == null || fallback.isEmpty() ? "本地视频" : fallback) : name;
    }
}
