package com.jerocine.tv.data

import org.junit.Assert.assertEquals
import org.junit.Test

class LocalHistoryTest {
    @Test
    fun upsertLocalHistory_replacesSameFilmAndSortsByUpdateTime() {
        val existing = listOf(
            LocalHistoryRecord(mid = 1, name = "Old A", updatedAt = 100),
            LocalHistoryRecord(mid = 2, name = "B", updatedAt = 300)
        )

        val result = upsertLocalHistory(
            records = existing,
            record = LocalHistoryRecord(mid = 1, name = "New A", updatedAt = 500),
            limit = 2
        )

        assertEquals(listOf(1L, 2L), result.map { it.mid })
        assertEquals("New A", result.first().name)
        assertEquals(listOf(500L, 300L), result.map { it.updatedAt })
    }

    @Test
    fun upsertLocalHistory_limitsOldestItems() {
        val result = upsertLocalHistory(
            records = listOf(
                LocalHistoryRecord(mid = 1, updatedAt = 100),
                LocalHistoryRecord(mid = 2, updatedAt = 200)
            ),
            record = LocalHistoryRecord(mid = 3, updatedAt = 300),
            limit = 2
        )

        assertEquals(listOf(3L, 2L), result.map { it.mid })
    }

    @Test
    fun detailResumeHistory_usesLocalHistoryWhenLoggedOut() {
        val result = detailResumeHistory(
            mid = 8,
            isLoggedIn = false,
            remoteHistories = listOf(HistoryItem(mid = 8, name = "Remote")),
            localHistories = listOf(LocalHistoryRecord(mid = 8, name = "Local"))
        )

        assertEquals("Local", result?.name)
    }
}
