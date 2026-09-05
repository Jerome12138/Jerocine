package com.jerocine.tv

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DoubleBackExitGateTest {
    @Test
    fun exitsOnlyOnSecondPressWithinWindow() {
        val gate = DoubleBackExitGate(intervalMs = 2_000)

        assertFalse(gate.shouldExit(1_000))
        assertTrue(gate.shouldExit(2_500))
        assertFalse(gate.shouldExit(2_600))
        assertFalse(gate.shouldExit(5_000))
    }

    @Test
    fun resetStartsANewWindow() {
        val gate = DoubleBackExitGate()

        assertFalse(gate.shouldExit(1_000))
        gate.reset()
        assertFalse(gate.shouldExit(1_500))
    }
}
