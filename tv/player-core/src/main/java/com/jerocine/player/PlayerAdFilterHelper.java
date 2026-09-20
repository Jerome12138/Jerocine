package com.jerocine.player;

import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.view.View;
import android.widget.TextView;

import androidx.media3.exoplayer.hls.playlist.DefaultHlsPlaylistParserFactory;
import androidx.media3.exoplayer.hls.playlist.HlsMediaPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsMultivariantPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylistParserFactory;
import androidx.media3.exoplayer.upstream.ParsingLoadable;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.concurrent.TimeUnit;

import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;

/**
 * 广告过滤助手 — 端侧混合过滤: 抓到 m3u8 后送 /m3u8/filter 剔广告再解析。
 */
public class PlayerAdFilterHelper {

    private static final String PREFS_NAME = "jerocine";
    private static final String PREF_AD_FILTER = "ad_filter_enabled";

    private final Context context;
    private final TextView adFilterBadge;
    private final PlayerActivity activity;

    String proxyBase = "";
    volatile boolean adFilterOn = true;
    List<String> currentRawUrls = new ArrayList<>();
    final Set<Integer> forceRawIdx = new HashSet<>();
    volatile String lastFilterToastUrl = "";
    volatile int pendingFilteredCount = 0;
    volatile boolean filterAttempted = false;
    volatile boolean filterFailed = false;
    volatile boolean filterProxyMissing = false;
    boolean filterToastShownForEpisode = false;

    private final OkHttpClient adStatsClient = new OkHttpClient.Builder()
            .connectTimeout(6, TimeUnit.SECONDS)
            .readTimeout(6, TimeUnit.SECONDS)
            .writeTimeout(6, TimeUnit.SECONDS)
            .callTimeout(8, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .build();

    public PlayerAdFilterHelper(Context context, TextView adFilterBadge, PlayerActivity activity) {
        this.context = context;
        this.adFilterBadge = adFilterBadge;
        this.activity = activity;
    }

    SharedPreferences prefs() {
        return context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE);
    }

    /**
     * 解析代理 base: 优先 web 传入的 EXTRA_PROXY_BASE; 为空时兜底用本机配置的服务器地址拼 /api。
     */
    String resolveProxyBase(Intent intent) {
        String pb = (intent != null) ? intent.getStringExtra(PlayerActivity.EXTRA_PROXY_BASE) : null;
        if (pb == null || pb.isEmpty()) {
            String server = context.getSharedPreferences("jerocine", Context.MODE_PRIVATE)
                    .getString("server_url", "https://jerocine.art");
            if (server != null && !server.isEmpty()) {
                pb = server.replaceAll("/+$", "") + "/api";
            }
        }
        return pb == null ? "" : pb;
    }

    void updateAdFilterBadge() {
        if (adFilterBadge != null) {
            adFilterBadge.setVisibility(adFilterOn ? View.VISIBLE : View.GONE);
            if (adFilterOn) adFilterBadge.setText("过滤");
        }
    }

    /**
     * 起播时按本集实际过滤结果给一次明确提示 + 刷新角标。
     */
    void showFilterStatus() {
        if (!adFilterOn) return;
        final String msg, badge;
        if (filterProxyMissing) {
            msg = "广告过滤未生效: 代理地址未传(请彻底重启/清缓存或更新到最新)";
            badge = "未生效";
        } else if (filterFailed && pendingFilteredCount == 0) {
            msg = "广告过滤失败: 网络异常";
            badge = "失败";
        } else if (pendingFilteredCount > 0) {
            msg = "已过滤 " + pendingFilteredCount + " 段广告";
            badge = String.valueOf(pendingFilteredCount);
        } else if (filterAttempted) {
            msg = "本集未发现广告";
            badge = "0";
        } else {
            msg = "广告过滤未触发";
            badge = "未触发";
        }
        activity.showCenterToast(msg, 2200);
        if (adFilterBadge != null) {
            activity.runOnUiThread(() -> {
                adFilterBadge.setText(badge);
                adFilterBadge.setVisibility(View.VISIBLE);
            });
        }
    }

