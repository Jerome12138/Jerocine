package com.jerocine.app

import android.content.Context
import android.content.SharedPreferences
import androidx.annotation.VisibleForTesting
import com.jerocine.player.data.ShellPrefs
import com.jerocine.player.ui.JcDeviceDiagnostics
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

    /**
     * 阈值常量 —— **单一来源在 core 的 [JcDeviceDiagnostics]**（设备诊断页要展示同一套判定），
     * 这里只做别名，防止"路由一套、诊断一套"的静默漂移。
     *
     * [MIN_SDK] 是真正的路由阈值（不再恒真）：整个包的 minSdk 已降到 21
     * （见 web/android/variables.gradle），API 21-23 的老设备**装得上**，
     * 但启动后会被本探针判为 NATIVE → 进原生壳。这正是"同一个包、低配也能安装
     * 并自动走原生"的设计要点；低配兜底由此阈值 + 内存/WebView 三条共同决定。
     */
    const val MIN_SDK = JcDeviceDiagnostics.MIN_SDK
    const val MIN_WEBVIEW_MAJOR = JcDeviceDiagnostics.MIN_WEBVIEW_MAJOR
    const val MIN_MEMORY_GB = JcDeviceDiagnostics.MIN_MEMORY_GB

    /** prefs 名与"壳模式"键 —— 契约只在 [ShellPrefs] 定义一次, 这里只做别名 */
    const val PREFS_NAME = ShellPrefs.NAME
    const val KEY_SHELL_MODE = ShellPrefs.KEY_SHELL_MODE

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

    /**
     * 从 SharedPreferences 读用户选择, 缺省 AUTO。
     *
     * 归一化比对(2026-10-12 修): 写入端(设置抽屉 {@code JcSettingsDrawer.cycleShellMode}
     * 与 WebView 不可恢复时的降级 {@code ServerNavigationWebViewClient})统一存**小写**
     * {@code "auto"/"web"/"native"}, 而本方法原先只认枚举大写名 {@code "WEB"/"NATIVE"},
     * 于是小写值全部落进 else 分支 → **用户在设置里选「Web」或「原生」被静默忽略**。
     * 现在两边都认(大小写不敏感), 兼容历史版本写入的大写值。
     */
    fun readUserMode(prefs: SharedPreferences): ShellMode {
        return when (prefs.getString(KEY_SHELL_MODE, "")?.trim()?.lowercase()) {
            "web" -> ShellMode.WEB
            "native" -> ShellMode.NATIVE
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
            // 与设备诊断页(**同一份**探测, 2026-10-12 修):
            // 原先这里自己反射 WebView.getCurrentWebViewPackageName() —— 该 API 已废弃,
            // 在 API 34+ 上取不到值, 被 catch 吞掉后恒返回 0 → AUTO 永远判原生壳。
            // 真机实测(API 37): 设备 WebView 实为 Chrome 150(远高于阈值 87), 却一直进原生壳。
            JcDeviceDiagnostics.webViewMajorOrZero(context)
        }

        override val totalMemoryBytes: Long by lazy {
            val am = context.getSystemService(Context.ACTIVITY_SERVICE) as? android.app.ActivityManager
            val mi = android.app.ActivityManager.MemoryInfo()
            am?.getMemoryInfo(mi)
            mi?.totalMem ?: 0L
        }
    }
}
