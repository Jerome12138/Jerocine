package com.jerocine.tv.data

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.serializer

/**
 * 登录 token 本地持久化。
 * 优先 EncryptedSharedPreferences 加密存储（AndroidKeyStore，API 23+）；
 * 初始化失败（如 API 21-22 / 部分魔改 ROM 无 KeyStore）时回退明文 SharedPreferences，保证可用。
 */
class TokenStore(context: Context) {

    private val sp: SharedPreferences = runCatching {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            "jerocine_tv_auth_enc",
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
        ) as SharedPreferences
    }.getOrElse {
        // 回退：老设备 / 无 KeyStore 时用明文，避免崩溃
        context.getSharedPreferences("jerocine_tv_auth", Context.MODE_PRIVATE)
    }
    private val playerSp: SharedPreferences = context.getSharedPreferences("jerocine", Context.MODE_PRIVATE)

    var token: String?
        get() = sp.getString(KEY_TOKEN, null)
        set(v) { sp.edit().putString(KEY_TOKEN, v).apply() }

    var userName: String?
        get() = sp.getString(KEY_USER, null)
        set(v) { sp.edit().putString(KEY_USER, v).apply() }

    /** 服务器地址（同一 APK 连不同后端） */
    var server: String?
        get() = sp.getString(KEY_SERVER, null)
        set(v) { sp.edit().putString(KEY_SERVER, v).apply() }

    /** 还原动画（老设备去抖动）：on / off / auto */
    var reduceMotion: String
        get() = sp.getString(KEY_REDUCE_MOTION, "auto") ?: "auto"
        set(v) { sp.edit().putString(KEY_REDUCE_MOTION, v).apply() }

    /** 与旧 PlayerActivity 共用的广告过滤开关，默认开启 */
    var adFilterEnabled: Boolean
        get() = playerSp.getBoolean(KEY_AD_FILTER, true)
        set(v) { playerSp.edit().putBoolean(KEY_AD_FILTER, v).apply() }

    fun localHistories(): List<LocalHistoryRecord> {
        val raw = sp.getString(KEY_LOCAL_HISTORY, null) ?: return emptyList()
        return runCatching {
            ApiClient.json.decodeFromString(ListSerializer(LocalHistoryRecord.serializer()), raw)
        }.getOrDefault(emptyList())
    }

    fun upsertLocalHistory(record: LocalHistoryRecord, limit: Int = LOCAL_HISTORY_LIMIT) {
        val next = upsertLocalHistory(localHistories(), record, limit)
        saveLocalHistories(next)
    }

    fun updateLocalHistory(
        mid: Long,
        source: String,
        episodeIndex: Int,
        progress: Double,
        duration: Double,
    ) {
        val existing = localHistories().firstOrNull { it.mid == mid } ?: return
        upsertLocalHistory(
            existing.copy(
                source = source.ifBlank { existing.source },
                episodeIndex = episodeIndex,
                progress = progress,
                duration = duration,
                updatedAt = System.currentTimeMillis(),
            )
        )
    }

    fun deleteLocalHistory(mid: Long): Boolean {
        val current = localHistories()
        val next = current.filterNot { it.mid == mid }
        saveLocalHistories(next)
        return next.size != current.size
    }

    fun clearLocalHistory(): Boolean {
        val hadItems = localHistories().isNotEmpty()
        saveLocalHistories(emptyList())
        return hadItems
    }

    fun localFavorites(): List<LocalFavoriteRecord> {
        val raw = sp.getString(KEY_LOCAL_FAVORITES, null) ?: return emptyList()
        return runCatching {
            ApiClient.json.decodeFromString(ListSerializer(LocalFavoriteRecord.serializer()), raw)
        }.getOrDefault(emptyList())
    }

    fun isLocalFavorite(mid: Long): Boolean =
        localFavorites().any { it.mid == mid }

    fun toggleLocalFavorite(record: LocalFavoriteRecord): Boolean {
        val (next, selected) = toggleLocalFavorite(localFavorites(), record, LOCAL_FAVORITES_LIMIT)
        saveLocalFavorites(next)
        return selected
    }

    fun deleteLocalFavorite(mid: Long): Boolean {
        val current = localFavorites()
        val next = current.filterNot { it.mid == mid }
        saveLocalFavorites(next)
        return next.size != current.size
    }

    fun localSkipSettings(): List<SkipSetting> {
        val raw = sp.getString(KEY_LOCAL_SKIP_SETTINGS, null) ?: return emptyList()
        return runCatching {
            ApiClient.json.decodeFromString(ListSerializer(SkipSetting.serializer()), raw)
        }.getOrDefault(emptyList())
    }

    fun localSkipFor(mid: Long): SkipSetting? =
        localSkipSettings().firstOrNull { it.mid == mid }

    fun saveLocalSkipSetting(mid: Long, intro: Int, outro: Int) {
        val next = upsertLocalSkipSetting(
            localSkipSettings(),
            SkipSetting(mid = mid, intro = intro, outro = outro)
        )
        saveLocalSkipSettings(next)
    }

    fun searchHistory(): List<String> {
        val raw = sp.getString(KEY_SEARCH_HISTORY, null) ?: return emptyList()
        return runCatching {
            ApiClient.json.decodeFromString(ListSerializer(String.serializer()), raw)
        }.getOrDefault(emptyList())
    }

    fun addSearchHistory(keyword: String) {
        saveSearchHistory(addSearchHistory(searchHistory(), keyword, SEARCH_HISTORY_LIMIT))
    }

    fun removeSearchHistory(keyword: String): Boolean {
        val current = searchHistory()
        val next = removeSearchHistory(current, keyword)
        saveSearchHistory(next)
        return next.size != current.size
    }

    fun clearSearchHistory(): Boolean {
        val hadItems = searchHistory().isNotEmpty()
        saveSearchHistory(emptyList())
        return hadItems
    }

    /** 只清 token（换服务器/退出登录时保留 server 配置） */
    fun clear() { sp.edit().remove(KEY_TOKEN).remove(KEY_USER).apply() }

    private fun saveLocalHistories(records: List<LocalHistoryRecord>) {
        val encoded = ApiClient.json.encodeToString(ListSerializer(LocalHistoryRecord.serializer()), records)
        sp.edit().putString(KEY_LOCAL_HISTORY, encoded).apply()
    }

    private fun saveLocalFavorites(records: List<LocalFavoriteRecord>) {
        val encoded = ApiClient.json.encodeToString(ListSerializer(LocalFavoriteRecord.serializer()), records)
        sp.edit().putString(KEY_LOCAL_FAVORITES, encoded).apply()
    }

    private fun saveLocalSkipSettings(settings: List<SkipSetting>) {
        val encoded = ApiClient.json.encodeToString(ListSerializer(SkipSetting.serializer()), settings)
        sp.edit().putString(KEY_LOCAL_SKIP_SETTINGS, encoded).apply()
    }

    private fun saveSearchHistory(history: List<String>) {
        val encoded = ApiClient.json.encodeToString(ListSerializer(String.serializer()), history)
        sp.edit().putString(KEY_SEARCH_HISTORY, encoded).apply()
    }

    companion object {
        private const val KEY_TOKEN = "token"
        private const val KEY_USER = "userName"
        private const val KEY_SERVER = "server"
        private const val KEY_REDUCE_MOTION = "reduce_motion"
        private const val KEY_AD_FILTER = "ad_filter_enabled"
        private const val KEY_LOCAL_HISTORY = "local_history"
        private const val KEY_LOCAL_FAVORITES = "local_favorites"
        private const val KEY_LOCAL_SKIP_SETTINGS = "local_skip_settings"
        private const val KEY_SEARCH_HISTORY = "search_history"
        private const val LOCAL_HISTORY_LIMIT = 60
        private const val LOCAL_FAVORITES_LIMIT = 60
        private const val SEARCH_HISTORY_LIMIT = 10
    }
}
