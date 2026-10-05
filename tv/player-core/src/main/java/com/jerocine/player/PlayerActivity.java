package com.jerocine.player;

import android.content.Context;
import android.content.Intent;
import android.os.Bundle;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.ImageView;
import android.widget.ProgressBar;
import android.widget.TextView;

import androidx.activity.OnBackPressedCallback;
import androidx.annotation.NonNull;
import androidx.annotation.OptIn;
import androidx.appcompat.app.AppCompatActivity;
import androidx.media3.common.MediaItem;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.PlaybackParameters;
import androidx.media3.common.Player;
import androidx.media3.common.VideoSize;
import androidx.media3.common.util.UnstableApi;
import androidx.media3.database.StandaloneDatabaseProvider;
import androidx.media3.datasource.DataSource;
import androidx.media3.datasource.HttpDataSource;
import androidx.media3.datasource.cache.CacheDataSource;
import androidx.media3.datasource.cache.LeastRecentlyUsedCacheEvictor;
import androidx.media3.datasource.cache.SimpleCache;
import androidx.media3.datasource.okhttp.OkHttpDataSource;
import androidx.media3.exoplayer.DefaultLoadControl;
import androidx.media3.exoplayer.DefaultRenderersFactory;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.LoadControl;
import androidx.media3.exoplayer.hls.HlsMediaSource;
import androidx.media3.ui.PlayerView;

import org.json.JSONObject;

import java.io.File;
import java.util.Locale;
import java.util.concurrent.TimeUnit;

import okhttp3.OkHttpClient;

/**
 * Jerocine 原生视频播放器(公共模块).
 *
 * 启动模式:
 *   A) 单 URL: EXTRA_URL + EXTRA_TITLE
 *   B) 单源 playlist: EXTRA_PLAYLIST_URLS + EXTRA_PLAYLIST_TITLES + EXTRA_START_INDEX
 *   C) 多源(v3): EXTRA_SOURCES_JSON + EXTRA_CURRENT_SOURCE_ID + EXTRA_START_INDEX
 *
 * 缓存: SimpleCache 1GB LRU(壳缓存目录下 video_cache)
 * 倍速: 0.5 / 1 / 1.25 / 1.5 / 2 / 3
 * 遥控: ←→ ±10s(长按渐进步进) / Enter 播暂 / Menu·Info 控制面板 / Back 双击退出
 *
 * 本类只做三件事: ① 装配视图与 helper; ② 实现 {@link PlayerSession.Host} 的 UI 出口;
 * ③ 向 {@link PlayerControl} 注册自身 / 上报播放结局.
 * 播放状态与跨 helper 操作都在 {@link PlayerSession}.
 */
@OptIn(markerClass = UnstableApi.class)
public class PlayerActivity extends AppCompatActivity implements PlayerSession.Host {

    public static final String EXTRA_URL = "url";
    public static final String EXTRA_TITLE = "title";
    public static final String EXTRA_PLAYLIST_URLS = "playlist_urls";
    public static final String EXTRA_PLAYLIST_TITLES = "playlist_titles";
    public static final String EXTRA_START_INDEX = "start_index";
    public static final String EXTRA_RESUME_MS = "resume_ms";
    public static final String EXTRA_SKIP_INTRO_MS = "skip_intro_ms";
    public static final String EXTRA_SKIP_OUTRO_MS = "skip_outro_ms";
    public static final String EXTRA_AUTO_NEXT = "auto_next";
    public static final String EXTRA_FILM_ID = "film_id";
    public static final String EXTRA_FILM_NAME = "film_name";
    public static final String EXTRA_SOURCES_JSON = "sources_json";
    public static final String EXTRA_CURRENT_SOURCE_ID = "current_source_id";
    public static final String EXTRA_PROXY_BASE = "proxy_base";
    public static final String EXTRA_CACHE_DIR = "cache_dir";

    /** 播放事件回调 — 壳层实现并设置, 播放器在关键节点触发. */
    public interface PlayerEventCallback {
        void onPlayerEvent(String name, JSONObject payload);
    }

    static void emit(String name, JSONObject payload) {
        JerocinePlayer.dispatch(name, payload);
    }

    /** 改当前播放器倍速 — 由 {@link PlayerControl#setSpeed} 在主线程回调. */
    void applyPlaybackSpeed(float speed) {
        ExoPlayer p = session == null ? null : session.player;
        if (p != null) p.setPlaybackParameters(new PlaybackParameters(speed));
    }

    private static final long CACHE_SIZE = 1024L * 1024L * 1024L; // 1GB
    private static final long PROGRESS_TICK_MS = 5000L;

    /** 进程内单例, 避免重开时 "Another SimpleCache instance" 报错 — 进程级资源, 必须静态. */
    private static SimpleCache sCache;
    /** sCache 当前指向的缓存目录(与 intent 期望目录不一致时重建, 支持离线播放切到下载缓存区). */
    private static File sCacheDir;

    /** 当前播放缓存实例(与播放器共用; BufferPrefetcher 写缓存缓冲用, 同包访问). */
    static SimpleCache cacheInstance() {
        return sCache;
    }

