package com.jerocine.tv

class DoubleBackExitGate(
    private val intervalMs: Long = 2_000L,
) {
    private var firstPressAt: Long? = null

    fun shouldExit(nowMs: Long): Boolean {
        val previous = firstPressAt
        if (previous != null && nowMs - previous in 0..intervalMs) {
            firstPressAt = null
            return true
        }
        firstPressAt = nowMs
        return false
    }

    fun reset() {
        firstPressAt = null
    }
}
