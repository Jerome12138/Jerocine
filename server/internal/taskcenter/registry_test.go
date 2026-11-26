package taskcenter

import (
	"testing"
	"time"
)

// 友好文本周期：只接受文档里定义的四类写法。
func TestValidateSchedule(t *testing.T) {
	ok := []string{"", "every:30m", "every:6h", "daily:03:00", "weekly:1 03:00", "weekly:7 23:59", "monthly:1 04:00", "monthly:28 00:00"}
	for _, s := range ok {
		if err := ValidateSchedule(s); err != nil {
			t.Fatalf("%q 应合法: %v", s, err)
		}
	}
	bad := []string{
		"0 3 * * *",        // 裸 cron 不接受
		"every:0m",         // 间隔必须 ≥1
		"every:90m",        // 分钟 >59 要用小时
		"daily:24:00",      // 小时越界
		"daily:03:60",      // 分钟越界
		"weekly:8 03:00",   // 星期越界（正则已挡）
		"monthly:29 04:00", // 月末可能不存在
		"every:6",          // 缺单位
		"daily:3:00",       // 小时必须是两位
	}
	for _, s := range bad {
		if err := ValidateSchedule(s); err == nil {
			t.Fatalf("%q 应被判非法", s)
		}
	}
}

func TestDescribeSchedule(t *testing.T) {
	cases := map[string]string{
		"":                "仅手动",
		"every:30m":       "每 30 分钟",
		"every:6h":        "每 6 小时",
		"daily:03:00":     "每天 03:00",
		"weekly:1 03:00":  "每周一 03:00",
		"weekly:7 23:00":  "每周日 23:00",
		"monthly:1 04:00": "每月 1 日 04:00",
	}
	for in, want := range cases {
		if got := DescribeSchedule(in); got != want {
			t.Fatalf("DescribeSchedule(%q) = %q, want %q", in, got, want)
		}
	}
}

// NextRun 覆盖跨天/跨周/跨月，且必须严格晚于 now。
func TestNextRun(t *testing.T) {
	base := time.Date(2026, 11, 25, 2, 0, 0, 0, time.Local) // 周三 02:00

	if got := NextRun("daily:03:00", base); !got.Equal(time.Date(2026, 11, 25, 3, 0, 0, 0, time.Local)) {
		t.Fatalf("daily 当天未到点应算当天, got %s", got)
	}
	if got := NextRun("daily:01:00", base); !got.Equal(time.Date(2026, 11, 26, 1, 0, 0, 0, time.Local)) {
		t.Fatalf("daily 已过点应顺延次日, got %s", got)
	}
	// 周三 → 下周一 03:00
	if got := NextRun("weekly:1 03:00", base); !got.Equal(time.Date(2026, 11, 30, 3, 0, 0, 0, time.Local)) {
		t.Fatalf("weekly 应算到下周一, got %s", got)
	}
	if got := NextRun("monthly:1 04:00", base); !got.Equal(time.Date(2026, 12, 1, 4, 0, 0, 0, time.Local)) {
		t.Fatalf("monthly 应算到下月 1 日, got %s", got)
	}
	if got := NextRun("every:6h", base); !got.Equal(base.Add(6 * time.Hour)) {
		t.Fatalf("every 应从 now 起算, got %s", got)
	}
	if got := NextRun("", base); !got.IsZero() {
		t.Fatal("空周期应返回零值（仅手动）")
	}
}

// 注册表：按注册顺序返回，同 key 覆盖。
func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(Def{Key: "builtin:a", Name: "A"})
	r.Register(Def{Key: "builtin:b", Name: "B"})
	r.Register(Def{Key: "builtin:a", Name: "A2"})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("应去重为 2 项, got %d", len(all))
	}
	if all[0].Key != "builtin:a" || all[0].Name != "A2" {
		t.Fatalf("同 key 应覆盖且保持原顺序: %+v", all[0])
	}
	if d, ok := r.Get("builtin:b"); !ok || d.Name != "B" {
		t.Fatal("Get 失败")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("不存在的 key 不应命中")
	}
}
