package mysql

import (
	"context"
	"math"
	"sort"
	"strconv"

	"server/internal/danmu"
)

// ===== 对齐率分层漏斗（L0~L5，2026-11-17）=====
//
// 总则见 Harness：`docs/epid-集身份/方案-对齐率分层判定总则-2026-11-17.md`
//
// 只做**判定**（在 epid 现状下判断对齐），**不改 epid、不写任何表**；按需/离线：
// 不进 `heavyMetricsSnapshot` 的 600s 快照、不进请求路径。
//
// 分层（每层只吃上一层未判齐的剩余；分母恒为全部非空槽位）：
//
//	L0 单源槽（只有 1 个源有该集）                     → 默认对齐（覆盖差异不是解析问题）
//	L1 槽位全等（同一 ep_index 且 epid 全等）          → 对齐
//	L2 值序列容忍（允许插入/删除、不允许替换，M4）      → 整片位序错位救回
//	L3 日期锚（跨槽：按 (d,part) 对齐，s/v/n 空的行）    → 救回
//	L4 期号锚（跨槽：按 `n` 值对齐，s/v/d 空的行；part 仅作守卫）  → 救回
//	L5 正片子集（全片无 s/v ∧ 每源 epid 集合 ⊆ 最大源集合）        → 救回（覆盖差异不算错）
//	诊断档 多身份槽（含 s/v，**不判定** ⇒ 计入未对齐）              → 不救回
//	—  剩余                                                        → 真未对齐，进归因
//
// ⚠️ **L3/L4 是"跨槽"判定**：位置口径下 A 的第 5 集与 B 的第 6 集会被判成两个分歧槽；
// 按锚对齐能把它们认成同一集 ⇒ 这类"本身能对上、只因子集/错位没算进去"的槽才算救回。
// （审查教训 2026-11-17：若写成"槽内锚唯一"，则永不触发 —— 槽内 epid 不同时差异恰在锚上。）
//
// ⚠️ 不变量：SlotsTotal == L0+L1+L2+L3+L4+L5Subset+MixedDiag+Leftover（单测钉住）。
type FunnelStat struct {
	Pid      int64  `json:"pid"`
	Category string `json:"category"`

	SlotsTotal int64 `json:"slotsTotal"` // 分母：该分类全部非空 epid 槽位
	L0Single   int64 `json:"l0Single"`   // 单源槽（默认对齐）
	L1Slot     int64 `json:"l1Slot"`     // 多源且 epid 全等
	L2Tolerant int64 `json:"l2Tolerant"` // 值序列容忍（仅插删）救回的槽
	L3Date     int64 `json:"l3Date"`     // 日期锚（跨槽）救回的槽
	L4Num      int64 `json:"l4Num"`      // 期号锚（跨槽）救回的槽
	L5Subset   int64 `json:"l5Subset"`   // L5 正片子集救回（全片无 s/v ∧ 每源 epid 集合 ⊆ 最大源）
	MixedDiag  int64 `json:"mixedDiag"`  // 诊断档：多身份槽（含 s/v），不判定、计入未对齐
	Leftover   int64 `json:"leftover"`   // 以上都不成立

	MksTotal   int64 `json:"mksTotal"`   // 有非空 epid 的片数
	MksAligned int64 `json:"mksAligned"` // 片内 leftover == 0（且 mixedDiag == 0）的片数

	// ===== 对齐率（分层口径，2026-11-17 用户定：L2~L5 也纳入）=====
	// 分母恒为全部非空槽位/全部有身份片；分子 = 符合任一层口径的（L0~L5 救回的都算对齐）。
	AlignedSlots int64   `json:"alignedSlots"` // = SlotsTotal - MixedDiag - Leftover
	SlotPct      float64 `json:"slotPct"`      // 集级对齐率（分层口径）
	MkPct        float64 `json:"mkPct"`        // 片级对齐率（分层口径）
}

