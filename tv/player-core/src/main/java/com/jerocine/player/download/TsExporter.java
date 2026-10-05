package com.jerocine.player.download;

import android.content.ContentValues;
import android.content.Context;
import android.media.MediaScannerConnection;
import android.net.Uri;
import android.os.Build;
import android.os.Environment;
import android.provider.MediaStore;

import androidx.media3.datasource.DataSpec;
import androidx.media3.datasource.FileDataSource;
import androidx.media3.datasource.cache.CacheDataSource;
import androidx.media3.datasource.cache.SimpleCache;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * 导出 .ts — 把已完成下载的集从下载缓存区拼成单个 .ts 文件, 放到系统下载目录.
 *
 * <p>流程(见 harness 详细设计 §3.3):
 * <pre>
 *   1. 解析 filteredPlaylist 分片 URL 顺序列表({@link PlaylistSegments})
 *   2. 对每片: CacheDataSource(下载缓存区) 按分片 URL 读字节 — 命中缓存即明文
 *      (AES-128 流在 DownloadManager 下载时已解密入缓存, 读出即明文)
 *   3. 顺序拼接写 {@code cacheDir/export.tmp} → 成功后写入系统下载目录
 *      (API 29+: MediaStore Downloads, 无需权限; 旧版本写公共 Download, 无权限时回退应用私有目录)
 *   4. 幂等: 已导出且文件存在 → 直接返回已有路径; 导出中(EXPORTING) → 防并发
 * </pre>
 *
 * <p>依赖前提: 下载缓存区无 LRU 淘汰(NoOpCacheEvictor) → 导出时分片必然仍在;
 * 若某片缺失(缓存被清), 该片读取抛错, 导出失败并提示"缓存缺失, 需重新下载该集"。
 */
public final class TsExporter {

    /** 导出结果回调(主线程). */
    public interface Callback {
        void onExported(String path);

        void onError(String message);
    }

    private final Context appContext;
    private final SimpleCache downloadCache;
    /** 共享单线程池 — 导出任务互不阻塞, 避免每次 new 线程池堆积. */
    private static final ExecutorService SHARED_WORKER = Executors.newSingleThreadExecutor();
    /** 主线程 Handler(导出回调) — 静态复用, 避免每次 new. */
    private static final android.os.Handler MAIN = new android.os.Handler(android.os.Looper.getMainLooper());

    public TsExporter(Context context, SimpleCache downloadCache) {
        this.appContext = context.getApplicationContext();
        this.downloadCache = downloadCache;
    }

    /**
     * 导出任务(异步). 内部做 EXPORTING 状态迁移防并发; 导出中重复调用 → 回调 onError("正在导出").
     */
    public void export(DownloadRepository repo, DownloadTask task, Callback cb) {
        SHARED_WORKER.execute(() -> {
            try {
                exportInternal(repo, task, cb);
            } catch (Exception e) {
                String msg = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
                rollbackState(repo, task, "导出失败: " + msg);
                post(() -> cb.onError("导出失败: " + msg));
            }
        });
    }

