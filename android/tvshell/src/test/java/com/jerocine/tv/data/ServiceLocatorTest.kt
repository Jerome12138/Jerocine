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

    // ---------------------------------------------------------------- 接口根注入(2026-10-29 解耦)

    /** 注入接口根(含 /api)后, 生效接口根 = 注入值 + /v1 —— 形状与前端 VITE_API_BASE + '/v1' 对齐 */
    @Test
    fun injectedApiBaseProducesApiV1Root() {
        ServiceLocator.applyInjectedApiBase("https://api.jerocine.art/api")

        assertEquals("https://api.jerocine.art/api/v1", ServiceLocator.effectiveApiBase)
    }

    /** 注入值带尾斜杠也归一; 换站点地址不得带漂接口根(解耦语义的核心断言) */
    @Test
    fun injectedApiBaseIsImmuneToSetServer() {
        ServiceLocator.applyInjectedApiBase("https://api.example.com/api/")
        ServiceLocator.setServer("http://localhost:3600")

        assertEquals("https://api.example.com/api/v1", ServiceLocator.effectiveApiBase)
    }

    /** 空白注入 = 未注入: 回退 serverBase 旧行为(公开仓自构建连接自己部署的后端) */
    @Test
    fun blankInjectionFallsBackToServerBase() {
        ServiceLocator.applyInjectedApiBase("   ")
        ServiceLocator.setServer("https://video.example.com")

        assertEquals("https://video.example.com/api/v1", ServiceLocator.effectiveApiBase)
    }

    /** 未注入 + 站点带尾斜杠: 回退路径同样归一, Retrofit baseUrl 拼接不出双斜杠 */
    @Test
    fun fallbackTrimsTrailingSlash() {
        ServiceLocator.applyInjectedApiBase(null)
        ServiceLocator.setServer("https://jerocine.art/")

        assertEquals("https://jerocine.art/api/v1", ServiceLocator.effectiveApiBase)
    }

    /** 注入值不含 /api 的自建后端形态(裸根)同样成立: /v1 直接挂在根下 */
    @Test
    fun injectedBareRootGetsV1Suffix() {
        ServiceLocator.applyInjectedApiBase("http://192.168.1.9:8080")

        assertEquals("http://192.168.1.9:8080/v1", ServiceLocator.effectiveApiBase)
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
