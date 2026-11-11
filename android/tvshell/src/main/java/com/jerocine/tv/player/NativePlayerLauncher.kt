package com.jerocine.tv.player

import android.content.Context
import android.content.Intent
import com.jerocine.player.LivePlayerActivity
import com.jerocine.player.PlayerActivity
import com.jerocine.tv.data.PlayInfoResp
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json

data class NativePlayerPayload(
    val sourcesJson: String,
    val currentSourceId: String,
    val startIndex: Int,
    val resumeMs: Long,
    val skipIntroMs: Long,
    val skipOutroMs: Long,
    val filmId: String,
    val filmName: String,
    /** 直播标志(Q③): 直播载荷必须 true → 播放器隐藏进度行/屏蔽 ±10s seek/短路预取; 影片缺省 false。 */
    val isLive: Boolean = false,
)

@Serializable
private data class NativeSource(
    val id: String,
    val name: String,
    val episodes: List<NativeEpisode>,
)

@Serializable
private data class NativeEpisode(
    val url: String,
    val title: String,
    /**
     * **裸集名**（F1, 2026-11-11 评审）：`title` 是「片名 · 集名」**展示用**拼接串，
     * 不能拿它当身份解析基准（片名会被当成栏目名 ⇒ epid 恒错且污染历史/缓存 ✗）。
     * 缺省空串 ⇒ 原生侧**不产 epid**（宁缺勿错，端侧自然回退旧键）。
     */
    val rawTitle: String = "",
    /**
     * 集身份（P1-1b / M2, 2026-11-11 评审）：由服务端派生表下发的 `epid`（随 sources_json 传到
     * player-core，与 urls/titles 一一对应）；缺省空串 ⇒ player-core 回退"本源现算"。
     */
    val epid: String = "",
)

fun buildNativePlayerPayload(
    info: PlayInfoResp,
    requestedSource: String,
    requestedEpisode: Int,
    skipIntroSec: Int,
    skipOutroSec: Int,
    resumeSec: Double = 0.0,
): NativePlayerPayload? {
    val sources = info.detail.sources
        .map { source ->
            NativeSource(
                id = source.id,
                name = source.name,
                episodes = source.episodes
                    .filter { it.link.isNotBlank() }
                    .map { episode ->
                        NativeEpisode(
                            url = episode.link,
                            title = "${info.detail.name} · ${episode.episode}".trim(),
                            // F1：显式带上**裸集名**供 epid 解析（与 web `nativePlay.ts` 同口径）
                            rawTitle = episode.episode,
                            // M2（2026-11-11 评审）：透传服务端 epid（缺省空串 ⇒ player-core 回退现算）
                            epid = episode.epid
                        )
                    }
            )
        }
        .filter { it.episodes.isNotEmpty() }

    if (sources.isEmpty()) return null

    val selected = sources.firstOrNull { it.id == requestedSource }
        ?: sources.firstOrNull { it.id == info.currentSource }
        ?: sources.first()
    val startIndex = requestedEpisode.coerceIn(0, selected.episodes.lastIndex)

    return NativePlayerPayload(
        sourcesJson = Json.encodeToString(sources),
        currentSourceId = selected.id,
        startIndex = startIndex,
        resumeMs = (resumeSec * 1000).toLong().coerceAtLeast(0L),
        skipIntroMs = skipIntroSec.coerceAtLeast(0) * 1000L,
        skipOutroMs = skipOutroSec.coerceAtLeast(0) * 1000L,
        filmId = info.detail.mid.toString(),
        filmName = info.detail.name,
    )
}

fun launchNativePlayer(context: Context, payload: NativePlayerPayload) {
    // 目标 Activity 是公共播放模块 player-core 的实现(与 WebView 壳共用同一份播放器)
    // 按 payload.isLive 分流(2026-11-07 用户拍板): 直播 → LivePlayerActivity(专用直播壳),
    // 影片 → PlayerActivity(既有行为不动)。extras 两边照旧全传, 新 Activity 只读它需要的。
    val target = if (payload.isLive) LivePlayerActivity::class.java else PlayerActivity::class.java
    val intent = Intent(context, target).apply {
        putExtra(PlayerActivity.EXTRA_SOURCES_JSON, payload.sourcesJson)
        putExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID, payload.currentSourceId)
        putExtra(PlayerActivity.EXTRA_START_INDEX, payload.startIndex)
        putExtra(PlayerActivity.EXTRA_RESUME_MS, payload.resumeMs)
        putExtra(PlayerActivity.EXTRA_SKIP_INTRO_MS, payload.skipIntroMs)
        putExtra(PlayerActivity.EXTRA_SKIP_OUTRO_MS, payload.skipOutroMs)
        putExtra(PlayerActivity.EXTRA_FILM_ID, payload.filmId)
        putExtra(PlayerActivity.EXTRA_FILM_NAME, payload.filmName)
        putExtra(PlayerActivity.EXTRA_IS_LIVE, payload.isLive)
    }
    context.startActivity(intent)
}
