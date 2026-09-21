package com.jerocine.player;

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

import androidx.annotation.OptIn;
import androidx.appcompat.app.AppCompatActivity;
import androidx.media3.common.MediaItem;
import androidx.media3.common.PlaybackParameters;
import androidx.media3.common.Player;
import androidx.media3.common.util.UnstableApi;
import androidx.media3.database.StandaloneDatabaseProvider;
import androidx.media3.datasource.DataSource;
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

import java.io.File;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;

import okhttp3.OkHttpClient;

/**
 * Jerocine 原生视频播放器.
 *
 * 支持模式:
 *   A) 单 URL: EXTRA_URL + EXTRA_TITLE
 *   B) Playlist: EXTRA_PLAYLIST_URLS + EXTRA_PLAYLIST_TITLES + EXTRA_START_INDEX
 *      → 自动续集 (用 setMediaItems), 切集时自动跳片头, 接近片尾时自动跳下一集
 *
 * 缓存: SimpleCache 1GB LRU (app 内部 ~/cache/video_cache)
 * 倍速: 0.5 / 1 / 1.25 / 1.5 / 2 / 3
 * 遥控:
 *   ←/→  ±10s   长按 ±30s
 *   Enter/Center/MediaPlayPause: 播暂
 *   Menu/Info: 倍速菜单
 *   Back: 退出
 */
@OptIn(markerClass = UnstableApi.class)
public class PlayerActivity extends AppCompatActivity {

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

    /** native 内部事件回调接口, MainActivity 实现并设置, PlayerActivity 触发 */
    public interface PlayerEventCallback {
        void onPlayerEvent(String name, org.json.JSONObject payload);
    }
    public static volatile PlayerEventCallback sCallback;
    public static void setCallback(PlayerEventCallback l) { sCallback = l; }
    public static void emit(String name, org.json.JSONObject payload) {
        PlayerEventCallback l = sCallback;
        if (l != null) {
            try { l.onPlayerEvent(name, payload); } catch (Exception ignore) {}
        }
    }

    /** 当前运行中的 PlayerActivity 单例引用 (用于 bridge stop/setSpeed) */
    private static volatile PlayerActivity sCurrentInstance;
    public static void stopRunningInstance() {
        PlayerActivity inst = sCurrentInstance;
        if (inst != null) inst.runOnUiThread(inst::finish);
    }
    public static void setSpeedOnRunningInstance(float speed) {
        PlayerActivity inst = sCurrentInstance;
        if (inst != null) {
            inst.runOnUiThread(() -> {
                if (inst.player != null) {
                    inst.player.setPlaybackParameters(new PlaybackParameters(speed));
                }
            });
        }
    }

    private static final long CACHE_SIZE = 1024L * 1024L * 1024L; // 1GB
    private static final long SEEK_SHORT_MS = 10_000L;
    private static final long SEEK_LONG_MS = 30_000L;

    /** 按视频实际宽高给出展示画质(行业通用阈值, 与源站标称无直接关系) */
    static String resolutionLabel(int width, int height) {
        if (width <= 0 || height <= 0) return null;
        if (width >= 1800 || height >= 950) return "1080P";
        if (width >= 1200 || height >= 680) return "720P";
        if (width >= 800 || height >= 460) return "480P";
        return "标清";
    }

    /** 进程内单例, 避免重开 PlayerActivity 时 "Another SimpleCache instance" 报错 */
    private static SimpleCache sCache;

    /**
     * 跨 Activity 的播放失败 flag.
     * PlayerActivity 报错时置 true, finish() 后 MainActivity.onResume 检查并通知前端 fallback.
     */
    public static volatile boolean sLastPlaybackFailed = false;

    // ===== 播放器核心字段 =====
    ExoPlayer player;
    PlayerView playerView;
    ProgressBar bufferSpinner;
    TextView titleText;
    TextView speedText;
    TextView episodesCount;
    TextView resolutionBadge;
    TextView centerToast;
    ImageView centerIcon;

    private final Handler toastHandler = new Handler(Looper.getMainLooper());
    private final Handler iconHandler = new Handler(Looper.getMainLooper());
    private final Handler progressHandler = new Handler(Looper.getMainLooper());
    private static final long PROGRESS_TICK_MS = 5000L;

    /** 最近一次播放状态 — onDestroy 判断是否自然播完 */
    private int lastPlayerState = -1;
    /** 上一集索引 — onMediaItemTransition 回传 fromIndex 给 web 清上一集记忆 */
    private int lastMediaItemIndex = 0;

