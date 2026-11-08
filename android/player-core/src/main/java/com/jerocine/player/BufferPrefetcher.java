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

import com.jerocine.player.PlayerUrls;
import com.jerocine.player.download.DownloadEngine;
import com.jerocine.player.download.PlaylistSegments;

import java.io.IOException;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

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
 *   2. GET 当前集原始 m3u8 → 本地 M3u8FilterCore 过滤 → 过滤后清单
 *   3. PlaylistSegments 解析 + startIndexForOffset 定位当前播放位置所在分片(相对窗口起点)
 *      + rangeForMinutes 取"当前位置起往后 N 分钟"(minutes=0 则到片尾)
 *   4. 对每个分片 CacheWriter(upstream=OkHttpDataSource, cache=播放缓存, key=源站URL) 写缓存
 *      —— parallelism=1 顺序写(默认, 与 2026-11-09 前行为逐值一致);
 *        parallelism≥2 固定线程池并行写(buffer-perf 规格 D①)
 *   5. 进度回调(已缓存 x/N 分钟) → 完成回调 → 调用方自动 resume
 * </pre>
 *
 * <p>取消: {@link #cancel()} 停止剩余分片(已写分片留在缓存, 无害)。
 */
public final class BufferPrefetcher {

    private static final String TAG = "BufferPrefetcher";
    /** 与播放侧 UA 一致(PlayerActivity setUserAgent "Jerocine/1.0 (Android TV)"): UA 敏感源两边不一致会缓存 miss. */
    private static final String UA = "Jerocine/1.0 (Android TV)";
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
    /**
     * 写并行度(buffer-perf 规格 D①): 1 = 顺序档(默认, 与现状逐值同语义);
     * ≥2 = 固定线程池并行档。PlayerActivity 按用户开关传 1/3。
     */
    private final int parallelism;
    /** 调度线程(现状角色): pre-work + 提交分片任务 + 收尾回调(仍只从这里 post 主线程). */
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    /**
     * 写分片池, parallelism≥2 才建。红线(D①, DownloadEngine.java:1255-1263 OOM 教训):
     * 禁 cachedThreadPool —— 固定池线程数恒定; 池随实例生灭(构造建 / finally shutdown /
     * cancel shutdownNow), 无跨实例共享/静态池。
     */
    @Nullable
    private final ExecutorService writePool;
    /** 在途写登记处(D④): cancel 掐所有在途, 取代旧 activeWriter 单引用。 */
    private final BufferWriterRegistry registry = new BufferWriterRegistry();
    private volatile boolean cancelled;
    /** 首片失败标志(D④): 分片任务入口查 {@code cancelled || failed} 短路排队任务。 */
    private volatile boolean failed;

    public BufferPrefetcher(Cache cache) {
        this(cache, 1);
    }

    public BufferPrefetcher(Cache cache, int parallelism) {
        this.cache = cache;
        this.parallelism = Math.max(1, parallelism);
        this.writePool = this.parallelism >= 2
                ? Executors.newFixedThreadPool(this.parallelism) : null;
    }

    /**
     * 开始预取. minutes: 0 = 从 startOffsetSec 到片尾; >0 = 从 startOffsetSec 起往后 N 分钟
     * (2026-11-08 改相对语义: 用户原话"针对当前播放时间点往后的相对时间" —— 从片头起算
     * 会把已看过的片段重复写缓存, 真正卡顿的"还没看到的部分"反而没缓冲到)。
     * startOffsetSec = 发起缓冲时的播放位置(秒), ≤0 退化为从片头起算。
     *
     * <p>契约(第2轮评审钉死): <b>一实例一次 start</b> —— 正常结束在 finally shutdown 线程池,
     * cancel() 走 shutdownNow, 实例用毕即不可复用(复用会同步抛 RejectedExecutionException,
     * 或因 cancelled 永不复位而秒回"已取消"); 调用方每次缓冲都应 new 新实例
     * (PlayerActivity.startBufferPrefetch 即为每次重建)。
     */
    public void start(final String srcUrl, final double minutes, final double startOffsetSec,
                      final ProgressListener progress, final CompletionListener done) {
        worker.execute(() -> {
            try {
                run(srcUrl, minutes, startOffsetSec, progress, done);
            } catch (Throwable e) {
                // 取消引起的失败要与真实失败区分开。cancel() 会 shutdownNow 中断本线程,
                // 被打断的本地过滤返回 null → 这里抛 "广告过滤失败"。
                // 若不判cancelled, 用户在"缓冲30分钟"中途改选"缓冲5分钟"时, 旧实例会:
                //   ① 弹一个莫名其妙的"缓存缓冲失败: 广告过滤失败"(实际是用户自己取消的)
                //     → 下面的 cancelled 分支把它换成 onError("已取消");
                //   ② 执行 onError 里的 setPlayWhenReady(true) → 在新实例写缓存的同时恢复
                //      播放, 破坏"缓冲中暂停" —— **cancelled 分支挡不住这一半**(它照走
                //      onError 回调), 实际由调用方 PlayerActivity 的实例身份守卫解决:
                //      旧实例回调到达时 bufferPrefetcher 已指向新实例, 直接丢弃
                //      (第2轮评审: 修正本注释此前声称本分支已修复②的误导)。
                // Throwable 而非 Exception(终审 low): OOM/LinkageError 也走失败回调 ——
                // 否则顺序档收尾被跳过、零回调, bufferInProgress 卡 true 到手动取消。
                if (cancelled) {
                    post(() -> done.onError("已取消"));
                    return;
                }
                String msg = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
                Log.w(TAG, "prefetch failed", e);
                // 分片级失败(顺序档 runSequential 已带「分片 x/N」)不再二次加前缀 ——
                // 与并行档文案统一(终审 low: 两档失败 toast 文案不一致)
                String full = msg.startsWith("分片 ") ? msg : "缓存缓冲失败: " + msg;
                post(() -> done.onError(full));
            } finally {
                // 正常结束也要回收(cancel 才走不到的路径); 幂等, 与 cancel 的 shutdown 兼容
                worker.shutdown();
                // 写分片池与 worker 同轨回收(规格 §3.1 生命周期: 构造建/finally shutdown/cancel
                // shutdownNow); 走到此处时 latch 已归零或任务从未提交, shutdown 不甩任何任务
                ExecutorService pool = writePool;
                if (pool != null) pool.shutdown();
            }
        });
    }

    private void run(String srcUrl, double minutes, double startOffsetSec,
                     ProgressListener progress, CompletionListener done) throws Exception {
        // 1) 拉源站清单 → 过滤
        // 【2026-10-08 修“直播流”误报】端侧过滤模式下 mediaUriFor 返回的是**原始源 URL**,
        // 而多数源的根 URL 是 master 清单(master 永远没有 EXT-X-ENDLIST) —— 旧实现直接把
        // master 送 inspect(), 被误判成“直播流”拒绝, 用户看到“缓存缓冲失败: 该片源是直播流”
        // 但实际是纯 VOD。与 DownloadEngine.enqueue 同一修法: master 先下钻第一个 variant
        // (子表才是媒体级清单), 对子表过滤后再解析分片。
        byte[] raw = fetch(srcUrl);
        // 本地过滤(2026-10-28 起服务端过滤下线): M3u8FilterCore 纯函数绝对化+剔广告。
        M3u8FilterCore.Result r = M3u8FilterCore.rewriteM3u8(
                new String(raw, java.nio.charset.StandardCharsets.UTF_8), srcUrl, true);
        byte[] data = r.getText().getBytes(java.nio.charset.StandardCharsets.UTF_8);
        int filteredCount = r.getFiltered();
        String playlistUrl = srcUrl;
        if (!PlayerUrls.hasMediaSegments(data)) {
            String child = DownloadEngine.firstVariantUrl(data, srcUrl);
            if (child == null) {
                throw new IOException("master 清单中没有可用的子表");
            }
            byte[] childRaw = fetch(child);
            M3u8FilterCore.Result cr = M3u8FilterCore.rewriteM3u8(
                    new String(childRaw, java.nio.charset.StandardCharsets.UTF_8), child, true);
            data = cr.getText().getBytes(java.nio.charset.StandardCharsets.UTF_8);
            filteredCount += cr.getFiltered();
            playlistUrl = child; // 分片相对路径按子表 URL 绝对化
        }
        String filtered = new String(data, java.nio.charset.StandardCharsets.UTF_8);
        // 同导出一致: 加密/fMP4/BYTERANGE/直播流无法靠“从缓存取分片字节”生效, 明确拒绝,
        // 否则白耗带宽和磁盘, 播放依然卡。
        String blockReason = PlaylistSegments.inspect(filtered).exportBlockReason("缓冲");
        if (blockReason != null) {
            throw new IOException(blockReason);
        }
        List<PlaylistSegments.Segment> segments = PlaylistSegments.parse(filtered, playlistUrl);
        if (segments.isEmpty()) {
            throw new IOException("清单为空");
        }
        // 时长全为 0(清单缺 EXTINF)时无法定位播放位置、也无法按分钟估算窗口:
        //   · minutes>0 → 拒绝, 让用户改用「缓冲本集」(minutes<=0, 从当前位置到片尾也做不了时兜底从片头)
        //   · offset>0  → 拒绝(无法把播放位置映射到分片), 只有无 offset 的"本集"才退化为全集
        boolean durationKnown = false;
        for (PlaylistSegments.Segment s : segments) {
            if (s.durationSeconds > 0) { durationKnown = true; break; }
        }
        if ((minutes > 0 || startOffsetSec > 0) && !durationKnown) {
            throw new IOException(minutes > 0
                    ? "清单缺少分片时长, 无法按分钟缓冲, 请改用「缓冲本集」"
                    : "清单缺少分片时长, 无法从当前位置定位缓冲起点");
        }
        // 相对窗口起点: 播放位置所在分片(含半片)起; 已到片尾 → 无可缓冲
        int from = 0;
        if (startOffsetSec > 0) {
            from = PlaylistSegments.startIndexForOffset(segments, startOffsetSec);
            if (from < 0) throw new IOException("清单缺少分片时长, 无法定位缓冲起点");
            if (from >= segments.size()) {
                throw new IOException("当前播放位置已到片尾, 没有可缓冲的分片");
            }
        }
        int end = minutes > 0
                ? from + PlaylistSegments.rangeForMinutes(segments.subList(from, segments.size()), minutes)
                : segments.size();
        if (end <= from) {
            throw new IOException("没有可缓冲的分片");
        }

        // 3) 写 [from, end) 片 —— 进度按窗口本身算(target=窗口时长), 与所选档位语义一致
        double targetSec = 0;
        for (int i = from; i < end; i++) targetSec += segments.get(i).durationSeconds;
        // D③: 顺序/并行两模式统一走聚合器(单调 + 去重; 顺序完成序=序号升序, 与累加逐值一致)
        BufferProgressAggregator aggregator = new BufferProgressAggregator(targetSec);

        if (writePool == null) {
            runSequential(from, end, segments, progress, aggregator, done);
        } else {
            runParallel(from, end, segments, progress, aggregator, done);
        }
    }

    /**
     * 顺序档(parallelism=1, 默认): 现状 run() 结构原样 —— 逐片写, 仅两处替换
     * (activeWriter 单引用 → registry 登记/注销; cachedSec 累加 → 聚合器),
     * 观测行为与改前逐值一致(buffer-perf 规格 §3.1)。
     */
    private void runSequential(int from, int end, List<PlaylistSegments.Segment> segments,
                               ProgressListener progress, BufferProgressAggregator aggregator,
                               CompletionListener done) throws IOException {
        for (int i = from; i < end; i++) {
            if (cancelled) break;
            try {
                writeSegment(i, from, end, segments, progress, aggregator);
            } catch (IOException e) {
                // 单片失败(网络抖/源站断) → 停止, 已有缓存保留
                throw new IOException("分片 " + (i - from + 1) + "/" + (end - from) + " 缓冲失败: "
                        + (e.getMessage() == null ? "网络异常" : e.getMessage()));
            }
        }
        if (cancelled) {
            post(() -> done.onError("已取消"));
            return;
        }
        post(() -> done.onComplete());
    }

    /**
     * 并行档(parallelism≥2, buffer-perf 规格 D①/D④ §3.1): 每片一个任务提交固定池,
     * latch await 全部落幕后由本(调度)线程收尾 —— 回调仍只从这里 post 主线程、恰好一次,
     * 优先级 cancelled > 首个失败 > onComplete。
     */
    private void runParallel(int from, int end, List<PlaylistSegments.Segment> segments,
                             ProgressListener progress, BufferProgressAggregator aggregator,
                             CompletionListener done) throws InterruptedException {
        final int total = end - from;
        final CountDownLatch latch = new CountDownLatch(total);
        // 首个失败消息恰一条: 并行多片同炸时 CAS 保证只有第一个进收尾
        final AtomicReference<String> firstFailure = new AtomicReference<>();
        for (int i = from; i < end; i++) {
            final int idx = i;
            writePool.execute(() -> {
                try {
                    // 入口查标志(D④): registry 里只有已 register 的在途者, 排队未跑的任务
                    // 全靠这里查 cancelled||failed 短路 —— 否则首片失败后其余排队任务照常
                    // 依次写完全窗口(30 分钟档 onError 迟到几十分钟)。
                    if (cancelled || failed) return;
                    writeSegment(idx, from, end, segments, progress, aggregator);
                } catch (Throwable t) {
                    // Throwable 而非 Exception(终审 low): OOM 级错误也折成失败消息走
                    // firstFailure 收尾 —— 若放任外抛, 任务线程死亡但 finally 仍落 latch,
                    // 收尾会误判为「全部成功」发出假 onComplete。
                    String reason = t.getMessage() == null
                            ? t.getClass().getSimpleName() : t.getMessage();
                    String msg = (t instanceof IOException)
                            ? "分片 " + (idx - from + 1) + "/" + total + " 缓冲失败: " + reason
                            : "缓存缓冲失败: " + reason;
                    if (firstFailure.compareAndSet(null, msg)) {
                        failed = true;
                        Log.w(TAG, "segment prefetch failed", t);
                        // 掐所有在途(在途 CacheWriter 快速退出); 排队任务走上面的入口短路。
                        // 【不得】此处 writePool.shutdownNow 甩排队任务 —— 被甩的任务不落
                        // latch, await 反而挂死(规格 §3.1)。
                        registry.cancelAll();
                    }
                } finally {
                    latch.countDown();
                }
            });
        }
        // cancel → worker.shutdownNow 中断本线程 → InterruptedException → start 捕获判 cancelled
        latch.await();
        // 收尾判定基于 volatile cancelled/failed, 与任务结束顺序解耦
        if (cancelled) {
            post(() -> done.onError("已取消"));
            return;
        }
        String failure = firstFailure.get();
        if (failure != null) {
            post(() -> done.onError(failure));
            return;
        }
        post(() -> done.onComplete());
    }

    /**
     * 单分片写体(顺序/并行两档完全共用): 每片独立 DataSource + registry 登记/注销 + 聚合器上报。
     */
    private void writeSegment(int i, int from, int end, List<PlaylistSegments.Segment> segments,
                              ProgressListener progress, BufferProgressAggregator aggregator)
            throws IOException {
        PlaylistSegments.Segment seg = segments.get(i);
        // 每片用独立的 upstream: CacheDataSource.close() 会**顺手关掉 upstream**,
        // 共享一个实例的话, 任何一处给 ds 补上 close 都会让后续所有分片抛
        // "DataSource is closed"。目前恰好没 close 才没炸, 这条约定很脆。
        // (OkHttpDataSource 底层共享静态 HTTP 的连接池, 每片新建 DataSource
        //  的代价只是一层对象, 不会多建连接; 并行下更禁止共享, 规格 D⑨⑤)
        OkHttpDataSource upstream = new OkHttpDataSource.Factory(HTTP)
                .setUserAgent(UA)
                .createDataSource();
        CacheDataSource ds = new CacheDataSource(cache, upstream,
                CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR);
        final CacheWriter writer = new CacheWriter(ds, new DataSpec(Uri.parse(seg.url)), null,
                (requestedBytes, writtenBytes, cacheSize) -> {
                });
        // CacheWriter::cancel 适配(D④): registry 经 Cancellable 抽象承载, 不直接依赖 media3 类型;
        // 适配器只建一次 —— register/unregister 必须同一引用(身份集合, 各建一份会注销不掉)。
        final BufferWriterRegistry.Cancellable handle = writer::cancel;
        registry.register(handle);
        try {
            // 评审遗留#2 保守闭死: 任务入口检查与 register 之间存在固有竞态窗口
            // (通过检查后、register 前被置 cancelled/failed), 不复查则残余单片写完;
            // register 后再查一次再 cache(), 命中即刻退出(unregister 由 finally 对称兜底)。
            if (cancelled || failed) return;
            writer.cache();
            aggregator.onSegmentDone(i - from + 1, seg.durationSeconds);
            final double c = aggregator.cachedSeconds(), t = aggregator.targetSeconds();
            post(() -> progress.onProgress(c, t));
        } finally {
            registry.unregister(handle);
        }
    }

    /** 取消: 停止写剩余分片(已写分片留在缓存). */
    public void cancel() {
        cancelled = true;
        // D④: 掐**所有**在途(旧 activeWriter 单引用只掐一个); 单个抛异常吞掉继续其余
        registry.cancelAll();
        // 必须回收线程池: 本类每次"缓存缓冲"都被 new 一个, 而 newSingleThreadExecutor 的
        // 核心线程永不超时回收 → 用户每点一次就泄漏一个永久存活线程 + 一套连接池。
        worker.shutdownNow();
        // 写池同样必须 shutdownNow(D⑨⑥, 每漏一处 = 每次缓冲泄漏 permanent 线程);
        // cancel 路径与失败路径不同: 这里允许甩掉排队任务 —— worker 已中断,
        // latch.await 不再有人等, 被甩任务无需落 latch。
        ExecutorService pool = writePool;
        if (pool != null) pool.shutdownNow();
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
