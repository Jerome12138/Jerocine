package com.jerocine.tv.data

import com.jerocine.player.EpisodeKey
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * P0-3c（2026-11-10）；**2026-11-16 简化**：续播按 **服务端 epid 判等** 重定位 + 未命中回退。
 * 与 web `indexByServerEpid` / Go `danmu.EpidHash` 同口径 —— **不再有**单边通配 / `p` 前缀相容。
 */
class EpisodeEpidTest {

    /** 造"目标源"的集列表：判定只看每集的**服务端 epid**（`episode` 集名不再参与）。 */
    private fun eps(vararg epids: String): List<Episode> =
        epids.map { Episode(episode = "", link = "", epid = it) }

    @Test
    fun `服务端 epid 相等即命中（源内集序变化也能定位回原集）`() {
        val before = eps(
            "s=-|v=-|p=-|n=1|d=-",
            "s=-|v=-|p=-|n=2|d=-",
            "s=-|v=-|p=上|n=-|d=20260803",
            "s=-|v=-|p=中|n=-|d=20260803",
        )
        val after = eps(
            "s=-|v=-|p=上|n=-|d=20260803",
            "s=-|v=-|p=-|n=1|d=-",
            "s=-|v=-|p=-|n=9|d=-",
            "s=-|v=-|p=-|n=2|d=-",
            "s=-|v=-|p=中|n=-|d=20260803",
        )
        val ep = "s=-|v=-|p=上|n=-|d=20260803"
        assertEquals(2, indexByEpidForSource(ep, before))
        assertEquals(0, indexByEpidForSource(ep, after))
    }

    @Test
    fun `未命中回退下标：空 epid、坏串、空列表、不再宽容`() {
        val list = eps("s=-|v=-|p=-|n=1|d=-", "s=-|v=-|p=-|n=2|d=-")
        assertEquals(1, resumeEpisodeIndex("", 1, list))
        assertEquals(1, resumeEpisodeIndex("坏串", 1, list))
        assertEquals(1, resumeEpisodeIndex("s=-|v=-|p=下|n=-|d=20261231", 1, list))
        assertEquals(0, resumeEpisodeIndex("s=-|v=-|p=上|n=-|d=20260803", 0, emptyList()))
        // ⚠️ 简化后**不再宽容**：`p=中` 对 `p=中上` 判不中（旧②档的前缀相容已删）⇒ 回退
        val list2 = eps("s=-|v=-|p=中上|n=1|d=-")
        assertEquals(7, resumeEpisodeIndex("s=-|v=-|p=中|n=1|d=-", 7, list2))
    }

    @Test
    fun `epidHash 与 Go/web 同算法（黄金值锚定）`() {
        assertEquals("2y60kj4pkp7rs", EpisodeKey.epidHash("s=-|v=-|p=-|n=1|d=-"))
        assertEquals("1vo45avj8hymg", EpisodeKey.epidHash("s=王牌对王牌|v=-|p=-|n=5|d=-"))
    }
}
