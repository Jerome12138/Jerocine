package com.jerocine.player.download;

import android.content.Context;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;

import androidx.annotation.Nullable;
import androidx.media3.common.C;
import androidx.media3.common.MediaItem;
import androidx.media3.common.MimeTypes;
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
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.List;
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
 *   <li>广告过滤: 入队前 GET 源站清单 → {@link M3u8FilterClient} 过滤 → 清单落库(离线播放用),
 *       同时注入 {@link DownloadFilterPlaylistParserFactory} 让 HlsDownloader 解析时直接命中预取结果
 *       (master 不再付第二次 POST; 子表仍走 POST)。失败按 {@link DownloadFilterFallbackPolicy}
 *       分级: 网络类重试 3 轮后 FAIL; 无过滤服务/4xx/响应异常 → 原始流兜底(落盘前绝对化);</li>
 *   <li>进度/状态: {@link DownloadManager.Listener} 把 Media3 状态映射到 {@link DownloadTask} 并落库;</li>
 *   <li>崩溃恢复: 启动时把残留 DOWNLOADING 标 PAUSED(等用户续传)。</li>
 * </ul>
 */
@UnstableApi
public final class DownloadEngine {

    private static final String TAG = "DownloadEngine";
    private static final int MAX_PARALLEL = 3;
    /** 入队预取(拉清单 + 过滤)的并发上限 — 与 MAX_PARALLEL 解耦, 见构造器注释。 */
    private static final int ENQUEUE_PARALLEL = 4;
    private static final String CACHE_DIR_NAME = "download_cache";
    private static final String USER_AGENT = "Jerocine/1.0 (Android TV)";

    private static DownloadEngine sInstance;

    private final Context appContext;
    /**
     * 广告过滤接口 base — 可变: {@link #get} 时更新。
     *
     * <p>volatile 是必需的: 写入方是主线程({@code get}), 读取方是 worker 线程池
     * ({@link #enqueue} 的 lambda 与 {@link DownloaderFactoryImpl})。没有 happens-before 时
     * worker 可能读到空串/旧值 →过滤直接返回 null → 任务莫名"广告过滤失败"。
     *
     * <p><b>不要再把它以 final 固化进 {@link DownloaderFactoryImpl}</b>(见那里的说明)。
     */
    private volatile String proxyBase;
    private final File cacheRoot;
    private final SimpleCache cache;
    private final DownloadManager manager;
    private final DownloadRepository repository;
    private final CacheDatabaseProvider dbProvider;
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
        // 固定小线程池, **不要用 newCachedThreadPool**:
        // enqueue 会为每个选中集跑一次"拉清单 + 过滤 POST", 300 集就是 300 条并发 HTTPS +
        // 300 个线程, 必然 OOM 并把源站打挂(过滤接口还有 250ms 重试, 更会放大成风暴)。
        // MAX_PARALLEL=3 只管media3 的**下载阶段**, 管不到这里的入队预取 ——
        // 所以"集级并行 3"在预取阶段实际是 N 并行, 必须在这里也限流。
        this.worker = Executors.newFixedThreadPool(ENQUEUE_PARALLEL);
        this.mainHandler = new Handler(Looper.getMainLooper());

        repository = new DownloadRepository(appContext);
        // 上次进程被杀时仍在下载的任务 → 标 PAUSED(UI 不显示卡死的"下载中")
        repository.markInterruptedAsPaused();

        // 【2026-10-08 根修】下载缓存必须放 getExternalFilesDir(系统不会主动清理), 不能放
        // getCacheDir(): 真机实锤设备存储 93% 时系统把 cache/ 整个清掉 —— 分片、media3 索引、
        // playlist.m3u8 全没了, 但任务记录(databases/)幸存 → 列表显示"已完成 461MB"却
        // 导出 ENOENT / 离线播放"缓存缺失"。主动下载的数据用户花了真金白银的流量, 不是可再生的
        // 播放缓存; 可再生的 video_cache 留在 cache/ 由系统清理是对的。
        File external = appContext.getExternalFilesDir(null);
        File baseDir = (external != null && external.isDirectory()
                || (external != null && external.mkdirs()))
                ? external : appContext.getFilesDir();
        cacheRoot = new File(baseDir, CACHE_DIR_NAME);
        // 迁移: 旧版本数据仍在 cache/download_cache 且未被害的话, 搬到新家(同分区 rename 原子)。
        // 注意必须在 mkdirs 之前: 目标目录已存在时 renameTo 恒失败。
        File legacy = new File(appContext.getCacheDir(), CACHE_DIR_NAME);
        if (legacy.isDirectory() && !cacheRoot.exists() && !legacy.renameTo(cacheRoot)) {
            // rename 失败(极少见)不阻断: 引擎照常用新目录, 旧数据等同丢失(与被系统清理同效)
            android.util.Log.w(TAG, "download_cache 迁移失败, 放弃旧数据: " + legacy);
        }
        if (!cacheRoot.exists()) cacheRoot.mkdirs();
        CacheDatabaseProvider provider = new CacheDatabaseProvider(appContext);
        this.dbProvider = provider;
        cache = new SimpleCache(cacheRoot, new NoOpCacheEvictor(), provider);

