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

        assertEquals(listOf("news", "top"), sections.map { it.key })
        assertEquals(listOf("最新上线", "排行榜"), sections.map { it.title })
        assertEquals(listOf("每日更新", "按热度排序"), sections.map { it.subtitle })
        assertEquals(listOf("latest", "hot"), sections.map { it.sort })
        assertEquals(18, sections.first().items.size)
        assertEquals(2, sections.last().items.size)
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
        assertEquals(listOf("news", "top", "recent", "score"), withScore.map { it.key })
        assertEquals(listOf("latest", "hot", "update_stamp", "score"), withScore.map { it.sort })
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
        assertEquals(listOf("news", "top", "recent"), withoutScore.map { it.key })
    }

    private fun cards(prefix: String, count: Int): List<Card> =
        (1..count).map { Card(mid = it.toLong(), name = "$prefix$it") }
}
