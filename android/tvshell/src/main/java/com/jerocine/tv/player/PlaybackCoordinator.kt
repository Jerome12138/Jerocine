package com.jerocine.tv.player

import com.jerocine.tv.data.PlayInfoResp

fun interface PlayInfoLoader {
    suspend fun load(mid: Long, source: String?, episode: Int): PlayInfoResp
}

class PlaybackCoordinator(
    private val playInfoLoader: PlayInfoLoader,
) {
    suspend fun prepare(
        mid: Long,
        source: String,
        episode: Int,
        resumeSec: Double,
        skipIntroSec: Int,
        skipOutroSec: Int,
        proxyBase: String,
    ): NativePlayerPayload {
        val info = playInfoLoader.load(mid, source.ifBlank { null }, episode)
        return buildNativePlayerPayload(
            info = info,
            requestedSource = source.ifBlank { info.currentSource },
            requestedEpisode = episode,
            skipIntroSec = skipIntroSec,
            skipOutroSec = skipOutroSec,
            proxyBase = proxyBase,
            resumeSec = resumeSec,
        ) ?: error("播放地址为空")
    }
}
