package taskcenter

import "testing"

// 线上两条 + 常见形态必须能转；复杂语法不猜（返回 false 交给人工）。
func TestCronSpecToSchedule(t *testing.T) {
	ok := map[string]string{
		"0 */20 * * * ?": "every:20m",      // 线上 id=1（带秒 + Quartz ?）
		"0 0 4 * * 0":    "weekly:7 04:00", // 线上 id=2（周日）
		"0 30 3 * * *":   "daily:03:30",
		"*/30 * * * *":   "every:30m", // 5 段
		"0 0 4 1 * *":    "monthly:1 04:00",
		"0 0 4 * * 7":    "weekly:7 04:00", // 7 也是周日
		"0 0 4 * * 1":    "weekly:1 04:00",
	}
	for in, want := range ok {
		got, moved := CronSpecToSchedule(in)
		if !moved || got != want {
			t.Fatalf("CronSpecToSchedule(%q) = (%q,%v), want (%q,true)", in, got, moved, want)
		}
		// 转换结果必须能被调度器解析
		if err := ValidateSchedule(got); err != nil {
			t.Fatalf("转换结果不可解析: %q: %v", got, err)
		}
	}

	bad := []string{
		"0 0 4 * * MON",  // 星期用名字
		"0 0-5 4 * * *",  // 区间
		"0 0 4 1,15 * *", // 列表
		"0 0 4 1 1 *",    // 月+日都限定（不支持）
		"30 0 4 * * *",   // 秒不为 0
		"0 0 * * * *",    // 小时通配
		"garbage",
		"",
	}
	for _, in := range bad {
		if got, moved := CronSpecToSchedule(in); moved {
			t.Fatalf("%q 不应被转换, got %q", in, got)
		}
	}
}
