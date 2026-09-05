package com.jerocine.tv.player

import com.jerocine.tv.data.Episode
import com.jerocine.tv.data.FilmDetail
import com.jerocine.tv.data.PlayInfoResp
import com.jerocine.tv.data.PlaySource
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import org.junit.Assert.assertEquals
import org.junit.Test

class PlaybackCoordinatorTest {
    @Test
    fun preparesAllSourcesForRequestedEpisode() = runTest {
        var requested: Triple<Long, String?, Int>? = null
        val coordinator = PlaybackCoordinator { mid, source, episode ->
            requested = Triple(mid, source, episode)
            playInfo()
        }

        val payload = coordinator.prepare(
            mid = 42,
            source = "src_b",
            episode = 1,
            resumeSec = 12.5,
            skipIntroSec = 90,
            skipOutroSec = 60,
            proxyBase = "https://jerocine.art/api",
        )

        assertEquals(Triple(42L, "src_b", 1), requested)
        assertEquals("src_b", payload.currentSourceId)
        assertEquals(1, payload.startIndex)
        assertEquals(12_500, payload.resumeMs)
        assertEquals(2, Json.parseToJsonElement(payload.sourcesJson).jsonArray.size)
    }

    @Test
    fun rejectsPlaybackWithoutValidEpisodes() = runTest {
        val coordinator = PlaybackCoordinator { _, _, _ ->
            PlayInfoResp(detail = FilmDetail(mid = 42, name = "空影片"))
        }

        val error = runCatching {
            coordinator.prepare(42, "", 0, 0.0, 0, 0, "https://jerocine.art/api")
        }.exceptionOrNull()

        assertEquals(IllegalStateException::class.java, error?.javaClass)
        assertEquals("播放地址为空", error?.message)
    }

    private fun playInfo() = PlayInfoResp(
        detail = FilmDetail(
            mid = 42,
            name = "测试片",
            sources = listOf(
                PlaySource("src_a", "源 A", listOf(Episode("01", "https://cdn/a.m3u8"))),
                PlaySource(
                    "src_b",
                    "源 B",
                    listOf(
                        Episode("01", "https://cdn/b1.m3u8"),
                        Episode("02", "https://cdn/b2.m3u8"),
                    ),
                ),
            ),
        ),
        currentSource = "src_b",
        currentEpisode = 1,
    )
}
