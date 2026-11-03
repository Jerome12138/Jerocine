package com.jerocine.tv.player

import com.jerocine.tv.data.LiveChannel
import com.jerocine.tv.data.LiveSource
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

/**
 * 直播 → 原生播放器 payload 构建。
 *
 * 映射：**直播源 = 播放器的「源」，频道 = 播放器的「集」** —— 这样进入原生播放器后，
 * 现成的「换源」面板 = 换直播源、「选集」面板 = 切频道（与影片换源/切集同一套 UI）。
 *
 * 契约与 WebShell 的 playPlaylist 一致：`[{id, name, episodes:[{url, title}]}]`
 * （见 player-core PlayerSourceHelper.parseSources）。
 */
@Serializable
private data class LivePlayerSource(
    val id: String,
    val name: String,
    val episodes: List<LivePlayerEpisode>,
)

@Serializable
private data class LivePlayerEpisode(
    val url: String,
    val title: String,
)

/**
 * @param sources 全部直播源（顺序即播放器里源的顺序）
 * @param channelsBySource sourceId → 该源频道列表（顺序即播放器里集的顺序，也是 startChannelIndex 的依据）
 * @param currentSourceId 当前源 id（未命中则取第一个）
 * @param startChannelIndex 当前频道在该源频道列表中的下标（越界自动钳制）
 * @param filmName 播放器标题（直播固定文案 / 可传「频道名 · 源名」）
 * @return null 表示没有任何可用频道（调用方应提示，不启动播放器）
 */
fun buildLivePlayerPayload(
    sources: List<LiveSource>,
    channelsBySource: Map<Long, List<LiveChannel>>,
    currentSourceId: Long,
    startChannelIndex: Int,
    filmName: String = "直播",
): NativePlayerPayload? {
    val mapped = sources.mapNotNull { source ->
        val episodes = (channelsBySource[source.id] ?: emptyList())
            .filter { it.url.isNotBlank() }
            .map { ch -> LivePlayerEpisode(url = ch.url, title = ch.name.ifBlank { "频道" }) }
        if (episodes.isEmpty()) null else LivePlayerSource(id = source.id.toString(), name = source.name, episodes = episodes)
    }
    if (mapped.isEmpty()) return null

    val selected = mapped.firstOrNull { it.id == currentSourceId.toString() } ?: mapped.first()
    val startIndex = startChannelIndex.coerceIn(0, selected.episodes.lastIndex)

    return NativePlayerPayload(
        sourcesJson = Json.encodeToString(mapped),
        currentSourceId = selected.id,
        startIndex = startIndex,
        resumeMs = 0L,
        skipIntroMs = 0L,
        skipOutroMs = 0L,
        // 非数字 filmId：TV 壳 handlePlayerEvent 以 toLongOrNull 兜底 → 直播不会写入影视历史
        filmId = "",
        filmName = filmName,
    )
}
