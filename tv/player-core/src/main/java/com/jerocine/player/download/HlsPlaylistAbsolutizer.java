package com.jerocine.player.download;

/**
 * 整份 m3u8 清单的相对 URL 绝对化 — <b>原始流兜底下载专用</b>。纯 Java, JVM 可测。
 *
 * <p>服务端 FilterText 在剔广告的同时会把分片 URL 绝对化; 原始流兜底路径
 * ({@link DownloadFilterFallbackPolicy} USE_RAW)拿不到这份服务, 而 playlist.m3u8 落盘后由
 * file:// 播放 —— 相对分片会按 file:// 目录解析, 直接播不出来。因此原始流媒体级清单
 * 落盘前必须按其真实来源 URL 绝对化(语义对齐 media3 的 RFC 3986 解析,
 * 保证下载缓存 key 与播放期解析出的 URL 一致, 分片才能命中本地缓存)。
 *
 * <p>处理规则:
 * <ul>
 *   <li>标签行(# 开头): 只重写其中 {@code URI="..."} 属性(EXT-X-KEY / EXT-X-MAP /
 *       EXT-X-I-FRAME-STREAM-INF 等可能携带相对 URI), 其余文字原样保留;</li>
 *   <li>非标签非空行 = 分片/variant URL → {@link PlaylistSegments#resolveUrl} 绝对化
 *       (绝对 URL 原样保留, 与下载/播放既有的 URL 归并语义同一份实现);</li>
 *   <li>空行与换行结构原样保留。</li>
 * </ul>
 */
public final class HlsPlaylistAbsolutizer {

    private HlsPlaylistAbsolutizer() {
    }

    /** 清单文本按 baseUrl 绝对化; 任一入参为空 → 原样返回。 */
    public static String absolutize(String playlistText, String baseUrl) {
        if (playlistText == null || playlistText.isEmpty()
                || baseUrl == null || baseUrl.isEmpty()) {
            return playlistText;
        }
        String[] lines = playlistText.split("\\r?\\n");
        StringBuilder out = new StringBuilder(playlistText.length() + 256);
        for (int i = 0; i < lines.length; i++) {
            String line = lines[i];
            if (i > 0) out.append('\n');
            if (line.isEmpty()) continue;
            if (line.charAt(0) == '#') {
                out.append(rewriteUriAttrs(line, baseUrl));
            } else {
                // 分片/variant 行: resolveUrl 对绝对 URL 原样返回, 相对 URL 按 base 归并
                out.append(PlaylistSegments.resolveUrl(baseUrl, line));
            }
        }
        return out.toString();
    }

    /** 匹配标签行中的 URI="..." 属性(EXT-X-KEY/EXT-X-MAP/I-FRAME 等)。 */
    private static final java.util.regex.Pattern URI_ATTR =
            java.util.regex.Pattern.compile("(URI=\")([^\"]*)(\")");

    /** 重写标签行内所有 URI="..." 属性为绝对 URL(绝对输入原样保留 — 幂等)。 */
    private static String rewriteUriAttrs(String line, String baseUrl) {
        java.util.regex.Matcher m = URI_ATTR.matcher(line);
        if (!m.find()) return line;
        m.reset();
        StringBuffer sb = new StringBuffer(line.length() + 64);
        while (m.find()) {
            String resolved = PlaylistSegments.resolveUrl(baseUrl, m.group(2));
            // quoteReplacement: 分片 URL 常含 $ 字面量(签名串), 直接拼会被当组引用
            m.appendReplacement(sb, java.util.regex.Matcher.quoteReplacement(
                    m.group(1) + resolved + m.group(3)));
        }
        m.appendTail(sb);
        return sb.toString();
    }
}
