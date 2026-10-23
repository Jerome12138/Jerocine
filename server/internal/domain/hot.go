package domain

import (
	"strings"
	"time"
)

// cstZone 东八区 —— 年份新鲜度跨年边界按北京时间算, 避免服务器为 UTC 时元旦前后少算一年。
var cstZone = time.FixedZone("CST", 8*3600)

// ---- 热度评分公式(0.1 分一个单位, 落到 movie.hot_score 整型列) ----
//
// 总体设计: 榜单位次严格主导(相邻位次差 100, 远大于任何微调上限), 微调只在同一位次内部排序。
const (
	// HotBoardBase 榜单第 1 名的基础分。
	HotBoardBase = 100000
	// HotBoardStep 相邻位次的分数差(100)。刻意远大于微调上限 HotTiebreakMax,
	// 保证"第 N 名的最差片"也严格高于"第 N+1 名的最佳片"。
	HotBoardStep = 100

	// HotFreshMax 年份新鲜度上限(当年上映 +20)。
	HotFreshMax = 20
	// HotOnAirDaily 日更/在播片加分(略低于当年新片, 但高于去年老片)。
	HotOnAirDaily = 14
	// HotOnAirOther 周更/月更在播片加分。
	HotOnAirOther = 12
	// HotPraiseMax 口碑分上限(豆瓣 10 分制 × 系数, 见 HotPraiseScore)。
	HotPraiseMax = 10
	// HotFallbackMax 榜外兜底分上限(0.1 分单位), 见 HotFallbackScore。
	HotFallbackMax = 30

	// hotScoreScale 榜外兜底分放大到 hot_score 列的尺度: 兜底分 0–30 映射到 0–300。
	// 榜单分本身不再放大(已是 6 万+ 量级, 见 HotBoardBase), 两种尺度在 HotScore 内分流。
	hotScoreScale = 10
)

// ---- 地区权重(2026-10-23 起并入全站 hot_score, 不再仅限轮播) ----
//
// 站内以国产内容为主, 给中国大陆适度加权; 海外内容里美国工业片受众相对广, 小幅加权;
// 日韩剧受众相对窄, 小幅降权。地区分同时进入"榜内微调"与"榜外兜底", 但任何单项都
// 不跨越 HotBoardStep(100), 榜单位次依旧严格主导。
const (
	HotAreaChinaMainland = 30  // 中国大陆
	HotAreaUS            = 10  // 美国
	HotAreaKorea         = -15 // 韩国
	HotAreaJapan         = -25 // 日本
	HotAreaMax           = HotAreaChinaMainland
)

// HotAreaScore 地区权重分。area 为影片读模型的 area 字段(如"中国大陆"/"美国"/"韩国"/"日本")。
// 中国香港/中国台湾等不含"大陆"的归为中性 0, 不做加分。
func HotAreaScore(area string) int {
	area = strings.TrimSpace(area)
	switch {
	case strings.Contains(area, "大陆") || area == "中国":
		return HotAreaChinaMainland
	case strings.Contains(area, "美国"):
		return HotAreaUS
	case strings.Contains(area, "韩国"):
		return HotAreaKorea
	case strings.Contains(area, "日本"):
		return HotAreaJapan
	default:
		return 0
	}
}

// HotTiebreakMax 榜内微调上限 = 口碑(30) + 在播(14) + 年份新鲜度(20) + 地区(30)。
// 必须 < HotBoardStep(100), 否则相邻位次可能被微调翻转。
const HotTiebreakMax = 3*HotPraiseMax + HotOnAirDaily + HotFreshMax + HotAreaMax // = 94

// HotScore 一部影片的全站热度分。
//
//	rank > 0(在榜): 榜单位次主导, 位次差 100, 微调只在同位次内排序;
//	rank = 0(掉榜/未上榜): 0–30 的兜底分 ×10, 随内容(年份/在播/口碑/地区)自动更新。
//
// 2026-10-23 起新增 area 入参: 地区权重在全站热度分中生效(见 HotAreaScore)。
func HotScore(rank int, area string, year int, remarks string, dbScore float64, now time.Time) int {
	if rank <= 0 {
		return hotScoreScale * HotFallbackScore(year, area, remarks, dbScore, now)
	}
	return HotBoardBase - (rank-1)*HotBoardStep + HotBoardTiebreak(year, area, remarks, dbScore, now)
}

