package com.jerocine.tv.data

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

class FilmRepositoryTest {
    @Test
    fun loginReturnsTokenAndUserName() = runTest {
        val repo = FilmRepository(FakeJerocineApi())

        val result = repo.login(account = "testuser", password = "secret")

        assertEquals("tok", result.token)
        assertEquals("testuser", result.userName)
    }

    @Test
    fun latestVersionReturnsVersionMetadata() = runTest {
        val repo = FilmRepository(FakeJerocineApi())

        val result = repo.latestVersion()

        assertEquals(2, result.versionCode)
        assertEquals("0.2.0", result.versionName)
    }
}

private class FakeJerocineApi : JerocineApi {
    override suspend fun home(): HomeResp = HomeResp()

    override suspend fun categories(): List<NavCategory> = emptyList()

    override suspend fun films(
        keyword: String?,
        pid: Long?,
        category: Long?,
        plot: String?,
        area: String?,
        language: String?,
        year: Int?,
        sort: String?,
        current: Int,
        size: Int
    ): CardPage = CardPage()

    override suspend fun filters(pid: Long): FilmFilters = FilmFilters()

    override suspend fun classify(pid: Long): ClassifyResp = ClassifyResp()

    override suspend fun filmDetail(mid: Long): FilmDetailResp = FilmDetailResp()

    override suspend fun playInfo(mid: Long, source: String?, episode: Int): PlayInfoResp = PlayInfoResp()

    override suspend fun deviceCode(): DeviceCodeResp = DeviceCodeResp()

    override suspend fun devicePoll(body: Map<String, String>): DevicePollResp = DevicePollResp()

    override suspend fun login(body: LoginReq): LoginResp =
        LoginResp(userName = body.account, token = "tok")

    override suspend fun latestVersion(channel: Int): AppVersion =
        AppVersion(versionCode = 2, versionName = "0.2.0")
}
