package com.jerocine.player;

import android.content.Context;
import android.content.Intent;
import android.net.Uri;
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
import androidx.media3.datasource.DataSpec;
import androidx.media3.datasource.HttpDataSource;
import androidx.media3.datasource.TransferListener;
import androidx.media3.datasource.cache.Cache;
import androidx.media3.datasource.cache.CacheDataSource;
import androidx.media3.datasource.cache.LeastRecentlyUsedCacheEvictor;
import androidx.media3.datasource.cache.SimpleCache;
import androidx.media3.datasource.okhttp.OkHttpDataSource;
import androidx.media3.exoplayer.DefaultLoadControl;
import androidx.media3.exoplayer.DefaultRenderersFactory;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.LoadControl;
import androidx.media3.exoplayer.hls.HlsMediaSource;
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory;
import androidx.media3.ui.PlayerView;

import com.jerocine.player.download.DownloadEngine;

import org.json.JSONObject;

import java.io.File;
import java.io.IOException;
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
    /** 底栏"切本地/切在线"按钮(仅当前源本集有本地副本时可见; 文案随当前态翻转)。 */
    private Button btnLocalToggle;
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

        // 本地文件打开(文件管理器「打开方式」ACTION_VIEW): 立即标记本地模式,
        // 后续跳过 HLS 广告过滤链路, 用 DefaultMediaSourceFactory 自动识别 mp4/ts/m3u8.
        if (LocalPlayback.isLocalAction(getIntent().getAction()) && getIntent().getData() != null) {
            session.localPlayback = true;
            session.localUri = getIntent().getData().toString();
            session.localTitle = LocalPlayback.displayTitle(
                    getIntent().getData().getLastPathSegment(),
                    getIntent().getStringExtra(EXTRA_TITLE));
        }

        // 下载页「播放」无影片上下文时: EXTRA_URL 是本地过滤后清单(file://) + EXTRA_CACHE_DIR
        // 指到下载缓存区(供 buildCacheFactory 复用引擎缓存实例)。不再置独立"离线模式":
        // startFromIntent 走单 URL 兼容装载后, 会话即 isSingleLocalPlaylist(),
        // 与"已下载集播本地"共用同一套判定(见 PlayerModes 类注释)。

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
        // 开关初值统一走静态 getter(键名/默认值定义在 PlayerAdFilterHelper, 不在各处硬编码)
        session.adFilterOn = PlayerAdFilterHelper.isAdFilterEnabled(this);
        networkModeHelper.applyPersisted(); // 中转默认关(分片直连), 只有用户主动开过才为 true
        // 本地模式不显示广告过滤角标: 隐藏它的 hideOnlineControls() 只在控制面板首次变为
        // VISIBLE 时才被调用(见下方 ControllerVisibilityListener), 而面板默认 GONE
        // (setControllerAutoShow(false)) → 不加守卫的话, 本地文件起播后会闪现一个
        // 与本地播放毫无关系的"广告过滤"状态点, 直到用户首次唤出控制面板才消失。
        if (!session.localPlayback) {
            adFilterHelper.updateAdFilterBadge();
        }

        View titleBar = findViewById(R.id.title_bar);
        // 面板可见时: ① 标题栏跟随显隐 ② 进度条默认获焦(用户期望"面板出→进度条选中")
        playerView.setControllerVisibilityListener((PlayerView.ControllerVisibilityListener) v -> {
            titleBar.setVisibility(v);
            // 控制面板根节点在 PlayerView 内部(懒加载), 每次显隐时现取, 避免 onCreate 时为空
            View controlsRoot = playerView.findViewById(R.id.player_controls_root);
            if (controlsRoot != null) controlsRoot.setVisibility(v);
            if (v == View.VISIBLE) {
                // "过滤"/"倍速"/"跳过"/"切本地/在线"按钮(含左上角状态点)在 PlayerView 的控制视图里(懒加载): 面板显示时取到
                adFilterButton = playerView.findViewById(R.id.btn_ad_filter);
                dotAdFilter = playerView.findViewById(R.id.dot_ad_filter);
                dotSpeed = playerView.findViewById(R.id.dot_speed);
                dotSkip = playerView.findViewById(R.id.dot_skip);
                btnLocalToggle = playerView.findViewById(R.id.btn_local_toggle);
                if (session.localPlayback || session.isSingleLocalPlaylist()) {
                    hideOnlineControls(); // 本地文件/单集本地: 隐藏在线专属控件(过滤/换源/选集/上下集/跳过)
                    // 倍速/退出对本地文件同样有意义(可调速、可退出), 且不在 hideOnlineControls 的
                    // 隐藏列表里 —— 所以**必须**绑定监听器, 否则它们是"可见但点不动"的死按钮。
                    // TV 遥控器上表现为"按了没反应", 很容易被当成播放器卡死。
                    dialogHelper.bindControlButtons();
                    renderSpeedDot(isSpeedOn());
                } else {
                    dialogHelper.bindControlButtons();
                    renderAdFilterSwitch(session.adFilterOn);
                    renderSpeedDot(isSpeedOn());
                    renderSkipDot(session.skipEnabled);
                }
                bindMoreMenu(); // 在线与本地模式都要(本地=选择本地文件/诊断)
                updateLocalOnlineUi(); // 按当前集本地/在线态刷新角标与切换按钮(每面板次显隐都要重算)
                playerView.post(() -> {
                    View prog = playerView.findViewById(androidx.media3.ui.R.id.exo_progress);
                    if (prog != null) prog.requestFocus();
                });
            }
        });
        // 切集/缓冲/暂停时不自动弹面板 — 仅用户显式唤起(确认键/点击)才显示
        playerView.setControllerAutoShow(false);
        playerView.setOnTouchListener(gestureHelper::onPlayerTouch);
        // 暂停/缓冲中面板不主动消失(策略见 PlayerAutoHidePolicy); 起播前也按当前态初始化
        playerView.setControllerShowTimeoutMs(
                PlayerAutoHidePolicy.timeoutMs(false, true));

        applySkipSettingsFromIntent(getIntent());

        if (session.localPlayback) {
            initLocalPlayer();
        } else {
            initPlayer();
            sourceHelper.startFromIntent(getIntent());
            // 单集本地清单(下载页无上下文播单集): 把清单登记进本地副本表,
            // 之后 isEpisodePlayingLocal(0) 恒为 true —— 角标/按钮/自愈/预取与"已下载集播本地"同轨。
            if (session.isSingleLocalPlaylist() && !session.currentRawUrls.isEmpty()) {
                java.util.Map<Integer, String> m = new java.util.HashMap<>();
                m.put(0, session.currentRawUrls.get(0));
                session.localEpisodePlaylists = m;
            }
        }
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

    /**
     * 本地文件播放器: 不经 HLS 广告过滤链路, 用 DefaultMediaSourceFactory 按扩展名自动识别
     * mp4 / 裸 ts / 本地 m3u8(相对分片由 media3 按文件目录解析). 无缓存(文件已本地),
     * 复用同一套渲染器/缓冲参数/分辨率角标.
     */
    private void initLocalPlayer() {
        DefaultRenderersFactory renderersFactory = new DefaultRenderersFactory(this)
                .setEnableDecoderFallback(true)
                .forceEnableMediaCodecAsynchronousQueueing();
        LoadControl loadControl = new DefaultLoadControl.Builder()
                .setBufferDurationsMs(30_000, 120_000, 1_500, 2_500)
                .setPrioritizeTimeOverSizeThresholds(true)
                .setBackBuffer(30_000, true)
                .build();
        final ExoPlayer player = new ExoPlayer.Builder(this, renderersFactory)
                .setMediaSourceFactory(new DefaultMediaSourceFactory(this))
                .setLoadControl(loadControl)
                .build();
        session.player = player;
        playerView.setPlayer(player);

        player.addListener(new Player.Listener() {
            @Override
            public void onPlaybackStateChanged(int state) {
                lastPlayerState = state;
                bufferSpinner.setVisibility(state == Player.STATE_BUFFERING ? View.VISIBLE : View.GONE);
                updateControllerAutoHide(); // 暂停/缓冲中面板不主动消失(本地播放同轨)
            }

            @Override
            public void onPlayWhenReadyChanged(boolean playWhenReady, int reason) {
                updateControllerAutoHide();
            }

            @Override
            public void onVideoSizeChanged(@NonNull VideoSize videoSize) {
                if (resolutionBadge == null) return;
                String label = PlayerQuality.resolutionLabel(videoSize.width, videoSize.height);
                resolutionBadge.setVisibility(label == null ? View.GONE : View.VISIBLE);
                if (label != null) resolutionBadge.setText(label);
            }

            @Override
            public void onPlayerError(@NonNull PlaybackException error) {
                showCenterToast("本地文件播放失败: " + error.getErrorCodeName(), 2600);
            }
        });

        titleText.setText(session.localTitle.isEmpty() ? "本地视频" : session.localTitle);
        player.setMediaItem(MediaItem.fromUri(Uri.parse(session.localUri)));
        player.prepare();
        player.setPlayWhenReady(true);
    }

    private void initPlayer() {
        DataSource.Factory cacheFactory = buildCacheFactory();
        // 端侧混合广告过滤: 全 HLS 内容用自定义播放列表解析器, 抓到 m3u8 后送服务端剔除广告再解析.
        // 本地清单(file://)天然安全: FilterPlaylistParser 经 PlayerUrls.needsClientSideFilter
        // 对 file:///content:// 直接放行(不 POST、不升级代理), 无需按会话形态切换解析器 ——
        // 统一挂在工厂上, 在线/本地分片装载同一条路(2026-10-09 去掉 offlinePlayback 特例)。
        HlsMediaSource.Factory msFactory = new HlsMediaSource.Factory(cacheFactory);
        msFactory.setPlaylistParserFactory(adFilterHelper.new FilterPlaylistParserFactory());
        msFactory.setAllowChunklessPreparation(true);
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
                updateControllerAutoHide(); // 暂停/缓冲中面板不主动消失
                updateFilterLoadingText(state);
                if (state == Player.STATE_READY) {
                    session.episodeSwitching = false;
                    if (!session.introSkippedForCurrent) {
                        skipHelper.applySkipIntro();
                        session.introSkippedForCurrent = true;
                    }
                    // 起播弹一次"过滤状态"(每集一次); 本地集清单已过滤, 无过滤状态可弹(角标显示"本集本地播放")
                    if (!session.filterToastShownForEpisode
                            && !session.isEpisodePlayingLocal(session.player.getCurrentMediaItemIndex())) {
                        session.filterToastShownForEpisode = true;
                        adFilterHelper.showFilterStatus();
                    }
                }
            }

            @Override
            public void onPlayWhenReadyChanged(boolean playWhenReady, int reason) {
                updateControllerAutoHide(); // 暂停/缓冲中面板不主动消失
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
            public void onMediaItemTransition(MediaItem mediaItem, int reason) {
                session.introSkippedForCurrent = false;
                session.outroPromptShown = false;
                session.episodeSwitching = true;
                session.resetFilterStateForEpisode();
                updateTitleForCurrent();
                session.updateNetworkModeUi();
                updateLocalOnlineUi(); // 切集后按新集本地/在线态刷新角标与切换按钮
                maybeOtherSourceToast();
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
                HttpFailure httpFail = extractHttpFailure(error);
                String failedUrl = httpFail.url;
                final int errIdx = session.player != null
                        ? session.player.getCurrentMediaItemIndex() : -1;

                // 【本地优先播放的本集失败】当前 item 是本地 file:// 清单 → 本地缓存异常
                // (清单损坏/分片缺失), 线路自愈无意义; 自动切回在线一次(preferOnlineIdx.add
                // 已存在时返回 false, 天然防循环), 用户仍可在底栏再切回本地。
                if (currentUrl.startsWith("file://") && !session.isSingleLocalPlaylist()
                        && errIdx >= 0 && errIdx < session.currentRawUrls.size()
                        && session.preferOnlineIdx.add(errIdx)) {
                    PlayerControl.get().clearPlaybackFailure();
                    session.retryCurrentItem(errIdx, "本地缓存读取失败, 已切换在线播放");
                    return;
                }

                // 直连失败(端侧过滤下设备自己抓清单/分片) → 本集改走全量中转(自愈).
                // 前两版只在"当前是 proxy 清单"时才自愈; 端侧混合过滤成为主路径后当前地址是**原始 m3u8**,
                // 所以判据放宽为"失败的不是代理请求本身"(见 PlayerUrls.shouldRetryWithRelay).
                // 服务端抓不到该源的(proxyUsable=false)中转也无意义, 直接走下面的报错/回退.
                // 本地播放: 分片要么在缓存/本地盘里, 要么就得重新下载, 换线路/换代理救不了
                // (file:// 被包进代理只会让服务端去抓一个不存在的本地地址) → 不做任何线路自愈。
                if (!session.isEpisodePlayingLocal(errIdx) && session.adFilterOn && session.sourceProxyUsable
                        && PlayerUrls.shouldRetryWithRelay(currentUrl, failedUrl)
                        && errIdx >= 0 && errIdx < session.currentRawUrls.size()
                        && !session.forceRelayIdx.contains(errIdx)) {
                    session.forceRelayIdx.add(errIdx);
                    session.forceRawIdx.remove(errIdx);
                    PlayerControl.get().clearPlaybackFailure();
                    session.retryCurrentItem(errIdx, "直连失败, 已切换中转");
                    return;
                }
                // 【新增第三档自愈】中转也失败后, 再回退到**原始源直连**试一次。
                // 为什么需要: forceRelayIdx 是本集**永久**标记, 原实现在"已标记"时直接落到
                // 错误上报 —— 于是"直连失败→切中转→中转也失败"这一条常见路径走到死胡同,
                // 用户只能退出重进。而实际现场(2004/404)恰恰就是这种形态:
                // 源站把旧哈希路径删了(index.m3u8 404), 中转只是转发源站响应, 救不了;
                // 但换回原始源/换一条线路往往能拿到现存的路径。
                // forceRelay/forceRaw 互斥标记天然限制了重试轮数(每集最多来回一次),
                // 不会无限循环。
                if (!session.isEpisodePlayingLocal(errIdx) && session.adFilterOn
                        && session.forceRelayIdx.contains(errIdx)
                        && !session.forceRawIdx.contains(errIdx)
                        && errIdx >= 0 && errIdx < session.currentRawUrls.size()) {
                    session.forceRawIdx.add(errIdx);
                    session.forceRelayIdx.remove(errIdx);
                    PlayerControl.get().clearPlaybackFailure();
                    session.retryCurrentItem(errIdx, "中转也失败, 已回退原始源");
                    return;
                }
                // 服务端无法抓取清单时, 仅本集回退原始源(广告不过滤, 但保证能放)
                if (!session.isEpisodePlayingLocal(errIdx) && session.adFilterOn && currentUrl.toLowerCase(Locale.US).contains("/m3u8/proxy")
                        && !currentUrl.toLowerCase(Locale.US).contains("proxymedia=1")
                        && (failedUrl.isEmpty()
                            || failedUrl.toLowerCase(Locale.US).contains("/m3u8/proxy"))
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
                    // 供前端上报/日志用: 脱敏版(不含源站签名)。
                    // 注意不要改成完整 URL —— 前端日志/上报链路会把签名一起带走。
                    // 若前端确实需要原地址做换源重试, 应另行走壳层接口而非事件字段。
                    p.put("currentUrl", ErrorDiag.safeUrl(currentUrl));
                    p.put("causeDetail", causeDetail);
                    // 【关键】真实 HTTP 状态码: errorCode 2004 只说明"非 2xx",
                    // 把 403(防盗链/签名过期) / 404(源站路径失效) / 429(限流) 混在一起,
                    // 后台无法区分根因。加了这两个字段才能按 status 分布看问题:
                    //   403 集中 → 源站防盗链或签名过期, 需重取清单
                    //   404 集中 → 源站把旧哈希路径删了(只能重试/换源)
                    //   429 集中 → 被限流, 必须退避
                    p.put("httpStatus", httpFail.statusCode);
                    p.put("failedUrl", ErrorDiag.safeUrl(failedUrl));
                    p.put("episodeIndex", errIdx);
                    // 自愈状态: 能直接看出"这次上报前是不是已经自愈过"(relayFlagged=true
                    // 说明已试过中转、rawFlagged=true 说明已试过回退原始源),
                    // 不用再从日志时序推断。
                    p.put("relayFlagged", session.forceRelayIdx.contains(errIdx));
                    p.put("rawFlagged", session.forceRawIdx.contains(errIdx));
                    emit("playerError", p);
                } catch (Exception ignore) {
                }
                // URL 必须脱敏后再上屏: 代理 URL 的尾部恰好是 src=<编码后的源站 URL>,
                // 而源站常带时效签名(?token=xxx&sign=yyy) → 直接显示尾部 60 字符会把
                // 签名印在电视屏幕上, 截图/录屏/直播都会扩散出去。
                // 保留 host + 路径末两段 + 非签名参数, 诊断信息基本不丢。
                String safeUrl = ErrorDiag.safeUrl(currentUrl);
                // 状态码直接打屏: 用户截图就能定性, 不必再翻后台。
                // 0 = 非 HTTP 类失败(连接被重置/超时/解析), 此时不显示以免误读成"状态码 0"。
                String statusText = httpFail.statusCode > 0 ? (" HTTP " + httpFail.statusCode) : "";
                showCenterToast("播放出错 " + error.errorCode + " ("
                        + error.getErrorCodeName() + ")" + statusText + "\n"
                        + safeUrl + causeDetail, 8000);
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
        // 关键: 下载缓存区已被 DownloadEngine 的 SimpleCache 进程级独占(见 heldCacheFor 注释),
        // 这里绝不能自己 new —— 会抛 IllegalStateException 导致离线播放点「播放」必崩。
        // 复用引擎实例, 同时保住它的 NoOpCacheEvictor(已下载分片不参与淘汰)。
        SimpleCache engineCache = DownloadEngine.heldCacheFor(wantDir);
        if (engineCache != null) {
            sCacheDir = wantDir;
            sCache = engineCache;
        } else {
            if (sCache != null && !wantDir.equals(sCacheDir)) {
                try {
                    sCache.release();
                } catch (Exception ignore) {
                }
                sCache = null;
            }
            if (sCache == null) {
                sCacheDir = wantDir;
                sCache = new SimpleCache(
                        sCacheDir,
                        new LeastRecentlyUsedCacheEvictor(CACHE_SIZE),
                        new StandaloneDatabaseProvider(getApplicationContext()));
            }
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
        OkHttpDataSource.Factory okHttp = new OkHttpDataSource.Factory(http)
                .setUserAgent("Jerocine/1.0 (Android TV)");
        // 【2026-10-08 修"离线播放 Malformed URL"】上游必须包一层 DefaultDataSource:
        // 离线播放的清单是 file://(下载时过滤落库的本地副本), OkHttpDataSource 对
        // 非 http(s) 直接抛 "Malformed URL"; DefaultDataSource 会把 file:// 委给
        // FileDataSource、http(s) 委给 base 工厂(OkHttp), 两类来源都通。
        // 在线播放行为不变(全部来源都是 http(s), DefaultDataSource 纯转发)。
        androidx.media3.datasource.DefaultDataSource.Factory upstream =
                new androidx.media3.datasource.DefaultDataSource.Factory(this, okHttp);
        DataSource.Factory onlineFactory = new CacheDataSource.Factory()
                .setCache(sCache)
                .setUpstreamDataSourceFactory(upstream)
                .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR);
        // 【本地优先播放】在线会话里, 已下载集的清单是 file://、分片在**下载缓存区**
        // (key = 源站分片 URL) —— sCache(video_cache) 里根本没有它们。用 RoutingDataSource
        // 把"已下载"的分片请求路由到 DownloadEngine 的下载缓存实例(命中即零网络,
        // 缺口部分经 upstream 回源, 无害), 其余请求照走 video_cache。
        // 判定用缓存索引内存查询(微秒级); 离线播放(sCache 本身就是下载缓存)无需路由。
        if (engineCache != null) return onlineFactory; // 离线播放: 单缓存
        DownloadEngine engine = DownloadEngine.existing();
        if (engine == null) return onlineFactory;
        final Cache dlCache = engine.cache();
        DataSource.Factory downloadedFactory = new CacheDataSource.Factory()
                .setCache(dlCache)
                .setUpstreamDataSourceFactory(upstream)
                .setFlags(CacheDataSource.FLAG_IGNORE_CACHE_ON_ERROR);
        return () -> new RoutingDataSource(dlCache,
                onlineFactory.createDataSource(), downloadedFactory.createDataSource());
    }

    /**
     * 按"分片是否已在下载缓存区"路由的数据源 — 本地优先播放的分片拾取器。
     * 清单(file://)与未下载分片走常规链路(video_cache); 已下载分片直读下载缓存。
     */
    private static final class RoutingDataSource implements DataSource {
        private final Cache downloadCache;
        private final DataSource online;
        private final DataSource downloaded;
        private DataSource active;

        RoutingDataSource(Cache downloadCache, DataSource online, DataSource downloaded) {
            this.downloadCache = downloadCache;
            this.online = online;
            this.downloaded = downloaded;
        }

        @Override
        public void addTransferListener(TransferListener transferListener) {
            // 两个 delegate 都挂: open 前不知道会路由到哪边, 索性两边都登记
            online.addTransferListener(transferListener);
            downloaded.addTransferListener(transferListener);
        }

        @Override
        public long open(DataSpec dataSpec) throws IOException {
            String scheme = dataSpec.uri.getScheme();
            boolean http = "http".equalsIgnoreCase(scheme) || "https".equalsIgnoreCase(scheme);
            boolean hit = false;
            if (http) {
                try {
                    hit = downloadCache.getCachedBytes(
                            dataSpec.uri.toString(), 0, androidx.media3.common.C.LENGTH_UNSET) > 0;
                } catch (Exception ignore) {
                    // 索引查询异常按未命中处理, 走在线链路
                }
            }
            active = hit ? downloaded : online;
            return active.open(dataSpec);
        }

        @Override
        public int read(byte[] buffer, int offset, int length) throws IOException {
            return active.read(buffer, offset, length);
        }

        @Override
        public Uri getUri() {
            return active == null ? null : active.getUri();
        }

        @Override
        public java.util.Map<String, java.util.List<String>> getResponseHeaders() {
            return active == null ? java.util.Collections.emptyMap()
                    : active.getResponseHeaders();
        }

        @Override
        public void close() throws IOException {
            if (active != null) {
                try {
                    active.close();
                } finally {
                    active = null;
                }
            }
        }
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

/**
 * 一次遍历异常链, 同时取出「失败请求 URL」与「HTTP 状态码」。
 *
 * <p><b>为什么必须补状态码</b>: {@code PlaybackException} 的 errorCode 粒度太粗 ——
 * {@code 2004 (ERROR_CODE_IO_BAD_HTTP_STATUS)} 把403(防盗链/签名过期)、404(源站路径失效)、
 * 429(限流) 全混在一起, 后台只能靠猜。而这三者的根因与后续策略完全不同:
 * 403 要重取清单换签名、404 是源站把旧哈希路径删了(只能重试/换源)、
 * 429 必须退避。之前只能靠用户截图才能定位, 就是因为这里没埋。
 *
 * <p>状态码在 {@code HttpDataSource.InvalidResponseCodeException.responseCode} 上 ——
 * 注意父类 {@code HttpDataSourceException} 只有 dataSpec/type, **没有** status,
 * 所以必须判子类(已核对 media3 1.4.1 的类结构)。
 */
private static final class HttpFailure {
    String url = "";
    int statusCode;
}

private static HttpFailure extractHttpFailure(Throwable error) {
    HttpFailure out = new HttpFailure();
    Throwable cause = error;
    while (cause != null) {
        if (cause instanceof HttpDataSource.InvalidResponseCodeException) {
            HttpDataSource.InvalidResponseCodeException e =
                    (HttpDataSource.InvalidResponseCodeException) cause;
            if (out.url.isEmpty() && e.dataSpec != null && e.dataSpec.uri != null) {
                out.url = e.dataSpec.uri.toString();
            }
            if (out.statusCode == 0) out.statusCode = e.responseCode;
        } else if (cause instanceof HttpDataSource.HttpDataSourceException) {
            // 非 2xx 之外的 HTTP 错误(如连接被重置)也带 dataSpec, 状态码留 0
            HttpDataSource.HttpDataSourceException e =
                    (HttpDataSource.HttpDataSourceException) cause;
            if (out.url.isEmpty() && e.dataSpec != null && e.dataSpec.uri != null) {
                out.url = e.dataSpec.uri.toString();
            }
        }
        cause = cause.getCause();
    }
    return out;
}

/** 从异常链里捞出真正失败的请求 URL(HttpDataSourceException 会带 dataSpec)。 */
private static String failedRequestUrl(Throwable error) {
    return extractHttpFailure(error).url;
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
        // 复位手势长按 2x 倍速: 用户长按视频区进入 2x 后未抬手就按 HOME/切后台时,
        // MotionEvent 不再派发 → cancelGesture() 不会被调用(它只在 ACTION_POINTER_DOWN 里触发),
        // 回来后播放器继续以 2.0 倍速播放且倍速状态点还亮着, 用户不知道是残留。
        if (gestureHelper != null) gestureHelper.cancelGesture();
        if (session.player != null && session.player.isPlaying()) session.player.pause();
    }

    private void emitProgressNow() {
        try {
            if (session.localPlayback || session.isSingleLocalPlaylist()) return; // 本地模式无影片/剧集上下文, 不上报进度
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
        // 本地模式: 不向壳层上报播放结局(无 web 心跳/无剧集上下文)
        if (!session.localPlayback && !session.isSingleLocalPlaylist()
                && (playbackEnded || !(player != null && skipHelper.inNoRecordTail()))) {
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
        // sCache 故意不 release: 进程内复用, 退出时缓存继续保留供下次用。
        // 离线播放时 sCache 是 DownloadEngine 持有的实例(见 buildCacheFactory) ——
        // 那更不能 release: 引擎无release 出口, 解锁后重新 new 会与在途下载抢同一目录。
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        // 本地文件再打开(文件管理器在应用存活时再次调起 / 本地模式再选文件):
        // recreate 重走 onCreate 装配本地模式, 旧实例走 onDestroy 释放.
        if (LocalPlayback.isLocalAction(intent.getAction()) && intent.getData() != null) {
            setIntent(intent);
            recreate();
            return;
        }
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

    /** 面板显示时绑定右上角「⋮ 更多」菜单(每次现取, 与其它懒加载控件一致).
     *  注意: btn_more 在 activity_player.xml 的标题栏(title_bar)里, 是 PlayerView 的兄弟节点,
     *  必须用 Activity 根视图 findViewById —— playerView.findViewById 会返回 null, 点击无响应. */
    private void bindMoreMenu() {
        Button more = findViewById(R.id.btn_more);
        if (more == null) return;
        more.setOnClickListener(v -> showMoreMenu());
    }

    /**
     * 本地模式: 隐藏在线专属控件(广告过滤/换源/选集/上下集/跳片头 + 标题栏角标),
     * 保留: 倍速/进度/手势/分辨率/更多(本地播放入口与诊断).
     */
    private void hideOnlineControls() {
        int[] inPlayerView = {R.id.btn_ad_filter, R.id.dot_ad_filter, R.id.btn_source,
                R.id.btn_episodes, R.id.btn_prev, R.id.btn_next, R.id.btn_skip, R.id.dot_skip,
                R.id.btn_local_toggle};
        for (int id : inPlayerView) {
            View v = playerView != null ? playerView.findViewById(id) : null;
            if (v != null) v.setVisibility(View.GONE);
        }
        if (adFilterBadge != null) adFilterBadge.setVisibility(View.GONE);
        if (episodesCount != null) episodesCount.setVisibility(View.GONE);
    }

    /**
     * 更多菜单 — 收纳非常用项(用户拍板): 下载管理 / 缓存缓冲 / 中转 / 诊断.
     * 播放器右上角保留原有显示(过滤广告/倍速/分辨率/总集数)不变。
     * 本地模式: 只留「选择本地文件播放 / 诊断信息」。
     *
     * <p><b>不含分辨率</b>(用户拍板): 原先有一项只做 toast 展示、点下去什么也不做,
     * 而右上角常有分辨率角标 —— 不能切换的项放进菜单只会让人以为能点, 故移除。
     * 若将来真要做分辨率切换, 应做成带 RadioGroup 的子菜单而不是这一行。
     */
    private void showMoreMenu() {
        if (session.localPlayback) {
            final String[] localItems = {"选择本地文件播放", "诊断信息"};
            com.jerocine.player.ui.JcDialog.list(this)
                    .title("更多")
                    .items(localItems)
                    .onItemClick((d, i) -> {
                        if (i == 0) openLocalFilePicker();
                        else if (i == 1) showDiagnostics();
                    })
                    .show();
            return;
        }
        // 单集本地清单(下载页无上下文播单集): 无在线可切, 缓冲/中转也无指代对象 → 只留下载管理与诊断
        if (session.isSingleLocalPlaylist()) {
            final String[] offItems = {"下载管理", "诊断信息"};
            com.jerocine.player.ui.JcDialog.list(this)
                    .title("更多")
                    .items(offItems)
                    .onItemClick((d, i) -> {
                        if (i == 0) openDownloadManager();
                        else if (i == 1) showDiagnostics();
                    })
                    .show();
            return;
        }
        final java.util.List<String> items = new java.util.ArrayList<>();
        final java.util.List<Runnable> actions = new java.util.ArrayList<>();
        items.add("下载管理");
        actions.add(this::openDownloadManager);
        // 本地/在线分集判定: 本地集的"缓冲/中转"无指代对象(分片已在本地), 不展示
        int curIdx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
        boolean curLocal = curIdx >= 0 && session.isEpisodePlayingLocal(curIdx);
        if (!curLocal) {
            items.add("缓存缓冲");
            actions.add(this::showBufferDialog);
            items.add("中转：" + (session.relayOn ? "开" : "关"));
            actions.add(this::toggleNetworkMode);
        }
        // 本集本地⇄在线切换已移至底栏按钮(去广告右侧, 用户拍板); 菜单里只保留"别源已下载"的换源入口
        android.util.Log.i("JcLocal", "more menu: curIdx=" + curIdx
                + " curLocal=" + curLocal
                + " mapSize=" + session.localEpisodePlaylists.size());
        if (curIdx >= 0 && session.otherSourceHasEpisode(curIdx)) {
            // 本集在别的源有已完成下载(如下载时 lz 源、续播恢复成 bf 源): 严格匹配下
            // 不自动播本地, 但必须给用户一条可达路径 —— 一键换到已下载的源并本地起播。
            com.jerocine.player.download.DownloadTask t = session.otherSourceTask(curIdx);
            int srcIdx = session.indexOfSourceKey(t.sourceKey);
            if (srcIdx >= 0) {
                final int idx = curIdx;
                final int target = srcIdx;
                String srcLabel = (t.sourceName != null && !t.sourceName.isEmpty())
                        ? t.sourceName : t.sourceKey;
                items.add("本集已在「" + srcLabel + "」源下载 · 换源播本地");
                actions.add(() -> {
                    long pos = session.player != null ? session.player.getCurrentPosition() : 0L;
                    // 与 PlayerDialogHelper 的换源流程一致: 必须先更新 currentSourceIndex,
                    // 否则 loadPlaylistIntoPlayer 里的 refreshLocalDownloads 仍按旧源查, 本地优先不生效。
                    session.currentSourceIndex = target;
                    session.loadSourceIntoPlayer(target, idx, pos);
                    session.host().showCenterToast("已切到「"
                            + session.sourceList.get(target).name + "」源本地播放", 2000);
                });
            }
        }
        items.add("诊断信息");
        actions.add(this::showDiagnostics);
        com.jerocine.player.ui.JcDialog.list(this)
                .title("更多")
                .items(items.toArray(new String[0]))
                .onItemClick((d, i) -> {
                    if (i >= 0 && i < actions.size()) actions.get(i).run();
                })
                .show();
    }

    /** 更多菜单 → 本集本地/在线切换: 切后重装当前集并保留进度。 */
    /** 底栏「切本地/切在线」(Host 回调): 按当前集翻转 preferOnlineIdx 并原位重装(保留进度). */
    @Override
    public void toggleLocalOnline() {
        int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
        if (!session.isEpisodeLocalAvailable(idx)) return; // 无本地副本/单集本地: 无可切
        if (session.preferOnlineIdx.contains(idx)) {
            session.preferOnlineIdx.remove(idx);
            session.retryCurrentItem(idx, "已切换本地播放");
        } else {
            session.preferOnlineIdx.add(idx);
            session.retryCurrentItem(idx, "已切换在线播放");
        }
        updateLocalOnlineUi();
    }

    /**
     * 按当前集本地/在线态刷新两处 UI(切集/切换/面板显隐时都要调):
     * <ul>
     *   <li>右上角原"去广告"角标位: 本地集显示「本集本地播放」, 在线集维持过滤角标不动
     *       (在线是默认态, 不做额外标识 —— 用户拍板);</li>
     *   <li>底栏「切本地/切在线」: 仅当前源本集有本地副本且存在在线替代时可见;
     *       本地集同时隐藏"去广告"按钮与状态点(本地分片已过滤过, 开关无指代对象)。</li>
     * </ul>
     */
    private void updateLocalOnlineUi() {
        if (session.localPlayback) return; // 本地文件模式: 控件已整体隐藏(hideOnlineControls), 无集概念
        int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
        boolean local = session.isEpisodePlayingLocal(idx);
        boolean avail = session.isEpisodeLocalAvailable(idx);
        if (local) {
            renderLocalBadge();
        } else if (idx >= 0) {
            adFilterHelper.updateAdFilterBadge(); // 恢复常规过滤角标(切回在线时)
        }
        if (adFilterButton != null) adFilterButton.setVisibility(local ? View.GONE : View.VISIBLE);
        if (dotAdFilter != null) dotAdFilter.setVisibility(local ? View.GONE : View.VISIBLE);
        if (btnLocalToggle != null) {
            btnLocalToggle.setVisibility(avail ? View.VISIBLE : View.GONE);
            if (avail) btnLocalToggle.setText(PlayerModes.localToggleText(local));
        }
    }

    /** 右上角角标 → 「本集本地播放」(绿点; 占原"去广告"角标位). */
    private void renderLocalBadge() {
        if (adFilterBadge == null) return;
        adFilterBadge.setText(PlayerModes.localBadgeText());
        adFilterBadge.setCompoundDrawablesRelativeWithIntrinsicBounds(
                R.drawable.jc_badge_dot_ok, 0, 0, 0);
        adFilterBadge.setVisibility(View.VISIBLE);
    }

    /** 播到"本集在别的源已下载"的在线集时, 每集弹一次提示(完整会话才弹). */
    private void maybeOtherSourceToast() {
        int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
        if (idx < 0 || session.localPlayback || session.isSingleLocalPlaylist()) return;
        if (session.isEpisodeLocalAvailable(idx)) return; // 当前源就有本地副本, 无需提示
        if (!session.otherSourceHasEpisode(idx)) return;
        if (!session.otherSourceToastShown.add(idx)) return; // 每集只弹一次
        com.jerocine.player.download.DownloadTask t = session.otherSourceTask(idx);
        session.host().showCenterToast(
                PlayerModes.otherSourceToastText(t != null ? t.sourceName : null), 2400);
    }

    /** 本地播放 SAF 选文件请求码. */
    private static final int REQ_OPEN_LOCAL = 4001;

    /** 更多菜单 → 选择本地文件播放: SAF 选文件 → 持久读授权 → 本地模式新实例承载. */
    private void openLocalFilePicker() {
        Intent i = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        i.addCategory(Intent.CATEGORY_OPENABLE);
        i.setType("*/*"); // .ts 常被识别为 application/octet-stream, 放开类型让用户自选
        try {
            startActivityForResult(i, REQ_OPEN_LOCAL);
        } catch (Exception e) {
            showCenterToast("系统文件选择器不可用", 1800);
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != REQ_OPEN_LOCAL || resultCode != RESULT_OK || data == null) return;
        Uri uri = data.getData();
        if (uri == null) return;
        try {
            getContentResolver().takePersistableUriPermission(uri,
                    Intent.FLAG_GRANT_READ_URI_PERMISSION);
        } catch (SecurityException ignore) {
            // 部分文件管理器只给本次临时授权 — 当前进程播放不受影响, 忽略
        }
        Intent play = new Intent(this, PlayerActivity.class);
        play.setAction(Intent.ACTION_VIEW);
        play.setData(uri);
        play.putExtra(EXTRA_TITLE, LocalPlayback.displayTitle(uri.getLastPathSegment(), null));
        play.setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        startActivity(play);
        finish();
    }

    /** 更多菜单 → 缓存缓冲: 选时长, 暂停播放后后台预取前 N 分钟分片(写播放缓存区). */
    private void showBufferDialog() {
        final double[] options = {5.0, 15.0, 30.0, 0.0};
        final String[] labels = {"缓冲 5 分钟", "缓冲 15 分钟", "缓冲 30 分钟", "缓冲本集"};
        com.jerocine.player.ui.JcDialog.list(this)
                .title("缓存缓冲")
                .items(labels)
                .onItemClick((d, i) -> startBufferPrefetch(options[i], labels[i]))
                .cancelButton("取消")
                .show();
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
        // 本地播放中的集分片已在下载缓存区, 缓冲无意义(统一判定, 含单集本地)
        if (session.isEpisodePlayingLocal(idx)) {
            showCenterToast("本集已下载, 无需缓冲", 1800);
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
        // 必须用播放器**实际会请求的** URL, 不能用原始源站 URL:
        // 开了「中转」或本集被自愈/端侧过滤失败升级时, 播放器走的是 /v1/m3u8/proxy 包装地址,
        // 服务端返回的清单里分片 URL 已被改写为代理地址。SimpleCache 的键就是 URL 字符串,
        // 两种地址是两个不同的键 → 按源站 URL 预取的分片一条也命中不了,
        // 表现为"缓冲了但播放依然卡", 白耗带宽和磁盘。
        final String srcUrl = session.mediaUriFor(idx, session.currentRawUrls.get(idx));
        showCenterToast("正在缓冲 " + label + " · 播放暂停", 2200);
        try {
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
        } catch (Exception e) {
            // start() 同步抛异常(如线程池已 shutdown)→ 不会触发回调,
            // 上面捕获的 playing 就丢了, 播放会停在暂停态无人恢复。
            bufferPrefetcher = null;
            showCenterToast("缓存缓冲启动失败: "
                    + (e.getMessage() == null ? "未知错误" : e.getMessage()), 2400);
            if (session.player != null && playing) session.player.setPlayWhenReady(true);
        }
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
        com.jerocine.player.ui.JcDialog.message(this)
                .title("诊断信息")
                .message(sb.toString())
                .confirmButton("知道了")
                .show();
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
    /**
     * 暂停/缓冲中控制面板不主动消失(用户要求 2026-10-09, 策略见 {@link PlayerAutoHidePolicy})。
     * media3 的自动收起是 show() 时排一个延时任务 — 暂停时唤出也会被收、缓冲中照样收。
     * 这里按状态切超时, 并对"面板当前可见"的场景清理/重排那个延时任务:
     *   保持可见(暂停/缓冲) → 超时 0 + hide→show 清掉已排任务(同帧同步执行, 无闪烁;
     *       showController 在播放抑制时会 no-op, 故先查 suppression, 抑制时面板保持原状);
     *   播放中 → 恢复 3s, 可见则 show() 重排计时。
     * 监听点: onPlaybackStateChanged + onPlayWhenReadyChanged(在线/本地两套监听器都挂)。
     */
    private void updateControllerAutoHide() {
        if (playerView == null || session.player == null) return;
        boolean buffering = session.player.getPlaybackState() == Player.STATE_BUFFERING;
        boolean playWhenReady = session.player.getPlayWhenReady();
        playerView.setControllerShowTimeoutMs(
                PlayerAutoHidePolicy.timeoutMs(buffering, playWhenReady));
        if (!playerView.isControllerFullyVisible()) return;
        boolean hold = PlayerAutoHidePolicy.shouldHoldVisible(buffering, playWhenReady);
        boolean suppressed = session.player.getPlaybackSuppressionReason()
                != Player.PLAYBACK_SUPPRESSION_REASON_NONE;
        if (hold) {
            if (!suppressed) {
                // show() 只在超时>0 时重排延时任务 → 先 hide 清旧的再 show(永不自动收)
                playerView.hideController();
                playerView.showController();
            }
        } else {
            playerView.showController(); // 重排 3s 自动收起
        }
    }

    private void updateFilterLoadingText(int state) {
        View tv = findViewById(R.id.filter_loading_text);
        if (tv == null) return;
        int idx = session.player != null ? session.player.getCurrentMediaItemIndex() : -1;
        boolean show = state == Player.STATE_BUFFERING
                && session.episodeSwitching
                && session.adFilterOn
                // 本地集清单已过滤过, 不存在"过滤中"(统一判定, 见 PlayerModes)
                && !session.isEpisodePlayingLocal(idx);
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
