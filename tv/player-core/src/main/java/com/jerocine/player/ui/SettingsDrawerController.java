package com.jerocine.player.ui;

import android.app.Activity;
import android.content.Context;
import android.content.SharedPreferences;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.StateListDrawable;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import com.jerocine.player.PlayerAdFilterHelper;
import com.jerocine.player.R;
import com.jerocine.player.data.ShellPrefs;

/**
 * 统一设置抽屉控制器 —— 单包双壳**共用一份**（下沉到 player-core，细则 §5.1 / §6.3）。
 *
 * 合并前 web 壳在 WebShellActivity 里内联约 450 行构建、原生壳是全页 SettingsFragment；
 * 现在两壳都在 onCreate 里 new 出本控制器，外观与交互完全一致
 * （§10 验收项"设置页在两种模式下同一份抽屉、外观一致"）。
 *
 * 设计原则: 通用项(广告过滤/设备诊断/服务器/版本/检查更新/壳模式/退出)直接处理;
 * 壳特有项(刷新页面/账号登录态/显示模式/减少动效)通过 {@link Host} 回调交给各壳实现:
 * web 壳有"显示模式"无"减少动效", 原生壳相反 —— 由 host.hasDisplayMode()/hasMotionRow()
 * 各自隐藏对方那一行。
 *
 * 用法:
 *   controller = new SettingsDrawerController(activity, prefs, host);
 *   controller.build();          // onCreate 时构建(懒挂载到 android.R.id.content)
 *   controller.show();           // 菜单键/按钮触发
 *   controller.hide();           // 外部点击/返回键
 *   controller.refreshAccount(); // 登录态变化时刷新账号行
 *   controller.refreshDisplayMode(); // web 壳显示模式变化时刷新
 */
public class SettingsDrawerController {

    // ==================== 设计令牌(与 WebShellActivity 原常量一致) ====================
    static final int GF_GLASS       = 0xCC15151C;
    static final int GF_GLASS_SOFT  = 0xB31E1E28;
    static final int GF_SCRIM       = 0x99000000;
    static final int GF_STROKE      = 0x22FFFFFF;
    static final int GF_TEXT        = 0xFFFFFFFF;
    static final int GF_TEXT_SEC    = 0xB3FFFFFF;
    static final int GF_TEXT_TER    = 0x80FFFFFF;
    static final int GF_ACCENT      = 0xFF4AD1E5;
    static final int GF_ACCENT_DIM  = 0x334AD1E5;
    static final int GF_ACCENT_PRESS= 0x554AD1E5;
    static final int GF_KNOB        = 0xFFFFFFFF;
    static final int DRAWER_W_DP = 400;
    static final int ROW_H_DP = 56;
    /**
     * 壳模式存储键 —— 契约只在 {@link ShellPrefs} 定义一次(与 EntryRouter/DeviceProbe 共用),
     * 这里保留常量只为兼容既有引用。
     */
    public static final String KEY_SHELL_MODE = ShellPrefs.KEY_SHELL_MODE;

    /** 壳特有操作回调 —— web 壳和原生壳各自实现 */
    public interface Host {
        /** 轻提示 —— 两壳统一走 core 的 {@link GlassToast}(玻璃风格, TV 上观感一致) */
        void toast(String msg);
        /** 刷新页面(web: webView.reload; 原生: 刷新数据) */
        void onRefreshPage();
        /** 退出应用 */
        void onExitApp();
        /** 检查更新 */
        void onCheckUpdate();
        /** 设备诊断 */
        void onDeviceDiagnostics();
        /** 编辑服务器地址 */
        void onServerUrlEdit();
        /** 当前服务器地址 */
        String getServerUrl();

        // ---- 账号 ----
        /** 账号行点击(登录/退出) */
        void onAccountAction();
        /** 账号行信息(标题/副标题/按钮文案/是否登录) */
        AccountInfo getAccountInfo();

        // ---- 显示模式(web 特有; 原生 hasDisplayMode=false 隐藏) ----
        /** 是否显示"显示模式"行(原生壳返回 false) */
        boolean hasDisplayMode();
        /** 循环切换显示模式 */
        void onCycleDisplayMode();
        /** 当前显示模式标签 */
        String getDisplayModeLabel();

