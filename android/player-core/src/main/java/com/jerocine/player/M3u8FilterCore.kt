package com.jerocine.player

import java.net.URI

/**
 * m3u8 去广告过滤内核 —— Kotlin 移植(Go: server/internal/service/m3u8_service.go rewriteM3u8Core)。
 *
 * 纯 Kotlin、零 Android 依赖(JVM 可单测)。算法规范见 docs/m3u8-filter-ALGORITHM.md;
 * 三端(Go/TS/Kotlin)一致性由 fixtures/m3u8-filter/cases.json 锁死, 改规则必须三端同步重跑夹具。
 *
 * client 语义: 仅绝对化 + 过滤(可选), 子播放列表只绝对化、不代理回环。
 * 与 TS 版本(web/src/utils/m3u8Filter.ts)逐行对齐; 与 Go 有歧义时以 Go 为真相。
 */
object M3u8FilterCore {

    // ---- 阈值常量(三端一致, ALGORITHM.md §8) ----
    const val AD_NUM_MIN_SEGS = 10
    const val AD_NUM_MIN_COVERAGE = 0.80
    const val AD_NUM_MIN_CONTENT = 0.60
    const val AD_NUM_MAX_DROP_FRAC = 0.30
    const val AD_CROSS_MIN_SEGS = 10
    const val AD_CROSS_MAX_DROP_FRAC = 0.30
    const val AD_SHORT_MAX_DUR = 1.0
    const val AD_SHORT_SKIP_HEAD = 4
    const val AD_SHORT_BURST_MIN_DUR = 3.0
    const val M3U8_SAMPLE_MAX = 5

    /** 过滤结果 —— Java 侧经 getText()/getFiltered() 等访问。 */
    class Result {
        /** 重写后的清单文本(逐行处理, 末尾空行保留)。 */
        val text: String
        /** 被判为广告剔除的分片数。 */
        val filtered: Int
        /** 本列表内真实分片总数。 */
        val total: Int
        /** 跨域/异目录命中剔除数(排查用)。 */
        val cross: Int
        /** 编号跳脱命中剔除数(排查用)。 */
        val num: Int
        /** 极短簇命中剔除数(排查用)。 */
        val short: Int
        /** 是否含 #EXT-X-DISCONTINUITY(排查用)。 */
        val hasDisc: Boolean

        constructor(
            text: String,
            filtered: Int,
            total: Int,
            cross: Int,
            num: Int,
            short: Int,
            hasDisc: Boolean
        ) {
            this.text = text
            this.filtered = filtered
            this.total = total
            this.cross = cross
            this.num = num
            this.short = short
            this.hasDisc = hasDisc
        }
    }

