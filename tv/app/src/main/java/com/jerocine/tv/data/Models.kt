package com.jerocine.tv.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull

/**
 * 对齐生产 /api/v1 契约（server/internal/dto）。
 * 响应无 {code,data,msg} 信封，直接是数据本身。
 */

@Serializable
data class Card(
    val mid: Long = 0,
    val name: String = "",
    val cover: String = "",
    val cid: Long = 0,
    val pid: Long = 0,
    val cName: String = "",
    val subTitle: String = "",
    val area: String = "",
    val year: Int = 0,
    val state: String = "",
    val remarks: String = "",
    val dbScore: Double = 0.0
)

@Serializable
data class NavCategory(
    val id: Long = 0,
    val pid: Long = 0,
    val name: String = "",
    val children: List<NavCategory> = emptyList()
)

@Serializable
data class Episode(
    val episode: String = "",
    val link: String = ""
)

@Serializable
data class PlaySource(
    val id: String = "",
    val name: String = "",
    val episodes: List<Episode> = emptyList()
)

@Serializable
data class FilmDetail(
    val mid: Long = 0,
    val name: String = "",
    val cover: String = "",
    val cid: Long = 0,
    val pid: Long = 0,
    val cName: String = "",
    val subTitle: String = "",
    val actor: String = "",
    val director: String = "",
    val area: String = "",
    val language: String = "",
    val year: Int = 0,
    val classTag: String = "",
    val remarks: String = "",
    val state: String = "",
    val dbScore: Double = 0.0,
    val content: String = "",
    val playFrom: List<String> = emptyList(),
    val sources: List<PlaySource> = emptyList()
)

/** GET /home */
@Serializable
data class HomeRow(
    val nav: NavCategory = NavCategory(),
    val latest: List<Card> = emptyList(),
    val hot: List<Card> = emptyList()
)

@Serializable
data class HomeResp(
    val categories: List<NavCategory> = emptyList(),
    val rows: List<HomeRow> = emptyList()
)

/** GET /films/{mid} */
@Serializable
data class FilmDetailResp(
    val detail: FilmDetail = FilmDetail(),
    val related: List<Card> = emptyList()
)

/** GET /films/{mid}/play */
@Serializable
data class PlayInfoResp(
    val detail: FilmDetail = FilmDetail(),
    val current: Episode = Episode(),
    val currentSource: String = "",
    val currentEpisode: Int = 0,
    val related: List<Card> = emptyList()
)

/** GET /films/classify */
@Serializable
data class ClassifyResp(
    val title: NavCategory? = null,
    val news: List<Card> = emptyList(),
    val top: List<Card> = emptyList(),
    val recent: List<Card> = emptyList()
)

@Serializable
data class FilterTag(
    val name: String = "",
    val value: String = "",
)

@Serializable
data class FilmFilters(
    val titles: Map<String, String> = emptyMap(),
    val tags: Map<String, List<FilterTag>> = emptyMap(),
    val sortList: List<String> = emptyList(),
)

/** 分页 {list, page} */
@Serializable
data class PageMeta(
    val current: Int = 1,
    val size: Int = 0,
    val total: Long = 0,
    val pageCount: Long = 0
)

@Serializable
data class CardPage(
    val list: List<Card> = emptyList(),
    val page: PageMeta = PageMeta()
)

@Serializable
data class Paginated<T>(
    val list: List<T> = emptyList(),
    val page: PageMeta = PageMeta()
)

@Serializable
data class HistoryItem(
    val id: Long = 0,
    val userId: Long = 0,
    val mid: Long = 0,
    val playFrom: String = "",
    val sourceId: String = "",
    val episode: JsonElement? = null,
    val episodeIndex: Int = 0,
    val progress: Double = 0.0,
    val duration: Double = 0.0,
    val createdAt: Long = 0,
    val updatedAt: Long = 0,
    val name: String = "",
    val cover: String = "",
    val cName: String = "",
    val remarks: String = "",
    val card: Card? = null
) {
    fun previewCard(): Card = card ?: Card(
        mid = mid,
        name = name,
        cover = cover,
        cName = cName,
        remarks = remarks
    )

    fun source(): String = sourceId.ifBlank { playFrom }

    fun episodeNumber(): Int {
        val numericEpisode = (episode as? JsonPrimitive)?.intOrNull
        return when {
            episodeIndex > 0 -> episodeIndex
            numericEpisode != null -> numericEpisode
            else -> 0
        }
    }

    fun episodeLabel(): String {
        val label = (episode as? JsonPrimitive)?.contentOrNull.orEmpty()
        return label.ifBlank { "第 ${episodeNumber() + 1} 集" }
    }

    fun progressFraction(): Float =
        if (duration > 0.0) (progress / duration).toFloat().coerceIn(0f, 1f) else 0f

    fun resumeSeconds(): Double = progress.toInt().coerceAtLeast(0).toDouble()
}

