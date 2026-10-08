package com.jerocine.player;

import android.net.Uri;
import android.util.Log;

import androidx.annotation.Nullable;
import androidx.media3.datasource.DataSpec;
import androidx.media3.datasource.DataSource;
import androidx.media3.datasource.cache.Cache;
import androidx.media3.datasource.cache.CacheDataSource;
import androidx.media3.datasource.cache.CacheWriter;
import androidx.media3.datasource.okhttp.OkHttpDataSource;

import com.jerocine.player.download.PlaylistSegments;

import java.io.IOException;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;

/**
 * 缓存缓冲(网络差时提前把当前集前 N 分钟分片写进**播放缓存区**).
 *
 * <p>路径 B(见 harness 详细设计 §3.4): 写入与播放器共用的 {@code sCache}(video_cache 1GB LRU),
 * 写完后 CacheDataSource 直接命中 — 不新增容量逻辑, 自动 LRU 淘汰。
 *
 * <p>流程:
 * <pre>
 *   1. 播放暂停(调用方 session.player.pause())
 *   2. GET 当前集原始 m3u8 → M3u8FilterClient 过滤 → 过滤后清单
 *   3. PlaylistSegments 解析 + rangeForMinutes 取前 N 分钟分片
 *   4. 对每个分片 CacheWriter(upstream=OkHttpDataSource, cache=播放缓存, key=源站URL) 顺序写
 *   5. 进度回调(已缓存 x/N 分钟) → 完成回调 → 调用方自动 resume
 * </pre>
 *
 * <p>取消: {@link #cancel()} 停止剩余分片(已写分片留在缓存, 无害)。
 */
public final class BufferPrefetcher {