    private final Handler toastHandler = new Handler(Looper.getMainLooper());
    private final Handler iconHandler = new Handler(Looper.getMainLooper());
    private final Handler progressHandler = new Handler(Looper.getMainLooper());

    /** 最近一次播放状态 — onDestroy 判断是否自然播完 */
    private int lastPlayerState = -1;

    private PlayerSession session;
    private PlayerView playerView;
    private ProgressBar bufferSpinner;
    private TextView titleText;
    private TextView episodesCount;
    private TextView resolutionBadge;
    private Button adFilterButton;
    /** 开关类按钮左上角的状态点(绿=开/灰=关) — 叠在按钮上的兄弟 View. */
    private View dotAdFilter;
    private View dotSpeed;
    private View dotSkip;
    private TextView speedText;
    private TextView adFilterBadge;
    private TextView centerToast;
    private ImageView centerIcon;

    private PlayerAdFilterHelper adFilterHelper;
    private PlayerNetworkModeHelper networkModeHelper;
    private PlayerSkipHelper skipHelper;
    private PlayerGestureHelper gestureHelper;
    private PlayerDialogHelper dialogHelper;
    private PlayerKeyEventHelper keyEventHelper;
    private PlayerSourceHelper sourceHelper;
    private PlayerPrefetchHelper prefetchHelper;
    /** 缓存缓冲(网络差预取, 更多菜单触发); 退出时 cancel. */
    private BufferPrefetcher bufferPrefetcher;

    // ============================ 生命周期 ============================

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        PlayerControl.get().attach(this);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        setContentView(R.layout.activity_player);
        hideSystemUi();

        session = new PlayerSession(this);

        playerView = findViewById(R.id.player_view);
        bufferSpinner = findViewById(R.id.buffer_spinner);
        titleText = findViewById(R.id.title_text);
        episodesCount = findViewById(R.id.episodes_count);
        resolutionBadge = findViewById(R.id.resolution_badge);
        centerToast = findViewById(R.id.center_toast);
        centerIcon = findViewById(R.id.center_icon);

        speedText = findViewById(R.id.speed_text);
        adFilterBadge = findViewById(R.id.ad_filter_badge);

        // helper 之间互不引用: 只依赖 session(状态) + session.host()(UI 出口)
        skipHelper = new PlayerSkipHelper(session);
        adFilterHelper = new PlayerAdFilterHelper(this, session);
        networkModeHelper = new PlayerNetworkModeHelper(this, session);
        gestureHelper = new PlayerGestureHelper(session);
        dialogHelper = new PlayerDialogHelper(session);
        keyEventHelper = new PlayerKeyEventHelper(session);

