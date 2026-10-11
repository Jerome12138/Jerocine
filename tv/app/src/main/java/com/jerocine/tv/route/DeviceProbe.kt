package com.jerocine.tv.route

import android.content.Context
import android.content.SharedPreferences
import androidx.annotation.VisibleForTesting
import kotlin.jvm.JvmStatic

/**
 * 单包合并后的壳模式路由器的设备能力探针。
 *
 * 职责: 根据设备能力判断"这台机器能不能跑 WebView 壳"。
 * - 能跑 → 走 WebShell (Vue3 SPA, 功能全、更新快)
 * - 跑不动 → 走 NativeShell (原生 UI, 低配兜底)
 *
 * 判定阈值(2026-10-11 定稿, 与执行手册一致):
 *   - Android SDK >= 24 (7.0)
 *   - 系统 WebView 主版本 >= 87 (2020年底的 Chrome, 能跑 Vue3 + Vite 产物)
 *   - 总内存 >= 2 GB
 * 三者同时满足 → web; 任一不满足 → native。
 *
 * 设计为纯逻辑 + 依赖注入: [DeviceCapabilities] 封装所有 Android API 调用,
 * 单测时注入假实现, 不需要 Robolectric / 真机。
 */
object DeviceProbe {

    /** 用户可在设置里强制选择的壳模式 */
    enum class ShellMode { AUTO, WEB, NATIVE }

    /** 探针最终判定结果 */
    enum class ShellTarget { WEB, NATIVE }

    /** 阈值常量 —— 集中在此, 调阈值不用翻逻辑 */
    const val MIN_SDK = 24
    const val MIN_WEBVIEW_MAJOR = 87
    const val MIN_MEMORY_GB = 2L

    /** SharedPreferences 里存用户选择的键 */
    const val PREFS_NAME = "jerocine_shell"
    const val KEY_SHELL_MODE = "jc-shell-mode"

    /**
     * 设备能力抽象 —— 真实实现走 Android API, 单测用假实现。
     * 把所有"需要 Context / PackageManager / ActivityManager"的调用隔离到这里,
     * DeviceProbe 的核心逻辑就变成纯函数, 可以直接 JVM 单测。
     */
    interface DeviceCapabilities {
        val sdkInt: Int
        val webViewMajorVersion: Int  // 读不到返回 0
        val totalMemoryBytes: Long
    }

    /** 从 SharedPreferences 读用户选择, 缺省 AUTO */
    fun readUserMode(prefs: SharedPreferences): ShellMode {
        return when (prefs.getString(KEY_SHELL_MODE, ShellMode.AUTO.name)) {
            ShellMode.WEB.name -> ShellMode.WEB
            ShellMode.NATIVE.name -> ShellMode.NATIVE
            else -> ShellMode.AUTO
        }
    }

    /**
     * 综合判定: 用户选择优先, AUTO 时走探针。
     * @return 最终应该启动哪个壳
     */
    fun resolveTarget(userMode: ShellMode, caps: DeviceCapabilities): ShellTarget {
        return when (userMode) {
            ShellMode.WEB -> ShellTarget.WEB
            ShellMode.NATIVE -> ShellTarget.NATIVE
            ShellMode.AUTO -> probe(caps)
        }
    }

    /**
     * 纯探针逻辑: 三个阈值同时满足 → WEB, 否则 NATIVE。
     * 拆成独立函数便于单测每个阈值的边界。
     */
    @VisibleForTesting
    fun probe(caps: DeviceCapabilities): ShellTarget {
        val sdkOk = caps.sdkInt >= MIN_SDK
        val webViewOk = caps.webViewMajorVersion >= MIN_WEBVIEW_MAJOR
        val memOk = caps.totalMemoryBytes >= MIN_MEMORY_GB * 1024 * 1024 * 1024
        return if (sdkOk && webViewOk && memOk) ShellTarget.WEB else ShellTarget.NATIVE
    }

    /** 便捷入口: 从 Context 一次性读完用户模式 + 真实设备能力, 返回目标 */
    @JvmStatic
    fun resolve(context: Context): ShellTarget {
        val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        val userMode = readUserMode(prefs)
        val caps = RealCapabilities(context)
        return resolveTarget(userMode, caps)
    }

    /** 真实设备能力实现 —— 仅在 resolve(context) 时构造 */
    private class RealCapabilities(private val context: Context) : DeviceCapabilities {
        override val sdkInt: Int
            get() = android.os.Build.VERSION.SDK_INT

        override val webViewMajorVersion: Int by lazy {
            try {
                // WebView.getCurrentWebViewPackageName() 是 API 26+ 方法,
                // minSdk=21 不能直接调用(编译期 Unresolved), 用反射兼容旧版本。
                // API < 26 时反射抛 NoSuchMethodException → catch 返回 0 → 探针判 native。
                val method = android.webkit.WebView::class.java
                    .getMethod("getCurrentWebViewPackageName")
                val pkgName = method.invoke(null) as? String ?: return@lazy 0
                val pm = context.packageManager
                @Suppress("DEPRECATION")
                val info = pm.getPackageInfo(pkgName, 0)
                // versionName 形如 "120.0.6099.43" → 取第一段主版本号
                @Suppress("DEPRECATION")
                info.versionName?.substringBefore('.')?.toIntOrNull() ?: 0
            } catch (_: Exception) {
                0
            }
        }

        override val totalMemoryBytes: Long by lazy {
            val am = context.getSystemService(Context.ACTIVITY_SERVICE) as? android.app.ActivityManager
            val mi = android.app.ActivityManager.MemoryInfo()
            am?.getMemoryInfo(mi)
            mi?.totalMem ?: 0L
        }
    }
}
