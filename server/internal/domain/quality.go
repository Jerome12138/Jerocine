package domain

import (
	"regexp"
	"strings"
)

// 枪版(TC/TS/CAM 抢先版)识别 —— 只用于热榜与轮播的降级, 不参与全站排序。
//
// 线上实测(2026-10-23 全库 16 万部): 真枪版标识**只出现在 remarks 字段**, 且为独立 token
// ("TC"/"HDTC"), 共 15 部、全是当年新片; 而 name 字段里出现的 TS/CAM 全是英文巧合
// (BTS / Switch / Heart Catch / CRISIS 等)。因此:
//   - 只扫 remarks, 不扫片名;
//   - 英文标识必须是 ASCII 词边界内的完整词(靠 \b 排除 BTS、Switch 这类);
//   - HD / BD / 蓝光是正常高清/蓝光版本, 不降级。
var telesyncTokenRe = regexp.MustCompile(`(?i)\b(?:hdtc|camrip|cam|tc|ts)\b`)

// IsLowQualityTelesync 判定 remarks 是否标识本片为枪版/抢先版。
//
// 中文标识直接子串命中("枪版" / "抢先", 覆盖"抢先版""抢先看");
// 英文标识走 ASCII 词边界正则, "TS版"/"HDTC" 这类中英混排也能命中(中文字符是 ASCII 边界)。
func IsLowQualityTelesync(remarks string) bool {
	remarks = strings.TrimSpace(remarks)
	if remarks == "" {
		return false
	}
	if strings.Contains(remarks, "枪版") || strings.Contains(remarks, "抢先") {
		return true
	}
	return telesyncTokenRe.MatchString(remarks)
}

// ---- 枪版在热榜/轮播中的降级幅度 ----
const (
	// HotBoardTCRankDrop 单榜视图: 枪版片有效位次后移位数(约降一个位次)。
	HotBoardTCRankDrop = 1
	// HotBoardTCStepDrop 全榜视图(按 hot_score 排序): 枪版片动态减去一个位次步长。
	HotBoardTCStepDrop = HotBoardStep
	// HotBoardTCCandidateDrop 轮播候选: 枪版片减分(大于同榜位次差 200 内的回血, 见下)。
	HotBoardTCCandidateDrop = 200
	// HotBoardTCMultiBoardBonus 枪版片每多命中一个榜单的回血加分。
	HotBoardTCMultiBoardBonus = 60
	// HotBoardTCMultiBoardCap 多榜回血上限。
	HotBoardTCMultiBoardCap = 120
)

// HotBoardTCMultiBoardBonusScore 枪版片按命中榜数计算的多榜回血分(封顶 HotBoardTCMultiBoardCap)。
// boardCount 为该片命中的榜单数(含主榜); 仅 1 个榜时回血量为 0。
func HotBoardTCMultiBoardBonusScore(boardCount int) int {
	if boardCount <= 1 {
		return 0
	}
	b := (boardCount - 1) * HotBoardTCMultiBoardBonus
	if b > HotBoardTCMultiBoardCap {
		b = HotBoardTCMultiBoardCap
	}
	return b
}