    private static final String TAG = "BufferPrefetcher";
    private static final String UA = "Jerocine/1.0 (Android)";
    /** 公共 HTTP 客户端(拉清单 + 写分片) — 避免每次预取新建 OkHttpClient. */
    private static final OkHttpClient HTTP = new OkHttpClient.Builder()
            .connectTimeout(30, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .followRedirects(true)
            .followSslRedirects(true)
            .build();

    public interface ProgressListener {
        /** 主线程回调: 已缓存 / 目标 分钟数(0~1 之间语义见实现). */
        void onProgress(double cachedMinutes, double targetMinutes);
    }

    public interface CompletionListener {
        void onComplete();

        void onError(String message);
    }

    private final Cache cache;
    private final String proxyBase;
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private volatile boolean cancelled;
    @Nullable
    private volatile CacheWriter activeWriter;

    public BufferPrefetcher(Cache cache, String proxyBase) {
        this.cache = cache;
        this.proxyBase = proxyBase == null ? "" : proxyBase;
    }

    /** 开始预取. minutes: 0 = 本集全部; >0 = 前 N 分钟. */
    public void start(final String srcUrl, final double minutes,
                      final ProgressListener progress, final CompletionListener done) {
        worker.execute(() -> {
            try {
                run(srcUrl, minutes, progress, done);
            } catch (Exception e) {
                // 取消引起的失败要与真实失败区分开。cancel() 会 shutdownNow 中断本线程,
                // 被打断的 M3u8FilterClient.filter 返回 null → 这里抛 "广告过滤失败"。
                // 若不判cancelled, 用户在"缓冲30分钟"中途改选"缓冲5分钟"时, 旧实例会:
                //   ①弹一个莫名其妙的"缓存缓冲失败: 广告过滤失败"(实际是用户自己取消的);
                //   ② 执行 onError 里的 setPlayWhenReady(true) → **在新实例正在写缓存的
                //      同时恢复播放**, 把"缓冲中暂停"的语义彻底破坏。
                if (cancelled) {
                    post(() -> done.onError("已取消"));
                    return;
                }
                String msg = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
                Log.w(TAG, "prefetch failed", e);
                post(() -> done.onError("缓存缓冲失败: " + msg));
            } finally {
                // 正常结束也要回收(cancel 才走不到的路径); 幂等, 与 cancel 的 shutdown 兼容
                worker.shutdown();
            }
        });
    }

    private void run(String srcUrl, double minutes, ProgressListener progress,
                     CompletionListener done) throws Exception {
        // 1) 拉源站清单 → 过滤
        byte[] raw = fetch(srcUrl);
        M3u8FilterClient.Result r = M3u8FilterClient.filter(proxyBase, srcUrl, raw);
        if (r == null) {
            throw new IOException("广告过滤失败");
        }
        String filtered = new String(r.data, java.nio.charset.StandardCharsets.UTF_8);
        // 同导出一致: 加密/fMP4/BYTERANGE/直播流无法靠"从缓存取分片字节"生效, 明确拒绝,
        // 否则白耗带宽和磁盘, 播放依然卡。
        String blockReason = PlaylistSegments.inspect(filtered).exportBlockReason("缓冲");
        if (blockReason != null) {
            throw new IOException(blockReason);
        }
        List<PlaylistSegments.Segment> segments = PlaylistSegments.parse(filtered, srcUrl);
        if (segments.isEmpty()) {
            throw new IOException("清单为空");
        }
        // 时长全为 0(清单缺 EXTINF)时 rangeForMinutes 会返回全集 → "缓冲 N 分钟"变缓冲整集(数 GB),
        // 且进度条 target=0 恒显 0%。此时直接拒绝, 让用户改用「缓冲本集」(minutes<=0)。
        boolean durationKnown = false;
        for (PlaylistSegments.Segment s : segments) {
            if (s.durationSeconds > 0) { durationKnown = true; break; }
        }
        if (minutes > 0 && !durationKnown) {
            throw new IOException("清单缺少分片时长, 无法按分钟缓冲, 请改用「缓冲本集」");
        }
        int end = minutes > 0
                ? PlaylistSegments.rangeForMinutes(segments, minutes)
                : segments.size();
        if (end <= 0) {
            throw new IOException("没有可缓冲的分片");
        }

        // 2) 顺序写前 end 片
        double targetSec = 0;
        for (int i = 0; i < end; i++) targetSec += segments.get(i).durationSeconds;

        double cachedSec = 0;
        for (int i = 0; i < end; i++) {
            if (cancelled) break;
            PlaylistSegments.Segment seg = segments.get(i);
            // 每片用独立的 upstream: CacheDataSource.close() 会**顺手关掉 upstream**,
            // 共享一个实例的话, 任何一处给 ds 补上 close 都会让后续所有分片抛
            // "DataSource is closed"。目前恰好没 close 才没炸, 这条约定很脆。
            // (OkHttpDataSource 底层共享静态 HTTP 的连接池, 每片新建 DataSource
            //  的代价只是一层对象, 不会多建连接)
            OkHttpDataSource upstream = new OkHttpDataSource.Factory(HTTP)
                    .setUserAgent(UA)
                    .createDataSource();
            CacheDataSource ds = new CacheDataSource(cache, upstream,
                    CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR);
            final CacheWriter writer = new CacheWriter(ds, new DataSpec(Uri.parse(seg.url)), null,
                    (requestedBytes, writtenBytes, cacheSize) -> {
                    });
            activeWriter = writer;
            try {
                writer.cache();
                cachedSec += seg.durationSeconds;
                final double c = cachedSec, t = targetSec;
                post(() -> progress.onProgress(c, t));
            } catch (IOException e) {
                // 单片失败(网络抖/源站断) → 停止, 已有缓存保留
                throw new IOException("分片 " + (i + 1) + "/" + end + " 缓冲失败: "
                        + (e.getMessage() == null ? "网络异常" : e.getMessage()));
            } finally {
                if (activeWriter == writer) activeWriter = null;
            }
        }
        if (cancelled) {
            post(() -> done.onError("已取消"));
            return;
        }
        post(() -> done.onComplete());
    }

    /** 取消: 停止写剩余分片(已写分片留在缓存). */
    public void cancel() {
        cancelled = true;
        CacheWriter w = activeWriter;
        if (w != null) {
            try {
                w.cancel();
            } catch (Exception ignore) {
            }
        }
        // 必须回收线程池: 本类每次"缓存缓冲"都被 new 一个, 而 newSingleThreadExecutor 的
        // 核心线程永不超时回收 → 用户每点一次就泄漏一个永久存活线程 + 一套连接池。
        worker.shutdownNow();
    }

    private byte[] fetch(String url) throws IOException {
        Request req = new Request.Builder().url(url).header("User-Agent", UA).build();
        try (Response resp = HTTP.newCall(req).execute()) {
            if (!resp.isSuccessful() || resp.body() == null) {
                throw new IOException("HTTP " + resp.code());
            }
            return resp.body().bytes();
        }
    }

    private void post(Runnable r) {
        new android.os.Handler(android.os.Looper.getMainLooper()).post(r);
    }
}
