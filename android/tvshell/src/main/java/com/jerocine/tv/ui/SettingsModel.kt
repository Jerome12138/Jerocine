package com.jerocine.tv.ui

// 服务器地址归一化已下沉 core: com.jerocine.player.data.ServerUrl.normalize
// (原 normalizeServerUrl 与 web 壳 ServerNavigationPolicy.hostOf 语义相邻、各写一份 → 合并;
//  单测见 player-core 的 ServerUrlTest)

fun reduceMotionLabel(mode: String): String = when (mode) {
    "on" -> "减少"
    "off" -> "完整"
    else -> "自动"
}
