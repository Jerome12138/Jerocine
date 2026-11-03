package com.jerocine.tv.data

/** 直播数据仓库：源列表 + 单源频道（/api/v1 无信封；错误经 call 转 ApiException） */
class LiveRepository(private val api: JerocineApi) {

    suspend fun sources(): List<LiveSource> = call { api.liveSources() }.sources

    suspend fun channels(sourceId: Long): List<LiveChannel> =
        call { api.liveChannels(sourceId) }.channels

    /**
     * 拉取「全部源 + 各自频道」，供一次启动原生播放器即带上多源多频道
     * （播放器内换源 = 切直播源，切集 = 切频道）。
     * 单源失败不阻断其它源（返回空列表），避免一个源挂掉整个直播不可用。
     */
    suspend fun loadAll(): Pair<List<LiveSource>, Map<Long, List<LiveChannel>>> {
        val sources = sources()
        val map = linkedMapOf<Long, List<LiveChannel>>()
        for (s in sources) {
            map[s.id] = runCatching { channels(s.id) }.getOrDefault(emptyList())
        }
        return sources to map
    }
}
