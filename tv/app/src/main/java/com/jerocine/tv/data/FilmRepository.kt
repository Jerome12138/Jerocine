package com.jerocine.tv.data

import retrofit2.HttpException

/** 业务异常：从 RFC7807 problem+json 提取 message */
class ApiException(val status: Int, message: String) : Exception(message)

private inline fun <T> call(block: () -> T): T = try {
    block()
} catch (e: HttpException) {
    val msg = runCatching {
        val body = e.response()?.errorBody()?.string().orEmpty()
        ApiClient.json.decodeFromString(Problem.serializer(), body).message
    }.getOrNull()
    throw ApiException(e.code(), msg?.ifBlank { null } ?: "请求失败(${e.code()})")
}

/** 影视数据仓库：/api/v1 无信封，直接返回；错误转 ApiException */
class FilmRepository(private val api: JerocineApi) {

    suspend fun home(): HomeResp = call { api.home() }

    suspend fun categories(): List<NavCategory> = call { api.categories() }

    suspend fun films(
        keyword: String? = null,
        pid: Long? = null,
        category: Long? = null,
        plot: String? = null,
        area: String? = null,
        language: String? = null,
        year: Int? = null,
        sort: String? = null,
        current: Int = 1,
        size: Int = 30,
    ): CardPage = call {
        api.films(
            keyword = keyword,
            pid = pid,
            category = category,
            plot = plot,
            area = area,
            language = language,
            year = year,
            sort = sort,
            current = current,
            size = size,
        )
    }

    suspend fun filters(pid: Long): FilmFilters = call { api.filters(pid) }

    suspend fun classify(pid: Long): ClassifyResp = call { api.classify(pid) }

    suspend fun filmDetail(mid: Long): FilmDetailResp = call { api.filmDetail(mid) }

    suspend fun playInfo(mid: Long, source: String? = null, episode: Int = 0): PlayInfoResp =
        call { api.playInfo(mid, source, episode) }

    // 设备码登录
    suspend fun deviceCode(): DeviceCodeResp = call { api.deviceCode() }
    suspend fun devicePoll(deviceCode: String): DevicePollResp =
        call { api.devicePoll(mapOf("deviceCode" to deviceCode)) }
    suspend fun login(account: String, password: String): LoginResp =
        call { api.login(LoginReq(account = account, password = password)) }
    suspend fun latestVersion(channel: Int = 0): AppVersion =
        call { api.latestVersion(channel) }
}
