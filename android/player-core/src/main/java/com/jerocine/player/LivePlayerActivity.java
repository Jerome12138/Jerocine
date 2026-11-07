package com.jerocine.player;

import android.content.Intent;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.view.KeyEvent;
import android.view.View;
import android.view.WindowManager;
import android.widget.ImageView;
import android.widget.ProgressBar;
import android.widget.TextView;

import androidx.activity.OnBackPressedCallback;
import androidx.annotation.NonNull;
import androidx.annotation.OptIn;
import androidx.appcompat.app.AppCompatActivity;
import androidx.media3.common.MediaItem;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.common.Timeline;
import androidx.media3.common.VideoSize;
import androidx.media3.common.util.UnstableApi;
import androidx.media3.datasource.DataSource;
import androidx.media3.datasource.DefaultDataSource;
import androidx.media3.datasource.okhttp.OkHttpDataSource;
import androidx.media3.exoplayer.DefaultLoadControl;
import androidx.media3.exoplayer.DefaultRenderersFactory;
import androidx.media3.exoplayer.ExoPlayer;
import androidx.media3.exoplayer.LoadControl;
import androidx.media3.exoplayer.hls.HlsMediaSource;
import androidx.media3.ui.PlayerView;
import androidx.recyclerview.widget.LinearLayoutManager;
import androidx.recyclerview.widget.RecyclerView;

import com.jerocine.player.ui.LiveChannelAdapter;
import com.jerocine.player.ui.LiveSourceChipAdapter;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;

import okhttp3.OkHttpClient;

/**
 * 直播播放器 — 独立于点播 {@link PlayerActivity} 的 Activity(2026-11-07 用户拍板:
 * 直播走独立壳, 不复用点播播放器; 两壳 manifest 已预注册本类)。
 *
 * <p>与点播的关键差异(均 2026-11-07 用户拍板):
 * <ul>
 *   <li><b>全程不过 SimpleCache</b>: 直播滑窗写缓存语义无意义 —— 不建 CacheDataSource、
 *       不碰 PlayerActivity.sCache。清单/分片同走一份 DefaultDataSource(OkHttp 上游),
 *       同一工厂当两参传 {@link HlsManifestSplitDataSourceFactory}(直播没有 file:// 本地
 *       清单, 拆分工厂在此退化为直通, 仅为沿用同一条 HLS 装配路径)。</li>
 *   <li><b>直播不走去广告链路</b>: 不挂 FilterPlaylistParserFactory(setPlaylistParserFactory
 *       不调用), 不引 PlayerAdFilterHelper / M3u8FilterCore / PlayerSession。</li>
 *   <li><b>无进度概念</b>: 无进度行/倍速/跳片头/预取/进度轮询; 无 filmId → 天然不写影视历史;
 *       不发 playerProgress / playerClosed 等任何点播事件。</li>
 *   <li><b>←→ 双击才换源</b>: 单击只提示防误触, 判定收在 {@link LiveKeyPolicy}(窗口内同向
 *       再按一次才真换); ↑↓ 与频道键切台不回绕(到头停住); 频道不分组, 抽屉平铺列表
 *       (分组语义由 LiveChannelAdapter 保证不出现)。</li>
 * </ul>
 *
 * <p><b>事件</b>: 只发 {@code liveChannelChange}, 且每条装载路恰好一次。换源/换频道
 * (时间线非空 → 非空)的发出点是 onMediaItemTransition; **初次装载不在其列** ——
 * setMediaItems 首次装载**不触发** onMediaItemTransition(PlayerSession.loadPlaylistIntoPlayer
 * 2026-10-18 的实测结论), 故 {@link #loadChannel} 在时间线为空的首装时补发一次。
 * 按规格「不在别处重复」: 其余监听回调与按键路径都不发, 防同一次装载双发。
 * 2026-11-07 用户拍板(评审修复): onMediaItemTransition 只认 PLAYLIST_CHANGED ——
 * 流自然播完(伪直播 ENDLIST / 源端停流)触发的 AUTO 前进既非装载路也非用户切台,
 * 不发事件, 只同步下标与 UI(否则事件携带切走前的旧频道下标, 壳层把没在看的频道
 * 写进 LiveLastViewed, 实际播的却是下一频道), 详见 {@link #syncAutoTransition}。
 * payload 键: sourceId(long, Long.parseLong 失败给 0) / sourceName / sourceIndex /
 * channelIndex / channelName / ts(long, System.currentTimeMillis())。
 *
 * <p><b>按键两态模型</b>: 直播没有进度条, 只有「抽屉聚焦/未聚焦」两态 ——
 * 抽屉内任一 View 有焦点时仅 BACK(清焦点并收抽屉)/MENU(开关抽屉)自处理, 其余一律
 * super.dispatchKeyEvent 让 RecyclerView 焦点导航自然工作; 无焦点(含隐藏)时 ↑↓/频道键切台、
 * ←→ 防误触换源、OK/空格播暂、媒体键切台。BACK 沿用 PlayerKeyEventHelper 细节:
 * DOWN 走逻辑、UP 吞掉(防系统再触发一次), 双击 2s 确认退出。
 * 全量键控只认首按 ACTION_DOWN(getRepeatCount()==0), 系统长按 repeat 一律过滤(2026-11-07
 * 用户拍板评审修复): repeat 也是 ACTION_DOWN(getRepeatCount&gt;0), 不滤则按住 ←/→ 约 500ms
 * 即被 LiveKeyPolicy 判成「双击」换源, 击穿「←→ 双击才换源」拍板; 按住 ↑/↓ 每个 repeat
 * 都整流重载一次频道; 按住 BACK/MENU 则双击确认与抽屉开关同理被击穿。
 * 手势/导航栏返回经 OnBackPressedDispatcher 也接到同一 handleBack(见 onCreate 注册处)。
 *
 * <p><b>装配</b>对照 PlayerActivity.initPlayer: OkHttp 客户端可注入则复用(newBuilder 保连接池),
 * UA 用点播同一份 {@link PlayerActivity#MEDIA_UA}; LoadControl 收窄为 20s/60s(直播滑窗无需
 * 120s 深缓冲); behind-live 自愈原样复用 {@link PlayerLiveErrorPolicy}(判定 + RecoveryGate
 * 防循环)。不注册 PlayerControl / 不实现 PlayerSession.Host —— 本类完全独立于点播会话。
 */
