package com.jerocine.player.ui;

import android.app.Activity;
import android.app.ActivityManager;
import android.content.Context;
import android.content.pm.PackageInfo;
import android.os.Build;
import android.util.DisplayMetrics;
import android.view.WindowManager;
import android.webkit.WebView;

/**
 * 设备诊断信息收集 —— 单包双壳**共用一份**。
 *
 * 合并前这里有两份等价实现（tv 壳 {@code com.jerocine.tv.ui.DeviceDiagnostics} 与
 * web 壳 {@code WebShellActivity.showDeviceDiagnostics()} 内联），同一个"设备诊断"
 * 菜单项在两壳显示的字段还不一致。合并后统一收敛到这里（细则 §5.1 / §9 步骤 5.5）。
 *
 * 核心目的：判断当前设备能不能跑 **Web 容器壳**（Capacitor，界面跑在系统 WebView 里）。
 * 单包双壳的设备能力路由器见 {@code com.jerocine.tv.route.DeviceProbe}，两者**同一套阈值**——
 * 阈值常量只在本类定义一次，DeviceProbe 引用它们，避免"注释说同源、实为两处写死"的漂移。
 *
 * 判定规则（与前端 vite 构建目标对齐）：
 *  - WebView 内核 Chrome &lt; 87 → 不推荐 Web 容器（Vue3 + Vite 产物跑不动）
 *  - 内存 &lt; 2GB → 不推荐（WebView 常驻内存压力大）
 *  - 两者都不触发 → 支持
 *
 * ⚠️ 不判"Android 版本够不够装"：合并后只有一个 APK（minSdk 21，低配也装得上），
 *    版本低只会被路由进原生壳，不是"装不上"的问题。
 */
public final class JcDeviceDiagnostics {

    // ---- 路由 / 诊断共用的设备能力阈值（单一来源，DeviceProbe 引用） ----

    /** 能跑 WebView 壳的最低 Android SDK（API 24 = 7.0）；低于它路由进原生壳 */
    public static final int MIN_SDK = 24;

    /** 前端 Vite 构建目标(es2020)要求的最低 Chromium 内核主版本 */
    public static final int MIN_WEBVIEW_MAJOR = 87;

    /** Web 容器可接受的最低总内存(GB) */
    public static final long MIN_MEMORY_GB = 2L;

    private JcDeviceDiagnostics() {}

    /** 一次采集到的设备信息快照（纯数据，便于单测直接构造）。 */
    public static final class Info {
        public final String manufacturer;
        public final String model;
        public final String androidRelease;
        public final int sdkInt;
        public final String abi;
        public final String webView;
        /** WebView 内核主版本；解析不到为 null */
        public final Integer chromeMajor;
        public final String resolution;
        public final float density;
        public final long totalRamMB;
        public final long availRamMB;
        public final String appVersion;

        public Info(String manufacturer, String model, String androidRelease, int sdkInt,
                    String abi, String webView, Integer chromeMajor, String resolution,
                    float density, long totalRamMB, long availRamMB, String appVersion) {
            this.manufacturer = manufacturer;
            this.model = model;
            this.androidRelease = androidRelease;
            this.sdkInt = sdkInt;
            this.abi = abi;
            this.webView = webView;
            this.chromeMajor = chromeMajor;
            this.resolution = resolution;
            this.density = density;
            this.totalRamMB = totalRamMB;
            this.availRamMB = availRamMB;
            this.appVersion = appVersion;
        }

        /** 能否跑 Web 容器壳（内核与内存两条阈值；读不到的值不阻塞判定） */
        public boolean supportsWebShell() {
            boolean chromeOk = chromeMajor == null || chromeMajor >= MIN_WEBVIEW_MAJOR;
            boolean ramOk = totalRamMB == 0L || totalRamMB >= MIN_MEMORY_GB * 1024L;
            return chromeOk && ramOk;
        }

        /** 一句话判定 + 处置建议（用户在设置里能自行切壳） */
        public String verdict() {
            if (chromeMajor != null && chromeMajor < MIN_WEBVIEW_MAJOR) {
                return "本机 WebView 内核 Chrome " + chromeMajor + " 过旧，Web 容器界面可能白屏/错乱；"
                        + "可在「设置 → 壳模式」固定为「原生」。";
            }
            if (totalRamMB > 0 && totalRamMB < MIN_MEMORY_GB * 1024L) {
                return "本机内存 " + totalRamMB + "MB 偏小，Web 容器可能卡顿；"
                        + "如有异常可在「设置 → 壳模式」固定为「原生」。";
            }
            if (chromeMajor != null) {
                return "本机满足 Web 容器运行条件(Android " + androidRelease
                        + " / WebView Chrome " + chromeMajor + ")。";
            }
            return "本机 WebView 内核版本未知；若界面异常，可在「设置 → 壳模式」固定为「原生」。";
        }

        /** 纯文本汇总（可一键复制发给维护者） */
        public String toText() {
            StringBuilder sb = new StringBuilder();
            sb.append("设备型号: ").append(manufacturer).append(' ').append(model).append('\n');
            sb.append("Android: ").append(androidRelease).append(" (API ").append(sdkInt).append(")\n");
            sb.append("ABI: ").append(abi).append('\n');
            sb.append("浏览器内核: ").append(webView).append('\n');
            sb.append("分辨率: ").append(resolution).append(" (density=").append(density).append(")\n");
            sb.append("内存: 总计 ").append(totalRamMB).append(" MB, 可用 ").append(availRamMB).append(" MB\n");
            sb.append("应用版本: ").append(appVersion).append('\n');
            sb.append("判定: ").append(supportsWebShell() ? "可跑 Web 容器壳" : "建议使用原生壳");
            return sb.toString();
        }
    }

