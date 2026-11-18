package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// FunnelPolicyVersion 漏斗口径版本。
//
// ⚠️ **口径语义变更时必须递增**（比如 L5/L6 的准入、L3/L4 的键、part 守卫），
// 否则历史快照与现值不可比 —— 报表与归因文档都以此字段做「同口径对账」。
const FunnelPolicyVersion = "L0-L6@2026-11-18"

// FunnelSnapshot 一次漏斗快照（整表覆盖写：库里只保留最近一批）。
type FunnelSnapshot struct {
	ComputedAt    time.Time
	PolicyVersion string
	Truncated     bool
	ByCategory    []FunnelStat
	All           FunnelStat
}

// SaveFunnelSnapshot 覆盖写入一次快照（先清空再插入，单事务）。
// rows = 各分类；all = 全库合计（pid = -1）。
func (r *EpidRepo) SaveFunnelSnapshot(ctx context.Context, rows []FunnelStat, all FunnelStat, truncated bool) error {
	return dbFrom(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM epid_alignment_funnel_snapshot").Error; err != nil {
			return err
		}
		now := time.Now()
		trunc := 0
		if truncated {
			trunc = 1
		}
		batch := make([]map[string]interface{}, 0, len(rows)+1)
		appendRow := func(st FunnelStat) {
			batch = append(batch, map[string]interface{}{
				"computed_at":    now,
				"policy_version": FunnelPolicyVersion,
				"truncated":      trunc,
				"pid":            st.Pid,
				"category":       st.Category,
				"slots_total":    st.SlotsTotal,
				"l0_single":      st.L0Single,
				"l1_slot":        st.L1Slot,
				"l2_tolerant":    st.L2Tolerant,
				"l3_date":        st.L3Date,
				"l4_num":         st.L4Num,
				"l5_subset":      st.L5Subset,
				"l6_tol":         st.L6Tol,
				"l6_num":         st.L6Num,
				"l6_date":        st.L6Date,
				"l6_slot":        st.L6Slot,
				"mixed_diag":     st.MixedDiag,
				"leftover":       st.Leftover,
				"mks_total":      st.MksTotal,
				"mks_aligned":    st.MksAligned,
				"aligned_slots":  st.AlignedSlots,
				"slot_pct":       st.SlotPct,
				"mk_pct":         st.MkPct,
			})
		}
		for _, st := range rows {
			appendRow(st)
		}
		appendRow(all)
		return tx.Table("epid_alignment_funnel_snapshot").Create(&batch).Error
	})
}

// LoadFunnelSnapshot 读最近一次快照；没有快照时返回 (零值, false, nil)。
func (r *EpidRepo) LoadFunnelSnapshot(ctx context.Context) (FunnelSnapshot, bool, error) {
	var out FunnelSnapshot
	var rows []struct {
		ComputedAt    time.Time
		PolicyVersion string
		Truncated     int
		Pid           int64
		Category      string
		SlotsTotal    int64
		L0Single      int64
		L1Slot        int64
		L2Tolerant    int64
		L3Date        int64
		L4Num         int64
		L5Subset      int64
		L6Tol         int64
		L6Num         int64
		L6Date        int64
		L6Slot        int64
		MixedDiag     int64
		Leftover      int64
		MksTotal      int64
		MksAligned    int64
		AlignedSlots  int64
		SlotPct       float64
		MkPct         float64
	}
	if err := dbFrom(ctx, r.db).Raw(`
		SELECT computed_at, policy_version, truncated, pid, category,
		       slots_total, l0_single, l1_slot, l2_tolerant, l3_date, l4_num,
		       l5_subset, l6_tol, l6_num, l6_date, l6_slot, mixed_diag, leftover, mks_total, mks_aligned,
		       aligned_slots, slot_pct, mk_pct
		FROM epid_alignment_funnel_snapshot
		ORDER BY (pid = -1), slots_total DESC`).Scan(&rows).Error; err != nil {
		return out, false, err
	}
	if len(rows) == 0 {
		return out, false, nil
	}
	out.ComputedAt = rows[0].ComputedAt
	out.PolicyVersion = rows[0].PolicyVersion
	out.Truncated = rows[0].Truncated == 1
	out.ByCategory = make([]FunnelStat, 0, len(rows))
	for _, r0 := range rows {
		st := FunnelStat{
			Pid: r0.Pid, Category: r0.Category,
			SlotsTotal: r0.SlotsTotal, L0Single: r0.L0Single, L1Slot: r0.L1Slot,
			L2Tolerant: r0.L2Tolerant, L3Date: r0.L3Date, L4Num: r0.L4Num,
			L5Subset: r0.L5Subset, L6Tol: r0.L6Tol, L6Num: r0.L6Num, L6Date: r0.L6Date, L6Slot: r0.L6Slot, MixedDiag: r0.MixedDiag,
			Leftover: r0.Leftover, MksTotal: r0.MksTotal, MksAligned: r0.MksAligned,
			AlignedSlots: r0.AlignedSlots, SlotPct: r0.SlotPct, MkPct: r0.MkPct,
		}
		if r0.Pid == -1 {
			out.All = st
			continue
		}
		out.ByCategory = append(out.ByCategory, st)
	}
	return out, true, nil
}
