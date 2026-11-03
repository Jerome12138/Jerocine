package com.jerocine.tv.data

import android.content.Context
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * 记住直播「最后观看」（源 + 频道），供两点使用：
 *  1. 点「直播」时从上次频道起播；
 *  2. 观看历史页顶部的「直播」本地专用条目（把整个直播当一个影片，只记最新一条）。
 */
object LiveLastViewed {
    /** 观看历史里「直播」条目的伪 mid（把整个直播当一个影片，只记最新一条） */
    const val LIVE_HISTORY_MID = -1L

    private const val PREF = "jerocine_live"
    private const val KEY = "last"
    private val json = Json { ignoreUnknownKeys = true }

    @Serializable
    data class Selection(
        val sourceId: Long = 0,
        val index: Int = 0,
        val sourceName: String = "",
        val channelName: String = "",
        /** 本次播放时间（毫秒）——历史条目按它参与排序（不置顶） */
        val ts: Long = 0,
    )

    /** JSON → Selection；缺失/损坏/非法（源 id 非正、下标为负）返回 null（纯函数，可测） */
    fun decode(raw: String?): Selection? {
        if (raw.isNullOrBlank()) return null
        val value = runCatching { json.decodeFromString(Selection.serializer(), raw) }.getOrNull() ?: return null
        if (value.sourceId <= 0 || value.index < 0) return null
        return value
    }

    fun encode(selection: Selection): String = json.encodeToString(Selection.serializer(), selection)

    fun read(context: Context): Selection? =
        decode(context.getSharedPreferences(PREF, Context.MODE_PRIVATE).getString(KEY, null))

    fun write(context: Context, selection: Selection) {
        context.getSharedPreferences(PREF, Context.MODE_PRIVATE).edit()
            .putString(KEY, encode(selection))
            .apply()
    }

    fun clear(context: Context) {
        context.getSharedPreferences(PREF, Context.MODE_PRIVATE).edit().remove(KEY).apply()
    }
}
