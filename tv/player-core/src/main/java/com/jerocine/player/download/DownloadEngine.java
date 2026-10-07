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
import androidx.media3.datasource.DataSpec;
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
     * 入队一个下载任务: 拉源站清单 → 端侧过滤 → 保存过滤后清单(离线播用) → 交给 DownloadService.
     * 过滤失败 → 任务 FAILED(不静默下原始流, 防广告进产物). 幂等: 已存在同 id 任务则忽略.
     */
    public void enqueue(DownloadTask task) {
        // 入队时取快照: worker 线程池异步执行, 直接读 volatile 字段虽可见但语义不清,
        // 且本任务生命周期内应始终用同一个 base(避免中途被 get() 改动导致前后不一致)
        final String base = proxyBase;
        worker.execute(() -> {
            if (repository.get(task.id) != null) {
                return; // 幂等: 已存在(重复入队被 UI 拦截过, 双保险)
            }
            DownloadTask fresh = task;
            try {
                byte[] raw = fetchPlaylist(task.srcUrl);
                M3u8FilterClient.Result r = M3u8FilterClient.filter(base, task.srcUrl, raw);
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
                    .setMimeType("application/vnd.apple.mpegurl")
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
                // DataSpec 不设 key → 与 TsExporter/BufferPrefetcher 的读侧一致
                cache.removeResource(new DataSpec(Uri.parse(seg.url)).key);
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
        repository.updateProgressOnly(taskId, state, d.getBytesDownloaded(),
                d.contentLength, error, System.currentTimeMillis());
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

    /** 自定义 DownloaderFactory — HLS 注入过滤解析器(下载列表=过滤后分片, 广告段不进缓存). */
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
         * 下载执行线程池. 注意**必须用缓存线程池**: DownloadManager 按 maxParallelDownloads(3)
         * 并行调度多个 Downloader, 若这里共享单线程 executor, 所有集的分片加载会退化成串行,
         * "集级并行 3" 形同虚设。cachedThreadPool 空闲 60s 自动回收, 无泄漏.
         */
        private final java.util.concurrent.Executor executor =
                Executors.newCachedThreadPool();

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
                return new HlsDownloader(item,
                        new DownloadFilterPlaylistParserFactory(base, prefetched),
                        dsFactory,
                        executor);
            }
            return new androidx.media3.exoplayer.offline.ProgressiveDownloader(
                    item, dsFactory, executor);
        }
    }
}
