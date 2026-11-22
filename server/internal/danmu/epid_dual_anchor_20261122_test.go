package danmu

import "testing"

// ===== D10 双锚互鉴（2026-11-22 用户设计定稿）黄金用例 =====
//
// 撤销 A2 锚点计数推 n（审计可对照面乱对率 51.8%、96.6% 不可验证）后的替代规则：
// 片内「双锚行」（解析后 Num>0 ∧ Date≠""，源站自己写了期号+日期的行）建两张复合键映射——
//   正查 (Series, Variant, Date, Part) → Num：给 d-only 行借 n（= 既有他源桥接 BridgeNum，同规）；
//   反查 (Series, Variant, Num, Part) → Date：给 n-only 行借 d（新增，D10a / poolIndex.dualAnchorDate）。
// 四守卫：①同复合键 ≥2 个不同值 ⇒ 该键跳过不借；②part 进复合键（partCompatible：空=通配、上/下严格、
// 数字序严格）——同日双期（同 d 两个 n=上下期）不互借错；③borrower 与映射行必须同 Series 同 Variant
// （类隔离，加更/特别篇不互借）；④片内无双锚行 ⇒ 两张映射为空 ⇒ 不借（宁缺勿错）。
// 借成功 from_src='bridge'（复用既有枚举，不新增取值）。
// 撤销后行为：原 anchor 行（raw 只有日期+part，如 `20260331上`）不再得到推算 n ⇒ 只落 d+p（n=-），
// 身份按日期。

// 1. 正查（= 桥接）：donor「第3期20260331上」(n=3,d=20260331,p=上) + borrower「20260331上」（d+p 同、无 n）
//    ⇒ 借 n=3、from_src=bridge；donor 与 borrower 同身份。
func TestDualAnchor_正查借n(t *testing.T) {
	out := EpidsForSourceRowWithPool([]string{"第3期20260331上", "20260331上"}, nil)
	if out[0][1] != EpKeySourceSelf || out[0][0] != "s=-|v=-|p=上|n=3|d=20260331" {
		t.Fatalf("双锚 donor 应 self: got %v", out[0])
	}
	if out[1][0] != "s=-|v=-|p=上|n=3|d=20260331" || out[1][1] != EpKeySourceBridge {
		t.Fatalf("正查应借 n=3 且记 bridge: got %v", out[1])
	}
}

// 2. 反查：borrower「第5期」(n=5 无 d) + 双锚行「第5期20260601上」⇒ 借 d=20260601、from_src=bridge。
//    borrower 的 part 为空 = 通配（partCompatible），donor 的 p=上 不挡借。
func TestDualAnchor_反查借d(t *testing.T) {
	out := EpidsForSourceRowWithPool([]string{"第5期", "第5期20260601上"}, nil)
	if out[0][0] != "s=-|v=-|p=-|n=5|d=20260601" || out[0][1] != EpKeySourceBridge {
		t.Fatalf("反查应借 d=20260601 且记 bridge: got %v", out[0])
	}
	if out[1][1] != EpKeySourceSelf || out[1][0] != "s=-|v=-|p=上|n=5|d=20260601" {
		t.Fatalf("双锚 donor 应 self: got %v", out[1])
	}
	// 线性版与池索引版同值（同规双实现）
	poolKeys := []EpisodeKey{ParseEpisodeKey("第5期20260601上")}
	cur := ParseEpisodeKey("第5期")
	if got := DualAnchorDate(cur, poolKeys); got != "20260601" {
		t.Fatalf("线性 DualAnchorDate 应 20260601, got %q", got)
	}
	if got := newPoolIndex(poolKeys).dualAnchorDate(cur); got != "20260601" {
		t.Fatalf("poolIndex.dualAnchorDate 应 20260601, got %q", got)
	}
}

// 3. 守卫①（正查）：同 d 的两个双锚行期号不一致（(20260331,上,n=3) 与 (20260331,上,n=4)）⇒ 该键跳过不借。
func TestDualAnchor_正查同键冲突跳过(t *testing.T) {
	out := EpidsForSourceRowWithPool([]string{"20260331上"},
		[]string{"20260331上", "第3期20260331上", "第4期20260331上"})
	if out[0][0] != "s=-|v=-|p=上|n=-|d=20260331" || out[0][1] != EpKeySourceNone {
		t.Fatalf("正查同键冲突应不借（n 留空）: got %v", out[0])
	}
}

// 3b. 守卫①（反查）：同 (s,v,n,p) 的两个双锚行日期不一致 ⇒ 不借、回落 self（n 不变）。
func TestDualAnchor_反查同键冲突跳过(t *testing.T) {
	out := EpidsForSourceRowWithPool([]string{"第5期"},
		[]string{"第5期", "第5期20260601上", "第5期20260608上"})
	if out[0][0] != "s=-|v=-|p=-|n=5|d=-" || out[0][1] != EpKeySourceSelf {
		t.Fatalf("反查同键冲突应不借、回落 self: got %v", out[0])
	}
}

