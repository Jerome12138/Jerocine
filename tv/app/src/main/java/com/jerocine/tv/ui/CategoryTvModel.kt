package com.jerocine.tv.ui

import com.jerocine.tv.data.Card
import com.jerocine.tv.data.ClassifyResp

data class CategoryTvSection(
    val key: String,
    val title: String,
    val subtitle: String,
    val sort: String,
    val items: List<Card>,
)

sealed interface CategoryRow {
    data class Header(
        val key: String,
        val title: String,
        val subtitle: String,
        val sort: String,
    ) : CategoryRow
    data class Poster(val sectionKey: String, val card: Card) : CategoryRow
}

fun flattenCategoryRows(sections: List<CategoryTvSection>): List<CategoryRow> = sections.flatMap { section ->
    listOf(CategoryRow.Header(section.key, section.title, section.subtitle, section.sort)) +
        section.items.map { CategoryRow.Poster(section.key, it) }
}

fun deriveCategoryTvSections(input: ClassifyResp): List<CategoryTvSection> =
    listOf(
        CategoryTvSection("news", "最新上映", "每日更新", "release_stamp", input.news.take(18)),
        CategoryTvSection("top", "排行榜", "按热度排序", "hits", input.top.take(18)),
        CategoryTvSection("recent", "最近更新", "追更不迷路", "update_stamp", input.recent.take(18)),
    ).filter { it.items.isNotEmpty() }
