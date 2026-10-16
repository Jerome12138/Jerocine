package com.jerocine.player.ui;

import android.app.Activity;
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

import androidx.core.content.ContextCompat;

import com.jerocine.player.PlayerAdFilterHelper;
import com.jerocine.player.R;
import com.jerocine.player.data.ShellPrefs;

import java.util.Locale;

/**
 * 设置抽屉 —— 单包双壳**共用一份**（下沉到 player-core，细则 §5.1 / §6.3）。
 * 命名与设计稿 §6.3 对齐（2026-10-12 由 `SettingsDrawerController` 改名 `JcSettingsDrawer`、
 * 内部接口 `Host` 改名 `JcSettingsHost`），与 core `ui` 包其他公共件（`JcDialog` /
 * `JcDeviceDiagnostics` / `GlassToast`）保持同一命名规范。
 *
 * 合并前 web 壳在 WebShellActivity 里内联约 450 行构建、原生壳是全页 SettingsFragment；
 * 现在两壳都在 onCreate 里 new 出本控制器，外观与交互完全一致
 * （§10 验收项"设置页在两种模式下同一份抽屉、外观一致"）。
 *
 * 设计原则: 通用项(广告过滤/设备诊断/服务器/版本/检查更新/壳模式/退出)直接处理;
 * 壳特有项(刷新页面/账号登录态/显示模式/减少动效)通过 {@link JcSettingsHost} 回调交给各壳实现:
 * web 壳有"显示模式"无"减少动效", 原生壳相反 —— 由 host.hasDisplayMode()/hasMotionRow()
 * 各自隐藏对方那一行。
 *
 * 用法:
 *   controller = new JcSettingsDrawer(activity, prefs, host);
 *   controller.build();          // onCreate 时构建(懒挂载到 android.R.id.content)
 *   controller.show();           // 菜单键/按钮触发
 *   controller.hide();           // 外部点击/返回键
 *   controller.refreshAccount(); // 登录态变化时刷新账号行
 *   controller.refreshDisplayMode(); // web 壳显示模式变化时刷新
 */
public class JcSettingsDrawer {

    // ==================== 设计令牌 ====================
    // ⚠️ 颜色/尺寸的**唯一真源**是 player-core 的 res/values/colors.xml + res/values/dimens.xml。
    //    本类不再写十六进制/数字字面量 —— 历史上本类与 web 壳的 WebShellActivity 各存一份 GF_*
    //    与 DRAWER_W_DP/ROW_H_DP(加上资源共 3 份), 改一处色必漏另两处(2026-10-12 收编)。
    //    编程式构建 View 仍需要 int, 但一律经下面的 {@link #col(int)} / {@link #dimenPx(int)} 从资源解析。
    /**
     * 壳模式存储键 —— 契约只在 {@link ShellPrefs} 定义一次(与 EntryRouter/DeviceProbe 共用),
     * 这里保留常量只为兼容既有引用。
     */
    public static final String KEY_SHELL_MODE = ShellPrefs.KEY_SHELL_MODE;

    /** 壳特有操作回调 —— web 壳和原生壳各自实现 */
    public interface JcSettingsHost {
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

        // ---- 壳类型 ----
        /**
         * 当前是不是 Web 壳。
         *
         * 抽屉据此收敛"某壳专属"的条目与文案 —— 原生壳下"显示模式"之类 web 专属项不出现，
         * "刷新页面 / 检查更新"的副标题也不能带 web 口径（"重新加载当前站点" / "GET /app/version/latest"
         * 在原生壳里是无意义的）。比只看 hasDisplayMode()/hasMotionRow() 更直白:
         * 那两个是"本壳有没有这一行", 这个是"本壳是谁", 加新差异项时判据不漂。
         */
        boolean isWebShell();

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

    /**
     * 账号行快照。
     *
     * ⚠️ 文案（标题/副标题/按钮）**只在这里定义一次**：抽屉两壳共用，账号行的字面量也必须同源，
     *    否则同一个抽屉在两种模式下显示不同的字（§10 验收项「外观一致」会名存实亡）。
     *    历史上 web 壳曾在 {@code SettingsDrawerLogic} 里另写一套 accountTitle/accountSubtitle/
     *    accountActionText（其中 accountSubtitle 从未被生产代码调用、且 refreshAccountRow 恒传
     *    "登录后…" 副标题 ⇒ 已登录时显示的是未登录文案），已删除。
     *    各壳只负责把"登录态 + 昵称"喂进来，拼装一律走下面两个工厂。
     *
     * `title` 取昵称（拿不到才退回"已登录"）；`subtitle` 区分已/未登录两句。
     */
    public static class AccountInfo {
        public final String title;
        public final String subtitle;
        public final String actionText;
        public final boolean loggedIn;
        public AccountInfo(String title, String subtitle, String actionText, boolean loggedIn) {
            this.title = title; this.subtitle = subtitle;
            this.actionText = actionText; this.loggedIn = loggedIn;
        }

