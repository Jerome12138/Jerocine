package com.jerocine.tv.ui

import android.app.ActivityManager
import android.content.Context
import android.os.Build
import android.util.DisplayMetrics
import android.view.WindowManager
import android.webkit.WebView
import com.jerocine.tv.route.DeviceProbe

/**
 * 设备诊断信息收集: 安装后异常排查用。
 *
 * 核心目的: 判断当前设备能不能跑 **Web 容器壳**(Capacitor, 界面跑在系统 WebView 里)。
 * 单包合并后两壳在同一个 APK 里(见 [com.jerocine.tv.route.DeviceProbe] 的路由规则):
 *  - 能跑 WebView 壳 → 走 WebShell(功能全、更新快)
 *  - 跑不动 → 走 NativeShell(原生 UI, 低配兜底), **不需要另装独立版**
 *
 * 判定规则(与 DeviceProbe 的阈值同源、与前端 vite 构建目标对齐):
 *  - WebView 内核 Chrome < 87 → 不推荐 Web 容器(网页界面可能白屏/错乱)
 *  - 内存 < 2GB → 不推荐(WebView 常驻内存压力大)
 *  - 其余 → 支持
 *
 * ⚠️ 不再判"Android 版本够不够装 Web 版 APK": 合并后只有一个 APK, minSdk 由
 *    Capacitor 决定(24 / Android 7.0), 装得上就说明版本够了。
 */
object DeviceDiagnostics {

    /** 前端 Vite 构建目标(es2020)要求的最低 Chromium 内核 —— 直接引用路由阈值, 避免两处漂移 */
    const val MIN_CHROME_FOR_WEB = DeviceProbe.MIN_WEBVIEW_MAJOR

    /** Web 容器可接受的最低内存(MB) —— 同上, 与 DeviceProbe 的路由阈值同源 */
    const val MIN_RAM_MB_FOR_WEB = DeviceProbe.MIN_MEMORY_GB * 1024

    data class Info(
        val manufacturer: String,
        val model: String,
        val androidRelease: String,
        val sdkInt: Int,
        val webView: String,
        val chromeMajor: Int?,
        val resolution: String,
        val ramMB: Long,
        val appVersion: String,
    ) {
        /** 能否跑 Web 容器壳(内核与内存两条阈值, 引用 DeviceProbe 同源常量) */
        val supportsWebShell: Boolean
            get() = (chromeMajor == null || chromeMajor >= MIN_CHROME_FOR_WEB) &&
                (ramMB == 0L || ramMB >= MIN_RAM_MB_FOR_WEB)

        val verdict: String
            get() = when {
                chromeMajor != null && chromeMajor < MIN_CHROME_FOR_WEB ->
                    "本机 WebView 内核 Chrome $chromeMajor 过旧，Web 容器界面可能白屏/错乱；" +
                        "可在「设置 → 壳模式」固定为「原生」。"
                ramMB in 1 until MIN_RAM_MB_FOR_WEB ->
                    "本机内存 ${ramMB}MB 偏小，Web 容器可能卡顿；如有异常可在「设置 → 壳模式」固定为「原生」。"
                chromeMajor != null ->
                    "本机满足 Web 容器运行条件(Android $androidRelease / WebView Chrome $chromeMajor)。"
                else ->
                    "本机 WebView 内核版本未知；若界面异常，可在「设置 → 壳模式」固定为「原生」。"
            }

        /** 纯文本汇总(可一键复制发给维护者) */
        fun toText(): String = buildString {
            append("设备型号: ").append(manufacturer).append(' ').append(model).append('\n')
            append("Android: ").append(androidRelease).append(" (API ").append(sdkInt).append(")\n")
            append("浏览器内核: ").append(webView).append('\n')
            append("分辨率: ").append(resolution).append('\n')
            append("内存: ").append(ramMB).append(" MB\n")
            append("应用版本: ").append(appVersion).append('\n')
            append("判定: ").append(if (supportsWebShell) "可跑 Web 容器壳" else "建议使用原生壳")
        }
    }