    /**
     * 主入口。base 可空: 解析失败时相对 URI 原样保留(与 TS absolutize 失败原样一致)。
     * 永远不抛异常 —— 调用方(播放/下载)无需 try 即可本地优先。
     */
    @JvmStatic
    fun rewriteM3u8(body: String, base: String?, filterAds: Boolean): Result {
        val lines = body.split('\n')
        val baseUri = safeUri(base)
        val dominant = dominantSegmentKey(lines, baseUri)

        var adByIdx: Set<Int>? = null
        var shortByIdx: Set<Int>? = null
        var crossByIdx: Set<Int>? = null
        if (filterAds) {
            val segURIs = ArrayList<String>()
            val segDurs = ArrayList<Double>()
            var pendingDur = -1.0
            for (ln in lines) {
                val t = ln.trim()
                if (t.isEmpty()) continue
                if (t.startsWith("#EXTINF")) {
                    val d = parseExtinfDuration(t)
                    if (d != null) pendingDur = d
                    continue
                }
                if (t.startsWith("#")) continue
                val abs = absolutize(t, baseUri)
                if (isPlaylistURI(abs)) continue
                segURIs.add(abs)
                segDurs.add(pendingDur)
                pendingDur = -1.0
            }
            adByIdx = numberOutlierAds(segURIs)
            shortByIdx = shortBurstAds(segDurs)
            crossByIdx = crossDomainAds(segURIs, dominant)
            // 时长异常(z-score)预警仅服务端日志用(Go stats.DurOutlier/DurSample);
            // 客户端 Result 不暴露该统计 → 不在本地内核中计算(避免白跑一趟 O(n))
        }

        val out = ArrayList<String>(lines.size)
        var pendingExtinf = ""
        var filtered = 0
        var total = 0
        var cross = 0
        var num = 0
        var short = 0
        var hasDisc = false
        var segIdx = -1

        for (ln in lines) {
            val t = ln.trim()
            if (t.isEmpty()) {
                out.add(ln)
                continue
            }
            if (t.startsWith("#EXTINF")) {
                pendingExtinf = ln
                continue
            }
            if (t.startsWith("#")) {
                if (t.startsWith("#EXT-X-DISCONTINUITY")) hasDisc = true
                out.add(rewriteTagURI(ln, baseUri))
                continue
            }
            val abs = absolutize(t, baseUri)
            if (isPlaylistURI(abs)) {
                if (pendingExtinf.isNotEmpty()) {
                    out.add(pendingExtinf)
                    pendingExtinf = ""
                }
                out.add(abs)
                continue
            }
            total += 1
            segIdx += 1
            if (filterAds && pendingExtinf.isNotEmpty()) {
                val crossDrop = crossByIdx?.contains(segIdx) == true
                val numDrop = !crossDrop && adByIdx?.contains(segIdx) == true
                val shortDrop = !crossDrop && !numDrop && shortByIdx?.contains(segIdx) == true
                if (crossDrop || numDrop || shortDrop) {
                    pendingExtinf = ""
                    filtered += 1
                    when {
                        crossDrop -> cross += 1
                        numDrop -> num += 1
                        else -> short += 1
                    }
                    continue
                }
            }
            if (pendingExtinf.isNotEmpty()) {
                out.add(pendingExtinf)
                pendingExtinf = ""
            }
            out.add(abs)
        }

        return Result(out.joinToString("\n"), filtered, total, cross, num, short, hasDisc)
    }

    // ============================ 内部纯函数 ============================

    private fun safeUri(s: String?): URI? {
        if (s == null || s.isEmpty()) return null
        return try {
            URI.create(s)
        } catch (e: Exception) {
            null
        }
    }

    /** RFC3986 相对解析: http(s) 绝对原样; 其余用 base 解析; 失败/无 base 原样返回。 */
    @JvmStatic
    fun absolutize(ref: String, base: URI?): String {
        if (ref.startsWith("http://") || ref.startsWith("https://")) return ref
        if (base == null) return ref
        return try {
            base.resolve(ref).toString()
        } catch (e: Exception) {
            // java.net.URI 拒绝未转义字符(空格/中文/| 等): 委托 PlayerUrls 的手工 RFC3986 兜底
            // (协议相对/根相对/路径相对 + ".." 消化), 否则过滤产物留相对引用 ——
            // 下载落盘 file:// 离线播放时这些分片会脱靶。
            PlayerUrls.absolutize(ref, base.toString()) ?: ref
        }
    }

    /** 忽略 query/fragment, path 以 .m3u8 结尾(大小写不敏感)。 */
    @JvmStatic
    fun isPlaylistURI(rawURL: String): Boolean {
        val u = try {
            URI(rawURL)
        } catch (e: Exception) {
            return false
        }
        val p = u.path ?: return false
        return p.lowercase().endsWith(".m3u8")
    }

    /** 解析 #EXTINF 时长(秒); 失败/空/负值返回 null。 */
    @JvmStatic
    fun parseExtinfDuration(line: String): Double? {
        if (!line.startsWith("#EXTINF")) return null
        val i = line.indexOf(':')
        if (i < 0) return null
        var rest = line.substring(i + 1).trim()
        val j = rest.indexOf(',')
        if (j >= 0) rest = rest.substring(0, j).trim()
        if (rest.isEmpty()) return null
        val d = rest.toDoubleOrNull() ?: return null
        // 对齐 TS 内核的 Number.isFinite 拒绝: "Infinity"/"NaN" 字面量不是合法时长
        if (d < 0 || !d.isFinite()) return null
        return d
    }

