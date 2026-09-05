package com.jerocine.tv.ui

import com.jerocine.tv.data.FilmDetail

data class DetailTvMeta(val label: String, val value: String)

data class DetailTvModel(
    val chips: List<String>,
    val meta: List<DetailTvMeta>,
    val summary: String,
)

fun deriveDetailTvModel(detail: FilmDetail): DetailTvModel {
    val chips = buildList {
        if (detail.year > 0) add(detail.year.toString())
        if (detail.cName.isNotBlank()) add(detail.cName)
        if (detail.area.isNotBlank()) add(detail.area)
        splitDetailNames(detail.classTag, max = 6).forEach { add(it) }
    }.distinct().take(6)

    val meta = buildList {
        detail.remarks.takeIf { it.isNotBlank() }?.let { add(DetailTvMeta("状态", it)) }
        detail.language.takeIf { it.isNotBlank() }?.let { add(DetailTvMeta("语言", it)) }
        splitDetailNames(detail.director, max = 6).takeIf { it.isNotEmpty() }?.let {
            add(DetailTvMeta("导演", it.joinToString(" / ")))
        }
    }

    return DetailTvModel(
        chips = chips,
        meta = meta,
        summary = detail.content.cleanDetailSummary(),
    )
}

private fun splitDetailNames(raw: String, max: Int): List<String> =
    raw.split(Regex("[,，、/\\s]+"))
        .map { it.trim() }
        .filter { it.isNotEmpty() }
        .take(max)

private fun String.cleanDetailSummary(): String =
    replace(Regex("<[^>]*>"), " ")
        .replace("&nbsp;", " ")
        .replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace(Regex("\\s+"), " ")
        .trim()
