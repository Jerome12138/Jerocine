package com.jerocine.player

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * M3u8FilterCore 与 Go 内核的一致性测试 —— 跑 fixtures/m3u8-filter/cases.json 全语料。
 * 定位顺序: ① classpath resource("cases.json", build.gradle 已把 fixtures/m3u8-filter 挂为
 * test resource) → ② 文件系统向上回溯(Gradle 单测工作目录 = 模块目录, 仓库根 fixtures 在 ../../)。
 * 两路都找不到则报错提示(夹具是硬依赖, 不允许静默跳过)。
 */
class M3u8FilterCoreTest {

    private fun fixtureCases(): List<CaseJson> {
        val cp = javaClass.classLoader?.getResource("cases.json")
        val f = if (cp != null && cp.protocol == "file") File(cp.toURI()) else findFixtures()
        if (f == null || !f.exists()) throw AssertionError(
            "找不到 fixtures/m3u8-filter/cases.json (classpath 与文件系统回溯均失败)"
        )
        val doc = com.google.gson.JsonParser.parseString(f.readText()).asJsonObject
        val arr = doc.getAsJsonArray("cases")
        return arr.map { e ->
            val o = e.asJsonObject
            val exp = o.getAsJsonObject("expected")
            CaseJson(
                o.get("name").asString,
                o.get("base").asString,
                o.get("m3u8").asString,
                o.get("filterAds").asBoolean,
                exp.get("text").asString,
                exp.get("filtered").asInt,
                exp.get("total").asInt
            )
        }
    }

    private fun findFixtures(): File? {
        var dir: File? = File(".").absoluteFile
        repeat(4) {
            val c = File(dir, "fixtures/m3u8-filter/cases.json")
            if (c.exists()) return c
            dir = dir?.parentFile
        }
        return null
    }

    private data class CaseJson(
        val name: String,
        val base: String,
        val m3u8: String,
        val filterAds: Boolean,
        val expText: String,
        val expFiltered: Int,
        val expTotal: Int
    )

    @Test
    fun `golden 夹具全语料与 Go 内核逐例对拍`() {
        val cases = fixtureCases()
        assertTrue("夹具至少应有 1 例(实际 ${cases.size})", cases.isNotEmpty())
        cases.forEach { c ->
            val got = M3u8FilterCore.rewriteM3u8(c.m3u8, c.base, c.filterAds)
            assertEquals("${c.name}: text", c.expText, got.text)
            assertEquals("${c.name}: filtered", c.expFiltered, got.filtered)
            assertEquals("${c.name}: total", c.expTotal, got.total)
        }
    }

    @Test
    fun `segKey 先剥文件名再取目录段`() {
        assertEquals("cdn.example.com/play", M3u8FilterCore.segKey("https://cdn.example.com/play/seg0.ts"))
        assertEquals(
            "v.fengbao8.com/video/liurendiliuji",
            M3u8FilterCore.segKey("https://v.fengbao8.com/video/liurendiliuji/ecca40a6bdd2/0000001.ts")
        )
        assertEquals("cdnA.com", M3u8FilterCore.segKey("https://cdnA.com/s0.ts"))
    }

    @Test
    fun `segNumber 截末 15 位且忽略 query`() {
        assertEquals(123L, M3u8FilterCore.segNumber("https://h/x/seg123.ts?token=abc"))
        assertEquals(952435470000000L, M3u8FilterCore.segNumber("/video/adjump/time/17766952435470000000.ts"))
        assertEquals(-1L, M3u8FilterCore.segNumber("https://h/x/seg_aa.ts"))
    }

    @Test
    fun `segNumber URI 严格解析失败时剥 query 再取尾数字`() {
        // 含非法字符(空格)的绝对 URL: java.net.URI 解析失败 → 兜底剥 query/fragment,
        // 只按 path 段取尾数字, query 尾数字不得污染编号判定
        assertEquals(0L, M3u8FilterCore.segNumber("https://h/seg0.ts?token=abc 123"))
        assertEquals(7L, M3u8FilterCore.segNumber("https://h/seg0007.ts?token=x y&z=9"))
        assertEquals(42L, M3u8FilterCore.segNumber("https://h/seg42.ts#frag 1"))
    }

    @Test
    fun `absolutize 相对解析与绝对 URL 原样`() {
        val base = java.net.URI.create("https://cdn.example.com/play/index.m3u8")
        assertEquals("https://cdn.example.com/play/seg0.ts", M3u8FilterCore.absolutize("seg0.ts", base))
        assertEquals("https://cdn.example.com/abs/seg1.ts", M3u8FilterCore.absolutize("/abs/seg1.ts", base))
        assertEquals("https://ads.evil.com/ad.ts", M3u8FilterCore.absolutize("https://ads.evil.com/ad.ts", base))
    }

    @Test
    fun `parseExtinfDuration 边界`() {
        assertEquals(4.0, M3u8FilterCore.parseExtinfDuration("#EXTINF:4.0,")!!, 0.0001)
        assertEquals(3.0, M3u8FilterCore.parseExtinfDuration("#EXTINF:3")!!, 0.0001)
        assertTrue(M3u8FilterCore.parseExtinfDuration("#EXTINF:,") == null)
        assertTrue(M3u8FilterCore.parseExtinfDuration("#EXT-X-TARGETDURATION:10") == null)
    }
}
