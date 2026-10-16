package com.jerocine.app

import android.content.SharedPreferences
import com.jerocine.app.DeviceProbe.MIN_MEMORY_GB
import com.jerocine.app.DeviceProbe.MIN_SDK
import com.jerocine.app.DeviceProbe.MIN_WEBVIEW_MAJOR
import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Test

/**
 * DeviceProbe 纯逻辑单测。
 * 所有 Android API 调用都通过 DeviceCapabilities 接口注入假实现, 不需要 Robolectric / 真机 / Mockito。
 */
class DeviceProbeTest {

    /** 构造一个满足所有阈值的"好设备" */
    private fun goodCaps() = object : DeviceProbe.DeviceCapabilities {
        override val sdkInt = MIN_SDK + 5
        override val webViewMajorVersion = MIN_WEBVIEW_MAJOR + 10
        override val totalMemoryBytes = (MIN_MEMORY_GB + 2) * 1024 * 1024 * 1024
    }

    /** 假的 SharedPreferences: 只实现 getString, 其余抛异常(测试不会用到) */
    private fun fakePrefs(mode: String?): SharedPreferences = object : SharedPreferences {
        override fun getString(key: String?, defValue: String?): String? = mode
        override fun getAll(): MutableMap<String, *> = throw UnsupportedOperationException()
        override fun getStringSet(key: String?, defValues: MutableSet<String>?): MutableSet<String>? = throw UnsupportedOperationException()
        override fun getInt(key: String?, defValue: Int): Int = throw UnsupportedOperationException()
        override fun getLong(key: String?, defValue: Long): Long = throw UnsupportedOperationException()
        override fun getFloat(key: String?, defValue: Float): Float = throw UnsupportedOperationException()
        override fun getBoolean(key: String?, defValue: Boolean): Boolean = throw UnsupportedOperationException()
        override fun contains(key: String?): Boolean = throw UnsupportedOperationException()
        override fun edit(): SharedPreferences.Editor = throw UnsupportedOperationException()
        override fun registerOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) {}
        override fun unregisterOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) {}
    }

    // ---- probe() 阈值判定 ----

    @Test
    fun probe_allPass_returnsWeb() {
        assertSame(DeviceProbe.ShellTarget.WEB, DeviceProbe.probe(goodCaps()))
    }

    @Test
    fun probe_sdkBelow_returnsNative() {
        val caps = object : DeviceProbe.DeviceCapabilities by goodCaps() {
            override val sdkInt = MIN_SDK - 1
        }
        assertSame(DeviceProbe.ShellTarget.NATIVE, DeviceProbe.probe(caps))
    }

    @Test
    fun probe_webViewBelow_returnsNative() {
        val caps = object : DeviceProbe.DeviceCapabilities by goodCaps() {
            override val webViewMajorVersion = MIN_WEBVIEW_MAJOR - 1
        }
        assertSame(DeviceProbe.ShellTarget.NATIVE, DeviceProbe.probe(caps))
    }

    @Test
    fun probe_memoryBelow_returnsNative() {
        val caps = object : DeviceProbe.DeviceCapabilities by goodCaps() {
            override val totalMemoryBytes = (MIN_MEMORY_GB - 1) * 1024 * 1024 * 1024
        }
        assertSame(DeviceProbe.ShellTarget.NATIVE, DeviceProbe.probe(caps))
    }

    @Test
    fun probe_webViewZero_returnsNative() {
        val caps = object : DeviceProbe.DeviceCapabilities by goodCaps() {
            override val webViewMajorVersion = 0
        }
        assertSame(DeviceProbe.ShellTarget.NATIVE, DeviceProbe.probe(caps))
    }

    @Test
    fun probe_boundaryExactly_returnsWeb() {
        val caps = object : DeviceProbe.DeviceCapabilities {
            override val sdkInt = MIN_SDK
            override val webViewMajorVersion = MIN_WEBVIEW_MAJOR
            override val totalMemoryBytes = MIN_MEMORY_GB * 1024 * 1024 * 1024
        }
        assertSame(DeviceProbe.ShellTarget.WEB, DeviceProbe.probe(caps))
    }

    // ---- readUserMode() ----
    // 写入端(JcSettingsDrawer / ServerNavigationWebViewClient)统一存**小写**,
    // 历史版本存过枚举大写名 —— 两边都要认(2026-10-12 修真: 原实现只认大写,
    // 导致用户在设置里选「Web」/「原生」被静默忽略, 全部退化为 AUTO)。

    @Test
    fun readUserMode_default_returnsAuto() {
        assertSame(DeviceProbe.ShellMode.AUTO, DeviceProbe.readUserMode(fakePrefs(null)))
    }

    @Test
    fun readUserMode_lowercaseWeb_returnsWeb() {
        assertSame(DeviceProbe.ShellMode.WEB, DeviceProbe.readUserMode(fakePrefs("web")))
    }

    @Test
    fun readUserMode_lowercaseNative_returnsNative() {
        assertSame(DeviceProbe.ShellMode.NATIVE, DeviceProbe.readUserMode(fakePrefs("native")))
    }

    @Test
    fun readUserMode_lowercaseAuto_returnsAuto() {
        assertSame(DeviceProbe.ShellMode.AUTO, DeviceProbe.readUserMode(fakePrefs("auto")))
    }

    @Test
    fun readUserMode_legacyUppercase_isStillAccepted() {
        assertSame(DeviceProbe.ShellMode.WEB, DeviceProbe.readUserMode(fakePrefs("WEB")))
        assertSame(DeviceProbe.ShellMode.NATIVE, DeviceProbe.readUserMode(fakePrefs("NATIVE")))
    }

    @Test
    fun readUserMode_mixedCaseAndPadding_isNormalized() {
        assertSame(DeviceProbe.ShellMode.WEB, DeviceProbe.readUserMode(fakePrefs(" Web ")))
        assertSame(DeviceProbe.ShellMode.NATIVE, DeviceProbe.readUserMode(fakePrefs("Native")))
    }

    @Test
    fun readUserMode_garbage_returnsAuto() {
        assertSame(DeviceProbe.ShellMode.AUTO, DeviceProbe.readUserMode(fakePrefs("invalid_value")))
    }

    @Test
    fun readUserMode_emptyString_returnsAuto() {
        assertSame(DeviceProbe.ShellMode.AUTO, DeviceProbe.readUserMode(fakePrefs("")))
    }

    /** 写入端写什么, readUserMode 就得读回什么 —— 防"写了 web 却仍判 AUTO"的契约漂移。 */
    @Test
    fun readUserMode_roundTripsWithWriterValues() {
        // JcSettingsDrawer.nextShellMode 产生的值域 + ServerNavigationWebViewClient 的降级值
        listOf("auto", "web", "native").forEach { written ->
            val mode = DeviceProbe.readUserMode(fakePrefs(written))
            val back = when (mode) {
                DeviceProbe.ShellMode.WEB -> "web"
                DeviceProbe.ShellMode.NATIVE -> "native"
                DeviceProbe.ShellMode.AUTO -> "auto"
            }
            assertEquals(written, back)
        }
    }

    // ---- resolveTarget() 用户选择优先 ----

    @Test
    fun resolveTarget_webForced_returnsWebEvenOnBadDevice() {
        val badCaps = object : DeviceProbe.DeviceCapabilities {
            override val sdkInt = 21
            override val webViewMajorVersion = 0
            override val totalMemoryBytes = 512L * 1024 * 1024
        }
        assertSame(DeviceProbe.ShellTarget.WEB,
            DeviceProbe.resolveTarget(DeviceProbe.ShellMode.WEB, badCaps))
    }

    @Test
    fun resolveTarget_nativeForced_returnsNativeEvenOnGoodDevice() {
        assertSame(DeviceProbe.ShellTarget.NATIVE,
            DeviceProbe.resolveTarget(DeviceProbe.ShellMode.NATIVE, goodCaps()))
    }

    @Test
    fun resolveTarget_auto_delegatesToProbe() {
        assertSame(DeviceProbe.ShellTarget.WEB,
            DeviceProbe.resolveTarget(DeviceProbe.ShellMode.AUTO, goodCaps()))
        val badCaps = object : DeviceProbe.DeviceCapabilities by goodCaps() {
            override val sdkInt = MIN_SDK - 1
        }
        assertSame(DeviceProbe.ShellTarget.NATIVE,
            DeviceProbe.resolveTarget(DeviceProbe.ShellMode.AUTO, badCaps))
    }

    @Test
    fun thresholdConstants_areExpectedValues() {
        assertEquals(24, MIN_SDK)
        assertEquals(61, MIN_WEBVIEW_MAJOR)
        assertEquals(2L, MIN_MEMORY_GB)
    }
}
