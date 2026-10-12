package com.jerocine.tv.data

import org.junit.Assert.assertEquals
import org.junit.Test

class ServiceLocatorTest {

    @Test
    fun reduceMotionModeDefaultsToAutoBeforeInitialization() {
        assertEquals("auto", ServiceLocator.reduceMotionMode())
    }

    /**
     * 生产环境媒体/代理基址必须与业务 API **同源同协议(HTTPS)**。
     *
     * 2026-10-12 改: 此前生产媒体链刻意降级为 http(规避老设备 CA), 但 80→443 的 308 带
     * `Connection: close`, 连接无法复用 ⇒ 端侧过滤的每次 POST 都重付一次握手, 叠加后单次
     * 起播多等十几秒。老设备的 CA 由媒体 OkHttp 的 trustAll 兜住, 不靠降协议。
     */
    @Test
    fun productionMediaProxyStaysHttpsToAvoidRedirectHop() {
        ServiceLocator.setServer("https://jerocine.art")

        assertEquals("https://jerocine.art", ServiceLocator.serverBase)
        assertEquals("https://jerocine.art/api", ServiceLocator.proxyBase())
        assertEquals(
            "https://jerocine.art/api/v1/m3u8/proxy?src=https%3A%2F%2Fcdn.example%2F1.m3u8&filterAds=1&proxyMedia=0",
            ServiceLocator.m3u8ProxyUrl("https://cdn.example/1.m3u8")
        )
    }

    @Test
    fun customServerUsesItsOwnMediaProxy() {
        ServiceLocator.setServer("http://localhost:3600")

        assertEquals("http://localhost:3600/api", ServiceLocator.proxyBase())
    }

    /** 自定义 https 后端也必须原样保留 https(不得被"默认服务器"的特例改写协议)。 */
    @Test
    fun customHttpsServerKeepsHttpsForMediaProxy() {
        ServiceLocator.setServer("https://video.example.com")

        assertEquals("https://video.example.com/api", ServiceLocator.proxyBase())
    }
}