// ClassAlignmentFunnel 分层漏斗读数（按分类 + 全库）。
// 批策略与 `ClassAlignmentTolerant` 一致（keyset 每批 10 万 mk、批内 Go 归并）。
func (r *EpidRepo) ClassAlignmentFunnel(ctx context.Context) ([]FunnelStat, FunnelStat, error) {
	catPID := map[int64]int64{}
	catName := map[int64]string{}
	{
		var rows []struct {
			Id   int64
			Pid  int64
			Name string
		}
		if err := dbFrom(ctx, r.db).Raw("SELECT id, pid, name FROM category").Scan(&rows).Error; err != nil {
			return nil, FunnelStat{}, err
		}
		for _, c := range rows {
			catPID[c.Id], catName[c.Id] = c.Pid, c.Name
		}
	}
	resolveCategory := func(pid, cid int64) int64 {
		out := pid
		if out == 0 {
			out = cid
		}
		if p, ok := catPID[out]; ok && p > 0 {
			out = p
		}
		return out
	}

	accs := map[int64]*FunnelStat{}
	var all FunnelStat
	const page = 100000
	after := int64(math.MinInt64)
	for i := 0; i < 32; i++ {
		var next int64
		if err := dbFrom(ctx, r.db).Raw(
			"SELECT COALESCE(MAX(mk),0) FROM (SELECT match_key AS mk FROM movie_play_source WHERE match_key > ? ORDER BY match_key LIMIT ?) t",
			after, page).Scan(&next).Error; err != nil {
			return nil, all, err
		}
		if next == 0 {
			break
		}
		var rows []struct {
			Mk     string
			RowID  int64
			EpIdx  int
			Epid   string
			Pid    int64
			Cid    int64
			HasMid int64
		}
		err := dbFrom(ctx, r.db).Raw(`
			SELECT e.match_key AS mk, e.src_row_id AS row_id, e.ep_index AS ep_idx, e.epid,
			       MAX(m.pid) AS pid, MAX(m.cid) AS cid, MAX(e.mid IS NOT NULL) AS has_mid
			FROM movie_episode_epid e
			LEFT JOIN movie_play_source src ON src.id = e.src_row_id
			LEFT JOIN movie m ON m.mid = src.mid
			WHERE e.epid <> '' AND e.dup_copy = 0 AND e.match_key > ? AND e.match_key <= ?
			GROUP BY e.match_key, e.src_row_id, e.ep_index`, after, next).Scan(&rows).Error
		if err != nil {
			return nil, all, err
		}

		type mkSeq struct {
			pid, cid, hasMid int64
			bySrc            map[int64]map[int]string
		}
		perMK := make(map[string]*mkSeq, len(rows)/4+1)
		for j := range rows {
			rr := rows[j]
			m := perMK[rr.Mk]
			if m == nil {
				m = &mkSeq{bySrc: map[int64]map[int]string{}}
				perMK[rr.Mk] = m
			}
			if rr.Pid != 0 {
				m.pid = rr.Pid
			}
			if rr.Cid != 0 {
				m.cid = rr.Cid
			}
			if rr.HasMid == 1 {
				m.hasMid = 1
			}
			sm := m.bySrc[rr.RowID]
			if sm == nil {
				sm = map[int]string{}
				m.bySrc[rr.RowID] = sm
			}
			sm[rr.EpIdx] = rr.Epid
		}

		for _, m := range perMK {
			st, ok := funnelPiece(m.bySrc)
			if !ok {
				continue
			}
			cat := resolveCategory(m.pid, m.cid)
			if cat <= 0 && m.hasMid == 1 {
				cat = PidUnresolvedWithMid
			}
			a := accs[cat]
			if a == nil {
				a = &FunnelStat{}
				accs[cat] = a
			}
			addFunnel(a, st)
		}
		after = next
	}

	out := make([]FunnelStat, 0, len(accs))
	for pid, a := range accs {
		st := *a
		st.Pid, st.Category = pid, categoryDisplayName(pid, catName)
		finalizeFunnel(&st)
		out = append(out, st)
		addFunnel(&all, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SlotsTotal > out[j].SlotsTotal })
	all.Pid, all.Category = -1, "全部"
	finalizeFunnel(&all)
	return out, all, nil
}

