package domain

import (
	"math"
	"strings"
	"time"
)

// 榜单热度合成分 —— 纯计算, 无副作用, 唯一来源。
//
//	hot_score = 榜单位次分(0 或 60–100) + 更新活跃分(0–20) + 口碑分(0–10)
//
// 分项设计见 docs/榜单热度方案-2026-09-11.md §3.1; 关键性质是**榜内下限(60) > 榜外上限(30)**,
// 于是榜内片必然排在榜外片之前, 两段互不污染 —— 外部榜单整体挂掉时 hot_score 退化为
// "最近上映 + 高分" 的兜底分, 排序不崩、覆盖 100%、零外部请求。
const (
	// HotRankTop / HotRankFloor 榜内位次分区间: 第 1 名 100, 榜单末位 60。
	HotRankTop   = 100
	HotRankFloor = 60
	// HotFreshMax 年份新鲜度上限。当前年 = 满分, 之后每老一年减 1。
	HotFreshMax = 20
	// HotOnAirDaily 在播加分(日更/周更节目: remarks 形如 "更新至20260911期")。
	HotOnAirDaily = 14
	// HotOnAirOther 在播加分(其他在播: remarks 形如 "更新至第20集")。
	HotOnAirOther = 12
	// HotPraiseMax 口碑分上限。
	HotPraiseMax = 10
	// HotFallbackMax 榜外兜底分上限 = max(年份新鲜度, 在播加分) + 口碑分。
	HotFallbackMax = HotFreshMax + HotPraiseMax
)

// HotRankScore 把榜位折算成 60–100 的位次分: 第 1 名 100, 末位 60。
// depth 是该榜单的实测深度(N 条), rank > depth 时按末位处理(理论不可达, 防御性收敛)。
func HotRankScore(rank, depth int) int {
	if rank <= 0 {
		return 0
	}
	if depth <= 1 {
		return HotRankTop
	}
	if rank > depth {
		rank = depth
	}
	return HotRankTop - (HotRankTop-HotRankFloor)*(rank-1)/(depth-1)
}

// HotFallbackScore 榜外兜底分(0–30) = max(年份新鲜度, 在播加分) + 口碑分。
//
// **取大而非相加**是刻意的: "年份老但一直在更新"的连载内容不能因为年份掉队 ——
// 2004 年的在播综艺(HK 综艺, remarks="更新至20260911期")年份新鲜度为 0, 靠在播加分 14 顶上。
// 这类片实测只有 109 部(2020 年前仍在播的占在播库 5.2%), 但正是最需要豁免年份的一批。
func HotFallbackScore(year int, remarks string, dbScore float64, now time.Time) int {
	return max(HotYearFresh(year, now), HotOnAirBonus(remarks)) + HotPraiseScore(dbScore)
}

// hotScoreScale hot_score 的存储单位: 0.1 分。
//
// 为什么要放大 10 倍: 位次分要覆盖到榜单深度(movie_hot_gaia 实测 308 条), 而它的分值跨度只有
// 40 分 —— 按 1 分制存, 整条榜单只有 41 个档位, 前 8 名必然同分, "热度榜"的前几名就退化成年份序,
// 名不副实。放大后 400 个档位, 每一个位次都能区分; 兜底分顺带从 31 档变成 301 档。
// 语义完全不变, 只是单位: 榜内 600–1300, 榜外 0–300, 榜内下限仍远高于榜外上限。
const hotScoreScale = 10

// HotScore 合成热度分 —— **写库就用这个函数**(存储单位 0.1 分):
//
//	榜内: (位次分 60–100 + 兜底分 0–30) × 10  → 600–1300
//	榜外: 兜底分 0–30 × 10                     → 0–300
//
// 分项的算式见 HotRankScore / HotFallbackScore(单位仍是"分", 便于与方案文档对照)。
func HotScore(rank, depth, year int, remarks string, dbScore float64, now time.Time) int {
	fallback := hotScoreScale * HotFallbackScore(year, remarks, dbScore, now)
	if rank <= 0 {
		return fallback
	}
	return hotRankScoreScaled(rank, depth) + fallback
}

// hotRankScoreScaled 与 HotRankScore 同形, 但**在 0.1 分单位下先放大再整除**。
//
// 这一层是必需的, 不是重复: HotRankScore 的斜率 `40/(depth-1)` 在 1 分单位下是**整数除法**,
// 深度 308 时每名只摊到 0.13 分 —— 截断后前 8 名(0.13×7≈0.91)全部落在同一档, 而"位次分辨率"
// 正是放大 10 倍的全部理由(见 hotScoreScale 注释)。先乘以 10 再除, 每名摊 1.3 档, 位次即可区分。
func hotRankScoreScaled(rank, depth int) int {
	if rank <= 0 {
		return 0
	}
	if depth <= 1 {
		return hotScoreScale * HotRankTop
	}
	if rank > depth {
		rank = depth
	}
	span := hotScoreScale * (HotRankTop - HotRankFloor)
	return hotScoreScale*HotRankTop - span*(rank-1)/(depth-1)
}

// HotYearFresh 年份新鲜度: 当前年满分 20, 每老一年减 1, 到 0 为止; 年份未知(0)不给分。
func HotYearFresh(year int, now time.Time) int {
	if year <= 0 {
		return 0
	}
	diff := now.Year() - year
	if diff <= 0 {
		return HotFreshMax
	}
	if diff >= HotFreshMax {
		return 0
	}
	return HotFreshMax - diff
}

// HotOnAirBonus 在播加分, 按 remarks 形态分两档。
//
// 判据是**片名之外的内容字段**(remarks), 不依赖任何分类 id —— 切源重建分类树后依然成立。
// 实测: 含"期"的 remarks 全部以"更新至"开头, 故无需单独判定 "%期"。
func HotOnAirBonus(remarks string) int {
	r := strings.TrimSpace(remarks)
	if !strings.HasPrefix(r, "更新至") {
		return 0
	}
	if strings.HasSuffix(r, "期") {
		return HotOnAirDaily
	}
	return HotOnAirOther
}

// HotPraiseScore 口碑分 = min(10, round(豆瓣分)); 无评分(0)不给分。
func HotPraiseScore(dbScore float64) int {
	if dbScore <= 0 {
		return 0
	}
	v := int(math.Round(dbScore))
	if v > HotPraiseMax {
		return HotPraiseMax
	}
	return v
}
