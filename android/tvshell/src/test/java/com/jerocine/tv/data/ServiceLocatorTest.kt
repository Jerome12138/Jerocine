package com.jerocine.tv.data

import org.junit.Assert.assertEquals
import org.junit.Test

class ServiceLocatorTest {

    @Test
    fun reduceMotionModeDefaultsToAutoBeforeInitialization() {
        assertEquals("auto", ServiceLocator.reduceMotionMode())
    }

    /**
     * 生产环境业务基址必须与 API **同源同协议(HTTPS)**。
     *
     * 2026-10-12 改: 此前生产媒体链刻意降级为 http(规避老设备 CA), 但 80→443 的 308 带
     * `Connection: close`, 连接无法复用 ⇒ 每次请求都重付一次握手。老设备的 CA 由媒体
     * OkHttp 的 trustAll 兜住, 不靠降协议。
     */
    @Test
    fun productionServerStaysHttpsToAvoidRedirectHop() {
        ServiceLocator.setServer("https://jerocine.art")

        assertEquals("https://jerocine.art", ServiceLocator.serverBase)
    }

    @Test
    fun customServerKeepsItsOwnBase() {
        ServiceLocator.setServer("http://localhost:3600")

        assertEquals("http://localhost:3600", ServiceLocator.serverBase)
    }

    /** 自定义 https 后端也必须原样保留 https(不得被"默认服务器"的特例改写协议)。 */
    @Test
    fun customHttpsServerKeepsHttps() {
        ServiceLocator.setServer("https://video.example.com")

        assertEquals("https://video.example.com", ServiceLocator.serverBase)
    }
}
