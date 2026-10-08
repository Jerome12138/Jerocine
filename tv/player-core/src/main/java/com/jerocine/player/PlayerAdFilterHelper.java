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
 * 广告过滤 — 端侧混合过滤: 抓到 m3u8 后送 /v1/m3u8/filter 剔广告再交默认解析器.
 *
 * 这是**主路径**(与 web 播放页一致): 默认不把清单包成服务端代理, 由设备自己抓清单 ——
 * 少一跳服务器回源(起播更快), 且服务端抓不到的源(源站拒机房 IP)也能正常过滤。
 * 端侧失败时由 {@link #escalateToProxy()} 升级为服务端 /v1/m3u8/proxy。
 *
 * 状态(开关/代理地址/线路/统计)全部在 {@link PlayerSession}; 本类只负责过滤动作与持久化,
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
        session.forceProxyIdx.clear();
        session.reloadCurrentSourceKeepPosition();
        updateAdFilterBadge();
        session.host().renderAdFilterSwitch(session.adFilterOn);
        session.host().showCenterToast(session.adFilterOn ? "广告过滤已开启" : "广告过滤已关闭", 1200);
    }

    /**
     * 同步 POST 原始 m3u8 到 /v1/m3u8/filter, 返回过滤后字节; 失败返回 null(用原始, 优雅降级).
     * 在 loader 线程调用. 网络/重试细节在 {@link M3u8FilterClient}, 这里只维护 session 过滤状态.
     */
    byte[] filterViaServer(String srcUrl, byte[] raw) {
        session.filterAttempted = true;
        M3u8FilterClient.Result r = M3u8FilterClient.filter(session.proxyBase, srcUrl, raw);
        if (r == null) {
            session.filterFailed = true;
            return null;
        }
        // master 表 cnt=0、子表才 cnt>0; 取最大, 待 STATE_READY 弹一次状态
        if (r.filteredCount > session.pendingFilteredCount) {
            session.pendingFilteredCount = r.filteredCount;
        }
        return r.data;
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
     * 端侧过滤失败 → 把**本集**升级为服务端清单代理(与 web resolvePlaySrc 的"降级1"一致)。
     *
     * 为什么要有这一步: 端侧混合过滤是主路径, 它挂掉时不能就这么放着广告播 —— 若服务端抓得到
     * 这个源(proxyUsable), 就换成 /v1/m3u8/proxy 由服务器抓 + 过滤。
     *
     * 防打转(必须): ① 服务端抓不到该源时不升级(包了也是 500); ② 本集已被"回退直连"(forceRawIdx)
     * 或已升级过(forceProxyIdx)则不再动 —— 否则"直连失败 → 代理 → 代理失败 → 直连"会来回切换。
     * parse 跑在 loader 线程, 读当前集索引/重建播放器必须回主线程。
     */
    private void escalateToProxy(final String failedUrl) {
        if (!session.sourceProxyUsable) return;
        if (session.proxyBase == null || session.proxyBase.isEmpty()) return;
        // 【关键】在**调用点(loader 线程)**解析失败的是哪一集, 不能在 post 里读"当前索引"。
        // 原实现在 post 里才读 getCurrentMediaItemIndex(): 第5 集起播 → 过滤超时 8s →
        // 用户在这期间按▼切到第 6 集 → post 执行时索引已是 5 → **第 6 集被无端标记
        // forceProxy 并 replaceMediaItem 重建**(打断用户已在看的这一集), 并连带
        // invalidatePrefetch() 把第 6 集已预取好的清单一起清掉。真正失败的第 5 集反被漏标。
        final int failedIdx = indexOfRawUrl(failedUrl);
        if (failedIdx < 0) return;
        new Handler(Looper.getMainLooper()).post(() -> {
            if (session.player == null) return;
            // 用户已经切走了就别动手: 修复"打错集"的最后一层保险
            if (session.player.getCurrentMediaItemIndex() != failedIdx) return;
            if (session.forceRawIdx.contains(failedIdx) || session.forceProxyIdx.contains(failedIdx)) {
                return;
            }
            session.forceProxyIdx.add(failedIdx);
            // 本片源端侧过滤已证明不可靠 → 后续集直接用代理, 免得每集都白等一轮超时
            session.sourcePreferProxy = true;
            // 后续集都走代理了, 已预取/在途的端侧清单缓存全部作废(也顺手停掉在途任务)
            session.invalidatePrefetch();
            session.retryCurrentItem(failedIdx, "端侧过滤失败, 已切换服务端过滤");
        });
    }

    /** 在当前播放列表里找该URL 对应的集索引; 找不到返回 -1. */
    private int indexOfRawUrl(String rawUrl) {
        if (rawUrl == null) return -1;
        List<String> urls = session.currentRawUrls;
        for (int i = 0; i < urls.size(); i++) {
            if (rawUrl.equals(urls.get(i))) return i;
        }
        return -1;
    }

    /**
     * 自定义 HLS 播放列表解析器 — 抓到 m3u8 后送 /v1/m3u8/filter 剔广告, 再交默认解析器.
     *
     * 先查 {@link PlayerSession} 的预取缓存(起播后由 {@link PlayerPrefetchHelper} 提前过滤好的
     * master + 子表): 命中就直接用, 省掉一次 1.4~5.1s 的 POST —— 这是"切集不再白等"的关键。
     */
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
                    // 预取命中: 起播后已把下一集(master + 子表)过滤好放缓存 → 直接用, 不再付 POST 代价
                    PlayerSession.Prefetched cached = session.takePrefetched(uri.toString());
                    if (cached != null) {
                        data = cached.data;
                        session.filterAttempted = true;
                        // master 层 cnt=0、子表层才 >0; 取最大, 待 STATE_READY 弹一次状态
                        if (cached.filteredCount > session.pendingFilteredCount) {
                            session.pendingFilteredCount = cached.filteredCount;
                        }
                    } else {
                        byte[] f = filterViaServer(uri.toString(), data);
                        if (f != null) {
                            data = f;
                        } else {
                            escalateToProxy(uri.toString()); // 端侧失败 → 本集升级服务端代理(带失败 URL 以定位是哪一集)
                        }
                    }
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
