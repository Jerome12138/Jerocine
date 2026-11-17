package mysql

import "testing"

// 分层漏斗的判定单测（总则 §6 防误伤清单的行为化）。
// funnelPiece / anchorAligned 都是纯函数（不碰 DB）⇒ 可直跑。

func mkSlot(rows map[int]string) map[int]string { return rows }

// 便捷构造：A 源、B 源
func piece(a, b map[int]string) map[int64]map[int]string {
	return map[int64]map[int]string{1: a, 2: b}
}

func TestFunnelPiece_L0_L1(t *testing.T) {
	// 单源槽 ⇒ L0（覆盖差异默认对齐）
	st, ok := funnelPiece(map[int64]map[int]string{1: {1: "s=-|v=-|p=-|n=1|d=-"}})
	if !ok || st.L0Single != 1 || st.SlotsTotal != 1 || st.Leftover != 0 || st.MksAligned != 1 {
		t.Fatalf("L0 单源槽判定错: %+v", st)
	}
	// 两源同 epid ⇒ L1
	st2, _ := funnelPiece(piece(
		map[int]string{1: "s=-|v=-|p=-|n=1|d=-"},
		map[int]string{1: "s=-|v=-|p=-|n=1|d=-"}))
	if st2.L1Slot != 1 || st2.Leftover != 0 {
		t.Fatalf("L1 槽位全等判定错: %+v", st2)
	}
}

// L2：值序列仅差插入/删除（缺集造成的整体错位）⇒ 救回
func TestFunnelPiece_L2_Tolerant(t *testing.T) {
	a := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=3|d=-"}
	b := map[int]string{1: "s=-|v=-|p=-|n=2|d=-", 2: "s=-|v=-|p=-|n=3|d=-"}
	st, _ := funnelPiece(piece(a, b))
	if st.L2Tolerant == 0 {
		t.Fatalf("整体错位应被 L2 救回: %+v", st)
	}
	if st.Leftover != 0 {
		t.Fatalf("L2 救回后不应有 leftover: %+v", st)
	}
}

// L2 不放过"替换"（A 缺 x / B 缺 y，x≠y）
func TestFunnelPiece_L2_NoReplace(t *testing.T) {
	a := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=3|d=-"}
	b := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=4|d=-"}
	st, _ := funnelPiece(piece(a, b))
	if st.Leftover == 0 && st.L2Tolerant > 0 {
		t.Fatalf("同位置替换不得被 L2 当作对齐: %+v", st)
	}
}

// ⚠️ 实测发现（2026-11-17）：**L2（M4）远比预想宽松** —— `insertDeleteDiffs` 只在
// "两侧当前值互不存在于对方剩余"时才判冲突，因此**换位、缺集、交错缺集都能被它放行**。
// ⇒ L3/L4 的独有空间可能接近 0（要靠实测确认），下面两个用例改为**直测锚抽取函数**，
// 不再假设"L2 拦不住、只有锚层能救"。

func TestAnchorKeySets_DateAndNum(t *testing.T) {
	bySrc := map[int64]map[int]string{
		1: {
			1: "s=-|v=-|p=中|n=-|d=20240101", // 日期锚（带 part）
			2: "s=-|v=-|p=-|n=5|d=-",        // 期号锚
			3: "s=栏目|v=-|p=-|n=6|d=-",       // 含 s ⇒ 不干净 ⇒ 两层都不进
			4: "s=-|v=-|p=-|n=7|d=20240108", // 两种锚并存 ⇒ 都不进
		},
	}
	dk := anchorKeySets(bySrc, anchorDate)
	if len(dk) != 1 || len(dk[0]) != 1 || dk[0][0] != "20240101#中" {
		t.Fatalf("日期锚抽取错（应只含干净且仅有日期的行）: %v", dk)
	}
	nk := anchorKeySets(bySrc, anchorNum)
	if len(nk) != 1 || len(nk[0]) != 1 || nk[0][0] != "5#" {
		t.Fatalf("期号锚抽取错（应只含干净且仅有期号的行）: %v", nk)
	}
}

