package com.jerocine.tv.data

import com.jerocine.player.EpisodeKey

/**
 * P0-3c（2026-11-10）：按历史里的**集身份**（epid）在目标源里重定位集下标。
 *
 * <p>复用 player-core 的 [EpisodeKey.indexByEpid]（与 web `indexByEpid`、Go 侧同口径）：
 * ① 严格 —— epid 串全等（**源内集序变化** —— 重排/插集/补集 —— 时 `episodeIndex` 已失效，epid 仍能对上）；
 * ② 宽松 —— 忽略 `n` 段（记录时的期号可能是 A2 桥接/锚点算出的值，本地重算不出来），
 * 其余 `s`/`v`(含变体序号)/`d` 相等、`p` 段前缀相容，且**候选唯一**才采纳（多个 ⇒ 放弃）。
 *
 * @return 定位到的集下标；定位不到返回 -1（调用方回退 `episodeIndex` —— 方案 §3.1.9 的两级回退）。
 */
fun indexByEpidForSource(epid: String, episodes: List<Episode>): Int {
    if (epid.isBlank() || episodes.isEmpty()) return -1
    return EpisodeKey.indexByEpid(epid, episodes.map { it.episode })
}

/**
 * 续播落点：先按 `epid` 在 `episodes` 里重定位，**未命中回退** `fallbackIndex`。
 * 空 epid / 旧记录 / 未触发的集（裸「第N集」）⇒ 直接回退，与改动前行为一致。
 */
fun resumeEpisodeIndex(epid: String, fallbackIndex: Int, episodes: List<Episode>): Int {
    val byEpid = indexByEpidForSource(epid, episodes)
    return if (byEpid >= 0) byEpid else fallbackIndex
}
