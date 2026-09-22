package com.jerocine.player;

import java.net.URLEncoder;
import java.util.Locale;

/**
 * 播放地址 / 线路策略 — 纯逻辑, 不依赖 Android, 可直接单测.
 *
 * 策略说明:
 *   - 服务端 /v1/m3u8/proxy 已完成清单过滤 + 分片地址改写, 默认 proxyMedia=0(分片由设备直连);
 *   - 直连分片失败(CDN 地域封/证书链问题)时, 对"本集"改走 proxyMedia=1(分片也过中转);
 *   - 中转仍失败则对"本集"回退原始地址(广告不过滤, 但保证能放).
 */
public final class PlayerUrls {

    private PlayerUrls() {}

    /**
     * 该清单是否还需要在设备侧再过滤一次.
     * 已经是 /m3u8/proxy 的清单由服务端处理过, 不再重复同步 POST(否则只是白白多一跳).
     */
    public static boolean needsClientSideFilter(String playlistUrl) {
        return playlistUrl == null
                || !playlistUrl.toLowerCase(Locale.US).contains("/m3u8/proxy?");
    }

    /** 是否 HLS 清单(可被 /m3u8/proxy 包装). */
    public static boolean isM3u8(String rawUrl) {
        if (rawUrl == null) return false;
        String lower = rawUrl.toLowerCase(Locale.US);
        return lower.endsWith(".m3u8") || lower.contains(".m3u8?") || lower.contains(".m3u8#");
    }

    /**
     * 计算某集的实际播放地址.
     *
     * @param rawUrl     原始 m3u8
     * @param adFilterOn 广告过滤总开关
     * @param forceRaw   本集强制原始(代理失败后的兜底)
     * @param forceRelay 本集强制全量中转(直连分片失败后的自愈)
     * @param proxyBase  代理 base(到 /api), 空表示无代理 → 直接用原始
     */
    public static String buildPlayableUrl(
            String rawUrl,
            boolean adFilterOn,
            boolean forceRaw,
            boolean forceRelay,
            String proxyBase
    ) {
        if (rawUrl == null) return "";
        if (!adFilterOn || forceRaw || proxyBase == null || proxyBase.isEmpty()) return rawUrl;
        if (!isM3u8(rawUrl)) return rawUrl;
        if (rawUrl.toLowerCase(Locale.US).contains("/m3u8/proxy?")) return rawUrl;
        try {
            String base = proxyBase.endsWith("/")
                    ? proxyBase.substring(0, proxyBase.length() - 1)
                    : proxyBase;
            return base + "/v1/m3u8/proxy?src="
                    + URLEncoder.encode(rawUrl, "UTF-8")
                    + "&filterAds=1&proxyMedia=" + (forceRelay ? "1" : "0");
        } catch (Exception e) {
            return rawUrl;
        }
    }

    /**
     * 直连分片失败 → 是否应改走全量中转.
     * 仅当"当前播放的是 proxy 清单且 proxyMedia=0(分片直连)"、"这次失败的请求不是 proxy 请求"
     * (即确实是 CDN 分片拿不到) 时才成立.
     */
    public static boolean shouldRetryWithRelay(String currentMediaUrl, String failedRequestUrl) {
        if (currentMediaUrl == null || failedRequestUrl == null) return false;
        String current = currentMediaUrl.toLowerCase(Locale.US);
        String failed = failedRequestUrl.toLowerCase(Locale.US);
        return current.contains("/m3u8/proxy?")
                && current.contains("proxymedia=0")
                && !failed.isEmpty()
                && !failed.contains("/m3u8/proxy?");
    }
}
