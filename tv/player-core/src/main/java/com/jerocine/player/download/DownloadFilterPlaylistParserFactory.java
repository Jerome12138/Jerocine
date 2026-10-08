package com.jerocine.player.download;

import androidx.media3.exoplayer.hls.playlist.HlsPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylistParser;
import androidx.media3.exoplayer.upstream.ParsingLoadable;

import com.jerocine.player.M3u8FilterClient;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;

/**
 * 下载器专用过滤解析器 — 下载前把源站 m3u8 送 /v1/m3u8/filter 剔广告, 再交默认解析器.
 *
 * <p>实现 {@link ParsingLoadable.Parser}, 直接适配 {@code HlsDownloader} 构造
 * (media3 1.4.1 的 HlsDownloader 第二参是 Parser&lt;HlsPlaylist&gt;, 不是 HlsPlaylistParserFactory);
 * 无参 {@link HlsPlaylistParser} 自动识别 master/media.
 *
 * <p>与播放器的 {@code PlayerAdFilterHelper.FilterPlaylistParser} 语义相同但**不共享**:
 * 播放器解析器依赖 PlayerSession(预取缓存/失败升级代理/统计), 下载器没有 session;
 * 两者都收敛到 {@link M3u8FilterClient} 这一个网络客户端, 过滤规则只有一份(服务端)。
 *
 * <p>注入方式: {@link DownloadEngine} 的 DownloaderFactory 对 HLS 请求
 * new HlsDownloader(item, new DownloadFilterPlaylistParserFactory(proxyBase, prefetched), ...),
 * HlsDownloader 用该 parser 解析 master+子表 → 下载列表即"过滤后分片" → 广告段不会进缓存。
 */
public final class DownloadFilterPlaylistParserFactory implements ParsingLoadable.Parser<HlsPlaylist> {

    private static final String TAG = "DownloadFilter";

    private final ParsingLoadable.Parser<HlsPlaylist> delegate = new HlsPlaylistParser();
    private final String proxyBase;
    /** 入队时已过滤好的 master 清单(按源站 URL) — HlsDownloader 解析 master 时命中, 不再付一次 POST. 可空. */
    private final java.util.Map<String, byte[]> prefetched;

    public DownloadFilterPlaylistParserFactory(String proxyBase) {
        this(proxyBase, null);
    }

    public DownloadFilterPlaylistParserFactory(String proxyBase, java.util.Map<String, byte[]> prefetched) {
        this.proxyBase = proxyBase;
        this.prefetched = prefetched;
    }

    @Override
    public HlsPlaylist parse(android.net.Uri uri, InputStream in) throws IOException {
        byte[] raw = readAll(in);
        // 注意: 必须 android.util.Log —— androidx.media3.common.util.Log 默认 logLevel=WARN,
        // info 全被吞(2026-10-08 排查 DownloadFilter 零日志时踩坑)
        android.util.Log.i(TAG, "parse: " + uri + " rawLen=" + raw.length);
        // 入队预取命中(master 清单) → 直接用过滤结果; 子表未预取 → POST 过滤
        byte[] data = null;
        if (prefetched != null) {
            byte[] hit = prefetched.get(uri.toString());
            if (hit != null) data = hit;
        }
        if (data == null) {
            M3u8FilterClient.Result r = M3u8FilterClient.filter(proxyBase, uri.toString(), raw);
            if (r == null) {
                //只记 host, 不记完整 URL: 这个 message 会经 DownloadEngine.fail()
                // 持久化进 DownloadTask.error 并显示在下载列表的错误列里, 而源站 URL
                // 常带时效签名(?token=xxx&sign=yyy) —— 落库 + 上屏等于扩散凭据。
                android.util.Log.w(TAG,
                        "filter failed: " + com.jerocine.player.ErrorDiag.safeUrl(uri.toString()));
                throw new IOException("广告过滤失败: " + com.jerocine.player.ErrorDiag.safeUrl(uri.toString()));
            }
            data = r.data;
        }
        android.util.Log.i(TAG,
                "parse done: " + uri + " dataLen=" + data.length
                        + (data == raw ? " (原文)" : " (过滤后)"));
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
