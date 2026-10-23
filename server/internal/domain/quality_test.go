package domain

import (
	"regexp"
	"strings"
	"testing"
)

// TestIsLowQualityTelesync 枪版识别: 只认 remarks 独立 token, 排除英文巧合与正常高清标识。
func TestIsLowQualityTelesync(t *testing.T) {
	positive := []string{
		"HDTC", "TC", "TS", "HDTC版", "TS版", "TC版", "CAM", "CAMRIP",
		"高清 TC", "HD CAM 抢先", "枪版", "抢先版", "抢先看", "TC抢先",
	}
	for _, r := range positive {
		if !IsLowQualityTelesync(r) {
			t.Errorf("IsLowQualityTelesync(%q) = false, want true", r)
		}
	}
	negative := []string{
		"", "HD", "HD高清", "BD", "蓝光", "1080P", "更新至20260911期", "已完结",
		// 线上 name 字段实测的英文巧合, 不能误判:
		"BTS幕后", "Switch", "ATCS", "TSCAM", "Heart Catch", "CRISIS",
	}
	for _, r := range negative {
		if IsLowQualityTelesync(r) {
			t.Errorf("IsLowQualityTelesync(%q) = true, want false", r)
		}
	}
}

// ---- SQL 侧口径等价锁定 ----
//
// repository/mysql.search_repo.go 的 hotBoardTCExpr 用 SQL 正则做同样的枪版判定。
// 实测 MySQL 8(ICU)的 [[:alnum:]] 会把中文算作字母数字, 故 SQL 边界改用 ASCII 集合
// [^0-9A-Za-z_], 与 Go 侧 \b(\w=[0-9A-Za-z_]) 严格一致。这里用 Go 正则模拟 SQL 表达式,
// 对一批边界样例断言两种口径结果完全一致 —— 任何一侧改动导致分叉都会被此测试拦住。

var telesyncSQLEquivRe = regexp.MustCompile(`(?i)(^|[^0-9A-Za-z_])(HDTC|CAMRIP|CAM|TC|TS)([^0-9A-Za-z_]|$)`)

// telesyncSQLEquiv 模拟修复后的 hotBoardTCExpr: 中文子串走 LIKE, 英文走 ASCII 词边界。
func telesyncSQLEquiv(remarks string) bool {
	if strings.Contains(remarks, "枪版") || strings.Contains(remarks, "抢先") {
		return true
	}
	return telesyncSQLEquivRe.MatchString(remarks)
}

func TestTelesyncSQLSemanticsEquivalent(t *testing.T) {
	cases := []string{
		// 中英紧贴(曾因 [[:alnum:]] 匹配中文而分叉的场景, 修复后两侧一致):
		"TC高清", "高清TC", "TC第1集", "第TC集", "高清TC版", "TC-高清", "TC 高清", "高TC抢",
		// 独立 token / 空格 / 符号边界:
		"TC", "HDTC", "CAMRIP", "CAM", "TS", "HDTC版", "TS版", "TC版", "高清 TC", "HD CAM 抢先",
		// 中文子串:
		"枪版", "抢先版", "抢先看", "TC抢先",
		// 英文巧合(两侧必须都不命中):
		"CAMERA", "BTS", "BTS幕后", "Switch", "VICTORIA", "WATCH", "TCL", "tcp", "ATCS", "TSCAM",
		"Heart Catch", "CRISIS", "TCU", "CTCF", "backup", "HDTV",
		// 正常高清/无标识:
		"", "HD", "HD高清", "BD", "蓝光", "1080P", "更新至20260911期", "已完结", "高清", "国语",
	}
	for _, r := range cases {
		goGot := IsLowQualityTelesync(r)
		sqlGot := telesyncSQLEquiv(r)
		if goGot != sqlGot {
			t.Errorf("口径分叉 remarks=%q: Go=%v, SQL等价模拟=%v (请同步 domain/quality.go 与 search_repo.go hotBoardTCExpr)", r, goGot, sqlGot)
		}
	}
}

// TestHotBoardTCMultiBoardBonusScore 多榜回血: 每多一榜 +60, 封顶 120。
func TestHotBoardTCMultiBoardBonusScore(t *testing.T) {
	cases := []struct{ count, want int }{
		{0, 0}, {1, 0}, {2, 60}, {3, 120}, {4, 120}, {8, 120},
	}
	for _, c := range cases {
		if got := HotBoardTCMultiBoardBonusScore(c.count); got != c.want {
			t.Errorf("bonus(%d)=%d, want %d", c.count, got, c.want)
		}
	}
}