    private void exportInternal(DownloadRepository repo, DownloadTask task, Callback cb) throws Exception {
        // 幂等: 已导出且文件存在 → 直接返回
        if (task.exportedPath != null && !task.exportedPath.isEmpty()) {
            File exists = new File(task.exportedPath);
            if (exists.exists() && exists.length() > 0) {
                post(() -> cb.onExported(task.exportedPath));
                return;
            }
        }
        // EXPORTING 防并发: 仅 COMPLETED 可进入
        if (!task.canTransitionTo(DownloadTask.STATE_EXPORTING)) {
            post(() -> cb.onError(task.state == DownloadTask.STATE_EXPORTING
                    ? "正在导出中, 请稍候" : "任务未完成, 无法导出"));
            return;
        }
        task.state = DownloadTask.STATE_EXPORTING;
        task.error = "";
        repo.update(task);

        if (task.filteredPlaylist == null || task.filteredPlaylist.isEmpty()) {
            throw new IOException("缺少过滤后清单");
        }
        List<PlaylistSegments.Segment> segments =
                PlaylistSegments.parse(task.filteredPlaylist, task.srcUrl);
        if (segments.isEmpty()) {
            throw new IOException("清单中没有可导出的分片");
        }

        // 1) 拼到暂存文件
        File tmp = new File(task.cacheDir, "export.tmp");
        File parent = tmp.getParentFile();
        if (parent != null && !parent.exists()) parent.mkdirs();
        byte[] buf = new byte[128 * 1024];
        long written = 0;
        try (FileOutputStream out = new FileOutputStream(tmp)) {
            CacheDataSource reader = new CacheDataSource(downloadCache, new FileDataSource(),
                    CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR);
            for (PlaylistSegments.Segment seg : segments) {
                DataSpec spec = new DataSpec(Uri.parse(seg.url));
                reader.open(spec);
                try {
                    int n;
                    while ((n = reader.read(buf, 0, buf.length)) > 0) {
                        out.write(buf, 0, n);
                        written += n;
                    }
                } finally {
                    reader.close();
                }
            }
            out.flush();
        }
        if (written <= 0) {
            throw new IOException("导出结果为空(分片均缺失?)");
        }

        // 2) 写入系统下载目录(API 29+ MediaStore; 旧版本公共 Download, 失败回退应用私有)
        String finalPath = moveToDownloads(task, tmp);
        tmp.delete();

        // 3) 状态回 COMPLETED + 记录导出路径
        task.state = DownloadTask.STATE_COMPLETED;
        task.exportedPath = finalPath;
        task.updatedAt = System.currentTimeMillis();
        repo.update(task);
        post(() -> cb.onExported(finalPath));
    }

    private void rollbackState(DownloadRepository repo, DownloadTask task, String error) {
        try {
            task.state = DownloadTask.STATE_COMPLETED;
            task.error = error;
            task.updatedAt = System.currentTimeMillis();
            repo.update(task);
        } catch (Exception ignore) {
        }
    }

    /** 移动到系统下载目录, 返回最终路径. */
    private String moveToDownloads(DownloadTask task, File tmp) throws IOException {
        String fileName = DownloadTask.exportFileName(task.filmTitle, task.episode);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ContentValues v = new ContentValues();
            v.put(MediaStore.Downloads.DISPLAY_NAME, fileName);
            v.put(MediaStore.Downloads.MIME_TYPE, "video/mp2t");
            v.put(MediaStore.Downloads.IS_PENDING, 1);
            Uri insert = appContext.getContentResolver()
                    .insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, v);
            if (insert == null) {
                throw new IOException("MediaStore 插入失败");
            }
            try (OutputStream os = appContext.getContentResolver().openOutputStream(insert)) {
                if (os == null) throw new IOException("打开下载目录输出流失败");
                copyFile(tmp, os);
            }
            v.clear();
            v.put(MediaStore.Downloads.IS_PENDING, 0);
            appContext.getContentResolver().update(insert, v, null, null);
            return insert.toString();
        }
        // API < 29: 公共 Download 目录(需要 WRITE_EXTERNAL_STORAGE; 无权限时回退应用私有)
        File pub = Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS);
        File dest = new File(pub, fileName);
        try {
            if (!pub.exists()) pub.mkdirs();
            copyFile(tmp, dest);
            MediaScannerConnection.scanFile(appContext, new String[]{dest.getAbsolutePath()}, null, null);
            return dest.getAbsolutePath();
        } catch (Exception e) {
            File dir = appContext.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS);
            if (dir == null) {
                throw new IOException("无法获取应用下载目录");
            }
            File fallback = new File(dir, fileName);
            if (fallback.getParentFile() != null) fallback.getParentFile().mkdirs();
            copyFile(tmp, fallback);
            return fallback.getAbsolutePath() + "(无存储权限, 已存应用目录)";
        }
    }

    private static void copyFile(File src, File dest) throws IOException {
        try (java.io.InputStream in = new java.io.FileInputStream(src);
             FileOutputStream out = new FileOutputStream(dest)) {
            byte[] buf = new byte[128 * 1024];
            int n;
            while ((n = in.read(buf)) > 0) {
                out.write(buf, 0, n);
            }
        }
    }

    private static void copyFile(File src, OutputStream os) throws IOException {
        try (java.io.InputStream in = new java.io.FileInputStream(src)) {
            byte[] buf = new byte[128 * 1024];
            int n;
            while ((n = in.read(buf)) > 0) {
                os.write(buf, 0, n);
            }
        }
    }

    private void post(Runnable r) {
        MAIN.post(r);
    }
}
