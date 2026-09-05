package com.jerocine.tv.player

import android.content.Context
import android.content.Intent
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
    val proxyBase: String,
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
    proxyBase: String,
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
        proxyBase = proxyBase.trimEnd('/'),
    )
}

fun launchNativePlayer(context: Context, payload: NativePlayerPayload) {
    val intent = Intent(context, PlayerActivity::class.java).apply {
        putExtra(PlayerActivity.EXTRA_SOURCES_JSON, payload.sourcesJson)
        putExtra(PlayerActivity.EXTRA_CURRENT_SOURCE_ID, payload.currentSourceId)
        putExtra(PlayerActivity.EXTRA_START_INDEX, payload.startIndex)
        putExtra(PlayerActivity.EXTRA_RESUME_MS, payload.resumeMs)
        putExtra(PlayerActivity.EXTRA_SKIP_INTRO_MS, payload.skipIntroMs)
        putExtra(PlayerActivity.EXTRA_SKIP_OUTRO_MS, payload.skipOutroMs)
        putExtra(PlayerActivity.EXTRA_FILM_ID, payload.filmId)
        putExtra(PlayerActivity.EXTRA_FILM_NAME, payload.filmName)
        putExtra(PlayerActivity.EXTRA_PROXY_BASE, payload.proxyBase)
    }
    context.startActivity(intent)
}