        OkHttpClient http = new OkHttpClient.Builder()
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .writeTimeout(30, TimeUnit.SECONDS)
                // 2026-10-08 真机实测: 只有 connect/read/write 超时不够 —— 某些 CDN 的响应
                // 会"涓滴"式卡住(每次 read 都在超时前吐 1 字节), 整个请求永远不结束,
                // worker 线程被无限占用, 任务永远不落库也不失败(下载列表一片空白)。
                // callTimeout 是单次调用总闸, 从根上保证 enqueue 一定能走到终态。
                .callTimeout(90, TimeUnit.SECONDS)
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
                // Supplier 而非值: 引擎可能被后续 get(ctx, base) 刷新 proxyBase,
                // factory 必须读到最新值(进程被杀后服务重建的场景尤其关键)
                new DownloaderFactoryImpl(dsFactory, () -> this.proxyBase, prefetchedPlaylists));
        manager.setMaxParallelDownloads(MAX_PARALLEL);
        manager.setMinRetryCount(2);
        manager.addListener(managerListener);
        // media3 DownloadManager 构造后 downloadsPaused=true, 只有 service 首次 onCreate 会
        // resumeDownloads(); 若 helper 已存在或 manager 被恢复重建, 将永远不恢复 → 任务卡 QUEUED。
        // 这里主动 resume 一次, 不依赖 service 时序, 保证任何创建方入队的任务都能启动。
        manager.resumeDownloads();
        requeueOrphanTasks();
        repairLegacySizes();
    }

    /**
     * 旧任务体积自愈: 历史版本把 media3 的 contentLength(-1) 原样落库, 且卡在 100% 的任务
     * bytesDownloaded=0 → 已完成列表全部显示 "0KB"(2026-10-08 用户实锤)。
     * 这里在引擎创建时扫一遍 COMPLETED 任务: 优先取 media3 DownloadIndex 的真实字节数,
     * 取不到(全缓存命中)则按过滤后清单实测 SimpleCache; 拿到就回写业务表。
     * 跑在 worker 线程(几百集也只有毫秒级元数据查询), 失败静默(UI 仍是 0KB, 不影响功能)。
     */
    private void repairLegacySizes() {
        worker.execute(() -> {
            try {
                DefaultDownloadIndex index = new DefaultDownloadIndex(dbProvider);
                int n = 0;
                for (DownloadTask t : repository.listAll()) {
                    if (t.state != DownloadTask.STATE_COMPLETED) continue;
                    if (t.totalBytes > 0 || t.progressBytes > 0) continue;
                    Download d;
                    try {
                        d = index.getDownload(t.id);
                    } catch (IOException e) {
                        d = null;
                    }
                    long bytes = d != null ? Math.max(0L, d.getBytesDownloaded()) : 0L;
                    if (bytes <= 0) bytes = cachedBytesFor(t);
                    if (bytes <= 0) continue;
                    repository.updateProgressOnly(t.id, DownloadTask.STATE_COMPLETED,
                            bytes, bytes, null, System.currentTimeMillis());
                    notifyChanged(t.id);
                    n++;
                }
                if (n > 0) Log.i(TAG, "自愈: 修复旧任务体积 " + n + " 个");
            } catch (Exception e) {
                Log.w(TAG, "repairLegacySizes failed", e);
            }
        });
    }

    /**
     * 孤儿任务自愈: 业务表里 QUEUED 但 media3 DownloadIndex 没有记录的任务 → 重新发 AddDownload。
     *
     * <p>怎么产生的: enqueue 流程是「拉清单+过滤(网络, 秒级) → insertIgnore(QUEUED) →
     * sendAddDownload」。若进程在 insert 之后、media3 落索引之前被杀(典型: 服务构造函数
     * 崩溃把整个进程带走, 2026-10-08 实测), 业务表就留下了永远没人认领的 QUEUED 行 ——
     * UI 一直显示"排队中", 重启也不会动。
     *
     * <p>过滤后清单大概率已落库(insertIgnore 前写的), media3 重下时经
     * DownloadFilterPlaylistParserFactory 走正常过滤路径或按
     * {@link DownloadFilterFallbackPolicy} 原始流兜底, 与入队侧同一份策略。
     */
    private void requeueOrphanTasks() {
        try {
            DefaultDownloadIndex index = new DefaultDownloadIndex(dbProvider);
            int n = 0;
            for (DownloadTask t : repository.listAll()) {
                if (t.state != DownloadTask.STATE_QUEUED) continue;
                if (t.srcUrl == null || t.srcUrl.isEmpty()) continue;
                boolean exists;
                try {
                    exists = index.getDownload(t.id) != null;
                } catch (IOException e) {
                    exists = false;
                }
                if (exists) continue;
                DownloadRequest request = new DownloadRequest.Builder(t.id, Uri.parse(t.srcUrl))
                        .setMimeType(MimeTypes.APPLICATION_M3U8)
                        .setData(t.id.getBytes(StandardCharsets.UTF_8))
                        .build();
                try {
                    DownloadService.sendAddDownload(appContext, JerocineDownloadService.class,
                            request, true);
                    n++;
                } catch (IllegalStateException e) {
                    // 后台启动限制: 放弃本次自愈, 下次引擎创建时再试(不影响已在跑的下载)
                    Log.w(TAG, "requeue blocked by background start limit: " + t.id);
                    return;
                }
            }
            if (n > 0) Log.i(TAG, "自愈: 重新入队孤儿任务 " + n + " 个");
        } catch (Exception e) {
            // 自愈是尽力而为, 任何异常都不能挡住引擎初始化
            Log.w(TAG, "requeueOrphanTasks failed", e);
        }
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

    /**
     * 取已持有 {@code dir} 的下载缓存实例; 本引擎未初始化或目录不匹配返回 null。
     *
     * <p>{@link SimpleCache} 对目录是进程级独占的: media3 内部维护 lockedCacheDirs,
     * 只有 {@link SimpleCache#release()} 才解锁。本引擎的 {@code release()} 是私有且无调用方,
     * 所以 {@code download_cache} 在进程存活期内始终被本引擎的实例锁住 ——
     * 任何第二处 {@code new SimpleCache(download_cache, ...)} 都会抛
     * {@code IllegalStateException("Another SimpleCache instance uses the folder")}。
     *
     * <p>播放器离线播放(EXTRA_CACHE_DIR 指到下载缓存区)必须走这里拿实例, 不能自己new。
     * 注意: 复用时**不能**由播放器另配 LRU 驱逐器, 否则会把"主动下载永不清"的
     * 已下载分片当缓存淘汰掉(引擎用的是 {@link NoOpCacheEvictor})。
     */
    @Nullable
    public static SimpleCache heldCacheFor(File dir) {
        DownloadEngine e = sInstance;
        if (e == null || dir == null) return null;
        return sameDir(e.cacheRoot, dir) ? e.cache : null;
    }

    /** 目录比较(解析符号链接/相对路径), 失败退化为绝对路径比较。 */
    private static boolean sameDir(File a, File b) {
        if (a == null || b == null) return false;
        try {
            return a.getCanonicalPath().equals(b.getCanonicalPath());
        } catch (IOException e) {
            return a.getAbsolutePath().equals(b.getAbsolutePath());
        }
    }

    /**
     * 某集的下载缓存目录(放过滤后清单/导出临时文件, SimpleCache 数据在其父目录)。
     *
     * <p><b>路径安全</b>: {@code filmId} 来自 Intent extra, 而 PlayerActivity 已 exported=true
     * (为接 ACTION_VIEW 本地文件), 任意 App 可注入 {@code film_id=../../databases/x}。
     * 这里双层防护:
     * <ol>
     *   <li>{@link DownloadTask#safeSegment} 字符白名单 —— 路径分隔符直接变下划线;</li>
     *   <li>canonical 断言 —— 兜住符号链接等字符清洗挡不住的越界。</li>
     * </ol>
     * 任一层不通过就抛 {@link IllegalArgumentException}: 调用方是下载/删除路径,
     * 失败可见远优于悄悄写到错误位置(尤其是 {@link #remove()} 会递归删除)。
     */
    public File episodeCacheDir(DownloadTask task) {
        File dir = new File(new File(cacheRoot, DownloadTask.safeSegment(task.filmId)),
                String.valueOf(task.episode));
        try {
            String root = cacheRoot.getCanonicalPath();
            String path = dir.getCanonicalPath();
            if (!path.startsWith(root + File.separator)) {
                throw new IllegalArgumentException("非法filmId: " + task.filmId);
            }
        } catch (IOException e) {
            throw new IllegalArgumentException("无法校验缓存目录路径: " + task.filmId, e);
        }
        return dir;
    }

    // ============================ 入队 / 暂停 / 恢复 / 删除 ============================

    /**
     * 入队一个下载任务: 拉源站清单 → 端侧过滤(失败按 {@link DownloadFilterFallbackPolicy} 分级降级)
     * → 保存清单(离线播用) → 交给 DownloadService. 幂等: 已存在同 id 任务则忽略.
     *
     * <p>降级策略(用户拍板 2026-10-10): 数据获取类失败(NETWORK)重试 3 轮, 耗尽仍失败 → 任务 FAIL;
     * 无过滤服务/服务端拒绝/响应异常 → <b>原始流兜底</b>(媒体级清单落盘前按来源 URL 绝对化,
     * 见 {@link HlsPlaylistAbsolutizer}; 产物含潜在广告段, 用户明确接受该取舍)。
     */
    public void enqueue(DownloadTask task) {
        // 入队时取快照: worker 线程池异步执行, 直接读 volatile 字段虽可见但语义不清,
        // 且本任务生命周期内应始终用同一个 base(避免中途被 get() 改动导致前后不一致)
        final String base = proxyBase;
        worker.execute(() -> {
            Log.i(TAG, "入队开始: " + task.id);
            if (repository.get(task.id) != null) {
                return; // 幂等: 已存在(重复入队被 UI 拦截过, 双保险)
            }
            DownloadTask fresh = task;
            try {
                byte[] raw = fetchPlaylist(task.srcUrl);
                Log.i(TAG, "拉清单完成: " + task.id + " len=" + raw.length);
                FilterOutcome master = filterWithFallback(base, task.srcUrl, raw, "清单");
                if (master.cancelled) {
                    Log.i(TAG, "已取消, 停止入队: " + task.id);
                    return;
                }
                if (master.error != null) {
                    fail(task, master.error);
                    return;
                }
                Log.i(TAG, "过滤完成(" + (master.rawFallback ? "原始流兜底" : "filtered=" + master.filteredCount) + "): " + task.id);
                // 【2026-10-08 根修 "0KB+秒完成+离线播放走网络"】源 URL 常是 **master 清单**
                // (实锤: lz 源的 index.m3u8 只有 96 字节, 一行 variant 指向 2000k/hls/mixed.m3u8)。
                // 若把 master 当 filteredPlaylist 落库:
                //   · PlaylistSegments.parse(master) 会把 variant 行误当 1 个"分片" →
                //     导出/清缓存/体积自愈/离线播放全部建立在错误分片表上;
                //   · 真正的分片要靠 HlsDownloader 下钻 variant 子表再过一轮滤 — 而该步在
                //     getSegments(removeWhenParsed=true) 里异常直接跳过, 设备实测 0 分片秒完成。
                // 修法: 入队时下钻第一个 variant, 拉子表 → 过滤 → 把**媒体级清单**作为
                // filteredPlaylist 落库 + prefetched(子表分片已被服务端绝对化)。
                // HlsDownloader 对 master URI 解析直接命中这份媒体清单(分片全绝对化),
                // 不再需要下钻; 离线播放 file://playlist.m3u8 同理命中缓存。
                byte[] playlistData = master.data;
                if (isMasterPlaylist(playlistData)) {
                    Log.i(TAG, "master 清单, 下钻子表: " + task.id);
                    String childUrl = firstVariantUrl(playlistData, task.srcUrl);
                    if (childUrl == null || childUrl.isEmpty()) {
                        fail(task, "master 清单中没有可用的子清单, 无法下载");
                        return;
                    }
                    byte[] childRaw = fetchPlaylist(childUrl);
                    FilterOutcome child = filterWithFallback(base, childUrl, childRaw, "子表");
                    if (child.cancelled) {
                        Log.i(TAG, "已取消, 停止入队: " + task.id);
                        return;
                    }
                    if (child.error != null) {
                        fail(task, child.error);
                        return;
                    }
                    playlistData = child.data;
                    // 原始流兜底时没有服务端绝对化 —— 子表(媒体级)相对分片按子表 URL 绝对化,
                    // 否则 file:// 离线播放解析不出分片、下载缓存 key 也与播放期不一致。
                    if (child.rawFallback) {
                        playlistData = HlsPlaylistAbsolutizer
                                .absolutize(new String(child.data, StandardCharsets.UTF_8), childUrl)
                                .getBytes(StandardCharsets.UTF_8);
                        Log.i(TAG, "原始流子清单已按子表 URL 绝对化: " + task.id);
                    }
                    Log.i(TAG, "子表完成(" + (child.rawFallback ? "原始流兜底" : "filtered=" + child.filteredCount)
                            + ") len=" + playlistData.length);
                } else if (master.rawFallback) {
                    // 源 URL 本身就是媒体级清单 + 原始流兜底: 相对分片按源 URL 绝对化
                    // (与下方子表兜底同一道理, 否则 file:// 离线播放解析不出分片)
                    playlistData = HlsPlaylistAbsolutizer
                            .absolutize(new String(master.data, StandardCharsets.UTF_8), task.srcUrl)
                            .getBytes(StandardCharsets.UTF_8);
                    Log.i(TAG, "原始流媒体清单已按源 URL 绝对化: " + task.id);
                }
                fresh.filteredPlaylist = new String(playlistData, StandardCharsets.UTF_8);
                prefetchedPlaylists.put(task.srcUrl, playlistData); // master URI 命中, HlsDownloader 直接拿到媒体清单
                File dir = episodeCacheDir(fresh);
                if (!dir.exists()) dir.mkdirs();
                writePlaylistFile(new File(dir, "playlist.m3u8"), playlistData);
                fresh.state = DownloadTask.STATE_QUEUED;
                fresh.error = "";
                if (repository.insertIgnore(fresh)) {
                    Log.i(TAG, "已入队 media3: " + task.id);
                    DownloadRequest request = new DownloadRequest.Builder(task.id, Uri.parse(task.srcUrl))
                            .setMimeType(MimeTypes.APPLICATION_M3U8)
                            .setData(task.id.getBytes(StandardCharsets.UTF_8))
                            .build();
                    DownloadService.sendAddDownload(appContext, JerocineDownloadService.class, request, true);
                }
            } catch (IllegalArgumentException e) {
                // 路径校验失败(filmId 非法): 说清是哪个任务出的问题, 不混进"清单获取失败"
                Log.w(TAG, "enqueue bad filmId: " + task.id, e);
                fail(fresh, "任务参数非法(影片ID 含非法字符), 无法下载");
            } catch (IllegalStateException e) {
                // Android 8+ 后台启动前台服务被系统拒绝: sendAddDownload 跑在 worker 线程,
                // 用户点完"开始下载"可能已退出页面 → 抛 IllegalStateException。
                // 原实现被下面的 catch(Exception) 吞掉并报"清单获取失败", 用户完全看不懂。
                Log.w(TAG, "enqueue blocked by background start limit: " + task.id, e);
                fail(fresh, "系统限制后台启动下载服务, 请回到应用前台后重试");
            } catch (Exception e) {
                Log.w(TAG, "enqueue failed: " + task.id, e);
                fail(fresh, "清单获取失败: " + safeMessage(e));
            }
        });
    }

    /**
 * 暂停(手动): media3 用自定义 stop reason(>0) 表达手动暂停 → 任务 PAUSED。
 *
 * <p><b>必须同步业务表</b>: 只调 {@code manager.setStopReason} 的话, 存在两个问题 ——
 * <ol>
 *   <li><b>竞态窗口</b>: 入队流程先 {@code insertIgnore(QUEUED)} 再异步
 *       {@code sendAddDownload}; 此窗口内 media3 的 DownloadIndex 尚无该 id,
 *       {@code setStopReason} 找不到记录即<b>静默返回</b> → 用户点暂停没反应,
 *       任务照常下载, UI 还一直显示"暂停"按钮可反复点。</li>
 *   <li><b>UI 无反馈</b>: 不写业务表则 UI 拿不到新状态, 用户以为按钮坏了。</li>
 * </ol>
 * 两侧都做: media3 侧对已注册任务生效, 业务表保证即时可见;
 * 若任务还没注册到 media3,业务表的 PAUSED 会在其首次 onDownloadChanged 时被一致保留
 * (onDownloadChanged 以 stopReason 为准, 不会把它改回 QUEUED/DOWNLOADING)。
 */
public void pause(String taskId) {
        manager.setStopReason(taskId, 1);
        DownloadTask t = repository.get(taskId);
        if (t == null) return;
        // 条件更新: 只在"进行中"时改。不用 update(t) 全行覆盖, 避免与导出/进度回写互相踩。
        if (t.state == DownloadTask.STATE_QUEUED || t.state == DownloadTask.STATE_DOWNLOADING
                || t.state == DownloadTask.STATE_PAUSED) {
            repository.updateStateIf(taskId, DownloadTask.STATE_PAUSED,
                    DownloadTask.STATE_QUEUED, DownloadTask.STATE_DOWNLOADING,
                    DownloadTask.STATE_PAUSED);
            notifyChanged(taskId);
        }
    }

    /**
     * 恢复/重试: 清 stop reason 并 resume.
     * FAILED 任务 media3 的 setStopReason 不会重启 → 重新发 AddDownload.
     * 第 4 参 isRemoveFile=true: media3 对已存在同 id 任务会删旧缓存后完整重下
     * (安全, 不抛 "Task already exists"; 代价是已缓存分片不保留, 全量重下).
     */
    public void resume(String taskId) {
        DownloadTask t = repository.get(taskId);
        if (t == null) return;
        if (t.state == DownloadTask.STATE_FAILED) {
            DownloadRequest request = new DownloadRequest.Builder(t.id, Uri.parse(t.srcUrl))
                    .setMimeType(MimeTypes.APPLICATION_M3U8)
                    .setData(t.id.getBytes(StandardCharsets.UTF_8))
                    .build();
            try {
                DownloadService.sendAddDownload(appContext, JerocineDownloadService.class, request, true);
            } catch (IllegalStateException e) {
                // 同 enqueue: 后台启动前台服务被系统拒绝。给可操作的提示, 而不是让异常上抛。
                Log.w(TAG, "resume blocked by background start limit: " + taskId, e);
                notifyChanged(taskId);
                return;
            }
            // media3 会触发 onDownloadChanged(QUEUED/DOWNLOADING) → 业务表状态随之更新
            return;
        }
        // 同步业务表(与 pause 对称): 不然 UI 一直显示"暂停"按钮, 用户看不到恢复生效。
        repository.updateStateIf(taskId, DownloadTask.STATE_QUEUED,
                DownloadTask.STATE_PAUSED);
        manager.setStopReason(taskId, Download.STOP_REASON_NONE);
        manager.resumeDownloads();
        notifyChanged(taskId);
    }

    /**
     * 删除任务: 停下载 + **清 SimpleCache 里的真实分片** + 清任务目录 + 删业务记录。
     *
     * <p><b>为什么必须显式清 SimpleCache</b>: media3 的分片不在 {@code episodeCacheDir} 下,
     * {@code SimpleCache} 的磁盘布局是 {@code download_cache/<0..9>/<contentId>.<uid>.<ext>}
     * (数字子目录, 见 {@code SimpleCache.startFile}), 而 {@code episodeCacheDir} 只是
     * {@code download_cache/<filmId>/<episode>/} —— 两者不是同一条路径。
     * {@code DownloadManager.removeDownload} 也<b>不会</b>删缓存文件(已核对 media3 1.4.1 字节码:
     * 它只发消息并回调 onDownloadRemoved, 无任何 File/Cache 操作)。
     * 所以只删 episodeCacheDir 的话, 用户点删除后几个 GB 的分片纹丝不动, 而缓存区是
     * {@code NoOpCacheEvictor}(主动下载永不清) → 空间永远不释放。
     *
     * <p>cache key 必须与读侧一致: {@link TsExporter} 用 {@code new DataSpec(Uri.parse(url))}
     * (key 为 null), media3 的 {@code DefaultCacheKeyFactory} 此时退化为 {@code uri.toString()}。
     * 这里用同样方式构造, 才能命中同一批 span。
     */
    public void remove(String taskId) {
        DownloadTask t = repository.get(taskId);
        if (t != null) {
            clearDownloadedSegments(t);
            manager.removeDownload(taskId);
            repository.delete(taskId);
            prefetchedPlaylists.remove(t.srcUrl);
            // 用库里的持久化路径而不是重算: safeSegment 上线前入队的任务, DB 里存的是旧路径,
            // 重算会算到新目录 → 老目录永远删不掉(读侧 playOffline/TsExporter 用的也是 DB 值)。
            if (t.cacheDir != null && !t.cacheDir.isEmpty()) {
                deleteRecursive(new File(t.cacheDir));
            } else {
                deleteRecursive(episodeCacheDir(t));
            }
        } else {
            manager.removeDownload(taskId);
            repository.delete(taskId);
        }
    }

    /**
     * 删掉某集在 SimpleCache 里的全部分片 — 按过滤后清单逐个 removeResource。
     *
     * <p>{@code removeResource} 对未命中的 key 是静默无害的, 所以不必先查 {@code getKeys()}。
     * 解析清单失败时不做特殊处理: 最坏情况是残留分片(仍可由用户在系统里清理),
     * 强删则可能误伤其他集 —— 宁可少删不可错删。
     */
    private void clearDownloadedSegments(DownloadTask task) {
        try {
            String playlist = repository.getFilteredPlaylist(task.id);
            if (playlist == null || playlist.isEmpty()) return;
            List<PlaylistSegments.Segment> segments =
                    PlaylistSegments.parse(playlist, task.srcUrl);
            int n = 0;
            for (PlaylistSegments.Segment seg : segments) {
                // 缓存 key 必须与读侧一致: media3 的 DefaultCacheKeyFactory 在 DataSpec.key
                // 为 null 时退化为 uri.toString()。⚠ 2026-10-08 修 bug: 旧代码传
                // `new DataSpec(...).key` —— 那是**恒 null**(DataSpec(Uri) 构造不设 key),
                // removeResource(null) 抛异常被 catch 吞掉 → 分片从来没删掉过,
                // 重下时全部缓存命中 → 秒完成且 bytesDownloaded=0(用户实锤"0KB+秒完成")。
                cache.removeResource(seg.url);
                n++;
            }
            Log.i(TAG, "已清理分片缓存: " + task.id + " × " + n + "片");
        } catch (Exception e) {
            Log.w(TAG, "清理分片缓存失败(残留分片需手动清理): " + task.id, e);
        }
    }

    // ============================ Media3 状态 → 业务状态 ============================

    private void onDownloadChanged(Download d, @Nullable Exception exception) {
        String taskId = new String(d.request.data, StandardCharsets.UTF_8);
        DownloadTask t = repository.get(taskId);
        if (t == null) return;
        int state;
        String error = null;
        switch (d.state) {
            case Download.STATE_QUEUED:
                // media3 setStopReason 只改 stopReason 字段、不改 state → 暂停后 state 仍可能是
                // QUEUED/DOWNLOADING。映射必须以 stopReason 为准, 否则"暂停"在 UI 上看起来无效。
                state = d.stopReason == 0
                        ? DownloadTask.STATE_QUEUED : DownloadTask.STATE_PAUSED;
                break;
            case Download.STATE_DOWNLOADING:
                state = d.stopReason == 0
                        ? DownloadTask.STATE_DOWNLOADING : DownloadTask.STATE_PAUSED;
                break;
            case Download.STATE_COMPLETED:
                state = DownloadTask.STATE_COMPLETED;
                break;
            case Download.STATE_FAILED:
                state = DownloadTask.STATE_FAILED;
                error = failureReasonText(d.failureReason, exception);
                break;
            case Download.STATE_STOPPED:
                state = d.stopReason == 0
                        ? DownloadTask.STATE_QUEUED : DownloadTask.STATE_PAUSED;
                break;
            default: // REMOVING / RESTARTING: 保持原状态(用哨兵表示不写 state)
                state = DownloadRepository.EXPORTING_SENTINEL;
                break;
        }
        // 局部更新(不写全列): 进度回调每 5 秒一次, 全行覆盖会把导出流程正在写的
        // EXPORTING / exportedPath 写回旧值, 击穿防并发并丢失"已导出"标记。
        // updateProgressOnly 内部还带 "state<>EXPORTING" 条件, 双重保护。
        // ⚠ 2026-10-08 修"下载的视频 0KB": HLS 下载的 contentLength 恒为 LENGTH_UNSET(-1)
        // (DownloadRequest 从未 setContentLength, master 清单也不报总体积), 旧代码把 -1
        // 原样落库 → sizeText(-1) 永远显示 "0KB"。完成时 bytesDownloaded 就是真实体积;
        // 若为 0(分片全部缓存命中, 如"删除后重下"), media3 不把缓存命中计入字节数,
        // 此时按过滤后清单实测 SimpleCache 累计(与 TsExporter/离线播放同一套 key)。
        long bytes = Math.max(0L, d.getBytesDownloaded());
        if (d.state == Download.STATE_COMPLETED && bytes <= 0) {
            bytes = cachedBytesFor(t);
        }
        long total = d.state == Download.STATE_COMPLETED ? bytes : d.contentLength;
        Log.i(TAG, "状态变更: " + taskId + " media3state=" + d.state
                + " 业务state=" + state + " bytes=" + bytes + " total=" + total);
        repository.updateProgressOnly(taskId, state, bytes,
                total, error, System.currentTimeMillis());
        notifyChanged(taskId);
        // 终态后 master 预取字节(~77KB/集)已无用(HlsDownloader 不再读它), 及时释放;
        // remove() 也会清, 这里覆盖"下载完成但任务仍在列表里"的常态路径。
        if (d.state == Download.STATE_COMPLETED || d.state == Download.STATE_FAILED) {
            prefetchedPlaylists.remove(t.srcUrl);
        }
    }

    private void onDownloadRemoved(Download d) {
        // prefetchedPlaylists 的 key 是**源站 URL**(enqueue 时 put), 而 Download 只有
        // taskId(=filmId:sourceKey:episode, 反查不到 srcUrl) → 这里无法精确移除。
        // 改由 remove()(有完整任务对象, 按 srcUrl 清) 与 onDownloadChanged 的终态分支兜底。
    }

    /**
     * 按过滤后清单实测某集在下载缓存区的字节数 — COMPLETED 但 bytesDownloaded=0 时的体积兜底。
     *
     * <p>什么时候会走到这里: 分片已经全部在 SimpleCache 里(典型: 删除任务后重下, 而旧版
     * removeResource(null) 又从没删成功过), media3 的 CacheWriter 只计**网络**字节,
     * 缓存命中一个都不计 → bytesDownloaded=0, 但缓存里实打实存着整集。
     *
     * <p>key 语义与 {@link #clearDownloadedSegments}/{@link TsExporter} 一致:
     * DefaultCacheKeyFactory 在 DataSpec.key 为 null 时用 uri.toString()。
     * getCachedBytes 在 worker 语义上是纯内存元数据查询(分片几百个也就毫秒级);
     * 解析/查询失败一律返回 0(UI 退回"大小未知"), 绝不能把异常抛进 onDownloadChanged。
     */
    private long cachedBytesFor(DownloadTask t) {
        try {
            String playlist = repository.getFilteredPlaylist(t.id);
            if (playlist == null || playlist.isEmpty()) return 0L;
            List<PlaylistSegments.Segment> segments =
                    PlaylistSegments.parse(playlist, t.srcUrl);
            long sum = 0L;
            for (PlaylistSegments.Segment seg : segments) {
                sum += cache.getCachedBytes(seg.url, 0L, C.LENGTH_UNSET);
            }
            return sum;
        } catch (Exception e) {
            Log.w(TAG, "实测缓存字节数失败: " + t.id, e);
            return 0L;
        }
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

    /** {@link #filterWithFallback} 的结果: data/error/cancelled 互斥使用。 */
    private static final class FilterOutcome {
        final byte[] data;            // 成功(过滤后或原始流兜底)
        final int filteredCount;      // 成功时被剔除的广告段数(原始流兜底 = 0)
        final boolean rawFallback;    // true = 原始流兜底(调用方需对媒体级清单绝对化)
        final String error;           // 重试耗尽等需要任务 FAIL 的消息
        final boolean cancelled;      // 任务被取消(静默退出, 不写 error)

        private FilterOutcome(byte[] data, int filteredCount, boolean rawFallback,
                              String error, boolean cancelled) {
            this.data = data;
            this.filteredCount = filteredCount;
            this.rawFallback = rawFallback;
            this.error = error;
            this.cancelled = cancelled;
        }

        static FilterOutcome ok(byte[] data, int cnt) {
            return new FilterOutcome(data, cnt, false, null, false);
        }

        static FilterOutcome raw(byte[] data, int cnt) {
            return new FilterOutcome(data, cnt, true, null, false);
        }

        static FilterOutcome err(String msg) {
            return new FilterOutcome(null, 0, false, msg, false);
        }

        static FilterOutcome cancel() {
            return new FilterOutcome(null, 0, false, null, true);
        }
    }

    /**
     * 过滤一单层清单, 带 {@link DownloadFilterFallbackPolicy} 降级策略(用户拍板 2026-10-10):
     * NETWORK → 最多 3 轮(每轮内 M3u8FilterClient 自带 1 次快速重试), 耗尽仍失败 → FAIL;
     * NO_FILTER / REJECTED / BAD_RESPONSE → 原始流兜底(调用方需对媒体级清单绝对化);
     * CANCELLED → 取消。取消失败链路: 中断位已由 client 重设, 这里不再 sleep 直接返回。
     */
    private FilterOutcome filterWithFallback(String base, String url, byte[] rawBytes, String what) {
        int attempt = 0;
        while (true) {
            M3u8FilterClient.Outcome o = M3u8FilterClient.filterDetailed(base, url, rawBytes);
            if (o.success()) {
                return FilterOutcome.ok(o.result.data, o.result.filteredCount);
            }
            attempt++;
            DownloadFilterFallbackPolicy.Action a =
                    DownloadFilterFallbackPolicy.onFilterFailure(o.cause, attempt);
            switch (a) {
                case RETRY:
                    try {
                        Thread.sleep(300);
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        return FilterOutcome.cancel();
                    }
                    break;
                case USE_RAW:
                    // 只记 host, 不记完整 URL: 源站 URL 常带时效签名, 落日志等于扩散凭据
                    Log.w(TAG, what + "过滤不可用(" + o.cause + "), 按原始流下载: "
                            + com.jerocine.player.ErrorDiag.safeUrl(url));
                    return FilterOutcome.raw(rawBytes, 0);
                case FAIL:
                    return FilterOutcome.err("广告过滤失败(网络异常, 已重试"
                            + (attempt - 1) + "次), 可重试");
                case ABORT:
                default:
                    Log.i(TAG, what + "过滤被取消");
                    return FilterOutcome.cancel();
            }
        }
    }

    /** 是否 master 清单(含 #EXT-X-STREAM-INF; 见到 #EXTINF 即媒体级清单, 立即否定)。
     *  public: BufferPrefetcher(缓冲)同样要先下钻 master, 同一套判定。 */
    public static boolean isMasterPlaylist(byte[] data) {
        for (String rawLine : new String(data, StandardCharsets.UTF_8).split("\\r?\\n")) {
            String line = rawLine.trim();
            if (line.startsWith("#EXT-X-STREAM-INF")) return true;
            if (line.startsWith("#EXTINF")) return false;
        }
        return false;
    }

    /**
     * 取 master 清单第一个 variant 的 URL(多码率取首个, 与播放器行为对齐;
     * 相对路径按 master URL 绝对化 — 服务端 FilterText 一般已绝对化, 这里兑底)。
     * public: BufferPrefetcher(缓冲)同样要下钻 master, 同一套取法。
     */
    public static String firstVariantUrl(byte[] data, String masterUrl) {
        boolean pending = false;
        for (String rawLine : new String(data, StandardCharsets.UTF_8).split("\\r?\\n")) {
            String line = rawLine.trim();
            if (line.isEmpty()) continue;
            if (pending && !line.startsWith("#")) {
                return PlaylistSegments.resolveUrl(masterUrl, line);
            }
            pending = line.startsWith("#EXT-X-STREAM-INF");
        }
        return null;
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

    /**
 * 异常 message → 用户可读文案。
 *
     * <p><b>落库前统一脱敏</b>: 这个返回值会写进 {@code DownloadTask.error} 并显示在
     * 下载列表的错误列。异常 message 里常夹带完整源站 URL(含 {@code ?token=}时效签名),
     * 一旦落库 + 上屏就等于扩散了凭据, 而且是**持久化**的(比 toast 更难收回)。
     * 故凡出现 {@code http(s)://} 形态就整体脱敏成"host + 路径末两段"。
     */
    private static String safeMessage(Exception e) {
        String m = e.getMessage();
        if (m == null || m.isEmpty()) return e.getClass().getSimpleName();
        return redactUrls(m);
    }

    /**
     * 把文本中所有 http(s) URL 替换成脱敏形式; 非 URL 文本原样返回。
     *
     * <p>实现取向: 找到 {@code http} 起点后**一直吃到分隔符为止**整段替换, 不做"起点回溯"。
     * 早期版本试图回溯到 URL 起始以保留紧贴的前缀, 但在"括号/引号包裹"与"中文紧贴"两种
     * 真实文案里都会切错(切不到起点 → 整段不替换 → 反而漏掉签名)。
     * 宁可多脱敏一点文本, 也不能漏一个 URL。
     */
    static String redactUrls(String text) {
        if (text == null || !text.contains("://")) return text;
        StringBuilder out = new StringBuilder();
        int i = 0;
        while (i < text.length()) {
            int p = text.indexOf("://", i);
            if (p < 0) {
                out.append(text, i, text.length());
                break;
            }
            // URL 结束: 空白/引号/括号/逗号即止
            int end = p + 3;
            while (end < text.length() && !Character.isWhitespace(text.charAt(end))
                    && text.charAt(end) != '"' && text.charAt(end) != '\''
                    && text.charAt(end) != ')' && text.charAt(end) != ',') {
                end++;
            }
            // 起点: **向前找完整的 http/https 前缀**, 而不是逐字符回溯。
            // 逐字符回溯在"URL 紧贴中文"时会被 8 字符上限截断, 切出不含 http 的残段
            // (如 "h://cdn.y.com/a?token=1") → 判定成非 http 而保留原文 → 反而泄漏。
            int start = -1;
            for (int k = p - 1; k >= i && p - k <= 12; k--) {
                if (!isSchemeBoundary(text, k)) continue;
                // 必须匹配**已知** scheme 名, 不能只判"字符集合法":
                // 否则 "https://" 会先在 k=5命中("tps" 全是合法 scheme 字符),
                // 切出 scheme="ttps" 的残段 → 既漏脱敏又留下原文尾巴。
                if (isKnownScheme(text, k, p)) {
                    start = k;
                    break;
                }
            }
            if (start < 0) {
                // 完全认不出 scheme(跨中文/超长前缀): **整段丢弃**。
                // 宁可少显示几个字, 也不能留一个可能带 token= 的原文。
                i = end;
                continue;
            }
            out.append(text, i, start);
            String scheme = text.substring(start, p).toLowerCase(java.util.Locale.US);
            if ("http".equals(scheme) || "https".equals(scheme)) {
                out.append(com.jerocine.player.ErrorDiag.safeUrl(text.substring(start, end)));
            } else {
                out.append(text, start, end); // ftp/magnet 等不带签名, 原样保留
            }
            i = end;
        }
        return out.toString();
    }

/** 已知的、可能出现在异常文案里的 scheme。 */
    private static final String[] KNOWN_SCHEMES = {
            "http", "https", "ftp", "ftps", "magnet", "file", "content", "rtsp",
    };

    /** text[k..p) 是否恰好是某个已知 scheme 名(大小写不敏感)。 */
    private static boolean isKnownScheme(String text, int k, int p) {
        int len = p - k;
        for (String s : KNOWN_SCHEMES) {
            if (s.length() != len) continue;
            if (text.regionMatches(true, k, s, 0, len)) return true;
        }
        return false;
    }

    /** k 处是否是一个 scheme 的合法起点(前一字符不能是 scheme 字符, 否则是更长单词的一部分)。 */
    private static boolean isSchemeBoundary(String text, int k) {
        if (k == 0) return true;
        char c = text.charAt(k - 1);
        // 只认 ASCII 字母数字与 +-.: 中文等非 ASCII 不能当边界, 否则紧贴中文的
        // "…https://" 会被判成"前一字符是单词一部分"而找不到起点。
        boolean schemeChar = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
                || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.';
        return !schemeChar;
    }

    /**
     * 递归删除 — 带根目录边界断言。
     *
     * <p>边界检查是必须的: {@code episodeCacheDir} 已有字符清洗与 canonical 校验, 但
     * deleteRecursive 是"对任意 File 递归删"的通用操作, 一旦将来被别处复用而传入未校验的路径
     * (或目录内含指向别处的符号链接), 就会删掉应用私有目录树里的其他数据。
     * 这里再挡一层: 不在 cacheRoot 之下就拒绝, 宁可不删也不误删。
     */
    private void deleteRecursive(File f) {
        if (f == null || !f.exists()) return;
        try {
            String root = cacheRoot.getCanonicalPath();
            String path = f.getCanonicalPath();
            if (!path.equals(root) && !path.startsWith(root + File.separator)) {
                Log.w(TAG, "拒绝删除缓存区外的路径: " + path);
                return;
            }
        } catch (IOException e) {
            Log.w(TAG, "无法校验待删路径, 已跳过: " + f);
            return;
        }
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

    /** 自定义 DownloaderFactory — HLS 注入过滤解析器(下载列表=过滤后分片; 原始流兜底任务含潜在广告段, 见 DownloadFilterFallbackPolicy). */
    private static final class DownloaderFactoryImpl implements DownloaderFactory {
        private final CacheDataSource.Factory dsFactory;
        /**
         * 广告过滤 base 的<b>动态</b>读取入口(不能存成final String)。
         *
         * <p>曾经把 proxyBase 以 final 固化在构造时, 后果: 进程被系统回收后
         * {@link JerocineDownloadService} 用空串重建引擎, 之后即使 Activity 再
         * {@code get(ctx, 真实base)} 刷新了字段, 已创建的 factory 仍拿着旧值 →
         * {@link M3u8FilterClient#filter} 直接返回 null → 进程重启后
         * <b>所有在途下载 100% 失败</b>, 且无法自愈。
         */
        private final java.util.function.Supplier<String> proxyBaseSupplier;
        private final java.util.Map<String, byte[]> prefetched;
        /**
         * 下载执行线程池. **必须用固定容量池**: media3 SegmentDownloader 会把一个清单里的
         * **所有分片一次性全部提交**到这个 executor(并行度=池容量), 用 cachedThreadPool
         * 等于无界并发 —— bf 源 51KB 清单几百个分片, 真机实测 710 个线程 + 堆耗尽 OOM,
         * 进程崩溃 → DownloadService 重建再下 → 再崩, 无限循环。固定 4 线程 =
         * 每集分片级并发 4, 集级(3)×分片级(4)=12 并发连接, 安全且不影响集级并行。
         */
        private final java.util.concurrent.Executor executor =
                java.util.concurrent.Executors.newFixedThreadPool(4);

        DownloaderFactoryImpl(CacheDataSource.Factory dsFactory,
                              java.util.function.Supplier<String> proxyBaseSupplier,
                              java.util.Map<String, byte[]> prefetched) {
            this.dsFactory = dsFactory;
            this.proxyBaseSupplier = proxyBaseSupplier;
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
                // 每次现取: 引擎可能被 get() 刷新过 proxyBase, factory 必须跟随
                String base = proxyBaseSupplier.get();
                android.util.Log.i(TAG, "createDownloader: HLS " + request.id
                        + " uri=" + request.uri);
                return new HlsDownloader(item,
                        new DownloadFilterPlaylistParserFactory(base, prefetched),
                        dsFactory,
                        executor);
            }
            // 【2026-10-08 根修】走到这里 = mimeType 推断失败 → ProgressiveDownloader 会把
            // master 清单当"单个资源"整块下载(96 字节, 0.5s COMPLETED, 真分片一个不下)。
            // 曾经填 "application/vnd.apple.mpegurl" 而 media3 只严格认 "application/x-mpegURL",
            // 结果所有任务都掉进这个分支。保持日志, 一旦再出现立刻可见。
            android.util.Log.w(TAG, "createDownloader: 非HLS(type=" + type + ") " + request.id
                    + " uri=" + request.uri + " mimeType=" + request.mimeType);
            return new androidx.media3.exoplayer.offline.ProgressiveDownloader(
                    item, dsFactory, executor);
        }
    }
}
