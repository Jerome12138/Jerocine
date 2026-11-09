package com.jerocine.player;

import android.os.Handler;
import android.os.Looper;
import android.util.Log;

import com.jerocine.player.download.DownloadEngine;

import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

import java.security.SecureRandom;
import java.security.cert.X509Certificate;

import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;

/**
 * 后台源测速引擎(buffer-perf 规格 D⑦) — <b>完全不阻塞起播路径</b>: 调用方在起播装载
 * 完成后延迟 5s 才排程本类, 结果经回调回主线程, 择优与自动切换在 PlayerActivity
 * (D⑧ 守卫 + {@link SourceSpeedRanker})。
 *
 * <p>每源: 取「切过去将播放的那一集」URL({@link PlaybackTarget#switchSource} 钳位口径
 * {@code min(curEp, size-1)}, 消「被测的集 ≠ 将播的集」)→ GET(UA 每请求显式
 * {@link PlayerActivity#MEDIA_UA}, D⑨④ 三处同串)→ master 下钻 ≤2 层
 * ({@link DownloadEngine#firstVariantUrl} + {@link PlayerUrls#hasMediaSegments},
 * 与播放/预取同深度)→ 计「媒体清单 GET 完成」耗时。清单口径而非分片口径:
 * 流量小、预算内必出结果、足够反映源站健康度/RTT。
 *
 * <p>预算与并发: 并行 ≤4(固定池 {@code min(4, 源数)}, 禁 cachedThreadPool ——
 * DownloadEngine OOM 教训红线); 单源 3s 总预算(callTimeout 按剩余预算递减,
 * 钻两层也不超支) + connectTimeout 3s + {@code retryOnConnectionFailure(false)}
 * (计时保真, 死源快速放弃)。
 *
 * <p>client: 新建但 TLS 与播放侧同款(trust-all X509TrustManager + hostnameVerifier
 * 放宽, 镜像 {@code ApiClient.mediaOkhttp} 的传输策略) —— 老设备(Android 6, CA 库停在
 * 2015)不信任现代根证书, 用默认 client 会「播放正常而探测全败」, 恰触发 D⑧ 把好端端的
 * 会话切走。<b>不能</b> newBuilder 复用 mediaOkhttp: 其浏览器 UA 拦截器摘不掉, 而探测
 * 必须 UA 与播放一致。探测批结束即 shutdown 池与 client(对齐 BufferPrefetcher 泄漏教训)。
 *
 * <p>结果仅经回调交付, 会话内存、不落盘不上报; 全程 logcat tag {@code #TAG}。
 * 网络行为依赖真机 —— 不可 JVM 直测面(规格 §4.8), 以码审 + 真机清单覆盖。
 */
final class SourceProbeHelper {

    static final String TAG = "SourceSpeedTest";

    /** 单源总预算(ms): callTimeout/connectTimeout 同值, 死源 3s 内必出结果(D⑦)。 */
    private static final long PROBE_BUDGET_MS = 3_000L;
    /** 并行上限(D⑦ 指令拍板 4; 实际取 min(4, 源数))。 */
    private static final int MAX_PARALLEL = 4;

    /** 探测完成回调 — 引擎保证 post 到主线程、恰好一次。 */
    interface Callback {
        void onProbeDone(List<SourceSpeedRanker.Result> results);
    }

    private SourceProbeHelper() {}

    /**
     * 发起一轮探测(异步, 立即返回): 对 {@code sources} 每源各测「将播放集」的清单 GET
     * 耗时, 全部落幕后以源序(results 与 sources 同序同长)回调。
     *
     * <p><b>整批汇总不占用调用线程</b>(终审修复): 唯一调用点 runSourceProbe 在主线程,
     * 若 futures.get() 循环就地执行, 每轮探测会冻结播放页主线程
     * ≈ ceil(源数/4)×3s、防御超时路径最长 14s(挂起输入超 5s 有 ANR 风险), 直接违反
     * D⑦「延迟 5s <b>后台</b>探测(完全不阻塞)」。故整批搬进一根一次性后台编排线程,
     * 本方法调用即返回; 回调仍保证 post 主线程、恰好一次。
     *
     * @param sources        源快照(调用方在排程时刻拷贝; 本类只读)
     * @param currentEpisode 当前集号(每源按切源钳位口径换算将播集)
     * @param callback       主线程回调
     */
    static void probe(List<PlayerSession.SourceData> sources, int currentEpisode,
                      List<String> curTitles,
                      Callback callback) {
        final List<PlayerSession.SourceData> srcs =
                (sources == null) ? new ArrayList<>() : sources;
        if (srcs.isEmpty()) {
            postToMain(callback, new ArrayList<SourceSpeedRanker.Result>());
            return;
        }
        // 一次性编排线程(命名对齐 jc-prefetch 惯例): run() 结束线程即终止, 无跨批共享;
        // 进程级节流(SourceProbeThrottle)保证其触发频率 ≤ 1 次/30 分钟, 无线程堆积面
        Thread orchestrator = new Thread(
                () -> runBatch(srcs, currentEpisode, curTitles, callback), "jc-source-probe");
        orchestrator.start();
    }

