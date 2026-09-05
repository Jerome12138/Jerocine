package com.jerocine.app;

import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.CompoundButton;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.Switch;
import android.widget.TextView;

import androidx.annotation.OptIn;
import androidx.appcompat.app.AppCompatActivity;
import androidx.media3.common.MediaItem;
import androidx.media3.common.PlaybackParameters;
import androidx.media3.common.Player;
import androidx.media3.common.Timeline;
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
import androidx.media3.exoplayer.hls.playlist.DefaultHlsPlaylistParserFactory;
import androidx.media3.exoplayer.hls.playlist.HlsMediaPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsMultivariantPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylist;
import androidx.media3.exoplayer.hls.playlist.HlsPlaylistParserFactory;
import androidx.media3.exoplayer.upstream.ParsingLoadable;
import androidx.media3.ui.PlayerView;

import java.io.File;
import java.io.IOException;
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
    /** 每集开头跳过的毫秒数, 0 = 不跳 */
    public static final String EXTRA_SKIP_INTRO_MS = "skip_intro_ms";
    /** 每集结尾提前切下一集的毫秒数, 0 = 不跳 */
    public static final String EXTRA_SKIP_OUTRO_MS = "skip_outro_ms";
    /** 自动连播开关(web 播放页开关的持久化值). false 时跳片尾=连播不生效, 跳片头不受影响 */
    public static final String EXTRA_AUTO_NEXT = "auto_next";
    /** 影片 ID (用于事件回传给 web 写历史) */
    public static final String EXTRA_FILM_ID = "film_id";
    /** 影片名 (前缀拼到 title) */
    public static final String EXTRA_FILM_NAME = "film_name";
    /** 多源 JSON (v3): [{"id","name","episodes":[{url,title}]}] */
    public static final String EXTRA_SOURCES_JSON = "sources_json";
    /** 多源初始选中的源 id (找不到时用 sources[0]) */
    public static final String EXTRA_CURRENT_SOURCE_ID = "current_source_id";
    /** 方案B: 代理 base(到 /api), 原生对原始 m3u8 每集自行拼 /v1/m3u8/proxy 过滤 */
    public static final String EXTRA_PROXY_BASE = "proxy_base";

    /** native 内部事件回调接口, MainActivity 实现并设置, PlayerActivity 触发 */
    public interface EventListener {
        void onPlayerEvent(String name, org.json.JSONObject payload);
    }
    private static volatile EventListener sListener;
    public static void setEventListener(EventListener l) { sListener = l; }
    private static void emit(String name, org.json.JSONObject payload) {
        EventListener l = sListener;
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
    private static final float[] SPEEDS = {0.5f, 1.0f, 1.25f, 1.5f, 2.0f, 3.0f};
    private static final String[] SPEED_LABELS = {"0.5×", "1.0×", "1.25×", "1.5×", "2.0×", "3.0×"};
    /** 跳片尾轮询间隔 */
    private static final long OUTRO_POLL_MS = 1000L;
    /** 跳片头/片尾不应用于过短的视频 */
    private static final long MIN_DURATION_FOR_SKIP_MS = 5 * 60_000L;

    /** 进程内单例, 避免重开 PlayerActivity 时 "Another SimpleCache instance" 报错 */
    private static SimpleCache sCache;

    /**
     * 跨 Activity 的播放失败 flag.
     * PlayerActivity 报错时置 true, finish() 后 MainActivity.onResume 检查并通知前端 fallback.
     */
    public static volatile boolean sLastPlaybackFailed = false;

    private ExoPlayer player;
    private PlayerView playerView;
    private ProgressBar bufferSpinner;
    private TextView titleText;
    private TextView speedText;
    private TextView episodesCount;
    private TextView centerToast;
    private ImageView centerIcon;  // 中央 播放/暂停 反馈图标(无外框)

    private int speedIndex = 1; // 默认 1.0×
    private final Handler toastHandler = new Handler(Looper.getMainLooper());
    private final Handler iconHandler = new Handler(Looper.getMainLooper());
    private final Handler outroHandler = new Handler(Looper.getMainLooper());
    /** 周期发 playerProgress 事件给 web (5s 一次, 用于写历史) */
    private final Handler progressHandler = new Handler(Looper.getMainLooper());
    private static final long PROGRESS_TICK_MS = 5000L;

    /** 默认值 — 跳过开关打开时若用户没改, 用这个 */
    private static final long DEFAULT_SKIP_INTRO_MS = 90_000L;
    private static final long DEFAULT_SKIP_OUTRO_MS = 60_000L;
    /** #4: 倒数 5 分钟内不上报进度(web 层不写播放记忆), 避免重开续播到片尾。
     *  时长 <= 5min 的短视频不适用(否则永远不记录)。 */
    private static final long NO_RECORD_TAIL_MS = 300_000L;
    private long skipIntroMs = DEFAULT_SKIP_INTRO_MS;
    private long skipOutroMs = DEFAULT_SKIP_OUTRO_MS;
    /** 跳过总开关 — 默认关. 开关关时即使 skipIntroMs/OutroMs > 0 也不执行 skip */
    private boolean skipEnabled = false;
    /** 自动连播开关(默认开). 关 → outroWatcher 不触发(跳过片尾属连播范畴); 跳片头不受影响 */
    private boolean autoNext = true;

    /* ========= 触屏手势(对齐 web 端语义): 长按临时 2x / 横滑刮擦进度 / 轻扫 ±10s =========
     * 长按(450ms, 播放中) = 临时 2x 倍速, 松手恢复原速 — 不写任何持久化, 不影响倍速菜单档位;
     * 按住横拖 = 连续刮擦进度(全屏宽≈全片时长, 节流实时 seek, 中央提示跟手);
     * 短促轻扫(<300ms 且 <60px) = 固定 ±10s; 控制条/按钮等子控件自吃触摸, 不参与手势。 */
    private static final int GESTURE_LONG_PRESS_MS = 450;
    private static final int GESTURE_SWIPE_TRIGGER_PX = 12;
    private static final int GESTURE_FLICK_MS = 300;
    private static final int GESTURE_FLICK_PX = 60;
    private static final int GESTURE_FLICK_SEEK_SEC = 10;
    private static final int GESTURE_SEEK_INTERVAL_MS = 400;
    /** 刮擦中提示保持显示的时长(收尾时手动清) — 避免 800ms 常规 toast 一闪而过 */
    private static final long GESTURE_HINT_KEEP_MS = 60_000L;
    /** 手势模式: 0=无 1=长按2x 2=横滑刮擦 */
    private int gestureMode = 0;
    private float gestureStartX, gestureStartY;
    private long gestureStartTime;
    private long gestureSeekBaseMs, gestureTargetMs;
    private float gestureLastDx;
    private long gestureLastSeekAt;
    private float gestureTempRateBefore = 1f;
    private final Runnable gestureLongPressRunnable = new Runnable() {
        @Override public void run() {
            // 仅播放中(缓冲/暂停不触发)进入临时 2x — 与 web 端一致
            if (player == null || !player.getPlayWhenReady()
                    || player.getPlaybackState() == Player.STATE_BUFFERING) return;
            gestureMode = 1;
            gestureTempRateBefore = player.getPlaybackParameters().speed;
            player.setPlaybackParameters(new PlaybackParameters(2f));
            showCenterToast("2x 倍速中 ▶▶", GESTURE_HINT_KEEP_MS);
        }
    };
    /** 最近一次播放状态 — onDestroy 判断是否自然播完(ENDED → web 清该集记忆而非写进度) */
    private int lastPlayerState = -1;
    /** 上一集索引 — onMediaItemTransition 回传 fromIndex 给 web 清上一集记忆 */
    private int lastMediaItemIndex = 0;
    /** 当前集是否已在 ready 时跳过了片头, 避免重复跳 */
    private boolean introSkippedForCurrent = false;
    /** 当前集是否已弹过"片尾即将跳过"的提前 10s 预告 */
    private boolean outroPromptShown = false;
    /** 正在切集(含自动续集): 期间的缓冲不弹面板; 切集完成(READY)清除 */
    private boolean episodeSwitching = false;
    private List<String> playlistTitles = new ArrayList<>();

    /** 多源模式 state (单源模式时 sourceList 为空) */
    private static class SourceData {
        String id;
        String name;
        ArrayList<String> urls = new ArrayList<>();
        ArrayList<String> titles = new ArrayList<>();
    }
    private final ArrayList<SourceData> sourceList = new ArrayList<>();
    private int currentSourceIndex = 0;

    /** ===== 方案B: 广告过滤(原生每集自包装代理 + 反馈 + 失败仅回退本集) ===== */
    private static final String PREFS_NAME = "jerocine";
    private static final String PREF_AD_FILTER = "ad_filter_enabled";
    private String proxyBase = "";                            // 代理 base(到 /api)
    private volatile boolean adFilterOn = true;                      // 过滤开关(SharedPreferences 持久, 默认开)
    private List<String> currentRawUrls = new ArrayList<>();  // 当前源每集"原始" m3u8
    private final Set<Integer> forceRawIdx = new HashSet<>(); // 代理失败强制用原始的集(切源重置)
    private volatile String lastFilterToastUrl = "";          // 防同集重复弹"已过滤 N 段"(解析线程写)
    private volatile int pendingFilteredCount = 0;            // 本集过滤到的广告段数(取主/子表最大; 解析线程写)
    private volatile boolean filterAttempted = false;         // 本集是否真正调用过 /m3u8/filter(解析线程写)
    private volatile boolean filterFailed = false;            // 本集过滤请求失败过(网络/服务端; 解析线程写)
    private volatile boolean filterProxyMissing = false;      // 开关开着但 proxyBase 空(前端没传/WebView 旧缓存)
    private boolean filterToastShownForEpisode = false;       // 本集"过滤状态"提示是否已弹(每集一次)
    private TextView adFilterBadge;                            // 标题栏"🛡 过滤"常驻标(起播后显示本集结果)
    // 端侧过滤 POST 客户端: 显式超时上限(默认无 callTimeout, 卡住会拖死播放列表解析线程),
    // 切集时网络争用偶发失败 → filterViaServer 内重试一次, 降低"切集有时不过滤广告"概率。
    private final OkHttpClient adStatsClient = new OkHttpClient.Builder()
            .connectTimeout(6, TimeUnit.SECONDS)
            .readTimeout(6, TimeUnit.SECONDS)
            .writeTimeout(6, TimeUnit.SECONDS)
            .callTimeout(8, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .build();

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
        adFilterBadge = findViewById(R.id.ad_filter_badge);
        centerToast = findViewById(R.id.center_toast);
        centerIcon = findViewById(R.id.center_icon);

        // 方案B: 代理 base + 过滤开关(原生持久化为准, 默认开 — 自动修复"web 端被卡在关闭"的旧坑)
        proxyBase = resolveProxyBase(getIntent());
        adFilterOn = prefs().getBoolean(PREF_AD_FILTER, true);
        updateAdFilterBadge();

        View titleBar = findViewById(R.id.title_bar);
        // 控制面板可见时:
        //   1) titleBar 跟随显隐
        //   2) 进度条 (exo_progress) 默认获焦 — 用户期望"控制面板出, 进度条选中, 上下切别的按钮"
        //   3) 默认右下角设置按钮 (exo_settings 齿轮) 的点击改成我们的 showPlayMenu —
        //      用户要求"我们自定义的设置替换掉右下角播放器的设置按钮", 原 Media3 默认弹窗
        //      只有播放速度 (已在 showPlayMenu 内), HLS 一般没字幕/音轨多轨, 直接整体替换.
        playerView.setControllerVisibilityListener((PlayerView.ControllerVisibilityListener) v -> {
            titleBar.setVisibility(v);
            if (v == View.VISIBLE) {
                bindControlButtons();
                playerView.post(() -> {
                    View prog = playerView.findViewById(androidx.media3.ui.R.id.exo_progress);
                    if (prog != null) prog.requestFocus();
                });
            }
        });
        // 切集/缓冲/暂停时不自动弹控制面板 — 仅用户显式唤起(确认键/点击)才显示,
        // 否则"连按下一集"会误弹操作浮窗。
        playerView.setControllerAutoShow(false);

        // 触屏手势接管(长按 2x / 横滑刮擦进度 / 轻扫 ±10s)。整体消费触摸后
        // PlayerView 默认"点击切换控制面板"失效 → 单击分支在 onPlayerTouch 内手动复刻。
        playerView.setOnTouchListener(this::onPlayerTouch);

        // 账号记忆: web 按 mid 从账号/本地取跳过秒数传来(默认 90/60, 关闭则传 0)。
        // 值>0 即视为"已开启"(与 web 端默认自动跳片头一致, 跨设备记忆)。
        long iMs = getIntent().getLongExtra(EXTRA_SKIP_INTRO_MS, 0L);
        long oMs = getIntent().getLongExtra(EXTRA_SKIP_OUTRO_MS, 0L);
        skipIntroMs = iMs > 0 ? iMs : DEFAULT_SKIP_INTRO_MS;
        skipOutroMs = oMs > 0 ? oMs : DEFAULT_SKIP_OUTRO_MS;
        skipEnabled = (iMs > 0 || oMs > 0);
        autoNext = getIntent().getBooleanExtra(EXTRA_AUTO_NEXT, true);

        initPlayer();
        startFromIntent(getIntent());
    }

    private void initPlayer() {
        DataSource.Factory cacheFactory = buildCacheFactory();
        // 端侧混合广告过滤: 全 HLS 内容, 用 HlsMediaSource.Factory + 自定义播放列表解析器,
        // 抓到 m3u8 后送服务器 /m3u8/filter 剔广告再解析(设备直连分片)。解决服务器抓不到的源(bf 地域封)。
        HlsMediaSource.Factory msFactory = new HlsMediaSource.Factory(cacheFactory)
                .setPlaylistParserFactory(new FilterPlaylistParserFactory())
                .setAllowChunklessPreparation(true);
        // 解码加速/抗卡顿: ① 硬解吃不消时回退软解(setEnableDecoderFallback), 避免某些源直接卡死;
        //                  ② 异步队列送解码(多核更顺, 默认仅 API31+, 这里全机型强制开).
        DefaultRenderersFactory renderersFactory = new DefaultRenderersFactory(this)
                .setEnableDecoderFallback(true)
                .forceEnableMediaCodecAsynchronousQueueing();
        // 缓冲调深: PC 端 hls.js 缓冲很激进, 安卓默认偏浅 → 同源安卓更易卡.
        // 最多缓 60s + 按"时长"而非字节预算缓冲(高码率也缓够时长); 卡顿后多缓 5s 再续, 减少二次卡顿.
        // 抗卡顿/快起播: 起播阈值 1.5s(更快出画面), 卡顿后 2.5s 恢复; 稳态目标缓冲 25s,
        // 上限拉到 90s(带宽够时多缓、弱网多扛); 留 30s 回看缓冲免重下。
        LoadControl loadControl = new DefaultLoadControl.Builder()
                .setBufferDurationsMs(25_000, 90_000, 1_500, 2_500)
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
                // 缓冲不再自动弹控制面板(只保留中央 spinner 指示)。
                // 原因: 播放中网络抖动/重缓冲会反复触发 STATE_BUFFERING, 一弹面板就把上下键
                // 变成"面板内焦点移动", 抢掉"上下键两次切上/下一集"——用户感知为"播放中按上下键
                // 唤出了控制面板"。面板仅由 MENU / 暂停 / 确认键显式唤起。
                if (state == Player.STATE_READY) {
                    episodeSwitching = false;
                    if (!introSkippedForCurrent) {
                        applySkipIntro();
                        introSkippedForCurrent = true;
                    }
                    // 开始播放时弹一次"过滤状态"(每集一次): 让用户明确知道本集到底有没有过滤
                    if (!filterToastShownForEpisode) {
                        filterToastShownForEpisode = true;
                        showFilterStatus();
                    }
                }
            }

            @Override
            public void onPlayWhenReadyChanged(boolean playWhenReady, int reason) {
                if (player == null) return;
                if (!playWhenReady) {
                    // 暂停: 中央暂停图标"持久"显示(不自动消失, 让用户看到处于暂停) + 弹控制面板
                    if (player.getPlaybackState() == Player.STATE_READY) {
                        showCenterIconPersistent(R.drawable.ic_pause);
                        if (playerView != null) playerView.showController();
                    }
                } else {
                    // 恢复播放: 短暂提示播放图标后消失
                    showCenterIcon(R.drawable.ic_play, 600);
                }
            }

            @Override
            public void onMediaItemTransition(MediaItem mediaItem, int reason) {
                introSkippedForCurrent = false;
                outroPromptShown = false;
                episodeSwitching = true;   // 切集期间缓冲不弹面板, READY 后清除
                pendingFilteredCount = 0;  // 切集重置: 待新集解析后重新计 + READY 弹一次
                filterAttempted = false;
                filterFailed = false;
                filterProxyMissing = false;
                filterToastShownForEpisode = false;
                updateTitleForCurrent();
                try {
                    org.json.JSONObject p = new org.json.JSONObject();
                    p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
                    p.put("episodeIndex", player.getCurrentMediaItemIndex());
                    // fromIndex: 上一集索引 — web 收到后清理上一集的播放记忆
                    // (配合"倒数5分钟不记进度": 自动切集=上一集已播完, 旧进度不应滞留)
                    p.put("fromIndex", lastMediaItemIndex);
                    lastMediaItemIndex = player.getCurrentMediaItemIndex();
                    p.put("source", currentSourceLabel());
                    p.put("reason", reason);
                    emit("playerEpisodeChange", p);
                } catch (Exception ignore) {}
            }

            @Override
            public void onPlayerError(@androidx.annotation.NonNull androidx.media3.common.PlaybackException error) {
                // 标记给 MainActivity, 让前端关 adFilter 用原 URL 重启播放
                sLastPlaybackFailed = true;
                String currentUrl = "";
                try {
                    if (player != null && player.getCurrentMediaItem() != null
                            && player.getCurrentMediaItem().localConfiguration != null) {
                        currentUrl = String.valueOf(
                                player.getCurrentMediaItem().localConfiguration.uri);
                    }
                } catch (Exception ignore) {}
                // 方案B: 代理失败 → 仅本集回退原始源(不全局关过滤, 不退出播放器)。
                final int errIdx = (player != null) ? player.getCurrentMediaItemIndex() : -1;
                if (adFilterOn && currentUrl.contains("/m3u8/proxy")
                        && errIdx >= 0 && errIdx < currentRawUrls.size()
                        && !forceRawIdx.contains(errIdx)) {
                    forceRawIdx.add(errIdx);
                    sLastPlaybackFailed = false;       // 正在自愈, 不当致命失败
                    final long pos = Math.max(0, player.getCurrentPosition());
                    final String raw = currentRawUrls.get(errIdx);
                    outroHandler.post(() -> {
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
                // 抓底层 cause (HttpDataSourceException 会带具体 url + http code)
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
                // Toast 直接显示具体 URL 末尾 + 错误码名, 帮诊断 (复制完整 URL 难,
                // 至少先看到是不是 m3u8 索引层 fail 还是 segment 层 fail)
                String urlTail = currentUrl.length() > 60
                        ? "..." + currentUrl.substring(currentUrl.length() - 60)
                        : currentUrl;
                showCenterToast("播放出错 " + error.errorCode + " (" + error.getErrorCodeName() + ")\n"
                        + urlTail + causeDetail, 8000);
                outroHandler.postDelayed(() -> {
                    if (!isFinishing()) finish();
                }, 8000);
            }
        });

        outroHandler.postDelayed(outroWatcher, OUTRO_POLL_MS);
        progressHandler.postDelayed(progressTick, PROGRESS_TICK_MS);
    }

    /** 5s 一次发 playerProgress 给 web (含 filmId + episodeIndex + position). 不传 duration,
     *  避免每 tick 都触发 history 写带 duration; 用户要求"时长仅退出时落". */
    private final Runnable progressTick = new Runnable() {
        @Override
        public void run() {
            try {
                if (player != null && player.isPlaying() && !inNoRecordTail()) {
                    org.json.JSONObject p = new org.json.JSONObject();
                    p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
                    p.put("episodeIndex", player.getCurrentMediaItemIndex());
                    p.put("source", currentSourceLabel());
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
    private void startFromIntent(Intent intent) {
        int startIndex = intent.getIntExtra(EXTRA_START_INDEX, 0);
        long resumeMs = intent.getLongExtra(EXTRA_RESUME_MS, 0L);
        lastMediaItemIndex = startIndex;

        // v3: 多源模式
        String sourcesJson = intent.getStringExtra(EXTRA_SOURCES_JSON);
        if (sourcesJson != null && !sourcesJson.isEmpty()) {
            try {
                sourceList.clear();
                org.json.JSONArray arr = new org.json.JSONArray(sourcesJson);
                String currentId = intent.getStringExtra(EXTRA_CURRENT_SOURCE_ID);
                int curIdx = 0;
                for (int i = 0; i < arr.length(); i++) {
                    org.json.JSONObject src = arr.getJSONObject(i);
                    SourceData sd = new SourceData();
                    sd.id = src.optString("id", "");
                    sd.name = src.optString("name", "源 " + (i + 1));
                    org.json.JSONArray eps = src.optJSONArray("episodes");
                    if (eps != null) {
                        for (int j = 0; j < eps.length(); j++) {
                            org.json.JSONObject ep = eps.getJSONObject(j);
                            String u = ep.optString("url", "");
                            if (u.isEmpty()) continue;
                            sd.urls.add(u);
                            sd.titles.add(ep.optString("title", ""));
                        }
                    }
                    sourceList.add(sd);
                    if (currentId != null && currentId.equals(sd.id)) curIdx = i;
                }
                if (sourceList.isEmpty()) { finish(); return; }
                currentSourceIndex = Math.max(0, Math.min(curIdx, sourceList.size() - 1));
                loadSourceIntoPlayer(currentSourceIndex, startIndex, resumeMs);
                return;
            } catch (Exception e) {
                // JSON 异常 fallback 单 URL
            }
        }

        // v2: 单源 playlist
        ArrayList<String> urls = intent.getStringArrayListExtra(EXTRA_PLAYLIST_URLS);
        ArrayList<String> titles = intent.getStringArrayListExtra(EXTRA_PLAYLIST_TITLES);
        if (urls != null && !urls.isEmpty()) {
            playlistTitles = (titles != null) ? titles : new ArrayList<>();
            currentRawUrls = new ArrayList<>(urls);
            forceRawIdx.clear();
            List<MediaItem> items = new ArrayList<>(urls.size());
            for (int i = 0; i < urls.size(); i++) items.add(MediaItem.fromUri(mediaUriFor(i, urls.get(i))));
            player.setMediaItems(items, Math.max(0, Math.min(startIndex, items.size() - 1)), resumeMs);
            player.prepare();
            player.setPlayWhenReady(true);
            updateTitleForCurrent();
            return;
        }

        // 兼容单 URL
        String url = intent.getStringExtra(EXTRA_URL);
        String title = intent.getStringExtra(EXTRA_TITLE);
        if (url == null || url.isEmpty()) {
            finish();
            return;
        }
        playlistTitles = new ArrayList<>();
        playlistTitles.add(title != null ? title : "");
        player.setMediaItem(MediaItem.fromUri(url));
        if (resumeMs > 0) player.seekTo(resumeMs);
        player.prepare();
        player.setPlayWhenReady(true);
        titleText.setText(title != null ? title : "");
    }

    /** 把指定 source 的 episodes 装入 player; 从 startEpisodeIndex 位置开始, resumeMs 续播 */
    private void loadSourceIntoPlayer(int sourceIdx, int startEpisodeIndex, long resumeMs) {
        if (sourceIdx < 0 || sourceIdx >= sourceList.size()) return;
        SourceData src = sourceList.get(sourceIdx);
        playlistTitles = new ArrayList<>(src.titles);
        currentRawUrls = new ArrayList<>(src.urls);  // 原始 m3u8(供代理包装/反馈/回退)
        forceRawIdx.clear();                          // 切源重置"强制原始"
        List<MediaItem> items = new ArrayList<>(src.urls.size());
        for (int i = 0; i < src.urls.size(); i++) {
            items.add(MediaItem.fromUri(mediaUriFor(i, src.urls.get(i))));
        }
        int safeStart = Math.max(0, Math.min(startEpisodeIndex, items.size() - 1));
        introSkippedForCurrent = false;
        outroPromptShown = false;
        player.setMediaItems(items, safeStart, resumeMs);
        player.prepare();
        player.setPlayWhenReady(true);
        updateTitleForCurrent();
    }

    private SharedPreferences prefs() {
        return getSharedPreferences(PREFS_NAME, MODE_PRIVATE);
    }

    /** 当前片源 id(多源时 = sourceList 选中项 id, 如 "src_lz:lzm3u8"), 供回传 web 更新历史片源。 */
    private String currentSourceLabel() {
        if (currentSourceIndex >= 0 && currentSourceIndex < sourceList.size()) {
            return sourceList.get(currentSourceIndex).id;
        }
        return "";
    }

    /** 端侧混合过滤: 直接返回原始 URL; 广告剔除由 FilterPlaylistParser 在解析 m3u8 时调 /m3u8/filter 完成。 */
    private String mediaUriFor(int idx, String rawUrl) {
        return rawUrl == null ? "" : rawUrl;
    }

    /** 解析代理 base: 优先 web 传入的 EXTRA_PROXY_BASE; 为空(前端没传/WebView 旧缓存/relaunch onNewIntent)时
     *  兜底用本机配置的服务器地址拼 /api, 保证端侧广告过滤始终有可用代理, 不再"未生效"。 */
    private String resolveProxyBase(Intent intent) {
        String pb = (intent != null) ? intent.getStringExtra(EXTRA_PROXY_BASE) : null;
        if (pb == null || pb.isEmpty()) {
            String server = getSharedPreferences(MainActivity.PREFS, MODE_PRIVATE)
                    .getString(MainActivity.KEY_SERVER_URL, "https://jerocine.art");
            if (server != null && !server.isEmpty()) {
                pb = server.replaceAll("/+$", "") + "/api";
            }
        }
        return pb == null ? "" : pb;
    }

    private void updateAdFilterBadge() {
        if (adFilterBadge != null) {
            adFilterBadge.setVisibility(adFilterOn ? View.VISIBLE : View.GONE);
            // 盾牌图标已在布局里(drawableStart=ic_shield), 文字只放纯文字; 起播后由 showFilterStatus 改成结果
            if (adFilterOn) adFilterBadge.setText("过滤");
        }
    }

    /** 起播时按本集实际过滤结果给一次明确提示 + 刷新角标(让"有没有过滤掉"肉眼可见, 并暴露未生效原因)。 */
    private void showFilterStatus() {
        if (!adFilterOn) return; // 用户主动关了过滤, 不打扰
        // 角标盾牌图标已在布局里(drawableStart=ic_shield), badge 文字只放纯文字/数字
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
        showCenterToast(msg, 2200);
        if (adFilterBadge != null) {
            runOnUiThread(() -> { adFilterBadge.setText(badge); adFilterBadge.setVisibility(View.VISIBLE); });
        }
    }

    /** 端侧混合过滤: 自定义 HLS 播放列表解析器 —— 抓到 m3u8 后送 /m3u8/filter 剔广告, 再交默认解析器。 */
    private final class FilterPlaylistParser implements ParsingLoadable.Parser<HlsPlaylist> {
        private final ParsingLoadable.Parser<HlsPlaylist> delegate;

        FilterPlaylistParser(ParsingLoadable.Parser<HlsPlaylist> d) {
            this.delegate = d;
        }

        @Override
        public HlsPlaylist parse(Uri uri, java.io.InputStream in) throws IOException {
            byte[] data = readAll(in);
            if (adFilterOn) {
                if (proxyBase != null && !proxyBase.isEmpty()) {
                    byte[] f = filterViaServer(uri.toString(), data);
                    if (f != null) {
                        data = f;
                    }
                } else {
                    filterProxyMissing = true; // 开关开着却没代理地址 → 起播时提示"未生效"
                }
            }
            return delegate.parse(uri, new java.io.ByteArrayInputStream(data));
        }
    }

    private final class FilterPlaylistParserFactory implements HlsPlaylistParserFactory {
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

    /** 同步 POST 原始 m3u8 到 /m3u8/filter, 返回过滤后字节; 失败返回 null(用原始, 优雅降级)。在 loader 线程调用。 */
    private byte[] filterViaServer(String srcUrl, byte[] raw) {
        filterAttempted = true; // 真正发起了过滤请求(master/子表各算一次)
        final String url;
        try {
            url = proxyBase + "/v1/m3u8/filter?src=" + java.net.URLEncoder.encode(srcUrl, "UTF-8");
        } catch (Exception e) {
            filterFailed = true;
            return null;
        }
        // 切集时设备忙(下新集分片), 过滤 POST 偶发超时/失败 → 若直接回退原始, 整集(VOD 只加载一次)
        // 都不过滤广告。重试一次(短退避)显著降低"切集有时不触发过滤"概率。
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
                                // master 表 cnt=0、子表才 cnt>0; 取最大, 待 STATE_READY 弹一次状态
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
                    return null; // 两次均失败 → 用原始流(优雅降级, 照常播放)
                }
            }
            // 第一次失败, 短退避后重试一次
            try { Thread.sleep(250); }
            catch (InterruptedException ie) { Thread.currentThread().interrupt(); filterFailed = true; return null; }
        }
        filterFailed = true;
        return null;
    }

    private static byte[] readAll(java.io.InputStream in) throws IOException {
        java.io.ByteArrayOutputStream bos = new java.io.ByteArrayOutputStream();
        byte[] buf = new byte[8192];
        int n;
        while ((n = in.read(buf)) != -1) {
            bos.write(buf, 0, n);
        }
        return bos.toByteArray();
    }

    /** 切换广告过滤(原生菜单): 持久化 + 重载当前集(保留进度) + 刷新标。 */
    private void toggleAdFilter() {
        adFilterOn = !adFilterOn;
        prefs().edit().putBoolean(PREF_AD_FILTER, adFilterOn).apply();
        forceRawIdx.clear();
        lastFilterToastUrl = "";
        int idx = player != null ? player.getCurrentMediaItemIndex() : 0;
        long pos = player != null ? player.getCurrentPosition() : 0;
        loadSourceIntoPlayer(currentSourceIndex, idx, pos);
        updateAdFilterBadge();
        showCenterToast(adFilterOn ? "广告过滤已开启" : "广告过滤已关闭", 1200);
    }

    private void updateTitleForCurrent() {
        int idx = player.getCurrentMediaItemIndex();
        if (idx >= 0 && idx < playlistTitles.size()) {
            titleText.setText(playlistTitles.get(idx));
        }
        // 标题栏右上角显示当前片源总集数 (playlistTitles 即当前源全部集)
        if (episodesCount != null) {
            int total = playlistTitles.size();
            if (total > 0) {
                episodesCount.setText("共 " + total + " 集");
                episodesCount.setVisibility(View.VISIBLE);
            } else {
                episodesCount.setVisibility(View.GONE);
            }
        }
    }

    /** STATE_READY 第一次到达时, 跳到 skipIntroMs (短视频不跳). 总开关关时直接退出. */
    private void applySkipIntro() {
        if (player == null || !skipEnabled || skipIntroMs <= 0) return;
        long duration = player.getDuration();
        if (duration <= 0 || duration < MIN_DURATION_FOR_SKIP_MS) return;
        long currentPos = player.getCurrentPosition();
        // 如果是续播 (resumeMs > 0) 已经超过了片头位置, 就不再跳
        if (currentPos >= skipIntroMs) return;
        player.seekTo(skipIntroMs);
        showCenterToast("已跳过片头 " + (skipIntroMs / 1000) + "s", 1200);
    }

    /** 1s 一次轮询, 接近片尾时自动切下一集 */
    private final Runnable outroWatcher = new Runnable() {
        @Override
        public void run() {
            try {
                // !episodeSwitching: seekToNextMediaItem 是异步的, 切换未落地(READY)前
                // 旧集 remaining 仍 <= skipOutroMs, 1s 轮询会再次触发 → 连跳 2 集。
                // !autoNext: 自动连播关闭时不自动跳片尾(跳过片尾=进下一集, 属连播范畴)。
                if (player != null && skipEnabled && skipOutroMs > 0 && autoNext
                        && player.isPlaying() && !episodeSwitching) {
                    long duration = player.getDuration();
                    long pos = player.getCurrentPosition();
                    if (duration > MIN_DURATION_FOR_SKIP_MS && pos > 0 && player.hasNextMediaItem()) {
                        long remaining = duration - pos;
                        // 提前 10s 预告(每集只弹一次)
                        if (!outroPromptShown && remaining <= skipOutroMs + 10000 && remaining > skipOutroMs) {
                            outroPromptShown = true;
                            showCenterToast("10 秒后跳过片尾, 播放下一集", 1500);
                        }
                        if (remaining <= skipOutroMs) {
                            showCenterToast("跳过片尾 → 下一集", 1500);
                            episodeSwitching = true;
                            player.seekToNextMediaItem();
                        }
                    }
                }
            } finally {
                outroHandler.postDelayed(this, OUTRO_POLL_MS);
            }
        }
    };

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
        // 关键: 默认 OkHttp connect/read 各 10s, TV/CDN 节点经常超过 → ExoPlayer 报
        // 3003 TIMEOUT. 拉到 30s + 关 retryOnConnectionFailure 让 OkHttp 自己重试一次.
        // followRedirects: 部分 CDN 第一跳 302 到节点服, 默认 OkHttp 是开的但显式声明.
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

    private long lastEpisodeKeyAt = 0L;
    private int lastEpisodeKeyCode = 0;
    private long lastBackAt = 0L;
    private static final long EPISODE_CONFIRM_MS = 2000L;
    private static final long BACK_CONFIRM_MS = 2000L;

    /** 进度键长按渐进步进 — 按 getDownTime() 起算的实际按住时长 (ms) 计算步幅,
     *  比 repeatCount 更平滑 (repeat 取决于系统 key repeat rate). 快退 = 快进 50%. */
    private long seekStepFor(KeyEvent ev, boolean forward) {
        long heldMs = ev.getEventTime() - ev.getDownTime();
        long fwd;
        if (heldMs < 500)        fwd = 10_000L;   // <0.5s: 10s
        else if (heldMs < 2000)  fwd = 30_000L;   // 0.5-2s: 30s
        else if (heldMs < 5000)  fwd = 60_000L;   // 2-5s: 1min
        else                     fwd = 180_000L;  // >5s: 3min
        return forward ? fwd : fwd / 2;
    }

    /**
     * 三态键控:
     *  A. 控制面板隐藏 (默认): ←→=快进快退, ↑=上一集, ↓=下一集, OK=播放/暂停, MENU=显示控制面板,
     *     BACK=双击退出
     *  B. 控制面板可见, 焦点在进度条: ←→=快进快退, ↑↓=焦点上下移到其他按钮, OK=播放/暂停 (PlayerView 默认),
     *     MENU/BACK=隐藏控制面板
     *  C. 控制面板可见, 焦点在某个按钮 (含设置): ←→↑↓=焦点切换, OK=触发按钮 (super 处理),
     *     MENU/BACK=隐藏控制面板
     *  设置面板 (AlertDialog) 出现后由 dialog 自己处理 key, 这里不管.
     */
    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        if (event.getAction() != KeyEvent.ACTION_DOWN) {
            return super.dispatchKeyEvent(event);
        }
        int code = event.getKeyCode();
        boolean controllerVisible = playerView != null && playerView.isControllerFullyVisible();
        boolean onSeekbar = isFocusOnSeekbar();

        // ========= 全局键 =========
        switch (code) {
            case KeyEvent.KEYCODE_MENU:
            case KeyEvent.KEYCODE_INFO: {
                // MENU 始终切换控制面板, 不再直接开设置 — 设置必须从"设置按钮"进入
                if (controllerVisible) {
                    playerView.hideController();
                } else {
                    playerView.showController(); // listener 会自动聚焦进度条
                }
                return true;
            }
            case KeyEvent.KEYCODE_BACK: {
                if (controllerVisible) {
                    playerView.hideController();
                    lastBackAt = 0L; // 隐控制面板后, BACK 计数重置, 不会变成"误触退出"
                    return true;
                }
                // 控制面板已隐时, BACK 双击退出
                long now = System.currentTimeMillis();
                if (now - lastBackAt < BACK_CONFIRM_MS) {
                    lastBackAt = 0L;
                    finish();
                } else {
                    lastBackAt = now;
                    showCenterToast("再按一次返回退出播放", 1800);
                }
                return true;
            }
            case KeyEvent.KEYCODE_MEDIA_NEXT:
            case KeyEvent.KEYCODE_CHANNEL_UP:
                if (player != null && player.hasNextMediaItem()) {
                    episodeSwitching = true;
                    player.seekToNextMediaItem();
                    showCenterToast("下一集", 600);
                }
                return true;
            case KeyEvent.KEYCODE_MEDIA_PREVIOUS:
            case KeyEvent.KEYCODE_CHANNEL_DOWN:
                if (player != null && player.hasPreviousMediaItem()) {
                    episodeSwitching = true;
                    player.seekToPreviousMediaItem();
                    showCenterToast("上一集", 600);
                }
                return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:
            case KeyEvent.KEYCODE_SPACE:
                togglePlayPause();
                return true;
        }

        // ========= 控制面板隐藏: 经典播放页键 =========
        if (!controllerVisible) {
            switch (code) {
                case KeyEvent.KEYCODE_DPAD_LEFT:
                case KeyEvent.KEYCODE_MEDIA_REWIND:
                    seekRelative(-seekStepFor(event, false));
                    return true;
                case KeyEvent.KEYCODE_DPAD_RIGHT:
                case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD:
                    seekRelative(seekStepFor(event, true));
                    return true;
                case KeyEvent.KEYCODE_DPAD_UP:
                    return handleEpisodeKey(true, code);   // 上 = 上一集
                case KeyEvent.KEYCODE_DPAD_DOWN:
                    return handleEpisodeKey(false, code);  // 下 = 下一集
                case KeyEvent.KEYCODE_DPAD_CENTER:
                case KeyEvent.KEYCODE_ENTER:
                    togglePlayPause();
                    return true;
                default:
                    return super.dispatchKeyEvent(event);
            }
        }

        // ========= 控制面板可见 =========
        switch (code) {
            case KeyEvent.KEYCODE_DPAD_LEFT:
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                if (onSeekbar) {
                    seekRelative(code == KeyEvent.KEYCODE_DPAD_LEFT
                            ? -seekStepFor(event, false)
                            : seekStepFor(event, true));
                    return true;
                }
                // 焦点在按钮上 → 让 super 走焦点切换
                return super.dispatchKeyEvent(event);
            case KeyEvent.KEYCODE_DPAD_UP:
            case KeyEvent.KEYCODE_DPAD_DOWN:
                // 面板可见时 ↑↓ = 面板内焦点移动; 切集只在面板隐藏时(见上方 !controllerVisible 分支)。
                return super.dispatchKeyEvent(event);
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER:
                // 焦点在进度条上 → 确认键 = 播放/暂停(已移除单独的暂停键)
                if (onSeekbar) {
                    togglePlayPause();
                    return true;
                }
                // 其它按钮(选集/倍速/...)按下让 super 处理, 自己响应
                return super.dispatchKeyEvent(event);
            default:
                return super.dispatchKeyEvent(event);
        }
    }

    /** 是 / 否 焦点在进度条上 — 仅控制面板可见时有意义. */
    private boolean isFocusOnSeekbar() {
        if (playerView == null) return false;
        View focus = playerView.findFocus();
        if (focus == null) return false;
        return focus.getId() == androidx.media3.ui.R.id.exo_progress;
    }

    /** 切集 + 防误触: 1 下提示, 2s 内再按 1 下才生效. prev=true 上一集, false 下一集. */
    private boolean handleEpisodeKey(boolean prev, int code) {
        if (player == null) {
            return true;
        }
        boolean has = prev ? player.hasPreviousMediaItem() : player.hasNextMediaItem();
        if (!has) {
            showCenterToast(prev ? "已是第一集" : "已是最后一集", 800);
            return true;
        }
        long now = System.currentTimeMillis();
        if (lastEpisodeKeyCode == code && now - lastEpisodeKeyAt < EPISODE_CONFIRM_MS) {
            lastEpisodeKeyAt = 0; lastEpisodeKeyCode = 0;
            episodeSwitching = true; // 切集前置: 抑制随之而来的"加载下一集"缓冲弹面板(防竞态)
            if (prev) player.seekToPreviousMediaItem();
            else player.seekToNextMediaItem();
            showCenterToast(prev ? "上一集" : "下一集", 600);
        } else {
            lastEpisodeKeyAt = now; lastEpisodeKeyCode = code;
            showCenterToast(prev ? "再按 ▲ 切上一集" : "再按 ▼ 切下一集", 1500);
        }
        return true;
    }

    private void seekRelative(long deltaMs) {
        if (player == null) return;
        long target = Math.max(0, player.getCurrentPosition() + deltaMs);
        long duration = player.getDuration();
        if (duration > 0) target = Math.min(target, duration);
        player.seekTo(target);
        String dir = deltaMs > 0 ? "▶ +" : "◀ ";
        showCenterToast(dir + (deltaMs / 1000) + "s", 800);
    }

    private void togglePlayPause() {
        if (player == null) return;
        // 用 getPlayWhenReady()(播放"意图")判断, 而非 isPlaying()(缓冲/加载中返回 false):
        // 否则加载态按确认键会落进 else 分支误触发 play, 无法暂停。想播(含缓冲中)→暂停。
        // 中央图标显隐统一由 onPlayWhenReadyChanged 驱动(暂停持久显示, 播放短暂提示)。
        if (player.getPlayWhenReady()) {
            player.pause();
        } else {
            player.play();
        }
    }

    /** ========= 触屏手势处理(对齐 web 端语义) =========
     * 整体消费 playerView 的触摸; 控制条/按钮等子控件会先吃掉自己的触摸, 到不了这里。
     * 普通单击(未形成任何手势) = 切换控制面板 — 复刻被接管前 PlayerView 的默认行为。 */
    private boolean onPlayerTouch(View v, MotionEvent ev) {
        if (player == null) return false;
        switch (ev.getActionMasked()) {
            case MotionEvent.ACTION_DOWN: {
                gestureMode = 0;
                gestureStartX = ev.getX();
                gestureStartY = ev.getY();
                gestureStartTime = System.currentTimeMillis();
                gestureLastDx = 0;
                playerView.removeCallbacks(gestureLongPressRunnable);
                playerView.postDelayed(gestureLongPressRunnable, GESTURE_LONG_PRESS_MS);
                return true;
            }
            case MotionEvent.ACTION_POINTER_DOWN:
                // 多指: 视作手势取消, 各自语义安全收尾(长按恢复原速/刮擦不落 seek)
                cancelGesture();
                return true;
            case MotionEvent.ACTION_MOVE: {
                if (ev.getPointerCount() != 1) return true;
                float dx = ev.getX() - gestureStartX;
                float dy = ev.getY() - gestureStartY;
                if (gestureMode == 0) {
                    // 横向意图判定: 水平位移超阈值且强于竖直 → 进入刮擦, 同时取消长按
                    if (Math.abs(dx) > GESTURE_SWIPE_TRIGGER_PX && Math.abs(dx) > Math.abs(dy)) {
                        playerView.removeCallbacks(gestureLongPressRunnable);
                        gestureMode = 2;
                        gestureSeekBaseMs = player.getCurrentPosition();
                        gestureTargetMs = gestureSeekBaseMs;
                        gestureLastSeekAt = 0L;
                    }
                    return true; // 尚未判向或竖向滑动, 不做处理
                }
                if (gestureMode != 2) return true; // 已长按 2x: 移动不转刮擦
                gestureLastDx = dx;
                long durMs = player.getDuration();
                int width = Math.max(1, playerView.getWidth());
                // 映射: 全屏宽 ≈ 全片时长(短视频至少全屏宽≈120s), 与 web 端一致
                float perPxMs = durMs > 0 ? Math.max(durMs / (float) width, 120_000f / width) : 250f;
                long limit = durMs > 0 ? durMs - 500 : Long.MAX_VALUE;
                gestureTargetMs = Math.min(
                        Math.max(gestureSeekBaseMs + (long) (dx * perPxMs), 0L), Math.max(limit, 0L));
                long delta = gestureTargetMs - gestureSeekBaseMs;
                showCenterToast((delta >= 0 ? "快进 " : "快退 ") + fmtGestureMs(Math.abs(delta))
                                + " · " + fmtGestureMs(gestureTargetMs)
                                + " (" + (durMs > 0 ? Math.round(gestureTargetMs * 100.0 / durMs) : 0) + "%)",
                        GESTURE_HINT_KEEP_MS);
                // 长拖: 节流实时 seek 让进度条跟手; 短促轻扫留给 UP 判定固定步进
                long now = System.currentTimeMillis();
                if (now - gestureStartTime > GESTURE_FLICK_MS
                        && Math.abs(dx) > GESTURE_FLICK_PX
                        && now - gestureLastSeekAt > GESTURE_SEEK_INTERVAL_MS) {
                    gestureLastSeekAt = now;
                    player.seekTo(gestureTargetMs);
                }
                return true;
            }
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL: {
                playerView.removeCallbacks(gestureLongPressRunnable);
                int mode = gestureMode;
                gestureMode = 0;
                if (mode == 1) { // 长按结束: 恢复进入前的倍速
                    player.setPlaybackParameters(new PlaybackParameters(gestureTempRateBefore));
                    hideGestureToast();
                    return true;
                }
                if (mode == 2) {
                    long dt = System.currentTimeMillis() - gestureStartTime;
                    if (dt < GESTURE_FLICK_MS && Math.abs(gestureLastDx) < GESTURE_FLICK_PX) {
                        // 短促轻扫: 固定 ±10s
                        long target = player.getCurrentPosition()
                                + (gestureLastDx >= 0 ? 1 : -1) * GESTURE_FLICK_SEEK_SEC * 1000L;
                        long dur = player.getDuration();
                        target = Math.max(0, target);
                        if (dur > 0) target = Math.min(target, dur);
                        player.seekTo(target);
                        showCenterToast((gestureLastDx >= 0 ? "▶ 快进 " : "◀ 快退 ")
                                + GESTURE_FLICK_SEEK_SEC + "s", 800);
                    } else {
                        player.seekTo(gestureTargetMs);
                        showCenterToast("已跳转 " + fmtGestureMs(gestureTargetMs), 800);
                    }
                    return true;
                }
                // 普通单击(未形成手势): 切换控制面板
                if (System.currentTimeMillis() - gestureStartTime < 250) {
                    if (playerView.isControllerFullyVisible()) {
                        playerView.hideController();
                    } else {
                        playerView.showController(); // listener 会自动聚焦进度条
                    }
                }
                return true;
            }
            default:
                return true;
        }
    }

    /** 手势中途取消(多指/异常): 长按恢复原速, 刮擦不落 seek。 */
    private void cancelGesture() {
        playerView.removeCallbacks(gestureLongPressRunnable);
        if (gestureMode == 1 && player != null) {
            player.setPlaybackParameters(new PlaybackParameters(gestureTempRateBefore));
        }
        gestureMode = 0;
        hideGestureToast();
    }

    /** 隐藏手势提示(刮擦/长按期间用超长时长保持显示, 收尾手动清)。 */
    private void hideGestureToast() {
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        // 若仍处暂停态, 恢复"暂停图标停留"(与 showCenterToast 收尾逻辑一致)
        if (player != null && !player.getPlayWhenReady()
                && player.getPlaybackState() == Player.STATE_READY) {
            showCenterIconPersistent(R.drawable.ic_pause);
        }
    }

    /** ms → m:ss (手势提示用) */
    private static String fmtGestureMs(long ms) {
        long s = Math.max(0, ms) / 1000;
        return (s / 60) + ":" + String.format(java.util.Locale.US, "%02d", s % 60);
    }

    private void showSpeedDialog() {
        new AlertDialog.Builder(this, R.style.GfPlayerDialog)
                .setTitle("播放速度")
                .setSingleChoiceItems(SPEED_LABELS, speedIndex, (d, i) -> {
                    speedIndex = i;
                    if (player != null) {
                        player.setPlaybackParameters(new PlaybackParameters(SPEEDS[i]));
                    }
                    speedText.setText(SPEED_LABELS[i]);
                    showCenterToast("速度 " + SPEED_LABELS[i], 800);
                    d.dismiss();
                })
                .show();
    }

    /** 菜单键: 播放控制总入口 */
    /** 绑定进度条下方自定义控件按钮(控件面板每次显示时调, 幂等).
     *  已无单独播放/暂停键 — 进度条获焦时按确认键即播暂(见 dispatchKeyEvent). */
    private boolean ctlBtnsBound = false;
    private void bindControlButtons() {
        if (ctlBtnsBound) return;
        android.widget.Button prev = playerView.findViewById(R.id.btn_prev);
        android.widget.Button next = playerView.findViewById(R.id.btn_next);
        android.widget.Button speed = playerView.findViewById(R.id.btn_speed);
        android.widget.Button episodes = playerView.findViewById(R.id.btn_episodes);
        android.widget.Button source = playerView.findViewById(R.id.btn_source);
        android.widget.Button skip = playerView.findViewById(R.id.btn_skip);
        android.widget.Button close = playerView.findViewById(R.id.btn_close);
        if (close == null) return; // 自定义布局未加载(理论不至于), 安全退出

        boolean multi = sourceList.size() >= 2;
        boolean playlist = player != null && player.getMediaItemCount() > 1;
        if (prev != null) {
            prev.setVisibility(playlist ? View.VISIBLE : View.GONE);
            prev.setOnClickListener(b -> { if (player != null) player.seekToPreviousMediaItem(); });
        }
        if (next != null) {
            next.setVisibility(playlist ? View.VISIBLE : View.GONE);
            next.setOnClickListener(b -> { if (player != null) player.seekToNextMediaItem(); });
        }
        if (speed != null) speed.setOnClickListener(b -> showSpeedDialog());
        if (episodes != null) episodes.setOnClickListener(b -> showEpisodeDialog());
        if (source != null) {
            source.setVisibility(multi ? View.VISIBLE : View.GONE);
            source.setOnClickListener(b -> showSourceDialog());
        }
        if (skip != null) skip.setOnClickListener(b -> showSkipSettingsDialog());
        close.setOnClickListener(b -> finish());
        ctlBtnsBound = true;
    }

    private void showPlayMenu() {
        boolean multiSrc = sourceList.size() >= 2;
        String srcLabel = multiSrc
                ? "切换源 (" + sourceList.get(currentSourceIndex).name + ")"
                : null;
        String adLabel = "广告过滤: " + (adFilterOn ? "开" : "关");
        String[] items = multiSrc
                ? new String[]{"倍速 " + SPEED_LABELS[speedIndex], "选集", srcLabel, adLabel, "跳过设置", "关闭播放"}
                : new String[]{"倍速 " + SPEED_LABELS[speedIndex], "选集", adLabel, "跳过设置", "关闭播放"};
        new AlertDialog.Builder(this, R.style.GfPlayerDialog)
                .setTitle("播放控制")
                .setItems(items, (d, w) -> {
                    if (multiSrc) {
                        switch (w) {
                            case 0: showSpeedDialog(); break;
                            case 1: showEpisodeDialog(); break;
                            case 2: showSourceDialog(); break;
                            case 3: toggleAdFilter(); break;
                            case 4: showSkipSettingsDialog(); break;
                            case 5: finish(); break;
                        }
                    } else {
                        switch (w) {
                            case 0: showSpeedDialog(); break;
                            case 1: showEpisodeDialog(); break;
                            case 2: toggleAdFilter(); break;
                            case 3: showSkipSettingsDialog(); break;
                            case 4: finish(); break;
                        }
                    }
                })
                .show();
    }

    /** 切换源 dialog (仅多源模式) — 保留当前集数 + 当前播放时间, 不打断观看 */
    private void showSourceDialog() {
        if (sourceList.size() < 2) {
            showCenterToast("仅一个源, 无需切换", 1500);
            return;
        }
        String[] names = new String[sourceList.size()];
        for (int i = 0; i < sourceList.size(); i++) {
            SourceData s = sourceList.get(i);
            names[i] = s.name + " (" + s.urls.size() + " 集)";
        }
        final int curEp = player != null ? player.getCurrentMediaItemIndex() : 0;
        final long curPos = player != null ? player.getCurrentPosition() : 0;
        new AlertDialog.Builder(this, R.style.GfPlayerDialog)
                .setTitle("切换播放源")
                .setSingleChoiceItems(names, currentSourceIndex, (d, w) -> {
                    if (w == currentSourceIndex) { d.dismiss(); return; }
                    currentSourceIndex = w;
                    loadSourceIntoPlayer(w, curEp, curPos);
                    showCenterToast("已切到「" + sourceList.get(w).name + "」 (从 " + (curPos / 1000) + "s 续播)", 2000);
                    d.dismiss();
                })
                .show();
    }

    /** 每个选集分段(tab)的集数 — 与 web 选集分段一致 */
    private static final int EPISODE_SEG = 30;

    /** 选集 dialog: >30 集时先按 30 集一档分段选择, 再选具体集(D-pad 友好). */
    private void showEpisodeDialog() {
        if (playlistTitles == null || playlistTitles.size() < 2) {
            showCenterToast("仅一集, 无需选择", 1500);
            return;
        }
        int total = playlistTitles.size();
        int cur = player != null ? player.getCurrentMediaItemIndex() : 0;
        if (total <= EPISODE_SEG) {
            showEpisodeSegment(0, total, cur);
            return;
        }
        int segCount = (total + EPISODE_SEG - 1) / EPISODE_SEG;
        String[] segs = new String[segCount];
        for (int i = 0; i < segCount; i++) {
            int s = i * EPISODE_SEG + 1;
            int e = Math.min((i + 1) * EPISODE_SEG, total);
            segs[i] = "第 " + s + "-" + e + " 集";
        }
        new AlertDialog.Builder(this, R.style.GfPlayerDialog)
                .setTitle("选集 (共 " + total + " 集)")
                .setSingleChoiceItems(segs, cur / EPISODE_SEG, (d, w) -> {
                    d.dismiss();
                    int start = w * EPISODE_SEG;
                    showEpisodeSegment(start, Math.min(start + EPISODE_SEG, total), cur);
                })
                .show();
    }

    /** 展示 [start,end) 区间内的集供选择, 当前集在区间内则高亮. */
    private void showEpisodeSegment(int start, int end, int cur) {
        String[] arr = new String[end - start];
        for (int i = start; i < end; i++) arr[i - start] = playlistTitles.get(i);
        int checked = (cur >= start && cur < end) ? cur - start : -1;
        new AlertDialog.Builder(this, R.style.GfPlayerDialog)
                .setTitle("选集 " + (start + 1) + "-" + end)
                .setSingleChoiceItems(arr, checked, (d, w) -> {
                    if (player != null) player.seekTo(start + w, 0);
                    d.dismiss();
                })
                .show();
    }

    /** 跳过参数变化 → 通知 web 回写账号(跨设备记忆)。关闭时记 0/0(=不跳)。 */
    private void emitSkipChanged() {
        try {
            org.json.JSONObject p = new org.json.JSONObject();
            p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
            p.put("intro", skipEnabled ? skipIntroMs / 1000 : 0);
            p.put("outro", skipEnabled ? skipOutroMs / 1000 : 0);
            emit("skipSettingChanged", p);
        } catch (Exception ignore) {}
    }

    /** 跳过片头片尾设置 — 总开关 + ±10/±60 stepper, 改动即时生效并回写账号. */
    private void showSkipSettingsDialog() {
        LinearLayout ll = new LinearLayout(this);
        ll.setOrientation(LinearLayout.VERTICAL);
        int pad = (int) (16 * getResources().getDisplayMetrics().density);
        ll.setPadding(pad * 3, pad, pad * 3, pad);

        // 总开关 — 玻璃暗色弹窗内, 文字用浅色, 选中态走强调青
        Switch sw = new Switch(this);
        sw.setText("启用跳过 (开后片头" + DEFAULT_SKIP_INTRO_MS / 1000 + "s 片尾" + DEFAULT_SKIP_OUTRO_MS / 1000 + "s)");
        sw.setChecked(skipEnabled);
        sw.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 16);
        sw.setTextColor(0xFFFFFFFF);
        ll.addView(sw);

        final TextView introValue = new TextView(this);
        final TextView outroValue = new TextView(this);
        introValue.setTextColor(0xFFFFFFFF);
        outroValue.setTextColor(0xFFFFFFFF);
        // 操作即时生效: stepper 直接改 skipIntroMs/skipOutroMs, 无需"保存"
        final LinearLayout introRow = makeStepperRow(pad, -60, -10, 10, 60, (delta) -> {
            skipIntroMs = Math.max(0, skipIntroMs + delta * 1000L);
            introValue.setText("片头跳过: " + skipIntroMs / 1000 + " 秒");
            emitSkipChanged();
        });
        final LinearLayout outroRow = makeStepperRow(pad, -60, -10, 10, 60, (delta) -> {
            skipOutroMs = Math.max(0, skipOutroMs + delta * 1000L);
            outroValue.setText("片尾跳过: " + skipOutroMs / 1000 + " 秒");
            emitSkipChanged();
        });

        introValue.setText("片头跳过: " + skipIntroMs / 1000 + " 秒");
        introValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 18);
        introValue.setPadding(0, pad, 0, 0);
        ll.addView(introValue);
        ll.addView(introRow);

        outroValue.setText("片尾跳过: " + skipOutroMs / 1000 + " 秒");
        outroValue.setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 18);
        outroValue.setPadding(0, pad, 0, 0);
        ll.addView(outroValue);
        ll.addView(outroRow);

        // 开关 off 时禁用 stepper (灰显)
        Runnable applyEnabled = () -> {
            boolean on = skipEnabled;
            introValue.setAlpha(on ? 1f : 0.4f);
            outroValue.setAlpha(on ? 1f : 0.4f);
            setRowEnabled(introRow, on);
            setRowEnabled(outroRow, on);
        };
        applyEnabled.run();
        // 开关即时生效: 切换立即写 skipEnabled
        sw.setOnCheckedChangeListener((CompoundButton b, boolean isOn) -> {
            skipEnabled = isOn;
            applyEnabled.run();
            showCenterToast(isOn ? "跳过已开启" : "跳过已关闭", 1000);
            emitSkipChanged();
        });

        // 不放"完成"按钮: 开关/stepper 即时生效, 返回键关闭弹窗即可
        new AlertDialog.Builder(this, R.style.GfPlayerDialog)
                .setTitle("跳过片头 / 片尾")
                .setView(ll)
                .show();
    }

    private void setRowEnabled(LinearLayout row, boolean on) {
        for (int i = 0; i < row.getChildCount(); i++) {
            row.getChildAt(i).setEnabled(on);
        }
    }

    private interface IntConsumer { void accept(int v); }

    /** 一行 4 个 step 按钮: [-60s][-10s][+10s][+60s], D-pad 可聚焦. */
    private LinearLayout makeStepperRow(int pad, int s1, int s2, int s3, int s4, IntConsumer onStep) {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setPadding(0, pad / 2, 0, 0);
        int[] steps = {s1, s2, s3, s4};
        for (int step : steps) {
            final int s = step;
            Button b = new Button(this);
            b.setText((s > 0 ? "+" : "") + s + "s");
            b.setAllCaps(false);
            b.setFocusable(true);
            // 选中态更明显: 亮青实底 + 文字反色 + 聚焦放大
            b.setBackgroundResource(R.drawable.gf_step_btn_bg);
            b.setTextColor(getResources().getColorStateList(R.color.gf_step_btn_text));
            b.setOnFocusChangeListener((v, hasFocus) ->
                    v.animate().scaleX(hasFocus ? 1.12f : 1f).scaleY(hasFocus ? 1.12f : 1f)
                            .setDuration(120).start());
            LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(0,
                    ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
            int gap = (int) (4 * getResources().getDisplayMetrics().density);
            lp.setMargins(gap, 0, gap, 0);
            b.setLayoutParams(lp);
            b.setOnClickListener(v -> onStep.accept(s));
            row.addView(b);
        }
        return row;
    }

    private void showCenterToast(String msg, long durationMs) {
        // 与中央图标互斥(防重叠)
        if (centerIcon != null) centerIcon.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null);
        centerToast.setText(msg);
        centerToast.setVisibility(View.VISIBLE);
        toastHandler.removeCallbacksAndMessages(null);
        toastHandler.postDelayed(() -> {
            centerToast.setVisibility(View.GONE);
            // 若仍处暂停态, toast 结束后恢复"暂停图标停留"
            if (player != null && !player.getPlayWhenReady()
                    && player.getPlaybackState() == Player.STATE_READY) {
                showCenterIconPersistent(R.drawable.ic_pause);
            }
        }, durationMs);
    }

    /** 中央 播放/暂停 反馈: 统一矢量图标, 无外框, 短暂显示后淡出。与文字 toast 互斥(避免叠在一起)。 */
    private void showCenterIcon(int drawableRes, long durationMs) {
        if (centerIcon == null) return;
        // 与文字 toast 互斥
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
        iconHandler.removeCallbacksAndMessages(null);
        iconHandler.postDelayed(() -> centerIcon.setVisibility(View.GONE), durationMs);
    }

    /** 持久显示中央图标(不自动消失) — 用于"暂停时暂停图标停留在画面上"。 */
    private void showCenterIconPersistent(int drawableRes) {
        if (centerIcon == null) return;
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null); // 取消自动隐藏 → 持久停留
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
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
        // 离开/熄屏/关闭/被切后台都会走 onPause(比 onDestroy 可靠) → 立即记一次进度,
        // 防 5s tick 间隙 + 应用被系统杀掉时丢失最后进度(#13)。
        emitProgressNow();
        if (player != null && player.isPlaying()) player.pause();
    }

    /** 当前是否处于"倒数 5 分钟不记进度"区间(见 NO_RECORD_TAIL_MS)。 */
    private boolean inNoRecordTail() {
        try {
            if (player == null) return false;
            long duration = player.getDuration();
            if (duration <= NO_RECORD_TAIL_MS) return false; // 短视频不适用
            return duration - player.getCurrentPosition() <= NO_RECORD_TAIL_MS;
        } catch (Exception e) {
            return false;
        }
    }

    /** 立即上报一次当前进度(供 onPause 等关键时机, 不等 5s tick)。 */
    private void emitProgressNow() {
        try {
            if (player == null || inNoRecordTail()) return;
            org.json.JSONObject p = new org.json.JSONObject();
            p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
            p.put("episodeIndex", player.getCurrentMediaItemIndex());
            p.put("source", currentSourceLabel());
            p.put("position", player.getCurrentPosition() / 1000.0);
            emit("playerProgress", p);
        } catch (Exception ignore) {}
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        if (sCurrentInstance == this) sCurrentInstance = null;
        outroHandler.removeCallbacksAndMessages(null);
        toastHandler.removeCallbacksAndMessages(null);
        iconHandler.removeCallbacksAndMessages(null);
        progressHandler.removeCallbacksAndMessages(null);
        // 通知 web 播放结束 / 用户退出. 加 filmId — App.vue 的 writeProgress 没 filmId
        // 就 return, 之前 onDestroy 这里不传 filmId 导致退出后 web 详情页没收到最后一次
        // 进度同步, 用户感觉"退出后集数/进度还是老的"。
        // #4: 退出时正处于倒数 5 分钟内 → 整个 playerClosed 不发, web 不写播放记忆
        // (否则 App.vue 会以 position=0 兜底写入, 反而清掉已有进度)。
        // 播放完成(ENDED): 照常发 playerClosed 且带 ended=true → web 清该集播放记忆。
        boolean playbackEnded = player != null && lastPlayerState == androidx.media3.common.Player.STATE_ENDED;
        if (playbackEnded || !(player != null && inNoRecordTail())) {
            try {
                org.json.JSONObject p = new org.json.JSONObject();
                p.put("filmId", getIntent().getStringExtra(EXTRA_FILM_ID));
                if (player != null) {
                    p.put("position", player.getCurrentPosition() / 1000.0);
                    p.put("duration", player.getDuration() > 0 ? player.getDuration() / 1000.0 : 0);
                    p.put("episodeIndex", player.getCurrentMediaItemIndex());
                    p.put("source", currentSourceLabel());
                    if (playbackEnded) p.put("ended", true);
                }
                emit("playerClosed", p);
            } catch (Exception ignore) {}
        }
        if (player != null) {
            player.release();
            player = null;
        }
        // sCache 故意不 release: 进程内复用, 退出 PlayerActivity 时缓存继续保留供下次用
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        // 与 onCreate 一致重读代理 base(漏读会致 relaunch 后 proxyBase 用空值 → 切集"过滤未生效")。
        proxyBase = resolveProxyBase(intent);
        // 与 onCreate 一致地按新片重置跳过设置(含开关), 否则换片会沿用上一片的 skipEnabled → "跳过没按片记忆"。
        long iMs = intent.getLongExtra(EXTRA_SKIP_INTRO_MS, 0L);
        long oMs = intent.getLongExtra(EXTRA_SKIP_OUTRO_MS, 0L);
        skipIntroMs = iMs > 0 ? iMs : DEFAULT_SKIP_INTRO_MS;
        skipOutroMs = oMs > 0 ? oMs : DEFAULT_SKIP_OUTRO_MS;
        skipEnabled = (iMs > 0 || oMs > 0);
        autoNext = getIntent().getBooleanExtra(EXTRA_AUTO_NEXT, true);
        introSkippedForCurrent = false;
        startFromIntent(intent);
    }
}