// funnelPiece 对单片的槽位做 L0~L5 分层。返回 (该片统计, 是否有非空槽)。
func funnelPiece(bySrc map[int64]map[int]string) (FunnelStat, bool) {
	var st FunnelStat
	slots := map[int][]string{} // ep_index → 该槽所有行的 epid
	for _, sm := range bySrc {
		for idx, ep := range sm {
			slots[idx] = append(slots[idx], ep)
		}
	}
	if len(slots) == 0 {
		return st, false
	}
	st.SlotsTotal = int64(len(slots))
	st.MksTotal = 1

	unaligned := make([]int, 0, 8)
	for idx, eps := range slots {
		if len(eps) == 1 { // L0 单源槽
			st.L0Single++
			continue
		}
		if len(distinctStrings(eps)) == 1 { // L1 槽位全等
			st.L1Slot++
			continue
		}
		unaligned = append(unaligned, idx)
	}

	// L2 值序列容忍（仅插入/删除、不允许替换）
	if len(unaligned) > 0 {
		seqs := make([][]string, 0, len(bySrc))
		for _, sm := range bySrc {
			idx := make([]int, 0, len(sm))
			for k := range sm {
				idx = append(idx, k)
			}
			sort.Ints(idx)
			seq := make([]string, 0, len(idx))
			for _, k := range idx {
				seq = append(seq, sm[k])
			}
			seqs = append(seqs, seq)
		}
		if ok, _, _ := danmu.TolerantMkAligned(seqs, -1); ok {
			st.L2Tolerant += int64(len(unaligned))
			unaligned = nil
		}
	}

	// L3 日期锚 / L4 期号锚：**跨槽**按锚键两两相容（片级建立对应）；
	// ⚠️ 但只救"槽内**每一行**都是该锚的干净行"的槽（2026-11-17 审查）：
	// 锚只覆盖干净行（s/v 空），含 s/v 的槽不在锚的背书范围内 ⇒ 不得一并救回（防误伤）。
	if len(unaligned) > 0 {
		if usable, aligned := anchorAligned(anchorKeySets(bySrc, anchorDate)); usable && aligned {
			keep := unaligned[:0]
			for _, idx := range unaligned {
				if slotAllCleanAnchor(slots[idx], anchorDate) {
					st.L3Date++
				} else {
					keep = append(keep, idx)
				}
			}
			unaligned = keep
		}
	}
	if len(unaligned) > 0 && !numPartConflicted(bySrc) {
		if usable, aligned := anchorAligned(anchorKeySets(bySrc, anchorNum)); usable && aligned {
			keep := unaligned[:0]
			for _, idx := range unaligned {
				if slotAllCleanAnchor(slots[idx], anchorNum) {
					st.L4Num++
				} else {
					keep = append(keep, idx)
				}
			}
			unaligned = keep
		}
	}

	// L5 正片子集（2026-11-17 用户定）：**全片不含 s/v** ∧ 每源 epid 值集合都 ⊆ 最大源集合
	// ⇒ 只是"缺集"（覆盖差异），不算解析错 ⇒ 该片剩余未对齐槽全部救回。
	// ⚠️ 只吃正片：s/v 非空时"集合子集相等"不等于 epid 相等（不同栏目会撞成同集）。
	if len(unaligned) > 0 && normPiece(bySrc) && valueSubsetOK(bySrc) {
		st.L5Subset += int64(len(unaligned))
		unaligned = nil
	}

	// 剩余再分档：含 s/v 的多身份槽 → 诊断档（不判定，计入未对齐）；其余 → leftover
	for _, idx := range unaligned {
		if slotHasMultiIdentity(slots[idx]) {
			st.MixedDiag++
		} else {
			st.Leftover++
		}
	}
	// 片级对齐 = 整片无未对齐（诊断档同样阻断）
	if st.Leftover == 0 && st.MixedDiag == 0 {
		st.MksAligned = 1
	}
	return st, true
}

type anchorKind int

const (
	anchorDate anchorKind = iota // 日期锚（L3）：s/v 空 ∧ n 空
	anchorNum                    // 期号锚（L4）：s/v 空 ∧ d 空
)