    /** 整批执行(后台编排线程): 提交全部单源任务 → get 汇总 → 回收池/client → post 主线程。 */
    private static void runBatch(List<PlayerSession.SourceData> sources, int currentEpisode,
                                 List<String> curTitles,
                                 Callback callback) {
        final List<SourceSpeedRanker.Result> results = new ArrayList<>();
        final OkHttpClient client = buildClient();
        final ExecutorService pool =
                Executors.newFixedThreadPool(Math.min(MAX_PARALLEL, sources.size()));
        try {
            List<Future<SourceSpeedRanker.Result>> futures = new ArrayList<>();
            for (int i = 0; i < sources.size(); i++) {
                final int idx = i;
                futures.add(pool.submit(
                        () -> probeOne(client, sources.get(idx), idx, currentEpisode, curTitles)));
            }
            // 每任务受 3s 预算硬约束, get 超时纯防御(超时按失败源入列, 不拖死整批)
            for (int i = 0; i < futures.size(); i++) {
                SourceSpeedRanker.Result r;
                try {
                    r = futures.get(i).get(PROBE_BUDGET_MS * 4 + 2_000L, TimeUnit.MILLISECONDS);
                } catch (Exception e) {
                    Log.w(TAG, "probe future timeout/cancel, source=" + i, e);
                    r = new SourceSpeedRanker.Result(i, false, -1L);
                }
                results.add(r);
            }
        } catch (Throwable t) {
            // 兜底: 编排线程上的任何意外(理论只剩 client/pool 装配)也不吞回调 ——
            // callback「恰好一次」契约不破, 结果按已收集部分(或空)交付
            Log.w(TAG, "probe batch failed", t);
        } finally {
            // 批结束即回收(D⑦, 对齐「cancel 必须 shutdownNow」泄漏教训口径):
            // 所有任务已 get 归幕, shutdown 不甩任何任务
            pool.shutdown();
            try {
                client.dispatcher().executorService().shutdown();
                client.connectionPool().evictAll();
            } catch (Exception ignore) {
            }
        }
        postToMain(callback, results);
    }

    /** 单源探测: 计「媒体清单 GET 完成」耗时; 任何异常都折成 ok=false 结果, 不外抛。 */
    private static SourceSpeedRanker.Result probeOne(OkHttpClient client,
                                                     PlayerSession.SourceData src,
                                                     int sourceIndex, int currentEpisode,
                                                     List<String> curTitles) {
        long startNanos = System.nanoTime();
        String reason = "";
        boolean ok = false;
        try {
            if (src == null || src.urls == null || src.urls.isEmpty()) {
                throw new IOException("该源无集 URL");
            }
            // 「切过去将播放的那一集」(D⑦): 复用切源钳位, 不另写第二份 coerceIn
            // 「切过去将播放的那一集」(D⑦) —— 与**真换源同口径**（跨源集对齐）:
            // 否则测的是"另一集"的 URL（同源同 CDN，耗时不变，但口径必须一致 —— 用户要求）。
            // strictAuto=false: 探针只取下标, 对不上也要给一个 URL（放行兜底档）。
            int epIdx = PlaybackTarget.switchSourceAligned(
                    sourceIndex, currentEpisode, 0L, curTitles, src.titles, false).getEpisodeIndex();
            String url = src.urls.get(epIdx);
            long deadlineNanos = startNanos + PROBE_BUDGET_MS * 1_000_000L;
            // master 下钻 ≤2 层(D⑦): 初始 GET + 至多两次 firstVariantUrl 下钻,
            // 全程锁单源 3s 预算(每次 GET 的 callTimeout = 剩余预算)
            byte[] body = httpGet(client, url, deadlineNanos);
            for (int hop = 0; hop < 2 && !PlayerUrls.hasMediaSegments(body); hop++) {
                String child = DownloadEngine.firstVariantUrl(body, url);
                if (child == null) throw new IOException("master 清单无子表");
                url = child;
                body = httpGet(client, url, deadlineNanos);
            }
            if (!PlayerUrls.hasMediaSegments(body)) {
                throw new IOException("下钻 2 层仍非媒体清单");
            }
            ok = true;
        } catch (Exception e) {
            reason = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
        }
        long elapsedMs = (System.nanoTime() - startNanos) / 1_000_000L;
        // 只记源名与耗时, 不记 URL(源站清单常带时效签名, 落日志等于扩散凭据 ——
        // 对齐 ErrorDiag/DownloadEngine 的日志脱敏口径)
        Log.i(TAG, "probe source=" + sourceIndex
                + " name=" + (src != null ? src.name : "?")
                + " ok=" + ok + " elapsedMs=" + elapsedMs
                + (ok ? "" : " reason=" + reason));
        return new SourceSpeedRanker.Result(sourceIndex, ok, elapsedMs);
    }