    /**
     * 收集设备诊断信息。
     * 需在 UI 线程调用(构造 WebView 取 UA 兜底时需要 Looper)。
     */
    fun collect(context: Context): Info {
        val webViewInfo = webViewInfo(context)
        val wm = context.getSystemService(Context.WINDOW_SERVICE) as WindowManager
        val metrics = DisplayMetrics()
        @Suppress("DEPRECATION")
        wm.defaultDisplay.getRealMetrics(metrics)
        val resolution = "${metrics.widthPixels}×${metrics.heightPixels}"

        var ramMB = 0L
        runCatching {
            val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
            val mi = ActivityManager.MemoryInfo()
            am.getMemoryInfo(mi)
            ramMB = mi.totalMem / 1024 / 1024
        }

        val label = versionLabel(context)
        return Info(
            manufacturer = Build.MANUFACTURER.ifBlank { "unknown" },
            model = Build.MODEL.ifBlank { "unknown" },
            androidRelease = Build.VERSION.RELEASE,
            sdkInt = Build.VERSION.SDK_INT,
            webView = webViewInfo.first,
            chromeMajor = webViewInfo.second,
            resolution = resolution,
            ramMB = ramMB,
            appVersion = label,
        )
    }

    /**
     * "应用名 版本名(构建号)" —— 原来这里取的是 applicationInfo.loadLabel(应用**名**),
     * 却挂在"应用版本"标签下, 报障时给不出真正的版本。改为读已安装包的真实版本
     * (合并后唯一 APK 的版本来自宿主, 库自己的 BuildConfig 不可信)。
     */
    private fun versionLabel(context: Context): String {
        val name = runCatching {
            context.applicationInfo.loadLabel(context.packageManager)?.toString()
        }.getOrNull()?.takeIf { it.isNotBlank() } ?: "Jerocine"
        val version = runCatching {
            val pi = context.packageManager.getPackageInfo(context.packageName, 0)
            "${pi.versionName}(${pi.versionCode})"
        }.getOrNull()
        return if (version.isNullOrBlank()) name else "$name $version"
    }

    /** 返回 (WebView 描述, Chrome 内核主版本); 描述优先取包版本, 内核版本解析自版本号/UA。 */
    private fun webViewInfo(context: Context): Pair<String, Int?> {
        // 1) API 26+: WebView 实现可更新, 直接取当前 WebView 包(返回 PackageInfo)
        if (Build.VERSION.SDK_INT >= 26) {
            val pkg = runCatching { WebView.getCurrentWebViewPackage() }.getOrNull()
            if (pkg != null) {
                val major = pkg.versionName?.substringBefore('.')?.toIntOrNull()
                return "WebView ${pkg.versionName} (${pkg.packageName})" to major
            }
        }
        // 2) 常见 WebView 包名探测(API 21-25 的独立更新包)
        for (pkg in arrayOf("com.google.android.webview", "com.android.webview")) {
            val pi = runCatching { context.packageManager.getPackageInfo(pkg, 0) }.getOrNull() ?: continue
            val major = pi.versionName?.substringBefore('.')?.toIntOrNull()
            return "WebView ${pi.versionName} ($pkg)" to major
        }
        // 3) 兜底: 从 UA 解析内核版本(系统内置 WebView)
        runCatching {
            val ua = WebView(context).settings.userAgentString
            val m = Regex("Chrome/(\\d+)(?:\\.\\d+)*").find(ua)
            if (m != null) {
                val major = m.groupValues[1].toIntOrNull()
                return "系统内置 WebView (内核 Chrome ${m.groupValues[1]})" to major
            }
            if (ua.isNotBlank()) return "系统内置 WebView (UA: $ua)" to null
        }
        return "WebView 不可用" to null
    }
}
