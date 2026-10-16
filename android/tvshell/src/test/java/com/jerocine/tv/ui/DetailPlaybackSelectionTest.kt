package com.jerocine.tv.ui

import com.jerocine.tv.data.Episode
import com.jerocine.tv.data.PlaySource
import org.junit.Assert.assertEquals
import org.junit.Test

class DetailPlaybackSelectionTest {
    private val sources = listOf(
        PlaySource("a", "线路 A", List(45) { Episode("剧名 第${it + 1}集", "a-$it") }),
        PlaySource("b", "线路 B", List(3) { Episode("第${it + 1}集", "b-$it") }),
    )

    @Test
    fun restoresKnownSourceAndClampsEpisode() {
        assertEquals(
            DetailPlaybackSelection("b", 2, 0),
            initialDetailPlaybackSelection(sources, "b", 8),
        )
        assertEquals(
            DetailPlaybackSelection("a", 0, 0),
            initialDetailPlaybackSelection(sources, "missing", 0),
        )
    }

    @Test
    fun sourceSwitchKeepsEpisodeWithinNewSource() {
        assertEquals(
            DetailPlaybackSelection("b", 2, 0),
            switchDetailSource(sources, "b", currentEpisode = 31),
        )
    }

    @Test
    fun buildsThirtyEpisodeSegmentsAndCleansLabels() {
        assertEquals(
            listOf(
                DetailEpisodeSegment(0, 29, "1-30"),
                DetailEpisodeSegment(30, 44, "31-45"),
            ),
            detailEpisodeSegments(45),
        )
        assertEquals("第12集", detailEpisodeLabel("剧名", "剧名 - 第12集", 11))
        assertEquals("第 4 集", detailEpisodeLabel("剧名", "", 3))
    }
}
