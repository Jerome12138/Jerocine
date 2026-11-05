package com.jerocine.tv.ui

import com.jerocine.player.ui.EpisodeSegments
import com.jerocine.tv.data.PlaySource

data class DetailPlaybackSelection(
    val sourceId: String,
    val episodeIndex: Int,
    val segmentIndex: Int,
)

data class DetailEpisodeSegment(
    val start: Int,
    val end: Int,
    val label: String,
)

fun initialDetailPlaybackSelection(
    sources: List<PlaySource>,
    requestedSource: String,
    requestedEpisode: Int,
): DetailPlaybackSelection {
    val source = sources.firstOrNull { it.id == requestedSource } ?: sources.firstOrNull()
    val episode = requestedEpisode.coerceIn(0, (source?.episodes?.lastIndex ?: 0).coerceAtLeast(0))
    return DetailPlaybackSelection(
        sourceId = source?.id.orEmpty(),
        episodeIndex = episode,
        segmentIndex = EpisodeSegments.checkedSegment(episode),
    )
}

fun switchDetailSource(
    sources: List<PlaySource>,
    targetSourceId: String,
    currentEpisode: Int,
): DetailPlaybackSelection = initialDetailPlaybackSelection(
    sources = sources,
    requestedSource = targetSourceId,
    requestedEpisode = currentEpisode,
)

/**
 * 详情页选集分段(闭区间 [start,end])。分段数学唯一实现在 core {@link EpisodeSegments}
 * (与 Web 端"30 集一档"同源, 收编 tvshell 双写);本函数只负责 tvshell 的
 * 闭区间拆包与 "1-30" 标签拼装。>30 集才分段, ≤30 返回空(不分段)。
 */
fun detailEpisodeSegments(episodeCount: Int): List<DetailEpisodeSegment> {
    if (episodeCount <= EpisodeSegments.PER_SEG) return emptyList()
    return (0 until EpisodeSegments.count(episodeCount)).map { seg ->
        val start = EpisodeSegments.startOf(seg)
        val end = EpisodeSegments.endOf(seg, episodeCount)
        DetailEpisodeSegment(start, end - 1, "${start + 1}-$end")
    }
}

fun detailEpisodeLabel(filmName: String, episodeName: String, episodeIndex: Int): String {
    val name = episodeName.trim()
    if (name.isBlank()) return "第 ${episodeIndex + 1} 集"
    val film = filmName.trim()
    if (!name.startsWith(film) || film.isBlank()) return name
    return name.removePrefix(film)
        .replace(Regex("^[\\s·\\-_:：|/]+"), "")
        .trim()
        .ifBlank { name }
}
