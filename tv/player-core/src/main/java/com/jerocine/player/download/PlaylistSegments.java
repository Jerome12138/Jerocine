package com.jerocine.player.download;

import java.util.ArrayList;
import java.util.List;

/**
 * 过滤后 m3u8 清单解析 — 纯 Java 逻辑(不依赖网络/Android API), 可直接单测.
 *
 * <p>用于: ① {@link TsExporter} 导出 .ts(按分片序从下载缓存拼接); ② {@link BufferPrefetcher}
 * 缓存缓冲(按 EXTINF 累计时长选前 N 分钟分片)。
 *
 * <p>分片 URL 以 String 表达(纯 Java 可测); 调用方需要 {@code Uri} 时自行 parse。
 *
 * <p>只支持常规 EXTINF 分片清单(源站均为该形态); 出现 {@code #EXT-X-BYTERANGE} / KEY 分片等
 * 特殊形态时诚实跳过该片(不解析), 不影响其余分片。
 */
public final class PlaylistSegments {

    /** 一个分片: 绝对 URL(相对路径已按 base 拼接) + 时长(秒). */
    public static final class Segment {
        public final String url;
        public final double durationSeconds;

        Segment(String url, double durationSeconds) {
            this.url = url;
            this.durationSeconds = durationSeconds;
        }
    }

    private PlaylistSegments() {
    }

    /**
     * 解析 m3u8: 收集 {@code #EXTINF:<dur>} 后紧跟的分片 URL 行.
     * baseUrl 用于拼接相对路径(取源站 m3u8 URL, 与 Media3 缓存键一致).
     */
    public static List<Segment> parse(String playlist, String baseUrl) {
        List<Segment> out = new ArrayList<>();
        if (playlist == null || playlist.isEmpty()) return out;
        String[] lines = playlist.split("\\r?\\n");
        double pendingDuration = -1.0;
        for (String raw : lines) {
            String line = raw.trim();
            if (line.isEmpty()) continue;
            if (line.startsWith("#EXTINF:")) {
                // #EXTINF:10.000,  → 取冒号后逗号前的时长
                String rest = line.substring("#EXTINF:".length());
                int comma = rest.indexOf(',');
                if (comma >= 0) rest = rest.substring(0, comma);
                try {
                    pendingDuration = Double.parseDouble(rest.trim());
                } catch (NumberFormatException ignore) {
                    pendingDuration = -1.0;
                }
                continue;
            }
            if (line.startsWith("#EXT-X-BYTERANGE:")) {
                // BYTERANGE 分片复用前一个 URL 且带 range, 本实现不支持 → 跳过该片(诚实降级)
                pendingDuration = -1.0;
                continue;
            }
            if (line.startsWith("#")) continue; // 其它标签
            // 分片 URL 行
            out.add(new Segment(resolveUrl(baseUrl, line), pendingDuration > 0 ? pendingDuration : 0.0));
            pendingDuration = -1.0;
        }
        return out;
    }

    /**
     * 相对/绝对分片 URL 解析 — 与 media3 {@code Uri.resolve} 语义对齐(RFC 3986):
     * 绝对 URL(http/https/file)原样; 以 {@code /} 开头 → 替换为 base 的 scheme://authority;
     * 其余相对路径 → base 目录拼接, 并归一化 {@code ./} 与 {@code ../} 段。
     * 不依赖 android.net.Uri.resolve(本地单测无 android 运行时, 手工拼更可控且可测)。
     */
    private static String resolveUrl(String base, String url) {
        String u = url.trim();
        if (u.startsWith("http://") || u.startsWith("https://") || u.startsWith("file://")) {
            return u;
        }
        if (base == null || base.isEmpty()) return u;
        // 以 / 开头的路径 → 替换 base 的 scheme://authority 部分(与 Uri.resolve 一致)
        if (u.startsWith("/")) {
            int schemeEnd = base.indexOf("://");
            if (schemeEnd > 0) {
                int authEnd = base.indexOf('/', schemeEnd + 3);
                String authority = authEnd > 0 ? base.substring(0, authEnd) : base;
                return authority + u;
            }
            return u;
        }
        // 相对路径 → base 目录 + u, 归一化点段
        int slash = base.lastIndexOf('/');
        String dir = slash >= 0 ? base.substring(0, slash + 1) : base + "/";
        return normalizeDots(dir + u);
    }

    /** 归一化 ./ 与 ../ 路径段(仅路径部分, query/fragment 原样保留) — 对齐 Uri.resolve. */
    private static String normalizeDots(String url) {
        int q = url.indexOf('?');
        int f = url.indexOf('#');
        int cut = url.length();
        if (q >= 0) cut = Math.min(cut, q);
        if (f >= 0) cut = Math.min(cut, f);
        String path = url.substring(0, cut);
        String suffix = url.substring(cut);
        int scheme = path.indexOf("://");
        if (scheme < 0) return url;
        int pathStart = path.indexOf('/', scheme + 3);
        if (pathStart < 0) return url; // 无路径, 无需归一化
        String head = path.substring(0, pathStart); // scheme://authority
        String[] segs = path.substring(pathStart).split("/");
        java.util.ArrayDeque<String> stack = new java.util.ArrayDeque<>();
        for (String s : segs) {
            if (s.isEmpty() || s.equals(".")) continue;
            if (s.equals("..")) {
                if (!stack.isEmpty()) stack.pollLast(); // 越出根时丢弃(与 Uri.resolve 一致)
                continue;
            }
            stack.addLast(s);
        }
        StringBuilder sb = new StringBuilder(head);
        for (String s : stack) sb.append('/').append(s);
        return sb.toString() + suffix;
    }

    /**
     * 取前 {@code minutes} 分钟对应的分片下标范围 [0, end): 按 EXTINF 累计时长,
     * 不足整片时包含下一片(保证覆盖 N 分钟); 返回 0 表示无分片可取.
     */
    public static int rangeForMinutes(List<Segment> segments, double minutes) {
        if (segments == null || segments.isEmpty() || minutes <= 0) return 0;
        double acc = 0.0;
        for (int i = 0; i < segments.size(); i++) {
            acc += segments.get(i).durationSeconds;
            if (acc >= minutes * 60.0) {
                return Math.min(i + 1, segments.size());
            }
        }
        return segments.size();
    }
}
