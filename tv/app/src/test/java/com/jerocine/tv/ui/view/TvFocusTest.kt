package com.jerocine.tv.ui.view

import org.junit.Assert.assertEquals
import org.junit.Test

class TvFocusTest {
    @Test
    fun fullMotionUsesApprovedScaleAndDuration() {
        assertEquals(FocusMotion(1.04f, 120L), focusMotion("off"))
        assertEquals(FocusMotion(1.04f, 120L), focusMotion("auto"))
    }

    @Test
    fun reducedMotionDisablesScaleAnimation() {
        assertEquals(FocusMotion(1f, 0L), focusMotion("on"))
    }
}