    /**
     * 带 deadline 的 GET: callTimeout = 剩余预算(钻两层也不超支单源 3s)。
     * UA 每请求显式(D⑨④: 与播放侧 {@link PlayerActivity#MEDIA_UA} 同串)。
     */
    private static byte[] httpGet(OkHttpClient base, String url, long deadlineNanos)
            throws IOException {
        long remainingMs = (deadlineNanos - System.nanoTime()) / 1_000_000L;
        if (remainingMs <= 0) throw new IOException("单源探测预算耗尽");
        // newBuilder 共享连接池, 只覆写本请求的总时限
        OkHttpClient scoped = base.newBuilder()
                .callTimeout(remainingMs, TimeUnit.MILLISECONDS)
                .build();
        Request req = new Request.Builder()
                .url(url)
                .header("User-Agent", PlayerActivity.MEDIA_UA)
                .build();
        try (Response resp = scoped.newCall(req).execute()) {
            if (!resp.isSuccessful() || resp.body() == null) {
                throw new IOException("HTTP " + resp.code());
            }
            return resp.body().bytes();
        }
    }

    /**
     * 探测专用 client(每批一个, 用毕 shutdown): TLS 与播放侧同款(trust-all +
     * hostnameVerifier 放宽, 镜像 ApiClient.mediaOkhttp —— 不能 newBuilder 复用,
     * 其浏览器 UA 拦截器摘不掉); 计时保真三件套: callTimeout/connectTimeout 3s +
     * retryOnConnectionFailure(false)。
     */
    private static OkHttpClient buildClient() {
        OkHttpClient.Builder builder = new OkHttpClient.Builder()
                .connectTimeout(PROBE_BUDGET_MS, TimeUnit.MILLISECONDS)
                .callTimeout(PROBE_BUDGET_MS, TimeUnit.MILLISECONDS)
                .retryOnConnectionFailure(false)
                .followRedirects(true)
                .followSslRedirects(true);
        try {
            X509TrustManager trustAll = new X509TrustManager() {
                @Override
                public void checkClientTrusted(X509Certificate[] chain, String authType) {
                }

                @Override
                public void checkServerTrusted(X509Certificate[] chain, String authType) {
                }

                @Override
                public X509Certificate[] getAcceptedIssuers() {
                    return new X509Certificate[0];
                }
            };
            SSLContext ssl = SSLContext.getInstance("TLS");
            ssl.init(null, new TrustManager[]{trustAll}, new SecureRandom());
            builder.sslSocketFactory(ssl.getSocketFactory(), trustAll)
                    .hostnameVerifier((hostname, sslSession) -> true);
        } catch (Exception e) {
            // TLS 放宽装配失败(理论罕见): 退回系统默认校验 —— 老设备可能探测全败,
            // 熔断兜底, 不会误切(D⑧ margin/守卫仍在)
            Log.w(TAG, "trust-all TLS unavailable, fallback to system defaults", e);
        }
        return builder.build();
    }

    private static void postToMain(Callback callback, List<SourceSpeedRanker.Result> results) {
        new Handler(Looper.getMainLooper()).post(() -> callback.onProbeDone(results));
    }
}
