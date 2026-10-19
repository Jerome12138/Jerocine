package com.jerocine.player;

import java.util.concurrent.TimeUnit;

import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;

/**
 * 端侧广告过滤客户端 — 同步 POST 原始 m3u8 到 /v1/m3u8/filter 剔广告.
 *
 * <p>播放器({@link PlayerAdFilterHelper})与下载器(download 包)共用:
 * 下载任务入队前同样需要"过滤后清单"作为下载列表, 同一份客户端避免两处重复实现。
 *
 * <p><b>必须继承 {@link JerocinePlayer#mediaClient()}</b>: 壳层通过
 * {@code Config.setMediaClient} 注入的客户端可能带**放宽的 TLS 策略/连接池**(老电视 CA 不全,
 * 见 PlayerPrefetchHelper 与 PlayerActivity 的同类做法)。这里另起一个默认 Builder,
 * 会让过滤成为整条链路上**唯一握手失败的环节** —— 播放器能拉清单、能预取, 唯独每集过滤必失败,
 * 老设备上表现为"起播极慢 + 过滤总失败"而日志里看不出是 TLS 问题。
 *
 * <p>超时上限说明: 媒体表实际可达 900+ 段, 实测单张清单上行 ~25KB、过滤后下行 ~77KB,
 * 弱网设备上行更慢 → callTimeout 8s, 并重试一次(切集/入队时网络争用偶发失败)。
 * 端侧路径只有媒体列表走这里(master 层免 POST, 见 PlayerUrls.hasMediaSegments),
 * 且循环顶部检查线程中断(用户切集/退出时应尽快放弃, 而不是继续等满超时)。
 */
public final class M3u8FilterClient {

    /**
     * 单层清单响应体积上限(4MB)。实测过滤后 ~77KB, 4MB 已有 50 余倍余量。
     * 不设上限时, 源站返回一个"巨型 m3u8"(拼接错误/无限直播列表/恶意响应)会让
     * {@code body().bytes()} 直接把整个响应读进堆 → TV 端(可用堆常 128MB)OOM,
     * 且这发生在 loader 线程, 会带着整个播放进程一起死。
     */
    private static final int MAX_PLAYLIST_BYTES = 4 * 1024 * 1024;

    private static OkHttpClient buildClient() {
        OkHttpClient injected = JerocinePlayer.mediaClient();
        OkHttpClient.Builder b = (injected != null)
                ? injected.newBuilder() : new OkHttpClient.Builder();
        return b.connectTimeout(8, TimeUnit.SECONDS)
                .readTimeout(8, TimeUnit.SECONDS)
                .writeTimeout(8, TimeUnit.SECONDS)
                .callTimeout(8, TimeUnit.SECONDS)
                .retryOnConnectionFailure(true)
                .build();
    }

    /**
     * 懒加载 holder: {@link JerocinePlayer#mediaClient()} 由壳层在 Application/首屏注入,
     * 若用 static 初始化可能在注入之前就固化了默认 Builder, 继承不到壳层的 TLS 策略。
     */
    private static final class Holder {
        static final OkHttpClient CLIENT = buildClient();
    }

    private static OkHttpClient client() {
        return Holder.CLIENT;
    }

    private M3u8FilterClient() {}

    /**
     * 过滤失败原因分类 — 下载侧降级策略({@code DownloadFilterFallbackPolicy})按此分级行动;
     * 播放侧不区分(null 语义不变)。
     */
    public enum FailureCause {
        /** 无过滤服务可用(proxyBase 未配置 / raw 为空 / URL 构造失败) — 本地原因。 */
        NO_FILTER,
        /** 数据获取类失败(网络异常/超时/5xx/3xx/无 body) — 值得重试。 */
        NETWORK,
        /** 过滤服务拒绝请求(4xx, 如端点不存在/src 非法) — 重试无意义。 */
        REJECTED,
        /** 响应读失败(连接中断/流异常) — 瞬时故障, 与 NETWORK 同级可重试。 */
        BAD_RESPONSE,
        /** 响应超体积上限(&gt;4MB) — 确定性失败, 重试同样会超限。 */
        TOO_LARGE,
        /** 任务被取消(线程中断) — 既不重试也不降级。 */
        CANCELLED
    }

    /** {@link #filterDetailed} 的返回: result 与 cause 二选一非空。 */
    public static final class Outcome {
        public final Result result;
        public final FailureCause cause;

        private Outcome(Result r, FailureCause c) {
            this.result = r;
            this.cause = c;
        }

        static Outcome ok(Result r) {
            return new Outcome(r, null);
        }

        static Outcome fail(FailureCause c) {
            return new Outcome(null, c);
        }

        /** 成功与否(下载侧循环判定用)。 */
        public boolean success() {
            return result != null;
        }
    }

    /** 过滤结果: data=过滤后字节; filteredCount=本层被剔除的广告段数(master 恒 0). */
    public static final class Result {
        public final byte[] data;
        public final int filteredCount;

        Result(byte[] data, int filteredCount) {
            this.data = data;
            this.filteredCount = filteredCount;
        }
    }

