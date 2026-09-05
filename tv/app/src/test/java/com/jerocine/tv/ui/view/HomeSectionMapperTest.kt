package com.jerocine.tv.ui.view

import com.jerocine.tv.data.Card
import com.jerocine.tv.data.HomeResp
import com.jerocine.tv.data.HomeRow
import com.jerocine.tv.data.NavCategory
import com.jerocine.tv.ui.HomeDashboard
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class HomeSectionMapperTest {
    @Test
    fun mapsBoundedSectionsWithStableKeys() {
        val dashboard = HomeDashboard(
            home = HomeResp(
                rows = listOf(
                    row(7, "电影", cards(100, 7), cards(200, 7)),
                    row(9, "电视剧", cards(300, 7), cards(400, 7)),
                ),
            ),
        )

        val sections = mapHomeSections(dashboard)

        val panels = sections.flatMap { listOfNotNull(it.left, it.right) }
        assertEquals(listOf("hot", "latest", "category:7", "category:9"), panels.map { it.stableKey })
        assertTrue(panels.all { it.items.size == 3 })
        assertEquals("热门榜单", sections.first().left.title)
        assertEquals("最新上架", sections.first().right?.title)
        assertEquals(HomePanelTone.CYAN, panels[0].tone)
        assertEquals(HomePanelTone.AMBER, panels[1].tone)
        assertEquals(HomePanelTone.CYAN, panels[2].tone)
        assertEquals(HomePanelTone.PURPLE, panels[3].tone)
    }

    @Test
    fun limitsCategorySectionsForLowEndHome() {
        val rows = (1L..7L).map { id ->
            row(id, "分类$id", cards(id * 100, 3), cards(10_000 + id * 100, 3))
        }

        val sections = mapHomeSections(HomeDashboard(home = HomeResp(rows = rows)))

        val panels = sections.flatMap { listOfNotNull(it.left, it.right) }
        assertEquals(3, sections.size)
        assertEquals(4, panels.count { it.stableKey.startsWith("category:") })
    }

    @Test
    fun mapsAtMostFourTopLevelNavigationItems() {
        val dashboard = HomeDashboard(
            home = HomeResp(
                rows = (1L..6L).map { id ->
                    row(id, "分类$id", cards(id * 10, 6), emptyList())
                },
            ),
        )

        assertEquals(listOf(1L, 2L, 3L, 4L), mapHomeNavigation(dashboard).map { it.id })
    }

    private fun row(id: Long, name: String, hot: List<Card>, latest: List<Card>) = HomeRow(
        nav = NavCategory(id = id, pid = 0, name = name),
        hot = hot,
        latest = latest,
    )

    private fun cards(start: Long, count: Int) =
        (0 until count).map { Card(mid = start + it, name = "影片${start + it}") }
}