    /** 取分片文件名(去 query/扩展名)末尾连续数字; 无则 -1。超 15 位截末 15 位。 */
    @JvmStatic
    fun segNumber(rawURL: String): Long {
        var p = rawURL
        val u = try {
            URI(rawURL)
        } catch (e: Exception) {
            null
        }
        if (u != null && u.path != null) {
            p = u.path
        } else {
            // URI 严格解析失败(含空格/中文等非法字符; Go url.Parse 对这些字符是宽松解析,
            // 会成功取到 path) → 手动剥 query/fragment 后按 path 段处理 ——
            // 防止 query 尾数字污染编号判定(如 "seg0.ts?token=123" 解析失败时误取 123 而非 0,
            // 导致 num 维度把正常分片误剔)。
            var q = rawURL
            val hq = q.indexOf('?')
            if (hq >= 0) q = q.substring(0, hq)
            val hf = q.indexOf('#')
            if (hf >= 0) q = q.substring(0, hf)
            p = q
        }
        val slash = p.lastIndexOf('/')
        if (slash >= 0) p = p.substring(slash + 1)
        val dot = p.lastIndexOf('.')
        if (dot >= 0) p = p.substring(0, dot)
        var j = p.length
        while (j > 0 && p[j - 1] in '0'..'9') j -= 1
        if (j == p.length) return -1
        var digits = p.substring(j)
        if (digits.length > 15) digits = digits.substring(digits.length - 15)
        return digits.toLongOrNull() ?: -1
    }

    /** 排序数列相邻差值的中位数(估计正片编号步长)。 */
    @JvmStatic
    fun medianGap(sorted: LongArray): Long {
        if (sorted.size < 2) return 0
        val gaps = LongArray(sorted.size - 1)
        for (i in 1 until sorted.size) gaps[i - 1] = sorted[i] - sorted[i - 1]
        gaps.sort()
        return gaps[gaps.size / 2]
    }

    /**
     * 分片归属键 = host + 内容目录(剥文件名后取前两段)。
     * 必须先剥文件名再取目录段: 目录只有一层时第二段会取到文件名, 每片各自成键
     * 互为异键 → 正常分片被整批当跨域广告删掉(segKey 回归用例)。
     */
    @JvmStatic
    fun segKey(abs: String): String {
        val u = try {
            URI(abs)
        } catch (e: Exception) {
            // URI 严格解析失败(含空格/中文等) → 返回 "": 该片不计入任何归属键。
            // 与 Go url.Parse(宽松, 能取到 host)的差异: 此处跨域维度对这些异常片失效,
            // 方向是"漏过滤"而非误删(crossDomainAds 对空键直接跳过), 安全, 不为此引入手动 host 解析。
            return ""
        }
        val host = u.host ?: return ""
        var dir = u.path ?: return ""
        val slash = dir.lastIndexOf('/')
        if (slash >= 0) dir = dir.substring(0, slash) else dir = ""
        val parts = dir.trim('/').split('/').filter { it.isNotEmpty() }
        return when {
            parts.size >= 2 -> "$host/${parts[0]}/${parts[1]}"
            parts.size == 1 -> "$host/${parts[0]}"
            else -> host
        }
    }

    /** 统计本列表内分片(非子列表)归属键, 返回出现最多者(真实内容)。 */
    @JvmStatic
    fun dominantSegmentKey(lines: List<String>, base: URI?): String {
        // LinkedHashMap: 平票取先出现者(对齐 TS 按插入序); 三端一致性不依赖散列序
        val counts = LinkedHashMap<String, Int>()
        for (ln in lines) {
            val t = ln.trim()
            if (t.isEmpty() || t.startsWith("#")) continue
            val abs = absolutize(t, base)
            if (isPlaylistURI(abs)) continue
            val k = segKey(abs)
            if (k.isNotEmpty()) counts[k] = (counts[k] ?: 0) + 1
        }
        var best = ""
        var bestN = 0
        for ((k, n) in counts) {
            if (n > bestN) {
                best = k
                bestN = n
            }
        }
        return best
    }

    /** 跨域维度: 归属键 ≠ 主导键 → 广告; 异键占比超上限(长表)→ 整表不剔返回 null。 */
    @JvmStatic
    fun crossDomainAds(segURIs: List<String>, dominant: String): Set<Int>? {
        if (dominant.isEmpty() || segURIs.isEmpty()) return null
        val ads = HashSet<Int>()
        for (i in segURIs.indices) {
            val k = segKey(segURIs[i])
            if (k.isNotEmpty() && k != dominant) ads.add(i)
        }
        if (ads.isEmpty()) return null
        if (segURIs.size >= AD_CROSS_MIN_SEGS && ads.size > segURIs.size * AD_CROSS_MAX_DROP_FRAC) return null
        return ads
    }

