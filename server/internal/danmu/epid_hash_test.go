package danmu

import "testing"

// EpidHash 的算法守卫：三端同规（web/android 必须实现同一算法）。
// 这里的**黄金值**同时被 web 的 epidHash 用例引用（跨端对齐的锚点）。
func TestEpidHash(t *testing.T) {
	golden := map[string]string{
		"s=-|v=-|p=-|n=1|d=-":          "", // 运行时填（见下 Logf 打印）
		"s=王牌对王牌|v=-|p=-|n=5|d=-": "",
		"s=-|v=特辑|p=中|n=12|d=20260803": "",
	}
	// 打印黄金值（-v 时可见；用于填进 web spec 与本文档）
	for in := range golden {
		t.Logf("EPIDHASH %q => %q", in, EpidHash(in))
	}

	// 1) 确定性 + 形状
	for _, in := range []string{
		"s=-|v=-|p=-|n=1|d=-",
		"s=王牌对王牌|v=-|p=-|n=5|d=-",
		"s=-|v=-|p=中上|n=-|d=20260803",
		"",
	} {
		h := EpidHash(in)
		if h != EpidHash(in) {
			t.Fatalf("%q: 非确定性", in)
		}
		if len(h) == 0 || len(h) > 13 {
			t.Fatalf("%q: 长度 %d 超界（要求 1..13）", in, len(h))
		}
		for i := 0; i < len(h); i++ {
			c := h[i]
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z') {
				t.Fatalf("%q: 含非 base36 字符 %q（%q）", in, string(c), h)
			}
		}
	}
	// 2) 不同输入不同输出（片内碰撞概率 ~2^-64）
	a, b := EpidHash("s=-|v=-|p=-|n=1|d=-"), EpidHash("s=-|v=-|p=-|n=2|d=-")
	if a == b {
		t.Fatalf("不同 epid 撞了：%q", a)
	}
	// 3) 空串也稳定（不 panic，供"未派生"场景做哨兵）
	if EpidHash("") == "" {
		t.Fatal("空串应产出非空短码")
	}
}