    // ===== Helpers =====
    PlayerAdFilterHelper adFilterHelper;
    PlayerSkipHelper skipHelper;
    PlayerGestureHelper gestureHelper;
    PlayerDialogHelper dialogHelper;
    PlayerKeyEventHelper keyEventHelper;
    PlayerSourceHelper sourceHelper;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        sCurrentInstance = this;
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        setContentView(R.layout.activity_player);
        hideSystemUi();

        playerView = findViewById(R.id.player_view);
        bufferSpinner = findViewById(R.id.buffer_spinner);
        titleText = findViewById(R.id.title_text);
        speedText = findViewById(R.id.speed_text);
        episodesCount = findViewById(R.id.episodes_count);
        resolutionBadge = findViewById(R.id.resolution_badge);
        TextView adFilterBadge = findViewById(R.id.ad_filter_badge);
        centerToast = findViewById(R.id.center_toast);
        centerIcon = findViewById(R.id.center_icon);

        // 初始化 Helpers
        skipHelper = new PlayerSkipHelper(this);
        adFilterHelper = new PlayerAdFilterHelper(this, adFilterBadge, this);
        gestureHelper = new PlayerGestureHelper(this);
        dialogHelper = new PlayerDialogHelper(this, skipHelper);
        keyEventHelper = new PlayerKeyEventHelper(this);
        sourceHelper = new PlayerSourceHelper(this);

        // 方案B: 代理 base + 过滤开关
        adFilterHelper.proxyBase = adFilterHelper.resolveProxyBase(getIntent());
        adFilterHelper.adFilterOn = adFilterHelper.prefs().getBoolean("ad_filter_enabled", true);
        adFilterHelper.updateAdFilterBadge();

        View titleBar = findViewById(R.id.title_bar);
        playerView.setControllerVisibilityListener((PlayerView.ControllerVisibilityListener) v -> {
            titleBar.setVisibility(v);
            if (v == View.VISIBLE) {
                dialogHelper.bindControlButtons();
                playerView.post(() -> {
                    View prog = playerView.findViewById(androidx.media3.ui.R.id.exo_progress);
                    if (prog != null) prog.requestFocus();
                });
            }
        });
        playerView.setControllerAutoShow(false);
        playerView.setOnTouchListener(gestureHelper::onPlayerTouch);

        // 账号记忆: web 按 mid 从账号/本地取跳过秒数传来
        long iMs = getIntent().getLongExtra(EXTRA_SKIP_INTRO_MS, 0L);
        long oMs = getIntent().getLongExtra(EXTRA_SKIP_OUTRO_MS, 0L);
        skipHelper.skipIntroMs = iMs > 0 ? iMs : PlayerSkipHelper.DEFAULT_SKIP_INTRO_MS;
        skipHelper.skipOutroMs = oMs > 0 ? oMs : PlayerSkipHelper.DEFAULT_SKIP_OUTRO_MS;
        skipHelper.skipEnabled = (iMs > 0 || oMs > 0);
        skipHelper.autoNext = getIntent().getBooleanExtra(EXTRA_AUTO_NEXT, true);

