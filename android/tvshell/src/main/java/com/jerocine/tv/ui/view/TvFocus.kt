package com.jerocine.tv.ui.view

import android.view.View

data class FocusMotion(val scale: Float, val durationMs: Long)

fun focusMotion(mode: String): FocusMotion =
    if (mode == "on") FocusMotion(1f, 0L) else FocusMotion(1.04f, 120L)

fun View.installTvFocusAnimation(modeProvider: () -> String) {
    setOnFocusChangeListener { target, focused ->
        val motion = focusMotion(modeProvider())
        target.animate().cancel()
        target.animate()
            .scaleX(if (focused) motion.scale else 1f)
            .scaleY(if (focused) motion.scale else 1f)
            .setDuration(motion.durationMs)
            .start()
        target.translationZ = if (focused) 8f else 0f
    }
}

fun View.resetTvFocusAnimation() {
    animate().cancel()
    scaleX = 1f
    scaleY = 1f
    translationZ = 0f
}

fun View.revealTvContent(modeProvider: () -> String) {
    if (modeProvider() == "on") {
        alpha = 1f
        return
    }
    animate().cancel()
    alpha = 0f
    animate().alpha(1f).setDuration(150L).start()
}