// anchorKeySets 抽取每源的锚键集合（**只吃满足该层准入的干净行**）：
//   - 日期锚（L3）：`s` 空 ∧ `v` 空 ∧ `n` 空 ∧ `d` 非空 ⇒ 键 = `d`(+`#part`)
//   - 期号锚（L4）：`s` 空 ∧ `v` 空 ∧ `d` 空 ∧ `n` > 0 ⇒ 键 = **`n` 值**（不含 part；2026-11-17 放宽）
//
// ⚠️ L4 为什么把 part 从键里拿掉：实测 19,840 个"错位一格/小偏移"槽中，按 `n` 值子集有 71.5%
// 可救，而按 `(n,part)` 复合键会因"一侧有 part、一侧无"整体失败（槽内 part 冲突实测 0 例）。
// part 的语义改由 `numPartConflicted` 守卫（两侧 part 都有值且不同 ⇒ 不救）。
//
// ⚠️ 为什么只有这两层能用锚（2026-11-17 用户定）：这两层里"锚相等"**等价于 `epid` 相等**
// ——干净行（s/v/n 空）的 epid 只由 `d`(+`p`) 决定，`d` 相等 ⇒ 串必相等 ⇒ 判定结果与
// "epid 全等"完全一致。含 `s`/`v` 的行做不到这一点（锚相等 ≠ epid 相等）⇒ 不设锚层。
func anchorKeySets(bySrc map[int64]map[int]string, kind anchorKind) [][]string {
	out := make([][]string, 0, len(bySrc))
	for _, sm := range bySrc {
		keys := make([]string, 0, len(sm))
		for _, ep := range sm {
			k, ok := danmu.ParseEpidString(ep)
			if !ok {
				continue
			}
			if k.Series != "" || k.Variant != "" || k.VariantNum != 0 {
				continue // 多身份行：不进锚层（锚相等 ≠ epid 相等）
			}
			switch kind {
			case anchorDate:
				if k.Num != 0 || k.Date == "" {
					continue
				}
				keys = append(keys, k.Date+"#"+k.Part)
			case anchorNum:
				if k.Date != "" || k.Num == 0 {
					continue
				}
				// 键只用 n 值（2026-11-17 放宽）：实测「n 值子集」能覆盖 71.5% 的错位槽，
				// 而把 `part` 拉进等值判据会让"一侧有 part、一侧无 part"的片整体子集失败。
				// part 的语义改由 numPartConflicted 守卫处理（两侧都有值且不同 ⇒ 不救）。
				keys = append(keys, strconv.Itoa(k.Num))
			}
		}
		if len(keys) > 0 {
			out = append(out, keys)
		}
	}
	return out
}

// anchorAligned 判多源的锚键集合是否可用且两两相容。
// 守卫：① ≥2 个源有非空锚集合；② **源内锚键唯一**（同源同锚 ≥2 行 ⇒ 该层放弃，宁缺勿错）。
// 判据：任两源较短集合都是较长集合的子集（值语义、忽略位置；不放过交错缺集）。
func anchorAligned(perSrc [][]string) (usable bool, aligned bool) {
	cleaned := make([][]string, 0, len(perSrc))
	for _, keys := range perSrc {
		if len(keys) == 0 {
			continue
		}
		seen := make(map[string]struct{}, len(keys))
		for _, k := range keys {
			if _, dup := seen[k]; dup {
				return false, false // 源内锚不唯一 ⇒ 本层放弃
			}
			seen[k] = struct{}{}
		}
		cleaned = append(cleaned, keys)
	}
	if len(cleaned) < 2 {
		return false, false
	}
	for i := 0; i < len(cleaned); i++ {
		for j := i + 1; j < len(cleaned); j++ {
			if !anchorSubsetEither(cleaned[i], cleaned[j]) {
				return true, false
			}
		}
	}
	return true, true
}

func anchorSubsetEither(a, b []string) bool {
	sa, sb := stringSet(a), stringSet(b)
	if len(sa) <= len(sb) {
		return stringSubset(sa, sb)
	}
	return stringSubset(sb, sa)
}

func stringSet(in []string) map[string]struct{} {
	m := make(map[string]struct{}, len(in))
	for _, s := range in {
		m[s] = struct{}{}
	}
	return m
}

func stringSubset(small, big map[string]struct{}) bool {
	for k := range small {
		if _, ok := big[k]; !ok {
			return false
		}
	}
	return true
}

// slotAllCleanAnchor 该槽的**每一行**是否都属于该锚层的准入集合：
//   - 日期锚（L3）：`s` 空 ∧ `v` 空 ∧ `n` 空 ∧ `d` 非空
//   - 期号锚（L4）：`s` 空 ∧ `v` 空 ∧ `d` 空 ∧ `n` > 0
//
// 各层只给"整槽都属于本层"的槽背书 ⇒ 不跨层误伤。
func slotAllCleanAnchor(eps []string, kind anchorKind) bool {
	if len(eps) == 0 {
		return false
	}
	for _, ep := range eps {
		k, ok := danmu.ParseEpidString(ep)
		if !ok {
			return false
		}
		if k.Series != "" || k.Variant != "" || k.VariantNum != 0 {
			return false
		}
		switch kind {
		case anchorDate:
			if k.Num != 0 || k.Date == "" {
				return false
			}
		case anchorNum:
			if k.Date != "" || k.Num == 0 {
				return false
			}
		}
	}
	return true
}