// 4. 守卫②：part 严格 —— borrower p=下、映射只有 (d,上) ⇒ 不借（正查与反查同；
//    同日双期上下不互借错）。
func TestDualAnchor_part严格(t *testing.T) {
	// 正查
	out := EpidsForSourceRowWithPool([]string{"20260331下"},
		[]string{"20260331下", "第3期20260331上"})
	if out[0][0] != "s=-|v=-|p=下|n=-|d=20260331" || out[0][1] != EpKeySourceNone {
		t.Fatalf("part 不容应不借（正查）: got %v", out[0])
	}
	// 正查·同 d 双期各归各位：borrower p=下 只匹配 donor p=下 ⇒ 借到 n=4（part 进键的意义）
	out2 := EpidsForSourceRowWithPool([]string{"20260331下"},
		[]string{"20260331下", "第3期20260331上", "第4期20260331下"})
	if out2[0][0] != "s=-|v=-|p=下|n=4|d=20260331" || out2[0][1] != EpKeySourceBridge {
		t.Fatalf("同日双期应按 part 各借各位: got %v", out2[0])
	}
	// 反查
	out3 := EpidsForSourceRowWithPool([]string{"第5期下"},
		[]string{"第5期下", "第5期20260601上"})
	if out3[0][0] != "s=-|v=-|p=下|n=5|d=-" || out3[0][1] != EpKeySourceSelf {
		t.Fatalf("part 不容应不借（反查）: got %v", out3[0])
	}
}

// 5. 守卫③：类隔离 —— borrower v=加更、映射是 v=- 类 ⇒ 不互借（正查与反查同）。
func TestDualAnchor_类隔离(t *testing.T) {
	// 正查：加更行不借正片双锚行的 n
	out := EpidsForSourceRowWithPool([]string{"20260331加更"},
		[]string{"20260331加更", "第3期20260331"})
	if out[0][0] != "s=-|v=加更|p=-|n=-|d=20260331" || out[0][1] != EpKeySourceNone {
		t.Fatalf("变体不同应不借（正查）: got %v", out[0])
	}
	// 反查：加更期号行不借正片双锚行的 d
	out2 := EpidsForSourceRowWithPool([]string{"第5期加更"},
		[]string{"第5期加更", "第5期20260601"})
	if out2[0][0] != "s=-|v=加更|p=-|n=5|d=-" || out2[0][1] != EpKeySourceSelf {
		t.Fatalf("变体不同应不借（反查）: got %v", out2[0])
	}
	// 类隔离·Series：borrower s≠donor s ⇒ 不借（正查与反查同）
	out3 := EpidsForSourceRowWithPool([]string{"20260331上"},
		[]string{"20260331上", "20260331副本存档第3期上"})
	if out3[0][0] != "s=-|v=-|p=上|n=-|d=20260331" || out3[0][1] != EpKeySourceNone {
		t.Fatalf("Series 不同应不借: got %v", out3[0])
	}
}

// 6. 守卫④ + 撤销后行为：片内无双锚行 ⇒ 两张映射为空 ⇒ 不借；
//    原 anchor 行（raw 只有日期+part）只落 d+p（n=-）—— 身份按日期，宁缺勿错。
func TestDualAnchor_无双锚行不借_撤销后只落d(t *testing.T) {
	out := EpidsForSourceRowWithPool([]string{"20260331上", "20260407上", "20260414上"}, nil)
	for i, want := range []string{
		"s=-|v=-|p=上|n=-|d=20260331",
		"s=-|v=-|p=上|n=-|d=20260407",
		"s=-|v=-|p=上|n=-|d=20260414",
	} {
		if out[i][0] != want || out[i][1] != EpKeySourceNone {
			t.Fatalf("无双锚行应不推 n（原 anchor 行只落 d+p）: out[%d]=%v want %q", i, out[i], want)
		}
	}
}

// DerivedNumFor（线性参照链）：自带 n → 正查（桥接）→ 0；锚点兜底已退役。
func TestDualAnchor_DerivedNumFor链(t *testing.T) {
	if got := DerivedNumFor("第3期20260331上", []string{"第3期20260331上"}); got != 3 {
		t.Fatalf("自带期号应直取, got %d", got)
	}
	if got := DerivedNumFor("20260331上", []string{"20260331上", "第3期20260331上"}); got != 3 {
		t.Fatalf("正查应借 3, got %d", got)
	}
	// D10 撤销锚点：原 anchor 形态（池里只有「上」日期）不再推 n ⇒ 0
	if got := DerivedNumFor("20260331上", []string{"20260331上", "20260407上", "20260414上"}); got != 0 {
		t.Fatalf("锚点退役后应返回 0, got %d", got)
	}
}
