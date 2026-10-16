package com.jerocine.tv.ui

import com.jerocine.tv.data.HistoryItem
import org.junit.Assert.assertEquals
import org.junit.Test

class HistoryTvModelTest {
    @Test
    fun flattenedHistoryKeepsGroupOrder() {
        val history = item(mid = 9, updatedAt = 1)
        val rows = flattenHistoryRows(listOf(HistoryTvGroup("today", "今天", listOf(history))))

        assertEquals("今天", (rows.first() as HistoryRow.Header).label)
        assertEquals(history, (rows.last() as HistoryRow.Item).history)
    }

    @Test
    fun groupsHistoryIntoWebTvTimeBuckets() {
        val day = 24L * 60L * 60L * 1000L
        val now = 100L * day
        val groups = groupHistoryForTv(
            items = listOf(
                item(mid = 1, updatedAt = now - 2L * 60L * 60L * 1000L),
                item(mid = 2, updatedAt = now - 3L * day),
                item(mid = 3, updatedAt = now - 12L * day),
                item(mid = 4, updatedAt = now - 45L * day),
            ),
            nowMillis = now,
        )

        assertEquals(listOf("today", "week", "month", "earlier"), groups.map { it.bucket })
        assertEquals(listOf("今天", "本周", "本月", "更早"), groups.map { it.label })
        assertEquals(listOf(1L, 2L, 3L, 4L), groups.flatMap { group -> group.items.map { it.mid } })
    }

    @Test
    fun formatsStableRelativeHistoryTime() {
        val hour = 60L * 60L * 1000L
        val now = 100L * hour

        assertEquals("刚刚", historyUpdatedLabel(item(1, now - 30_000L), now))
        assertEquals("3 小时前", historyUpdatedLabel(item(2, now - 3L * hour), now))
        assertEquals("2 天前", historyUpdatedLabel(item(3, now - 48L * hour), now))
        assertEquals("最近观看", historyUpdatedLabel(item(4, 0L), now))
    }

    private fun item(mid: Long, updatedAt: Long) = HistoryItem(mid = mid, updatedAt = updatedAt)
}
