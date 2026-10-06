package com.jerocine.player.download;

import android.content.Context;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;

import androidx.annotation.Nullable;
import androidx.media3.common.C;
import androidx.media3.common.MediaItem;
import androidx.media3.common.util.UnstableApi;
import androidx.media3.datasource.DataSource;
import androidx.media3.datasource.cache.CacheDataSource;
import androidx.media3.datasource.cache.NoOpCacheEvictor;
import androidx.media3.datasource.cache.SimpleCache;
import androidx.media3.datasource.okhttp.OkHttpDataSource;
import androidx.media3.exoplayer.offline.DefaultDownloadIndex;
import androidx.media3.exoplayer.offline.Download;
import androidx.media3.exoplayer.offline.DownloadManager;
import androidx.media3.exoplayer.offline.DownloadRequest;
import androidx.media3.exoplayer.offline.DownloadService;
import androidx.media3.exoplayer.offline.Downloader;
import androidx.media3.exoplayer.offline.DownloaderFactory;
import androidx.media3.exoplayer.hls.offline.HlsDownloader;

import com.jerocine.player.M3u8FilterClient;

import java.io.File;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;

/**
 * 下载引擎 — Media3 {@link DownloadManager} 封装, 进程级单例.
 *
 * <p>职责:
 * <ul>
 *   <li>下载缓存区: {@code getCacheDir()/download_cache}, SimpleCache + NoOpCacheEvictor(主动下载永不清),
 *       数据库独立见 {@link CacheDatabaseProvider}(与播放器 video_cache 完全隔离);</li>
 *   <li>批量并发: {@code setMaxParallelDownloads(3)} —— 集级并行, 每集内部由 HlsDownloader 串行分片;</li>
 *   <li>广告过滤: 入队前 GET 源站清单 → {@link M3u8FilterClient} 过滤 → 过滤后清单落库(离线播放用),
 *       同时注入 {@link DownloadFilterPlaylistParserFactory} 让 HlsDownloader 解析时直接命中预取结果
 *       (master 不再付第二次 POST; 子表仍走 POST);</li>
 *   <li>进度/状态: {@link DownloadManager.Listener} 把 Media3 状态映射到 {@link DownloadTask} 并落库;</li>
 *   <li>崩溃恢复: 启动时把残留 DOWNLOADING 标 PAUSED(等用户续传)。</li>
 * </ul>
 */
@UnstableApi
public final class DownloadEngine {

    private static final String TAG = "DownloadEngine";
    private static final int MAX_PARALLEL = 3;
    private static final String CACHE_DIR_NAME = "download_cache";
    private static final String USER_AGENT = "Jerocine/1.0 (Android TV)";

    private static DownloadEngine sInstance;

    private final Context appContext;
    /** 广告过滤接口 base — 可变(get() 时更新), 仅影响后续 filter 调用, 不影响下载引擎实例. */
    private String proxyBase;
    private final File cacheRoot;
    private final SimpleCache cache;
    private final DownloadManager manager;
    private final DownloadRepository repository;
    private final ExecutorService worker;
    private final Handler mainHandler;

    /** 入队时已过滤好的 master 清单(按源站 URL) — HlsDownloader 解析 master 时直接命中, 省一次 POST. */
    private final java.util.Map<String, byte[]> prefetchedPlaylists = new java.util.concurrent.ConcurrentHashMap<>();
    /** 公共 HTTP 客户端(拉源站清单用) — 避免每次入队新建 OkHttpClient. */
    private final OkHttpClient playlistClient;

    public static synchronized DownloadEngine get(Context context, String proxyBase) {
        if (sInstance == null) {
            sInstance = new DownloadEngine(context, proxyBase);
            return sInstance;
        }
        // 实例已存在: 只更新过滤接口 base(不 release 重建)。
        // 之前"proxyBase 不同则重建"会 release 掉在途下载的 manager,
        // 导致 DownloadActivity/service helper 持有的旧实例失效 → 任务永远 QUEUED、暂停无效。
        sInstance.proxyBase = proxyBase == null ? "" : proxyBase;
        return sInstance;
    }

    /** 仅查询用(不重建); 未初始化返回 null. */
    @Nullable
    public static DownloadEngine existing() {
        return sInstance;
    }

