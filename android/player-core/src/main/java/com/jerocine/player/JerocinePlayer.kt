package com.jerocine.player

import android.content.Context
import android.content.Intent

/**
 * 播放器入口 — 壳层调用
 */
object JerocinePlayer {
    private var callback: PlayerEventCallback? = null

    fun start(context: Context, config: PlayerConfig) {
        val intent = Intent(context, PlayerActivity::class.java).apply {
            putExtra(PlayerActivity.EXTRA_SOURCES_JSON, config.sourcesJson)
            putExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID, config.currentSourceId)
            putExtra(PlayerActivity.EXTRA_START_INDEX, config.startIndex)
            putExtra(PlayerActivity.EXTRA_RESUME_MS, config.resumeMs)
            putExtra(PlayerActivity.EXTRA_SKIP_INTRO_MS, config.skipIntroMs)
            putExtra(PlayerActivity.EXTRA_SKIP_OUTRO_MS, config.skipOutroMs)
            putExtra(PlayerActivity.EXTRA_FILM_ID, config.filmId)
            putExtra(PlayerActivity.EXTRA_FILM_NAME, config.filmName)
            putExtra(PlayerActivity.EXTRA_PROXY_BASE, config.proxyBase)
            putExtra(PlayerActivity.EXTRA_CACHE_DIR, config.cacheDir.absolutePath)
        }
        context.startActivity(intent)
    }

    fun setCallback(callback: PlayerEventCallback?) {
        this.callback = callback
        PlayerActivity.sCallback = callback
    }

    fun stopCurrent() {
        PlayerActivity.stopRunningInstance()
    }

    fun setSpeed(speed: Float) {
        PlayerActivity.setSpeedOnRunningInstance(speed)
    }
}
