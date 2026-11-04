package com.jerocine.tv.data

import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope

/** 直播数据仓库：源列表 + 单源频道（/api/v1 无信封；错误经 call 转 ApiException） */
class LiveRepository(private val api: JerocineApi) {

    suspend fun sources(): List<LiveSource> = call { api.liveSources() }.sources

    suspend fun channels(sourceId: Long): List<LiveChannel> =
        call { api.liveChannels(sourceId) }.channels

    /**
     * 拉取「全部源 + 各自频道」，供一次启动原生播放器即带上多源多频道
     * （播放器内换源 = 切直播源，切集 = 切频道）。
     *
     * Q⑥: 各源频道**并行**拉取（对齐 web nativeLive.ts 的 Promise.all 语义）——
     * 服务端冷缓存替抓上游最坏 ~8s/源，串行相乘是「直播进入慢」的大头。
     * 返回 map 键序 = sources 顺序（toMap 保序），与串行版一致。
     * 单源失败不阻断其它源（runCatching → 空列表，同 web 的 per-source try/catch），
     * 故 awaitAll 不会因单源异常提前失败。
     */
    suspend fun loadAll(): Pair<List<LiveSource>, Map<Long, List<LiveChannel>>> {
        val sources = sources()
        val map = coroutineScope {
            sources.map { s ->
                async { s.id to runCatching { channels(s.id) }.getOrDefault(emptyList()) }
            }.awaitAll()
        }.toMap()
        return sources to map
    }
}
