package com.jerocine.tv.data

import org.junit.Assert.assertEquals
import org.junit.Test

class ServiceLocatorTest {

    @Test
    fun reduceMotionModeDefaultsToAutoBeforeInitialization() {
        assertEquals("auto", ServiceLocator.reduceMotionMode())
    }

    @Test
    fun productionApiStaysHttpsWhileMediaProxyUsesHttp() {
        ServiceLocator.setServer("https://jerocine.art")

        assertEquals("https://jerocine.art", ServiceLocator.serverBase)
        assertEquals("http://jerocine.art/api", ServiceLocator.proxyBase())
        assertEquals(
            "http://jerocine.art/api/v1/m3u8/proxy?src=https%3A%2F%2Fcdn.example%2F1.m3u8&filterAds=1&proxyMedia=0",
            ServiceLocator.m3u8ProxyUrl("https://cdn.example/1.m3u8")
        )
    }

    @Test
    fun customServerUsesItsOwnMediaProxy() {
        ServiceLocator.setServer("http://localhost:3600")

        assertEquals("http://localhost:3600/api", ServiceLocator.proxyBase())
    }
}