// 纯日期片（两侧全是干净日期行、集合相同、顺序相反）在漏斗里的归属：
// 只断言"不变量成立 + 该片被判齐"，**不断言救回发生在哪一层**（L2 很宽松，实测为准）。
func TestFunnelPiece_PureDatePiece_AlignedSomeLayer(t *testing.T) {
	a := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240101", 2: "s=-|v=-|p=-|n=-|d=20240108"}
	b := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240108", 2: "s=-|v=-|p=-|n=-|d=20240101"}
	st, _ := funnelPiece(piece(a, b))
	sum := st.L0Single + st.L1Slot + st.L2Tolerant + st.L3Date + st.L4Num + st.L5Mixed + st.Leftover
	if sum != st.SlotsTotal {
		t.Fatalf("不变量被破坏: %+v", st)
	}
	if st.Leftover != 0 || st.MksAligned != 1 {
		t.Fatalf("顺序相反的纯日期片应被判齐（层级不限）: %+v", st)
	}
}

// L3 守卫：源内锚不唯一 ⇒ 本层放弃（宁缺勿错）
func TestFunnelPiece_L3_DupAnchorRejected(t *testing.T) {
	a := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240101", 2: "s=-|v=-|p=-|n=-|d=20240101"}
	b := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240101"}
	st, _ := funnelPiece(piece(a, b))
	if st.L3Date > 0 {
		t.Fatalf("同源同日两行（无 part 区分）不得被日期锚判齐: %+v", st)
	}
}

// L4 同 L3：只断言"层级不限地被判齐"，不假设救回发生在期号锚层（L2 很宽松）。
func TestFunnelPiece_NumPiece_AlignedSomeLayer(t *testing.T) {
	a := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"}
	b := map[int]string{1: "s=-|v=-|p=-|n=2|d=-", 2: "s=-|v=-|p=-|n=1|d=-"}
	st, _ := funnelPiece(piece(a, b))
	if st.Leftover != 0 {
		t.Fatalf("顺序相反的纯期号片应被判齐（层级不限）: %+v", st)
	}
}

// 值互不相干（真冲突）⇒ 任何层都救不回来
func TestFunnelPiece_Leftover_RealConflict(t *testing.T) {
	a := map[int]string{1: "s=-|v=-|p=-|n=1|d=-"}
	b := map[int]string{1: "s=-|v=-|p=-|n=9|d=-"}
	st, _ := funnelPiece(piece(a, b))
	if st.Leftover == 0 {
		t.Fatalf("n=1 vs n=9 应落 leftover: %+v", st)
	}
	if st.MksAligned != 0 {
		t.Fatalf("有 leftover 的片不应算对齐: %+v", st)
	}
}

// 锚只给"整槽干净"的槽背书：含 s/v 的槽不得被日期/期号锚救回（防误伤，2026-11-17 审查）
func TestFunnelPiece_AnchorDoesNotBackMixedSlots(t *testing.T) {
	// 槽1 干净且两源一致（L1）；槽2 一源带 s ⇒ 既非 L1 也非干净 ⇒ 不得被锚背书 ⇒ leftover
	a := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240101", 2: "s=栏目A|v=-|p=-|n=-|d=20240108"}
	b := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240101", 2: "s=-|v=-|p=-|n=-|d=20240115"}
	st, _ := funnelPiece(piece(a, b))
	if st.L3Date > 0 {
		t.Fatalf("含 s 的槽不得被日期锚救回: %+v", st)
	}
	if st.L5Mixed+st.Leftover == 0 {
		t.Fatalf("该槽应落 L5 或 leftover: %+v", st)
	}
	// 对照：整槽干净 + 子集相容 + 覆盖差超过 M4 预算（6 vs 3，gap=3 > tol=max(2,5%×6)=2）
	// ⇒ M4 拦不住 ⇒ 只有日期锚能救回（这正是 L3/L4 的真实独有面）
	c1 := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240101", 2: "s=-|v=-|p=-|n=-|d=20240102",
		3: "s=-|v=-|p=-|n=-|d=20240103", 4: "s=-|v=-|p=-|n=-|d=20240104",
		5: "s=-|v=-|p=-|n=-|d=20240105", 6: "s=-|v=-|p=-|n=-|d=20240106",
	}
	c2 := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240104", 2: "s=-|v=-|p=-|n=-|d=20240105",
		3: "s=-|v=-|p=-|n=-|d=20240106",
	}
	st2, _ := funnelPiece(piece(c1, c2))
	if st2.L3Date == 0 {
		t.Fatalf("覆盖差超预算的错位纯日期片应被日期锚救回: %+v", st2)
	}
	if st2.Leftover != 0 {
		t.Fatalf("日期锚救回后不应有 leftover: %+v", st2)
	}
	if st2.SlotsTotal != st2.L0Single+st2.L1Slot+st2.L2Tolerant+st2.L3Date+st2.L4Num+st2.L5Mixed+st2.Leftover {
		t.Fatalf("不变量被破坏: %+v", st2)
	}
}

