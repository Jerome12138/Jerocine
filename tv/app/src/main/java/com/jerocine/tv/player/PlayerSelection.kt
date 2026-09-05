package com.jerocine.tv.player

data class PlaybackTarget(
    val sourceIndex: Int,
    val episodeIndex: Int,
    val positionMs: Long,
)

object PlayerSelection {
    @JvmStatic
    fun switchSource(
        targetSourceIndex: Int,
        currentEpisode: Int,
        currentPositionMs: Long,
        targetEpisodeCount: Int,
    ): PlaybackTarget = PlaybackTarget(
        sourceIndex = targetSourceIndex,
        episodeIndex = currentEpisode.coerceIn(0, (targetEpisodeCount - 1).coerceAtLeast(0)),
        positionMs = currentPositionMs.coerceAtLeast(0),
    )

    @JvmStatic
    fun selectEpisode(
        sourceIndex: Int,
        episodeIndex: Int,
        episodeCount: Int,
    ): PlaybackTarget = PlaybackTarget(
        sourceIndex = sourceIndex,
        episodeIndex = episodeIndex.coerceIn(0, (episodeCount - 1).coerceAtLeast(0)),
        positionMs = 0,
    )
}
