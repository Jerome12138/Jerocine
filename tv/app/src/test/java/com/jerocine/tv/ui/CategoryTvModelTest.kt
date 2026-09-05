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
            listOf(CategoryTvSection("news", "最新上映", "每日更新", "release_stamp", listOf(card)))
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
            )
        )

        assertEquals(listOf("news", "top"), sections.map { it.key })
        assertEquals(listOf("最新上映", "排行榜"), sections.map { it.title })
        assertEquals(listOf("每日更新", "按热度排序"), sections.map { it.subtitle })
        assertEquals(listOf("release_stamp", "hits"), sections.map { it.sort })
        assertEquals(18, sections.first().items.size)
        assertEquals(2, sections.last().items.size)
    }

    private fun cards(prefix: String, count: Int): List<Card> =
        (1..count).map { Card(mid = it.toLong(), name = "$prefix$it") }
}