@Serializable
data class LocalHistoryRecord(
    val mid: Long = 0,
    val source: String = "",
    val episodeIndex: Int = 0,
    val progress: Double = 0.0,
    val duration: Double = 0.0,
    val name: String = "",
    val cover: String = "",
    val cName: String = "",
    val remarks: String = "",
    val updatedAt: Long = 0
) {
    fun toHistoryItem(): HistoryItem = HistoryItem(
        mid = mid,
        sourceId = source,
        episodeIndex = episodeIndex,
        progress = progress,
        duration = duration,
        updatedAt = updatedAt,
        name = name,
        cover = cover,
        cName = cName,
        remarks = remarks
    )
}

fun upsertLocalHistory(
    records: List<LocalHistoryRecord>,
    record: LocalHistoryRecord,
    limit: Int = 60,
): List<LocalHistoryRecord> =
    (listOf(record) + records.filterNot { it.mid == record.mid })
        .sortedByDescending { it.updatedAt }
        .take(limit.coerceAtLeast(0))

fun detailResumeHistory(
    mid: Long,
    isLoggedIn: Boolean,
    remoteHistories: List<HistoryItem>,
    localHistories: List<LocalHistoryRecord>,
): HistoryItem? =
    if (isLoggedIn) {
        remoteHistories.firstOrNull { it.mid == mid }
    } else {
        localHistories.firstOrNull { it.mid == mid }?.toHistoryItem()
    }

@Serializable
data class FavoriteItem(
    val id: Long = 0,
    val userId: Long = 0,
    val mid: Long = 0,
    val createdAt: Long = 0,
    val name: String = "",
    val cover: String = "",
    val cName: String = "",
    val remarks: String = "",
    val card: Card? = null
) {
    fun previewCard(): Card = card ?: Card(
        mid = mid,
        name = name,
        cover = cover,
        cName = cName,
        remarks = remarks
    )
}

@Serializable
data class LocalFavoriteRecord(
    val mid: Long = 0,
    val name: String = "",
    val cover: String = "",
    val cName: String = "",
    val remarks: String = "",
    val createdAt: Long = 0
) {
    fun toFavoriteItem(): FavoriteItem = FavoriteItem(
        mid = mid,
        createdAt = createdAt,
        name = name,
        cover = cover,
        cName = cName,
        remarks = remarks
    )
}

fun toggleLocalFavorite(
    records: List<LocalFavoriteRecord>,
    record: LocalFavoriteRecord,
    limit: Int = 60,
): Pair<List<LocalFavoriteRecord>, Boolean> {
    val exists = records.any { it.mid == record.mid }
    if (exists) return records.filterNot { it.mid == record.mid } to false

    val next = (listOf(record) + records)
        .sortedByDescending { it.createdAt }
        .take(limit.coerceAtLeast(0))
    return next to true
}

/** 设备码登录 */
@Serializable
data class DeviceCodeResp(
    val deviceCode: String = "",
    val userCode: String = "",
    val expiresIn: Int = 0,
    val interval: Int = 3
)

@Serializable
data class DevicePollResp(
    val status: String = "pending", // pending | ok
    val userName: String = "",
    val token: String = "",
    val expires: Long = 0,
    val role: Int = 0
)

@Serializable
data class LoginReq(
    val account: String,
    val password: String
)

@Serializable
data class LoginResp(
    val userName: String = "",
    val token: String = "",
    val expires: Long = 0,
    val role: Int = 0
)

@Serializable
data class AppVersion(
    val id: Long = 0,
    val channel: Int = 0,
    val versionCode: Int = 0,
    val versionName: String = "",
    val apkUrl: String = "",
    val changelog: String = "",
    val force: Boolean = false,
    val whitelist: List<String> = emptyList(),
    val createdAt: Long = 0
)

/** GET /me */
@Serializable
data class MeResp(
    val id: Long = 0,
    val userName: String = "",
    val role: Int = 0
)

/**
 * GET /me/skip-settings 返回**每部片**的跳过配置数组（账号级，全量）。
 * intro=片头跳过秒数，outro=片尾提前进入下一集的秒数。0 表示不跳。
 */
@Serializable
data class SkipSetting(
    val mid: Long = 0,
    val intro: Int = 0,
    val outro: Int = 0
)

fun upsertLocalSkipSetting(
    settings: List<SkipSetting>,
    setting: SkipSetting,
): List<SkipSetting> =
    listOf(
        setting.copy(
            intro = setting.intro.coerceAtLeast(0),
            outro = setting.outro.coerceAtLeast(0),
        )
    ) + settings.filterNot { it.mid == setting.mid }

/** 未配置时的默认跳过（对齐 Web：片头 90s / 片尾 60s） */
object SkipDefaults {
    const val INTRO = 90
    const val OUTRO = 60
}

/** RFC7807 problem+json */
@Serializable
data class Problem(
    val code: Int = 0,
    val message: String = "",
    val traceId: String = ""
)
