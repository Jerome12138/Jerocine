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
	sum := st.L0Single + st.L1Slot + st.L2Tolerant + st.L3Date + st.L4Num + st.L5Subset + st.L6Tol + st.MixedDiag + st.Leftover
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
	// 注意：单纯 n=1 vs n=9 属"纯 n 片"，按 L6 会被容忍为缺集；这里用「n+d 混合行」构造真冲突
	a := map[int]string{1: "s=-|v=-|p=-|n=1|d=20240101"}
	b := map[int]string{1: "s=-|v=-|p=-|n=9|d=20240102"}
	st, _ := funnelPiece(piece(a, b))
	if st.Leftover == 0 {
		t.Fatalf("混合 n+d 且值不相干应落 leftover: %+v", st)
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
	if st2.SlotsTotal != st2.L0Single+st2.L1Slot+st2.L2Tolerant+st2.L3Date+st2.L4Num+st2.L5Subset+st2.L6Tol+st2.MixedDiag+st2.Leftover {
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

// L4 放宽（2026-11-17）：键用 **n 值**（不含 part），part 改作守卫。
//   - 一侧有 part、一侧空 ⇒ 属"表示差异" ⇒ 仍可救；
//   - 同一 n 上两侧 part 都有值且不同 ⇒ 是不同集 ⇒ **不救**。
func TestFunnelPiece_L4_PartGuard(t *testing.T) {
	// 情形 A：part 一侧空（每源各 2 集，键 {1,2} 相同）⇒ 应救
	a := map[int]string{
		1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-",
	}
	b := map[int]string{
		1: "s=-|v=-|p=上|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-",
	}
	st, _ := funnelPiece(piece(a, b))
	if st.Leftover != 0 || st.MksAligned != 1 {
		t.Fatalf("一侧 part 空的错位片应被判齐（层级不限，L2 常先吃掉）: %+v", st)
	}
	// 情形 B：同一 n 上两侧 part 不同（上 vs 中）⇒ 守卫拒绝，不得救
	c := map[int]string{1: "s=-|v=-|p=上|n=1|d=-"}
	d := map[int]string{1: "s=-|v=-|p=中|n=1|d=-"}
	st2, _ := funnelPiece(piece(c, d))
	if st2.L4Num != 0 {
		t.Fatalf("同 n 但 part 不同（上/中）不得被 L4 救: %+v", st2)
	}
	if st2.Leftover == 0 {
		t.Fatalf("该槽应落 leftover: %+v", st2)
	}
}

// L4 的边界（与用户 2026-11-17 澄清对齐）：
//
//	✅ 兼容"一侧有 part、一侧无 part"（part 不进键）；
//	❌ 不兼容"某 n 在另一源完全没有"——子集要求较小源 ⊆ 最大源，互有独有值 ⇒ 不救；
//	❌ 含 s/v 的槽不进 L4（整槽准入）。
func TestFunnelPiece_L4_Boundaries(t *testing.T) {
	// 互有独有值：A={1,2,3}、B={1,2,9}（9 只在 B、3 只在 A）⇒ 不救
	a := map[int]string{
		1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=3|d=-",
	}
	b := map[int]string{
		1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=9|d=-",
	}
	st, _ := funnelPiece(piece(a, b))
	if st.L4Num != 0 {
		t.Fatalf("某 n 在另一源无对应（互有独有值）不得被 L4 救: %+v", st)
	}
	if st.L6Tol == 0 {
		t.Fatalf("该片是纯 n 片 ⇒ 应由 L6 容忍为缺集: %+v", st)
	}
	// 含 s 的槽不进 L4（走诊断档）
	c := map[int]string{1: "s=栏目|v=-|p=-|n=1|d=-"}
	d := map[int]string{1: "s=-|v=-|p=-|n=2|d=-"}
	st2, _ := funnelPiece(piece(c, d))
	if st2.L4Num != 0 {
		t.Fatalf("含 s 的槽不得进 L4: %+v", st2)
	}
	if st2.MixedDiag == 0 {
		t.Fatalf("含 s 的未对齐槽应落诊断档: %+v", st2)
	}
}

// L3 与 L4 同规（2026-11-17）：键 = `d` 值（不含 part）；part 降为守卫。
//   - 一侧有 part、一侧空 ⇒ 仍可救；
//   - 同日两侧 part 都有值且不同 ⇒ 不救。
func TestFunnelPiece_L3_PartGuard(t *testing.T) {
	// 一侧 part 空 ⇒ 判齐（L2 常先吃掉，这里只断言结果）
	a := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240101"}
	b := map[int]string{1: "s=-|v=-|p=上|n=-|d=20240101"}
	st, _ := funnelPiece(piece(a, b))
	if st.Leftover != 0 || st.MksAligned != 1 {
		t.Fatalf("一侧 part 空的日期槽应判齐: %+v", st)
	}
	// 同日但 part 不同（上 vs 中）⇒ 守卫拒绝
	c := map[int]string{1: "s=-|v=-|p=上|n=-|d=20240101"}
	d := map[int]string{1: "s=-|v=-|p=中|n=-|d=20240101"}
	st2, _ := funnelPiece(piece(c, d))
	if st2.L3Date != 0 {
		t.Fatalf("同日 part 不同的槽不得被 L3 救: %+v", st2)
	}
	if st2.Leftover == 0 {
		t.Fatalf("该槽应落 leftover: %+v", st2)
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
	if st.L0Single+st.L1Slot+st.L2Tolerant+st.L3Date+st.L4Num+st.L5Subset+st.L6Tol+st.MixedDiag+st.Leftover != st.SlotsTotal {
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

// L6 容忍层（2026-11-17 用户定）：纯 n / 纯 d 片，缺集不算解析异常 ⇒ 整片剩余槽容忍为对齐。
//   - 纯 n 片要求每行 n 来源 ∈ {self, single}（bridge/anchor 不得自证）；
//   - 纯 d 片**不校验来源**（日期不可跨源借）；
//   - 含 s/v 的片不进 L6；同一 n/d 上两侧 part 都不同仍拒救。
func TestFunnelPiece_L6_PureTolerance(t *testing.T) {
	// 纯 n 片、互有独有值（A={1,2,3}、B={1,2,9}）⇒ L6 容忍为对齐
	a := map[int]string{
		1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=3|d=-",
	}
	b := map[int]string{
		1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=-|v=-|p=-|n=9|d=-",
	}
	from := map[int64]string{1: "self", 2: "self"}
	st, _ := funnelPieceSrc(piece(a, b), from)
	if st.L6Tol == 0 || st.Leftover != 0 {
		t.Fatalf("纯 n 片（来源 self）应被 L6 容忍为对齐: %+v", st)
	}
	// 纯 n 片但含 bridge 来源 ⇒ 不救
	fromBridge := map[int64]string{1: "self", 2: "bridge"}
	st2, _ := funnelPieceSrc(piece(a, b), fromBridge)
	if st2.L6Tol != 0 {
		t.Fatalf("含 bridge 来源的纯 n 片不得被 L6 救: %+v", st2)
	}
	// 纯 d 片、来源 none（原文）⇒ 不校验来源，仍救
	c := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240101", 2: "s=-|v=-|p=-|n=-|d=20240102", 3: "s=-|v=-|p=-|n=-|d=20240103",
		4: "s=-|v=-|p=-|n=-|d=20240104", 5: "s=-|v=-|p=-|n=-|d=20240105", 6: "s=-|v=-|p=-|n=-|d=20240106",
	}
	// B 里有一个 A 没有的日期（0109）⇒ 日期集不构成子集 ⇒ L3 救不了 ⇒ 只能靠 L6
	d := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240104", 2: "s=-|v=-|p=-|n=-|d=20240105", 3: "s=-|v=-|p=-|n=-|d=20240109",
	}
	fromD := map[int64]string{1: "none", 2: "none"}
	st3, _ := funnelPieceSrc(piece(c, d), fromD)
	if st3.L6Tol == 0 || st3.Leftover != 0 {
		t.Fatalf("纯 d 片应被 L6 容忍（不校验来源）: %+v", st3)
	}
	// 含 s/v 的片不进 L6
	e := map[int]string{1: "s=栏目|v=-|p=-|n=1|d=-"}
	f := map[int]string{1: "s=-|v=-|p=-|n=2|d=-"}
	st4, _ := funnelPieceSrc(piece(e, f), from)
	if st4.L6Tol != 0 {
		t.Fatalf("含 s 的片不得进 L6: %+v", st4)
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
	sum := st.L0Single + st.L1Slot + st.L2Tolerant + st.L3Date + st.L4Num + st.L5Subset + st.L6Tol + st.MixedDiag + st.Leftover
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

// 复审（2026-11-18）：from_src 按 (mk, src_row_id, ep_index) 逐行取值，但 L6 的 self 门按**源行**判定
// ⇒ 同源行各集混 self/bridge 时曾以"最后一次写入"为准 ⇒ bridge 推算的 n 可能被误当自证。
// 修法：同源行来源不一时标 "mixed"（ownN=false 保守）——与按集判定**判等价**
// （混来源行必含 bridge-n ⇒ 纯 n 本就 false）。本测试钉住：混来源行不得自证。
func TestFunnelPiece_L6_MixedFromSrc不自证(t *testing.T) {
	bySrc := map[int64]map[int]string{
		1: {1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"},  // 同源行：ep1=self、ep2=bridge（混）
		2: {1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"}, // 纯 self 行
	}
	from := map[int64]string{1: "self"} // 末次写入=self（构造前会先写 bridge）——直接给 mixed 行为的终态
	// 模拟修复前的坍缩：from[1] 只剩一个值 ⇒ ep2 的 bridge-n 被误当自证
	// 修复后：混合来源行 ⇒ ownN=false ⇒ 纯 n 判 false ⇒ L6 不救
	st, ok := funnelPieceSrc(bySrc, from)
	_ = st
	_ = ok
	// 该片槽 1/2 都是双行同 epid ⇒ L0/L1 已判齐，L6 不会触达 —— 换成双行不同 epid 的构造：
	bySrc2 := map[int64]map[int]string{
		1: {1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"},
		2: {1: "s=-|v=-|p=-|n=1|d=-", 3: "s=-|v=-|p=-|n=3|d=-"},
	}
	// 源1 的 from_src 混（self/bridge 各一）⇒ ownN=false ⇒ 纯 n 判 false
	st2, ok2 := funnelPieceSrc(bySrc2, map[int64]string{1: "mixed", 2: "self"})
	if ok2 && st2.L6Tol > 0 {
		t.Fatalf("混来源行不得自证 ⇒ L6 不应救回: L6Tol=%d", st2.L6Tol)
	}
	_ = st
}

// L6.3（S1，2026-11-18）：纯 d 按槽容忍
//   - 正例：片内有 s 脏行 ⇒ L6.1/L6.2（整片）不成立，但"槽内纯 d 且同日唯一"的槽应被 L6.3 容忍；
//   - 反例1：同日在同一源出现 2 次（未识别的同日多集/重复行）⇒ 日期不能当身份 ⇒ 拒绝；
//   - 反例2：槽内一侧是 n ⇒ 不是纯 d ⇒ 拒绝。
func TestFunnelPiece_L6_3_SlotPureDate(t *testing.T) {
	a := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240101",
		2: "s=-|v=-|p=-|n=-|d=20240102",
		3: "s=大结局|v=-|p=-|n=-|d=20240103",
	}
	b := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240101",
		2: "s=-|v=-|p=-|n=-|d=20240105",
		3: "s=-|v=-|p=-|n=-|d=20240103",
	}
	st, _ := funnelPieceSrc(piece(a, b), map[int64]string{1: "none", 2: "none"})
	if st.L6Slot != 1 || st.L6Tol != 1 {
		t.Fatalf("应有 1 槽走 L6.3：%+v", st)
	}
	if st.L6Num != 0 || st.L6Date != 0 {
		t.Fatalf("L6.1/L6.2 不应命中（片内有 s 行）：%+v", st)
	}
	if st.SlotsTotal != st.L0Single+st.L1Slot+st.L2Tolerant+st.L3Date+st.L4Num+st.L5Subset+st.L6Tol+st.MixedDiag+st.Leftover {
		t.Fatalf("不变量被破坏：%+v", st)
	}

	// 反例1：a 源同一天两条（同日重复）⇒ 该日期不可当身份
	a2 := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240101",
		2: "s=-|v=-|p=-|n=-|d=20240102",
		3: "s=-|v=-|p=-|n=-|d=20240102",
		4: "s=会员版|v=-|p=-|n=-|d=20240109",
	}
	b2 := map[int]string{
		1: "s=-|v=-|p=-|n=-|d=20240101",
		2: "s=-|v=-|p=-|n=-|d=20240105",
		3: "s=-|v=-|p=-|n=-|d=20240106",
		4: "s=-|v=-|p=-|n=-|d=20240109",
	}
	st2, _ := funnelPieceSrc(piece(a2, b2), map[int64]string{1: "none", 2: "none"})
	if st2.L6Slot != 0 {
		t.Fatalf("同日重复不应被 L6.3 容忍：%+v", st2)
	}
	if st2.Leftover+st2.MixedDiag == 0 {
		t.Fatalf("应有未对齐槽：%+v", st2)
	}

	// 反例2：槽内一侧是 n ⇒ 不是纯 d
	a3 := map[int]string{1: "s=-|v=-|p=-|n=-|d=20240101"}
	b3 := map[int]string{1: "s=-|v=-|p=-|n=3|d=-"}
	st3, _ := funnelPieceSrc(piece(a3, b3), map[int64]string{1: "none", 2: "none"})
	if st3.L6Slot != 0 {
		t.Fatalf("槽内含 n 不应走 L6.3：%+v", st3)
	}
}
// L7（2026-11-19，子集法）：剔除 `s=合集` 行后按子集判对齐。
//   - 正例A：源1 只有合集行 + 源2 正常集 ⇒ 槽内剔合集后仅剩可比较行 ⇒ 对齐；
//   - 正例B：源1 正常 + 源2 正常+合集 ⇒ 剔合集后可比较行一致 ⇒ 对齐；
//   - 反例1：整槽全为合集行且冲突 ⇒ **不剔**（保持原判，不得白送对齐）；
//   - 反例2：可比较行之间冲突 ⇒ 仍不对齐（L7 不得掩盖真实分歧）。
func TestFunnelPiece_L7_PackSubset(t *testing.T) {
	// 正例A：a 只有合集（上/下），b 有正常集
	a := map[int]string{1: "s=合集|v=-|p=上|n=-|d=-", 2: "s=合集|v=-|p=下|n=-|d=-"}
	b := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"}
	st, _ := funnelPieceSrc(piece(a, b), map[int64]string{1: "none", 2: "none"})
	if st.Leftover != 0 || st.MixedDiag != 0 {
		t.Fatalf("场景A 应判对齐：%+v", st)
	}
	if st.L7PackRows != 2 || st.L7Slots != 2 {
		t.Fatalf("场景A 应剔除 2 行/2 槽：%+v", st)
	}

	// 正例B：a 正常，b 正常 + 合集（多出一个合集槽）
	a2 := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-"}
	b2 := map[int]string{1: "s=-|v=-|p=-|n=1|d=-", 2: "s=-|v=-|p=-|n=2|d=-", 3: "s=合集|v=-|p=-|n=-|d=-"}
	st2, _ := funnelPieceSrc(piece(a2, b2), map[int64]string{1: "none", 2: "none"})
	if st2.Leftover != 0 || st2.MixedDiag != 0 {
		t.Fatalf("场景B 应判对齐（合集槽单独存在、无可比较对手）：%+v", st2)
	}

	// 反例1：整槽全为合集行且两行不同（p 不同）⇒ 不剔 ⇒ 该槽仍算未对齐
	// 说明：此时 hasCmp 为空 ⇒ 不剔除 ⇒ 走原逻辑
	a3 := map[int]string{1: "s=合集|v=-|p=上|n=-|d=-"}
	b3 := map[int]string{1: "s=合集|v=-|p=下|n=-|d=-"}
	st3, _ := funnelPieceSrc(piece(a3, b3), map[int64]string{1: "none", 2: "none"})
	if st3.L7PackRows != 0 {
		t.Fatalf("整槽全合集不应剔除：%+v", st3)
	}

	// 反例2：可比较行冲突 ⇒ 仍不对齐（单槽，L2 不介入）
	a4 := map[int]string{1: "s=-|v=-|p=-|n=1|d=-"}
	b4 := map[int]string{1: "s=-|v=-|p=-|n=9|d=-"}
	st4, _ := funnelPieceSrc(piece(a4, b4), map[int64]string{1: "none", 2: "none"})
	if st4.Leftover+st4.MixedDiag == 0 {
		t.Fatalf("可比较行冲突不应被判对齐：%+v", st4)
	}
	if st4.L7PackRows != 0 {
		t.Fatalf("无可剔除行：%+v", st4)
	}
}