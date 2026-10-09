package com.jerocine.tv.ui

import com.jerocine.tv.data.Card
import com.jerocine.tv.data.ClassifyResp
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class CategoryTvModelTest {
    @Test
    fun flattensHeadersBeforeTheirCards() {
        val card = Card(mid = 7, name = "影片")
        val rows = flattenCategoryRows(
            listOf(CategoryTvSection("news", "最新上线", "每日更新", "latest", listOf(card)))
        )

        assertTrue(rows[0] is CategoryRow.Header)
        assertEquals(card, (rows[1] as CategoryRow.Poster).card)
    }

    @Test
    fun derivesWebTvCategorySectionsAndBoundsItems() {
        val sections = deriveCategoryTvSections(
            ClassifyResp(
                news = cards("news", 20),
                top = cards("top", 2),
                recent = emptyList(),
                score = emptyList(),
                scoredCount = 0,
            )
        )

        // 模块顺序对齐 web ClassifyView(用户定稿): 排行榜最前, 之后 最新上线 / 最近更新 / 高分榜
        assertEquals(listOf("top", "news"), sections.map { it.key })
        assertEquals(listOf("排行榜", "最新上线"), sections.map { it.title })
        assertEquals(listOf("按热度排序", "每日更新"), sections.map { it.subtitle })
        assertEquals(listOf("hot", "latest"), sections.map { it.sort })
        // 空分区(此处 recent)被过滤掉, 每段上限 18
        assertEquals(2, sections.first().items.size)
        assertEquals(18, sections.last().items.size)
    }

    @Test
    fun addsScoreSectionOnlyWhenCategoryHasScoredItems() {
        val withScore = deriveCategoryTvSections(
            ClassifyResp(
                news = cards("news", 3),
                top = cards("top", 3),
                recent = cards("recent", 3),
                score = cards("score", 20),
                scoredCount = 500,
            )
        )
        assertEquals(listOf("top", "news", "recent", "score"), withScore.map { it.key })
        assertEquals(listOf("hot", "latest", "update_stamp", "score"), withScore.map { it.sort })
        assertEquals(listOf("排行榜", "最新上线", "最近更新", "高分榜"), withScore.map { it.title })
        assertEquals(
            listOf("按热度排序", "每日更新", "追更不迷路", "豆瓣评分优先"),
            withScore.map { it.subtitle },
        )
        assertEquals(18, withScore.last().items.size)

        // scoredCount=0(如体育/短剧等无豆瓣分分类) → 不出现高分榜
        val withoutScore = deriveCategoryTvSections(
            ClassifyResp(
                news = cards("news", 3),
                top = cards("top", 3),
                recent = cards("recent", 3),
                score = emptyList(),
                scoredCount = 0,
            )
        )
        assertEquals(listOf("top", "news", "recent"), withoutScore.map { it.key })
        // score 有数据但 scoredCount=0 → 仍不显示(以 scoredCount 为唯一闸门, 不做分类白名单)
        val scoreDataButZeroCount = deriveCategoryTvSections(
            ClassifyResp(
                news = cards("news", 3),
                top = cards("top", 3),
                recent = cards("recent", 3),
                score = cards("score", 20),
                scoredCount = 0,
            )
        )
        assertEquals(listOf("top", "news", "recent"), scoreDataButZeroCount.map { it.key })
    }

    private fun cards(prefix: String, count: Int): List<Card> =
        (1..count).map { Card(mid = it.toLong(), name = "$prefix$it") }
}
