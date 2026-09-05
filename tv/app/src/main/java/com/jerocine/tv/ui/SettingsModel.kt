package com.jerocine.tv.ui

fun normalizeServerUrl(value: String): String = value.trim().trimEnd('/')

fun reduceMotionLabel(mode: String): String = when (mode) {
    "on" -> "减少"
    "off" -> "完整"
    else -> "自动"
}
