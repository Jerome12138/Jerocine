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
 * 服务端纯过滤耗时 3s+, 弱网设备上行更慢 → callTimeout 8s, 并重试一次(切集/入队时网络争用偶发失败)。
 * 端侧路径要对 master + 子表各过滤一次, 最坏 ~32s 阻塞 loader 线程, 故 4xx 不重试
 * (src 非法/token 过期/源站不存在时重试只会再赔一个 RTT), 且循环顶部检查线程中断
 * (用户切集/退出时应尽快放弃, 而不是继续等满超时)。
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
        /** 响应异常(超体积上限/读失败) — 服务端返回内容有问题, 重试同样会超限。 */
        BAD_RESPONSE,
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
        if (proxyBase == null || proxyBase.isEmpty() || raw == null || raw.length == 0) {
            return Outcome.fail(FailureCause.NO_FILTER);
        }
        final String url;
        try {
            String base = proxyBase.endsWith("/")
                    ? proxyBase.substring(0, proxyBase.length() - 1)
                    : proxyBase;
            // 编码与 PlayerUrls.encodeParam / web 端 encodeURIComponent 一致(RFC 3986):
            // 不用 URLEncoder(空格→'+'、'*' 不编码), 否则同一集在两端产生不同的 src 串,
            // 服务端按 src 归并的过滤缓存与统计会对不上。
            url = base + "/v1/m3u8/filter?src=" + PlayerUrls.encodeParam(srcUrl);
        } catch (Exception e) {
            return Outcome.fail(FailureCause.NO_FILTER);
        }
        for (int attempt = 0; attempt < 2; attempt++) {
            // 用户切集/退出会中断 loader 线程(BufferPrefetcher.cancel → shutdownNow):
            // 此时应尽快放弃, 而不是继续等满一轮超时(最坏 16s×2 层= 30s+ 黑屏)。
            if (Thread.currentThread().isInterrupted()) {
                return Outcome.fail(FailureCause.CANCELLED);
            }
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
                        return Outcome.fail(FailureCause.REJECTED);
                    }
                    if (resp.isSuccessful() && resp.body() != null) {
                        byte[] out = readCapped(resp.body(), MAX_PLAYLIST_BYTES);
                        // 超限: 不重试(重试同样会超)
                        if (out == null) return Outcome.fail(FailureCause.BAD_RESPONSE);
                        int cnt = 0;
                        String n = resp.header("X-Ad-Filtered");
                        if (n != null) {
                            try {
                                cnt = Integer.parseInt(n);
                            } catch (NumberFormatException ignore) {
                            }
                        }
                        return Outcome.ok(new Result(out, cnt));
                    }
                    // 5xx / 3xx / 无 body → 值得重试一次
                    if (attempt == 1) return Outcome.fail(FailureCause.NETWORK);
                }
            } catch (Exception e) {
                // 必须重设中断位: OkHttp 的 execute() 在线程被 shutdownNow 中断时抛的异常
                // 类型不一定是 InterruptedException, 若不在这里 interrupt(), 中断状态会丢,
                // 上层(cancel → 已取消)就再也判断不出"这是取消不是失败"。
                // 注意 InterruptedException 在 java.lang(默认导入), 不是 java.io。
                if (e instanceof InterruptedException) {
                    Thread.currentThread().interrupt();
                    return Outcome.fail(FailureCause.CANCELLED);
                }
                if (attempt == 1) return Outcome.fail(FailureCause.NETWORK);
            }
            try {
                Thread.sleep(250);
            } catch (InterruptedException ie) {
                Thread.currentThread().interrupt();
                return Outcome.fail(FailureCause.CANCELLED);
            }
        }
        return Outcome.fail(FailureCause.NETWORK);
    }

    /**
     * 读响应体并对体积设上限 — 超限返回 null(而非抛异常), 调用方按"过滤失败"处理。
     *
     * <p>{@code body.bytes()} 会把整个响应读进堆, 没有上限时源站返回一个"巨型 m3u8"
     * 就能让 TV 端 OOM。
     */
    private static byte[] readCapped(okhttp3.ResponseBody body, int cap) {
        try {
            if (body.contentLength() > cap) return null;
            java.io.InputStream in = body.byteStream();
            java.io.ByteArrayOutputStream bos = new java.io.ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            int n, total = 0;
            while ((n = in.read(buf)) != -1) {
                total += n;
                if (total > cap) return null;
                bos.write(buf, 0, n);
            }
            return bos.toByteArray();
        } catch (java.io.IOException e) {
            return null;
        }
    }
}
