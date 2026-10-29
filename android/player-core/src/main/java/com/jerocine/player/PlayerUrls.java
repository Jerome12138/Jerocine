package com.jerocine.player;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * 播放地址工具 — 纯逻辑, 不依赖 Android, 可直接单测.
 *
 * 端侧本地过滤是唯一路径(服务端 m3u8 代理/中转已下线): 设备自己抓清单,
 * FilterPlaylistParser 用本地 M3u8FilterCore 剔广告。这里的 isM3u8 判形、
 * hasMediaSegments master 识别、absolutize/childPlaylistUrls 预取下钻为其服务。
 */
public final class PlayerUrls {

    private PlayerUrls() {}

/**
 * 是否 HLS 清单.
 *
 * <p><b>不能只判 URL 形状</b>: 大量真实 HLS 源不以 .m3u8结尾 ——
 * 尾部斜杠({@code /playlist.m3u8/})、无扩展名({@code /hls/master}、{@code /live/index.HLS})、
 * 把 m3u8 放在查询串里({@code /play?type=m3u8&url=...})。它们都真的返回
 * {@code application/vnd.apple.mpegurl}, 但旧判据一律 false 会让
 * 端侧过滤/角标/子表预取多处连锁失效。
 */
public static boolean isM3u8(String rawUrl) {
        if (rawUrl == null) return false;
        String s = rawUrl.toLowerCase(Locale.US);
        // 依次剥离 fragment 与 query, 只留 path 判形态; query 另存(有些源把 m3u8 写在查询串里)。
        int hash = s.indexOf('#');
        String beforeFragment = (hash > 0) ? s.substring(0, hash) : s;
        int q = beforeFragment.indexOf('?');
        String path = (q >= 0) ? beforeFragment.substring(0, q) : beforeFragment;
        String query = (q >= 0) ? beforeFragment.substring(q + 1) : "";
        // 尾部形态: 注意要兼容"尾斜杠"——部分 CDN 返回 /playlist.m3u8/ 同样是清单
        while (path.endsWith("/")) {
            path = path.substring(0, path.length() - 1);
        }
        if (path.endsWith(".m3u8") || path.endsWith("/m3u8")
                || path.endsWith(".hls") || path.endsWith("/hls")) {
            return true;
        }
        return path.contains("/m3u8/") || path.contains(".m3u8/")
                || path.contains("/hls/") || path.contains("/.hls/")
                || query.contains("m3u8") || query.contains(".hls");
}

    /**
     * 判断 m3u8 文本是否为**媒体列表**(含分片, 有 #EXTINF) —— 区别于 master 主表。
     *
     * <p>master 主表只有变体声明(#EXT-X-STREAM-INF/#EXT-X-MEDIA), 没有任何分片可剔:
     * 服务端 /v1/m3u8/filter 对它的全部产出就是"URI 绝对化", 而 ExoPlayer 解析时本来就会做。
     * 所以 master 层可以**整层免 POST**(播放路径 master-direct / 预取器同款), 把端侧路径的
     * "master + 子表"两趟"上行清单 + 服务器往返"(实测单次 0.13~3.3s)减为一趟。
     * 判据用 #EXTINF: 按 HLS 规范媒体列表必有它, master/空表没有 —— 与服务端
     * FilterText"无分片即无事可做"的行为一致, 跳过不丢任何过滤能力。
     */
    public static boolean hasMediaSegments(String text) {
        if (text == null || text.isEmpty()) return false;
        for (String rawLine : text.split("\r?\n")) {
            if (rawLine.trim().toUpperCase(Locale.US).startsWith("#EXTINF")) return true;
        }
        return false;
    }

    /** byte[] 版本，与上面的 String 版本判据完全一致。 */
    public static boolean hasMediaSegments(byte[] data) {
        if (data == null || data.length == 0) return false;
        return hasMediaSegments(new String(data, java.nio.charset.StandardCharsets.UTF_8));
    }

    /**
     * 相对引用 → 绝对 URL; 解析不了返回 null(调用方跳过即可, 不影响正确性)。
     *
     * <p>供预取器在"master 免 POST 直通"后下钻子表: 以前 master 的子表地址是靠服务端
     * 过滤输出顺手绝对化的, 那一层不再 POST 后需端侧自己解析。先走 java.net.URI
     * (标准 RFC 3986 resolve); 源 URL 常含未转义字符(空格/中文/|)会抛异常 → 手工兜底
     * (绝对/协议相对/根相对/路径相对, 含 ".." 消化); 再不行返回 null, 退化为切集时
     * 现过滤(只损失预取收益)。
     */
    public static String absolutize(String ref, String base) {
        if (ref == null || ref.isEmpty() || base == null || base.isEmpty()) return null;
        if (ref.startsWith("http://") || ref.startsWith("https://")) return ref;
        try {
            return new java.net.URI(base).resolve(new java.net.URI(ref)).toString();
        } catch (Exception ignore) {
            // fall through → 手工兜底
        }
        if (ref.startsWith("//")) {
            int schemeEnd = base.indexOf("://");
            return schemeEnd > 0 ? base.substring(0, schemeEnd) + ":" + ref : null;
        }
        int schemeEnd = base.indexOf("://");
        if (schemeEnd <= 0) return null;
        int pathStart = schemeEnd + 3;
        if (ref.startsWith("/")) {
            int pathSlash = base.indexOf('/', pathStart);
            return (pathSlash > 0 ? base.substring(0, pathSlash) : base) + ref;
        }
        // 路径相对: 以 base 去掉 query/fragment 后的"目录"为基准, 逐段消化 "." / ".."
        int q = base.indexOf('?', pathStart);
        String basePath = q > 0 ? base.substring(0, q) : base;
        int lastSlash = basePath.lastIndexOf('/');
        String dir = lastSlash >= pathStart ? basePath.substring(0, lastSlash + 1) : basePath + "/";
        java.util.Deque<String> segs = new java.util.ArrayDeque<>();
        for (String seg : ref.split("/")) {
            if (seg.isEmpty() || seg.equals(".")) continue;
            if (seg.equals("..")) {
                if (!segs.isEmpty()) segs.pollLast();
                continue;
            }
            segs.addLast(seg);
        }
        StringBuilder sb = new StringBuilder(dir);
        for (String seg : segs) {
            if (sb.charAt(sb.length() - 1) != '/') sb.append('/');
            sb.append(seg);
        }
        return sb.toString();
    }

    /**
     * 从 HLS master 文本里提取**子清单** URL — master 表的各码率子表, 供端侧预取下钻一层。
     * 文本可以是原始 master(相对地址)或已绝对化的过滤输出; 相对地址由调用方
     * {@link #absolutize} 解析。
     *
     * 为什么必须下钻: 端侧路径里 master 层已免 POST(见 {@link #hasMediaSegments}),
     * 子表(媒体列表)才是唯一要过滤的一层 —— 预取器不把子表一起预热,
     * 切集就仍要付一次"上行清单 + 服务器往返"(实测单次 1.4~5.1s)。
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
}
