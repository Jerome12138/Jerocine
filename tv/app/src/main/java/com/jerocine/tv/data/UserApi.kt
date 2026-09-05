package com.jerocine.tv.data

import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.PUT
import retrofit2.http.Query

/** 观看历史上报（对齐 POST /me/histories） */
@Serializable
data class HistoryReq(
    val mid: Long,
    val playFrom: String = "",
    val episode: Int = 0,
    val progress: Int = 0,
    val duration: Int = 0
)

/** 收藏（POST /me/favorites {mid}） */
@Serializable
data class FavoriteReq(val mid: Long)

@Serializable
data class SkipReq(
    val intro: Int = 0,
    val outro: Int = 0
)

interface UserApi {
    @GET("me")
    suspend fun me(): MeResp

    @GET("me/histories")
    suspend fun historyList(
        @Query("page") page: Int = 1,
        @Query("size") size: Int = 30
    ): Paginated<HistoryItem>

    @POST("me/histories")
    suspend fun upsertHistory(@Body body: HistoryReq)

    @DELETE("me/histories")
    suspend fun historyDelete(@Query("mid") mid: Long)

    @DELETE("me/histories/clear")
    suspend fun historyClear()

    @GET("me/favorites")
    suspend fun favoriteList(
        @Query("page") page: Int = 1,
        @Query("size") size: Int = 30
    ): Paginated<FavoriteItem>

    @POST("me/favorites")
    suspend fun addFavorite(@Body body: FavoriteReq)

    @DELETE("me/favorites")
    suspend fun favoriteRemove(@Query("mid") mid: Long)

    @GET("me/favorites/{mid}")
    suspend fun checkFavorite(@Path("mid") mid: Long): Map<String, Boolean>

    @GET("me/skip-settings")
    suspend fun skipSettings(): List<SkipSetting>

    @PUT("me/skip-settings/{mid}")
    suspend fun skipSave(@Path("mid") mid: Long, @Body body: SkipReq)
}
