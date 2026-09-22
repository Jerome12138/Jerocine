package com.jerocine.player;

import android.content.Context;
import android.content.Intent;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowManager;
import android.widget.ImageView;
import android.widget.ProgressBar;
import android.widget.TextView;

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
 * 本类只做两件事: ① 装配视图与 helper; ② 实现 {@link PlayerSession.Host} 的 UI 出口.
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

    public static void setCallback(PlayerEventCallback l) {
        JerocinePlayer.setCallback(l);
    }

    static void emit(String name, JSONObject payload) {
        JerocinePlayer.dispatch(name, payload);
    }

    /** 当前运行中的 PlayerActivity 单例引用(用于壳层 stop / setSpeed). */
    private static volatile PlayerActivity sCurrentInstance;

    public static void stopRunningInstance() {
        PlayerActivity inst = sCurrentInstance;
        if (inst != null) inst.runOnUiThread(inst::finish);
    }

    public static void setSpeedOnRunningInstance(float speed) {
        PlayerActivity inst = sCurrentInstance;
        if (inst != null) {
            inst.runOnUiThread(() -> {
                ExoPlayer p = inst.session == null ? null : inst.session.player;
                if (p != null) p.setPlaybackParameters(new PlaybackParameters(speed));
            });
        }
    }

    private static final long CACHE_SIZE = 1024L * 1024L * 1024L; // 1GB
    private static final long PROGRESS_TICK_MS = 5000L;

    /** 进程内单例, 避免重开时 "Another SimpleCache instance" 报错. */
    private static SimpleCache sCache;

    /** 跨 Activity 的播放失败标志: 报错时置 true, finish 后壳层 onResume 检查并通知前端 fallback. */
    public static volatile boolean sLastPlaybackFailed = false;

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
    private TextView centerToast;
    private ImageView centerIcon;

    private PlayerAdFilterHelper adFilterHelper;
    private PlayerSkipHelper skipHelper;
    private PlayerGestureHelper gestureHelper;
    private PlayerDialogHelper dialogHelper;
    private PlayerKeyEventHelper keyEventHelper;
    private PlayerSourceHelper sourceHelper;

    // ============================ 生命周期 ============================

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        sCurrentInstance = this;
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

        session.playerView = playerView;
        session.speedText = findViewById(R.id.speed_text);
        session.adFilterBadge = findViewById(R.id.ad_filter_badge);
        session.networkModeBadge = findViewById(R.id.network_mode_badge);

        // helper 之间互不引用: 只依赖 session(状态) + session.host()(UI 出口)
        skipHelper = new PlayerSkipHelper(session);
        adFilterHelper = new PlayerAdFilterHelper(this, session);
        gestureHelper = new PlayerGestureHelper(session);
        dialogHelper = new PlayerDialogHelper(session);
        keyEventHelper = new PlayerKeyEventHelper(session);
        sourceHelper = new PlayerSourceHelper(session);

        session.proxyBase = adFilterHelper.resolveProxyBase(getIntent());
        session.adFilterOn = adFilterHelper.prefs().getBoolean("ad_filter_enabled", true);
        adFilterHelper.updateAdFilterBadge();

        View titleBar = findViewById(R.id.title_bar);
        // 面板可见时: ① 标题栏跟随显隐 ② 进度条默认获焦(用户期望"面板出→进度条选中")
        playerView.setControllerVisibilityListener((PlayerView.ControllerVisibilityListener) v -> {
            titleBar.setVisibility(v);
            // 控制面板根节点在 PlayerView 内部(懒加载), 每次显隐时现取, 避免 onCreate 时为空
            View controlsRoot = playerView.findViewById(R.id.player_controls_root);
            if (controlsRoot != null) controlsRoot.setVisibility(v);
            if (v == View.VISIBLE) {
                dialogHelper.bindControlButtons();
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
                sLastPlaybackFailed = true;
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

                // 直连分片失败 → 本集改走全量中转(自愈)
                if (session.adFilterOn && PlayerUrls.shouldRetryWithRelay(currentUrl, failedUrl)
                        && errIdx >= 0 && errIdx < session.currentRawUrls.size()
                        && !session.forceRelayIdx.contains(errIdx)) {
                    session.forceRelayIdx.add(errIdx);
                    session.forceRawIdx.remove(errIdx);
                    sLastPlaybackFailed = false;
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
                    sLastPlaybackFailed = false;
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
                if (session.player != null && session.player.isPlaying()
                        && !skipHelper.inNoRecordTail()) {
                    JSONObject p = new JSONObject();
                    p.put("filmId", filmId());
                    p.put("episodeIndex", session.player.getCurrentMediaItemIndex());
                    p.put("source", session.currentSourceLabel());
                    p.put("position", session.player.getCurrentPosition() / 1000.0);
                    emit("playerProgress", p);
                }
            } catch (Exception ignore) {
            } finally {
                progressHandler.postDelayed(this, PROGRESS_TICK_MS);
            }
        }
    };

    private DataSource.Factory buildCacheFactory() {
        if (sCache == null) {
            File cacheDir = resolveCacheDir();
            sCache = new SimpleCache(
                    cacheDir,
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
        if (sCurrentInstance == this) sCurrentInstance = null;
        skipHelper.stopOutroWatcher();
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
    public void showPlayMenu() {
        dialogHelper.showPlayMenu();
    }

    @Override
    public void toggleAdFilter() {
        adFilterHelper.toggleAdFilter();
    }

    @Override
    public boolean dispatchToSuper(KeyEvent event) {
        return super.dispatchKeyEvent(event);
    }
}
