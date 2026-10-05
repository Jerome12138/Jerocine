package com.jerocine.player;

import java.net.URLEncoder;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * 播放地址 / 线路策略 — 纯逻辑, 不依赖 Android, 可直接单测.
 *
 * 策略说明(过滤开启 + m3u8 源时, 按优先级):
 *   ① **默认 → 原始地址**: 清单由设备自己抓, 再送端侧混合过滤(/v1/m3u8/filter)剔广告。
 *      与 web 播放页一致("端侧混合过滤优先"), 比让服务器回源少一跳, 起播更快。
 *   ② 本集端侧过滤失败 → 升级为服务端清单代理(/v1/m3u8/proxy, proxyMedia=0):
 *      由服务器抓清单 + 过滤。仅在服务端抓得到该源(proxyUsable)时才可用。
 *   ③ 用户开"中转" / 本集直连失败自愈 → 代理 + proxyMedia=1(分片也过服务器转发)。
 *   ④ 以上都走不通(服务端抓不到该源 / 代理本身也失败) → 回退原始地址(广告不过滤, 但保证能放)。
 *
 * 旧版是"服务端代理优先"(① 就是包代理), 导致服务端可达但回源慢的源每次起播都要白等
 * 0.6~9s, 且代理一挂就弹"清单代理失败, 已切换直连"; 现与 web 对齐为端侧优先。
 */
public final class PlayerUrls {

    private PlayerUrls() {}

