package com.jerocine.tv.data

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class UserRepositoryTest {
    @Test
    fun toggleFavoriteAddsWhenMovieIsNotFavorited() = runTest {
        val api = FakeUserApi()
        val repo = UserRepository(api)

        val favorited = repo.toggleFavorite(42)

        assertTrue(favorited)
        assertTrue(api.favorites.contains(42))
    }

    @Test
    fun toggleFavoriteRemovesWhenMovieIsFavorited() = runTest {
        val api = FakeUserApi(mutableSetOf(42))
        val repo = UserRepository(api)

        val favorited = repo.toggleFavorite(42)

        assertFalse(favorited)
        assertFalse(api.favorites.contains(42))
    }

    @Test
    fun clearHistoryDeletesAllHistoryItems() = runTest {
        val api = FakeUserApi(historyIds = mutableSetOf(1, 2))
        val repo = UserRepository(api)

        val cleared = repo.historyClear()

        assertTrue(cleared)
        assertTrue(api.historyIds.isEmpty())
    }

    @Test
    fun saveSkipSettingClampsNegativeValuesAndCallsApi() = runTest {
        val api = FakeUserApi()
        val repo = UserRepository(api)

        val saved = repo.saveSkipSetting(mid = 7, intro = -10, outro = 45)

        assertTrue(saved)
        assertEquals(7L, api.savedSkipMid)
        assertEquals(0, api.savedSkip?.intro)
        assertEquals(45, api.savedSkip?.outro)
    }
}

private class FakeUserApi(
    val favorites: MutableSet<Long> = mutableSetOf(),
    val historyIds: MutableSet<Long> = mutableSetOf(),
) : UserApi {
    var savedSkipMid: Long = 0
    var savedSkip: SkipReq? = null

    override suspend fun me(): MeResp = MeResp()

    override suspend fun historyList(page: Int, size: Int): Paginated<HistoryItem> = Paginated()

    override suspend fun upsertHistory(body: HistoryReq) = Unit

    override suspend fun historyDelete(mid: Long) {
        historyIds.remove(mid)
    }

    override suspend fun historyClear() {
        historyIds.clear()
    }

    override suspend fun favoriteList(page: Int, size: Int): Paginated<FavoriteItem> = Paginated()

    override suspend fun addFavorite(body: FavoriteReq) {
        favorites.add(body.mid)
    }

    override suspend fun favoriteRemove(mid: Long) {
        favorites.remove(mid)
    }

    override suspend fun checkFavorite(mid: Long): Map<String, Boolean> =
        mapOf("favorited" to favorites.contains(mid))

    override suspend fun skipSettings(): List<SkipSetting> = emptyList()

    override suspend fun skipSave(mid: Long, body: SkipReq) {
        savedSkipMid = mid
        savedSkip = body
    }
}
