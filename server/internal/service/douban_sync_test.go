package service

import (
	"testing"
	"time"
)

// 每周一 03:00 的排期：今天还没到点算今天，到点或已过算下周一。
func TestNextMonday3am(t *testing.T) {
	loc := time.Local
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"周一凌晨未到点 → 当天 03:00", time.Date(2026, 11, 23, 2, 0, 0, 0, loc), time.Date(2026, 11, 23, 3, 0, 0, 0, loc)},
		{"周一已过点 → 下周一", time.Date(2026, 11, 23, 5, 0, 0, 0, loc), time.Date(2026, 11, 30, 3, 0, 0, 0, loc)},
		{"周日 → 次日周一", time.Date(2026, 11, 22, 23, 0, 0, 0, loc), time.Date(2026, 11, 23, 3, 0, 0, 0, loc)},
		{"周二 → 下周一", time.Date(2026, 11, 24, 10, 0, 0, 0, loc), time.Date(2026, 11, 30, 3, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		got := nextMonday3am(c.now)
		if !got.Equal(c.want) {
			t.Fatalf("%s: got %s want %s", c.name, got.Format("2006-01-02 15:04"), c.want.Format("2006-01-02 15:04"))
		}
		if got.Weekday() != time.Monday || got.Hour() != 3 {
			t.Fatalf("%s: 结果不是周一 03:00：%s", c.name, got)
		}
	}
}