// 多身份行 ⇒ L5 用「去掉 s 的联合键」判定：s 不同但 v/n/d 相同 ⇒ 判齐（栏目名差异不算错）
func TestFunnelPiece_L5_MixedIdentity(t *testing.T) {
	a := map[int]string{1: "s=栏目A|v=-|p=-|n=1|d=-"}
	b := map[int]string{1: "s=栏目B|v=-|p=-|n=1|d=-"}
	st, _ := funnelPiece(piece(a, b))
	if st.L5Mixed != 1 {
		t.Fatalf("s 不同但 n 相同的多身份槽应被 L5 联合键救回: %+v", st)
	}
	if st.Leftover != 0 {
		t.Fatalf("L5 救回后不应有 leftover: %+v", st)
	}
	// 反向：联合键不同的多身份槽 ⇒ 仍落 leftover（不得被 s 忽略而误判）
	c := map[int]string{1: "s=栏目A|v=-|p=-|n=1|d=-"}
	d := map[int]string{1: "s=栏目A|v=-|p=-|n=2|d=-"}
	st2, _ := funnelPiece(piece(c, d))
	if st2.L5Mixed != 0 || st2.Leftover == 0 {
		t.Fatalf("联合键不同的多身份槽应落 leftover: %+v", st2)
	}
	// 单锚层不得给多身份槽背书（含 s 的槽不进 L3/L4）
	e := map[int]string{1: "s=栏目A|v=-|p=-|n=-|d=20240101"}
	f := map[int]string{1: "s=栏目B|v=-|p=-|n=-|d=20240101"}
	st3, _ := funnelPiece(piece(e, f))
	if st3.L3Date != 0 {
		t.Fatalf("含 s 的槽不得被日期锚背书（应走 L5）: %+v", st3)
	}
	if st3.L5Mixed != 1 {
		t.Fatalf("含 s 的纯日期槽应由 L5 联合键判定：%+v", st3)
	}
}

// 不变量：分母 = 各层之和（总则 §2）
func TestFunnelInvariant(t *testing.T) {
	st, _ := funnelPiece(piece(
		map[int]string{
			1: "s=-|v=-|p=-|n=1|d=-",
			2: "s=-|v=-|p=-|n=2|d=-",
			3: "s=栏目|v=-|p=-|n=3|d=-",
		},
		map[int]string{
			1: "s=-|v=-|p=-|n=1|d=-",
			3: "s=-|v=-|p=-|n=3|d=-",
		}))
	sum := st.L0Single + st.L1Slot + st.L2Tolerant + st.L3Date + st.L4Num + st.L5Mixed + st.Leftover
	if sum != st.SlotsTotal {
		t.Fatalf("不变量被破坏: 各层之和=%d 分母=%d %+v", sum, st.SlotsTotal, st)
	}
}

func TestAnchorAligned_Guards(t *testing.T) {
	// 单源 ⇒ 不可用
	if usable, _ := anchorAligned([][]string{{"a", "b"}}); usable {
		t.Fatal("单源锚集合不可用")
	}
	// 源内重复 ⇒ 不可用
	if usable, _ := anchorAligned([][]string{{"a", "a"}, {"a"}}); usable {
		t.Fatal("源内锚重复应放弃本层")
	}
	// 子集 ⇒ 相容
	if usable, aligned := anchorAligned([][]string{{"a", "b"}, {"a"}}); !usable || !aligned {
		t.Fatal("子集应判相容")
	}
	// 交错 ⇒ 不相容
	if _, aligned := anchorAligned([][]string{{"a", "b"}, {"b", "c"}}); aligned {
		t.Fatal("交错缺集不得判相容")
	}
}
