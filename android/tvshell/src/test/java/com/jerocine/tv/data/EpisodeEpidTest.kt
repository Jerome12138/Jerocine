package com.jerocine.tv.data

import com.jerocine.player.EpisodeKey
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * P0-3c（2026-11-10）：续播按 epid 重定位 + 未命中回退。
 * 与 player-core `EpisodeKey.indexByEpid` / web `indexByEpid` 同口径。
 */
class EpisodeEpidTest {

    private fun eps(vararg titles: String): List<Episode> =
        titles.map { Episode(episode = it, link = "") }

    @Test
    fun `epid 严格命中（源内集序变化也能定位回原集）`() {
        val before = eps("第1集", "第2集", "20260803上", "20260803中")
        val after = eps("20260803上", "第1集", "第9集", "第2集", "20260803中")
        val ep = EpisodeKey.parse("20260803上").epid()
        assertEquals(2, indexByEpidForSource(ep, before))
        assertEquals(0, indexByEpidForSource(ep, after))
    }

    @Test
    fun `未命中回退下标：空 epid、坏串、空列表同样回退`() {
        val list = eps("第1集", "第2集")
        assertEquals(1, resumeEpisodeIndex("", 1, list))
        assertEquals(1, resumeEpisodeIndex("坏串", 1, list))
        assertEquals(1, resumeEpisodeIndex("s=-|v=-|p=下|n=-|d=20261231", 1, list))
        assertEquals(0, resumeEpisodeIndex("s=-|v=-|p=上|n=-|d=20260803", 0, emptyList()))
    }
}
