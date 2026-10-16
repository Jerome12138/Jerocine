package com.jerocine.player;

/**
 * 播放错误诊断信息 — 用于把 URL 呈现给用户/宿主时**只保留可诊断的部分, 去掉签名**。
 *
 * <p><b>为什么必须脱敏</b>: 源站 URL 常带时效签名({@code ?token=xxx&sign=yyy&exp=...})。
 * 而代理 URL 的尾部恰好就是 {@code src=<编码后的源站 URL>}, 于是
 * {@code currentUrl.substring(尾部 60 字符)} 这类写法会把完整签名**显示在电视屏幕上**
 * (截图/录屏/直播时扩散), 下载侧还会把它持久化进 {@code DownloadTask.error} 并
 * 显示在下载列表的错误列里。
 *
 * <p>判定为签名参数(会被整段替换为 ***):
 * {@code token} {@code sign} {@code signature} {@code auth} {@code authorization}
 * {@code key} {@code secret} {@code password} {@code passwd} {@code session}
 * {@code sessionid} {@code uid} {@code userid} {@code expires} {@code exp}
 * {@code policy} {@code hmac} {@code ecode} {@code jwt} {@code access_token}
 *
 * <p>保留 {@code url}/{@code v}/{@code type} 等结构参数 —— 诊断时真正有用的是
 * "哪个源的哪一集", 而不是签名本身。
 */
public final class ErrorDiag {

    private ErrorDiag() {
    }

    private static final String[] SIGNING_KEYS = {
            "token", "sign", "signature", "auth", "authorization", "key", "secret",
            "password", "passwd", "session", "sessionid", "uid", "userid",
            "expires", "exp", "policy", "hmac", "ecode", "jwt", "access_token",
    };

    /** 整个查询串里含签名参数时, 只显示到第一个签名参数出现为止, 并追加统一标记。 */
    private static final String REDACTED = "?***";

    /**
     * 把 URL 脱敏成可安全显示的诊断串: 保留 scheme://host/path, 查询串只留非签名参数。
     *
     * <p>无法解析时(非法 URL)退化为"不输出路径与查询, 只输出长度" —— 宁可信息少,
     * 也不能泄漏签名。
     */
    public static String safeUrl(String url) {
        if (url == null || url.isEmpty()) return "(空)";
        try {
            android.net.Uri u = android.net.Uri.parse(url);
            String scheme = u.getScheme();
            String host = u.getHost();
            String path = u.getPath();
            StringBuilder sb = new StringBuilder();
            if (scheme != null && !scheme.isEmpty()) sb.append(scheme).append("://");
            if (host != null && !host.isEmpty()) sb.append(host);
            // path 里也可能嵌着签名(部分源把token 放路径), 但 path 通常是诊断必需的,
            // 只保留末两段, 避免把整条带签名的子清单地址暴露出去。
            if (path != null && !path.isEmpty()) {
                String[] segs = path.split("/");
                if (segs.length <= 3) {
                    sb.append(path);
                } else {
                    sb.append("/.../").append(segs[segs.length - 2]).append('/')
                            .append(segs[segs.length - 1]);
                }
            }
            sb.append(safeQuery(u.getQuery()));
            return sb.toString();
        } catch (Exception e) {
            return "(无法解析的地址, 长度 " + url.length() + ")";
        }
    }

    /** 查询串脱敏: 签名参数整段替换, 其余保留。 */
private static String safeQuery(String query) {
        if (query == null || query.isEmpty()) return "";
        String[] pairs = query.split("&");
        StringBuilder sb = new StringBuilder();
        boolean redacted = false;
        for (String p : pairs) {
            if (p.isEmpty()) continue;
            int eq = p.indexOf('=');
            String k = (eq >= 0) ? p.substring(0, eq) : p;
            if (isSigningKey(k)) {
                redacted = true;
                continue;
            }
            // **最关键的一条**: 代理 URL 的 src= 参数是**二次编码**的源站 URL
            // (src=https%3A%2F%2Fcdn%2Fa.m3u8%3Ftoken%3DSECRET)。编码态下
            // "token%3DSECRET" 不含 "token=" 的形态, 只判参数名会整条漏掉 —— 而这恰好
            // 是最该脱敏的那一种(它就是源站的真实签名)。所以还要对**值**判定。
            if (eq >= 0 && valueLooksSecret(p.substring(eq + 1))) {
                redacted = true;
                continue;
            }
            if (sb.length() > 0) sb.append('&');
            sb.append(p);
        }
        if (!redacted) return "?" + sb;
        return sb.length() > 0 ? "?" + sb + REDACTED : REDACTED;
    }

    /**
     * 值本身是否像签名 — 先 URL 解码(处理二次编码), 再看是否含签名参数名。
     *
     * <p>解码失败时按原串判(不因此泄漏: 判不出就当普通参数, 但签名参数名通常仍能命中)。
     */
    private static boolean valueLooksSecret(String value) {
        if (value == null || value.isEmpty()) return false;
        String decoded = value;
        try {
            decoded = java.net.URLDecoder.decode(value, "UTF-8");
        } catch (Exception ignore) {
        }
        String lower = decoded.toLowerCase(java.util.Locale.US);
        for (String s : SIGNING_KEYS) {
            if (lower.contains(s + "=") || lower.contains(s + ":") || lower.contains("&" + s)) {
                return true;
            }
        }
        return false;
    }

    private static boolean isSigningKey(String key) {
        String k = key.toLowerCase(java.util.Locale.US);
        for (String s : SIGNING_KEYS) {
            if (k.equals(s) || k.contains(s)) return true;
        }
        return false;
    }
}