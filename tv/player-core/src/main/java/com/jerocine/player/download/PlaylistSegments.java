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
 * <p><b>能力边界</b>(重要): 导出只支持<b>明文 TS 分片</b>清单。
 * 以下形态无法通过"从下载缓存按序拼接分片字节"得到可播放文件, 必须由 {@link #inspect}检出
 * 并显式报错, <b>不能静默产出坏文件</b>:
 * <ul>
 *   <li>{@code #EXT-X-KEY}(METHOD != NONE)—— AES-128 加密流: media3 {@code HlsDownloader}
 *       下载时<b>从不解密</b>(key 只被当作普通资源下载进缓存), 缓存里是<b>密文</b>,
 *       拼出的 .ts 任何播放器都解不出来;</li>
 *   <li>{@code #EXT-X-MAP}—— fMP4: 媒体分片是裸 fragment(非 MPEG-TS), 且 init 分片
 *       (ftyp+moov) 根本不在分片列表里, 拼出来缺 moov 结构不完整;</li>
 *   <li>{@code #EXT-X-BYTERANGE}—— 分片是同一文件的字节区间, 按完整分片拼接会缺字节;</li>
 *   <li>无 {@code #EXT-X-ENDLIST}—— 直播流, 拼出的文件是被截断的录像。</li>
 * </ul>
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

    /**
     * 清单能力检查结果 — {@link #inspect} 的产物。
     *
     * <p>{@link #exportBlockReason} 非空即表示"不能按序拼接导出", 值是可直接展示给用户的中文原因。
     */
    public static final class Capabilities {
        /** 加密方法(METHOD=NONE / AES-128 / SAMPLE-AES 等); 清单无 KEY 标签时为 null. */
        public final String encryptionMethod;
        /** fMP4 init 分片 URI; 无 EXT-X-MAP 时为 null. */
        public final String initUri;
        /** 是否出现 EXT-X-BYTERANGE. */
        public final boolean hasByteRange;
        /** 是否以 EXT-X-ENDLIST 结尾(false → 直播流, 导出必然不完整). */
        public final boolean hasEndList;
        /** 首行是否是 #EXTM3U(用于识别"服务端返回了 HTML 错误页"这类垃圾响应). */
        public final boolean looksLikePlaylist;

        Capabilities(String encryptionMethod, String initUri, boolean hasByteRange,
                     boolean hasEndList, boolean looksLikePlaylist) {
            this.encryptionMethod = encryptionMethod;
            this.initUri = initUri;
            this.hasByteRange = hasByteRange;
            this.hasEndList = hasEndList;
            this.looksLikePlaylist = looksLikePlaylist;
        }

        /**
         * 返回不可导出的原因; 可导出(明文 TS 完整清单)时返回 null。
         *
         * <p>顺序按"用户最可能遇到"排: 加密 → fMP4 → BYTERANGE → 直播 → 非清单。
         */
        public String exportBlockReason() {
            if (encryptionMethod != null && !"NONE".equalsIgnoreCase(encryptionMethod)) {
                return "该片源是加密流(" + encryptionMethod + "), 无法导出为可播放文件, 请在线播放";
            }
            if (initUri != null) {
                return "该片源是 fMP4 格式(EXT-X-MAP), 无法导出为 .ts, 请在线播放";
            }
            if (hasByteRange) {
                return "该片源使用字节区间分片(EXT-X-BYTERANGE), 无法导出, 请在线播放";
            }
            if (!looksLikePlaylist) {
                return "清单格式异常(不是有效的 m3u8), 可能是源站返回了错误页面, 请重试";
            }
            if (!hasEndList) {
                return "该片源是直播流(无 EXT-X-ENDLIST), 导出的文件不完整, 请在线播放或等待完播后再试";
            }
            return null;
        }
    }

    private PlaylistSegments() {
    }

    /**
     * 检查清单能力(不下载、不拼接, 纯字符串扫描) — 导出前必须先问一次。
     *
     * <p>存在的意义: 让"导出得到一个打不开的文件"变成"明确告诉用户为什么不能导出"。
     * 片源不固定时尤其重要 —— 同一个导出按钮, 不同片源能力不同。
     */
    public static Capabilities inspect(String playlist) {
        String encryptionMethod = null;
        String initUri = null;
        boolean hasByteRange = false;
        boolean hasEndList = false;
        boolean looksLikePlaylist = false;

        if (playlist != null && !playlist.isEmpty()) {
            for (String raw : playlist.split("\\r?\\n")) {
                String line = stripBom(raw.trim());
                if (line.isEmpty()) continue;
                if (!looksLikePlaylist) {
                    looksLikePlaylist = line.startsWith("#EXTM3U");
                }
                if (line.startsWith("#EXT-X-KEY:")) {
                    String m = attribute(line, "METHOD=");
                    if (m != null && encryptionMethod == null) encryptionMethod = m;
                } else if (line.startsWith("#EXT-X-MAP:")) {
                    String u = attribute(line, "URI=\"");
                    if (u != null && initUri == null) initUri = u;
                } else if (line.startsWith("#EXT-X-BYTERANGE:")) {
                    hasByteRange = true;
                } else if (line.startsWith("#EXT-X-ENDLIST")) {
                    hasEndList = true;
                }
            }
        }
        return new Capabilities(encryptionMethod, initUri, hasByteRange, hasEndList, looksLikePlaylist);
    }

    /** 读 m3u8 属性值: key 形如 {@code METHOD=} 或 {@code URI="} ; 无值返回 null. */
    private static String attribute(String line, String key) {
        int i = line.indexOf(key);
        if (i < 0) return null;
        String v = line.substring(i + key.length());
        String out;
        if (key.endsWith("\"")) {
            int end = v.indexOf('"');
            return end > 0 ? v.substring(0, end) : null;
        }
        // 无引号值: 到下一个逗号为止
        int comma = v.indexOf(',');
        out = (comma >= 0 ? v.substring(0, comma) : v).trim();
        // HLS 允许属性值带引号: METHOD="AES-128"(RFC 8216 §4.3.4.2)。
        // 不剥掉会让 equalsIgnoreCase("NONE") 判断失效 → 明文流被误判为加密而拦下,
        // 且用户看到的加密方式是带引号的怪字符串。
        if (out.length() >= 2 && out.charAt(0) == '"' && out.charAt(out.length() - 1) == '"') {
            out = out.substring(1, out.length() - 1);
        }
        return out;
    }

    /**
     * 剥 UTF-8 BOM(U+FEFF)。
     *
     * <p>{@code String.trim()} 只移除 &lt;= U+0020, <b>不处理</b> U+FEFF。
     * 带 BOM 的 m3u8 首行 {@code #EXTM3U} 会因首字符是 U+FEFF 而既不匹配任何标签、
     * 又不以 {@code #} 开头 → 被当成一个分片 URL 收入, 后续导出/预取必然失败在
     * "不存在的地址"上, 报错信息完全误导。
     */
    private static String stripBom(String s) {
        return (!s.isEmpty() && s.charAt(0) == '\uFEFF') ? s.substring(1) : s;
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
            // BOM 必须剥: trim() 不处理 U+FEFF, 会把首行 #EXTM3U 当成分片 URL
            String line = stripBom(raw.trim());
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
        // 目录 = base 最后一个 '/' 之前(含该斜杠)。注意 lastIndexOf('/') 不能无条件用:
        // base="https://cdn.example.com"(无尾斜杠、无路径)时它会命中 "https://" 里的第二个斜杠,
        // dir="https://" → 拼出 "https://seg.ts" 这种畸形 URL。
        // 故先定位主机名终点(scheme 之后第一个 '/'); 它不存在说明 base 无路径,
        // 它存在时 lastIndexOf 必然落在路径内, 可安全使用。
        int schemeEnd = base.indexOf("://");
        int authorityEnd = base.indexOf('/', schemeEnd > 0 ? schemeEnd + 3 : 0);
        if (authorityEnd < 0) {
            // base 落在主机根(无路径) → 分片直接挂在根下
            return normalizeDots(base + "/" + u);
        }
        int lastSlash = base.lastIndexOf('/');
        if (lastSlash < authorityEnd) lastSlash = authorityEnd; // base 以 '/' 结尾
        return normalizeDots(base.substring(0, lastSlash + 1) + u);
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
     * 不足整片时包含下一片(保证覆盖 N 分钟); 返回 0 表示无分片可取。
     *
     * <p><b>注意</b>: 分片时长全为 0 时无法按分钟估算, 此时返回全集 —— 调用方
     * (BufferPrefetcher)必须在调用前自行校验时长已知, 否则"缓冲 N 分钟"会退化成
     * 缓冲整集。这里的封顶只是最后一道防线, 不替代调用方校验。
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
        // 时长全 0: 累加永不达标 → 原本返回全集(等于缓冲整集)。此处封顶到 MINUTES_FALLBACK_CAP 片,
        // 让"时长未知"不会被放大成"下载全部分片"。
        if (acc <= 0) {
            return Math.min(segments.size(), MINUTES_FALLBACK_CAP);
        }
        return segments.size();
    }

    /**
     * 时长未知时"缓冲 N 分钟"的兜底分片上限(约一集常规分片量级, 不会放大成全集)。
     *
     * <p>公开可见是为了让单测能引用它构造"远超上限"的输入 —— 否则用例里写死一个魔法数,
     * 常量将来调大就会变成静默的假通过。
     */
    public static final int MINUTES_FALLBACK_CAP = 60;
}
