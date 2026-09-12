package domain

import (
	"math"
	"strings"
	"time"
)

// 榜单热度合成分 —— 纯计算, 无副作用, 唯一来源。
//
//	榜内: hot_score = HotBoardBase - (rank-1)×HotBoardStep + tiebreak(0–60)
//	榜外: hot_score = 兜底分(0–30) × 10 → 0–300
//
// 设计要点(2026-09-12 调整, 取代旧"位次 60–100 + 兜底相加"模型):
//
//  1. **位次严格主导**。旧模型里位次分跨度只有 40 分, 而兜底分(年份+在播+口碑)最高 30 分 ——
//     无评分的高位次片会被"稍低位次但评分高"的片反超(实测 No.2 被压到 No.30 之后)。
//     新模型每名之间固定隔 HotBoardStep(=10 分), 榜内排序与豆瓣榜单完全同序,
//     评分/在播/年份只在**同一位次**内做次序微调(多榜并列 No.2 时评分高的在前)。
//  2. **评分/在播聚焦榜外兜底**。榜外片没有位次可依, 靠"最近上映 + 在播 + 高分"排兜底序;
//     榜内片已有豆瓣权威位次, 这些本地信号降为 tiebreak, 权重压到不可能越级。
//  3. **榜内下限 > 榜外上限**的不变式保留: 深度 308 的榜末位 ≈ 69300 ≫ 300。
//     HotBoardBase 支持的最大榜单深度约 (HotBoardBase-300)/HotBoardStep ≈ 997, 远超实测 308。
const (
	// HotBoardBase 榜内位次分基数: 第 1 名的位次部分。
	HotBoardBase = 100000
	// HotBoardStep 相邻榜位的分差(0.1 分单位, 即每名 10 分)。
	// tiebreak 上限 60 < 100, 保证任何 tiebreak 组合都不能让低位次反超高位次。
	HotBoardStep = 100
	// HotFreshMax 年份新鲜度上限(榜外兜底用)。当前年 = 满分, 之后每老一年减 1。
	HotFreshMax = 20
	// HotOnAirDaily 在播加分(日更/周更节目: remarks 形如 "更新至20260911期")。
	HotOnAirDaily = 14
	// HotOnAirOther 在播加分(其他在播: remarks 形如 "更新至第20集")。
	HotOnAirOther = 12
	// HotPraiseMax 口碑分上限。
	HotPraiseMax = 10
	// HotFallbackMax 榜外兜底分上限 = max(年份新鲜度, 在播加分) + 口碑分。
	HotFallbackMax = HotFreshMax + HotPraiseMax
	// HotTiebreakMax 榜内同位次 tiebreak 上限(必须 < HotBoardStep, 测试把关)。
	// 口碑×4(0–40) + 在播(0–14) + 年份新鲜度/3(0–6)。
	HotTiebreakMax = 4*HotPraiseMax + HotOnAirDaily + HotFreshMax/3
)

// hotScoreScale 榜外兜底分的存储单位: 0.1 分(0–30 分 → 0–300 档, 与位次分同单位)。
const hotScoreScale = 10

// HotScore 合成热度分 —— **写库就用这个函数**(存储单位 0.1 分):
//
//	榜内: HotBoardBase - (rank-1)×HotBoardStep + 同位次 tiebreak(0–60)
//	榜外: 兜底分 0–30 × 10 → 0–300
//
// 旧签名的 depth 参数已移除(新模型位次差是固定档差, 不按榜单深度归一化);
// 榜单深度只用于抓取完整性校验(见 hot_service)。
func HotScore(rank, year int, remarks string, dbScore float64, now time.Time) int {
	if rank <= 0 {
		return hotScoreScale * HotFallbackScore(year, remarks, dbScore, now)
	}
	return HotBoardBase - (rank-1)*HotBoardStep + HotBoardTiebreak(year, remarks, dbScore, now)
}

// HotBoardTiebreak 榜内同位次的次序微调(0–HotTiebreakMax):
// 口碑为主(×4), 在播次之, 年份新鲜度最弱(除 3)。
// 只在**同一位次**内生效 —— 任意组合都小于相邻位次的 HotBoardStep, 不可能越级。
func HotBoardTiebreak(year int, remarks string, dbScore float64, now time.Time) int {
	return HotPraiseScore(dbScore)*4 + HotOnAirBonus(remarks) + HotYearFresh(year, now)/3
}

// HotFallbackScore 榜外兜底分(0–30) = max(年份新鲜度, 在播加分) + 口碑分。
//
// **取大而非相加**是刻意的: "年份老但一直在更新"的连载内容不能因为年份掉队 ——
// 2004 年的在播综艺(HK 综艺, remarks="更新至20260911期")年份新鲜度为 0, 靠在播加分 14 顶上。
// 这类片实测只有 109 部(2020 年前仍在播的占在播库 5.2%), 但正是最需要豁免年份的一批。
func HotFallbackScore(year int, remarks string, dbScore float64, now time.Time) int {
	return max(HotYearFresh(year, now), HotOnAirBonus(remarks)) + HotPraiseScore(dbScore)
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
