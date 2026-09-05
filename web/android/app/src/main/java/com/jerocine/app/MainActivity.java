package com.jerocine.app;

import android.app.AlertDialog;
import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.StateListDrawable;
import android.net.ConnectivityManager;
import android.net.NetworkInfo;
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
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import com.getcapacitor.BridgeActivity;

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

    private long lastExitBackAt = 0L;
    private UpdateChecker updateChecker;
    private ProgressBar bootProgress;
    /** package-private — JerocineBridge.handleEchoTest 需要直接拿到 WebView 发反向事件 */
    WebView webViewRef;
    private FrameLayout settingsOverlay;
    private ScrollView settingsPanelScroll;
    private LinearLayout settingsPanel;
    private TextView settingsServerLine;
    private boolean settingsOpen = false;
    /** 断网兜底页 (原生全屏覆盖, 带重试按钮) */
    private FrameLayout offlineOverlay;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
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

        // PlayerActivity 事件 → 通过 bridge 转发给 web
        PlayerActivity.setEventListener((name, payload) -> {
            JerocineBridge.sendEvent(webViewRef, name, payload);
        });

        String url = prefs().getString(KEY_SERVER_URL, null);
        if (url == null || url.isEmpty()) {
            // 首次启动: 用默认地址, 写回 SharedPreferences, 直接 loadUrl, 不弹框
            url = DEFAULT_SERVER_URL;
            prefs().edit().putString(KEY_SERVER_URL, url).apply();
        }
        loadServer(url);

        // 自升级: 启动 5s 后异步检查 (避开首屏并行 IO)
        updateChecker = new UpdateChecker(this);
        new Handler(Looper.getMainLooper()).postDelayed(
                () -> updateChecker.check(false), 5000);
    }

    /** 加一个全屏覆盖、居中显示的 ProgressBar 作为启动加载动画 */
    private void addBootProgress() {
        bootProgress = new ProgressBar(this);
        bootProgress.setIndeterminate(true);
        bootProgress.setIndeterminateTintList(android.content.res.ColorStateList.valueOf(GF_ACCENT));
        FrameLayout root = findViewById(android.R.id.content);
        if (root != null) {
            FrameLayout.LayoutParams lp = new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            lp.gravity = Gravity.CENTER;
            root.addView(bootProgress, lp);
        }
    }
    private void hideBootProgress() {
        if (bootProgress != null) bootProgress.setVisibility(View.GONE);
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
        // PlayerActivity 报错后回到主界面: 通知前端 fallback (关 adFilter 重播)
        if (PlayerActivity.sLastPlaybackFailed) {
            PlayerActivity.sLastPlaybackFailed = false;
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

        AlertDialog dlg = new AlertDialog.Builder(this, R.style.GfPlayerDialog)
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

        // 右侧抽屉外壳 (ScrollView 包裹 — 按钮多, 在小屏 TV 可能撑出去, 必须能滚)
        // 玻璃面 (半透明深色) + 左侧细描边, 营造贴边玻璃抽屉感.
        settingsPanelScroll = new ScrollView(this);
        GradientDrawable drawerBg = new GradientDrawable();
        drawerBg.setColor(GF_GLASS);
        drawerBg.setStroke(dp(1), GF_STROKE);
        settingsPanelScroll.setBackground(drawerBg);
        settingsPanelScroll.setClickable(true); // 拦截到自己上的点击, 别冒泡给遮罩
        FrameLayout.LayoutParams panelLp = new FrameLayout.LayoutParams(
                dp(360), ViewGroup.LayoutParams.MATCH_PARENT, Gravity.END);
        settingsOverlay.addView(settingsPanelScroll, panelLp);

        // 真正的按钮容器
        settingsPanel = new LinearLayout(this);
        settingsPanel.setOrientation(LinearLayout.VERTICAL);
        int pad = dp(24);
        settingsPanel.setPadding(pad, pad, pad, pad);
        ScrollView.LayoutParams panelInner = new ScrollView.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT);
        settingsPanelScroll.addView(settingsPanel, panelInner);

        // 标题
        TextView title = new TextView(this);
        title.setText("设置");
        title.setTextColor(GF_TEXT);
        title.setTextSize(TypedValue.COMPLEX_UNIT_SP, 22);
        LinearLayout.LayoutParams titleLp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        titleLp.bottomMargin = dp(16);
        settingsPanel.addView(title, titleLp);

        // 版本号
        TextView ver = new TextView(this);
        ver.setText("版本: " + BuildConfig.VERSION_NAME + " (" + BuildConfig.VERSION_CODE + ")");
        ver.setTextColor(GF_TEXT_TER);
        ver.setTextSize(TypedValue.COMPLEX_UNIT_SP, 14);
        settingsPanel.addView(ver);

        // 服务器地址行
        settingsServerLine = new TextView(this);
        settingsServerLine.setTextColor(GF_TEXT_TER);
        settingsServerLine.setTextSize(TypedValue.COMPLEX_UNIT_SP, 14);
        LinearLayout.LayoutParams svrLp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        svrLp.topMargin = dp(4);
        svrLp.bottomMargin = dp(20);
        settingsPanel.addView(settingsServerLine, svrLp);

        // 按钮区 — "刷新页面" 是最高频操作, 放最前; "退出/关闭" 放最后
        settingsPanel.addView(makeDrawerButton("刷新页面", v -> {
            hideSettingsDrawer();
            if (webViewRef != null) webViewRef.reload();
        }));
        settingsPanel.addView(makeDrawerButton("诊断信息 (toast)", v -> showDiagToast()));
        settingsPanel.addView(makeDrawerButton("修改服务器地址", v -> {
            hideSettingsDrawer();
            promptServerUrl(true);
        }));
        settingsPanel.addView(makeDrawerButton("检查更新", v -> {
            hideSettingsDrawer();
            checkUpdateExposed();
        }));
        settingsPanel.addView(makeDrawerButton("强制 TV 模式 (持久化)", v -> {
            persistViewMode("tv");
            GlassToast.show(this, "已设为 TV 模式");
        }));
        settingsPanel.addView(makeDrawerButton("切桌面模式 (用于调试)", v -> {
            persistViewMode("desktop");
            GlassToast.show(this, "已设为 desktop 模式");
        }));
        settingsPanel.addView(makeDrawerButton("清除模式 (自动检测)", v -> {
            persistViewMode(null);
            GlassToast.show(this, "已清除模式");
        }));
        settingsPanel.addView(makeDrawerButton("退出应用", v -> finishAffinity()));
        settingsPanel.addView(makeDrawerButton("关闭", v -> hideSettingsDrawer()));
    }

    private Button makeDrawerButton(String label, View.OnClickListener click) {
        Button b = new Button(this);
        b.setText(label);
        b.setTextColor(GF_TEXT);
        b.setTextSize(TypedValue.COMPLEX_UNIT_SP, 15);
        // 关键: ColorDrawable 是单状态, D-pad 移焦点没视觉反馈, 用户以为 D-pad 不通.
        // 玻璃风 StateListDrawable: focused (强调青描边 + 半透明青底) / pressed / 默认 (玻璃面).
        b.setBackground(buildDrawerButtonBg());
        b.setAllCaps(false);
        b.setFocusable(true);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, dp(52));
        lp.topMargin = dp(10);
        b.setLayoutParams(lp);
        b.setOnClickListener(click);
        return b;
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

    /** 在 WebView 内 localStorage 写 gf-mode, 然后 reload 让 useViewMode 生效 */
    private void persistViewMode(String mode) {
        if (webViewRef == null) return;
        final String js;
        if (mode == null) {
            js = "try{localStorage.removeItem('gf-mode')}catch(e){};location.reload();";
        } else {
            js = "try{localStorage.setItem('gf-mode','" + mode + "')}catch(e){};location.reload();";
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
                        new AlertDialog.Builder(this, R.style.GfPlayerDialog)
                                .setTitle("诊断")
                                .setMessage(msg)
                                .setPositiveButton("OK", null)
                                .show();
                    });
                }
        );
    }

    private void showSettingsDrawer() {
        if (settingsOverlay == null) buildSettingsDrawer();
        if (settingsOverlay == null) return;
        // 更新当前服务器地址显示
        if (settingsServerLine != null) {
            settingsServerLine.setText("服务器: " + prefs().getString(KEY_SERVER_URL, ""));
        }
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
            float w = settingsPanelScroll.getWidth() > 0 ? settingsPanelScroll.getWidth() : dp(360);
            settingsPanelScroll.setTranslationX(w);
            settingsPanelScroll.animate().translationX(0f).setDuration(220).start();
            // 焦点定到第一个按钮 (面板第 4 个 child: title + ver + svr + 第一个按钮)
            if (settingsPanel.getChildCount() > 3) {
                View firstBtn = settingsPanel.getChildAt(3);
                firstBtn.requestFocus();
            }
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
            else showSettingsDrawer();
            return true;
        }
        if (event.getKeyCode() == KeyEvent.KEYCODE_BACK
                && event.getAction() == KeyEvent.ACTION_DOWN) {
            // 抽屉打开时, BACK 仅关抽屉
            if (settingsOpen) {
                hideSettingsDrawer();
                return true;
            }
            // 转给前端 router (window.gfTvBack); 首页(前端不消费)则双击返回退出。
            // 注: "改服务器地址"已在菜单(MENU 抽屉)里, 不再用"连按4次返回"触发重置弹窗。
            WebView webView = (bridge != null) ? bridge.getWebView() : null;
            if (webView != null) {
                webView.evaluateJavascript(
                        "(function(){try{return !!(window.gfTvBack&&window.gfTvBack());}catch(e){return false;}})();",
                        new ValueCallback<String>() {
                            @Override
                            public void onReceiveValue(String value) {
                                if ("true".equals(value)) return;
                                // 前端没消费 = 已在首页 → 双击返回退出应用
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
        }
        return super.dispatchKeyEvent(event);
    }
}