    /**
     * 同步 POST 原始 m3u8 到 /v1/m3u8/filter, 返回过滤后结果.
     *
     * @param proxyBase 服务端 API base(如 https://jerocine.art/api); 空/异常 → null
     * @param srcUrl    源站 m3u8 原始 URL(用作 ?src= 参数, 服务端据此判断源站可达性)
     * @param raw       原始清单字节(application/vnd.apple.mpegurl)
     * @return 过滤后结果; 失败(重试一次后仍失败) → null(原因见 {@link #filterDetailed})
     */
    public static Result filter(String proxyBase, String srcUrl, byte[] raw) {
        return filterDetailed(proxyBase, srcUrl, raw).result;
    }

    /**
     * {@link #filter} 的分类版 — 失败时给出 {@link FailureCause}, 永不返回 null。
     * 行为与原 filter() 逐分支等价, 只是每个 null 出口都带上了原因。
     */
    public static Outcome filterDetailed(String proxyBase, String srcUrl, byte[] raw) {
        final String srcTag = AdFilterTrace.srcTag(srcUrl);
        if (proxyBase == null || proxyBase.isEmpty() || raw == null || raw.length == 0) {
            return endFail(srcTag, 0, 0L, FailureCause.NO_FILTER);
        }
        final String url;
        final long filterStartNanos = System.nanoTime();
        try {
            String base = proxyBase.endsWith("/")
                    ? proxyBase.substring(0, proxyBase.length() - 1)
                    : proxyBase;
            // 编码与 PlayerUrls.encodeParam / web 端 encodeURIComponent 一致(RFC 3986):
            // 不用 URLEncoder(空格→'+'、'*' 不编码), 否则同一集在两端产生不同的 src 串,
            // 服务端按 src 归并的过滤缓存与统计会对不上。
            url = base + "/v1/m3u8/filter?src=" + PlayerUrls.encodeParam(srcUrl);
        } catch (Exception e) {
            return endFail(srcTag, 0, msSince(filterStartNanos), FailureCause.NO_FILTER);
        }
        android.util.Log.i(AdFilterTrace.TAG, AdFilterTrace.begin(srcTag, raw.length));
        for (int attempt = 0; attempt < 2; attempt++) {
            // 用户切集/退出会中断 loader 线程(BufferPrefetcher.cancel → shutdownNow):
            // 此时应尽快放弃, 而不是继续等满一轮超时(最坏 16s×2 层= 30s+ 黑屏)。
            if (Thread.currentThread().isInterrupted()) {
                // 本次请求还没发出去: 只记整次汇总, 不记 attempt 行(免得读成"试了一次")
                return endFail(srcTag, attempt, msSince(filterStartNanos), FailureCause.CANCELLED);
            }
            final long attemptStartNanos = System.nanoTime();
            try {
                Request req = new Request.Builder().url(url)
                        .post(RequestBody.create(
                                MediaType.parse("application/vnd.apple.mpegurl"), raw))
                        .build();
                try (Response resp = client().newCall(req).execute()) {
                    // 4xx 是请求本身有问题(src 非法/ token 过期 / 源站不存在), 重试没有意义,
                    // 只会再赔一个 RTT, 且 PlayerAdFilterHelper 会紧接着 escalateToProxy
                    // 再付一次"重试+升级+重新 prepare"的代价。直接放弃。
                    if (resp.code() >= 400 && resp.code() < 500) {
                        return endAttempt(srcTag, attempt, attemptStartNanos, filterStartNanos,
                                resp.code(), FailureCause.REJECTED);
                    }
                    if (resp.isSuccessful() && resp.body() != null) {
                        byte[] out;
                        try {
                            out = readCapped(resp.body(), MAX_PLAYLIST_BYTES);
                        } catch (ResponseTooLargeException e) {
                            // 超限: 不重试(重试同样会超)
                            return endAttempt(srcTag, attempt, attemptStartNanos, filterStartNanos,
                                    resp.code(), FailureCause.TOO_LARGE);
                        } catch (java.io.IOException e) {
                            // 读失败(连接中断/流异常): 瞬时, 与 NETWORK 同待遇 —— 第 1 次失败
                            // 落下去快速重试一次, 第 2 次失败才上报(播放/下载两侧共用此客户端;
                            // 播放侧上报即触发 escalateToProxy, 360 这类 proxyUsable=false 的源
                            // 会直接原始流播放, 所以这里能自己恢复就别上报)
                            if (attempt == 0) {
                                logAttempt(srcTag, attempt, attemptStartNanos, resp.code(),
                                        FailureCause.BAD_RESPONSE);
                                out = null; // 落下去 sleep 重试
                            } else {
                                return endAttempt(srcTag, attempt, attemptStartNanos, filterStartNanos,
                                        resp.code(), FailureCause.BAD_RESPONSE);
                            }
                        }
                        if (out != null) {
                            int cnt = 0;
                            String n = resp.header("X-Ad-Filtered");
                            if (n != null) {
                                try {
                                    cnt = Integer.parseInt(n);
                                } catch (NumberFormatException ignore) {
                                }
                            }
                            return endOk(srcTag, attempt, attemptStartNanos, filterStartNanos,
                                    resp.code(), out, cnt);
                        }
                        // out == null: 仅"读失败第 1 轮"会走到这里 → 落下去 sleep 重试
                    }
                    // 5xx / 3xx / 无 body → 值得重试一次
                    if (attempt == 1) {
                        return endAttempt(srcTag, attempt, attemptStartNanos, filterStartNanos,
                                resp.code(), FailureCause.NETWORK);
                    }
                    logAttempt(srcTag, attempt, attemptStartNanos, resp.code(), FailureCause.NETWORK);
                }
            } catch (Exception e) {
                // 必须重设中断位: OkHttp 的 execute() 在线程被 shutdownNow 中断时抛的异常
                // 类型不一定是 InterruptedException, 若不在这里 interrupt(), 中断状态会丢,
                // 上层(cancel → 已取消)就再也判断不出"这是取消不是失败"。
                // 注意 InterruptedException 在 java.lang(默认导入), 不是 java.io。
                if (e instanceof InterruptedException) {
                    Thread.currentThread().interrupt();
                    return endAttempt(srcTag, attempt, attemptStartNanos, filterStartNanos,
                            0, FailureCause.CANCELLED);
                }
                if (attempt == 1) {
                    return endAttempt(srcTag, attempt, attemptStartNanos, filterStartNanos,
                            0, FailureCause.NETWORK);
                }
                logAttempt(srcTag, attempt, attemptStartNanos, 0, FailureCause.NETWORK);
            }
            try {
                Thread.sleep(250);
            } catch (InterruptedException ie) {
                Thread.currentThread().interrupt();
                return endFail(srcTag, attempt + 1, msSince(filterStartNanos), FailureCause.CANCELLED);
            }
        }
        return endFail(srcTag, 2, msSince(filterStartNanos), FailureCause.NETWORK);
    }

