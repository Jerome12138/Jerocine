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

/** 首页导航里「直播」入口的哨兵 id（不与真实分类 id 冲突） */
const val LIVE_NAV_ID = -1L

fun mapHomeNavigation(dashboard: HomeDashboard): List<HomeNavItem> {
    val categories = dashboard.home.rows
        .asSequence()
        // 000037: show=顶栏导航显示(仅一级有意义; 旧 server 不返回时 show=true 默认 true 兼容)
        .filter { it.nav.show }
        .map { HomeNavItem(it.nav.id, it.nav.name.ifBlank { "分类" }) }
        .distinctBy(HomeNavItem::id)
        .take(4)
        .toList()
    // 直播固定在最前，作为首页一级入口
    return listOf(HomeNavItem(LIVE_NAV_ID, "直播")) + categories
}

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