    /**
     * 该清单是否还需要在设备侧再过滤一次.
     * 已经是 /m3u8/proxy 的清单由服务端处理过, 不再重复同步 POST(否则只是白白多一跳);
     * file:// / content:// 是本地/离线清单(下载时已过滤), 同样不再过滤.
     */
    public static boolean needsClientSideFilter(String playlistUrl) {
        if (playlistUrl == null) return true;
        String lower = playlistUrl.toLowerCase(Locale.US);
        if (lower.startsWith("file://") || lower.startsWith("content://")) return false;
        return !lower.contains("/m3u8/proxy?");
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
     * @param rawUrl      原始 m3u8
     * @param adFilterOn  广告过滤总开关
     * @param forceRaw    本集强制原始(代理失败后的兜底)
     * @param forceProxy  本集强制服务端代理(端侧过滤失败后的升级; 也承载"本片源端侧已失败"的粘性偏好)
     * @param relay       分片全量中转(用户"中转"开关 或 本集直连失败自愈) → proxyMedia=1
     * @param proxyUsable 本片源服务端能否代理(adFilterOk=false ⇒ 抓不到)。false ⇒ 任何代理包装都无意义
     * @param proxyBase   代理 base(到 /api), 空表示无代理 → 只能用原始
     */
    public static String buildPlayableUrl(
            String rawUrl,
            boolean adFilterOn,
            boolean forceRaw,
            boolean forceProxy,
            boolean relay,
            boolean proxyUsable,
            String proxyBase
    ) {
        if (rawUrl == null) return "";
        if (!adFilterOn || forceRaw) return rawUrl;
        if (!isM3u8(rawUrl)) return rawUrl;
        if (rawUrl.toLowerCase(Locale.US).contains("/m3u8/proxy?")) return rawUrl;
        // 默认不包装代理: 清单由设备抓 → FilterPlaylistParser 送 /v1/m3u8/filter 端侧过滤.
        // 只有"端侧失败升级"或"要中转分片"才需要服务端介入。
        if (!forceProxy && !relay) return rawUrl;
        if (!proxyUsable || proxyBase == null || proxyBase.isEmpty()) return rawUrl;
        try {
            String base = proxyBase.endsWith("/")
                    ? proxyBase.substring(0, proxyBase.length() - 1)
                    : proxyBase;
            return base + "/v1/m3u8/proxy?src="
                    + URLEncoder.encode(rawUrl, "UTF-8")
                    + "&filterAds=1&proxyMedia=" + (relay ? "1" : "0");
        } catch (Exception e) {
            return rawUrl;
        }
    }

    /**
     * 从(已绝对化的)HLS 清单文本里提取**子清单** URL — master 表的各码率子表, 供端侧预取下钻一层。
     *
     * 为什么必须下钻: /v1/m3u8/filter 只做"剔广告 + URI 绝对化", **不改写子表地址**
     * (服务端 FilterText 以 proxyChild=false 调用)。所以 master 过滤后 ExoPlayer 仍会去抓
     * **原始 CDN 子表**, 再送一次 filter ⇒ 端侧路径每集要付两次过滤(实测单次 1.4~5.1s)。
     * 预取器不把子表一起预热, 切集就仍要白等一轮。
     *
     * 识别规则: `#EXT-X-STREAM-INF` 之后的第一行(非注释非空)是子清单; `#EXT-X-MEDIA` 的
     * `URI="..."` 也是子清单(备用音轨/字幕)。只收 m3u8 形态的 URI, 去重保序。
     *
     * @param max 最多返回几个 —— 多码率源按 AUTO 选择的顺序取前面几个即可, 避免塞爆预取缓存
     */
    public static List<String> childPlaylistUrls(String text, int max) {
        List<String> out = new ArrayList<>();
        if (text == null || max <= 0) return out;
        Set<String> seen = new HashSet<>();
        boolean pendingStreamUri = false;
        for (String rawLine : text.split("\r?\n")) {
            if (out.size() >= max) break;
            String line = rawLine.trim();
            if (line.isEmpty()) continue;
            if (line.startsWith("#")) {
                String upper = line.toUpperCase(Locale.US);
                if (upper.startsWith("#EXT-X-STREAM-INF")) {
                    pendingStreamUri = true; // 紧随其后的第一个非注释行才是子清单
                } else if (upper.startsWith("#EXT-X-MEDIA")) {
                    pendingStreamUri = false;
                    addChildPlaylist(out, seen, attributeUri(line), max);
                }
                continue;
            }
            if (pendingStreamUri) {
                pendingStreamUri = false;
                addChildPlaylist(out, seen, line, max);
            }
        }
        return out;
    }

    /** 取标签行里 `URI="..."` 的属性值; 无则 null. */
    private static String attributeUri(String tagLine) {
        int at = tagLine.toUpperCase(Locale.US).indexOf("URI=\"");
        if (at < 0) return null;
        int start = at + 5;
        int end = tagLine.indexOf('"', start);
        if (end <= start) return null;
        return tagLine.substring(start, end);
    }

    private static void addChildPlaylist(List<String> out, Set<String> seen, String uri, int max) {
        if (uri == null || uri.isEmpty() || out.size() >= max) return;
        if (!isM3u8(uri)) return; // 只认 m3u8 形态(过滤后已是绝对地址)
        if (seen.add(uri)) out.add(uri);
    }

    /**
     * 直连失败 → 是否应改走全量中转.
     *
     * 判据: 当前**不是**已在全量中转的地址(proxyMedia=1, 再中转无意义), 且这次失败的请求不是代理
     * 请求本身(代理挂了该走"回退原始", 不是中转)。
     * 与旧版的差别: 不再要求"当前必须是 proxy 清单" —— 端侧混合过滤成为主路径后, 当前地址是
     * **原始 m3u8**, 这时直连清单/分片失败同样应该升级为中转(服务器能抓到时即可自愈)。
     */
    public static boolean shouldRetryWithRelay(String currentMediaUrl, String failedRequestUrl) {
        if (currentMediaUrl == null || failedRequestUrl == null || failedRequestUrl.isEmpty()) {
            return false;
        }
        if (currentMediaUrl.toLowerCase(Locale.US).contains("proxymedia=1")) return false;
        return !failedRequestUrl.toLowerCase(Locale.US).contains("/m3u8/proxy?");
    }
}
