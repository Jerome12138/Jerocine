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

fun deriveCategoryTvSections(input: ClassifyResp): List<CategoryTvSection> {
    val sections = mutableListOf(
        CategoryTvSection("news", "最新上线", "每日更新", "latest", input.news.take(18)),
        CategoryTvSection("top", "排行榜", "按热度排序", "hot", input.top.take(18)),
        CategoryTvSection("recent", "最近更新", "追更不迷路", "update_stamp", input.recent.take(18)),
    )
    // 高分榜: 仅该分类有豆瓣评分数据时显示(后端 scoredCount 运行时探测, 不做分类白名单)
    if (input.scoredCount > 0) {
        sections.add(
            CategoryTvSection("score", "高分榜", "豆瓣评分优先", "score", input.score.take(18))
        )
    }
    return sections.filter { it.items.isNotEmpty() }
}
