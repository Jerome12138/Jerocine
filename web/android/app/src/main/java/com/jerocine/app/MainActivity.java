package com.jerocine.app;

import com.jerocine.player.JerocinePlayer;
import com.jerocine.player.PlayerAdFilterHelper;
import com.jerocine.player.PlayerNetworkModeHelper;

import android.app.AlertDialog;
import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.StateListDrawable;
import android.net.ConnectivityManager;
import android.net.NetworkInfo;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.util.Log;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.ConsoleMessage;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import com.getcapacitor.BridgeActivity;

import androidx.activity.OnBackPressedCallback;

import org.json.JSONObject;

/**
 * Jerocine TV 主 Activity.
 *
 * 1. 启动时弹原生输入框让用户输入 Jerocine 服务器地址 (http://ip 或 https://domain),
 *    保存到 SharedPreferences. 下次启动如果已存就直接 loadUrl, 不再弹.
 *    这样同一个 APK 能装到不同 TV 连不同后端.
 *
 * 2. BACK 键桥: 前端 App.vue 暴露 window.gfTvBack(): boolean,
 *    true=已消费 (路由回退), false=放过去 (退到桌面).
 *
 * 3. 重置入口: 连按 BACK 4 次 (1.2s 内) 弹"重置服务器"对话框,
 *    避免遥控器没法长按 / 没菜单键 的尴尬.
 */
public class MainActivity extends BridgeActivity {

    static final String PREFS = "jerocine_prefs";
    static final String KEY_SERVER_URL = "server_url";

    // ===== 玻璃暗色设计令牌 (与 res/values/colors.xml 对应, 编程式 UI 用) =====
    private static final int GF_BG          = 0xFF0B0B0F; // 应用根背景
    private static final int GF_GLASS       = 0xCC15151C; // 主玻璃面 (对话框/抽屉)
    private static final int GF_GLASS_SOFT  = 0xB31E1E28; // 次玻璃面 (按钮默认底)
    private static final int GF_SCRIM       = 0x99000000; // 遮罩
    private static final int GF_STROKE      = 0x22FFFFFF; // 细描边
    private static final int GF_TEXT        = 0xFFFFFFFF; // 主文本
    private static final int GF_TEXT_SEC    = 0xB3FFFFFF; // 次文本
    private static final int GF_TEXT_TER    = 0x80FFFFFF; // 三级文本
    private static final int GF_ACCENT      = 0xFF4AD1E5; // 强调青
    private static final int GF_ACCENT_DIM  = 0x334AD1E5; // 半透明青 (焦点底)
    private static final int GF_ACCENT_PRESS= 0x554AD1E5; // 按下青底
    /** 默认服务器地址 — 首次启动直接用, 不再弹输入框 */
    private static final String DEFAULT_SERVER_URL = "https://jerocine.art";
    /** 首页双击返回退出应用的确认窗口 */
    private static final long EXIT_CONFIRM_MS = 2000L;
    /** 品牌首屏最长展示时间 — 超时强制淡出, 露出 WebView 内容/错误(方案 §7.3 兜底) */
    private static final long SPLASH_TIMEOUT_MS = 15000L;

    /** 设置抽屉宽度(抽屉重设计 §4.1: 360 → 400dp, 分组标题 + 副标题需要横向空间) */
    private static final int DRAWER_W_DP = 400;
    /** 抽屉行高(D-pad 焦点友好) */
    private static final int ROW_H_DP = 56;

    private long lastExitBackAt = 0L;
    private UpdateChecker updateChecker;
    /** 启动品牌首屏(替代原裸转圈) */
    private BrandSplashView brandSplash;
    /** package-private — JerocineBridge.handleEchoTest 需要直接拿到 WebView 发反向事件 */
    WebView webViewRef;
    private FrameLayout settingsOverlay;
    private ScrollView settingsPanelScroll;
    private LinearLayout settingsPanel;
    /** 组装时记录第一行可聚焦项 —— 分组后行序会变, 不能再靠固定下标定位焦点 */
    private View settingsFirstFocus;
    /** 抽屉里的两处开关图形(切换后就地刷新, 不重建面板以保住遥控器焦点) */
    private FrameLayout adFilterSwitch;
    private FrameLayout relaySwitch;
    /** "显示模式"行右侧状态(存在 WebView localStorage, 打开抽屉时异步读一次) */
    private TextView displayModeValue;
    private String currentDisplayMode;
    private boolean settingsOpen = false;
    /** 断网兜底页 (原生全屏覆盖, 带重试按钮) */
    private FrameLayout offlineOverlay;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        // 系统 Splash(Android 12+ 及 core-splashscreen 兼容层): 必须在 super.onCreate 之前安装,
        // 否则 AppTheme.NoActionBarLaunch 的 postSplashScreenTheme 不生效, "系统 splash 无缝接品牌首屏"
        // 的前提不成立(评审 C10)。背景为纯黑, 图标即品牌图标, 与下面的 BrandSplashView 视觉连续。
        androidx.core.splashscreen.SplashScreen.installSplashScreen(this);
        super.onCreate(savedInstanceState);
        // 底部系统导航栏(手势条/三键条)与页面背景同色 — 装平板/手机时避免黑白条突兀
        if (Build.VERSION.SDK_INT >= 21) {
            getWindow().setNavigationBarColor(GF_BG);
        }
        // 注入 JS Bridge:
        //  v1 名: JerocinePlayer  (兼容旧 PlayView playVideo/playPlaylist)
        //  v2 名: JerocineNative  (通用 invoke + 事件总线)
        WebView wv = (bridge != null) ? bridge.getWebView() : null;
        webViewRef = wv;
        if (wv != null) {
            JerocineBridge b = new JerocineBridge(this);
            wv.addJavascriptInterface(b, "JerocinePlayer");
            wv.addJavascriptInterface(b, "JerocineNative");
        }

