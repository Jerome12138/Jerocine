package com.jerocine.player.download;

import androidx.media3.exoplayer.hls.playlist.HlsPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylistParser;
import androidx.media3.exoplayer.upstream.ParsingLoadable;

import com.jerocine.player.M3u8FilterCore;
import com.jerocine.player.ErrorDiag;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;

/**
 * 下载器专用过滤解析器 — 下载前用 {@link M3u8FilterCore}(Go 内核 Kotlin 移植) 本地过滤源站 m3u8,
 * 再交默认解析器。2026-10-28 起服务端过滤下线, 本地过滤是唯一路径。
 *
 * <p>实现 {@link ParsingLoadable.Parser}, 直接适配 {@code HlsDownloader} 构造
 * (media3 1.4.1 的 HlsDownloader 第二参是 Parser&lt;HlsPlaylist&gt;, 不是 HlsPlaylistParserFactory);
 * 无参 {@link HlsPlaylistParser} 自动识别 master/media。
 *
 * <p>与播放器的 {@code PlayerAdFilterHelper.FilterPlaylistParser} 语义相同但**不共享**:
 * 播放器解析器依赖 PlayerSession(预取缓存/统计), 下载器没有 session。
 *
 * <p>注入方式: {@link DownloadEngine} 的 DownloaderFactory 对 HLS 请求
 * new HlsDownloader(item, new DownloadFilterPlaylistParserFactory(prefetched), ...),
 * HlsDownloader 用该 parser 解析 master+子表 → 下载列表即"过滤后分片" → 广告段不会进缓存。
 */
public final class DownloadFilterPlaylistParserFactory implements ParsingLoadable.Parser<HlsPlaylist> {

    private static final String TAG = "DownloadFilter";

    private final ParsingLoadable.Parser<HlsPlaylist> delegate = new HlsPlaylistParser();
    /** 入队时已过滤好的 master 清单(按源站 URL) — HlsDownloader 解析 master 时命中, 不再付一次过滤. 可空. */
    private final java.util.Map<String, byte[]> prefetched;

    public DownloadFilterPlaylistParserFactory() {
        this(null);
    }

    public DownloadFilterPlaylistParserFactory(java.util.Map<String, byte[]> prefetched) {
        this.prefetched = prefetched;
    }

    @Override
    public HlsPlaylist parse(android.net.Uri uri, InputStream in) throws IOException {
        byte[] raw = readAll(in);
        // 注意: 必须 android.util.Log —— androidx.media3.common.util.Log 默认 logLevel=WARN,
        // info 全被吞(2026-10-08 排查 DownloadFilter 零日志时踩坑)
        android.util.Log.i(TAG, "parse: " + uri + " rawLen=" + raw.length);
        // 入队预取命中(master 清单) → 直接用结果; 未命中(进程重启后自愈/重试) → 本地过滤
        // (M3u8FilterCore, Go 内核 Kotlin 移植, 纯函数绝对化+过滤)。内核为纯函数不抛;
        // 防御性 catch → 原始流兜底(不过滤, 不阻塞下载): 相对 URL 按本解析 URI 绝对化
        // (语义对齐 media3, 保证下载缓存 key 与播放期一致)。master 级绝对化同样无害。
        byte[] data = null;
        boolean rawFallback = false;
        if (prefetched != null) {
            byte[] hit = prefetched.get(uri.toString());
            if (hit != null) data = hit;
        }
        if (data == null) {
            try {
                M3u8FilterCore.Result r = M3u8FilterCore.rewriteM3u8(
                        new String(raw, StandardCharsets.UTF_8), uri.toString(), true);
                data = r.getText().getBytes(StandardCharsets.UTF_8);
                android.util.Log.i(TAG, "parse local-filter: " + uri
                        + " filtered=" + r.getFiltered() + " total=" + r.getTotal());
            } catch (Throwable t) {
                // 本地内核异常(理论不发生) → 原始流兜底(不过滤, 不阻塞下载)
                android.util.Log.w(TAG, "local filter failed, raw fallback: "
                        + ErrorDiag.safeUrl(uri.toString()));
                data = HlsPlaylistAbsolutizer.absolutize(
                        new String(raw, StandardCharsets.UTF_8), uri.toString())
                        .getBytes(StandardCharsets.UTF_8);
                rawFallback = true;
            }
        }
        android.util.Log.i(TAG,
                "parse done: " + uri + " dataLen=" + data.length
                        + (rawFallback ? " (原始流兜底)" : " (过滤后)"));
        return delegate.parse(uri, new ByteArrayInputStream(data));
    }

    private static byte[] readAll(InputStream in) throws IOException {
        ByteArrayOutputStream bos = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) != -1) {
            bos.write(buf, 0, n);
        }
        return bos.toByteArray();
    }
}
