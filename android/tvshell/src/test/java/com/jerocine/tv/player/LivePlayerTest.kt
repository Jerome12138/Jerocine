package com.jerocine.tv.player

import com.jerocine.tv.data.LiveChannel
import com.jerocine.tv.data.LiveSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 直播 → 原生播放器 payload（换源=源面板，切频道=集面板） */
class LivePlayerTest {

    private fun ch(name: String, group: String = "", url: String = "http://s/$name.m3u8") =
        LiveChannel(name = name, group = group, url = url)

    @Test
    fun mapsLiveSourcesToPlayerSourcesAndChannelsToEpisodes() {
        val sources = listOf(LiveSource(1, "iptv-org"), LiveSource(2, "Free-TV"))
        val map = mapOf(
            1L to listOf(ch("CCTV1"), ch("CCTV2")),
            2L to listOf(ch("CNN")),
        )

        val p = buildLivePlayerPayload(sources, map, currentSourceId = 1, startChannelIndex = 1)!!

        assertTrue(p.sourcesJson.contains("\"id\":\"1\""))
        assertTrue(p.sourcesJson.contains("CCTV1"))
        assertTrue(p.sourcesJson.contains("iptv-org"))
        assertEquals("1", p.currentSourceId)
        assertEquals(1, p.startIndex)
        // 非数字 filmId：原生壳 handlePlayerEvent 以 toLongOrNull 兜底 → 直播不写影视历史
        assertEquals("", p.filmId)
        assertEquals(0L, p.resumeMs)
        assertEquals(0L, p.skipIntroMs)
    }

    @Test
    fun clampsStartIndexAndFallsBackToFirstSourceWhenCurrentMissing() {
        val sources = listOf(LiveSource(1, "A"), LiveSource(2, "B"))
        val map = mapOf(1L to listOf(ch("x")), 2L to listOf(ch("y1"), ch("y2")))

        val p = buildLivePlayerPayload(sources, map, currentSourceId = 999, startChannelIndex = 99)!!

        assertEquals("1", p.currentSourceId)
        assertEquals(0, p.startIndex)
    }

    @Test
    fun dropsSourcesWithoutPlayableChannels() {
        val sources = listOf(LiveSource(1, "A"), LiveSource(2, "B"))
        val map = mapOf(1L to listOf(ch("x", url = "")), 2L to listOf(ch("y")))

        val p = buildLivePlayerPayload(sources, map, currentSourceId = 1, startChannelIndex = 0)!!

        assertEquals("2", p.currentSourceId)
        assertTrue(p.sourcesJson.contains("\"id\":\"2\""))
        assertTrue(!p.sourcesJson.contains("\"id\":\"1\""))
    }

    @Test
    fun returnsNullWhenNoPlayableChannel() {
        assertNull(buildLivePlayerPayload(emptyList(), emptyMap(), currentSourceId = 0, startChannelIndex = 0))
        assertNull(
            buildLivePlayerPayload(
                sources = listOf(LiveSource(1, "A")),
                channelsBySource = mapOf(1L to listOf(ch("x", url = ""))),
                currentSourceId = 1,
                startChannelIndex = 0,
            )
        )
    }

    @Test
    fun blankChannelNameFallsBackToPlaceholder() {
        val p = buildLivePlayerPayload(
            sources = listOf(LiveSource(1, "A")),
            channelsBySource = mapOf(1L to listOf(LiveChannel(name = "", url = "http://s/u.m3u8"))),
            currentSourceId = 1,
            startChannelIndex = 0,
        )!!
        assertTrue(p.sourcesJson.contains("频道"))
    }
}
