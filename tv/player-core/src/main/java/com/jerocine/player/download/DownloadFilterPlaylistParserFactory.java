package com.jerocine.player.download;

import androidx.media3.exoplayer.hls.playlist.HlsPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylistParser;
import androidx.media3.exoplayer.upstream.ParsingLoadable;

import com.jerocine.player.M3u8FilterClient;
import com.jerocine.player.ErrorDiag;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;

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
        // 入队预取命中(master 清单) → 直接用结果; 未命中(进程重启后自愈/重试) → POST 过滤,
        // 失败按 {@link DownloadFilterFallbackPolicy} 分级(与入队侧同一份策略):
        //   NETWORK 重试 3 轮后与 NO_FILTER/REJECTED/BAD_RESPONSE 一样原始流兜底;
        //   CANCELLED → 抛错终止本次解析(取消场景任务已被移除, 抛错只是收尾)。
        // 原始流兜底时没有服务端绝对化 → 相对 URL 按本解析 URI 绝对化(语义对齐 media3,
        // 保证下载缓存 key 与播放期一致)。master 级绝对化同样无害(variant 行/URI 属性)。
        byte[] data = null;
        boolean rawFallback = false;
        if (prefetched != null) {
            byte[] hit = prefetched.get(uri.toString());
            if (hit != null) data = hit;
        }
        if (data == null) {
            int attempt = 0;
            while (true) {
                M3u8FilterClient.Outcome o =
                        M3u8FilterClient.filterDetailed(proxyBase, uri.toString(), raw);
                if (o.success()) {
                    data = o.result.data;
                    break;
                }
                attempt++;
                DownloadFilterFallbackPolicy.Action a =
                        DownloadFilterFallbackPolicy.onFilterFailure(o.cause, attempt);
                if (a == DownloadFilterFallbackPolicy.Action.RETRY) continue;
                if (a == DownloadFilterFallbackPolicy.Action.USE_RAW) {
                    // 只记 host, 不记完整 URL: 源站 URL 常带时效签名, 落日志等于扩散凭据
                    android.util.Log.w(TAG, "filter unavailable (" + o.cause
                            + "), raw fallback: " + ErrorDiag.safeUrl(uri.toString()));
                    data = HlsPlaylistAbsolutizer.absolutize(
                            new String(raw, StandardCharsets.UTF_8), uri.toString())
                            .getBytes(StandardCharsets.UTF_8);
                    rawFallback = true;
                    break;
                }
                // FAIL / ABORT: message 只记 host + 原因, 会经 DownloadEngine.fail() 落库上屏
                android.util.Log.w(TAG, "filter failed (" + o.cause + ", attempt " + attempt
                        + "): " + ErrorDiag.safeUrl(uri.toString()));
                throw new IOException("广告过滤失败("
                        + (o.cause == M3u8FilterClient.FailureCause.CANCELLED ? "已取消" : "未知原因")
                        + "): " + ErrorDiag.safeUrl(uri.toString()));
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
