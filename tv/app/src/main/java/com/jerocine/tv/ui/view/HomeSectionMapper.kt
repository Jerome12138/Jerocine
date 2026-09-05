package com.jerocine.tv.ui.view

import com.jerocine.tv.data.Card
import com.jerocine.tv.ui.HomeDashboard
import com.jerocine.tv.ui.deriveHomeTvDashboard

data class HomeSectionItem(
    val stableKey: String,
    val title: String,
    val subtitle: String,
    val items: List<Card>,
    val categoryId: Long? = null,
    val tone: HomePanelTone = HomePanelTone.CYAN,
)

enum class HomePanelTone { CYAN, AMBER, PURPLE }

data class HomeSectionPair(
    val left: HomeSectionItem,
    val right: HomeSectionItem?,
)

data class HomeNavItem(
    val id: Long,
    val title: String,
)

fun mapHomeNavigation(dashboard: HomeDashboard): List<HomeNavItem> = dashboard.home.rows
    .asSequence()
    .map { HomeNavItem(it.nav.id, it.nav.name.ifBlank { "分类" }) }
    .distinctBy(HomeNavItem::id)
    .take(4)
    .toList()

fun mapHomeSections(dashboard: HomeDashboard): List<HomeSectionPair> {
    val model = deriveHomeTvDashboard(dashboard)
    val panels = buildList {
        if (model.hotPanel.isNotEmpty()) {
            add(HomeSectionItem("hot", "热门榜单", "最热抢先看", model.hotPanel))
        }
        if (model.latestPanel.isNotEmpty()) {
            add(
                HomeSectionItem(
                    "latest",
                    "最新上架",
                    "每日更新",
                    model.latestPanel,
                    tone = HomePanelTone.AMBER,
                ),
            )
        }
        model.categoryPanels.take(4).forEachIndexed { index, panel ->
            add(
                HomeSectionItem(
                    stableKey = "category:${panel.id}",
                    title = panel.title,
                    subtitle = panel.subtitle,
                    items = panel.items,
                    categoryId = panel.id,
                    tone = if (index % 2 == 0) HomePanelTone.CYAN else HomePanelTone.PURPLE,
                ),
            )
        }
    }
    return panels.chunked(2).map { pair -> HomeSectionPair(pair[0], pair.getOrNull(1)) }
}