    /**
     * 切换广告过滤: 持久化 + 重载当前集(保留进度) + 刷新标。
     */
    void toggleAdFilter() {
        adFilterOn = !adFilterOn;
        prefs().edit().putBoolean(PREF_AD_FILTER, adFilterOn).apply();
        forceRawIdx.clear();
        lastFilterToastUrl = "";
        int idx = activity.player != null ? activity.player.getCurrentMediaItemIndex() : 0;
        long pos = activity.player != null ? activity.player.getCurrentPosition() : 0;
        activity.loadSourceIntoPlayer(activity.currentSourceIndex, idx, pos);
        updateAdFilterBadge();
        activity.showCenterToast(adFilterOn ? "广告过滤已开启" : "广告过滤已关闭", 1200);
    }

    /**
     * 同步 POST 原始 m3u8 到 /m3u8/filter, 返回过滤后字节; 失败返回 null(用原始, 优雅降级)。
     */
    byte[] filterViaServer(String srcUrl, byte[] raw) {
        filterAttempted = true;
        final String url;
        try {
            url = proxyBase + "/v1/m3u8/filter?src=" + java.net.URLEncoder.encode(srcUrl, "UTF-8");
        } catch (Exception e) {
            filterFailed = true;
            return null;
        }
        for (int attempt = 0; attempt < 2; attempt++) {
            try {
                Request req = new Request.Builder().url(url)
                        .post(RequestBody.create(MediaType.parse("application/vnd.apple.mpegurl"), raw)).build();
                try (Response resp = adStatsClient.newCall(req).execute()) {
                    if (!resp.isSuccessful() || resp.body() == null) {
                        if (attempt == 1) { filterFailed = true; return null; }
                    } else {
                        byte[] out = resp.body().bytes();
                        String n = resp.header("X-Ad-Filtered");
                        if (n != null) {
                            try {
                                int cnt = Integer.parseInt(n);
                                if (cnt > pendingFilteredCount) pendingFilteredCount = cnt;
                            } catch (NumberFormatException ignore) {
                            }
                        }
                        return out;
                    }
                }
            } catch (Exception e) {
                if (attempt == 1) {
                    filterFailed = true;
                    return null;
                }
            }
            try { Thread.sleep(250); }
            catch (InterruptedException ie) { Thread.currentThread().interrupt(); filterFailed = true; return null; }
        }
        filterFailed = true;
        return null;
    }

    static byte[] readAll(InputStream in) throws IOException {
        ByteArrayOutputStream bos = new ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) != -1) {
            bos.write(buf, 0, n);
        }
        return bos.toByteArray();
    }

    /**
     * 自定义 HLS 播放列表解析器 — 抓到 m3u8 后送 /m3u8/filter 剔广告, 再交默认解析器。
     */
    class FilterPlaylistParser implements ParsingLoadable.Parser<HlsPlaylist> {
        private final ParsingLoadable.Parser<HlsPlaylist> delegate;

        FilterPlaylistParser(ParsingLoadable.Parser<HlsPlaylist> d) {
            this.delegate = d;
        }

        @Override
        public HlsPlaylist parse(Uri uri, InputStream in) throws IOException {
            byte[] data = readAll(in);
            if (adFilterOn) {
                if (proxyBase != null && !proxyBase.isEmpty()) {
                    byte[] f = filterViaServer(uri.toString(), data);
                    if (f != null) {
                        data = f;
                    }
                } else {
                    filterProxyMissing = true;
                }
            }
            return delegate.parse(uri, new ByteArrayInputStream(data));
        }
    }

    class FilterPlaylistParserFactory implements HlsPlaylistParserFactory {
        private final DefaultHlsPlaylistParserFactory def = new DefaultHlsPlaylistParserFactory();

        @Override
        public ParsingLoadable.Parser<HlsPlaylist> createPlaylistParser() {
            return new FilterPlaylistParser(def.createPlaylistParser());
        }

        @Override
        public ParsingLoadable.Parser<HlsPlaylist> createPlaylistParser(
                HlsMultivariantPlaylist m, HlsMediaPlaylist p) {
            return new FilterPlaylistParser(def.createPlaylistParser(m, p));
        }
    }
}
