package com.jerocine.app;

import com.jerocine.player.JerocinePlayer;
import com.jerocine.player.PlayerAdFilterHelper;
import com.jerocine.player.data.ServerUrl;
import com.jerocine.player.ui.BrandSplashView;
import com.jerocine.player.ui.GlassToast;
import com.jerocine.player.ui.JcDeviceDiagnostics;
import com.jerocine.player.ui.JcServerUrlPrompt;
import com.jerocine.player.ui.JcSettingsDrawer;

import android.content.Context;
import android.content.SharedPreferences;
import android.net.ConnectivityManager;
import android.net.NetworkInfo;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
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
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import com.getcapacitor.BridgeActivity;
import com.jerocine.player.data.DoubleBackExitGate;
import com.jerocine.player.data.NetLink;
import com.jerocine.player.ui.JcUi;

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
public class WebShellActivity extends BridgeActivity
        implements ServerNavigationWebViewClient.WebViewFallbackCallback,
                   JcSettingsDrawer.JcSettingsHost {

    // 壳层共享 prefs 契约(文件名/键名)只在 com.jerocine.player.data.ShellPrefs 定义一次 ——
    // 合并成单包后原生壳读写的是同一份 prefs, 键名漂移会导致"切壳后设置/登录态错配"。
    static final String PREFS = com.jerocine.player.data.ShellPrefs.NAME;
    static final String KEY_SERVER_URL = com.jerocine.player.data.ShellPrefs.KEY_SERVER_URL;

    // 玻璃暗色设计令牌的**唯一真源**是 player-core 的 res/values/colors.xml（library 资源合并进本模块 R），
    // 编程式 UI 一律经 col(com.jerocine.player.R.color.jc_*) 取色（公式下沉 core 的 JcUi，见 dp/col 转引）——
    // 壳里不再存第二份十六进制。
    // 历史: 本类曾与 core 的 JcSettingsDrawer 各有一套完全相同的 GF_* 字面量(共 3 份含资源),
    // 改色必漏改一处; 2026-10-12 收编(见 docs/单包合并架构复盘与共性件组织方案-2026-10-12.md §3.4)。
    /** 出厂站点地址(web 壳加载页默认, 设置抽屉可改) —— 构建期注入(JEROCINE_WEB_BASE), 公开仓不含真实域名 */
    private static final String DEFAULT_SERVER_URL = BuildConfig.JEROCINE_WEB_BASE;
    /** 首页双击返回退出应用的确认窗口 */
    private static final long EXIT_CONFIRM_MS = 2000L;
    /** 品牌首屏最长展示时间 — 超时强制淡出, 露出 WebView 内容/错误(方案 §7.3 兜底) */
    private static final long SPLASH_TIMEOUT_MS = 15000L;

    /**
     * 双击返回退出确认窗口 —— 纯逻辑在 core 的 DoubleBackExitGate(组件化二期 A4 下沉,
     * 与原生壳 NativeShellActivity 同一份); 时钟用 elapsedRealtime(单调), 与原生壳一致。
     */
    private final DoubleBackExitGate exitGate = new DoubleBackExitGate(EXIT_CONFIRM_MS);
    private UpdateChecker updateChecker;
    /** 启动品牌首屏(替代原裸转圈) */
    private BrandSplashView brandSplash;
    /** package-private — JerocineBridge.handleEchoTest 需要直接拿到 WebView 发反向事件 */
    WebView webViewRef;
    /** 当前服务器 host(loadServer 每次刷新) —— 供 ServerNavigationWebViewClient 判定"站内导航" */
    private String currentServerHost;
    /** 设置抽屉(core 的 JcSettingsDrawer, web 壳与原生壳共用同一份) */
    private JcSettingsDrawer settingsDrawer;
    /** 最近一次读到的 web 登录态(决定账号行的动作语义) */
    private boolean accountLoggedIn;
    /** 最近一次读到的 web 昵称(仅用于账号行标题; 拿不到则回退"已登录") */
    private String currentAccountName = "";
    /** 当前显示模式覆盖值: tv / desktop / null(=没选过, 壳内默认强制 TV) */
    private String currentDisplayMode;
    static final String KEY_SHELL_MODE = com.jerocine.player.data.ShellPrefs.KEY_SHELL_MODE;
    /** 断网兜底页 (原生全屏覆盖, 带重试按钮) */
    private FrameLayout offlineOverlay;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        // 系统 Splash(Android 12+ 及 core-splashscreen 兼容层): 必须在 super.onCreate 之前安装,
        // 否则 AppTheme.NoActionBarLaunch 的 postSplashScreenTheme 不生效, "系统 splash 无缝接品牌首屏"
        // 的前提不成立(评审 C10)。背景为纯黑, 图标即品牌图标, 与下面的 BrandSplashView 视觉连续。
        androidx.core.splashscreen.SplashScreen.installSplashScreen(this);
        // ⚠️ 显式切到正式主题 —— 不能依赖 postSplashScreenTheme(2026-10-10 实锤):
        // core-splashscreen 1.2.0 反编译, 只有 API<31 的 Impl.install() 调 Activity.setTheme,
        // API 31+ 的 Impl31.install() 全程不 setTheme ⇒ 31+ 设备上启动主题(其 profile 里
        // 曾有 android:background)一直生效, 编程式创建的抽屉行/文字列全吃到默认黑背景。
        // 在此显式 setTheme 才是所有 API 上一致的做法(必须在任何 View 创建之前)。
        setTheme(R.style.AppTheme_NoActionBar);
        super.onCreate(savedInstanceState);
        registerBackHandler();
        // 底部系统导航栏(手势条/三键条)与页面背景同色 — 装平板/手机时避免黑白条突兀
        if (Build.VERSION.SDK_INT >= 21) {
            getWindow().setNavigationBarColor(col(com.jerocine.player.R.color.jc_bg));
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
        // 与 tv 原生壳 NativeShellActivity(onBackPressedDispatcher.addCallback) 机制保持一致.
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
        promptServerUrl();
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
        // 关键: 不要直接 `wv.setWebViewClient(...)` 换成裸 WebViewClient, 否则覆盖 Capacitor
        // BridgeWebViewClient, 导致 window.Capacitor 不注入, 前端 useViewMode 无法识别 TV 模式
        // → 页面渲染错乱。要改导航判定必须**继承** BridgeWebViewClient 并经 bridge.setWebViewClient
        // 安装(见 installServerNavigationClient)。错误监听改用 setWebChromeClient (不冲突),
        // 抓前端 console.error 显示给用户。
        //
        // 站内顶层导航留在 WebView(2026-10-10 真机修复): 本壳的 SPA 是**远程加载**的, 而
        // Capacitor 只把 appUrl(http://localhost) 与 server.allowNavigation 当站内,
        // 于是 https://<server>/... 的顶层导航会被 Intent.ACTION_VIEW 丢给系统浏览器 ——
        // 顶栏「刷新」按钮的 location.reload() 正好命中。详见 ServerNavigationWebViewClient。
        currentServerHost = ServerUrl.hostOf(url);
        installServerNavigationClient();

        wv.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onProgressChanged(WebView view, int progress) {
                super.onProgressChanged(view, progress);
                if (progress >= 100) {
                    hideBootProgress();
                    // P2.3: 页面加载成功, 重置 WebView 失败计数
                    if (bridge != null
                            && bridge.getWebViewClient() instanceof ServerNavigationWebViewClient) {
                        ((ServerNavigationWebViewClient) bridge.getWebViewClient()).onPageLoadSuccess();
                    }
                }
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
                                WebShellActivity.this, msg, Toast.LENGTH_LONG));
                    }
                }
                return super.onConsoleMessage(cm);
            }
        });
        wv.loadUrl(url);
    }

    /**
     * 安装"站内导航判定"(幂等). 让指向当前服务器 host 的顶层导航留在 WebView 内,
     * 而不是被 Capacitor 当外链丢给系统浏览器(见 ServerNavigationWebViewClient 注释).
     *
     * 必须在 Capacitor 初始化 Bridge 之后调用(即 onCreate 里 super 之后), 且必须**继承**
     * BridgeWebViewClient —— 换成裸 WebViewClient 会丢掉 Capacitor 的本地资源拦截能力.
     * 经 Bridge.setWebViewClient 设置, 顺带把 bridge 内部的引用也换成同一个实例.
     */
    private void installServerNavigationClient() {
        if (bridge == null) {
            return;
        }
        if (bridge.getWebViewClient() instanceof ServerNavigationWebViewClient) {
            return;
        }
        bridge.setWebViewClient(
                new ServerNavigationWebViewClient(bridge, () -> currentServerHost, this, prefs()));
    }

    /**
     * P2.3 运行时降级: WebView 连续失败/渲染进程崩溃后触发。
     * ServerNavigationWebViewClient 已写 prefs(jc-shell-mode=native),
     * 这里负责重启应用让 EntryRouter 走原生壳。
     */
    @Override
    public void onWebViewUnrecoverable() {
        runOnUiThread(() -> {
            GlassToast.show(this, "Web 容器异常，切换为原生模式，重启中…",
                    android.widget.Toast.LENGTH_LONG);
            // 用 core 的重启(重建任务栈 → EntryRouter 重新路由): 降级后自动进入原生壳。
            // 旧实现只 finishAffinity + System.exit, 会停在桌面, 用户得自己找图标重开。
            new android.os.Handler(android.os.Looper.getMainLooper())
                    .postDelayed(() -> JcSettingsDrawer.restartApp(this), 1500);
        });
    }

    // Capacitor BridgeActivity.onDestroy 是 public —— 保持同可见性(缩小即编译错)。
    @Override
    public void onDestroy() {
        // 进程级静态单例(JerocinePlayer.Config)的回调捕获了本 Activity 的 webViewRef ——
        // 不清掉会钉住整个 Activity+WebView 泄漏, 且回调继续向已销毁 WebView evaluateJavascript。
        // 同 NativeShellActivity.onDestroy 范式。
        JerocinePlayer.config().setCallback(null);
        super.onDestroy();
    }

    /**
     * 当前是否有可用网络连接 —— 判定公式下沉 core 的 NetLink(组件化二期 A5)。
     * 方向 = {@code FAIL_OPEN}: 拿不到 ConnectivityManager / 探测异常时按"有网"兜底,
     * 避免把有网用户误拦在离线兜底页(下行 loadServer 的前置判定, 错拦代价高)。
     * 与旧实现的逐分支对照: cm==null→有网 / 探测异常→有网 / ni==null→无网 /
     * isConnected 决定 —— 全部保持不变。
     */
    @SuppressWarnings("deprecation")
    private boolean isOnline() {
        try {
            ConnectivityManager cm =
                    (ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
            if (cm == null) return NetLink.isOnline(null, NetLink.FAIL_OPEN);
            NetworkInfo ni = cm.getActiveNetworkInfo();
            return NetLink.isOnline(ni == null ? Boolean.FALSE : ni.isConnected(),
                    NetLink.FAIL_OPEN);
        } catch (Exception e) {
            return NetLink.isOnline(null, NetLink.FAIL_OPEN);
        }
    }

    /** 显示原生无网络兜底页 (全屏覆盖 + 重试 / 改地址按钮). 懒构建, 复用同一实例. */
    private void showOfflineOverlay() {
        hideBootProgress();
        FrameLayout root = findViewById(android.R.id.content);
        if (root == null) return;
        if (offlineOverlay == null) {
            offlineOverlay = new FrameLayout(this);
            offlineOverlay.setBackgroundColor(col(com.jerocine.player.R.color.jc_bg));
            offlineOverlay.setClickable(true); // 吃掉点击, 别穿到下面 WebView

            // 居中玻璃卡片 (深色玻璃面 + 细描边 + 大圆角), 内容竖排
            LinearLayout col = new LinearLayout(this);
            col.setOrientation(LinearLayout.VERTICAL);
            col.setGravity(Gravity.CENTER);
            col.setBackground(JcUi.glassPanelBg(this, 20));
            int cardPad = dp(36);
            col.setPadding(cardPad, cardPad, cardPad, cardPad);
            FrameLayout.LayoutParams colLp = new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            colLp.gravity = Gravity.CENTER;
            offlineOverlay.addView(col, colLp);

            TextView title = new TextView(this);
            title.setText("网络未连接");
            title.setTextColor(col(com.jerocine.player.R.color.jc_text));
            title.setTextSize(TypedValue.COMPLEX_UNIT_SP, 24);
            title.setGravity(Gravity.CENTER);
            col.addView(title);

            TextView sub = new TextView(this);
            sub.setText("请检查 WiFi / 网络连接后重试");
            sub.setTextColor(col(com.jerocine.player.R.color.jc_text_secondary));
            sub.setTextSize(TypedValue.COMPLEX_UNIT_SP, 15);
            sub.setGravity(Gravity.CENTER);
            LinearLayout.LayoutParams subLp = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            subLp.topMargin = dp(12);
            subLp.bottomMargin = dp(28);
            col.addView(sub, subLp);

            Button retry = new Button(this);
            retry.setText("重试");
            retry.setTextColor(col(com.jerocine.player.R.color.jc_text));
            retry.setAllCaps(false);
            retry.setBackground(JcUi.glassButtonBg(this));
            retry.setFocusable(true);
            col.addView(retry, new LinearLayout.LayoutParams(dp(240), dp(54)));
            retry.setOnClickListener(v ->
                    loadServer(prefs().getString(KEY_SERVER_URL, DEFAULT_SERVER_URL)));

            Button settings = new Button(this);
            settings.setText("修改站点地址");
            settings.setTextColor(col(com.jerocine.player.R.color.jc_text));
            settings.setAllCaps(false);
            settings.setBackground(JcUi.glassButtonBg(this));
            settings.setFocusable(true);
            LinearLayout.LayoutParams setLp = new LinearLayout.LayoutParams(dp(240), dp(54));
            setLp.topMargin = dp(14);
            col.addView(settings, setLp);
            settings.setOnClickListener(v -> promptServerUrl());

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

    /** 弹站点地址输入框 —— 弹窗与归一化下沉在 core 的 JcServerUrlPrompt(两壳同一份),
     *  本壳只保留保存回调: 写 prefs + 镜像 ServiceLocator + 重新加载。 */
    private void promptServerUrl() {
        JcServerUrlPrompt.show(this, prefs().getString(KEY_SERVER_URL, ""), url -> {
            prefs().edit().putString(KEY_SERVER_URL, url).apply();
            // P2.1 数据一致性: 后端地址也写进公共模块(→ TokenStore)。
            // 单包内两个壳必须收敛到同一个地址, 否则切壳模式后会连到不同后端
            // (web 壳读 jerocine_prefs/server_url, 原生壳读 TokenStore.server)。
            com.jerocine.tv.data.ServiceLocator.setServer(url);
            loadServer(url);
        });
    }

    // ======================== Native 设置侧边栏 (按 MENU 唤出) ========================

    // dp()/col() 公式已下沉 core 的 JcUi(组件化二期 A3, 值不变) —— 这里一行转引, 调用点保持不变。

    private int dp(int v) {
        return JcUi.dp(this, v);
    }

    /**
     * 从资源取色 —— 编程式 View 构建的**唯一取色入口**。
     * 色值唯一真源是 player-core 的 res/values/colors.xml（library 资源合并进本模块 R），
     * 本类不写任何十六进制（见类顶部注释）。
     */
    private int col(int colorResId) {
        return JcUi.col(this, colorResId);
    }

    // ======================== JcSettingsDrawer.JcSettingsHost 实现 ========================
    // 通用设置抽屉以 web 壳原有样式为基础, 原生壳共用; 壳特有操作经此回调。

    @Override public void toast(String msg) { GlassToast.show(this, msg); }
    @Override public void onRefreshPage() {
        if (webViewRef != null) webViewRef.reload();
    }
    @Override public void onExitApp() { finishAffinity(); }
    @Override public void onCheckUpdate() { checkUpdateExposed(); }
    @Override public void onDeviceDiagnostics() { JcDeviceDiagnostics.show(this); }
    @Override public void onServerUrlEdit() { promptServerUrl(); }
    @Override public String getServerUrl() {
        return prefs().getString(KEY_SERVER_URL, "");
    }

    // ---- 账号(web 特有: 经 window.__jcAuth 读写 web 登录态) ----
    @Override public void onAccountAction() {
        if (webViewRef == null) return;
        if (accountLoggedIn) {
            evalJs("window.__jcAuth && window.__jcAuth.logout();");
            GlassToast.show(this, "已退出登录");
            new Handler(Looper.getMainLooper()).postDelayed(this::refreshAccountRow, 800);
        } else {
            evalJs("window.__jcAuth && window.__jcAuth.login();");
            hideSettingsDrawer();
        }
    }
    @Override public JcSettingsDrawer.AccountInfo getAccountInfo() {
        // 文案拼装一律走 core 的工厂(loggedIn/loggedOut) —— 两壳账号行必须同源,
        // 否则同一个抽屉在两种模式下显示不同的字。
        return accountLoggedIn
                ? JcSettingsDrawer.AccountInfo.loggedIn(currentAccountName)
                : JcSettingsDrawer.AccountInfo.loggedOut();
    }

    // ---- 壳类型 ----
    @Override public boolean isWebShell() { return true; }

    // ---- 显示模式(web 特有; 原生壳 hasDisplayMode=false 隐藏此行) ----
    @Override public boolean hasDisplayMode() { return true; }
    @Override public void onCycleDisplayMode() { cycleDisplayMode(); }
    @Override public String getDisplayModeLabel() {
        return SettingsDrawerLogic.displayModeLabel(currentDisplayMode);
    }

    // ---- 减少动效(原生壳特有; web 壳不显示) ----
    @Override public boolean hasMotionRow() { return false; }
    @Override public void onCycleMotion() { }
    @Override public String getMotionLabel() { return ""; }

    // ---- 壳模式(通用) ----
    @Override public void onShellModeChanged(String mode) {
        // 说明后果 + 询问是否立即重启 —— 弹窗与文案都在 core(两壳同一份)
        JcSettingsDrawer.showShellModeSwitchDialog(this, mode);
    }
    @Override public String getCurrentShellMode() {
        return prefs().getString(KEY_SHELL_MODE, "auto");
    }

    // ---- Web 播放设置(经 window.__jcConfig 读写 web localStorage; 契约见 WebSettingsContract) ----

    @Override public void getWebSettings(JcSettingsDrawer.WebSettingsCallback cb) {
        evalJs("(function(){try{return window.__jcConfig?window.__jcConfig.getAll():null}"
                + "catch(e){return null}})();",
                value -> cb.onResult(unwrapJsString(value)));
    }

    @Override public void setWebSetting(String key, String value, JcSettingsDrawer.WebSettingsCallback cb) {
        // key/值仅来自本抽屉的白名单调用方, 再做一层字符白名单防注入
        String k = key == null ? "" : key.replaceAll("[^A-Za-z]", "");
        String v = value == null ? "" : value.replaceAll("[^A-Za-z0-9]", "");
        evalJs("(function(){try{if(!window.__jcConfig)return null;"
                        + "window.__jcConfig.set('" + k + "','" + v + "');"
                        + "return window.__jcConfig.getAll()}catch(e){return null}})();",
                r -> cb.onResult(unwrapJsString(r)));
    }

    /** evaluateJavascript 对字符串回参会包一层 JSON 引号; null 结果回 "null"。解回原始字符串。 */
    private static String unwrapJsString(String raw) {
        if (raw == null) return null;
        String s = raw.trim();
        if ("null".equals(s)) return null;
        if (s.length() >= 2 && s.startsWith("\"") && s.endsWith("\"")) {
            s = s.substring(1, s.length() - 1)
                    .replace("\\\"", "\"")
                    .replace("\\\\", "\\");
        }
        return s;
    }

    /** 显示模式三态循环: TV → 桌面 → 自动(清除) → TV…; 经 __jcSetMode 钩子即时生效 */
    private void cycleDisplayMode() {
        String next = SettingsDrawerLogic.nextDisplayMode(currentDisplayMode);
        applyViewMode(next);
        GlassToast.show(this, SettingsDrawerLogic.displayModeToast(next));
    }

    /**
     * 读一次 web 登录态(经 window.__jcAuth.user()), 刷新「账号」行文案。
     * __jcAuth 只在原生壳里安装且 user() 返回 JSON 字符串 ⇒ 让 JS 直接 parse 成对象
     * 回传(evaluateJavascript 对对象回 JSON 文本, 对字符串会再包一层引号, 少一层解码)。
     */
    private void refreshAccountRow() {
        if (webViewRef == null) return;
        evalJs("(function(){try{return window.__jcAuth"
                + "?JSON.parse(window.__jcAuth.user()):null}catch(e){return null}})();",
                value -> {
                    SettingsDrawerLogic.AccountSnapshot snap =
                            SettingsDrawerLogic.parseAccountSnapshot(value);
                    accountLoggedIn = snap.loggedIn;
                    currentAccountName = snap.name == null ? "" : snap.name;
                    runOnUiThread(() -> {
                        if (settingsDrawer == null) return;
                        settingsDrawer.refreshAccount(accountLoggedIn
                                ? JcSettingsDrawer.AccountInfo.loggedIn(currentAccountName)
                                : JcSettingsDrawer.AccountInfo.loggedOut());
                    });
                });
    }

    /** evaluateJavascript 的空指针安全包装 */
    private void evalJs(String script) {
        evalJs(script, null);
    }

    private void evalJs(String script, ValueCallback<String> cb) {
        if (webViewRef == null) return;
        webViewRef.evaluateJavascript(script, cb);
    }

    /**
     * 读一次 WebView localStorage 里的显示模式覆盖值(jc-native-mode), 刷新"显示模式"行右侧状态。
     * 键名须与前端 useViewMode.NATIVE_MODE_KEY 一致 —— 只有抽屉(applyViewMode)会写它;
     * 空 ⇒ null ⇒ 文案"自动"(即"没选过", 壳内按默认强制 TV)。
     */
    private void refreshDisplayModeValue() {
        if (webViewRef == null) return;
        webViewRef.evaluateJavascript(
                "(function(){try{return localStorage.getItem('jc-native-mode')||''}"
                        + "catch(e){return ''}})();",
                value -> {
                    String raw = value == null ? "" : value.replaceAll("^\"|\"$", "");
                    if ("null".equals(raw)) raw = "";
                    final String mode = raw;
                    currentDisplayMode = mode.isEmpty() ? null : mode;
                    runOnUiThread(() -> {
                        if (settingsDrawer != null) {
                            settingsDrawer.refreshDisplayMode(
                                    SettingsDrawerLogic.displayModeLabel(mode));
                        }
                    });
                });
    }

    /** 抽屉行/按钮背景已下沉 core 的 JcUi.glassButtonBg(两份逐行合并, 值不变)。 */

    /**
     * 应用显示模式(tv / desktop / null=自动): 交前端 useViewMode 的 window.__jcSetMode 钩子
     * (前端 installNativeModeHook 暴露, 先例 window.gfTvBack)。前端只切换自身的 data-mode,
     * **不 reload** —— 切完页面即时重排(以前写 localStorage 再 location.reload 的做法在壳内
     * 根本不生效: 壳内 detectMode() 刻意忽略 jc-mode, 见前端 NATIVE_MODE_KEY 注释)。
     */
    private void applyViewMode(String mode) {
        if (webViewRef == null) return;
        final String arg = SettingsDrawerLogic.jsModeArg(mode);
        webViewRef.evaluateJavascript(
                "(function(){try{return !!(window.__jcSetMode&&window.__jcSetMode('" + arg + "'))}"
                        + "catch(e){return false}})();",
                null);
        hideSettingsDrawer();
    }

    /** 打开原生设置抽屉 — 供 MENU 键(dispatchKeyEvent) 与 JerocineBridge 的
     *  openSettings(web 胶囊行"设置"按钮 / 首页"我的"卡) 共用; 命名与
     *  checkUpdateExposed / promptServerUrlExposed 保持一致。 */
    void showSettingsDrawerExposed() {
        if (settingsDrawer == null) {
            settingsDrawer = new JcSettingsDrawer(this, prefs(), this);
        }
        // 抽屉打开时禁掉 WebView 的可聚焦, 不然 D-pad 事件被 WebView 抢走
        if (webViewRef != null) {
            webViewRef.setFocusable(false);
            webViewRef.setFocusableInTouchMode(false);
        }
        settingsDrawer.show();
        // web 特有: 账号登录态和显示模式存在 web 侧, 打开抽屉时异步同步一次
        refreshDisplayModeValue();
        refreshAccountRow();
    }

    private void hideSettingsDrawer() {
        if (settingsDrawer == null || !settingsDrawer.isShowing()) return;
        // 恢复 WebView 焦点能力
        if (webViewRef != null) {
            webViewRef.setFocusable(true);
            webViewRef.setFocusableInTouchMode(true);
            webViewRef.requestFocus();
        }
        settingsDrawer.hide();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        // 菜单键: 弹出 native 设置侧边栏 (不再转发给 web — 设置应该是壳层能力,
        // 后续要改菜单条目直接改 APK, 不依赖 web 是否就绪)
        if (event.getKeyCode() == KeyEvent.KEYCODE_MENU
                && event.getAction() == KeyEvent.ACTION_DOWN) {
            if (settingsDrawer != null && settingsDrawer.isShowing()) hideSettingsDrawer();
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
        if (settingsDrawer != null && settingsDrawer.isShowing()) {
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
                            if ("true".equals(value)) {
                                // 前端消费了返回 = 路由栈变化 → 对齐原生壳"backstack 变化 reset"语义
                                // (NativeShellActivity 的 addOnBackStackChangedListener): 取消上一次
                                // "再按一次退出"的未决窗口, 避免在页间往返后回到首页的第一次 BACK
                                // 还落在旧窗口里直接退出。
                                exitGate.reset();
                                return;
                            }
                            // 前端没消费 = 已在首页 → 双击返回退出应用 (与遥控器返回一致)
                            runOnUiThread(() -> {
                                if (exitGate.shouldExit(SystemClock.elapsedRealtime())) {
                                    finishAffinity(); // 真正退出应用
                                } else {
                                    GlassToast.show(WebShellActivity.this, "再按一次退出应用");
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
     *
     * <p>现代写法: OnBackPressedDispatcher 替代已废弃的 onBackPressed 重写
     * (Android 13+ 预测性返回手势要求; 2026-10-15 lint GestureBackNavigation)。在 onCreate 注册。
     */
    private void registerBackHandler() {
        getOnBackPressedDispatcher().addCallback(this, new androidx.activity.OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                if (!handleBackPressed()) {
                    // 无 WebView 历史可退 → 放行系统默认(退出)
                    setEnabled(false);
                    getOnBackPressedDispatcher().onBackPressed();
                    setEnabled(true);
                }
            }
        });
    }
}
