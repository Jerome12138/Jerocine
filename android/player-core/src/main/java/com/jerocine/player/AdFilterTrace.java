package com.jerocine.player;

/**
 * 端侧广告过滤链路的埋点格式 — 纯字符串处理, 不依赖 Android API, 便于单测.
 *
 * <p>为什么单独抽一个类: 过滤链路(抓清单 → 本地 M3u8FilterCore 过滤 → 交解析器)此前**零日志**,
 * 起播慢只能靠两侧 curl 反推 —— 回答不了"这一次尝试花了多久、是第几次成的、失败原因是什么"。
 * 需要埋点的出口有三处且分散:
 * <ul>
 *   <li>{@link PlayerAdFilterHelper.FilterPlaylistParser} — 播放路径: 解析器层的路径决策(预取命中/端侧过滤);</li>
 *   <li>{@link PlayerPrefetchHelper} — 预取链路: 后台抓取与过滤(独立实现, 不共用播放路径);</li>
 * </ul>
 * 三处共用同一份格式与 {@link #TAG}, 一次 {@code adb logcat | grep JcAdFilter} 就能拼出完整时间线。
 *
 * <p>字段固定为 {@code k=v} 且顺序稳定(attempt/ms/src/http/out/ads/result), 便于
 * {@code grep -o "ms=[0-9]*"} 这类命令直接取数做分布统计。源地址一律经 {@link #srcTag} 脱敏 ——
 * 清单 URL 常带 token/签名, 原样打日志等于把凭据写进 logcat。
 */
public final class AdFilterTrace {

    /** 过滤链路统一日志 tag: 三处调用点都引用这里, 避免各处硬编码后漂移。 */
    public static final String TAG = "JcAdFilter";

    /** src 标识最大长度 — 只用于人眼区分"哪张清单", 不需要完整 URL。 */
    private static final int MAX_SRC_TAG = 48;
    /** 超长时保留的头部长度(host 在这里)与尾部长度(文件名在这里, 丢了就分不出 master/子表)。 */
    private static final int SRC_HEAD = MAX_SRC_TAG - 12;
    private static final int SRC_TAIL = 10;

    private AdFilterTrace() {
    }

    /**
     * 源地址脱敏 — 只保留 host + 末段文件名, 去掉 scheme、目录与 query。
     *
     * <p>清单 URL 形如 {@code https://v.xxx.com/2026/10442/2000k/hls/mixed.m3u8?token=...&expire=...},
     * 目录段与 query 都不携带排障信息, 却可能含签名凭据, 故不落盘。host + 末段已足够区分
     * master 表 / 子表 / 不同剧集。
     */
    public static String srcTag(String srcUrl) {
        if (srcUrl == null || srcUrl.isEmpty()) return "-";
        String s = srcUrl;
        int cut = indexOfAny(s, '?', '#');
        if (cut >= 0) s = s.substring(0, cut);
        int scheme = s.indexOf("://");
        String rest = (scheme >= 0) ? s.substring(scheme + 3) : s;
        int slash = rest.indexOf('/');
        if (slash < 0) return clip(rest);
        String host = rest.substring(0, slash);
        String path = rest.substring(slash + 1);
        int lastSep = path.lastIndexOf('/');
        String leaf = (lastSep >= 0) ? path.substring(lastSep + 1) : path;
        return clip(leaf.isEmpty() ? host : host + "/" + leaf);
    }

    /** 一次过滤的开始行(记录上行体积) — 与 {@link #end} 成对。 */
    public static String begin(String srcTag, int rawBytes) {
        return "begin src=" + srcTag + " in=" + rawBytes;
    }

    /**
     * 单次尝试的结果行 — 排障主用: 第几次尝试、这次花了多久、HTTP 多少、剔除几段、成没成。
     *
     * @param attempt   第几次尝试(从 1 起)
     * @param elapsedMs 本次尝试耗时(含连接/TLS/上行/下行)
     * @param httpCode  响应码; 未拿到响应(连接失败/超时/取消)传 -1 → 该字段省略
     * @param cause     失败原因描述; 成功传 null → 输出 result=ok
     * @param outBytes  过滤后体积; 失败传 -1 → 该字段省略
     * @param ads       本层剔除段数(master 恒 0); 失败传 -1 → 该字段省略
     * @param srcTag    见 {@link #srcTag}
     */
    public static String attempt(int attempt, long elapsedMs, int httpCode, String cause,
                                 int outBytes, int ads, String srcTag) {
        StringBuilder sb = new StringBuilder(112);
        sb.append("attempt=").append(attempt)
                .append(" ms=").append(elapsedMs)
                .append(" src=").append(srcTag);
        if (httpCode > 0) sb.append(" http=").append(httpCode);
        if (outBytes >= 0) sb.append(" out=").append(outBytes);
        if (ads >= 0) sb.append(" ads=").append(ads);
        appendResult(sb, cause);
        return sb.toString();
    }

    /**
     * 一次过滤的收尾行 — 与 {@link #begin} 成对。
     *
     * <p>关键信息是 {@code attempts}: 1 = 第一次就成了; 2 = 重试过一次(时间翻倍)。
     * {@code totalMs} 是整次过滤(含 attempt 间的退避 sleep)的总耗时。
     */
    public static String end(String srcTag, int attempts, long totalMs, String cause,
                             int outBytes, int ads) {
        StringBuilder sb = new StringBuilder(112);
        sb.append("end src=").append(srcTag)
                .append(" attempts=").append(attempts)
                .append(" totalMs=").append(totalMs);
        if (outBytes >= 0) sb.append(" out=").append(outBytes);
        if (ads >= 0) sb.append(" ads=").append(ads);
        appendResult(sb, cause);
        return sb.toString();
    }

    /**
     * 路径决策行 — 记录"这一层清单走了哪条路"(预取命中/端侧过滤)。
     *
     * <p>用来解释一次起播为什么快或慢: 命中预取应≈0ms; 走端侧过滤则等于一次 {@code attempt} 的耗时;

     *
     * @param phase    发生位置(parse/prefetch-fetch/prefetch-filter/escalate)
     * @param decision 决策结果
     * @param elapsedMs 该决策耗时; 纯判定(命中缓存/无需过滤)传 0
     * @param srcTag   见 {@link #srcTag}
     */
    public static String layer(String phase, String decision, long elapsedMs, String srcTag) {
        return "layer=" + phase + " decision=" + decision + " ms=" + elapsedMs + " src=" + srcTag;
    }

    private static void appendResult(StringBuilder sb, String cause) {
        if (cause == null) {
            sb.append(" result=ok");
        } else {
            sb.append(" result=fail cause=").append(cause);
        }
    }

    private static int indexOfAny(String s, char a, char b) {
        int i = s.indexOf(a);
        int j = s.indexOf(b);
        if (i < 0) return j;
        if (j < 0) return i;
        return Math.min(i, j);
    }

    private static String clip(String tag) {
        if (tag.length() <= MAX_SRC_TAG) return tag;
        return tag.substring(0, SRC_HEAD) + ".." + tag.substring(tag.length() - SRC_TAIL);
    }
}
