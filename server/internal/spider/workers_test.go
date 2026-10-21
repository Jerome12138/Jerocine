package spider

import "testing"

// TestEffectiveWorkers 并发策略: 限速/小增量单线程, 其余取 min(总上限, 主站/从站硬上限, 页数)。
// 主站 12 / 从站 8 原本是代码常量, 2026-10-21 搬进后台后必须以 Limits 为准 ——
// 挂掉说明硬上限又被写死回去了。
func TestEffectiveWorkers(t *testing.T) {
	def := DefaultLimits()
	cases := []struct {
		name      string
		lim       Limits
		pageCount int
		master    bool
		full      bool
		throttled bool
		want      int
	}{
		{"源站要求限速 → 单线程", def, 1000, true, true, true, 1},
		{"增量小窗口 → 单线程", def, concurrentPageMin, true, false, false, 1},
		{"全量主站 → 主站硬上限", def, 1000, true, true, false, 12},
		{"全量从站 → 从站硬上限", def, 1000, false, true, false, 8},
		{"总上限更小 → 用总上限", Limits{Max: 5, Master: 12, Slave: 8}, 1000, true, true, false, 5},
		{"页数更小 → 用页数", def, 3, true, true, false, 3},
		{"后台调大主站上限后按新值", Limits{Max: 32, Master: 20, Slave: 4}, 1000, true, true, false, 20},
		{"后台调小从站上限后按新值", Limits{Max: 32, Master: 20, Slave: 4}, 1000, false, true, false, 4},
		{"增量但页数超阈值 → 允许并发", def, concurrentPageMin + 1, false, false, false, 8},
	}
	for _, c := range cases {
		if got := effectiveWorkers(c.lim, c.pageCount, c.master, c.full, c.throttled); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

// TestLimitsClamp 后台写入 0/负值(或旧库没这两列)时不能变成 0 并发或丢掉硬上限。
func TestLimitsClamp(t *testing.T) {
	def := DefaultLimits()
	if def != (Limits{Max: 32, Master: 12, Slave: 8}) {
		t.Fatalf("默认值变了: %+v", def)
	}
	if got := (Limits{}).clamp(); got != def {
		t.Fatalf("全零应回默认: %+v", got)
	}
	if got := (Limits{Max: -1, Master: 0, Slave: 5}).clamp(); got != (Limits{Max: 32, Master: 12, Slave: 5}) {
		t.Fatalf("只补非法项: %+v", got)
	}
}

// TestEngineLimitsFallback 未接线提供者时回落构造期 maxG + 默认硬上限。
func TestEngineLimitsFallback(t *testing.T) {
	e := &Engine{maxG: 16}
	if got := e.limits(); got != (Limits{Max: 16, Master: 12, Slave: 8}) {
		t.Fatalf("未接线: %+v", got)
	}
	e.SetLimitsProvider(func() Limits { return Limits{Max: 40, Master: 10, Slave: 6} })
	if got := e.limits(); got != (Limits{Max: 40, Master: 10, Slave: 6}) {
		t.Fatalf("提供者生效: %+v", got)
	}
	e.SetLimitsProvider(func() Limits { return Limits{} })
	if got := e.limits(); got != DefaultLimits() {
		t.Fatalf("提供者返回非法值应回默认: %+v", got)
	}
}
