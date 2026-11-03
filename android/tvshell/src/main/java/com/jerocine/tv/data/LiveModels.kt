package com.jerocine.tv.data

import kotlinx.serialization.Serializable

/**
 * 直播（对齐生产 /api/v1 契约，无信封）：
 *   GET /live/sources            → { sources: [{id, name}] }
 *   GET /live/sources/{id}/channels → { id, name, channels: [{name, group, logo, url}] }
 */

@Serializable
data class LiveSource(
    val id: Long = 0,
    val name: String = "",
)

@Serializable
data class LiveSourcesResp(
    val sources: List<LiveSource> = emptyList(),
)

@Serializable
data class LiveChannel(
    val name: String = "",
    /** 分组名（m3u 的 group-title），空则归默认组 */
    val group: String = "",
    val logo: String = "",
    val url: String = "",
)

@Serializable
data class LiveChannelsResp(
    val id: Long = 0,
    val name: String = "",
    val channels: List<LiveChannel> = emptyList(),
)
