package com.jerocine.tv.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LocalFavoriteTest {
    @Test
    fun toggleLocalFavorite_addsMissingFavoriteAndSortsByCreatedAt() {
        val (items, selected) = toggleLocalFavorite(
            records = listOf(LocalFavoriteRecord(mid = 1, createdAt = 100)),
            record = LocalFavoriteRecord(mid = 2, name = "New", createdAt = 300),
            limit = 2
        )

        assertTrue(selected)
        assertEquals(listOf(2L, 1L), items.map { it.mid })
        assertEquals("New", items.first().name)
    }

    @Test
    fun toggleLocalFavorite_removesExistingFavorite() {
        val (items, selected) = toggleLocalFavorite(
            records = listOf(
                LocalFavoriteRecord(mid = 1, createdAt = 100),
                LocalFavoriteRecord(mid = 2, createdAt = 300)
            ),
            record = LocalFavoriteRecord(mid = 2, createdAt = 500)
        )

        assertFalse(selected)
        assertEquals(listOf(1L), items.map { it.mid })
    }
}
