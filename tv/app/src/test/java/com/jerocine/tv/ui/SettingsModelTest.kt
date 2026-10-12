package com.jerocine.tv.ui

import org.junit.Assert.assertEquals
import org.junit.Test

class SettingsModelTest {
    // 服务器地址归一化的用例已随实现下沉到 player-core(见 ServerUrlTest)

    @Test
    fun labelsReduceMotionModes() {
        assertEquals("自动", reduceMotionLabel("auto"))
        assertEquals("减少", reduceMotionLabel("on"))
        assertEquals("完整", reduceMotionLabel("off"))
    }
}
