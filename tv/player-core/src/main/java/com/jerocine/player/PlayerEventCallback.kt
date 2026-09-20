package com.jerocine.player

import org.json.JSONObject

/**
 * 播放器事件回调 — 壳层实现
 */
interface PlayerEventCallback {
    fun onEvent(name: String, payload: JSONObject)
}
