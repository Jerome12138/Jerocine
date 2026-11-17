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
	sum := st.L0Single + st.L1Slot + st.L2Tolerant + st.L3Date + st.L4Num + st.L5Subset + st.MixedDiag + st.Leftover
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
	if st.MixedDiag+st.Leftover == 0 {
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
	if st2.SlotsTotal != st2.L0Single+st2.L1Slot+st2.L2Tolerant+st2.L3Date+st2.L4Num+st2.L5Subset+st2.MixedDiag+st2.Leftover {
		t.Fatalf("不变量被破坏: %+v", st2)
	}
}

// 诊断档是**不判定**：多身份槽（含 s/v）只有 epid 全等才算齐（那在 L1 判掉），
// 否则单列进 MixedDiag 并**计入未对齐**（用户 2026-11-17 定：判定必须等价于 epid 相等，
// 不能靠"去掉 s"把不同栏目判成同集）。
func TestFunnelPiece_MixedDiag_NotJudgment(t *testing.T) {
	// s 不同 ⇒ 未对齐（不得被宽松键救回）
	a := map[int]string{1: "s=栏目A|v=-|p=-|n=1|d=-"}
	b := map[int]string{1: "s=栏目B|v=-|p=-|n=1|d=-"}
	st, _ := funnelPiece(piece(a, b))
	if st.MixedDiag != 1 {
		t.Fatalf("含 s 的未对齐槽应落诊断档 MixedDiag: %+v", st)
	}
	if st.MksAligned != 0 {
		t.Fatalf("诊断档属未对齐 ⇒ 该片不得算对齐: %+v", st)
	}
	// s 相同（epid 全等）⇒ 由 L1 判齐，不进诊断档
	c := map[int]string{1: "s=栏目A|v=-|p=-|n=1|d=-"}
	d := map[int]string{1: "s=栏目A|v=-|p=-|n=1|d=-"}
	st2, _ := funnelPiece(piece(c, d))
	if st2.L1Slot != 1 || st2.MixedDiag != 0 || st2.MksAligned != 1 {
		t.Fatalf("epid 全等的多身份槽应由 L1 判齐: %+v", st2)
	}
	// 单锚层不得给含 s 的槽背书（整槽准入）
	e := map[int]string{1: "s=栏目A|v=-|p=-|n=-|d=20240101"}
	f := map[int]string{1: "s=栏目B|v=-|p=-|n=-|d=20240101"}
	st3, _ := funnelPiece(piece(e, f))
	if st3.L3Date != 0 || st3.MixedDiag != 1 {
		t.Fatalf("含 s 的日期槽不得走 L3，应落诊断档: %+v", st3)
	}
}

// L5 正片子集（用户 2026-11-17 定）：全片无 s/v ∧ 每源 epid 集合 ⊆ 最大源集合 ⇒ 救回。
// ⚠️ 用例设计教训：**纯期号/纯日期的子集片会被 L4/L3 先救走**（这正是"L5 增量小"的原因），
// 所以这里刻意构造**跨表示**（槽内一行仅 n、一行仅 d）且规模超过 M4 预算 ⇒ 只有 L5 能救。
func TestFunnelPiece_L5_NormSubset(t *testing.T) {
	a := map[int]string{
		1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=3|d=-",
		4: "s=-|v=-|p=-|n=-|d=20240101", 5: "s=-|v=-|p=-|n=-|d=20240102", 6: "s=-|v=-|p=-|n=-|d=20240103",
	}
	b := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240101", 2: "s=-|v=-|p=-|n=-|d=20240102", 3: "s=-|v=-|p=-|n=-|d=20240103",
		4: "s=-|v=-|p=-|n=1|d=-", 5: "s=-|v=-|p=-|n=2|d=-", 6: "s=-|v=-|p=-|n=3|d=-",
	}
	st, _ := funnelPiece(piece(a, b))
	if st.L5Subset == 0 {
		t.Fatalf("正片值子集（跨表示、集合相同）应被 L5 救回: %+v", st)
	}
	if st.Leftover != 0 || st.MksAligned != 1 {
		t.Fatalf("L5 救回后应整片对齐: %+v", st)
	}
	// 反向：不是子集（两侧各有独有值）⇒ L5 不得救
	c := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"}
	d := map[int]string{1: "s=-|v=-|p=-|n=2|d=-", 2: "s=-|v=-|p=-|n=3|d=-"}
	st2, _ := funnelPiece(piece(c, d))
	if st2.L5Subset != 0 {
		t.Fatalf("非子集（各有独有集）不得被 L5 救回: %+v", st2)
	}
	// 含 s/v ⇒ 不进 L5（正片才判）
	e := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"}
	f := map[int]string{1: "s=栏目|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"}
	st3, _ := funnelPiece(piece(e, f))
	if st3.L5Subset != 0 {
		t.Fatalf("含 s 的片不得走 L5: %+v", st3)
	}
}

// 不变量 + 分层口径对齐率（2026-11-17 用户定：L2~L5 也纳入对齐率）
func TestFunnelRates(t *testing.T) {
	st := FunnelStat{
		SlotsTotal: 10,
		L0Single:   4,
		L1Slot:     2,
		L2Tolerant: 1,
		L3Date:     1,
		L4Num:      0,
		L5Subset:   1,
		MixedDiag:  1,
		Leftover:   0,
		MksTotal:   3,
		MksAligned: 2,
	}
	if st.L0Single+st.L1Slot+st.L2Tolerant+st.L3Date+st.L4Num+st.L5Subset+st.MixedDiag+st.Leftover != st.SlotsTotal {
		t.Fatalf("不变量被破坏: %+v", st)
	}
	finalizeFunnel(&st)
	// 分子 = 10 - 1(诊断档) - 0 = 9
	if st.AlignedSlots != 9 || st.SlotPct != 90 {
		t.Fatalf("集级对齐率应为 9/10=90%%: %+v", st)
	}
	if st.MkPct != 200.0/3.0 {
		t.Fatalf("片级对齐率应为 2/3: %+v", st)
	}
	// 边界：空统计不得除零
	var empty FunnelStat
	finalizeFunnel(&empty)
	if empty.SlotPct != 0 || empty.MkPct != 0 {
		t.Fatalf("空统计应保持 0: %+v", empty)
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
	sum := st.L0Single + st.L1Slot + st.L2Tolerant + st.L3Date + st.L4Num + st.L5Subset + st.MixedDiag + st.Leftover
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
