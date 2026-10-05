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
     * 相对/绝对分片 URL 解析: 绝对 URL(http/https/file)原样; 相对路径按 base 的目录部分拼接.
     * 不依赖 android.net.Uri.resolve(本地单测无 android 运行时, 手工拼更可控).
     */
    private static String resolveUrl(String base, String url) {
        String u = url.trim();
        if (u.startsWith("http://") || u.startsWith("https://") || u.startsWith("file://")) {
            return u;
        }
        if (base == null || base.isEmpty()) return u;
        int slash = base.lastIndexOf('/');
        String dir = slash >= 0 ? base.substring(0, slash + 1) : base + "/";
        return dir + u;
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