@OptIn(markerClass = UnstableApi.class)
public class LivePlayerActivity extends AppCompatActivity {

    // ============================ 常量 ============================

    /** 抽屉自动隐藏延时: 播放中唤出后 3s 自动收(暂停时不排计时, 抽屉保持)。 */
    private static final long DRAWER_AUTO_HIDE_MS = 3000L;

    /** BACK 双击确认窗口(沿用 PlayerKeyEventHelper.BACK_CONFIRM_MS = 2s)。 */
    private static final long BACK_CONFIRM_MS = 2000L;

    // ============================ sources JSON 载体 ============================

    /**
     * sources JSON 的轻量载体 — 契约同 {@code PlayerSourceHelper.parseSources}
     * ([{id,name,episodes:[{url,title}]}]), 直播侧用 org.json 自解析, 不建 PlayerSession。
     * 一个源 = 一份频道表: urls[i]/titles[i] 一一对应, i 即频道下标。
     */
    private static final class SourceInfo {
        final String id;
        final String name;
        final List<String> urls = new ArrayList<>();
        final List<String> titles = new ArrayList<>();

        SourceInfo(String id, String name) {
            this.id = id;
            this.name = name;
        }
    }

    // ============================ 状态 ============================

    private final List<SourceInfo> sources = new ArrayList<>();
    private int currentSourceIndex;
    private int currentChannelIndex;
    private ExoPlayer player;

    /** 直播 behind-live/reset 兜底防循环闸门(与 PlayerActivity 同款, 原样复用)。 */
    private final PlayerLiveErrorPolicy.RecoveryGate liveRecoveryGate =
            new PlayerLiveErrorPolicy.RecoveryGate(
                    PlayerLiveErrorPolicy.MAX_RECOVERS_PER_WINDOW,
                    PlayerLiveErrorPolicy.RECOVERY_WINDOW_MS);

    /** ←→ 双击换源判定(每个直播会话持一个; 单击提示/双击放行的状态都在里面)。 */
    private final LiveKeyPolicy keyPolicy = LiveKeyPolicy.create();

    /** 最近一次 BACK 按下时刻(双击确认退出用; 收抽屉成功后归零重计)。 */
    private long lastBackAt = 0L;

    /**
     * 影片名(按契约接收 EXTRA_FILM_NAME)。直播页无标题栏(布局没有 title 视图),
     * 且无 filmId → 不写影视历史, 该字段暂无消费点, 仅解析留存保持载荷契约完整。
     */
    private String filmName;

    /** 正在切台/换源(装载起 true, STATE_READY 清) — 与点播 episodeSwitching 同语义的直播版。 */
    private boolean switching;

    // ============================ 视图 ============================

    private PlayerView playerView;
    private ProgressBar bufferSpinner;
    private TextView resolutionBadge;
    private TextView centerToast;
    private ImageView centerIcon;
    private View liveDrawer;
    private RecyclerView chipList;
    private RecyclerView channelList;

    private LiveSourceChipAdapter chipAdapter;
    private LiveChannelAdapter channelAdapter;

    // ============================ Handler ============================

    private final Handler toastHandler = new Handler(Looper.getMainLooper());
    private final Handler iconHandler = new Handler(Looper.getMainLooper());
    private final Handler drawerHandler = new Handler(Looper.getMainLooper());

    private final Runnable drawerAutoHideRunnable = this::hideDrawer;

    // ============================ 生命周期 ============================

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        setContentView(R.layout.activity_live_player);
        hideSystemUi();

        playerView = findViewById(R.id.player_view);
        // ⚠️ PlayerView 必须**不可聚焦**(2026-10-10 焦点教训, 同 PlayerActivity):
        // 全屏 View 一旦可聚焦就会截胡 D-pad 焦点搜索, 焦点聚上去就出不来。
        // XML 已设 false, 代码级再设一次防 PlayerView 构造器内 setFocusable。
        playerView.setFocusable(false);
        playerView.setFocusableInTouchMode(false);
        bufferSpinner = findViewById(R.id.buffer_spinner);
        resolutionBadge = findViewById(R.id.resolution_badge);
        centerToast = findViewById(R.id.center_toast);
        centerIcon = findViewById(R.id.center_icon);
        liveDrawer = findViewById(R.id.live_drawer);
        chipList = findViewById(R.id.source_chip_list);
        channelList = findViewById(R.id.channel_list);

        // layoutManager 不在 XML 设(布局注明的契约): 横向给源 chips, 纵向给频道列表
        chipList.setLayoutManager(
                new LinearLayoutManager(this, RecyclerView.HORIZONTAL, false));
        channelList.setLayoutManager(
                new LinearLayoutManager(this, RecyclerView.VERTICAL, false));