        /** 未登录 —— 抽屉两壳唯一来源 */
        public static AccountInfo loggedOut() {
            return new AccountInfo("未登录", "登录后收藏 / 历史 / 跳过设置多端同步", "登录", false);
        }

        /** 已登录 —— `name` 为昵称，可为 null/空（拿不到昵称时标题退回"已登录"） */
        public static AccountInfo loggedIn(String name) {
            String t = (name == null) ? "" : name.trim();
            return new AccountInfo(t.isEmpty() ? "已登录" : t,
                    "收藏 / 历史 / 跳过设置多端同步", "退出", true);
        }
    }

    private final Activity activity;
    private final SharedPreferences prefs;
    private final JcSettingsHost host;

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

    public JcSettingsDrawer(Activity activity, SharedPreferences prefs, JcSettingsHost host) {
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
        settingsOverlay.setBackgroundColor(col(R.color.jc_scrim));
        settingsOverlay.setVisibility(View.GONE);
        settingsOverlay.setClickable(true);
        settingsOverlay.setOnClickListener(v -> hide());
        root.addView(settingsOverlay, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

        settingsPanelScroll = new ScrollView(activity);
        GradientDrawable drawerBg = new GradientDrawable();
        drawerBg.setColor(col(R.color.jc_glass));
        drawerBg.setStroke(dp(1), col(R.color.jc_stroke));
        settingsPanelScroll.setBackground(drawerBg);
        settingsPanelScroll.setClickable(true);
        FrameLayout.LayoutParams panelLp = new FrameLayout.LayoutParams(
                dimenPx(R.dimen.jc_settings_drawer_width), ViewGroup.LayoutParams.MATCH_PARENT, Gravity.END);
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
        head.addView(makeText("设置", 20, col(R.color.jc_text), true), new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f));
        head.addView(makeText(installedVersionText(), 12, col(R.color.jc_text_tertiary), false));
        ImageView closeBtn = new ImageView(activity);
        closeBtn.setImageResource(R.drawable.ic_close);
        closeBtn.setColorFilter(col(R.color.jc_text_secondary));
        closeBtn.setContentDescription("关闭");
        closeBtn.setPadding(dp(4), dp(4), dp(4), dp(4));
        closeBtn.setFocusable(true);
        closeBtn.setClickable(true);
        closeBtn.setOnClickListener(v -> hide());
        head.addView(closeBtn, new LinearLayout.LayoutParams(dp(24), dp(24)));
        settingsPanel.addView(head, rowParams(ViewGroup.LayoutParams.WRAP_CONTENT, 8));

        // ---------- 置顶: 刷新 ----------
        // 文案随壳: web 壳重新加载站点, 原生壳重新拉取当前页数据 —— 原生壳没有"站点"可言。
        final boolean web = host.isWebShell();
        addNavRow(web ? "刷新页面" : "刷新数据",
                web ? "重新加载当前站点" : "重新拉取当前页面内容", "", () -> {
            host.onRefreshPage();
            hide();
        });

        // ---------- 账号 ----------
        addGroupHeader("账号");
        AccountInfo acct = host.getAccountInfo();
        LinearLayout accountRow = makeRowBase(
                acct.title, acct.subtitle, true, v -> host.onAccountAction());
        accountTitleView = (TextView) ((LinearLayout) accountRow.getChildAt(0)).getChildAt(0);
        accountAction = makeText(acct.actionText, 13, col(R.color.jc_accent), false);
        addRowTail(accountRow, accountAction);
        settingsPanel.addView(accountRow, rowParams(dimenPx(R.dimen.jc_settings_row_height), 6));

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
        addNavRow("检查更新", web ? "GET /app/version/latest" : "查看服务端最新版本", "立即检查",
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
                    ? settingsPanelScroll.getWidth() : dimenPx(R.dimen.jc_settings_drawer_width);
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
                ? settingsPanelScroll.getWidth() : dimenPx(R.dimen.jc_settings_drawer_width);
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

    /**
     * 壳模式切换后的**后果说明**（两壳同源文案）。
     *
     * <p>为什么在 core 写死一处：用户切壳前必须知道"会变成什么样" —— Web 壳功能全但低配卡，
     * 原生壳流畅却没有应用内更新。这段文案两壳必须一字不差，否则同一个设置在两种壳里
     * 给出不同承诺（与账号行文案同源同理）。
     */
    public static String shellModeSwitchMessage(String mode) {
        String m = normalizeShellMode(mode);
        if ("web".equals(m)) {
            return "界面由网页渲染，功能与线上版本一致（含应用内更新）。"
                    + "低配设备可能较卡；Web 容器不可用时会自动回退原生壳。";
        }
        if ("native".equals(m)) {
            return "界面由本机渲染，流畅省电，适合低配设备。"
                    + "不含应用内更新等 Web 专属功能。";
        }
        return "每次启动按设备能力自动选择：跑得动 Web 容器就用 Web 壳，否则用原生壳。";
    }

    /**
     * 壳模式切换确认弹窗：说明后果 + 询问是否立即重启（两壳共用一份）。
     *
     * <p>壳模式**只在下次启动生效**（EntryRouterActivity.onCreate 读 prefs 路由），
     * 所以必须问一句"要不要现在重启"，选"立即重启"就自动重启，别让用户自己去杀进程。
     */
    public static void showShellModeSwitchDialog(final Activity activity, String mode) {
        if (activity == null || activity.isFinishing()) return;
        JcDialog.message(activity)
                .title("已切换为「" + shellModeLabel(mode) + "」模式")
                .message(shellModeSwitchMessage(mode) + "\n\n壳模式重启后生效。")
                .confirmButton("立即重启", () -> restartApp(activity))
                .cancelButton("稍后")
                .show();
    }

    /**
     * 重启应用 —— 让 EntryRouterActivity 按新的壳模式重新路由。
     *
     * <p>做法：用带 {@code CLEAR_TASK} 的 launcher intent 重建任务栈 —— 清掉当前壳（及其上
     * 可能压着的播放器/下载页），让 EntryRouterActivity 重新 onCreate 按 prefs 路由。
     * **不杀进程**。
     *
     * <p>为什么不是"杀进程 + 闹钟拉起"（更彻底的写法，先试过但在这台机器上不可用）：
     * {@code AlarmManager.set} 在 MIUI 上被省电策略拦掉 —— 实测点完"立即重启"进程已死、
     * 12s 后仍停在桌面没被拉起；换 {@code setExact} 要 SCHEDULE_EXACT_ALARM 权限，
     * {@code setAlarmClock} 会在状态栏常挂一个闹钟图标。而"切壳"真正需要的只是**重新路由**：
     * 重建任务栈即可，旧壳 Activity 会被正常销毁（WebView 这类重资源随之释放），
     * 进程级初始化（ServiceLocator / mediaClient / Coil）本就幂等、不需要重跑。
     */
    public static void restartApp(final Activity activity) {
        try {
            android.content.Intent launch = activity.getPackageManager()
                    .getLaunchIntentForPackage(activity.getPackageName());
            if (launch == null) return;
            launch.addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK
                    | android.content.Intent.FLAG_ACTIVITY_CLEAR_TASK);
            activity.startActivity(launch);
        } catch (Exception ignore) {
            // 拿不到 launcher intent（极罕见）→ 什么都不做，用户手动重开
        }
    }

    static String nextShellMode(String current) {
        String m = normalizeShellMode(current);
        if ("web".equals(m)) return "native";
        if ("native".equals(m)) return "auto";
        return "web";
    }

    public static String shellModeLabel(String mode) {
        String m = normalizeShellMode(mode);
        if ("web".equals(m)) return "Web";
        if ("native".equals(m)) return "原生";
        return "自动";
    }

    /**
     * 壳模式取值归一(去空白 + 转小写)。
     *
     * <p>写入端(本类 {@link #cycleShellMode} 与 web 壳的 WebView 降级)统一存**小写**
     * {@code auto/web/native}; 历史版本存过枚举大写名。归一后本类的标签/循环顺序才与读取端
     * {@code DeviceProbe.readUserMode} 完全一致 —— 否则会出现"值是大写 NATIVE、标签却显示
     * 自动、点一下跳错方向"的诡异状态(2026-10-12 修)。</p>
     */
    private static String normalizeShellMode(String mode) {
        return mode == null ? "" : mode.trim().toLowerCase(Locale.ROOT);
    }

    // ==================== UI 辅助方法(从 WebShellActivity 抽取) ====================

    private int dp(int v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v,
                activity.getResources().getDisplayMetrics());
    }