// HotBoardTiebreak 榜内同位次的次序微调(0–HotTiebreakMax):
// 口碑(×3, 0–30) + 在播(0–14) + 年份新鲜度(全额 0–20, 2026-10-23 起不再 ÷3) + 地区(−25~+30)。
// 钳制在 [0, HotTiebreakMax]: 日本/韩国等负地区分最多把微调压到 0, 不产生负的微调分。
func HotBoardTiebreak(year int, area, remarks string, dbScore float64, now time.Time) int {
	s := HotPraiseScore(dbScore)*3 + HotOnAirScore(remarks) + HotYearFresh(year, now) + HotAreaScore(area)
	if s < 0 {
		return 0
	}
	if s > HotTiebreakMax {
		return HotTiebreakMax
	}
	return s
}

// HotFallbackScore 榜外兜底分(0–HotFallbackMax+地区上限):
// 年份新鲜度 / 在播加分取大者, 加口碑分, 再叠加地区分; 最终钳制为非负。
func HotFallbackScore(year int, area, remarks string, dbScore float64, now time.Time) int {
	fresh := HotYearFresh(year, now)
	onAir := HotOnAirScore(remarks)
	base := fresh
	if onAir > base {
		base = onAir
	}
	base += HotPraiseScore(dbScore)
	if base < 0 {
		base = 0
	}
	if base > HotFallbackMax {
		base = HotFallbackMax
	}
	s := base + HotAreaScore(area)
	if s < 0 {
		return 0
	}
	// 兜底分上限随最大地区加分抬高, 仍远低于最末位榜单分(×10 后也只 600, 榜单末位 6.9 万+)。
	if cap := HotFallbackMax + HotAreaMax; s > cap {
		return cap
	}
	return s
}

// HotYearFresh 年份新鲜度: 当年上映 +20, 每老一年 −2, 10 年及更早归零。
// 跨年边界(元旦前后)按东八区判定"今年"(与采集/榜单口径一致)。
func HotYearFresh(year int, now time.Time) int {
	if year <= 0 {
		return 0
	}
	age := now.In(cstZone).Year() - year
	if age < 0 {
		age = 0 // 标了明年(跨年档)按最新处理
	}
	s := HotFreshMax - 2*age
	if s < 0 {
		return 0
	}
	return s
}

// HotOnAirScore 在播加分: 日更 14 / 周更·月更 12 / 已完结或静态 0。
//
// 判定优先级(2026-10-23 审查修正): "更新至/更新到/连载"是在播铁证, 必须先于"全"字判定 ——
// 防"更新至全40集"这类混排形态被"含全即完结"误杀(线上实测当前无此形态, 纯防御;
// "全集/N集全/12期全"等无在播字样的才是完结, 落到兜底 0)。
func HotOnAirScore(remarks string) int {
	r := strings.TrimSpace(remarks)
	if r == "" || strings.Contains(r, "完结") {
		return 0
	}
	if strings.Contains(r, "更新至") || strings.Contains(r, "连载") || strings.Contains(r, "更新到") {
		// 日更(综艺/短剧按"期")略高于周更月更, 其余在播统一 12。
		if strings.Contains(r, "期") {
			return HotOnAirDaily
		}
		return HotOnAirOther
	}
	return 0
}

// HotPraiseScore 豆瓣口碑分(0–10): 直接取 db_score 的整数部分; 无评分(db_score<=0)为 0。
func HotPraiseScore(dbScore float64) int {
	if dbScore <= 0 {
		return 0
	}
	s := int(dbScore)
	if s > HotPraiseMax {
		return HotPraiseMax
	}
	if s < 0 {
		return 0
	}
	return s
}