        // AndroidX OnBackPressedDispatcher 接管返回(predictive back / 系统手势 / 导航栏返回
        // 都走这里), 必须注册 callback —— 否则手势/导航栏返回会被 dispatcher 默认直接
        // finish(), 绕过 handleBack 的「收抽屉 + 双击 2s 确认退出」契约, 与遥控器 BACK
        // (dispatchKeyEvent)行为不一致(同 PlayerActivity:222 已修的同款缺口,
        // 2026-11-07 用户拍板评审修复)。
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                handleBack();
            }
        });

        initPlayer();

        // 解析载荷: EXTRA_SOURCES_JSON(契约同 PlayerSourceHelper.parseSources) +
        // EXTRA_CURRENT_SOURCE_ID + EXTRA_START_INDEX(频道下标) + EXTRA_FILM_NAME。
        // 不读 EXTRA_IS_LIVE —— 本 Activity 天生直播。EXTRA_FILM_NAME 仅按契约接收:
        // 直播页无标题栏(布局没有 title 视图), 暂无处展示, 解析留存备后用。
        Intent intent = getIntent();
        parseSourcesFromIntent(intent);
        filmName = intent.getStringExtra(PlayerActivity.EXTRA_FILM_NAME);
        bindAdapters();

        SourceInfo current = currentSource();
        if (current == null || current.urls.isEmpty()) {
            // 无可用频道: toast 后 finish(延时到与 toast 同长, 让提示真正可见再退)。
            showCenterToast("没有可播放的频道", 2000);
            toastHandler.postDelayed(() -> {
                if (!isFinishing()) finish();
            }, 2000);
            return;
        }
        // 起始频道下标钳进当前源频道表(载荷越界不炸, 与点播钳制思路一致)
        int startIndex = intent.getIntExtra(PlayerActivity.EXTRA_START_INDEX, 0);
        currentChannelIndex = Math.max(0, Math.min(startIndex, current.urls.size() - 1));
        chipAdapter.setCurrentIndex(currentSourceIndex);
        channelAdapter.setTitles(current.titles);
        channelAdapter.setCurrentIndex(currentChannelIndex);
        loadChannel(currentSourceIndex, currentChannelIndex);
    }

    @Override
    protected void onPause() {
        super.onPause();
        // 不后台播: 离开/熄屏/切后台即暂停
        if (player != null && player.isPlaying()) player.pause();
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        toastHandler.removeCallbacksAndMessages(null);
        iconHandler.removeCallbacksAndMessages(null);
        drawerHandler.removeCallbacksAndMessages(null);
        if (player != null) {
            player.release();
            player = null;
        }
    }

    // ============================ 装配 ============================

    /**
     * 装配直播播放器 — 对照 PlayerActivity.initPlayer 的直播差异版:
     * ① 全程不过 SimpleCache(不建 CacheDataSource、不碰 PlayerActivity.sCache);
     * ② 不走去广告链路(不调 setPlaylistParserFactory, 不引 PlayerAdFilterHelper/
     *    M3u8FilterCore/PlayerSession)—— 2026-11-07 用户拍板;
     * ③ 清单/分片同一份 DefaultDataSource 工厂(无缓存链路可拆), 同一工厂当两参传
     *    HlsManifestSplitDataSourceFactory 沿用同一条 HLS 装配路径;
     * ④ 缓冲收窄 20s/60s(直播滑窗, 无需点播 30s/120s 的深缓冲)。
     */
    private void initPlayer() {
        OkHttpClient http = buildMediaHttp();
        // 清单与分片共用同一工厂: OkHttp 上游包一层 DefaultDataSource(非 http(s) 协议
        // 由 DefaultDataSource 兜底, OkHttpDataSource 对其会抛 Malformed URL), 再同参传给
        // 拆分工厂 —— 无 SimpleCache 可绕, DATA_TYPE_MANIFEST 分流在此退化为直通。
        DataSource.Factory okHttp = new OkHttpDataSource.Factory(http)
                .setUserAgent(PlayerActivity.MEDIA_UA);
        DataSource.Factory dataSourceFactory = new DefaultDataSource.Factory(this, okHttp);
        HlsMediaSource.Factory msFactory = new HlsMediaSource.Factory(
                new HlsManifestSplitDataSourceFactory(dataSourceFactory, dataSourceFactory));
        msFactory.setAllowChunklessPreparation(true);
        // 解码: 硬解吃不消时回退软解; 异步队列送解码(全机型强制开, 同点播)
        DefaultRenderersFactory renderersFactory = new DefaultRenderersFactory(this)
                .setEnableDecoderFallback(true)
                .forceEnableMediaCodecAsynchronousQueueing();
        // 缓冲: 起播阈值 1.5s, 卡顿后 2.5s 恢复; 稳态 20s / 上限 60s; 留 30s 回看缓冲
        LoadControl loadControl = new DefaultLoadControl.Builder()
                .setBufferDurationsMs(20_000, 60_000, 1_500, 2_500)
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
                bufferSpinner.setVisibility(
                        state == Player.STATE_BUFFERING ? View.VISIBLE : View.GONE);
                if (state == Player.STATE_READY) {
                    switching = false; // 装载完成, 清切台标记
                }
            }

            @Override
            public void onPlayWhenReadyChanged(boolean playWhenReady, int reason) {
                if (player == null) return;
                if (!playWhenReady) {
                    // 暂停: 中央暂停图标持久显示 + 弹出抽屉(不抢焦点, 暂停中不排自动隐藏)
                    if (player.getPlaybackState() == Player.STATE_READY) {
                        showCenterIconPersistent(R.drawable.ic_pause);
                        showDrawer(false);
                    }
                } else {
                    showCenterIcon(R.drawable.ic_play, 600);
                    scheduleDrawerAutoHide(); // 恢复播放后, 已开的抽屉重新计 3s 自动收
                }
            }

            /** 分辨率实时更新: 码率切换/切台后都会回调; 未就绪宽高为 0 → 隐藏. */
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

            /**
             * 换源/换频道(时间线非空→非空)的 liveChannelChange 发出点。首次装载不经过这里
             * (setMediaItems 首装不触发 transition, 2026-10-18 实测), 由 loadChannel 补发;
             * 按规格「不在别处重复」, 除此之外任何路径都不再发同一次装载的事件。
             *
             * <p>reason 必须过滤(2026-11-07 用户拍板评审修复): 只认 PLAYLIST_CHANGED ——
             * 全部装载路都经 loadChannel 的 setMediaItems → PLAYLIST_CHANGED; 流自然播完
             * (伪直播 ENDLIST / 源端正常停流)触发的 AUTO 前进是「静默换台」, 不发事件
             * (既非装载路也非用户切台), 只同步下标与 UI, 见 {@link #syncAutoTransition};
             * SEEK/REPEAT 当前无产生点(本类换源/换频道全走 setMediaItems, 无 seekTo* 切台,
             * 未设 repeat mode), 不发。
             */
            @Override
            public void onMediaItemTransition(MediaItem mediaItem, int reason) {
                if (reason == Player.MEDIA_ITEM_TRANSITION_REASON_PLAYLIST_CHANGED) {
                    emitLiveChannelChange();
                    return;
                }
                if (reason == Player.MEDIA_ITEM_TRANSITION_REASON_AUTO) {
                    syncAutoTransition();
                }
            }

            @Override
            public void onPlayerError(@NonNull PlaybackException error) {
                // behind-live/reset 类 → seekToDefaultPosition+prepare 重定位滑窗边缘,
                // 不弹错; 防循环由 RecoveryGate 短窗限次(与 PlayerActivity 同款, 原样复用)。
                if (maybeRecoverBehindLive(error)) return;
                // 其余报错: center toast 即可, **不自动退出** —— 用户可切台自救。
                String currentUrl = "";
                try {
                    if (player != null && player.getCurrentMediaItem() != null
                            && player.getCurrentMediaItem().localConfiguration != null) {
                        currentUrl = String.valueOf(
                                player.getCurrentMediaItem().localConfiguration.uri);
                    }
                } catch (Exception ignore) {
                }
                // URL 必须经 ErrorDiag.safeUrl 脱敏再上屏(签名不印在屏幕上, 同点播口径)。
                showCenterToast("播放出错 " + error.errorCode + " ("
                        + error.getErrorCodeName() + ")\n"
                        + ErrorDiag.safeUrl(currentUrl), 8000);
            }
        });
    }

    /**
     * 播放侧共享 HTTP 客户端(清单与分片同一实例, 共享连接池): 壳层可注入自己的媒体客户端
     * (newBuilder 复用连接池/TLS 策略), 未注入则自建。超时口径同点播 buildMediaHttp:
     * connect/read/write 各 30s; callTimeout 0(不设整次请求总时限, 长分片不被掐断);
     * 连接失败重试 + 跟随 http/https 重定向(直播源 302 跳转常见)。
     */
    private static OkHttpClient buildMediaHttp() {
        OkHttpClient injected = JerocinePlayer.mediaClient();
        OkHttpClient.Builder builder = (injected != null)
                ? injected.newBuilder() : new OkHttpClient.Builder();
        return builder
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(30, TimeUnit.SECONDS)
                .writeTimeout(30, TimeUnit.SECONDS)
                .callTimeout(0, TimeUnit.SECONDS)
                .retryOnConnectionFailure(true)
                .followRedirects(true)
                .followSslRedirects(true)
                .build();
    }

    /**
     * behind-live/reset 类报错自愈(逻辑同 PlayerActivity.maybeRecoverBehindLive):
     * 判定与限次都在 {@link PlayerLiveErrorPolicy}; 额度用尽走报错路径(可见性优先)。
     *
     * @return true = 已兜底(调用方短路, 不再走报错路径)
     */
    private boolean maybeRecoverBehindLive(PlaybackException error) {
        if (player == null) return false;
        if (!PlayerLiveErrorPolicy.isBehindLiveError(error.errorCode, error.getCause())) {
            return false;
        }
        if (!liveRecoveryGate.tryAcquire(SystemClock.elapsedRealtime())) {
            android.util.Log.w("LivePlayerActivity", "live recovery suppressed by loop guard");
            return false;
        }
        android.util.Log.w("LivePlayerActivity", "behind-live/reset -> seekToDefaultPosition+prepare");
        player.seekToDefaultPosition();
        player.prepare();
        return true;
    }

    // ============================ 载荷解析 / 适配器 ============================

    /**
     * 解析 EXTRA_SOURCES_JSON 为 {@link SourceInfo} 列表(契约同 PlayerSourceHelper.parseSources:
     * id 缺省空串、name 缺省「源 N」、episode 无 url 跳过、按 EXTRA_CURRENT_SOURCE_ID 定起始源,
     * 缺省 0)。解析异常按「无可用频道」处理(列表清空, onCreate 统一 toast 后 finish)。
     */
    private void parseSourcesFromIntent(Intent intent) {
        sources.clear();
        String json = intent.getStringExtra(PlayerActivity.EXTRA_SOURCES_JSON);
        if (json == null || json.isEmpty()) return;
        try {
            JSONArray arr = new JSONArray(json);
            String currentId = intent.getStringExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID);
            int curIdx = 0;
            for (int i = 0; i < arr.length(); i++) {
                JSONObject src = arr.getJSONObject(i);
                SourceInfo info = new SourceInfo(
                        src.optString("id", ""),
                        src.optString("name", "源 " + (i + 1)));
                JSONArray eps = src.optJSONArray("episodes");
                if (eps != null) {
                    for (int j = 0; j < eps.length(); j++) {
                        JSONObject ep = eps.getJSONObject(j);
                        String u = ep.optString("url", "");
                        if (u.isEmpty()) continue;
                        info.urls.add(u);
                        info.titles.add(ep.optString("title", ""));
                    }
                }
                sources.add(info);
                if (currentId != null && currentId.equals(info.id)) curIdx = i;
            }
            currentSourceIndex = Math.max(0, Math.min(curIdx, sources.size() - 1));
        } catch (Exception e) {
            sources.clear(); // JSON 异常: 一律按无可用频道走
            currentSourceIndex = 0;
        }
    }

    /** 绑定抽屉两个列表(纯 UI 层适配器, 业务回调用 lambda 落地)。 */
    private void bindAdapters() {
        List<String> names = new ArrayList<>();
        for (SourceInfo s : sources) names.add(s.name);
        chipAdapter = new LiveSourceChipAdapter(this, names,
                pos -> {
                    // chips 点击 → 切源: 频道下标钳制保留, 抽屉保持开, 刷新两个列表, OSD
                    switchSource(pos, true);
                });
        chipList.setAdapter(chipAdapter);
        channelAdapter = new LiveChannelAdapter(this,
                currentSource() != null ? currentSource().titles : new ArrayList<String>(),
                pos -> {
                    // 频道行点击 → 切频道: 抽屉保持开, OSD
                    switchChannelByClick(pos);
                });
        channelList.setAdapter(channelAdapter);
    }

    // ============================ 装载 ============================

    /** 当前源(下标越界/无源时返回 null)。 */
    private SourceInfo currentSource() {
        if (currentSourceIndex < 0 || currentSourceIndex >= sources.size()) return null;
        return sources.get(currentSourceIndex);
    }

    /**
     * 装载一个频道: 该源 urls → List&lt;MediaItem&gt;, 从 channelIndex 起播。
     *
     * <p>换源: 调用方先更新 currentSourceIndex 并用
     * {@link LiveKeyPolicy#channelIndexForNewSource} 钳制频道下标;
     * 换频道: 调用方用 {@link LiveKeyPolicy#stepChannel} 步进(不回绕)。本方法只同步
     * 状态字段并驱动播放器(setMediaItems/prepare/setPlayWhenReady(true))。
     *
     * <p>liveChannelChange 只在**首次装载**(时间线为空 → 非空)时由这里补发 ——
     * setMediaItems 首装不触发 onMediaItemTransition(PlayerSession.loadPlaylistIntoPlayer
     * 2026-10-18 实测结论), 不补发则壳层收不到首装事件; 换源/换频道的同一次装载会触发
     * transition, 由监听器发, 此处不再发(规格「不在别处重复」, 防双发)。
     */
    private void loadChannel(int sourceIndex, int channelIndex) {
        if (player == null || sourceIndex < 0 || sourceIndex >= sources.size()) return;
        SourceInfo src = sources.get(sourceIndex);
        if (channelIndex < 0 || channelIndex >= src.urls.size()) return;
        List<MediaItem> items = new ArrayList<>();
        for (String url : src.urls) {
            items.add(MediaItem.fromUri(url));
        }
        if (items.isEmpty()) return;
        boolean firstLoad = player.getCurrentTimeline().isEmpty();
        currentSourceIndex = sourceIndex;
        currentChannelIndex = channelIndex;
        switching = true;
        player.setMediaItems(items, channelIndex, 0);
        player.prepare();
        player.setPlayWhenReady(true);
        if (firstLoad) emitLiveChannelChange();
    }

    /**
     * AUTO 前进(当前频道流自然播完, 播放列表默认 REPEAT_MODE_OFF 下 ExoPlayer 自动切到
     * 下一媒体项)的状态同步 —— 播放列表是整源频道表(本方法上文的 loadChannel), AUTO 前进
     * 即实际播放的频道变了, 但 currentChannelIndex 只在 loadChannel/switchSource 更新,
     * 不同步则: ①此后 stepChannelBy 按过期下标步进(↓ 重载已在播频道、↑ 回到被跳过的频道);
     * ②抽屉高亮停在旧频道; ③OSD 不出, 换台完全无感知。这里把下标对齐到播放器实际位置并
     * 刷新抽屉高亮 + OSD。事件**不发**(2026-11-07 用户拍板评审修复, 见 onMediaItemTransition
     * 注释): AUTO 非装载路也非用户切台, 壳层 LiveLastViewed 保留用户最后一次主动选台,
     * 下次起播回到该频道, 不把无人值守的停流切台写进「上次观看」。
     */
    private void syncAutoTransition() {
        if (player == null) return;
        int idx = player.getCurrentMediaItemIndex();
        SourceInfo src = currentSource();
        if (src == null || channelAdapter == null
                || idx < 0 || idx >= src.urls.size() || idx == currentChannelIndex) {
            return; // 下标越界/原地: 不动状态(收尾 item 播完走 STATE_ENDED, 不经过这里)
        }
        currentChannelIndex = idx;
        channelAdapter.setCurrentIndex(idx);
        showOsd();
    }

    /**
     * 切台步进(↑↓ / 频道键 / 媒体键): {@link LiveKeyPolicy#stepChannel} 步进不回绕,
     * 到头停住(同下标 = 没切动, 不重装不 OSD, 免得原地重启直播流)。
     * 每次切台: loadChannel + OSD(「源名 · 频道名」2s) + showDrawer(false)(不抢焦点)。
     */
    private void stepChannelBy(int delta) {
        SourceInfo src = currentSource();
        if (src == null) return;
        int target = LiveKeyPolicy.stepChannel(currentChannelIndex, delta, src.urls.size());
        if (target == currentChannelIndex) return; // 到头停住
        loadChannel(currentSourceIndex, target);
        showOsd();
        showDrawer(false); // 不抢焦点
    }

    /**
     * 换源(←→ 双击放行 / 抽屉 chip 点击): 更新 currentSourceIndex, 频道下标用
     * {@link LiveKeyPolicy#channelIndexForNewSource}(当前频道下标, 新源频道数)钳制保留;
     * 刷新两个列表( chip 只需刷新高亮, 频道表整体替换 ), OSD, 抽屉按调用方意图开/保持。
     *
     * @param keepDrawerOpen true = 抽屉保持开(chip 点击路径); false = 收成无焦点态
     *                       (←→ 双击路径 showDrawer(false), 不抢焦点)
     */
    private void switchSource(int targetIndex, boolean keepDrawerOpen) {
        if (targetIndex < 0 || targetIndex >= sources.size()
                || targetIndex == currentSourceIndex) {
            return; // 越界/原地: 无动作(源不回绕, 与切台到头停住同口径)
        }
        SourceInfo target = sources.get(targetIndex);
        if (target.urls.isEmpty()) return; // 空源(无频道)不切入, 免得黑屏无反馈
        currentChannelIndex = LiveKeyPolicy.channelIndexForNewSource(
                currentChannelIndex, target.urls.size());
        loadChannel(targetIndex, currentChannelIndex);
        chipAdapter.setCurrentIndex(currentSourceIndex);
        channelAdapter.setTitles(target.titles);
        channelAdapter.setCurrentIndex(currentChannelIndex);
        showOsd();
        if (!keepDrawerOpen) showDrawer(false);
    }

    /** 抽屉频道行点击切频道: 下标即 titles 下标(点击路径不步进, 越界/原地静默忽略)。 */
    private void switchChannelByClick(int channelIndex) {
        SourceInfo src = currentSource();
        if (src == null || channelIndex < 0 || channelIndex >= src.urls.size()
                || channelIndex == currentChannelIndex) {
            return;
        }
        loadChannel(currentSourceIndex, channelIndex);
        channelAdapter.setCurrentIndex(channelIndex);
        showOsd();
    }

    // ============================ 抽屉 ============================

    /**
     * 开抽屉: live_drawer VISIBLE; grabFocus 时把焦点给 channel_list 的当前频道行并
     * scrollToPosition(RecyclerView 从 GONE 变 VISIBLE 需等一帧布局, post 里再取 VH)。
     * 播放中唤出后排 3s 自动隐藏; 暂停中不排(抽屉保持)。
     */
    private void showDrawer(boolean grabFocus) {
        liveDrawer.setVisibility(View.VISIBLE);
        if (grabFocus) {
            channelList.scrollToPosition(currentChannelIndex);
            channelList.post(() -> {
                RecyclerView.ViewHolder vh =
                        channelList.findViewHolderForAdapterPosition(currentChannelIndex);
                if (vh != null) vh.itemView.requestFocus();
            });
        }
        scheduleDrawerAutoHide();
    }

    /** 收抽屉: 清抽屉内焦点 + GONE, 并撤掉未触发的自动隐藏回调。 */
    private void hideDrawer() {
        drawerHandler.removeCallbacks(drawerAutoHideRunnable);
        View focused = liveDrawer.findFocus();
        if (focused != null) focused.clearFocus();
        liveDrawer.setVisibility(View.GONE);
    }

    /** MENU/INFO 开关抽屉: 可见(含无焦点的 OSD 态)即收, 否则开并抓焦点到当前频道行。 */
    private void toggleDrawer() {
        if (liveDrawer.getVisibility() == View.VISIBLE) {
            hideDrawer();
        } else {
            showDrawer(true);
        }
    }

    /**
     * 自动隐藏计时: 重复调用先清旧回调; 只在 playWhenReady 时排(暂停时不排, 抽屉保持,
     * 恢复播放后由 onPlayWhenReadyChanged 重排)。
     */
    private void scheduleDrawerAutoHide() {
        drawerHandler.removeCallbacks(drawerAutoHideRunnable);
        if (player != null && player.getPlayWhenReady()) {
            drawerHandler.postDelayed(drawerAutoHideRunnable, DRAWER_AUTO_HIDE_MS);
        }
    }

    // ============================ 按键 ============================

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        int code = event.getKeyCode();
        // BACK 必须全权处理: DOWN 走逻辑, UP 一并吞掉(同 PlayerKeyEventHelper ——
        // 否则 UP 落到 super → onBackPressed 对 BACK 再调一次, 双击确认形同虚设)。
        // repeat 也一并吞掉(2026-11-07 用户拍板评审修复): 长按 BACK 的 repeat 同样是
        // ACTION_DOWN(getRepeatCount>0), 不滤则 2s 确认窗内每个 repeat 都算「再按一次」,
        // 长按 ≈0.5s 就直接退出, 双击确认被击穿。
        if (code == KeyEvent.KEYCODE_BACK) {
            if (event.getAction() == KeyEvent.ACTION_DOWN && event.getRepeatCount() == 0) {
                return handleBack();
            }
            return true;
        }
        // 态一: 抽屉内任一 View 有焦点 —— 仅 BACK(上面已处理)/MENU 自理,
        // 其余一律 super.dispatchKeyEvent, 让 RecyclerView 焦点导航自然工作。
        // 交互顺延自动隐藏计时: 浏览列表到一半被收走的话「抽屉保持开」就不成立。
        if (liveDrawer.findFocus() != null) {
            if (code == KeyEvent.KEYCODE_MENU || code == KeyEvent.KEYCODE_INFO) {
                // repeat 不动作(2026-11-07 用户拍板评审修复): 长按 MENU 的每个 repeat
                // 都会 toggleDrawer, 抽屉开合闪跳。首按之外(repeat/非 DOWN)一律吞掉。
                if (event.getAction() == KeyEvent.ACTION_DOWN
                        && event.getRepeatCount() == 0) {
                    toggleDrawer();
                }
                return true;
            }
            scheduleDrawerAutoHide();
            return super.dispatchKeyEvent(event);
        }
        // 态二: 抽屉无焦点(含隐藏)—— 直播全量键控; 只认首按 ACTION_DOWN
        // (getRepeatCount()==0), 其余(repeat/非 DOWN)放行 super
        // (同 PlayerKeyEventHelper 的非 DOWN 处理)。
        // repeat 必须过滤(2026-11-07 用户拍板评审修复): 系统长按 repeat 起始延迟约
        // 400-500ms, 落在 SOURCE_ZAP_WINDOW_MS=700ms 判定窗内 —— 不滤则按住 ←/→ ≈0.5s
        // 即被 LiveKeyPolicy 判成「双击」换源, 击穿「←→ 双击才换源」拍板, 且 SWITCH 后
        // 状态清空、HINT/SWITCH 交替连切多源; 按住 ↑/↓ 每个 repeat(~50ms 一次)都
        // stepChannelBy → loadChannel 整流重载一次频道。repeat 交给 super 走默认
        // (本态无任何可聚焦 View, 等价忽略)。
        if (event.getAction() != KeyEvent.ACTION_DOWN || event.getRepeatCount() > 0) {
            return super.dispatchKeyEvent(event);
        }
        switch (code) {
            case KeyEvent.KEYCODE_MENU:
            case KeyEvent.KEYCODE_INFO:
                toggleDrawer(); // 开时给当前频道行设焦(showDrawer(true) 内做)
                return true;
            case KeyEvent.KEYCODE_DPAD_UP:
            case KeyEvent.KEYCODE_CHANNEL_DOWN: // 沿用点播 CHANNEL_DOWN→上一个 的映射
            case KeyEvent.KEYCODE_MEDIA_PREVIOUS:
                stepChannelBy(-1);
                return true;
            case KeyEvent.KEYCODE_DPAD_DOWN:
            case KeyEvent.KEYCODE_CHANNEL_UP:   // 沿用点播 CHANNEL_UP→下一个 的映射
            case KeyEvent.KEYCODE_MEDIA_NEXT:
                stepChannelBy(+1);
                return true;
            case KeyEvent.KEYCODE_DPAD_LEFT:
            case KeyEvent.KEYCODE_DPAD_RIGHT:
                handleSourceKey(code == KeyEvent.KEYCODE_DPAD_RIGHT
                        ? LiveKeyPolicy.SourceDir.NEXT : LiveKeyPolicy.SourceDir.PREV);
                return true;
            case KeyEvent.KEYCODE_DPAD_CENTER:
            case KeyEvent.KEYCODE_ENTER:
            case KeyEvent.KEYCODE_SPACE:
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:
                togglePlayPause();
                return true;
            default:
                return super.dispatchKeyEvent(event);
        }
    }

    /**
     * 系统手势/导航栏返回兜底(同 PlayerActivity.onBackPressed 的同款缺口, 2026-11-07
     * 用户拍板评审修复): 与遥控器 BACK 走同一个 handleBack(收抽屉 → 双击 2s 确认退出),
     * 不调 super —— 默认直接 finish() 会绕过二次确认。正常路径已被 onCreate 注册的
     * OnBackPressedCallback 接管(此后本覆写不会再被 dispatcher 走到), 这里只兜
     * callback 缺位/直调的残口; handleBack 恒返回 true, 无需再落 super。
     */
    @Override
    public void onBackPressed() {
        handleBack();
    }

    /**
     * BACK 统一处理(DOWN 才进来): 抽屉内任一 View 有焦点 → 清焦点并收抽屉(hideDrawer
     * 内含 clearFocus); 否则双击 2s 确认退出(文案沿用点播「再按一次返回退出播放」)。
     * 收抽屉成功后归零双击计时, 退出确认从头算。遥控 BACK(dispatchKeyEvent)与系统
     * 手势/导航栏返回(OnBackPressedCallback / 本覆写)三路共此一逻辑。
     */
    private boolean handleBack() {
        if (liveDrawer.findFocus() != null) {
            hideDrawer();
            lastBackAt = 0L;
            return true;
        }
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

    /**
     * ← / → 换源判定(2026-11-07 用户拍板: 双击才换源): {@link LiveKeyPolicy#onSourceKey}
     * HINT → 按方向给对应提示文案; SWITCH → 换源(频道下标钳制保留) + OSD + showDrawer(false)。
     * 以单调时钟 SystemClock.elapsedRealtime() 喂窗口(与 RecoveryGate 同款惯例)。
     */
    private void handleSourceKey(LiveKeyPolicy.SourceDir dir) {
        LiveKeyPolicy.ZapVerdict verdict = keyPolicy.onSourceKey(dir, SystemClock.elapsedRealtime());
        if (verdict == LiveKeyPolicy.ZapVerdict.HINT) {
            showCenterToast(dir == LiveKeyPolicy.SourceDir.NEXT
                    ? "再按一次 → 切换下一源" : "再按一次 ← 切换上一源", 1800);
            return;
        }
        int target = dir == LiveKeyPolicy.SourceDir.NEXT
                ? currentSourceIndex + 1 : currentSourceIndex - 1;
        switchSource(target, false);
    }

    /** 播/暂切换。 */
    private void togglePlayPause() {
        if (player == null) return;
        if (player.getPlayWhenReady()) {
            player.pause();
        } else {
            player.play();
        }
    }

    // ============================ OSD / 中央反馈 ============================

    /**
     * 切台/换源 OSD: center_toast 显示「源名 · 频道名」2s。频道名空串时以「频道 N」兜底,
     * 免得 OSD 出现悬空的「源A · 」。
     */
    private void showOsd() {
        SourceInfo src = currentSource();
        if (src == null) return;
        String channelName = (currentChannelIndex >= 0 && currentChannelIndex < src.titles.size())
                ? src.titles.get(currentChannelIndex) : "";
        if (channelName.isEmpty()) channelName = "频道 " + (currentChannelIndex + 1);
        showCenterToast(src.name + " · " + channelName, 2000);
    }

    /** 中央 toast(与中央图标互斥, 同 PlayerActivity 惯例); 由 Handler 定时消失。 */
    private void showCenterToast(String msg, long durationMs) {
        if (centerIcon != null) centerIcon.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null);
        centerToast.setText(msg);
        centerToast.setVisibility(View.VISIBLE);
        toastHandler.removeCallbacksAndMessages(null);
        toastHandler.postDelayed(() -> {
            centerToast.setVisibility(View.GONE);
            // 若仍处暂停态, toast 结束后恢复「暂停图标停留」(同 PlayerActivity 惯例)
            if (player != null && !player.getPlayWhenReady()
                    && player.getPlaybackState() == Player.STATE_READY) {
                showCenterIconPersistent(R.drawable.ic_pause);
            }
        }, durationMs);
    }

    /** 中央图标(限时): 与 toast 互斥。 */
    private void showCenterIcon(int drawableRes, long durationMs) {
        if (centerIcon == null) return;
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
        iconHandler.removeCallbacksAndMessages(null);
        iconHandler.postDelayed(() -> centerIcon.setVisibility(View.GONE), durationMs);
    }

    /** 中央图标(持久, 暂停态): 与 toast 互斥, 无自动消失。 */
    private void showCenterIconPersistent(int drawableRes) {
        if (centerIcon == null) return;
        toastHandler.removeCallbacksAndMessages(null);
        if (centerToast != null) centerToast.setVisibility(View.GONE);
        iconHandler.removeCallbacksAndMessages(null);
        centerIcon.setImageResource(drawableRes);
        centerIcon.setVisibility(View.VISIBLE);
    }

    // ============================ 事件 ============================

    /** 事件出口帮手: 同包直调 {@link JerocinePlayer#dispatch}(包级静态派发)。 */
    private static void emit(String name, JSONObject payload) {
        JerocinePlayer.dispatch(name, payload);
    }

    /**
     * 组装并发出 liveChannelChange。payload 键名固定:
     * sourceId(long, Long.parseLong(source.id), 解析失败给 0) / sourceName / sourceIndex /
     * channelIndex / channelName / ts(long, System.currentTimeMillis())。
     */
    private void emitLiveChannelChange() {
        try {
            JSONObject p = new JSONObject();
            SourceInfo src = currentSource();
            long sourceId = 0L;
            if (src != null && src.id != null && !src.id.isEmpty()) {
                try {
                    sourceId = Long.parseLong(src.id);
                } catch (NumberFormatException ignore) {
                    sourceId = 0L; // 非数字 id 一律给 0
                }
            }
            p.put("sourceId", sourceId);
            p.put("sourceName", src != null ? src.name : "");
            p.put("sourceIndex", currentSourceIndex);
            p.put("channelIndex", currentChannelIndex);
            String channelName = (src != null && currentChannelIndex >= 0
                    && currentChannelIndex < src.titles.size())
                    ? src.titles.get(currentChannelIndex) : "";
            p.put("channelName", channelName);
            p.put("ts", System.currentTimeMillis());
            emit("liveChannelChange", p);
        } catch (Exception ignore) {
        }
    }

    // ============================ 杂项 ============================

    /** 沉浸式全屏(同 PlayerActivity.hideSystemUi)。 */
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
}
