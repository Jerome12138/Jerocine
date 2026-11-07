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
                            title = "${info.detail.name} · ${episode.episode}".trim()
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
