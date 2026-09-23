package com.jerocine.player;

import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;

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
import java.util.Locale;
import java.util.concurrent.TimeUnit;

import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;

/**
 * 广告过滤 — 端侧混合过滤: 抓到 m3u8 后送 /v1/m3u8/filter 剔广告再交默认解析器.
 *
 * 状态(开关/代理地址/线路/统计)全部在 {@link PlayerSession}; 本类只负责过滤动作与持久化,
 * 与其它 helper 无互相引用.
 */
public class PlayerAdFilterHelper {

    private static final String PREFS_NAME = "jerocine";
    private static final String PREF_AD_FILTER = "ad_filter_enabled";

    private final Context context;
    private final PlayerSession session;

    /**
     * 端侧过滤 POST 客户端: 显式短超时上限.
     * 解析线程上同步等待, 不设上限会拖死播放列表解析; 切集时网络争用偶发失败, 调用处会重试一次.
     */
    private final OkHttpClient adStatsClient = new OkHttpClient.Builder()
            .connectTimeout(6, TimeUnit.SECONDS)
            .readTimeout(6, TimeUnit.SECONDS)
            .writeTimeout(6, TimeUnit.SECONDS)
            .callTimeout(8, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .build();

    public PlayerAdFilterHelper(Context context, PlayerSession session) {
        this.context = context;
        this.session = session;
    }

    SharedPreferences prefs() {
        return context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE);
    }

    /**
     * 解析代理 base: 优先壳层传入的 EXTRA_PROXY_BASE; 为空(壳没传 / 旧缓存 / relaunch)时
     * 用壳注入的兜底地址(见 {@link JerocinePlayer#setDefaultProxyBase}) —— 播放器本身不硬编码域名.
     */
    String resolveProxyBase(Intent intent) {
        String pb = (intent != null) ? intent.getStringExtra(PlayerActivity.EXTRA_PROXY_BASE) : null;
        if (pb == null || pb.isEmpty()) pb = JerocinePlayer.defaultProxyBase();
        return pb == null ? "" : pb;
    }

    /** 当前播放清单 URL(player 上媒体项); 未就绪/已释放返回空串. */
    private String currentMediaUrl() {
        try {
            if (session.player != null
                    && session.player.getCurrentMediaItem() != null
                    && session.player.getCurrentMediaItem().localConfiguration != null) {
                return String.valueOf(session.player.getCurrentMediaItem().localConfiguration.uri);
            }
        } catch (Exception ignore) {
            /* player 未就绪: 视为无片源 */
        }
        return "";
    }

    /** 求出角标状态(无片源 → null, 壳层隐藏角标). 判据与前端 web 播放器五态一致. */
    AdFilterStatus currentStatus() {
        String url = currentMediaUrl();
        if (url.isEmpty()) return null;
        // 服务端代理清单(已在服务端过滤, 端侧不再重复 POST) → 「服务端过滤中」
        boolean viaServerProxy = url.toLowerCase(Locale.US).contains("/m3u8/proxy?");
        return AdFilterStatus.of(
                session.adFilterOn,
                session.filterProxyMissing,
                viaServerProxy || PlayerUrls.isM3u8(url),
                viaServerProxy,
                session.filterFailed,
                session.filterAttempted,
                session.pendingFilteredCount);
    }

    /** 刷新角标(五态整句 + 状态点), 由起播/切集/开关切换调用; 无片源时隐藏. */
    void updateAdFilterBadge() {
        session.host().renderAdFilterBadge(currentStatus());
    }

    /** 起播时按本集实际过滤结果给一次明确提示 + 刷新角标(让"有没有过滤掉"肉眼可见). */
    void showFilterStatus() {
        AdFilterStatus st = currentStatus();
        session.host().renderAdFilterBadge(st);
        if (!session.adFilterOn) return; // 主动关了过滤: 角标已常驻"过滤未开启", 不再弹中央提示
        session.host().showCenterToast(toastFor(st), 2200);
    }

