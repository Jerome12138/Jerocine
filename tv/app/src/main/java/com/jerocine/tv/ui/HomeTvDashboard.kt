package com.jerocine.tv.ui

import com.jerocine.tv.data.Card

private const val HOME_PANEL_SIZE = 3

data class HomeTvPanel(
    val id: Long,
    val title: String,
    val subtitle: String,
    val items: List<Card>,
)

data class HomeTvDashboardModel(
    val hero: Card?,
    val hotPanel: List<Card>,
    val latestPanel: List<Card>,
    val categoryPanels: List<HomeTvPanel>,
    val firstPid: Long,
)

fun deriveHomeTvDashboard(input: HomeDashboard): HomeTvDashboardModel {
    val rows = input.home.rows
    val hero = rows.firstOrNull { it.hot.isNotEmpty() }?.hot?.firstOrNull()
        ?: rows.firstOrNull { it.latest.isNotEmpty() }?.latest?.firstOrNull()
    val hotPanel = rows.asSequence()
        .flatMap { it.hot.asSequence() }
        .distinctBy { it.mid }
        .take(HOME_PANEL_SIZE)
        .toList()
    val hotIds = hotPanel.map { it.mid }.toSet()
    val latestPanel = rows.asSequence()
        .flatMap { it.latest.asSequence() }
        .distinctBy { it.mid }
        .filterNot { it.mid in hotIds }
        .take(HOME_PANEL_SIZE)
        .toList()
    val categoryPanels = rows.mapNotNull { row ->
        val items = (if (row.hot.isNotEmpty()) row.hot else row.latest)
            .distinctBy { it.mid }
            .take(HOME_PANEL_SIZE)
        if (items.isEmpty()) null else HomeTvPanel(
            id = row.nav.id,
            title = row.nav.name.ifBlank { "推荐" },
            subtitle = categorySubtitle(row.nav.name),
            items = items,
        )
    }
    val firstPid = rows.firstOrNull { it.nav.pid == 0L }?.nav?.id
        ?: rows.firstOrNull()?.nav?.pid
        ?: 0L
    return HomeTvDashboardModel(hero, hotPanel, latestPanel, categoryPanels, firstPid)
}

private fun categorySubtitle(name: String): String = when (name) {
    "电视剧" -> "热播好剧抢先看"
    "电影" -> "院线大片合集"
    "综艺" -> "爆笑解压"
    "动漫", "动画" -> "国漫日番"
    "纪录片" -> "真实之美"
    "少儿" -> "放心看"
    "短剧" -> "高能反转"
    else -> "精彩内容随心看"
}