    private DownloadEngine(Context context, String proxyBase) {
        this.appContext = context.getApplicationContext();
        this.proxyBase = proxyBase == null ? "" : proxyBase;
        this.worker = Executors.newCachedThreadPool();
        this.mainHandler = new Handler(Looper.getMainLooper());

        repository = new DownloadRepository(appContext);
        // 上次进程被杀时仍在下载的任务 → 标 PAUSED(UI 不显示卡死的"下载中")
        repository.markInterruptedAsPaused();

        cacheRoot = new File(appContext.getCacheDir(), CACHE_DIR_NAME);
        if (!cacheRoot.exists()) cacheRoot.mkdirs();
        CacheDatabaseProvider provider = new CacheDatabaseProvider(appContext);
        cache = new SimpleCache(cacheRoot, new NoOpCacheEvictor(), provider);

        OkHttpClient http = new OkHttpClient.Builder()
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .writeTimeout(30, TimeUnit.SECONDS)
                .retryOnConnectionFailure(true)
                .followRedirects(true)
                .followSslRedirects(true)
                .build();
        playlistClient = http;
        OkHttpDataSource.Factory upstream = new OkHttpDataSource.Factory(http)
                .setUserAgent(USER_AGENT);
        CacheDataSource.Factory dsFactory = new CacheDataSource.Factory()
                .setCache(cache)
                .setUpstreamDataSourceFactory(upstream)
                .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR);

