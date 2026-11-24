package mysql

import (
	"testing"
	"time"
)

// 钉住写入闸：非毫秒时间戳必须被纠正为"当前毫秒"，合法值必须原样保留。
//
// 背景（2026-11-23）：`movie_play_source.updated_at` 曾有 322 行被写成
// YYYYMMDDHHMMSS 整数（如 20261118194207 ≈ 2.0e13），导致按 updated_at 的时间窗
// 审计/增量全部失效。写入侧统一过 guardMilli 防复发。
func TestGuardMilli(t *testing.T) {
	now := time.Now().UnixMilli()

	cases := []struct {
		name string
		in   int64
		// wantKeep=true 表示应原样返回；false 表示应被纠正为"接近当前毫秒"
		wantKeep bool
	}{
		{"正常毫秒(当前)", now, true},
		{"正常毫秒(2026-11-18)", 1763468527000, true},
		{"YYYYMMDDHHMMSS 整数", 20261118194207, false},
		{"秒级时间戳", 1763468527, false},
		{"零值", 0, false},
		{"负数", -1, false},
		{"超出上界", 5000000000000, false},
	}

	for _, c := range cases {
		got := guardMilli(c.in)
		if c.wantKeep {
			if got != c.in {
				t.Errorf("%s: 合法值被改动 in=%d got=%d", c.name, c.in, got)
			}
			continue
		}
		if got == c.in {
			t.Errorf("%s: 非法值未被纠正 in=%d", c.name, c.in)
		}
		// 纠正后必须落在合法区间且接近"现在"
		if got < tsMinValidMilli || got > tsMaxValidMilli {
			t.Errorf("%s: 纠正后仍非法 got=%d", c.name, got)
		}
		if d := got - time.Now().UnixMilli(); d > 5000 || d < -5000 {
			t.Errorf("%s: 纠正值应接近当前毫秒 got=%d", c.name, got)
		}
	}
}