    /**
     * 从资源取色 —— 编程式 View 构建的**唯一取色入口**。
     * 色值唯一真源是 res/values/colors.xml（见类顶部"设计令牌"说明），本类不写任何十六进制。
     */
    private int col(int colorResId) {
        return ContextCompat.getColor(activity, colorResId);
    }

    /** 从资源取尺寸(px) —— 尺寸唯一真源是 res/values/dimens.xml */
    private int dimenPx(int dimenResId) {
        return (int) activity.getResources().getDimension(dimenResId);
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
        barBg.setColor(col(R.color.jc_accent));
        barBg.setCornerRadius(dp(2));
        LinearLayout.LayoutParams barLp = new LinearLayout.LayoutParams(dp(3), dp(12));
        barLp.bottomMargin = dp(6);
        wrap.addView(bar, barLp);

        TextView t = makeText(label, 12, col(R.color.jc_text_tertiary), true);
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
        TextView t = makeText(title, 16, col(R.color.jc_text), false);
        col.addView(t, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        if (sub != null && !sub.isEmpty()) {
            TextView s = makeText(sub, 12, col(R.color.jc_text_secondary), false);
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

    /**
     * minSdk 21 无 coreLibraryDesugaring: {@code java.util.function.Consumer} 是 API 24+,
     * Android 6 老设备(坚果投影)上实现它的 lambda 合成类链接失败 → NoClassDefFoundError
     * (2026-10-15 实测)。自定义接口替代, 先例 {@link com.jerocine.player.PlayerDialogHelper}#IntConsumer。
     */
    interface Consumer<T> {
        void accept(T v);
    }

    private void addToggleRow(String title, String sub, boolean on,
                              Consumer<FrameLayout> switchOut,
                              Runnable onToggle) {
        LinearLayout row = makeRowBase(title, sub, true, v -> onToggle.run());
        FrameLayout sw = buildToggleSwitch(on);
        if (switchOut != null) switchOut.accept(sw);
        addRowTail(row, sw);
        settingsPanel.addView(row, rowParams(dimenPx(R.dimen.jc_settings_row_height), 6));
    }

    private TextView addNavRow(String title, String sub, String value, Runnable action) {
        LinearLayout row = makeRowBase(title, sub, true, v -> action.run());
        TextView v = makeText(value == null ? "" : value, 13, col(R.color.jc_accent), false);
        addRowTail(row, v);
        settingsPanel.addView(row, rowParams(dimenPx(R.dimen.jc_settings_row_height), 6));
        return v;
    }

    private void addInfoRow(String title, String value) {
        addInfoRow(title, null, value);
    }

    private void addInfoRow(String title, String sub, String value) {
        LinearLayout row = makeRowBase(title, sub, false, null);
        TextView v = makeText(value == null ? "" : value, 13, col(R.color.jc_text_secondary), false);
        addRowTail(row, v);
        settingsPanel.addView(row, rowParams(dimenPx(R.dimen.jc_settings_row_height), 6));
    }

    private FrameLayout buildToggleSwitch(boolean on) {
        FrameLayout sw = new FrameLayout(activity);
        sw.setLayoutParams(new LinearLayout.LayoutParams(dp(44), dp(24)));
        applyToggleLook(sw, on);
        View knob = new View(activity);
        GradientDrawable knobBg = new GradientDrawable();
        knobBg.setShape(GradientDrawable.OVAL);
        knobBg.setColor(col(R.color.jc_knob));
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
        track.setColor(on ? col(R.color.jc_accent) : col(R.color.jc_track_off));
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
        focused.setColor(col(R.color.jc_accent_dim));
        focused.setCornerRadius(dp(14));
        focused.setStroke(dp(2), col(R.color.jc_accent));

        GradientDrawable pressed = new GradientDrawable();
        pressed.setColor(col(R.color.jc_accent_press));
        pressed.setCornerRadius(dp(14));

        GradientDrawable normal = new GradientDrawable();
        normal.setColor(col(R.color.jc_glass_soft));
        normal.setCornerRadius(dp(14));
        normal.setStroke(dp(1), col(R.color.jc_stroke));

        StateListDrawable sld = new StateListDrawable();
        sld.addState(new int[]{android.R.attr.state_focused}, focused);
        sld.addState(new int[]{android.R.attr.state_pressed}, pressed);
        sld.addState(android.util.StateSet.WILD_CARD, normal);
        return sld;
    }
}
