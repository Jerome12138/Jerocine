package com.jerocine.tv.ui

import android.app.ActivityManager
import android.content.Context
import android.os.Build
import android.util.DisplayMetrics
import android.view.WindowManager
import android.webkit.WebView

/**
 * 设备诊断信息收集: 安装后异常排查用。
 *
 * 核心目的: 判断当前 TV 能否使用 **Web 嵌入版 APK**(Capacitor 壳, minSdk 24 / Android 7.0+,
 * 界面跑在系统 WebView 里)。老设备(如坚果 Nano: Android 6.0 / Chromium 48)装不上或渲染不了,
 * 应推荐使用本低配独立版。
 *
 * 判定规则(与 web/android 的 minSdk、前端 vite 构建目标对齐):
 *  - Android SDK < 24 → 不支持(Web 版 APK 无法安装)
 *  - WebView 内核 Chrome < 87 → 不推荐(网页界面可能白屏/错乱, 建议独立版)
 *  - 其余 → 支持
 */
object DeviceDiagnostics {

    /** Web 嵌入版(Capacitor)APK 的最低 Android SDK: web/android minSdk 24 */
    const val WEB_APK_MIN_SDK = 24

    /** 前端 Vite 构建目标(es2020)要求的最低 Chromium 内核 */
    const val MIN_CHROME_FOR_WEB = 87

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
        val supportsWebApk: Boolean
            get() = sdkInt >= WEB_APK_MIN_SDK &&
                (chromeMajor == null || chromeMajor >= MIN_CHROME_FOR_WEB)

        val verdict: String
            get() = when {
                sdkInt < WEB_APK_MIN_SDK ->
                    "本机 Android $androidRelease (API $sdkInt) 低于 Web 嵌入版要求(Android 7.0+)，无法安装 Web 嵌入版 APK；请继续使用本独立版。"
                chromeMajor != null && chromeMajor < MIN_CHROME_FOR_WEB ->
                    "本机 WebView 内核 Chrome $chromeMajor 过旧，Web 嵌入版界面可能白屏/错乱；建议使用本独立版。"
                chromeMajor != null ->
                    "本机满足 Web 嵌入版 APK 运行条件(Android $androidRelease / WebView Chrome $chromeMajor)，可安装 Web 嵌入版；如使用中异常，请反馈下方诊断信息。"
                else ->
                    "本机 Android 版本满足 Web 嵌入版要求(Android 7.0+)，但 WebView 内核版本未知；可尝试安装 Web 嵌入版，如异常请反馈下方诊断信息。"
            }

        /** 纯文本汇总(可一键复制发给维护者) */
        fun toText(): String = buildString {
            append("设备型号: ").append(manufacturer).append(' ').append(model).append('\n')
            append("Android: ").append(androidRelease).append(" (API ").append(sdkInt).append(")\n")
            append("浏览器内核: ").append(webView).append('\n')
            append("分辨率: ").append(resolution).append('\n')
            append("内存: ").append(ramMB).append(" MB\n")
            append("应用版本: ").append(appVersion).append('\n')
            append("判定: ").append(if (supportsWebApk) "支持 Web 嵌入版" else "建议使用低配独立版")
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

        val label = (context.applicationInfo.loadLabel(context.packageManager) ?: "Jerocine影视 TV").toString()
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