    // ============================ 埋点 ============================

    /** 记单次 attempt 的结果行(非终局: 该轮失败但还要再试一次)。 */
    private static void logAttempt(String srcTag, int attempt, long attemptStartNanos, int httpCode,
                                   FailureCause cause) {
        android.util.Log.i(AdFilterTrace.TAG, AdFilterTrace.attempt(
                attempt + 1, msSince(attemptStartNanos), httpCode, cause.name(), -1, -1, srcTag));
    }

    /** 单次 attempt 终局失败: 记 attempt 行 + 整次收尾行, 返回带原因的结果。 */
    private static Outcome endAttempt(String srcTag, int attempt, long attemptStartNanos,
                                      long filterStartNanos, int httpCode, FailureCause cause) {
        logAttempt(srcTag, attempt, attemptStartNanos, httpCode, cause);
        return endFail(srcTag, attempt + 1, msSince(filterStartNanos), cause);
    }

    /** 单次 attempt 成功(第 attempt+1 次成的): 记 attempt 行 + 整次收尾行。 */
    private static Outcome endOk(String srcTag, int attempt, long attemptStartNanos, long filterStartNanos,
                                 int httpCode, byte[] out, int ads) {
        android.util.Log.i(AdFilterTrace.TAG, AdFilterTrace.attempt(
                attempt + 1, msSince(attemptStartNanos), httpCode, null, out.length, ads, srcTag));
        android.util.Log.i(AdFilterTrace.TAG, AdFilterTrace.end(
                srcTag, attempt + 1, msSince(filterStartNanos), null, out.length, ads));
        return Outcome.ok(new Result(out, ads));
    }

    /** 整次过滤结束(失败): 记收尾行, 返回带原因的结果。 */
    private static Outcome endFail(String srcTag, int attempts, long totalMs, FailureCause cause) {
        android.util.Log.i(AdFilterTrace.TAG,
                AdFilterTrace.end(srcTag, attempts, totalMs, cause.name(), -1, -1));
        return Outcome.fail(cause);
    }

    private static long msSince(long startNanos) {
        return (System.nanoTime() - startNanos) / 1_000_000L;
    }

    /**
     * 读响应体并对体积设上限 — 超限抛 {@link ResponseTooLargeException}(确定性失败, 重试同样超限),
     * 读失败按普通 {@link java.io.IOException} 上抛(瞬时, 调用方按可重试处理)。
     *
     * <p>{@code body.bytes()} 会把整个响应读进堆, 没有上限时源站返回一个"巨型 m3u8"
     * 就能让 TV 端 OOM。
     */
    private static final class ResponseTooLargeException extends java.io.IOException {
    }

    private static byte[] readCapped(okhttp3.ResponseBody body, int cap) throws java.io.IOException {
        if (body.contentLength() > cap) throw new ResponseTooLargeException();
        java.io.InputStream in = body.byteStream();
        java.io.ByteArrayOutputStream bos = new java.io.ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n, total = 0;
        while ((n = in.read(buf)) != -1) {
            total += n;
            if (total > cap) throw new ResponseTooLargeException();
            bos.write(buf, 0, n);
        }
        return bos.toByteArray();
    }
}