        // ---- 减少动效(原生壳特有; web 壳 hasMotionRow=false 隐藏) ----
        /**
         * 是否显示"减少动效"行。
         * 这是原生壳**唯一**的动画开关入口 —— 抽屉化之前它在全页 SettingsFragment 里,
         * 抽屉必须保留, 否则用户再也改不了(老设备去抖动全靠它)。
         */
        boolean hasMotionRow();
        /** 循环切换动效档位(auto → on → off) */
        void onCycleMotion();
        /** 当前动效档位标签(自动 / 减少 / 完整) */
        String getMotionLabel();

        // ---- 壳模式(通用) ----
        /** 壳模式变更(已写入 prefs; host 可提示重启) */
        void onShellModeChanged(String mode);
        /** 当前壳模式(auto/web/native) */
        String getCurrentShellMode();
    }

    /** 账号行快照 */
    public static class AccountInfo {
        public final String title;
        public final String subtitle;
        public final String actionText;
        public final boolean loggedIn;
        public AccountInfo(String title, String subtitle, String actionText, boolean loggedIn) {
            this.title = title; this.subtitle = subtitle;
            this.actionText = actionText; this.loggedIn = loggedIn;
        }
        public static AccountInfo loggedOut() {
            return new AccountInfo("未登录", "登录后收藏 / 历史 / 跳过设置多端同步", "登录", false);
        }
    }

    private final Activity activity;
    private final SharedPreferences prefs;
    private final Host host;

    private FrameLayout settingsOverlay;
    private ScrollView settingsPanelScroll;
    private LinearLayout settingsPanel;
    private View settingsFirstFocus;
    private FrameLayout adFilterSwitch;
    private TextView displayModeValue;
    private TextView motionValue;
    private TextView shellModeValue;
    private TextView accountAction;
    private TextView accountTitleView;
    private String currentShellMode;
    /**
     * 抽屉"逻辑打开"标志 —— 与 overlay 的 visibility 不同步:
     * hide() 有 180ms 滑出动画, 期间 overlay 仍是 VISIBLE。若用 visibility 当状态,
     * 动画窗口内再按 MENU 会被判成"已打开"从而只触发 hide(用户按键像被吃掉);
     * 用独立标志可精确复刻抽屉化之前的 settingsOpen 语义(原 web 壳正是这么做的)。
     */
    private boolean open = false;

    public SettingsDrawerController(Activity activity, SharedPreferences prefs, Host host) {
        this.activity = activity;
        this.prefs = prefs;
        this.host = host;
    }

    // ==================== 构建 ====================

