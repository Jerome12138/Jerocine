package com.jerocine.tv.ui

import com.jerocine.tv.data.Card
import com.jerocine.tv.data.HomeResp
import com.jerocine.tv.data.HomeRow
import com.jerocine.tv.data.NavCategory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class HomeTvDashboardTest {
    @Test
    fun derivesBoundedHomePanelsLikeWebTvDashboard() {
        val dashboard = HomeDashboard(
            home = HomeResp(
                rows = listOf(
                    row(1, "电影", hot = cards("hot", 6), latest = cards("latestA", 6)),
                    row(2, "电视剧", hot = cards("dramaHot", 6), latest = cards("latestB", 6)),
                )
            )
        )

        val model = deriveHomeTvDashboard(dashboard)

        assertEquals("hot0", model.hero?.name)
        assertEquals(3, model.hotPanel.size)
        assertEquals(listOf("hot0", "hot1", "hot2"), model.hotPanel.map { it.name })
        assertEquals(3, model.latestPanel.size)
        assertTrue(model.latestPanel.none { latest -> model.hotPanel.any { it.mid == latest.mid } })
        assertEquals(2, model.categoryPanels.size)
        assertTrue(model.categoryPanels.all { it.items.size == 3 })
        assertEquals(1L, model.firstPid)
    }

    private fun row(id: Long, name: String, hot: List<Card>, latest: List<Card>) = HomeRow(
        nav = NavCategory(id = id, pid = 0, name = name),
        hot = hot,
        latest = latest,
    )

    private fun cards(prefix: String, count: Int): List<Card> =
        (0 until count).map { idx ->
            Card(mid = (prefix.hashCode().toLong() and 0xffff) * 100 + idx, name = "$prefix$idx")
        }
}