        initPlayer();
        sourceHelper.startFromIntent(getIntent());
    }

    private void initPlayer() {
        DataSource.Factory cacheFactory = buildCacheFactory();
        HlsMediaSource.Factory msFactory = new HlsMediaSource.Factory(cacheFactory)
                .setPlaylistParserFactory(adFilterHelper.new FilterPlaylistParserFactory())
                .setAllowChunklessPreparation(true);
        DefaultRenderersFactory renderersFactory = new DefaultRenderersFactory(this)
                .setEnableDecoderFallback(true)
                .forceEnableMediaCodecAsynchronousQueueing();
        LoadControl loadControl = new DefaultLoadControl.Builder()
                .setBufferDurationsMs(30_000, 120_000, 1_500, 2_500)
                .setPrioritizeTimeOverSizeThresholds(true)
                .setBackBuffer(30_000, true)
                .build();
        player = new ExoPlayer.Builder(this, renderersFactory)
                .setMediaSourceFactory(msFactory)
                .setLoadControl(loadControl)
                .build();
        playerView.setPlayer(player);

        player.addListener(new Player.Listener() {
            @Override
            public void onPlaybackStateChanged(int state) {
                lastPlayerState = state;
                bufferSpinner.setVisibility(state == Player.STATE_BUFFERING ? View.VISIBLE : View.GONE);
                if (state == Player.STATE_READY) {
                    skipHelper.episodeSwitching = false;
                    if (!skipHelper.introSkippedForCurrent) {
                        skipHelper.applySkipIntro();
                        skipHelper.introSkippedForCurrent = true;
                    }
                    if (!adFilterHelper.filterToastShownForEpisode) {
                        adFilterHelper.filterToastShownForEpisode = true;
                        adFilterHelper.showFilterStatus();
                    }
                }
            }

            @Override
            public void onPlayWhenReadyChanged(boolean playWhenReady, int reason) {
                if (player == null) return;
                if (!playWhenReady) {
                    if (player.getPlaybackState() == Player.STATE_READY) {
                        showCenterIconPersistent(R.drawable.ic_pause);
                        if (playerView != null) playerView.showController();
                    }
                } else {
                    showCenterIcon(R.drawable.ic_play, 600);
                }
            }

            @Override
            public void onMediaItemTransition(MediaItem mediaItem, int reason) {
                skipHelper.introSkippedForCurrent = false;
                skipHelper.outroPromptShown = false;
                skipHelper.episodeSwitching = true;
                adFilterHelper.pendingFilteredCount = 0;
                adFilterHelper.filterAttempted = false;
                adFilterHelper.filterFailed = false;
                adFilterHelper.filterProxyMissing = false;
                adFilterHelper.filterToastShownForEpisode = false;
                sourceHelper.updateTitleForCurrent();
                try {
                    org.json.JSONObject p = new org.json.JSONObject();
                    p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
                    p.put("episodeIndex", player.getCurrentMediaItemIndex());
                    p.put("fromIndex", lastMediaItemIndex);
                    lastMediaItemIndex = player.getCurrentMediaItemIndex();
                    p.put("source", sourceHelper.currentSourceLabel());
                    p.put("reason", reason);
                    emit("playerEpisodeChange", p);
                } catch (Exception ignore) {}
            }

            @Override
            public void onPlayerError(@androidx.annotation.NonNull androidx.media3.common.PlaybackException error) {
                sLastPlaybackFailed = true;
                String currentUrl = "";
                try {
                    if (player != null && player.getCurrentMediaItem() != null
                            && player.getCurrentMediaItem().localConfiguration != null) {
                        currentUrl = String.valueOf(
                                player.getCurrentMediaItem().localConfiguration.uri);
                    }
                } catch (Exception ignore) {}
                final int errIdx = (player != null) ? player.getCurrentMediaItemIndex() : -1;
                if (adFilterHelper.adFilterOn && currentUrl.contains("/m3u8/proxy")
                        && errIdx >= 0 && errIdx < adFilterHelper.currentRawUrls.size()
                        && !adFilterHelper.forceRawIdx.contains(errIdx)) {
                    adFilterHelper.forceRawIdx.add(errIdx);
                    sLastPlaybackFailed = false;
                    final long pos = Math.max(0, player.getCurrentPosition());
                    final String raw = adFilterHelper.currentRawUrls.get(errIdx);
                    runOnUiThread(() -> {
                        try {
                            player.replaceMediaItem(errIdx, MediaItem.fromUri(raw));
                            player.seekTo(errIdx, pos);
                            player.prepare();
                            player.play();
                            showCenterToast("本集代理失败, 已用原始源(广告未过滤)", 2500);
                        } catch (Exception ignore2) {}
                    });
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
                } catch (Exception ignore) {}
                try {
                    org.json.JSONObject p = new org.json.JSONObject();
                    p.put("code", error.errorCode);
                    p.put("errorCodeName", error.getErrorCodeName());
                    p.put("message", error.getMessage() == null ? "" : error.getMessage());
                    p.put("currentUrl", currentUrl);
                    p.put("causeDetail", causeDetail);
                    emit("playerError", p);
                } catch (Exception ignore) {}
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

    /** 5s 一次发 playerProgress 给 web (含 filmId + episodeIndex + position). */
    private final Runnable progressTick = new Runnable() {
        @Override
        public void run() {
            try {
                if (player != null && player.isPlaying() && !skipHelper.inNoRecordTail()) {
                    org.json.JSONObject p = new org.json.JSONObject();
                    p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
                    p.put("episodeIndex", player.getCurrentMediaItemIndex());
                    p.put("source", sourceHelper.currentSourceLabel());
                    p.put("position", player.getCurrentPosition() / 1000.0);
                    emit("playerProgress", p);
                }
            } catch (Exception ignore) {
            } finally {
                progressHandler.postDelayed(this, PROGRESS_TICK_MS);
            }
        }
    };

    /** 解析 Intent 启动: 优先级 sources_json (v3 多源) > playlist (v2 单源) > 单 URL */

    private DataSource.Factory buildCacheFactory() {
        if (sCache == null) {
            File cacheDir = new File(getCacheDir(), "video_cache");
            if (!cacheDir.exists()) cacheDir.mkdirs();
            sCache = new SimpleCache(
                    cacheDir,
                    new LeastRecentlyUsedCacheEvictor(CACHE_SIZE),
                    new StandaloneDatabaseProvider(getApplicationContext())
            );
        }
        OkHttpClient http = new OkHttpClient.Builder()
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .writeTimeout(30, TimeUnit.SECONDS)
                .callTimeout(60, TimeUnit.SECONDS)
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


    PlayerDialogHelper getDialogHelper() {
        return dialogHelper;
    }

    void showCenterToast(String msg, long durationMs) {
        if (centerIcon != null) centerIcon.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null);
        centerToast.setText(msg);
        centerToast.setVisibility(View.VISIBLE);
        toastHandler.removeCallbacksAndMessages(null);
        toastHandler.postDelayed(() -> {
            centerToast.setVisibility(View.GONE);
            if (player != null && !player.getPlayWhenReady()
                    && player.getPlaybackState() == Player.STATE_READY) {
                showCenterIconPersistent(R.drawable.ic_pause);
            }
        }, durationMs);
    }

    void showCenterIcon(int drawableRes, long durationMs) {
        if (centerIcon == null) return;
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
        iconHandler.removeCallbacksAndMessages(null);
        iconHandler.postDelayed(() -> centerIcon.setVisibility(View.GONE), durationMs);
    }

    void showCenterIconPersistent(int drawableRes) {
        if (centerIcon == null) return;
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null);
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
    }

    void hideGestureToast() {
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        if (player != null && !player.getPlayWhenReady()
                && player.getPlaybackState() == Player.STATE_READY) {
            showCenterIconPersistent(R.drawable.ic_pause);
        }
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

    @Override
    protected void onPause() {
        super.onPause();
        emitProgressNow();
        if (player != null && player.isPlaying()) player.pause();
    }

    private void emitProgressNow() {
        try {
            if (player == null || skipHelper.inNoRecordTail()) return;
            org.json.JSONObject p = new org.json.JSONObject();
            p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
            p.put("episodeIndex", player.getCurrentMediaItemIndex());
            p.put("source", sourceHelper.currentSourceLabel());
            p.put("position", player.getCurrentPosition() / 1000.0);
            emit("playerProgress", p);
        } catch (Exception ignore) {}
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        if (sCurrentInstance == this) sCurrentInstance = null;
        skipHelper.stopOutroWatcher();
        toastHandler.removeCallbacksAndMessages(null);
        iconHandler.removeCallbacksAndMessages(null);
        progressHandler.removeCallbacksAndMessages(null);
        boolean playbackEnded = player != null && lastPlayerState == androidx.media3.common.Player.STATE_ENDED;
        if (playbackEnded || !(player != null && skipHelper.inNoRecordTail())) {
            try {
                org.json.JSONObject p = new org.json.JSONObject();
                p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
                if (player != null) {
                    p.put("position", player.getCurrentPosition() / 1000.0);
                    p.put("duration", player.getDuration() > 0 ? player.getDuration() / 1000.0 : 0);
                    p.put("episodeIndex", player.getCurrentMediaItemIndex());
                    p.put("source", sourceHelper.currentSourceLabel());
                    if (playbackEnded) p.put("ended", true);
                }
                emit("playerClosed", p);
            } catch (Exception ignore) {}
        }
        if (player != null) {
            player.release();
            player = null;
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        adFilterHelper.proxyBase = adFilterHelper.resolveProxyBase(intent);
        long iMs = intent.getLongExtra(EXTRA_SKIP_INTRO_MS, 0L);
        long oMs = intent.getLongExtra(EXTRA_SKIP_OUTRO_MS, 0L);
        skipHelper.skipIntroMs = iMs > 0 ? iMs : PlayerSkipHelper.DEFAULT_SKIP_INTRO_MS;
        skipHelper.skipOutroMs = oMs > 0 ? oMs : PlayerSkipHelper.DEFAULT_SKIP_OUTRO_MS;
        skipHelper.skipEnabled = (iMs > 0 || oMs > 0);
        skipHelper.autoNext = getIntent().getBooleanExtra(EXTRA_AUTO_NEXT, true);
        skipHelper.introSkippedForCurrent = false;
        sourceHelper.startFromIntent(intent);
    }
}