        // AndroidX OnBackPressedDispatcher 接管返回(predictive back / 系统左滑手势 /
        // 导航栏返回都走这里), 必须注册 callback —— 否则左滑手势返回会被 dispatcher
        // 默认直接 finish() 退出播放器, 绕过 keyEventHelper.handleBack 的
        // "控制条收起 → 双击确认退出"逻辑(与遥控器 BACK 行为不一致).
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                keyEventHelper.handleBack();
            }
        });
        sourceHelper = new PlayerSourceHelper(session);
        prefetchHelper = new PlayerPrefetchHelper(session);

        session.proxyBase = adFilterHelper.resolveProxyBase(getIntent());
        session.adFilterOn = adFilterHelper.prefs().getBoolean("ad_filter_enabled", true);
        networkModeHelper.applyPersisted(); // 中转默认关(分片直连), 只有用户主动开过才为 true
        adFilterHelper.updateAdFilterBadge();

        View titleBar = findViewById(R.id.title_bar);
        // 面板可见时: ① 标题栏跟随显隐 ② 进度条默认获焦(用户期望"面板出→进度条选中")
        playerView.setControllerVisibilityListener((PlayerView.ControllerVisibilityListener) v -> {
            titleBar.setVisibility(v);
            // 控制面板根节点在 PlayerView 内部(懒加载), 每次显隐时现取, 避免 onCreate 时为空
            View controlsRoot = playerView.findViewById(R.id.player_controls_root);
            if (controlsRoot != null) controlsRoot.setVisibility(v);
            if (v == View.VISIBLE) {
                // "过滤"/"倍速"/"跳过"按钮(含左上角状态点)在 PlayerView 的控制视图里(懒加载): 面板显示时取到
                adFilterButton = playerView.findViewById(R.id.btn_ad_filter);
                dotAdFilter = playerView.findViewById(R.id.dot_ad_filter);
                dotSpeed = playerView.findViewById(R.id.dot_speed);
                dotSkip = playerView.findViewById(R.id.dot_skip);
                dialogHelper.bindControlButtons();
                bindMoreMenu();
                renderAdFilterSwitch(session.adFilterOn);
                renderSpeedDot(isSpeedOn());
                renderSkipDot(session.skipEnabled);
                playerView.post(() -> {
                    View prog = playerView.findViewById(androidx.media3.ui.R.id.exo_progress);
                    if (prog != null) prog.requestFocus();
                });
            }
        });
        // 切集/缓冲/暂停时不自动弹面板 — 仅用户显式唤起(确认键/点击)才显示
        playerView.setControllerAutoShow(false);
        playerView.setOnTouchListener(gestureHelper::onPlayerTouch);

        applySkipSettingsFromIntent(getIntent());

        initPlayer();
        sourceHelper.startFromIntent(getIntent());
    }

    /** 账号记忆: 壳层按 mid 从账号/本地取跳过秒数传来(默认 90/60, 关闭则传 0); 值 > 0 即视为已开启. */
    private void applySkipSettingsFromIntent(Intent intent) {
        long iMs = intent.getLongExtra(EXTRA_SKIP_INTRO_MS, 0L);
        long oMs = intent.getLongExtra(EXTRA_SKIP_OUTRO_MS, 0L);
        session.skipIntroMs = iMs > 0 ? iMs : PlayerSkipHelper.DEFAULT_SKIP_INTRO_MS;
        session.skipOutroMs = oMs > 0 ? oMs : PlayerSkipHelper.DEFAULT_SKIP_OUTRO_MS;
        session.skipEnabled = (iMs > 0 || oMs > 0);
        session.autoNext = intent.getBooleanExtra(EXTRA_AUTO_NEXT, true);
    }

    private void initPlayer() {
        DataSource.Factory cacheFactory = buildCacheFactory();
        // 端侧混合广告过滤: 全 HLS 内容用自定义播放列表解析器, 抓到 m3u8 后送服务端剔除广告再解析.
        HlsMediaSource.Factory msFactory = new HlsMediaSource.Factory(cacheFactory)
                .setPlaylistParserFactory(adFilterHelper.new FilterPlaylistParserFactory())
                .setAllowChunklessPreparation(true);
        // 解码: 硬解吃不消时回退软解; 异步队列送解码(全机型强制开)
        DefaultRenderersFactory renderersFactory = new DefaultRenderersFactory(this)
                .setEnableDecoderFallback(true)
                .forceEnableMediaCodecAsynchronousQueueing();
        // 缓冲: 起播阈值 1.5s, 卡顿后 2.5s 恢复; 稳态 30s / 上限 120s; 留 30s 回看缓冲
        LoadControl loadControl = new DefaultLoadControl.Builder()
                .setBufferDurationsMs(30_000, 120_000, 1_500, 2_500)
                .setPrioritizeTimeOverSizeThresholds(true)
                .setBackBuffer(30_000, true)
                .build();
        final ExoPlayer player = new ExoPlayer.Builder(this, renderersFactory)
                .setMediaSourceFactory(msFactory)
                .setLoadControl(loadControl)
                .build();
        session.player = player;
        playerView.setPlayer(player);

        player.addListener(new Player.Listener() {
            @Override
            public void onPlaybackStateChanged(int state) {
                lastPlayerState = state;
                bufferSpinner.setVisibility(state == Player.STATE_BUFFERING ? View.VISIBLE : View.GONE);
                updateFilterLoadingText(state);
                if (state == Player.STATE_READY) {
                    session.episodeSwitching = false;
                    if (!session.introSkippedForCurrent) {
                        skipHelper.applySkipIntro();
                        session.introSkippedForCurrent = true;
                    }
                    // 起播弹一次"过滤状态"(每集一次)
                    if (!session.filterToastShownForEpisode) {
                        session.filterToastShownForEpisode = true;
                        adFilterHelper.showFilterStatus();
                    }
                }
            }

            /** 分辨率实时更新: 码率切换/切集后都会回调; 未就绪宽高为 0 → 隐藏. */
            @Override
            public void onVideoSizeChanged(@NonNull VideoSize videoSize) {
                if (resolutionBadge == null) return;
                String label = PlayerQuality.resolutionLabel(videoSize.width, videoSize.height);
                if (label == null) {
                    resolutionBadge.setVisibility(View.GONE);
                } else {
                    resolutionBadge.setText(label);
                    resolutionBadge.setVisibility(View.VISIBLE);
                }
            }

            @Override
            public void onPlayWhenReadyChanged(boolean playWhenReady, int reason) {
                if (session.player == null) return;
                if (!playWhenReady) {
                    // 暂停: 中央暂停图标持久显示 + 弹控制面板
                    if (session.player.getPlaybackState() == Player.STATE_READY) {
                        showCenterIconPersistent(R.drawable.ic_pause);
                        if (playerView != null) playerView.showController();
                    }
                } else {
                    showCenterIcon(R.drawable.ic_play, 600);
                }
            }

            @Override
            public void onMediaItemTransition(MediaItem mediaItem, int reason) {
                session.introSkippedForCurrent = false;
                session.outroPromptShown = false;
                session.episodeSwitching = true;
                session.resetFilterStateForEpisode();
                updateTitleForCurrent();
                session.updateNetworkModeUi();
                try {
                    JSONObject p = new JSONObject();
                    p.put("filmId", filmId());
                    p.put("episodeIndex", session.player.getCurrentMediaItemIndex());
                    p.put("fromIndex", session.lastMediaItemIndex);
                    session.lastMediaItemIndex = session.player.getCurrentMediaItemIndex();
                    p.put("source", session.currentSourceLabel());
                    p.put("reason", reason);
                    emit("playerEpisodeChange", p);
                } catch (Exception ignore) {
                }
            }

            @Override
            public void onPlayerError(@NonNull PlaybackException error) {
                PlayerControl.get().markPlaybackFailed();
                String currentUrl = "";
                try {
                    if (session.player != null && session.player.getCurrentMediaItem() != null
                            && session.player.getCurrentMediaItem().localConfiguration != null) {
                        currentUrl = String.valueOf(
                                session.player.getCurrentMediaItem().localConfiguration.uri);
                    }
                } catch (Exception ignore) {
                }
                String failedUrl = failedRequestUrl(error);
                final int errIdx = session.player != null
                        ? session.player.getCurrentMediaItemIndex() : -1;

                // 直连失败(端侧过滤下设备自己抓清单/分片) → 本集改走全量中转(自愈).
                // 前两版只在"当前是 proxy 清单"时才自愈; 端侧混合过滤成为主路径后当前地址是**原始 m3u8**,
                // 所以判据放宽为"失败的不是代理请求本身"(见 PlayerUrls.shouldRetryWithRelay).
                // 服务端抓不到该源的(proxyUsable=false)中转也无意义, 直接走下面的报错/回退.
                if (session.adFilterOn && session.sourceProxyUsable
                        && PlayerUrls.shouldRetryWithRelay(currentUrl, failedUrl)
                        && errIdx >= 0 && errIdx < session.currentRawUrls.size()
                        && !session.forceRelayIdx.contains(errIdx)) {
                    session.forceRelayIdx.add(errIdx);
                    session.forceRawIdx.remove(errIdx);
                    PlayerControl.get().clearPlaybackFailure();
                    session.retryCurrentItem(errIdx, "直连失败, 已切换中转");
                    return;
                }
                // 服务端无法抓取清单时, 仅本集回退原始源(广告不过滤, 但保证能放)
                if (session.adFilterOn && currentUrl.contains("/m3u8/proxy")
                        && !currentUrl.toLowerCase(Locale.US).contains("proxymedia=1")
                        && (failedUrl.isEmpty() || failedUrl.contains("/m3u8/proxy"))
                        && errIdx >= 0 && errIdx < session.currentRawUrls.size()
                        && !session.forceRawIdx.contains(errIdx)) {
                    session.forceRawIdx.add(errIdx);
                    session.forceRelayIdx.remove(errIdx);
                    PlayerControl.get().clearPlaybackFailure();
                    session.retryCurrentItem(errIdx, "清单代理失败, 已切换直连");
                    return;
                }

                String causeDetail = "";
                try {
                    Throwable cause = error.getCause();
                    while (cause != null) {
                        causeDetail += "\n  ↳ " + cause.getClass().getSimpleName() + ": "
                                + (cause.getMessage() == null ? "" : cause.getMessage());
                        cause = cause.getCause();
                    }
                } catch (Exception ignore) {
                }
                try {
                    JSONObject p = new JSONObject();
                    p.put("code", error.errorCode);
                    p.put("errorCodeName", error.getErrorCodeName());
                    p.put("message", error.getMessage() == null ? "" : error.getMessage());
                    p.put("currentUrl", currentUrl);
                    p.put("causeDetail", causeDetail);
                    emit("playerError", p);
                } catch (Exception ignore) {
                }
                String urlTail = currentUrl.length() > 60
                        ? "..." + currentUrl.substring(currentUrl.length() - 60)
                        : currentUrl;
                showCenterToast("播放出错 " + error.errorCode + " (" + error.getErrorCodeName() + ")\n"
                        + urlTail + causeDetail, 8000);
                new Handler(Looper.getMainLooper()).postDelayed(() -> {
                    if (!isFinishing()) finish();
                }, 8000);
            }
        });

        skipHelper.startOutroWatcher();
        progressHandler.postDelayed(progressTick, PROGRESS_TICK_MS);
    }

    /** 5s 一次发 playerProgress 给壳层(含 filmId + episodeIndex + position). 不传 duration(仅退出时落). */
    private final Runnable progressTick = new Runnable() {
        @Override
        public void run() {
            try {
                if (session.player != null && session.player.isPlaying()) {
                    maybeRefreshPrefetchNearOutro();
                    if (!skipHelper.inNoRecordTail()) {
                        JSONObject p = new JSONObject();
                        p.put("filmId", filmId());
                        p.put("episodeIndex", session.player.getCurrentMediaItemIndex());
                        p.put("source", session.currentSourceLabel());
                        p.put("position", session.player.getCurrentPosition() / 1000.0);
                        emit("playerProgress", p);
                    }
                }
            } catch (Exception ignore) {
            } finally {
                progressHandler.postDelayed(this, PROGRESS_TICK_MS);
            }
        }
    };

    /**
     * 接近片尾(剩余 ≤ 跳过片尾秒数 + 60s) → 预取下一集的过滤后清单, 让切集不再白等端侧过滤.
     *
     * 这是预取的**唯一触发点**: 不做"起播就预取下一集" —— 预取结果 TTL 只有 10min, 而一集
     * 30~45min, 起播时预取的那份到切集时必然过期。5s 轮询一次调用, 去重节流都在
     * {@link PlayerPrefetchHelper#schedule()} 里(结果还新鲜就直接返回)。
     */
    private void maybeRefreshPrefetchNearOutro() {
        if (!session.autoNext || !session.adFilterOn) return;
        long duration = session.player.getDuration();
        long pos = session.player.getCurrentPosition();
        if (duration <= 0 || pos <= 0) return;
        long remaining = duration - pos;
        if (remaining > PlayerPrefetchHelper.NEAR_OUTRO_LEAD_MS + session.skipOutroMs) return;
        prefetchHelper.schedule();
    }

    private DataSource.Factory buildCacheFactory() {
        // 进程内单例; 若 intent 指定了不同的缓存目录(如离线播放指到下载缓存区),
        // 释放旧实例并重建, 否则继续用播放缓存(video_cache)
        File wantDir = resolveCacheDir();
        if (sCache != null && !wantDir.equals(sCacheDir)) {
            try {
                sCache.release();
            } catch (Exception ignore) {
            }
            sCache = null;
        }
        if (sCache == null) {
            sCacheDir = resolveCacheDir();
            sCache = new SimpleCache(
                    sCacheDir,
                    new LeastRecentlyUsedCacheEvictor(CACHE_SIZE),
                    new StandaloneDatabaseProvider(getApplicationContext())
            );
        }
        // connect/read 用 30s 无数据超时; 不设整次请求总时限, 否则长分片/MP4 会在固定时间被掐断.
        // 壳层可注入自己的媒体客户端(放宽 TLS 兼容老设备), 未注入则自建.
        OkHttpClient injected = JerocinePlayer.mediaClient();
        OkHttpClient.Builder builder = (injected != null)
                ? injected.newBuilder() : new OkHttpClient.Builder();
        OkHttpClient http = builder
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .writeTimeout(30, TimeUnit.SECONDS)
                .callTimeout(0, TimeUnit.SECONDS)
                .retryOnConnectionFailure(true)
                .followRedirects(true)
                .followSslRedirects(true)
                .build();
        OkHttpDataSource.Factory upstream = new OkHttpDataSource.Factory(http)
                .setUserAgent("Jerocine/1.0 (Android TV)");
        return new CacheDataSource.Factory()
                .setCache(sCache)
                .setUpstreamDataSourceFactory(upstream)
                .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR);
    }

    /** 视频缓存目录: 壳层可经 EXTRA_CACHE_DIR 指定, 否则用壳默认缓存目录下 video_cache. */
    private File resolveCacheDir() {
        String extra = getIntent().getStringExtra(EXTRA_CACHE_DIR);
        File dir = (extra != null && !extra.isEmpty())
                ? new File(extra)
                : new File(getCacheDir(), "video_cache");
        if (!dir.exists()) dir.mkdirs();
        return dir;
    }

    /** 从异常链里捞出真正失败的请求 URL(HttpDataSourceException 会带 dataSpec). */
    private static String failedRequestUrl(Throwable error) {
        Throwable cause = error;
        while (cause != null) {
            if (cause instanceof HttpDataSource.HttpDataSourceException) {
                HttpDataSource.HttpDataSourceException httpError =
                        (HttpDataSource.HttpDataSourceException) cause;
                if (httpError.dataSpec != null && httpError.dataSpec.uri != null) {
                    return httpError.dataSpec.uri.toString();
                }
            }
            cause = cause.getCause();
        }
        return "";
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        return keyEventHelper.dispatchKeyEvent(event);
    }

    @Override
    public void onBackPressed() {
        // 安卓系统手势/导航栏返回: 与遥控器 BACK 走同一个 handleBack (控制条收起 → 双击确认退出).
        // 不调 super 默认直接 finish, 否则左滑返回会绕过二次确认.
        if (!keyEventHelper.handleBack()) {
            super.onBackPressed();
        }
    }

    @Override
    protected void onPause() {
        super.onPause();
        // 离开/熄屏/切后台都会走 onPause(比 onDestroy 可靠) → 立即记一次进度
        emitProgressNow();
        if (session.player != null && session.player.isPlaying()) session.player.pause();
    }

    private void emitProgressNow() {
        try {
            if (session.player == null || skipHelper.inNoRecordTail()) return;
            JSONObject p = new JSONObject();
            p.put("filmId", filmId());
            p.put("episodeIndex", session.player.getCurrentMediaItemIndex());
            p.put("source", session.currentSourceLabel());
            p.put("position", session.player.getCurrentPosition() / 1000.0);
            emit("playerProgress", p);
        } catch (Exception ignore) {
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        PlayerControl.get().detach(this);
        skipHelper.stopOutroWatcher();
        prefetchHelper.shutdown(); // 停掉在途预取(否则后台线程还占着 socket 跑一次没人要的过滤)
        if (bufferPrefetcher != null) bufferPrefetcher.cancel(); // 停掉缓存缓冲(已写分片留在缓存)
        toastHandler.removeCallbacksAndMessages(null);
        iconHandler.removeCallbacksAndMessages(null);
        progressHandler.removeCallbacksAndMessages(null);
        final ExoPlayer player = session.player;
        boolean playbackEnded = player != null && lastPlayerState == Player.STATE_ENDED;
        if (playbackEnded || !(player != null && skipHelper.inNoRecordTail())) {
            try {
                JSONObject p = new JSONObject();
                p.put("filmId", filmId());
                if (player != null) {
                    p.put("position", player.getCurrentPosition() / 1000.0);
                    p.put("duration", player.getDuration() > 0 ? player.getDuration() / 1000.0 : 0);
                    p.put("episodeIndex", player.getCurrentMediaItemIndex());
                    p.put("source", session.currentSourceLabel());
                    if (playbackEnded) p.put("ended", true);
                }
                emit("playerClosed", p);
            } catch (Exception ignore) {
            }
        }
        if (player != null) {
            player.release();
            session.player = null;
        }
        // sCache 故意不 release: 进程内复用, 退出时缓存继续保留供下次用
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        // 与 onCreate 一致重读代理 base(漏读会致 relaunch 后 proxyBase 用空值 → 切集"过滤未生效")
        session.proxyBase = adFilterHelper.resolveProxyBase(intent);
        // 与 onCreate 一致地按新片重置跳过设置(含开关), 否则换片会沿用上一片的 skipEnabled
        applySkipSettingsFromIntent(intent);
        session.introSkippedForCurrent = false;
        sourceHelper.startFromIntent(intent);
    }

    private void hideSystemUi() {
        View decor = getWindow().getDecorView();
        decor.setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                        | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
        );
    }

    // ============================ PlayerSession.Host ============================

    @Override
    public Context context() {
        return this;
    }

    @Override
    public void showCenterToast(String msg, long durationMs) {
        // 与中央图标互斥(防重叠)
        if (centerIcon != null) centerIcon.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null);
        centerToast.setText(msg);
        centerToast.setVisibility(View.VISIBLE);
        toastHandler.removeCallbacksAndMessages(null);
        toastHandler.postDelayed(() -> {
            centerToast.setVisibility(View.GONE);
            // 若仍处暂停态, toast 结束后恢复"暂停图标停留"
            if (session.player != null && !session.player.getPlayWhenReady()
                    && session.player.getPlaybackState() == Player.STATE_READY) {
                showCenterIconPersistent(R.drawable.ic_pause);
            }
        }, durationMs);
    }

    @Override
    public void showCenterIcon(int drawableRes, long durationMs) {
        if (centerIcon == null) return;
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
        iconHandler.removeCallbacksAndMessages(null);
        iconHandler.postDelayed(() -> centerIcon.setVisibility(View.GONE), durationMs);
    }

    @Override
    public void showCenterIconPersistent(int drawableRes) {
        if (centerIcon == null) return;
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null);
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
    }

    @Override
    public void hideGestureToast() {
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        if (session.player != null && !session.player.getPlayWhenReady()
                && session.player.getPlaybackState() == Player.STATE_READY) {
            showCenterIconPersistent(R.drawable.ic_pause);
        }
    }

    @Override
    public void finishPlayer() {
        finish();
    }

    @Override
    public void updateTitleForCurrent() {
        int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : 0;
        if (idx >= 0 && idx < session.playlistTitles.size()) {
            titleText.setText(session.playlistTitles.get(idx));
        }
        if (episodesCount != null) {
            int total = session.playlistTitles.size();
            // 仅多集时显示集数角标(单集/单 URL 不显示"共 1 集")
            if (total > 1) {
                episodesCount.setText("共 " + total + " 集");
                episodesCount.setVisibility(View.VISIBLE);
            } else {
                episodesCount.setVisibility(View.GONE);
            }
        }
    }

    @Override
    public void emitEvent(String name, JSONObject payload) {
        emit(name, payload);
    }

    @Override
    public String filmId() {
        String id = getIntent().getStringExtra(EXTRA_FILM_ID);
        return id == null ? "" : id;
    }

    @Override
    public void toggleAdFilter() {
        adFilterHelper.toggleAdFilter();
    }

    @Override
    public void toggleNetworkMode() {
        networkModeHelper.toggle();
    }

    @Override
    public void renderAdFilterSwitch(boolean on) {
        // 底栏"过滤"按钮/状态点是 PlayerView 控制视图里懒加载的, 首次刷新时现取
        if (adFilterButton == null && playerView != null) {
            adFilterButton = playerView.findViewById(R.id.btn_ad_filter);
        }
        if (adFilterButton == null) return;
        // 文案恒定、不变色; 开关状态只由左上角状态点表达
        adFilterButton.setText("去广告");
        if (dotAdFilter == null && playerView != null) {
            dotAdFilter = playerView.findViewById(R.id.dot_ad_filter);
        }
        renderStatusDot(dotAdFilter, on);
    }

    @Override
    public void renderNetworkMode(boolean relay) {
        // 中转按钮已移入右上角「⋮ 更多」菜单: 菜单每次点开重建, 状态文本实时取 session.relayOn,
        // 无需维护常驻按钮引用; 开关切换的即时反馈由 toggle() 里的 centerToast 承担.
    }

    /** 面板显示时绑定右上角「⋮ 更多」菜单(每次现取, 与其它懒加载控件一致). */
    private void bindMoreMenu() {
        Button more = playerView != null ? playerView.findViewById(R.id.btn_more) : null;
        if (more == null) return;
        more.setOnClickListener(v -> showMoreMenu());
    }

    /**
     * 更多菜单 — 收纳非常用项(用户拍板): 下载管理 / 缓存缓冲 / 中转 / 分辨率 / 诊断.
     * 播放器右上角保留原有显示(过滤广告/倍速/分辨率/总集数)不变.
     */
    private void showMoreMenu() {
        final String[] items = {
                "下载管理",
                "缓存缓冲",
                "中转：" + (session.relayOn ? "开" : "关"),
                "分辨率：" + (resolutionBadge != null && resolutionBadge.getVisibility() == View.VISIBLE
                        ? resolutionBadge.getText().toString() : "播放中获取"),
                "诊断信息"
        };
        new android.app.AlertDialog.Builder(this, R.style.JcPlayerDialog)
                .setTitle("更多")
                .setItems(items, (d, i) -> {
                    switch (i) {
                        case 0:
                            openDownloadManager();
                            break;
                        case 1:
                            showBufferDialog();
                            break;
                        case 2:
                            toggleNetworkMode();
                            break;
                        case 3:
                            showCenterToast("当前分辨率 " + items[3].replace("分辨率：", ""), 2000);
                            break;
                        case 4:
                            showDiagnostics();
                            break;
                        default:
                            break;
                    }
                })
                .create().show();
    }

    /** 更多菜单 → 缓存缓冲: 选时长, 暂停播放后后台预取前 N 分钟分片(写播放缓存区). */
    private void showBufferDialog() {
        final double[] options = {5.0, 15.0, 30.0, 0.0};
        final String[] labels = {"缓冲 5 分钟", "缓冲 15 分钟", "缓冲 30 分钟", "缓冲本集"};
        new android.app.AlertDialog.Builder(this, R.style.JcPlayerDialog)
                .setTitle("缓存缓冲")
                .setItems(labels, (d, i) -> startBufferPrefetch(options[i], labels[i]))
                .setNegativeButton("取消", null)
                .create().show();
    }

    /**
     * 启动缓存缓冲(路径 B): 暂停 → BufferPrefetcher 把当前集前 N 分钟分片写进播放缓存区
     * → 完成自动恢复播放(CacheDataSource 命中缓存, 卡顿缓解).
     * minutes=0 表示本集全部; 缓冲中再次进入会先 cancel 旧的再开始新的.
     */
    private void startBufferPrefetch(double minutes, String label) {
        if (session == null || session.player == null) return;
        int idx = session.player.getCurrentMediaItemIndex();
        if (session.currentRawUrls.isEmpty() || idx < 0 || idx >= session.currentRawUrls.size()) {
            showCenterToast("当前集无可缓冲的片源", 1800);
            return;
        }
        if (sCache == null) {
            showCenterToast("缓存区未就绪, 稍后再试", 1800);
            return;
        }
        final boolean playing = session.player.getPlayWhenReady();
        session.player.pause();
        if (bufferPrefetcher != null) bufferPrefetcher.cancel();
        bufferPrefetcher = new BufferPrefetcher(sCache, session.proxyBase);
        final String srcUrl = session.currentRawUrls.get(idx);
        showCenterToast("正在缓冲 " + label + " · 播放暂停", 2200);
        bufferPrefetcher.start(srcUrl, minutes,
                (cached, target) -> {
                    int pct = target > 0 ? (int) (cached * 100.0 / target) : 0;
                    showCenterToast("缓存缓冲 " + Math.min(pct, 100) + "% · " + label, 1200);
                },
                new BufferPrefetcher.CompletionListener() {
                    @Override
                    public void onComplete() {
                        showCenterToast("缓存完成 · 已恢复播放", 2200);
                        if (session.player != null && playing) session.player.setPlayWhenReady(true);
                    }

                    @Override
                    public void onError(String message) {
                        showCenterToast(message, 2400);
                        if (session.player != null && playing) session.player.setPlayWhenReady(true);
                    }
                });
    }

    /** 更多菜单 → 下载管理: 透传影片上下文给 DownloadActivity(选集/下载中/已完成三 tab). */
    private void openDownloadManager() {        Intent i = new Intent(this, com.jerocine.player.download.DownloadActivity.class);
        i.putExtra(EXTRA_FILM_ID, filmId());
        i.putExtra(EXTRA_FILM_NAME, getIntent().getStringExtra(EXTRA_FILM_NAME));
        String sourcesJson = getIntent().getStringExtra(EXTRA_SOURCES_JSON);
        if (sourcesJson != null) i.putExtra(EXTRA_SOURCES_JSON, sourcesJson);
        i.putExtra(EXTRA_PROXY_BASE, session.proxyBase);
        startActivity(i);
    }

    /** 更多菜单 → 诊断信息: 安卓版本/设备/内核/媒体库/代理, 帮助判断播放异常归属. */
    private void showDiagnostics() {
        StringBuilder sb = new StringBuilder();
        sb.append("安卓版本: ").append(Build.VERSION.RELEASE)
                .append(" (API ").append(Build.VERSION.SDK_INT).append(")\n");
        sb.append("设备: ").append(Build.MANUFACTURER).append(" ").append(Build.MODEL).append("\n");
        sb.append("WebView 内核: ").append(webViewVersion()).append("\n");
        sb.append("媒体库: media3 1.4.1 (ExoPlayer)\n");
        sb.append("中转代理: ").append(session.proxyBase == null || session.proxyBase.isEmpty()
                ? "未配置" : session.proxyBase).append("\n");
        sb.append("中转开关: ").append(session.relayOn ? "开" : "关").append("\n");
        sb.append("广告过滤: ").append(session.adFilterOn ? "开" : "关").append("\n");
        sb.append("当前片源: ").append(session.currentSourceIndex + 1).append("/")
                .append(session.sourceList.size());
        new android.app.AlertDialog.Builder(this, R.style.JcPlayerDialog)
                .setTitle("诊断信息")
                .setMessage(sb.toString())
                .setPositiveButton("知道了", null)
                .create().show();
    }

    /** WebView 内核版本(用 UA 提取, 不启动 WebView 实例). */
    private String webViewVersion() {
        try {
            String ua = android.webkit.WebSettings.getDefaultUserAgent(this);
            if (ua != null && ua.contains("Chrome/")) {
                int s = ua.indexOf("Chrome/") + 7;
                int e = ua.indexOf(' ', s);
                if (e < 0) e = ua.length();
                return "Chromium " + ua.substring(s, e);
            }
            return ua == null ? "未知" : ua;
        } catch (Exception e) {
            return "未知";
        }
    }

    /**
     * "正在准备视频 · 广告过滤中"提示: 仅起播/切集加载期(缓冲中且广告过滤开启)显示,
     * 播放就绪或非缓冲状态隐藏. 与 web 播放器 loading 文案对齐.
     */
    private void updateFilterLoadingText(int state) {
        View tv = findViewById(R.id.filter_loading_text);
        if (tv == null) return;
        boolean show = state == Player.STATE_BUFFERING
                && session.episodeSwitching
                && session.adFilterOn;
        tv.setVisibility(show ? View.VISIBLE : View.GONE);
    }

    /** 状态点: 开=绿点, 关=灰点. */
    private void renderStatusDot(View dot, boolean on) {
        if (dot == null) return;
        dot.setBackgroundResource(on ? R.drawable.jc_status_dot_on : R.drawable.jc_status_dot_off);
    }

    /** 当前播放倍速是否 ≠ 1.0(非 1.0 即算"倍速开启", 含手势长按临时 2x). */
    private boolean isSpeedOn() {
        return session.player != null
                && Math.abs(session.player.getPlaybackParameters().speed - 1f) > 0.001f;
    }

    @Override
    public void renderSpeedDot(boolean on) {
        if (dotSpeed == null && playerView != null) {
            dotSpeed = playerView.findViewById(R.id.dot_speed);
        }
        renderStatusDot(dotSpeed, on);
    }

    @Override
    public void renderSkipDot(boolean on) {
        if (dotSkip == null && playerView != null) {
            dotSkip = playerView.findViewById(R.id.dot_skip);
        }
        renderStatusDot(dotSkip, on);
    }

    @Override
    public PlayerView playerView() {
        return playerView;
    }

    @Override
    public void renderAdFilterBadge(AdFilterStatus status) {
        if (adFilterBadge == null) return;
        if (status == null) {
            adFilterBadge.setVisibility(View.GONE);
            return;
        }
        int dot;
        if (status.tone == AdFilterStatus.Tone.BLUE) {
            dot = R.drawable.jc_badge_dot_busy;
        } else if (status.tone == AdFilterStatus.Tone.GRAY) {
            dot = R.drawable.jc_badge_dot_idle;
        } else {
            dot = R.drawable.jc_badge_dot_ok;
        }
        adFilterBadge.setText(status.text);
        adFilterBadge.setCompoundDrawablesRelativeWithIntrinsicBounds(dot, 0, 0, 0);
        adFilterBadge.setVisibility(View.VISIBLE);
    }

    @Override
    public void renderSpeedText(String label) {
        if (speedText != null) speedText.setText(label);
    }

    @Override
    public boolean dispatchToSuper(KeyEvent event) {
        return super.dispatchKeyEvent(event);
    }
}
