package com.jerocine.tv.data

import com.jerocine.player.EpisodeKey

/**
 * P0-3c（2026-11-10）；**2026-11-16 简化**：按历史里的**集身份**（epid）在目标源里重定位集下标。
 *
 * <p>**现在只判等**（方案 `方案-端侧epid判等简化-2026-11-13.md`，2026-11-16 落地）：
 * 比较的是**目标源每集的服务端 epid**（`Episode.epid`）与历史里的 epid —— **全等命中 ⇒ 下标，否则 -1**
 * （调用方回退 `episodeIndex`）。优先比短码（`EpisodeKey.epidHash`），原串兜底。
 *
 * <p>**已删**（旧两档判定的宽容部分）：单边缺通配（`n`/`p`/`d`）、`p` 前缀相容 —— 端侧不再需要
 * "本地重算值 vs 落库值"那层兜底（端侧只写服务端下发值 ⇒ 两边同池同规则）。
 *
 * @return 定位到的集下标；定位不到返回 -1（调用方回退 `episodeIndex` —— 方案 §3.1.9 的两级回退）。
 */
fun indexByEpidForSource(epid: String, episodes: List<Episode>): Int {
    if (epid.isBlank() || episodes.isEmpty()) return -1
    return EpisodeKey.indexByServerEpid(epid, episodes.map { it.epid })
}

/**
 * 续播落点：先按 `epid` 在 `episodes` 里重定位，**未命中回退** `fallbackIndex`。
 * 空 epid / 旧记录 / 未触发的集（裸「第N集」）⇒ 直接回退，与改动前行为一致。
 */
fun resumeEpisodeIndex(epid: String, fallbackIndex: Int, episodes: List<Episode>): Int {
    val byEpid = indexByEpidForSource(epid, episodes)
    return if (byEpid >= 0) byEpid else fallbackIndex
}

// U6（2026-11-11 拍板按 c）：原来这里还有一个 `epidForTitles()`（本地按集名算 epid，供匿名态起播写本地历史）。
// 按 (c) "端侧只读服务端 epid、不再产出自己的变体" ⇒ **已删除**；两处起播（DetailFragment / HistoryFragment）
// 改为直接取 `playable.getOrNull(下标)?.epid`（服务端下发值，未派生即空串）。
// `PlayerActivity.epidOfCurrentEpisode()` 的同源逻辑也已同步为"只读服务端"。

