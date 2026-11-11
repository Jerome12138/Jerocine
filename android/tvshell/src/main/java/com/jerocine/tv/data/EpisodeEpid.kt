package com.jerocine.tv.data

import com.jerocine.player.EpisodeKey

/**
 * P0-3c（2026-11-10）：按历史里的**集身份**（epid）在目标源里重定位集下标。
 *
 * <p>复用 player-core 的 [EpisodeKey.indexByEpid]（与 web `indexByEpid`、Go 侧同口径）：
 * ① 严格 —— epid 串全等（**源内集序变化** —— 重排/插集/补集 —— 时 `episodeIndex` 已失效，epid 仍能对上）；
 * ② 宽松 —— `s`/`v`(含变体序号) 必须**相等**；`n`/`p`/`d` 三段**单边缺失视为通配、双有必须满足各自规则**
 * （`n`/`d` 相等、`p` 前缀相容），且**候选唯一**才采纳（多个 ⇒ 放弃）。
 *
 * <p>⚠️ L5（2026-11-11 评审）：此处原写"② 宽松 —— **忽略 `n` 段**"，与方案 §4.4「**不得整体忽略 `n`**」
 * 相反（那是**错**的口径描述；实现一直是对的）。`n` 双有不等 ⇒ 必须拒绝。
 * <p>⚠️ F6：裸集名（只有 `n`，`s/v/p/d` 全空）**不构成身份** ⇒ ①②档都不参与。
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

/**
 * F4（2026-11-11 评审）：按**集名**算集身份（epid）—— 供"匿名态起播写本地历史"用。
 *
 * <p>口径与 `player-core PlayerActivity.epidOfCurrentEpisode` / web `danmakuEpid` 一致：
 * 当前集取**该源该位置**的**裸集名**，借用池取**全部源的全部裸集名**（只在"本集缺期号"时借）。
 * **F1 教训**：绝不能拿「片名 · 集名」展示串当解析基准（片名会被当成栏目名 ⇒ epid 恒错 ✗）。
 *
 * @param currentTitle 当前集**裸集名**；空 ⇒ 返回空串（宁缺勿错，端侧自然回退下标定位）
 * @param allTitles    借用池（全部源的裸集名）；全空则退化为只用当前集
 */
fun epidForTitles(currentTitle: String, allTitles: List<String>): String {
    if (currentTitle.isBlank()) return ""
    val cleaned = allTitles.filter { it.isNotBlank() }
    val pool = if (cleaned.isEmpty()) listOf(currentTitle) else cleaned
    return EpisodeKey.epidFor(EpisodeKey.parse(currentTitle), pool.map { EpisodeKey.parse(it) }.toTypedArray())
}
