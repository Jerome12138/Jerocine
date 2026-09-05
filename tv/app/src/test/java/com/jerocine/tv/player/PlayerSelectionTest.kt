package com.jerocine.tv.player

import org.junit.Assert.assertEquals
import org.junit.Test

class PlayerSelectionTest {
    @Test
    fun sourceSwitchClampsEpisodeAndPreservesPosition() {
        assertEquals(
            PlaybackTarget(sourceIndex = 1, episodeIndex = 2, positionMs = 45_000),
            PlayerSelection.switchSource(
                targetSourceIndex = 1,
                currentEpisode = 5,
                currentPositionMs = 45_000,
                targetEpisodeCount = 3,
            ),
        )
    }

    @Test
    fun episodeSelectionStartsAtZero() {
        assertEquals(
            PlaybackTarget(sourceIndex = 2, episodeIndex = 4, positionMs = 0),
            PlayerSelection.selectEpisode(
                sourceIndex = 2,
                episodeIndex = 4,
                episodeCount = 10,
            ),
        )
    }
}
