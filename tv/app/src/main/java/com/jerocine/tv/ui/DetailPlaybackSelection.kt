package com.jerocine.tv.ui

import com.jerocine.tv.data.PlaySource

const val DETAIL_EPISODE_PAGE_SIZE = 30

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
        segmentIndex = episode / DETAIL_EPISODE_PAGE_SIZE,
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

fun detailEpisodeSegments(
    episodeCount: Int,
    pageSize: Int = DETAIL_EPISODE_PAGE_SIZE,
): List<DetailEpisodeSegment> {
    if (episodeCount <= pageSize || pageSize <= 0) return emptyList()
    return (0 until episodeCount step pageSize).map { start ->
        val end = minOf(start + pageSize, episodeCount) - 1
        DetailEpisodeSegment(start, end, "${start + 1}-${end + 1}")
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
