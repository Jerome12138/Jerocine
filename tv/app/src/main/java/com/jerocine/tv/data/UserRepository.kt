package com.jerocine.tv.data

/**
 * 用户数据仓库：历史 / 收藏 / 跳过设置。均需登录（Bearer）。
 * 上报失败不影响播放，调用方 fire-and-forget。
 */
class UserRepository(private val api: UserApi) {

    suspend fun me(): MeResp? = runCatching { api.me() }.getOrNull()

    suspend fun upsertHistory(req: HistoryReq): Boolean =
        runCatching { api.upsertHistory(req); true }.getOrDefault(false)

    suspend fun historyList(page: Int = 1, size: Int = 30): List<HistoryItem> =
        runCatching { api.historyList(page, size).list }.getOrDefault(emptyList())

    suspend fun historyDelete(mid: Long): Boolean =
        runCatching { api.historyDelete(mid); true }.getOrDefault(false)

    suspend fun historyClear(): Boolean =
        runCatching { api.historyClear(); true }.getOrDefault(false)

    suspend fun addFavorite(mid: Long): Boolean =
        runCatching { api.addFavorite(FavoriteReq(mid)); true }.getOrDefault(false)

    suspend fun favoriteList(page: Int = 1, size: Int = 30): List<FavoriteItem> =
        runCatching { api.favoriteList(page, size).list }.getOrDefault(emptyList())

    suspend fun favoriteRemove(mid: Long): Boolean =
        runCatching { api.favoriteRemove(mid); true }.getOrDefault(false)

    suspend fun isFavorite(mid: Long): Boolean =
        runCatching { api.checkFavorite(mid).values.firstOrNull() == true }.getOrDefault(false)

    suspend fun toggleFavorite(mid: Long): Boolean {
        return if (isFavorite(mid)) {
            runCatching { api.favoriteRemove(mid); false }.getOrDefault(true)
        } else {
            runCatching { api.addFavorite(FavoriteReq(mid)); true }.getOrDefault(false)
        }
    }

    /**
     * 取某部片的账号级跳过配置：登录且服务端有该 mid 记录才返回，否则 null。
     * 默认值（连续剧 90/60）由播放层按「是否多集」决定——电影不应默认跳过片头。
     */
    suspend fun skipFor(mid: Long): SkipSetting? {
        if (!ServiceLocator.isLoggedIn) return null
        val list = runCatching { api.skipSettings() }.getOrDefault(emptyList())
        return list.firstOrNull { it.mid == mid }
    }

    suspend fun saveSkipSetting(mid: Long, intro: Int, outro: Int): Boolean =
        runCatching {
            api.skipSave(
                mid,
                SkipReq(
                    intro = intro.coerceAtLeast(0),
                    outro = outro.coerceAtLeast(0),
                )
            )
            true
        }.getOrDefault(false)
}