        // 启动加载动画 (覆盖在 WebView 上方居中, 加载完成隐藏)
        addBootProgress();

        // 播放器事件 → 通过 bridge 转发给 web
        JerocinePlayer.config().setCallback((name, payload) -> {
            JerocineBridge.sendEvent(webViewRef, name, payload);
        });

        String url = prefs().getString(KEY_SERVER_URL, null);
        if (url == null || url.isEmpty()) {
            // 首次启动: 用默认地址, 写回 SharedPreferences, 直接 loadUrl, 不弹框
            url = DEFAULT_SERVER_URL;
            prefs().edit().putString(KEY_SERVER_URL, url).apply();
        }
        loadServer(url);

        // AndroidX OnBackPressedDispatcher 完全接管返回(系统手势左滑 / predictive back /
        // 导航栏返回都走这里), 必须注册 OnBackPressedCallback —— 否则左滑手势返回会被
        // dispatcher 默认直接 finish() 退出应用, 绕过 handleBackPressed 的"先回上一页/双击退出"逻辑.
        // 与 tv 原生壳 MainActivity(onBackPressedDispatcher.addCallback) 机制保持一致.
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                handleBackPressed();
            }
        });

        // 自升级: 启动 5s 后异步检查 (避开首屏并行 IO)
        updateChecker = new UpdateChecker(this);
        new Handler(Looper.getMainLooper()).postDelayed(
                () -> updateChecker.check(false), 5000);
    }

    /**
     * 启动首屏(方案 §7): 黑底 + 品牌 logo + 呼吸 loading, 覆盖在 WebView 上方, 加载完成淡出。
     *
     * 超时兜底的必要性: 本 Activity **不能** setWebViewClient(会覆盖 Capacitor 的
     * BridgeWebViewClient ⇒ window.Capacitor 不注入 ⇒ TV 模式识别失败), 因此拿不到
     * onReceivedError / onPageFinished, 只能靠 onProgressChanged>=100 + 一个上限超时。
     * 超时值取 15s: 足够跨境弱网首屏, 又不至于让"加载彻底失败"时用户一直看 loading。
     */
    private void addBootProgress() {
        FrameLayout root = findViewById(android.R.id.content);
        if (root == null) return;
        brandSplash = new BrandSplashView(this);
        root.addView(brandSplash, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        new Handler(Looper.getMainLooper()).postDelayed(this::hideBootProgress, SPLASH_TIMEOUT_MS);
    }

    /** 淡出品牌首屏(幂等; 加载完成 / 超时 / 断网兜底页三条路径都会调) */
    private void hideBootProgress() {
        if (brandSplash != null) brandSplash.fadeOutAndRemove();
    }

    /** 供 JerocineBridge.checkUpdate() 主动触发更新检查 */
    public void checkUpdateExposed() {
        if (updateChecker == null) updateChecker = new UpdateChecker(this);
        updateChecker.check(true);
    }

    /** 暴露给 JerocineBridge 调用的"重置服务器地址" */
    public void promptServerUrlExposed() {
        promptServerUrl(true);
    }

    @Override
    public void onResume() {
        super.onResume();
        // 播放器报错后回到主界面: 通知前端 fallback (关 adFilter 重播)
        if (JerocinePlayer.control().consumePlaybackFailure()) {
            WebView wv = (bridge != null) ? bridge.getWebView() : null;
            if (wv != null) {
                wv.evaluateJavascript(
                        "window.__jerocineNativeError && window.__jerocineNativeError();",
                        null
                );
            }
        }
    }

    private SharedPreferences prefs() {
        return getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    /** 加载用户填的服务器地址 */
    private void loadServer(String url) {
        // 把当前服务器地址注入公共播放模块, 作为"未显式传 proxyBase"时的兜底代理
        // (播放器本身不硬编码任何域名)
        if (url != null && !url.isEmpty()) {
            JerocinePlayer.config().setDefaultProxyBase(url.replaceAll("/+$", "") + "/api");
        }
        WebView wv = (bridge != null) ? bridge.getWebView() : null;
        if (wv == null) return;
        // 断网兜底: 没网就不 loadUrl(避免 Chromium 原生 ERR_INTERNET_DISCONNECTED 错误页),
        // 改显示自定义无网络提示页(带重试). 有网则隐藏兜底页继续加载.
        if (!isOnline()) {
            showOfflineOverlay();
            return;
        }
        hideOfflineOverlay();
        // 显式确保关键 WebView 能力打开 (Capacitor 默认应已开, 这里冗余兜底)
        WebSettings s = wv.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
        s.setMediaPlaybackRequiresUserGesture(false);
        // 关键: 不开 wide viewport, WebView 默认按 980px CSS 视口渲染再缩放到屏幕,
        // 导致 100vw != 屏幕宽度, 页面看起来超宽错位.
        // 必须开 useWideViewPort 才会读 <meta name="viewport" content="width=device-width">.
        s.setUseWideViewPort(true);
        s.setLoadWithOverviewMode(true);
        // TV 上禁用内置缩放 (D-pad 无双指捏合, 也避免误放大)
        s.setBuiltInZoomControls(false);
        s.setSupportZoom(false);
        // 关键: 不要 setWebViewClient, 否则覆盖 Capacitor BridgeWebViewClient,
        // 导致 window.Capacitor 不注入, 前端 useViewMode 无法识别 TV 模式 → 页面渲染错乱.
        // 错误监听改用 setWebChromeClient (不冲突), 抓前端 console.error 显示给用户.
        wv.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onProgressChanged(WebView view, int progress) {
                super.onProgressChanged(view, progress);
                if (progress >= 100) hideBootProgress();
            }
            @Override
            public boolean onConsoleMessage(ConsoleMessage cm) {
                if (cm != null && cm.messageLevel() == ConsoleMessage.MessageLevel.ERROR) {
                    final String msg = "JS: " + cm.message()
                            + " (" + cm.sourceId() + ":" + cm.lineNumber() + ")";
                    // 始终记 logcat 供 chrome://inspect / adb 排查;
                    // 仅 debug 包弹 Toast — 生产环境不再用 web error 打扰用户.
                    Log.e("JerocineWeb", msg);
                    if (BuildConfig.DEBUG) {
                        runOnUiThread(() -> GlassToast.show(
                                MainActivity.this, msg, Toast.LENGTH_LONG));
                    }
                }
                return super.onConsoleMessage(cm);
            }
        });
        wv.loadUrl(url);
    }

    /** 当前是否有可用网络连接 (拿不到判定时按"有网"处理, 避免误拦) */
    @SuppressWarnings("deprecation")
    private boolean isOnline() {
        try {
            ConnectivityManager cm =
                    (ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
            if (cm == null) return true;
            NetworkInfo ni = cm.getActiveNetworkInfo();
            return ni != null && ni.isConnected();
        } catch (Exception e) {
            return true;
        }
    }

    /** 显示原生无网络兜底页 (全屏覆盖 + 重试 / 改地址按钮). 懒构建, 复用同一实例. */
    private void showOfflineOverlay() {
        hideBootProgress();
        FrameLayout root = findViewById(android.R.id.content);
        if (root == null) return;
        if (offlineOverlay == null) {
            offlineOverlay = new FrameLayout(this);
            offlineOverlay.setBackgroundColor(GF_BG);
            offlineOverlay.setClickable(true); // 吃掉点击, 别穿到下面 WebView

            // 居中玻璃卡片 (深色玻璃面 + 细描边 + 大圆角), 内容竖排
            LinearLayout col = new LinearLayout(this);
            col.setOrientation(LinearLayout.VERTICAL);
            col.setGravity(Gravity.CENTER);
            col.setBackground(buildGlassPanelBg(20));
            int cardPad = dp(36);
            col.setPadding(cardPad, cardPad, cardPad, cardPad);
            FrameLayout.LayoutParams colLp = new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            colLp.gravity = Gravity.CENTER;
            offlineOverlay.addView(col, colLp);

            TextView title = new TextView(this);
            title.setText("网络未连接");
            title.setTextColor(GF_TEXT);
            title.setTextSize(TypedValue.COMPLEX_UNIT_SP, 24);
            title.setGravity(Gravity.CENTER);
            col.addView(title);

            TextView sub = new TextView(this);
            sub.setText("请检查 WiFi / 网络连接后重试");
            sub.setTextColor(GF_TEXT_SEC);
            sub.setTextSize(TypedValue.COMPLEX_UNIT_SP, 15);
            sub.setGravity(Gravity.CENTER);
            LinearLayout.LayoutParams subLp = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            subLp.topMargin = dp(12);
            subLp.bottomMargin = dp(28);
            col.addView(sub, subLp);

            Button retry = new Button(this);
            retry.setText("重试");
            retry.setTextColor(GF_TEXT);
            retry.setAllCaps(false);
            retry.setBackground(buildDrawerButtonBg());
            retry.setFocusable(true);
            col.addView(retry, new LinearLayout.LayoutParams(dp(240), dp(54)));
            retry.setOnClickListener(v ->
                    loadServer(prefs().getString(KEY_SERVER_URL, DEFAULT_SERVER_URL)));

            Button settings = new Button(this);
            settings.setText("修改服务器地址");
            settings.setTextColor(GF_TEXT);
            settings.setAllCaps(false);
            settings.setBackground(buildDrawerButtonBg());
            settings.setFocusable(true);
            LinearLayout.LayoutParams setLp = new LinearLayout.LayoutParams(dp(240), dp(54));
            setLp.topMargin = dp(14);
            col.addView(settings, setLp);
            settings.setOnClickListener(v -> promptServerUrl(true));

            root.addView(offlineOverlay, new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        }
        offlineOverlay.setVisibility(View.VISIBLE);
        offlineOverlay.bringToFront();
        // 焦点落到"重试"按钮 (遥控器 / D-pad 可直接确认)
        offlineOverlay.post(() -> {
            View c = offlineOverlay.getChildAt(0);
            if (c instanceof LinearLayout) {
                View r = ((LinearLayout) c).getChildAt(2); // title,sub,retry
                if (r != null) r.requestFocus();
            }
        });
    }

    private void hideOfflineOverlay() {
        if (offlineOverlay != null) offlineOverlay.setVisibility(View.GONE);
    }

    /** 弹输入框. isReset=true 时初始填上当前已存的 URL, 方便修改 */
    private void promptServerUrl(boolean isReset) {
        final EditText input = new EditText(this);
        input.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        input.setHint("http://server-ip 或 https://domain");
        // 玻璃暗色弹窗里 EditText 用浅色文字/提示, 光标/下划线走强调青
        input.setTextColor(GF_TEXT);
        input.setHintTextColor(GF_TEXT_TER);
        int inPad = dp(8);
        LinearLayout inWrap = new LinearLayout(this);
        inWrap.setOrientation(LinearLayout.VERTICAL);
        inWrap.setPadding(dp(24), inPad, dp(24), 0);
        inWrap.addView(input);
        String existing = prefs().getString(KEY_SERVER_URL, "");
        if (existing != null && !existing.isEmpty()) {
            input.setText(existing);
            input.setSelection(existing.length());
        }

        AlertDialog dlg = new AlertDialog.Builder(this, com.jerocine.player.R.style.JcPlayerDialog)
                .setTitle(isReset ? "重置 Jerocine 服务器地址" : "请输入 Jerocine 服务器地址")
                .setView(inWrap)
                .setCancelable(false)
                .setPositiveButton("确定", (d, w) -> {
                    String url = input.getText().toString().trim();
                    if (url.isEmpty()) {
                        GlassToast.show(this, "地址不能为空");
                        promptServerUrl(isReset);
                        return;
                    }
                    if (!url.startsWith("http://") && !url.startsWith("https://")) {
                        url = "http://" + url;
                    }
                    prefs().edit().putString(KEY_SERVER_URL, url).apply();
                    loadServer(url);
                })
                .create();
        dlg.show();
    }

    // ======================== Native 设置侧边栏 (按 MENU 唤出) ========================

    private int dp(int v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v,
                getResources().getDisplayMetrics());
    }

    private void buildSettingsDrawer() {
        FrameLayout root = findViewById(android.R.id.content);
        if (root == null) return;
        // 全屏半透明遮罩 (拦截外部点击 = 关抽屉)
        settingsOverlay = new FrameLayout(this);
        settingsOverlay.setBackgroundColor(GF_SCRIM);
        settingsOverlay.setVisibility(View.GONE);
        settingsOverlay.setClickable(true);
        settingsOverlay.setOnClickListener(v -> hideSettingsDrawer());
        root.addView(settingsOverlay, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        // 右侧抽屉外壳 (ScrollView 包裹 — 分组后行数多, 小屏 TV 必须能滚)
        // 玻璃面 (半透明深色) + 左侧细描边, 营造贴边玻璃抽屉感.
        settingsPanelScroll = new ScrollView(this);
        GradientDrawable drawerBg = new GradientDrawable();
        drawerBg.setColor(GF_GLASS);
        drawerBg.setStroke(dp(1), GF_STROKE);
        settingsPanelScroll.setBackground(drawerBg);
        settingsPanelScroll.setClickable(true); // 拦截到自己上的点击, 别冒泡给遮罩
        FrameLayout.LayoutParams panelLp = new FrameLayout.LayoutParams(
                dp(DRAWER_W_DP), ViewGroup.LayoutParams.MATCH_PARENT, Gravity.END);
        settingsOverlay.addView(settingsPanelScroll, panelLp);

        // 真正的行容器
        settingsPanel = new LinearLayout(this);
        settingsPanel.setOrientation(LinearLayout.VERTICAL);
        int pad = dp(20);
        settingsPanel.setPadding(pad, pad, pad, dp(28));
        settingsPanelScroll.addView(settingsPanel, new ScrollView.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT));

        fillSettingsPanel();
    }

    /**
     * 按《TV 模式设置抽屉重设计》§4 组装抽屉内容(分组 + 行)。
     *
     * 分组承载"设备级设置"(无需登录态, Java 直读写原生偏好):
     *   播放(广告过滤) / 网络(中转) / 账号(跳 SPA) / 设备(诊断·服务器) / 关于(更新·平台) / 系统(刷新·显示模式·退出)
     * 账号级设置(跳过秒数 / 登录态)不在这里重复实现, 只给 SPA `/settings?group=...` 入口 ——
     * 避免原生壳再引一套 token 同步。
     */
    private void fillSettingsPanel() {
        settingsPanel.removeAllViews();
        settingsFirstFocus = null;
        adFilterSwitch = null;
        relaySwitch = null;
        displayModeValue = null;

        // ---------- 头部: 标题 + 版本 + ✕ ----------
        LinearLayout head = new LinearLayout(this);
        head.setOrientation(LinearLayout.HORIZONTAL);
        head.setGravity(Gravity.CENTER_VERTICAL);
        TextView title = makeText("设置", 20, GF_TEXT, true);
        head.addView(title, new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f));
        head.addView(makeText("v" + BuildConfig.VERSION_NAME
                + " (" + BuildConfig.VERSION_CODE + ")", 12, GF_TEXT_TER, false));
        TextView closeBtn = makeText("✕", 18, GF_TEXT_SEC, false);
        closeBtn.setGravity(Gravity.CENTER);
        closeBtn.setPadding(dp(10), dp(4), 0, dp(4));
        closeBtn.setFocusable(true);
        closeBtn.setClickable(true);
        closeBtn.setOnClickListener(v -> hideSettingsDrawer());
        head.addView(closeBtn);
        settingsPanel.addView(head, rowParams(ViewGroup.LayoutParams.WRAP_CONTENT, 8));

        // ---------- 播放 ----------
        addGroupHeader("播放");
        addToggleRow("广告过滤",
                "剔除 m3u8 插片/跨域广告 · 默认开启（原生播放器）",
                PlayerAdFilterHelper.isAdFilterEnabled(this),
                sw -> adFilterSwitch = sw,
                () -> {
                    boolean next = !PlayerAdFilterHelper.isAdFilterEnabled(this);
                    PlayerAdFilterHelper.setAdFilterEnabled(this, next);
                    refreshToggle(adFilterSwitch, next);
                    GlassToast.show(this, next ? "广告过滤已开启" : "广告过滤已关闭");
                });
        addNavRow("跳过片头 / 片尾",
                "秒数随账号同步(user_skip_setting) · 登录后多端一致",
                "去设置 ›",
                () -> openSpaSettings("play"));

        // ---------- 网络 ----------
        addGroupHeader("网络");
        addToggleRow("中转播放",
                "分片经服务器转发 · 默认关闭(设备直连更快更省流量)",
                PlayerNetworkModeHelper.isRelayEnabled(this),
                sw -> relaySwitch = sw,
                () -> {
                    boolean next = !PlayerNetworkModeHelper.isRelayEnabled(this);
                    PlayerNetworkModeHelper.setRelayEnabled(this, next);
                    refreshToggle(relaySwitch, next);
                    GlassToast.show(this, next
                            ? "中转已开启 · 仅直连异常时经服务器转发(不一定更快, 更耗带宽)"
                            : "中转已关闭 · 设备直连播放, 更快更省流量", Toast.LENGTH_LONG);
                });

        // ---------- 账号 (账号级设置留在 SPA, 这里只给入口) ----------
        addGroupHeader("账号");
        addNavRow("账号设置", "登录 / 跳过秒数 / 退出登录", "打开 ›",
                () -> openSpaSettings("account"));

        // ---------- 设备 ----------
        addGroupHeader("设备");
        addNavRow("设备诊断", "机型 / 系统 / WebView 内核 / 内存", "",
                this::onDeviceDiagnosticsClick);
        addNavRow("服务器地址", prefs().getString(KEY_SERVER_URL, ""), "修改 ›",
                () -> promptServerUrl(true));

        // ---------- 关于 ----------
        addGroupHeader("关于");
        addNavRow("检查更新", "GET /app/version/latest", "立即检查",
                this::checkUpdateExposed);
        addInfoRow("运行平台", "Android " + Build.VERSION.RELEASE
                + " (API " + Build.VERSION.SDK_INT + ")");

        // ---------- 系统 ----------
        addGroupHeader("系统");
        addNavRow("刷新页面", "重新加载当前站点", "", () -> {
            if (webViewRef != null) webViewRef.reload();
            hideSettingsDrawer();
        });
        displayModeValue = addNavRow("显示模式", "循环切换: TV → 桌面 → 自动",
                SettingsDrawerLogic.displayModeLabel(null), this::cycleDisplayMode);
        addNavRow("退出应用", "", "", () -> {
            hideSettingsDrawer();
            finishAffinity();
        });
    }

    private void onDeviceDiagnosticsClick() {
        hideSettingsDrawer();
        showDeviceDiagnostics();
    }

    /** 显示模式三态循环: TV → 桌面 → 自动(清除) → TV…; 写完后就地更新右侧状态 */
    private void cycleDisplayMode() {
        String next = SettingsDrawerLogic.nextDisplayMode(currentDisplayMode);
        persistViewMode(next); // 内部写 localStorage + reload + 关抽屉
        GlassToast.show(this, SettingsDrawerLogic.displayModeToast(next));
    }

    /** 打开 SPA 设置页的某个分组(账号级设置仍由 web 承载) */
    private void openSpaSettings(String group) {
        hideSettingsDrawer();
        if (webViewRef == null) return;
        String base = prefs().getString(KEY_SERVER_URL, DEFAULT_SERVER_URL);
        webViewRef.loadUrl(SettingsDrawerLogic.spaSettingsUrl(base, group));
    }

    /** 读一次 WebView localStorage 里的显示模式, 刷新"显示模式"行右侧状态 */
    private void refreshDisplayModeValue() {
        if (displayModeValue == null || webViewRef == null) return;
        webViewRef.evaluateJavascript(
                "(function(){try{return localStorage.getItem('jc-mode')||''}catch(e){return ''}})();",
                value -> {
                    String raw = value == null ? "" : value.replaceAll("^\"|\"$", "");
                    if ("null".equals(raw)) raw = "";
                    final String mode = raw;
                    currentDisplayMode = mode.isEmpty() ? null : mode;
                    runOnUiThread(() -> {
                        if (displayModeValue != null) {
                            displayModeValue.setText(SettingsDrawerLogic.displayModeLabel(mode));
                        }
                    });
                });
    }

    // ==================== 抽屉行 helper (分组化 UI, 见抽屉重设计 §4) ====================
    // 行规格: 高 56dp; 主标题 16sp(白 92%); 副标题 12sp(白 55%); 右侧控件右对齐;
    // 组标题 12sp(白 40%) 行高 32dp + 顶部 16dp 留白。

    private TextView makeText(String text, float sp, int color, boolean bold) {
        TextView t = new TextView(this);
        t.setText(text);
        t.setTextColor(color);
        t.setTextSize(TypedValue.COMPLEX_UNIT_SP, sp);
        if (bold) t.setPaintFlags(t.getPaintFlags() | android.graphics.Paint.FAKE_BOLD_TEXT_FLAG);
        return t;
    }

    private LinearLayout.LayoutParams rowParams(int height, int topMarginDp) {
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, height);
        lp.topMargin = dp(topMarginDp);
        return lp;
    }

    /** 组标题: 小字弱化 + 左侧竖条, 行高 32dp */
    private void addGroupHeader(String label) {
        LinearLayout wrap = new LinearLayout(this);
        wrap.setOrientation(LinearLayout.HORIZONTAL);
        wrap.setGravity(Gravity.BOTTOM);
        // 组标题与行之间留一点呼吸(不然 12sp 小字贴着行上沿)
        wrap.setPadding(0, 0, 0, 0);

        View bar = new View(this);
        GradientDrawable barBg = new GradientDrawable();
        barBg.setColor(GF_ACCENT);
        barBg.setCornerRadius(dp(2));
        LinearLayout.LayoutParams barLp = new LinearLayout.LayoutParams(dp(3), dp(12));
        barLp.bottomMargin = dp(6);
        wrap.addView(bar, barLp);

        TextView t = makeText(label, 12, GF_TEXT_TER, true);
        t.setLetterSpacing(0.18f);
        LinearLayout.LayoutParams tLp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        tLp.leftMargin = dp(8);
        tLp.bottomMargin = dp(4);
        wrap.addView(t, tLp);

        LinearLayout.LayoutParams lp = rowParams(dp(32), 16);
        lp.bottomMargin = dp(2);
        settingsPanel.addView(wrap, lp);
    }

    /** 行基底: 玻璃底 + 焦点态 + 左(主/副标题)右(控件) 两栏; focusable=false 时纯展示 */
    private LinearLayout makeRowBase(String title, String sub, boolean focusable,
                                     View.OnClickListener click) {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        row.setBackground(buildDrawerButtonBg());
        int padH = dp(16);
        row.setPadding(padH, 0, padH, 0);
        row.setFocusable(focusable);
        row.setClickable(focusable);
        if (focusable) {
            row.setOnClickListener(click);
            if (settingsFirstFocus == null) settingsFirstFocus = row;
        }

        LinearLayout col = new LinearLayout(this);
        col.setOrientation(LinearLayout.VERTICAL);
        col.setGravity(Gravity.CENTER_VERTICAL);
        TextView t = makeText(title, 16, GF_TEXT, false);
        col.addView(t, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        if (sub != null && !sub.isEmpty()) {
            TextView s = makeText(sub, 12, GF_TEXT_SEC, false);
            s.setMaxLines(2);
            LinearLayout.LayoutParams sLp = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            sLp.topMargin = dp(2);
            col.addView(s, sLp);
        }
        row.addView(col, new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f));
        return row;
    }

    /**
     * 右侧控件必须 **不可聚焦/不可点** —— 否则 D-pad 焦点会被它抢走(踩过的坑),
     * 行整体才是唯一的交互目标。
     */
    private void addRowTail(LinearLayout row, View tail) {
        tail.setFocusable(false);
        tail.setClickable(false);
        tail.setFocusableInTouchMode(false);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        lp.leftMargin = dp(12);
        row.addView(tail, lp);
    }

    /** 开关行: 整行可点(OK 键切换), 右侧是纯展示的开关图形 */
    private void addToggleRow(String title, String sub, boolean on,
                              java.util.function.Consumer<FrameLayout> switchOut,
                              Runnable onToggle) {
        LinearLayout row = makeRowBase(title, sub, true, v -> onToggle.run());
        FrameLayout sw = buildToggleSwitch(on);
        if (switchOut != null) switchOut.accept(sw);
        addRowTail(row, sw);
        settingsPanel.addView(row, rowParams(dp(ROW_H_DP), 6));
    }

    /** 跳转/动作行: 右侧是"打开 ›/立即检查"这类提示文案 */
    private TextView addNavRow(String title, String sub, String value, Runnable action) {
        LinearLayout row = makeRowBase(title, sub, true, v -> action.run());
        TextView v = makeText(value == null ? "" : value, 13, GF_ACCENT, false);
        addRowTail(row, v);
        settingsPanel.addView(row, rowParams(dp(ROW_H_DP), 6));
        return v;
    }

    /** 纯信息行(不可聚焦, 不进 D-pad 焦点链) */
    private void addInfoRow(String title, String value) {
        LinearLayout row = makeRowBase(title, null, false, null);
        TextView v = makeText(value == null ? "" : value, 13, GF_TEXT_SEC, false);
        addRowTail(row, v);
        settingsPanel.addView(row, rowParams(dp(ROW_H_DP), 6));
    }

    /** 开关图形: 44×24dp 药丸 + 18dp 圆点(纯展示, 不接收事件) */
    private FrameLayout buildToggleSwitch(boolean on) {
        FrameLayout sw = new FrameLayout(this);
        sw.setLayoutParams(new LinearLayout.LayoutParams(dp(44), dp(24)));
        applyToggleLook(sw, on);
        View knob = new View(this);
        sw.addView(knob, knobParams(on));
        sw.setTag(knob);
        return sw;
    }

    /** 就地刷新开关态(不重建整个面板 ⇒ 遥控器焦点不丢) */
    private void refreshToggle(FrameLayout sw, boolean on) {
        if (sw == null) return;
        applyToggleLook(sw, on);
        Object knob = sw.getTag();
        if (knob instanceof View) {
            // 换 knob 的 margin 而非重建: 用 layoutParams 直接改 gravity
            FrameLayout.LayoutParams lp = (FrameLayout.LayoutParams)
                    ((View) knob).getLayoutParams();
            lp.gravity = (on ? Gravity.END : Gravity.START) | Gravity.CENTER_VERTICAL;
            ((View) knob).setLayoutParams(lp);
        }
    }

    private void applyToggleLook(FrameLayout sw, boolean on) {
        GradientDrawable track = new GradientDrawable();
        track.setCornerRadius(dp(12));
        track.setColor(on ? GF_ACCENT : 0x33FFFFFF);
        sw.setBackground(track);
    }

    private FrameLayout.LayoutParams knobParams(boolean on) {
        FrameLayout.LayoutParams klp = new FrameLayout.LayoutParams(dp(18), dp(18));
        klp.gravity = (on ? Gravity.END : Gravity.START) | Gravity.CENTER_VERTICAL;
        return klp;
    }

    private Drawable buildDrawerButtonBg() {
        // 玻璃风按钮: 默认半透明深玻璃面 + 细描边; 聚焦 = 强调青描边 + 半透明青底(D-pad 焦点明显);
        // 按下 = 略深青底. 大圆角 14dp. (不依赖 RenderEffect, 弱机/TV 也流畅)
        GradientDrawable focused = new GradientDrawable();
        focused.setColor(GF_ACCENT_DIM);
        focused.setCornerRadius(dp(14));
        focused.setStroke(dp(2), GF_ACCENT);

        GradientDrawable pressed = new GradientDrawable();
        pressed.setColor(GF_ACCENT_PRESS);
        pressed.setCornerRadius(dp(14));

        GradientDrawable normal = new GradientDrawable();
        normal.setColor(GF_GLASS_SOFT);
        normal.setCornerRadius(dp(14));
        normal.setStroke(dp(1), GF_STROKE);

        StateListDrawable sld = new StateListDrawable();
        sld.addState(new int[]{android.R.attr.state_focused}, focused);
        sld.addState(new int[]{android.R.attr.state_pressed}, pressed);
        sld.addState(android.util.StateSet.WILD_CARD, normal);
        return sld;
    }

    /** 玻璃浮层/卡片背景: 半透明深色面 + 1px 细描边 + 大圆角(供抽屉/无网络页等编程式浮层复用)。 */
    private Drawable buildGlassPanelBg(int radiusDp) {
        GradientDrawable g = new GradientDrawable();
        g.setColor(GF_GLASS);
        g.setCornerRadius(dp(radiusDp));
        g.setStroke(dp(1), GF_STROKE);
        return g;
    }

    /** 在 WebView 内 localStorage 写 jc-mode, 然后 reload 让 useViewMode 生效 */
    private void persistViewMode(String mode) {
        if (webViewRef == null) return;
        final String js;
        if (mode == null) {
            js = "try{localStorage.removeItem('jc-mode')}catch(e){};location.reload();";
        } else {
            js = "try{localStorage.setItem('jc-mode','" + mode + "')}catch(e){};location.reload();";
        }
        webViewRef.evaluateJavascript(js, null);
        hideSettingsDrawer();
    }

    /** 抓 WebView 实际宽 + 屏幕宽 + data-mode + 滚动宽 — 帮排查"页面太宽"是哪一层的事 */
    private void showDiagToast() {
        if (webViewRef == null) {
            GlassToast.show(this, "WebView 未就绪", Toast.LENGTH_LONG);
            return;
        }
        android.util.DisplayMetrics dm = getResources().getDisplayMetrics();
        final String prefix = "屏: " + dm.widthPixels + "x" + dm.heightPixels
                + " density=" + dm.density
                + "\nWebView: " + webViewRef.getWidth() + "x" + webViewRef.getHeight();
        webViewRef.evaluateJavascript(
                "(function(){try{return JSON.stringify({" +
                        "innerW:window.innerWidth," +
                        "innerH:window.innerHeight," +
                        "docW:document.documentElement.scrollWidth," +
                        "docH:document.documentElement.scrollHeight," +
                        "dpr:window.devicePixelRatio," +
                        "mode:document.documentElement.getAttribute('data-mode')," +
                        "ua:navigator.userAgent.slice(0,80)" +
                        "})}catch(e){return JSON.stringify({err:String(e)})}})()",
                value -> {
                    String stripped = value == null ? "null"
                            : value.replaceAll("^\"|\"$", "").replace("\\\"", "\"");
                    final String msg = prefix + "\nweb: " + stripped;
                    runOnUiThread(() -> {
                        new AlertDialog.Builder(this, com.jerocine.player.R.style.JcPlayerDialog)
                                .setTitle("诊断")
                                .setMessage(msg)
                                .setPositiveButton("OK", null)
                                .show();
                    });
                }
        );
    }

    /**
     * 设备诊断对话框 — 安装后异常排查用(与 tv 原生版 DeviceDiagnostics 对齐):
     * 机型 / Android 版本 / WebView 内核 / 分辨率 / 内存 / 应用版本等, 并给出
     * "当前 TV 是否适合 Web 嵌入方式"的判定参考.
     */
    private void showDeviceDiagnostics() {
        StringBuilder sb = new StringBuilder();
        sb.append("机型: ").append(Build.MANUFACTURER).append(' ').append(Build.MODEL).append('\n');
        sb.append("Android: ").append(Build.VERSION.RELEASE)
                .append(" (API ").append(Build.VERSION.SDK_INT).append(")\n");
        sb.append("ABI: ").append(Build.SUPPORTED_ABIS != null && Build.SUPPORTED_ABIS.length > 0
                ? Build.SUPPORTED_ABIS[0] : "未知").append('\n');
        sb.append("应用版本: ").append(BuildConfig.VERSION_NAME)
                .append(" (").append(BuildConfig.VERSION_CODE).append(")\n");
        android.util.DisplayMetrics dm = getResources().getDisplayMetrics();
        sb.append("分辨率: ").append(dm.widthPixels).append('x').append(dm.heightPixels)
                .append(" density=").append(dm.density).append('\n');
        android.app.ActivityManager am =
                (android.app.ActivityManager) getSystemService(Context.ACTIVITY_SERVICE);
        if (am != null) {
            android.app.ActivityManager.MemoryInfo mi = new android.app.ActivityManager.MemoryInfo();
            am.getMemoryInfo(mi);
            sb.append("内存: 总计 ").append(mi.totalMem / 1024 / 1024 / 1024)
                    .append("G, 可用 ").append(mi.availMem / 1024 / 1024)
                    .append("M\n");
        }
        WebView wv = (bridge != null) ? bridge.getWebView() : null;
        if (wv != null) {
            sb.append("WebView 内核: ").append(WebSettings.getDefaultUserAgent(wv.getContext())).append('\n');
        } else {
            sb.append("WebView 内核: 未就绪\n");
        }
        // Web 嵌入方式支持判定: 低内存 / 老 Android(API<21) / 无 WebView 内核 建议用低配独立 APK
        int api = Build.VERSION.SDK_INT;
        long memGb = 0;
        if (am != null) {
            android.app.ActivityManager.MemoryInfo mi = new android.app.ActivityManager.MemoryInfo();
            am.getMemoryInfo(mi);
            memGb = mi.totalMem / 1024 / 1024 / 1024;
        }
        sb.append('\n').append("建议: ");
        if (api < 23 || (memGb > 0 && memGb < 2)) {
            sb.append("设备偏旧/内存偏小, Web 嵌入方式可能卡顿, 建议改用低配独立版 APK");
        } else {
            sb.append("设备满足 Web 嵌入方式运行要求(WebView 内核正常即可)");
        }
        new AlertDialog.Builder(this, com.jerocine.player.R.style.JcPlayerDialog)
                .setTitle("设备诊断")
                .setMessage(sb.toString())
                .setPositiveButton("OK", null)
                .show();
    }

    /** 打开原生设置抽屉 — 供 MENU 键(dispatchKeyEvent) 与 JerocineBridge 的
     *  openSettings(web 胶囊行"设置"按钮 / 首页"我的"卡) 共用; 命名与
     *  checkUpdateExposed / promptServerUrlExposed 保持一致。 */
    void showSettingsDrawerExposed() {
        if (settingsOverlay == null) buildSettingsDrawer();
        if (settingsOverlay == null) return;
        // 服务器地址是"值行"(组装时已写入), 显示模式存在 WebView localStorage ⇒ 打开时异步同步一次
        refreshDisplayModeValue();
        settingsOpen = true;
        // 关键: 抽屉打开时禁掉 WebView 的可聚焦, 不然 D-pad 事件被 WebView 抢走,
        // 按钮焦点上不去. 关抽屉时恢复.
        if (webViewRef != null) {
            webViewRef.setFocusable(false);
            webViewRef.setFocusableInTouchMode(false);
        }
        settingsOverlay.setFocusableInTouchMode(true);
        settingsOverlay.setFocusable(true);
        settingsOverlay.setVisibility(View.VISIBLE);
        // 滚回顶部 — 上次关时滚到中间的话, 重开应从头开始
        settingsPanelScroll.scrollTo(0, 0);
        // 等布局测量后再开始动画 + 抢焦点 (animate ScrollView 本身, 它是右边贴边的)
        settingsPanelScroll.post(() -> {
            float w = settingsPanelScroll.getWidth() > 0 ? settingsPanelScroll.getWidth() : dp(DRAWER_W_DP);
            settingsPanelScroll.setTranslationX(w);
            settingsPanelScroll.animate().translationX(0f).setDuration(220).start();
            // 焦点定到第一行可聚焦的项(组装时记录, 不再靠固定下标 —— 分组后行序会变)
            if (settingsFirstFocus != null) settingsFirstFocus.requestFocus();
        });
    }

    private void hideSettingsDrawer() {
        if (!settingsOpen || settingsOverlay == null) return;
        settingsOpen = false;
        // 恢复 WebView 焦点能力
        if (webViewRef != null) {
            webViewRef.setFocusable(true);
            webViewRef.setFocusableInTouchMode(true);
            webViewRef.requestFocus();
        }
        float w = settingsPanelScroll.getWidth() > 0 ? settingsPanelScroll.getWidth() : dp(360);
        settingsPanelScroll.animate().translationX(w).setDuration(180).withEndAction(
                () -> settingsOverlay.setVisibility(View.GONE)).start();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        // 菜单键: 弹出 native 设置侧边栏 (不再转发给 web — 设置应该是壳层能力,
        // 后续要改菜单条目直接改 APK, 不依赖 web 是否就绪)
        if (event.getKeyCode() == KeyEvent.KEYCODE_MENU
                && event.getAction() == KeyEvent.ACTION_DOWN) {
            if (settingsOpen) hideSettingsDrawer();
            else showSettingsDrawerExposed();
            return true;
        }
        if (event.getKeyCode() == KeyEvent.KEYCODE_BACK
                && event.getAction() == KeyEvent.ACTION_DOWN) {
            return handleBackPressed();
        }
        return super.dispatchKeyEvent(event);
    }

    /**
     * 返回统一出口 — 遥控器 BACK、系统手势返回(onBackPressed)共用同一套逻辑:
     * 抽屉开着 → 先关抽屉; 否则转前端路由(window.gfTvBack); 前端不消费(已在首页) → 双击返回退出。
     */
    private boolean handleBackPressed() {
        // 抽屉打开时, BACK 仅关抽屉
        if (settingsOpen) {
            hideSettingsDrawer();
            return true;
        }
        // 转给前端 router (window.gfTvBack); 首页(前端不消费)则双击返回退出。
        WebView webView = (bridge != null) ? bridge.getWebView() : null;
        if (webView != null) {
            webView.evaluateJavascript(
                    "(function(){try{return !!(window.gfTvBack&&window.gfTvBack());}catch(e){return false;}})();",
                    new ValueCallback<String>() {
                        @Override
                        public void onReceiveValue(String value) {
                            if ("true".equals(value)) return;
                            // 前端没消费 = 已在首页 → 双击返回退出应用 (与遥控器返回一致)
                            runOnUiThread(() -> {
                                long t = System.currentTimeMillis();
                                if (t - lastExitBackAt < EXIT_CONFIRM_MS) {
                                    lastExitBackAt = 0L;
                                    finishAffinity(); // 真正退出应用
                                } else {
                                    lastExitBackAt = t;
                                    GlassToast.show(MainActivity.this, "再按一次退出应用");
                                }
                            });
                        }
                    });
            return true; // 异步, 先吞掉
        }
        return false; // 无 WebView 时交由 onBackPressed() 走系统默认
    }

    /**
     * 系统返回 / 触屏返回按钮 / 左滑手势返回(Android 10+ 手势导航) — 统一走 handleBackPressed()。
     * 否则平板/手机装壳版时, 系统返回会直接退掉应用而不是先回上一页。
     */
    @Override
    public void onBackPressed() {
        if (!handleBackPressed()) {
            super.onBackPressed();
        }
    }
}
