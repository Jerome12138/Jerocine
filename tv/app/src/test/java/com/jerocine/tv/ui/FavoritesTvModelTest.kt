package com.jerocine.tv.ui

import com.jerocine.tv.data.FavoriteItem
import org.junit.Assert.assertEquals
import org.junit.Test

class FavoritesTvModelTest {
    @Test
    fun favoriteCardsPreserveServerOrder() {
        val first = FavoriteItem(mid = 2, name = "第二部")
        val second = FavoriteItem(mid = 1, name = "第一部")

        assertEquals(listOf(first.previewCard(), second.previewCard()), favoriteCards(listOf(first, second)))
    }

    @Test
    fun derivesRemoteFavoritesCopy() {
        val model = deriveFavoritesTvModel(isLoggedIn = true, count = 5)

        assertEquals("云端", model.sourceChip)
        assertEquals("云端收藏 · 跨设备同步 · 共 5 部", model.subtitle)
    }

    @Test
    fun derivesLocalFavoritesCopy() {
        val model = deriveFavoritesTvModel(isLoggedIn = false, count = 2)

        assertEquals("本地", model.sourceChip)
        assertEquals("本地收藏 · 仅当前设备 · 共 2 部", model.subtitle)
    }
}