    /** 构建抽屉 UI 并挂载到 activity 的 android.R.id.content(幂等; 重复调用跳过) */
    public void build() {
        if (settingsOverlay != null) return;
        FrameLayout root = activity.findViewById(android.R.id.content);
        if (root == null) return;

        settingsOverlay = new FrameLayout(activity);
        settingsOverlay.setBackgroundColor(GF_SCRIM);
        settingsOverlay.setVisibility(View.GONE);
        settingsOverlay.setClickable(true);
        settingsOverlay.setOnClickListener(v -> hide());
        root.addView(settingsOverlay, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        settingsPanelScroll = new ScrollView(activity);
        GradientDrawable drawerBg = new GradientDrawable();
        drawerBg.setColor(GF_GLASS);
        drawerBg.setStroke(dp(1), GF_STROKE);
        settingsPanelScroll.setBackground(drawerBg);
        settingsPanelScroll.setClickable(true);
        FrameLayout.LayoutParams panelLp = new FrameLayout.LayoutParams(
                dp(DRAWER_W_DP), ViewGroup.LayoutParams.MATCH_PARENT, Gravity.END);
        settingsOverlay.addView(settingsPanelScroll, panelLp);

        settingsPanel = new LinearLayout(activity);
        settingsPanel.setOrientation(LinearLayout.VERTICAL);
        int pad = dp(20);
        settingsPanel.setPadding(pad, pad, pad, dp(28));
        settingsPanelScroll.addView(settingsPanel, new ScrollView.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        fillPanel();
    }

    /** 抽屉内容: 头部 → 刷新页面 → 账号 → 播放 → 设备 → 关于 → 系统 */
    private void fillPanel() {
        settingsPanel.removeAllViews();
        settingsFirstFocus = null;
        adFilterSwitch = null;
        displayModeValue = null;
        motionValue = null;
        shellModeValue = null;
        accountAction = null;
        accountTitleView = null;

        // ---------- 头部: 标题 + 版本 + 关闭 ----------
        LinearLayout head = new LinearLayout(activity);
        head.setOrientation(LinearLayout.HORIZONTAL);
        head.setGravity(Gravity.CENTER_VERTICAL);
        head.addView(makeText("设置", 20, GF_TEXT, true), new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f));
        head.addView(makeText(installedVersionText(), 12, GF_TEXT_TER, false));
        ImageView closeBtn = new ImageView(activity);
        closeBtn.setImageResource(R.drawable.ic_close);
        closeBtn.setColorFilter(GF_TEXT_SEC);
        closeBtn.setContentDescription("关闭");
        closeBtn.setPadding(dp(4), dp(4), dp(4), dp(4));
        closeBtn.setFocusable(true);
        closeBtn.setClickable(true);
        closeBtn.setOnClickListener(v -> hide());
        head.addView(closeBtn, new LinearLayout.LayoutParams(dp(24), dp(24)));
        settingsPanel.addView(head, rowParams(ViewGroup.LayoutParams.WRAP_CONTENT, 8));

        // ---------- 置顶: 刷新页面 ----------
        addNavRow("刷新页面", "重新加载当前站点", "", () -> {
            host.onRefreshPage();
            hide();
        });

        // ---------- 账号 ----------
        addGroupHeader("账号");
        AccountInfo acct = host.getAccountInfo();
        LinearLayout accountRow = makeRowBase(
                acct.title, acct.subtitle, true, v -> host.onAccountAction());
        accountTitleView = (TextView) ((LinearLayout) accountRow.getChildAt(0)).getChildAt(0);
        accountAction = makeText(acct.actionText, 13, GF_ACCENT, false);
        addRowTail(accountRow, accountAction);
        settingsPanel.addView(accountRow, rowParams(dp(ROW_H_DP), 6));

        // ---------- 播放 ----------
        addGroupHeader("播放");
        addToggleRow("广告过滤",
                "剔除 m3u8 插片/跨域广告 · 默认开启",
                PlayerAdFilterHelper.isAdFilterEnabled(activity),
                sw -> adFilterSwitch = sw,
                () -> {
                    boolean next = !PlayerAdFilterHelper.isAdFilterEnabled(activity);
                    PlayerAdFilterHelper.setAdFilterEnabled(activity, next);
                    refreshToggle(adFilterSwitch, next);
                    host.toast(next ? "广告过滤已开启" : "广告过滤已关闭");
                });

        // ---------- 设备 ----------
        addGroupHeader("设备");
        addNavRow("设备诊断", "机型 / 系统 / WebView 内核 / 内存", "",
                () -> { hide(); host.onDeviceDiagnostics(); });
        addNavRow("服务器地址", host.getServerUrl(), "修改 ›",
                () -> { hide(); host.onServerUrlEdit(); });

        // ---------- 关于 ----------
        addGroupHeader("关于");
        addInfoRow("运行平台", "Android " + android.os.Build.VERSION.RELEASE
                + " (API " + android.os.Build.VERSION.SDK_INT + ")");
        addInfoRow("构建模式", "APK 构建类型 · 与线上包体无关",
                isDebuggableBuild() ? "debug" : "release");
        addNavRow("检查更新", "GET /app/version/latest", "立即检查",
                () -> { hide(); host.onCheckUpdate(); });

        // ---------- 系统 ----------
        addGroupHeader("系统");
        currentShellMode = host.getCurrentShellMode();
        shellModeValue = addNavRow("壳模式", "自动 / Web / 原生（重启生效）",
                shellModeLabel(currentShellMode), this::cycleShellMode);
        if (host.hasMotionRow()) {
            motionValue = addNavRow("减少动效", "自动 / 减少 / 完整（老设备去抖动）",
                    host.getMotionLabel(), () -> {
                        host.onCycleMotion();
                        // 档位在原生壳里是同步落盘的, 直接回读刷新右侧文案
                        if (motionValue != null) motionValue.setText(host.getMotionLabel());
                    });
        }
        if (host.hasDisplayMode()) {
            displayModeValue = addNavRow("显示模式", "TV → 桌面 → 自动（壳内默认 TV）",
                    host.getDisplayModeLabel(), host::onCycleDisplayMode);
        }
        addNavRow("退出应用", "", "", () -> {
            hide();
            host.onExitApp();
        });
    }

    // ==================== 显示/隐藏 ====================

    public void show() {
        if (settingsOverlay == null) build();
        if (settingsOverlay == null) return;
        fillPanel(); // 每次打开重刷(账号态/服务器地址可能变)
        open = true;
        settingsOverlay.setVisibility(View.VISIBLE);
        settingsOverlay.setFocusableInTouchMode(true);
        settingsOverlay.setFocusable(true);
        settingsPanelScroll.scrollTo(0, 0);
        // 滑入动画: 从右侧滑入
        settingsPanelScroll.post(() -> {
            float w = settingsPanelScroll.getWidth() > 0
                    ? settingsPanelScroll.getWidth() : dp(DRAWER_W_DP);
            settingsPanelScroll.setTranslationX(w);
            settingsPanelScroll.animate().translationX(0f).setDuration(220).start();
            if (settingsFirstFocus != null) settingsFirstFocus.requestFocus();
        });
    }

    public void hide() {
        if (!open) return;
        open = false;
        if (settingsOverlay == null) return;
        // 滑出动画: 向右滑出后隐藏
        float w = settingsPanelScroll.getWidth() > 0
                ? settingsPanelScroll.getWidth() : dp(DRAWER_W_DP);
        settingsPanelScroll.animate().translationX(w).setDuration(180).withEndAction(
                () -> { if (settingsOverlay != null) settingsOverlay.setVisibility(View.GONE); }
        ).start();
    }

    /** 逻辑打开态(滑出动画期间即为 false) —— 供 MENU/BACK 切换判定 */
    public boolean isShowing() {
        return open;
    }

    /** 登录态变化时刷新账号行(web 壳异步读 web 登录态后调用) */
    public void refreshAccount(AccountInfo info) {
        if (accountTitleView != null) accountTitleView.setText(info.title);
        if (accountAction != null) accountAction.setText(info.actionText);
    }

    /** 显示模式变化时刷新行右侧标签(web 壳) */
    public void refreshDisplayMode(String label) {
        if (displayModeValue != null) displayModeValue.setText(label);
    }

    // ==================== 壳模式 ====================

    private void cycleShellMode() {
        String next = nextShellMode(currentShellMode);
        prefs.edit().putString(KEY_SHELL_MODE, next).apply();
        currentShellMode = next;
        if (shellModeValue != null) shellModeValue.setText(shellModeLabel(next));
        host.onShellModeChanged(next);
    }

    static String nextShellMode(String current) {
        if ("web".equals(current)) return "native";
        if ("native".equals(current)) return "auto";
        return "web";
    }

    public static String shellModeLabel(String mode) {
        if ("web".equals(mode)) return "Web";
        if ("native".equals(mode)) return "原生";
        return "自动";
    }

    // ==================== UI 辅助方法(从 WebShellActivity 抽取) ====================

    private int dp(int v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v,
                activity.getResources().getDisplayMetrics());
    }