    /** 中央提示文案(角标状态 → 一句话), 与角标语义保持一致. */
    private static String toastFor(AdFilterStatus st) {
        if (st == null) return "广告过滤未触发";
        switch (st.kind) {
            case AdFilterStatus.KIND_INEFFECTIVE:
                return "广告过滤未生效: 代理地址未传(请彻底重启/清缓存或更新到最新)";
            case AdFilterStatus.KIND_FAILED:
                return "广告过滤失败: 网络异常";
            case AdFilterStatus.KIND_FILTERED:
                return "已过滤 " + st.count + " 段广告";
            case AdFilterStatus.KIND_PROXY:
                return "本集由服务端过滤";
            case AdFilterStatus.KIND_CLEAN:
                return "本集未发现广告";
            default:
                return "该源无需过滤";
        }
    }

    /** 切换广告过滤: 持久化 + 重载当前集(保留进度) + 刷新标. */
    void toggleAdFilter() {
        session.adFilterOn = !session.adFilterOn;
        prefs().edit().putBoolean(PREF_AD_FILTER, session.adFilterOn).apply();
        session.forceRawIdx.clear();
        session.reloadCurrentSourceKeepPosition();
        updateAdFilterBadge();
        session.host().showCenterToast(session.adFilterOn ? "广告过滤已开启" : "广告过滤已关闭", 1200);
    }

    /**
     * 同步 POST 原始 m3u8 到 /v1/m3u8/filter, 返回过滤后字节; 失败返回 null(用原始, 优雅降级).
     * 在 loader 线程调用.
     */
    byte[] filterViaServer(String srcUrl, byte[] raw) {
        session.filterAttempted = true;
        final String url;
        try {
            url = session.proxyBase + "/v1/m3u8/filter?src="
                    + java.net.URLEncoder.encode(srcUrl, "UTF-8");
        } catch (Exception e) {
            session.filterFailed = true;
            return null;
        }
        for (int attempt = 0; attempt < 2; attempt++) {
            try {
                Request req = new Request.Builder().url(url)
                        .post(RequestBody.create(
                                MediaType.parse("application/vnd.apple.mpegurl"), raw))
                        .build();
                try (Response resp = adStatsClient.newCall(req).execute()) {
                    if (!resp.isSuccessful() || resp.body() == null) {
                        if (attempt == 1) {
                            session.filterFailed = true;
                            return null;
                        }
                    } else {
                        byte[] out = resp.body().bytes();
                        String n = resp.header("X-Ad-Filtered");
                        if (n != null) {
                            try {
                                int cnt = Integer.parseInt(n);
                                // master 表 cnt=0、子表才 cnt>0; 取最大, 待 STATE_READY 弹一次状态
                                if (cnt > session.pendingFilteredCount) {
                                    session.pendingFilteredCount = cnt;
                                }
                            } catch (NumberFormatException ignore) {
                            }
                        }
                        return out;
                    }
                }
            } catch (Exception e) {
                if (attempt == 1) {
                    session.filterFailed = true;
                    return null;
                }
            }
            try {
                Thread.sleep(250);
            } catch (InterruptedException ie) {
                Thread.currentThread().interrupt();
                session.filterFailed = true;
                return null;
            }
        }
        session.filterFailed = true;
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

    /** 自定义 HLS 播放列表解析器 — 抓到 m3u8 后送 /v1/m3u8/filter 剔广告, 再交默认解析器. */
    class FilterPlaylistParser implements ParsingLoadable.Parser<HlsPlaylist> {
        private final ParsingLoadable.Parser<HlsPlaylist> delegate;

        FilterPlaylistParser(ParsingLoadable.Parser<HlsPlaylist> d) {
            this.delegate = d;
        }

        @Override
        public HlsPlaylist parse(Uri uri, InputStream in) throws IOException {
            byte[] data = readAll(in);
            if (session.adFilterOn) {
                if (!PlayerUrls.needsClientSideFilter(uri.toString())) {
                    // /m3u8/proxy 已在服务端完成过滤与媒体地址改写, 不必再同步 POST 一次
                    session.filterAttempted = true;
                } else if (session.proxyBase != null && !session.proxyBase.isEmpty()) {
                    byte[] f = filterViaServer(uri.toString(), data);
                    if (f != null) data = f;
                } else {
                    session.filterProxyMissing = true; // 开关开着却没代理地址 → 起播提示"未生效"
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