        manager = new DownloadManager(appContext, new DefaultDownloadIndex(provider),
                new DownloaderFactoryImpl(dsFactory, proxyBase, prefetchedPlaylists));
        manager.setMaxParallelDownloads(MAX_PARALLEL);
        manager.setMinRetryCount(2);
        manager.addListener(managerListener);
        // media3 DownloadManager 构造后 downloadsPaused=true, 只有 service 首次 onCreate 会
        // resumeDownloads(); 若 helper 已存在或 manager 被恢复重建, 将永远不恢复 → 任务卡 QUEUED。
        // 这里主动 resume 一次, 不依赖 service 时序, 保证任何创建方入队的任务都能启动。
        manager.resumeDownloads();
    }

    /** 保存 listener 引用 — release 时按实例移除(removeListener(null) 语义不明确, 避免). */
    private final DownloadManager.Listener managerListener = new DownloadManager.Listener() {
        @Override
        public void onDownloadChanged(DownloadManager downloadManager, Download download,
                                      @Nullable Exception exception) {
            DownloadEngine.this.onDownloadChanged(download, exception);
        }

        @Override
        public void onDownloadRemoved(DownloadManager downloadManager, Download download) {
            DownloadEngine.this.onDownloadRemoved(download);
        }
    };

    public DownloadManager manager() {
        return manager;
    }

    public DownloadRepository repository() {
        return repository;
    }

    public File cacheRoot() {
        return cacheRoot;
    }

    /** 下载缓存区实例(导出 .ts 用 TsExporter 直读分片). */
    public SimpleCache cache() {
        return cache;
    }

    /** 某集的下载缓存目录(放过滤后清单/导出临时文件, SimpleCache 数据在其父目录). */
    public File episodeCacheDir(DownloadTask task) {
        return new File(cacheRoot, task.filmId + File.separator + task.episode);
    }

    // ============================ 入队 / 暂停 / 恢复 / 删除 ============================

    /**
     * 入队一个下载任务: 拉源站清单 → 端侧过滤 → 保存过滤后清单(离线播用) → 交给 DownloadService.
     * 过滤失败 → 任务 FAILED(不静默下原始流, 防广告进产物). 幂等: 已存在同 id 任务则忽略.
     */
    public void enqueue(DownloadTask task) {
        worker.execute(() -> {
            if (repository.get(task.id) != null) {
                return; // 幂等: 已存在(重复入队被 UI 拦截过, 双保险)
            }
            DownloadTask fresh = task;
            try {
                byte[] raw = fetchPlaylist(task.srcUrl);
                M3u8FilterClient.Result r = M3u8FilterClient.filter(proxyBase, task.srcUrl, raw);
                if (r == null) {
                    fail(task, "广告过滤失败(网络异常或服务端不可用), 可重试");
                    return;
                }
                fresh.filteredPlaylist = new String(r.data, StandardCharsets.UTF_8);
                prefetchedPlaylists.put(task.srcUrl, r.data); // master 命中, HlsDownloader 不再付 POST
                File dir = episodeCacheDir(fresh);
                if (!dir.exists()) dir.mkdirs();
                writePlaylistFile(new File(dir, "playlist.m3u8"), r.data);
                fresh.state = DownloadTask.STATE_QUEUED;
                fresh.error = "";
                if (repository.insertIgnore(fresh)) {
                    DownloadRequest request = new DownloadRequest.Builder(task.id, Uri.parse(task.srcUrl))
                            .setMimeType("application/vnd.apple.mpegurl")
                            .setData(task.id.getBytes(StandardCharsets.UTF_8))
                            .build();
                    DownloadService.sendAddDownload(appContext, JerocineDownloadService.class, request, true);
                }
            } catch (Exception e) {
                Log.w(TAG, "enqueue failed: " + task.id, e);
                fail(fresh, "清单获取失败: " + safeMessage(e));
            }
        });
    }

    /** 暂停(手动): media3 用自定义 stop reason(>0) 表达手动暂停 → 任务 PAUSED. */
    public void pause(String taskId) {
        manager.setStopReason(taskId, 1);
    }

    /**
     * 恢复/重试: 清 stop reason 并 resume.
     * FAILED 任务 media3 的 setStopReason 不会重启 → 重新发 AddDownload.
     * 第 4 参 isRemoveFile=true: media3 对已存在同 id 任务会删旧缓存后完整重下
     * (安全, 不抛 "Task already exists"; 代价是已缓存分片不保留, 全量重下).
     */
    public void resume(String taskId) {
        DownloadTask t = repository.get(taskId);
        if (t != null && t.state == DownloadTask.STATE_FAILED) {
            DownloadRequest request = new DownloadRequest.Builder(t.id, Uri.parse(t.srcUrl))
                    .setMimeType("application/vnd.apple.mpegurl")
                    .setData(t.id.getBytes(StandardCharsets.UTF_8))
                    .build();
            DownloadService.sendAddDownload(appContext, JerocineDownloadService.class, request, true);
            // media3 会触发 onDownloadChanged(QUEUED/DOWNLOADING) → 业务表状态随之更新
            return;
        }
        manager.setStopReason(taskId, Download.STOP_REASON_NONE);
        manager.resumeDownloads();
    }

    /** 删除任务: 停下载 + 清缓存目录 + 删业务记录. */
    public void remove(String taskId) {
        DownloadTask t = repository.get(taskId);
        manager.removeDownload(taskId);
        repository.delete(taskId);
        if (t != null) {
            deleteRecursive(episodeCacheDir(t));
        }
    }

    // ============================ Media3 状态 → 业务状态 ============================

    private void onDownloadChanged(Download d, @Nullable Exception exception) {
        String taskId = new String(d.request.data, StandardCharsets.UTF_8);
        DownloadTask t = repository.get(taskId);
        if (t == null) return;
        DownloadTask next = new DownloadTask();
        next.id = t.id;
        next.filmId = t.filmId;
        next.filmTitle = t.filmTitle;
        next.sourceKey = t.sourceKey;
        next.sourceName = t.sourceName;
        next.episode = t.episode;
        next.episodeTitle = t.episodeTitle;
        next.srcUrl = t.srcUrl;
        next.filteredPlaylist = t.filteredPlaylist;
        next.cacheDir = t.cacheDir;
        next.exportedPath = t.exportedPath;
        next.createdAt = t.createdAt;
        next.updatedAt = System.currentTimeMillis();
        next.progressBytes = d.getBytesDownloaded();
        next.totalBytes = d.contentLength;
        switch (d.state) {
            case Download.STATE_QUEUED:
                // media3 setStopReason 只改 stopReason 字段、不改 state → 暂停后 state 仍可能是
                // QUEUED/DOWNLOADING。映射必须以 stopReason 为准, 否则"暂停"在 UI 上看起来无效。
                next.state = d.stopReason == 0
                        ? DownloadTask.STATE_QUEUED : DownloadTask.STATE_PAUSED;
                break;
            case Download.STATE_DOWNLOADING:
                next.state = d.stopReason == 0
                        ? DownloadTask.STATE_DOWNLOADING : DownloadTask.STATE_PAUSED;
                break;
            case Download.STATE_COMPLETED:
                next.state = DownloadTask.STATE_COMPLETED;
                break;
            case Download.STATE_FAILED:
                next.state = DownloadTask.STATE_FAILED;
                next.error = failureReasonText(d.failureReason, exception);
                break;
            case Download.STATE_STOPPED:
                next.state = d.stopReason == 0
                        ? DownloadTask.STATE_QUEUED : DownloadTask.STATE_PAUSED;
                break;
            default: // REMOVING / RESTARTING: 保持原状态
                next.state = t.state;
                break;
        }
        repository.update(next);
        notifyChanged(taskId);
    }

    private void onDownloadRemoved(Download d) {
        // 注意: prefetchedPlaylists 的 key 是**源站 URL**(enqueue 时 put), 而 Download 只有
        // taskId(=filmId:sourceKey:episode, 反查不到 srcUrl) → 这里无法精确移除。
        // 该 map 的 key 每次 enqueue 时按 srcUrl 覆盖(put), 最多残留一个已删任务的旧清单,
        // 下次同源入队即被覆盖, 无实际危害, 不清理。
    }

    /** UI 刷新钩子 — DownloadActivity 注册, 下载状态变化时在主线程回调. */
    public interface ChangeListener {
        void onDownloadChanged(String taskId);
    }

    private volatile ChangeListener changeListener;

    public void setChangeListener(@Nullable ChangeListener l) {
        this.changeListener = l;
    }

    private void notifyChanged(final String taskId) {
        mainHandler.post(() -> {
            ChangeListener l = changeListener;
            if (l != null) l.onDownloadChanged(taskId);
        });
    }

    // ============================ 内部 ============================

    private byte[] fetchPlaylist(String url) throws Exception {
        Request req = new Request.Builder().url(url).header("User-Agent", USER_AGENT).build();
        try (Response resp = playlistClient.newCall(req).execute()) {
            if (!resp.isSuccessful() || resp.body() == null) {
                throw new java.io.IOException("HTTP " + resp.code());
            }
            return resp.body().bytes();
        }
    }

    private void writePlaylistFile(File f, byte[] data) throws Exception {
        try (FileOutputStream out = new FileOutputStream(f)) {
            out.write(data);
        }
    }

    private void fail(DownloadTask t, String reason) {
        t.state = DownloadTask.STATE_FAILED;
        t.error = reason;
        t.updatedAt = System.currentTimeMillis();
        if (t.createdAt == 0) t.createdAt = System.currentTimeMillis();
        // 失败可能发生在 insertIgnore 之前(拉清单/过滤失败) → 此时 update 写不到行,
        // 失败任务会静默丢失(UI 看不到失败原因)。先查存在性, 不存在则插入。
        if (repository.get(t.id) != null) {
            repository.update(t);
        } else {
            repository.insertIgnore(t);
        }
        notifyChanged(t.id);
    }

    private static String failureReasonText(int failureReason, @Nullable Exception e) {
        switch (failureReason) {
            case Download.FAILURE_REASON_NONE:
                return e != null ? safeMessage(e) : "未知错误";
            case Download.FAILURE_REASON_UNKNOWN:
                return e != null ? safeMessage(e) : "网络异常";
            default:
                return "下载失败(" + failureReason + ")";
        }
    }

    private static String safeMessage(Exception e) {
        String m = e.getMessage();
        return (m == null || m.isEmpty()) ? e.getClass().getSimpleName() : m;
    }

    private static void deleteRecursive(File f) {
        if (f == null || !f.exists()) return;
        if (f.isDirectory()) {
            File[] kids = f.listFiles();
            if (kids != null) {
                for (File k : kids) deleteRecursive(k);
            }
        }
        f.delete();
    }

    private void release() {
        manager.removeListener(managerListener);
        manager.release();
        worker.shutdownNow();
        sInstance = null;
    }

    /** 自定义 DownloaderFactory — HLS 注入过滤解析器(下载列表=过滤后分片, 广告段不进缓存). */
    private static final class DownloaderFactoryImpl implements DownloaderFactory {
        private final CacheDataSource.Factory dsFactory;
        private final String proxyBase;
        private final java.util.Map<String, byte[]> prefetched;
        /**
         * 下载执行线程池. 注意**必须用缓存线程池**: DownloadManager 按 maxParallelDownloads(3)
         * 并行调度多个 Downloader, 若这里共享单线程 executor, 所有集的分片加载会退化成串行,
         * "集级并行 3" 形同虚设。cachedThreadPool 空闲 60s 自动回收, 无泄漏.
         */
        private final java.util.concurrent.Executor executor =
                Executors.newCachedThreadPool();

        DownloaderFactoryImpl(CacheDataSource.Factory dsFactory, String proxyBase,
                              java.util.Map<String, byte[]> prefetched) {
            this.dsFactory = dsFactory;
            this.proxyBase = proxyBase;
            this.prefetched = prefetched;
        }

        @Override
        public Downloader createDownloader(DownloadRequest request) {
            int type = C.CONTENT_TYPE_OTHER;
            try {
                type = androidx.media3.common.util.Util.inferContentTypeForUriAndMimeType(
                        request.uri, request.mimeType);
            } catch (Exception ignore) {
            }
            MediaItem item = request.toMediaItem();
            if (type == C.CONTENT_TYPE_HLS) {
                return new HlsDownloader(item,
                        new DownloadFilterPlaylistParserFactory(proxyBase, prefetched),
                        dsFactory,
                        executor);
            }
            return new androidx.media3.exoplayer.offline.ProgressiveDownloader(
                    item, dsFactory, executor);
        }
    }
}
