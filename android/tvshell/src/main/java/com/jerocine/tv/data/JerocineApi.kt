package com.jerocine.tv.data

import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

/** Jerocine /api/v1 公开接口（响应无信封，直接是数据） */
interface JerocineApi {

    @GET("home")
    suspend fun home(): HomeResp

    @GET("categories")
    suspend fun categories(): List<NavCategory>

    @GET("films")
    suspend fun films(
        @Query("keyword") keyword: String? = null,
        @Query("pid") pid: Long? = null,
        @Query("category") category: Long? = null,
        @Query("plot") plot: String? = null,
        @Query("area") area: String? = null,
        @Query("language") language: String? = null,
        @Query("year") year: Int? = null,
        @Query("sort") sort: String? = null,
        @Query("page") current: Int = 1,
        @Query("size") size: Int = 30
    ): CardPage

    @GET("categories/{pid}/filters")
    suspend fun filters(@Path("pid") pid: Long): FilmFilters

    @GET("films/classify")
    suspend fun classify(@Query("pid") pid: Long): ClassifyResp

    @GET("films/{mid}")
    suspend fun filmDetail(@Path("mid") mid: Long): FilmDetailResp

    @GET("films/{mid}/play")
    suspend fun playInfo(
        @Path("mid") mid: Long,
        @Query("source") source: String? = null,
        @Query("episode") episode: Int = 0
    ): PlayInfoResp

    // 设备码登录
    @POST("auth/device/code")
    suspend fun deviceCode(): DeviceCodeResp

    @POST("auth/device/poll")
    suspend fun devicePoll(@Body body: Map<String, String>): DevicePollResp

    @POST("auth/login")
    suspend fun login(@Body body: LoginReq): LoginResp

    @GET("app/version/latest")
    suspend fun latestVersion(@Query("channel") channel: Int = 0): AppVersion
}
