package com.jerocine.tv.ui

import com.jerocine.tv.data.HistoryItem

data class HistoryTvGroup(
    val bucket: String,
    val label: String,
    val items: List<HistoryItem>,
)

sealed interface HistoryRow {
    data class Header(val bucket: String, val label: String) : HistoryRow
    data class Item(val bucket: String, val history: HistoryItem) : HistoryRow
}

fun flattenHistoryRows(groups: List<HistoryTvGroup>): List<HistoryRow> = groups.flatMap { group ->
    listOf(HistoryRow.Header(group.bucket, group.label)) +
        group.items.map { HistoryRow.Item(group.bucket, it) }
}

fun groupHistoryForTv(
    items: List<HistoryItem>,
    nowMillis: Long = System.currentTimeMillis(),
): List<HistoryTvGroup> {
    val day = 24L * 60L * 60L * 1000L
    val grouped = items.groupBy { item ->
        val age = nowMillis - item.historyTimeMillis()
        when {
            age < day -> "today"
            age < 7L * day -> "week"
            age < 30L * day -> "month"
            else -> "earlier"
        }
    }
    return listOf(
        "today" to "今天",
        "week" to "本周",
        "month" to "本月",
        "earlier" to "更早",
    ).mapNotNull { (bucket, label) ->
        grouped[bucket]?.takeIf { it.isNotEmpty() }?.let { HistoryTvGroup(bucket, label, it) }
    }
}

private fun HistoryItem.historyTimeMillis(): Long =
    updatedAt.takeIf { it > 0L } ?: createdAt.takeIf { it > 0L } ?: 0L

fun historyUpdatedLabel(
    history: HistoryItem,
    nowMillis: Long = System.currentTimeMillis(),
): String {
    val timestamp = history.historyTimeMillis()
    if (timestamp <= 0L) return "最近观看"
    val age = (nowMillis - timestamp).coerceAtLeast(0L)
    val minute = 60L * 1000L
    val hour = 60L * minute
    val day = 24L * hour
    return when {
        age < minute -> "刚刚"
        age < hour -> "${age / minute} 分钟前"
        age < day -> "${age / hour} 小时前"
        age < 30L * day -> "${age / day} 天前"
        else -> "较早"
    }
}
