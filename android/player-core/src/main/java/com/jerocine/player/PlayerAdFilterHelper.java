package com.jerocine.player;

import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;

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
import java.util.List;
import java.util.Locale;

/**
 * 广告过滤 — 端侧本地过滤(唯一路径): 抓到 m3u8 后用 {@link M3u8FilterCore}(Go 内核 Kotlin 移植)
 * 纯函数绝对化+剔广告, 再交默认解析器。2026-10-28 起服务端过滤/中转(/v1/m3u8/filter、
 * /v1/m3u8/proxy、升级与中转开关)全部下线, 本类不再有服务端回退。
 *
 * 状态(开关/统计)全部在 {@link PlayerSession}; 本类只负责过滤动作与持久化,
 * 与其它 helper 无互相引用.
 */
public class PlayerAdFilterHelper {

    private static final String PREFS_NAME = "jerocine";
    private static final String PREF_AD_FILTER = "ad_filter_enabled";

    private final Context context;
    private final PlayerSession session;

    public PlayerAdFilterHelper(Context context, PlayerSession session) {
        this.context = context;
        this.session = session;
    }

    SharedPreferences prefs() {
        return context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE);
    }

    /**
     * 读取持久化的广告过滤开关(默认开) — 供设置页 / 壳层设置抽屉查询。
     *
     * 与 {@link PlayerNetworkModeHelper#isRelayEnabled} 同款: 开关态既要在播放器内持久化,
     * 又要被"播放器之外"(tv 壳设置抽屉 / web 壳设置抽屉)读写, 所以键名只在这里定义一次,
     * 壳层一律调这两个静态方法 —— 避免各处硬编码 "ad_filter_enabled" 后漂移。
     */
    public static boolean isAdFilterEnabled(Context context) {
        return context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
                .getBoolean(PREF_AD_FILTER, true);
    }

    /** 写持久化的广告过滤开关 — 播放器下一次起播生效(播放中切换走 {@link #toggleAdFilter()}). */
    public static void setAdFilterEnabled(Context context, boolean enabled) {
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
                .edit().putBoolean(PREF_AD_FILTER, enabled).apply();
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

    /** 求出角标状态(无片源 → null, 壳层隐藏角标). 判据与前端 web 播放器一致. */
    AdFilterStatus currentStatus() {
        String url = currentMediaUrl();
        if (url.isEmpty()) return null;
        return AdFilterStatus.of(
                session.adFilterOn,
                PlayerUrls.isM3u8(url),
                filteringInFlight(url),
                session.filterFailed,
                session.filterAttempted,
                session.pendingFilteredCount);
    }

    /**
     * 端侧过滤是否**在途**(角标应显「广告过滤中」): 本集还没有任何过滤结果
     * (attempted/failed/count 全空) 且 当前清单确实要端侧过滤。
     *
     * <p>完全派生、无独立状态位: attempted 由 {@link #filterViaServer} 在**结果落地时**才写,
     * 所以"POST 已发出还没回"与"切集后还没开始 POST"两种在途都由本判定覆盖 ——
     * 起播/切集窗口的第一帧就能如实显示(2026-10-19 用户要求), 不再误显"该源无需过滤"。
     * proxy 清单走「服务端过滤中」、本地集/非 HLS 由 AdFilterStatus 的更高优先级归态。
     */
    private boolean filteringInFlight(String url) {
        if (session.filterAttempted || session.filterFailed
                || session.pendingFilteredCount > 0) {
            return false;
        }
        if (!PlayerUrls.isM3u8(url)) return false;
        int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
        return !session.isEpisodePlayingLocal(idx);
    }

    /** 刷新角标(六态整句 + 状态点), 由起播/切集/开关切换调用; 无片源时隐藏. */
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
            case AdFilterStatus.KIND_FAILED:
                return "广告过滤失败: 网络异常";
            case AdFilterStatus.KIND_FILTERED:
                return "已过滤 " + st.count + " 段广告";
            case AdFilterStatus.KIND_FILTERING:
                return "正在过滤广告"; // 理论上 READY 前结果已落地, 仅防御兜底
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
        session.reloadCurrentSourceKeepPosition();
        updateAdFilterBadge();
        session.host().renderAdFilterSwitch(session.adFilterOn);
        session.host().showCenterToast(session.adFilterOn ? "广告过滤已开启" : "广告过滤已关闭", 1200);
    }

    /**
     * 媒体列表本地过滤: {@link M3u8FilterCore}(Go 内核 Kotlin 移植)纯函数绝对化+过滤,
     * 结果落地才写 filterAttempted; 剔除数取最大待 STATE_READY 弹一次状态。
     * 内核为纯函数不抛; 防御性 catch 返回原始清单(不过滤, 不阻塞播放)。
     */
    byte[] filterLocallyFirst(String srcUrl, byte[] raw) {
        try {
            M3u8FilterCore.Result r = M3u8FilterCore.rewriteM3u8(
                    new String(raw, java.nio.charset.StandardCharsets.UTF_8), srcUrl, true);
            session.filterAttempted = true; // 结果已出: 在途期间角标显「广告过滤中」
            if (r.getFiltered() > session.pendingFilteredCount) {
                session.pendingFilteredCount = r.getFiltered();
            }
            return r.getText().getBytes(java.nio.charset.StandardCharsets.UTF_8);
        } catch (Throwable t) {
            // 本地内核异常(理论不发生) → 原始清单(不过滤, 不阻塞播放); 服务端过滤已下线
            return raw;
        }
    }

    /** 埋点: 记录本层清单的路径决策(tag/格式见 {@link AdFilterTrace}); 网络耗时由真正的出口各自记录。 */
    private static void logTrace(String srcTag, String decision) {
        android.util.Log.i(AdFilterTrace.TAG, AdFilterTrace.layer("parse", decision, 0L, srcTag));
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
     * 自定义 HLS 播放列表解析器 — 抓到 m3u8 后本地过滤, 再交默认解析器.
     *
     * 分层动作: master 主表(无分片)免过滤直通(master-direct, URI 绝对化由 ExoPlayer
     * 自行完成); 只有媒体列表才查 {@link PlayerSession} 的预取缓存(起播后由
     * {@link PlayerPrefetchHelper} 提前过滤好的子表), 命中就直接用 —— 省掉一次过滤调用,
     * 这是"切集不再白等"的关键。2026-10-23: 媒体列表未命中缓存时本地过滤
     * (M3u8FilterCore, Go 内核 Kotlin 移植), 免服务端往返; 2026-10-28 起服务端过滤下线,
     * 本地为唯一路径。
     */
    class FilterPlaylistParser implements ParsingLoadable.Parser<HlsPlaylist> {
        private final ParsingLoadable.Parser<HlsPlaylist> delegate;

        FilterPlaylistParser(ParsingLoadable.Parser<HlsPlaylist> d) {
            this.delegate = d;
        }

        @Override
        public HlsPlaylist parse(Uri uri, InputStream in) throws IOException {
            byte[] data = readAll(in);
            // 记录最近一次解析的 URI: 切集 reset 的竞态防护依据(见 PlayerSession.resetFilterStateForEpisode)。
            // parse 跑在 loader 线程, 可能先于主线程的 transition 回调完成。
            session.lastParsedPlaylistUrl = uri.toString();
            if (session.adFilterOn) {
                // 埋点: 这一层清单(master / 子表)最终走了哪条路 —— 起播慢时用它区分
                // "在等端侧过滤"与"本来就不需要过滤"。
                final String srcTag = AdFilterTrace.srcTag(uri.toString());
                if (!PlayerUrls.hasMediaSegments(
                        new String(data, java.nio.charset.StandardCharsets.UTF_8))) {
                    // master 主表: 无分片可剔, 过滤的全部产出只是"URI 绝对化" —— ExoPlayer
                    // 解析时本来就会做。免过滤直接放行(2026-10-19 加速)。
                    logTrace(srcTag, "master-direct");
                } else {
                    // 媒体列表(分片在这层, 广告也在这层): 先查预取缓存(起播后由
                    // PlayerPrefetchHelper 提前过滤好的子表), 命中就直接用; 未命中走本地过滤。
                    PlayerSession.Prefetched cached = session.takePrefetched(uri.toString());
                    if (cached != null) {
                        data = cached.data;
                        session.filterAttempted = true;
                        // master 层 cnt=0、子表层才 >0; 取最大, 待 STATE_READY 弹一次状态
                        if (cached.filteredCount > session.pendingFilteredCount) {
                            session.pendingFilteredCount = cached.filteredCount;
                        }
                        logTrace(srcTag, "prefetch-hit ads=" + cached.filteredCount);
                    } else {
                        // 本地过滤: M3u8FilterCore(Go 内核 Kotlin 移植)纯函数绝对化+过滤。
                        byte[] f = filterLocallyFirst(uri.toString(), data);
                        data = f;
                        logTrace(srcTag, "local-filter out=" + data.length);
                    }
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
