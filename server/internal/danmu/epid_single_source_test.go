package danmu

import "testing"

// TestParseEpKeySource_Single 防呆（2026-11-14）：`EpKeySourceSingle` 必须登记进 `ParseEpKeySource` 的
// **已知值白名单** —— 这正是老规划 `方案-epid源头校正层-2026-11-12.md` §A.1 的「改动点 2」。
//
// 漏登记的后果：端侧一旦带上 `from=single`，会被**静默当"未提供"** ⇒ 服务端退回 `epNum > 0` 旧规则
// （`danmu_service.go:253-254`）⇒ 三端漂移哨兵（冲突打点）少一档信息，且排查时完全看不出被丢掉了 ✗。
func TestParseEpKeySource_Single(t *testing.T) {
	if got := ParseEpKeySource("v1|d=20260803|from=single"); got != EpKeySourceSingle {
		t.Errorf("ParseEpKeySource(from=single) = %q, want %q", got, EpKeySourceSingle)
	}
	for _, v := range []string{
		EpKeySourceSelf, EpKeySourceBridge, EpKeySourceAnchor, EpKeySourceNone, EpKeySourceSingle,
	} {
		if got := ParseEpKeySource("v1|from=" + v); got != v {
			t.Errorf("ParseEpKeySource(from=%s) = %q, want %q", v, got, v)
		}
	}
	if got := ParseEpKeySource("v1|from=bogus"); got != "" {
		t.Errorf("未知值应返回空串（避免把客户端异常值当来源统计），得 %q", got)
	}
}