    /**
     * 采集设备诊断信息。
     * 需在 UI 线程调用（兜底分支要构造 {@link WebView} 取 UA 时需要 Looper）。
     */
    public static Info collect(Context context) {
        WebViewInfo wv = webViewInfo(context);

        String resolution = "未知";
        float density = 0f;
        try {
            WindowManager wm = (WindowManager) context.getSystemService(Context.WINDOW_SERVICE);
            DisplayMetrics metrics = new DisplayMetrics();
            if (wm != null) {
                @SuppressWarnings("deprecation")
                DisplayMetrics real = new DisplayMetrics();
                wm.getDefaultDisplay().getRealMetrics(real);
                metrics = real;
            }
            resolution = metrics.widthPixels + "×" + metrics.heightPixels;
            density = metrics.density;
        } catch (Exception ignored) {
            // 分辨率读不到不影响其余诊断项
        }

        long totalRamMB = 0L;
        long availRamMB = 0L;
        try {
            ActivityManager am = (ActivityManager) context.getSystemService(Context.ACTIVITY_SERVICE);
            if (am != null) {
                ActivityManager.MemoryInfo mi = new ActivityManager.MemoryInfo();
                am.getMemoryInfo(mi);
                totalRamMB = mi.totalMem / 1024 / 1024;
                availRamMB = mi.availMem / 1024 / 1024;
            }
        } catch (Exception ignored) {
        }

        return new Info(
                blankTo(Build.MANUFACTURER, "unknown"),
                blankTo(Build.MODEL, "unknown"),
                blankTo(Build.VERSION.RELEASE, "unknown"),
                Build.VERSION.SDK_INT,
                (Build.SUPPORTED_ABIS != null && Build.SUPPORTED_ABIS.length > 0)
                        ? Build.SUPPORTED_ABIS[0] : "未知",
                wv.description,
                wv.major,
                resolution,
                density,
                totalRamMB,
                availRamMB,
                versionLabel(context));
    }

    /** 用项目统一的 JcDialog 展示（铁律：不手写 AlertDialog.Builder）。 */
    public static void show(Activity activity) {
        Info info = collect(activity);
        JcDialog.message(activity)
                .title("设备诊断")
                .message(info.toText())
                .confirmButton("关闭")
                .show();
    }

    // ---- internal ----

    private static String blankTo(String s, String fallback) {
        return (s == null || s.trim().isEmpty()) ? fallback : s;
    }

    /**
     * "应用名 版本名(构建号)" —— 读已安装包的真实版本。
     * 库自己的 BuildConfig 在单包合并后不可信（版本来自宿主），必须走 PackageManager。
     */
    private static String versionLabel(Context context) {
        String name = "Jerocine";
        try {
            CharSequence label = context.getApplicationInfo().loadLabel(context.getPackageManager());
            if (label != null && label.toString().trim().length() > 0) name = label.toString();
        } catch (Exception ignored) {
        }
        try {
            PackageInfo pi = context.getPackageManager().getPackageInfo(context.getPackageName(), 0);
            if (pi.versionName != null && pi.versionName.length() > 0) {
                return name + " " + pi.versionName + "(" + pi.versionCode + ")";
            }
        } catch (Exception ignored) {
        }
        return name;
    }

    /** (描述, 内核主版本) —— 主版本解析不到为 null */
    private static final class WebViewInfo {
        final String description;
        final Integer major;

        WebViewInfo(String description, Integer major) {
            this.description = description;
            this.major = major;
        }
    }

    private static WebViewInfo webViewInfo(Context context) {
        // 1) API 26+：WebView 实现可独立更新，直接取当前 WebView 包
        if (Build.VERSION.SDK_INT >= 26) {
            try {
                PackageInfo pkg = WebView.getCurrentWebViewPackage();
                if (pkg != null) {
                    return new WebViewInfo(
                            "WebView " + pkg.versionName + " (" + pkg.packageName + ")",
                            majorOf(pkg.versionName));
                }
            } catch (Exception ignored) {
            }
        }
        // 2) API 21-25：WebView 可能是独立更新包，按常见包名探测
        for (String pkgName : new String[]{"com.google.android.webview", "com.android.webview"}) {
            try {
                PackageInfo pi = context.getPackageManager().getPackageInfo(pkgName, 0);
                return new WebViewInfo(
                        "WebView " + pi.versionName + " (" + pkgName + ")",
                        majorOf(pi.versionName));
            } catch (Exception ignored) {
            }
        }
        // 3) 兜底：从 UA 解析内核版本（系统内置 WebView）
        try {
            String ua = new WebView(context).getSettings().getUserAgentString();
            java.util.regex.Matcher m = java.util.regex.Pattern
                    .compile("Chrome/(\\d+)(?:\\.\\d+)*").matcher(ua == null ? "" : ua);
            if (m.find()) {
                return new WebViewInfo(
                        "系统内置 WebView (内核 Chrome " + m.group(1) + ")",
                        parseIntOrNull(m.group(1)));
            }
            if (ua != null && ua.length() > 0) {
                return new WebViewInfo("系统内置 WebView (UA: " + ua + ")", null);
            }
        } catch (Exception ignored) {
        }
        return new WebViewInfo("WebView 不可用", null);
    }

    private static Integer majorOf(String versionName) {
        if (versionName == null) return null;
        int dot = versionName.indexOf('.');
        return parseIntOrNull(dot >= 0 ? versionName.substring(0, dot) : versionName);
    }

    private static Integer parseIntOrNull(String s) {
        try {
            return Integer.valueOf(s.trim());
        } catch (Exception e) {
            return null;
        }
    }
}