    /**
     * 头部版本号文案。
     *
     * ⚠️ 必须读**已安装包**的真实版本: 单包合并后唯一 APK 的版本来自宿主 :app(web.*),
     *    而本类在 player-core 库里 —— 库既没有 applicationId 也没有 versionCode/versionName，
     *    靠 BuildConfig 兜底只会显示空串或错号（用户报障时给的信息就是错的）。故只走 PackageManager。
     */
    @SuppressWarnings("deprecation") // getPackageInfo(String,int) 在 API 33 被标记过时, 但全版本可用
    private String installedVersionText() {
        try {
            android.content.pm.PackageInfo pi = activity.getPackageManager()
                    .getPackageInfo(activity.getPackageName(), 0);
            return "v" + pi.versionName + " (" + pi.versionCode + ")";
        } catch (Exception e) {
            return "";
        }
    }

    /** 是否 debug 包 —— 读已安装包的 debuggable 标志(库不自带 BuildConfig 版本语义) */
    private boolean isDebuggableBuild() {
        return (activity.getApplicationInfo().flags
                & android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE) != 0;
    }

    private TextView makeText(String text, float sp, int color, boolean bold) {
        TextView t = new TextView(activity);
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

    private void addGroupHeader(String label) {
        LinearLayout wrap = new LinearLayout(activity);
        wrap.setOrientation(LinearLayout.HORIZONTAL);
        wrap.setGravity(Gravity.BOTTOM);

        View bar = new View(activity);
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

    private LinearLayout makeRowBase(String title, String sub, boolean focusable,
                                     View.OnClickListener click) {
        LinearLayout row = new LinearLayout(activity);
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

        LinearLayout col = new LinearLayout(activity);
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

    private void addRowTail(LinearLayout row, View tail) {
        tail.setFocusable(false);
        tail.setClickable(false);
        tail.setFocusableInTouchMode(false);
        final LinearLayout.LayoutParams lp;
        if (tail.getLayoutParams() instanceof LinearLayout.LayoutParams) {
            lp = (LinearLayout.LayoutParams) tail.getLayoutParams();
        } else {
            lp = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        }
        lp.leftMargin = dp(12);
        row.addView(tail, lp);
    }

    private void addToggleRow(String title, String sub, boolean on,
                              java.util.function.Consumer<FrameLayout> switchOut,
                              Runnable onToggle) {
        LinearLayout row = makeRowBase(title, sub, true, v -> onToggle.run());
        FrameLayout sw = buildToggleSwitch(on);
        if (switchOut != null) switchOut.accept(sw);
        addRowTail(row, sw);
        settingsPanel.addView(row, rowParams(dp(ROW_H_DP), 6));
    }

    private TextView addNavRow(String title, String sub, String value, Runnable action) {
        LinearLayout row = makeRowBase(title, sub, true, v -> action.run());
        TextView v = makeText(value == null ? "" : value, 13, GF_ACCENT, false);
        addRowTail(row, v);
        settingsPanel.addView(row, rowParams(dp(ROW_H_DP), 6));
        return v;
    }

    private void addInfoRow(String title, String value) {
        addInfoRow(title, null, value);
    }

    private void addInfoRow(String title, String sub, String value) {
        LinearLayout row = makeRowBase(title, sub, false, null);
        TextView v = makeText(value == null ? "" : value, 13, GF_TEXT_SEC, false);
        addRowTail(row, v);
        settingsPanel.addView(row, rowParams(dp(ROW_H_DP), 6));
    }

    private FrameLayout buildToggleSwitch(boolean on) {
        FrameLayout sw = new FrameLayout(activity);
        sw.setLayoutParams(new LinearLayout.LayoutParams(dp(44), dp(24)));
        applyToggleLook(sw, on);
        View knob = new View(activity);
        GradientDrawable knobBg = new GradientDrawable();
        knobBg.setShape(GradientDrawable.OVAL);
        knobBg.setColor(GF_KNOB);
        knob.setBackground(knobBg);
        sw.addView(knob, knobParams(on));
        sw.setTag(knob);
        return sw;
    }

    private void refreshToggle(FrameLayout sw, boolean on) {
        if (sw == null) return;
        applyToggleLook(sw, on);
        Object knob = sw.getTag();
        if (knob instanceof View) {
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
        klp.leftMargin = dp(3);
        klp.rightMargin = dp(3);
        return klp;
    }

    private Drawable buildDrawerButtonBg() {
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
}