// slotHasMultiIdentity 该槽是否含"多身份行"（`s`/`v` 任一非空）⇒ L5 诊断分档。
func slotHasMultiIdentity(eps []string) bool {
	for _, ep := range eps {
		k, ok := danmu.ParseEpidString(ep)
		if !ok {
			continue
		}
		if k.Series != "" || k.Variant != "" || k.VariantNum != 0 {
			return true
		}
	}
	return false
}

// numPartConflicted 同一 n 值上是否出现"两个不同的非空 part"。
//
// L4 放宽（键=仅 n）后的守卫：若同一集号在两处各有明确 part 且不同（如 上/中/下），
// 那是**不同的集**，不得判齐。一侧无 part 属"表示差异"，不算冲突（实测该类冲突为 0，
// 所以这条守卫不会削弱放宽带来的收益）。
func numPartConflicted(bySrc map[int64]map[int]string) bool {
	parts := map[int]map[string]struct{}{}
	for _, sm := range bySrc {
		for _, ep := range sm {
			k, ok := danmu.ParseEpidString(ep)
			if !ok {
				continue
			}
			if k.Series != "" || k.Variant != "" || k.VariantNum != 0 || k.Date != "" || k.Num == 0 || k.Part == "" {
				continue
			}
			if parts[k.Num] == nil {
				parts[k.Num] = map[string]struct{}{}
			}
			parts[k.Num][k.Part] = struct{}{}
		}
	}
	for _, ps := range parts {
		if len(ps) > 1 {
			return true
		}
	}
	return false
}

// normPiece 全片每一行都不含 s/v（"正片"）：s、v、变体序号全空。
func normPiece(bySrc map[int64]map[int]string) bool {
	n := 0
	for _, sm := range bySrc {
		for _, ep := range sm {
			k, ok := danmu.ParseEpidString(ep)
			if !ok {
				return false
			}
			if k.Series != "" || k.Variant != "" || k.VariantNum != 0 {
				return false
			}
			n++
		}
	}
	return n > 0
}

// valueSubsetOK 每源 epid 值集合是否都 ⊆ 某个"最大源"的集合（等价：并集大小 == 最大集合大小），
// 且非空源 ≥2。这是 L5 的判据：只要没有任何"同集两身份"，缺集就只是覆盖差异。
func valueSubsetOK(bySrc map[int64]map[int]string) bool {
	sets := 0
	maxLen := 0
	union := map[string]struct{}{}
	for _, sm := range bySrc {
		s := make(map[string]struct{}, len(sm))
		for _, ep := range sm {
			if ep == "" {
				continue
			}
			s[ep] = struct{}{}
			union[ep] = struct{}{}
		}
		if len(s) == 0 {
			continue
		}
		sets++
		if len(s) > maxLen {
			maxLen = len(s)
		}
	}
	if sets < 2 || maxLen == 0 {
		return false
	}
	return len(union) == maxLen
}

// finalizeFunnel 补齐派生字段：AlignedSlots / SlotPct / MkPct（分层口径对齐率）。
// 分母恒为全部非空槽位与全部有身份片；混合身份诊断档与 leftover 都算未对齐。
func finalizeFunnel(st *FunnelStat) {
	st.AlignedSlots = st.SlotsTotal - st.MixedDiag - st.Leftover
	if st.SlotsTotal > 0 {
		st.SlotPct = float64(st.AlignedSlots) * 100 / float64(st.SlotsTotal)
	}
	if st.MksTotal > 0 {
		st.MkPct = float64(st.MksAligned) * 100 / float64(st.MksTotal)
	}
}

func addFunnel(dst *FunnelStat, src FunnelStat) {
	dst.SlotsTotal += src.SlotsTotal
	dst.L0Single += src.L0Single
	dst.L1Slot += src.L1Slot
	dst.L2Tolerant += src.L2Tolerant
	dst.L3Date += src.L3Date
	dst.L4Num += src.L4Num
	dst.L5Subset += src.L5Subset
	dst.MixedDiag += src.MixedDiag
	dst.Leftover += src.Leftover
	dst.MksTotal += src.MksTotal
	dst.MksAligned += src.MksAligned
}

func distinctStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
