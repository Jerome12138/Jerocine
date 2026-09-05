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
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

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
            proxyBase = "https://jerocine.art/api"
        )

        assertNotNull(payload)
        requireNotNull(payload)
        assertEquals("src_b", payload.currentSourceId)
        assertEquals(1, payload.startIndex)
        assertEquals(90_000L, payload.skipIntroMs)
        assertEquals(60_000L, payload.skipOutroMs)
        assertEquals("42", payload.filmId)
        assertEquals("测试片", payload.filmName)
        assertEquals("https://jerocine.art/api", payload.proxyBase)

        val sources = Json.parseToJsonElement(payload.sourcesJson).jsonArray
        assertEquals("src_a", sources[0].jsonObject["id"]?.jsonPrimitive?.content)
        assertEquals("https://cdn/a01.m3u8", sources[0].jsonObject["episodes"]?.jsonArray?.get(0)?.jsonObject?.get("url")?.jsonPrimitive?.content)
        assertEquals("src_b", sources[1].jsonObject["id"]?.jsonPrimitive?.content)
        assertEquals("测试片 · 02", sources[1].jsonObject["episodes"]?.jsonArray?.get(1)?.jsonObject?.get("title")?.jsonPrimitive?.content)
    }

    @Test
    fun playerWrapsM3u8WithProxyWhenAdFilterEnabled() {
        val url = PlayerActivity.buildPlayableUrl(
            0,
            "https://cdn.example.com/video/01.m3u8?token=a b",
            true,
            false,
            false,
            "https://jerocine.art/api/"
        )

        assertEquals(
            "https://jerocine.art/api/v1/m3u8/proxy?src=https%3A%2F%2Fcdn.example.com%2Fvideo%2F01.m3u8%3Ftoken%3Da+b&filterAds=1&proxyMedia=0",
            url
        )
    }

    @Test
    fun playerEnablesFullRelayOnlyForCompatibilityRetry() {
        val url = PlayerActivity.buildPlayableUrl(
            0,
            "https://cdn.example.com/video/01.m3u8",
            true,
            false,
            true,
            "http://jerocine.art/api"
        )

        assertTrue(url.endsWith("&filterAds=1&proxyMedia=1"))
    }

    @Test
    fun playerKeepsOriginalUrlWhenProxyFallbackIsForced() {
        val raw = "https://cdn.example.com/video/01.m3u8"

        assertEquals(
            raw,
            PlayerActivity.buildPlayableUrl(0, raw, true, true, false, "https://jerocine.art/api")
        )
    }

    @Test
    fun relayRetryRequiresAProxyManifestAndDirectCdnFailure() {
        val manifest = "http://jerocine.art/api/v1/m3u8/proxy?src=x&proxyMedia=0"

        assertTrue(
            PlayerActivity.shouldRetryWithRelay(
                manifest,
                "https://cdn.example.com/video/seg01.ts"
            )
        )
        assertFalse(PlayerActivity.shouldRetryWithRelay(manifest, manifest))
        assertFalse(
            PlayerActivity.shouldRetryWithRelay(
                manifest.replace("proxyMedia=0", "proxyMedia=1"),
                "https://cdn.example.com/video/seg01.ts"
            )
        )
        assertFalse(
            PlayerActivity.shouldRetryWithRelay(
                "https://cdn.example.com/video/index.m3u8",
                "https://cdn.example.com/video/seg01.ts"
            )
        )
    }

    @Test
    fun clientFilterSkipsManifestsAlreadyHandledByServerProxy() {
        assertFalse(
            PlayerActivity.needsClientSideFilter(
                "http://jerocine.art/api/v1/m3u8/proxy?src=x&proxyMedia=1"
            )
        )
        assertTrue(
            PlayerActivity.needsClientSideFilter(
                "https://cdn.example.com/video/index.m3u8"
            )
        )
    }
}
