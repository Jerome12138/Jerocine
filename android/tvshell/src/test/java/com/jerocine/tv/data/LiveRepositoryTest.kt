package com.jerocine.tv.data

import kotlinx.coroutines.delay
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

/**
 * Q⑥ 直播频道并行拉取（语义对齐 web nativeLive.ts 的 Promise.all）:
 *  ① 并行启动 —— 各源频道请求同时发出（总耗时 ≈ 最慢单源，不串行累加）;
 *  ② 顺序稳定 —— 返回 map 的键序 = sources 顺序（与串行版一致）;
 *  ③ 单源失败不阻断 —— 失败源 = 空列表，其余照常（对齐 web 的 per-source try/catch）。
 */
class LiveRepositoryTest {

    private class LiveApi(
        private val failId: Long? = null,
        private val overlapSeen: AtomicBoolean? = null,
    ) : StubJerocineApi() {
        private val inFlight = AtomicInteger(0)

        override suspend fun liveSources(): LiveSourcesResp = LiveSourcesResp(
            sources = listOf(LiveSource(1, "A"), LiveSource(2, "B"), LiveSource(3, "C")),
        )

        override suspend fun liveChannels(id: Long): LiveChannelsResp {
            // 挂起让出调度 → 若实现是串行 for-await，第二个源永远看不到 inFlight>1
            if (inFlight.incrementAndGet() > 1) overlapSeen?.set(true)
            delay(10)
            inFlight.decrementAndGet()
            if (id == failId) throw IllegalStateException("boom")
            return LiveChannelsResp(id = id, channels = listOf(LiveChannel(name = "c$id", url = "http://s/$id")))
        }
    }

    @Test
    fun loadAllFetchesChannelsConcurrently() = runTest {
        val overlapSeen = AtomicBoolean(false)
        val repo = LiveRepository(LiveApi(overlapSeen = overlapSeen))

        repo.loadAll()

        assertTrue("loadAll 必须并行拉各源频道（对齐 web Promise.all）", overlapSeen.get())
    }

    @Test
    fun loadAllKeepsSourceOrderInChannelMap() = runTest {
        val repo = LiveRepository(LiveApi())

        val (sources, channels) = repo.loadAll()

        assertEquals(listOf(1L, 2L, 3L), sources.map { it.id })
        assertEquals(listOf(1L, 2L, 3L), channels.keys.toList())
        assertEquals("c1", channels[1L]!!.single().name)
        assertEquals("c2", channels[2L]!!.single().name)
        assertEquals("c3", channels[3L]!!.single().name)
    }

    @Test
    fun loadAllIsolatesSingleSourceFailure() = runTest {
        val repo = LiveRepository(LiveApi(failId = 2L))

        val (sources, channels) = repo.loadAll()

        assertEquals(listOf(1L, 2L, 3L), sources.map { it.id })
        assertEquals(1, channels[1L]!!.size)
        assertTrue("失败源 = 空列表（不阻断其它源）", channels[2L]!!.isEmpty())
        assertEquals(1, channels[3L]!!.size)
    }
}

/** 全空 JerocineApi：只给本文件测试用的基类，用到的按需 override。 */
private abstract class StubJerocineApi : JerocineApi {
    override suspend fun home(): HomeResp = HomeResp()
    override suspend fun categories(): List<NavCategory> = emptyList()
    override suspend fun films(
        keyword: String?,
        pid: Long?,
        category: Long?,
        plot: String?,
        area: String?,
        language: String?,
        year: Int?,
        sort: String?,
        current: Int,
        size: Int
    ): CardPage = CardPage()
    override suspend fun filters(pid: Long): FilmFilters = FilmFilters()
    override suspend fun classify(pid: Long): ClassifyResp = ClassifyResp()
    override suspend fun filmDetail(mid: Long): FilmDetailResp = FilmDetailResp()
    override suspend fun playInfo(mid: Long, source: String?, episode: Int): PlayInfoResp = PlayInfoResp()
    override suspend fun deviceCode(): DeviceCodeResp = DeviceCodeResp()
    override suspend fun devicePoll(body: Map<String, String>): DevicePollResp = DevicePollResp()
    override suspend fun login(body: LoginReq): LoginResp = LoginResp(userName = "", token = "")
    override suspend fun latestVersion(channel: Int): AppVersion = AppVersion()
    override suspend fun liveSources(): LiveSourcesResp = LiveSourcesResp()
    override suspend fun liveChannels(id: Long): LiveChannelsResp = LiveChannelsResp()
}