    /** 编号跳脱: 正片尾号在紧凑主簇, 广告远离; 不满足条件返回 null(不剔)。 */
    @JvmStatic
    fun numberOutlierAds(segURIs: List<String>): Set<Int>? {
        val n = segURIs.size
        if (n < AD_NUM_MIN_SEGS) return null
        val nums = ArrayList<Long>(n)
        var numbered = 0
        for (u in segURIs) {
            val v = segNumber(u)
            nums.add(v)
            if (v >= 0) numbered += 1
        }
        if (numbered < n * AD_NUM_MIN_COVERAGE) return null
        val distinct = nums.filter { it >= 0 }.distinct().sorted().toLongArray()
        if (distinct.size < 2) return null
        val thr = Math.max(1000L, 1000L * medianGap(distinct))
        var lo = distinct[0]
        var hi = distinct[0]
        var bestLo = lo
        var bestHi = hi
        var bestCount = 0
        fun flush(l: Long, h: Long) {
            var cnt = 0
            for (x in nums) {
                if (x in l..h) cnt += 1
            }
            if (cnt > bestCount) {
                bestLo = l
                bestHi = h
                bestCount = cnt
            }
        }
        for (i in 1 until distinct.size) {
            val cur = distinct[i]
            if (cur - distinct[i - 1] > thr) {
                flush(lo, hi)
                lo = cur
            }
            hi = cur
        }
        flush(lo, hi)
        if (bestCount < n * AD_NUM_MIN_CONTENT) return null
        val ads = HashSet<Int>()
        for (i in nums.indices) {
            val x = nums[i]
            if (x >= 0 && (x < bestLo || x > bestHi)) ads.add(i)
        }
        if (ads.size > n * AD_NUM_MAX_DROP_FRAC) return null
        return ads
    }

    /** 连续极短分片簇(广告密集切片); 开头 adShortSkipHead 段豁免。 */
    @JvmStatic
    fun shortBurstAds(segDurs: List<Double>): Set<Int> {
        val out = HashSet<Int>()
        val n = segDurs.size
        var i = 0
        while (i < n) {
            val di = segDurs[i]
            if (di >= 0 && di < AD_SHORT_MAX_DUR && i > AD_SHORT_SKIP_HEAD) {
                var j = i
                var burst = 0.0
                while (j < n) {
                    val dj = segDurs[j]
                    if (!(dj >= 0 && dj < AD_SHORT_MAX_DUR && j > AD_SHORT_SKIP_HEAD)) break
                    burst += dj
                    j += 1
                }
                if (burst >= AD_SHORT_BURST_MIN_DUR) {
                    for (k in i until j) out.add(k)
                }
                i = j
            } else {
                i += 1
            }
        }
        return out
    }

    /** 时长异常分片(z-score>3) —— 仅预警不剔除(实现保留供排查, Result 不暴露)。 */
    @JvmStatic
    fun durationOutliers(segDurs: List<Double>, segURIs: List<String>): Pair<Set<Int>, List<String>> {
        val known = segDurs.filter { it >= 0 }
        if (known.size < AD_NUM_MIN_SEGS) return Pair(emptySet(), emptyList())
        val avg = known.sum() / known.size
        val variance = known.map { (it - avg) * (it - avg) }.sum() / known.size
        val std = Math.sqrt(variance)
        if (std <= 0) return Pair(emptySet(), emptyList())
        val outliers = HashSet<Int>()
        val samples = ArrayList<String>()
        for (i in segDurs.indices) {
            val d = segDurs[i]
            if (d < 0) continue
            if (Math.abs(d - avg) > 3 * std) {
                outliers.add(i)
                if (samples.size < M3U8_SAMPLE_MAX && i < segURIs.size) samples.add(segURIs[i])
            }
        }
        return Pair(outliers, samples)
    }

    /** 改写标签行内 URI="..." 属性(client 语义: 仅绝对化)。 */
    @JvmStatic
    fun rewriteTagURI(line: String, base: URI?): String {
        val m = Regex("URI=\"([^\"]+)\"").find(line) ?: return line
        val abs = absolutize(m.groupValues[1], base)
        val full = m.value
        val inner = m.groupValues[1]
        val start = m.range.first + full.indexOf(inner)
        return line.substring(0, start) + abs + line.substring(start + inner.length)
    }
}
