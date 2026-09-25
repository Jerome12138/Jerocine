package com.jerocine.tv.player

import com.jerocine.tv.data.Episode
import com.jerocine.tv.data.FilmDetail
import com.jerocine.tv.data.HistoryItem
import com.jerocine.tv.data.PlayInfoResp
import com.jerocine.tv.data.PlaySource
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * 播放地址 / 线路策略(PlayerUrls)的测试已随实现迁到 player-core 模块:
 *   tv/player-core/src/test/java/com/jerocine/player/PlayerUrlsTest.java
 * 这里只保留 TV 壳自己的 payload 组装与历史续播逻辑。
 */
class NativePlayerLauncherTest {
    @Test
    fun historyItemResumeSecondsUsesSavedProgress() {
        assertEquals(123.0, HistoryItem(progress = 123.4).resumeSeconds(), 0.0)
        assertEquals(0.0, HistoryItem(progress = -5.0).resumeSeconds(), 0.0)
    }

    @Test
    fun buildNativePlayerPayloadPreservesSourcesAndProxyBase() {
        val info = PlayInfoResp(
            detail = FilmDetail(
                mid = 42,
                name = "测试片",
                sources = listOf(
                    PlaySource(
                        id = "src_a",
                        name = "A",
                        episodes = listOf(Episode("01", "https://cdn/a01.m3u8"))
                    ),
                    PlaySource(
                        id = "src_b",
                        name = "B",
                        episodes = listOf(
                            Episode("01", "https://cdn/b01.m3u8"),
                            Episode("02", "https://cdn/b02.m3u8")
                        )
                    )
                )
            ),
            currentSource = "src_b",
            currentEpisode = 1
        )

        val payload = buildNativePlayerPayload(
            info = info,
            requestedSource = "src_b",
            requestedEpisode = 1,
            skipIntroSec = 90,
            skipOutroSec = 60,
            proxyBase = "https://example.com/api"
        )

        assertNotNull(payload)
        requireNotNull(payload)
        assertEquals("src_b", payload.currentSourceId)
        assertEquals(1, payload.startIndex)
        assertEquals(90_000L, payload.skipIntroMs)
        assertEquals(60_000L, payload.skipOutroMs)
        assertEquals("42", payload.filmId)
        assertEquals("测试片", payload.filmName)
        assertEquals("https://example.com/api", payload.proxyBase)

        val sources = Json.parseToJsonElement(payload.sourcesJson).jsonArray
        assertEquals("src_a", sources[0].jsonObject["id"]?.jsonPrimitive?.content)
        assertEquals("https://cdn/a01.m3u8", sources[0].jsonObject["episodes"]?.jsonArray?.get(0)?.jsonObject?.get("url")?.jsonPrimitive?.content)
        assertEquals("src_b", sources[1].jsonObject["id"]?.jsonPrimitive?.content)
        assertEquals("测试片 · 02", sources[1].jsonObject["episodes"]?.jsonArray?.get(1)?.jsonObject?.get("title")?.jsonPrimitive?.content)
    }

    /**
     * adFilterOk 只在"已知不可达"时透传, 未测(null)必须省略该键 —— 播放器据此区分
     * "服务端抓不到(跳过代理)" 与 "没测过(沿用走代理的旧行为)"。
     */
    @Test
    fun payloadCarriesAdFilterOkOnlyWhenKnown() {
        val info = PlayInfoResp(
            detail = FilmDetail(
                mid = 7,
                name = "测试片",
                sources = listOf(
                    PlaySource(
                        id = "src_bad",
                        name = "bad",
                        adFilterOk = false,
                        episodes = listOf(Episode("01", "https://cdn/bad.m3u8"))
                    ),
                    PlaySource(
                        id = "src_unknown",
                        name = "unknown",
                        episodes = listOf(Episode("01", "https://cdn/unk.m3u8"))
                    )
                )
            ),
            currentSource = "src_bad"
        )

        val payload = buildNativePlayerPayload(
            info = info,
            requestedSource = "src_bad",
            requestedEpisode = 0,
            skipIntroSec = 0,
            skipOutroSec = 0,
            proxyBase = "https://example.com/api"
        )

        requireNotNull(payload)
        val sources = Json.parseToJsonElement(payload.sourcesJson).jsonArray
        assertEquals("false", sources[0].jsonObject["adFilterOk"]?.jsonPrimitive?.content)
        assertNull(sources[1].jsonObject["adFilterOk"])
    }
}
