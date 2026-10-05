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
                String msg = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
                Log.w(TAG, "prefetch failed", e);
                post(() -> done.onError("缓存缓冲失败: " + msg));
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
        List<PlaylistSegments.Segment> segments = PlaylistSegments.parse(filtered, srcUrl);
        if (segments.isEmpty()) {
            throw new IOException("清单为空");
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

        OkHttpClient http = new OkHttpClient.Builder()
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .build();
        OkHttpDataSource upstream = new OkHttpDataSource.Factory(http)
                .setUserAgent(UA)
                .createDataSource();

        double cachedSec = 0;
        for (int i = 0; i < end; i++) {
            if (cancelled) break;
            PlaylistSegments.Segment seg = segments.get(i);
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
    }

    private byte[] fetch(String url) throws IOException {
        OkHttpClient client = new OkHttpClient.Builder()
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .build();
        Request req = new Request.Builder().url(url).header("User-Agent", UA).build();
        try (Response resp = client.newCall(req).execute()) {
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
