package com.jerocine.tv.ui

import org.junit.Assert.assertEquals
import org.junit.Test

class SettingsModelTest {
    @Test
    fun normalizesServerWithoutTrailingSlash() {
        assertEquals("https://example.com", normalizeServerUrl(" https://example.com/// "))
    }

    @Test
    fun labelsReduceMotionModes() {
        assertEquals("自动", reduceMotionLabel("auto"))
        assertEquals("减少", reduceMotionLabel("on"))
        assertEquals("完整", reduceMotionLabel("off"))
    }
}
