package mysql

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"server/internal/domain/entity"
)

// EpidOverrideRepo 裁定台账 movie_epid_override 的读写（epid 源头校正层 · 第②段，迁移 000049，2026-11-12）。
//
// 与 EpidRepo（派生表）的分工，口径见 docs/epid-集身份/方案-epid源头校正层-2026-11-12.md §3.3/§8：
//   - 派生表 movie_episode_epid 单向派生、只由派生任务写；本台账只放"已经判过"的结论；
//   - 大表**零新增列** ⇒ 应用侧（T3 ApplyOverrides）从本台账 ListActive 一次性预加载，
//     在派生写库前对 pendingRows 做**内存命中替换**（不额外取业务数据，不影响派生效率）；
//   - 应用前三元组校验 (src_row_id, key_raw_name, key_epid) —— 对不上宁可不用（**宁缺勿错**）。
//
// 刻意导出**具体类型**（同 EpidRepo 的取向，不走 ports.go 公共端口）：服务层只依赖它，降低回归面。
type EpidOverrideRepo struct{ db *gorm.DB }

func NewEpidOverrideRepository(db *gorm.DB) *EpidOverrideRepo { return &EpidOverrideRepo{db: db} }

// setStatusAllowed SetStatus 允许的目标状态（方案 §4 状态机的**人工侧**迁移）。
// pending 只由 UpsertPending 在 import 时写入；orphan 只由孤儿清扫写 —— 均不走本入口。
var setStatusAllowed = map[string]struct{}{
	entity.OverrideStatusActive:   {},
	entity.OverrideStatusStale:    {},
	entity.OverrideStatusObsolete: {},
	entity.OverrideStatusRevoked:  {},
}

// ListActive 整表取 status=active —— 供派生前**一次性预加载**（方案 §8.2 A 点"台账很小可预加载"）。
//
// 为什么可以整表：台账只放人工逐条确认过的结论，第②段 12 条量级，行数以人的核对带宽兜底；
// 与派生表（百万行、全表表达式分组实测 Errno 28 ⇒ 红线④必须按 match_key 分批）完全不是一个量级。
// 若未来台账涨到万级，本方法与 Stats 再改按 match_key 分批。
func (r *EpidOverrideRepo) ListActive(ctx context.Context) ([]entity.MovieEpidOverride, error) {
	var out []entity.MovieEpidOverride
	err := dbFrom(ctx, r.db).
		Where("status = ?", entity.OverrideStatusActive).
		Order("id ASC").
		Find(&out).Error
	return out, err
}

// ListAll 台账**整表**读（T4 GET /manage/epid/overrides 的列表数据面，2026-11-12）。
//
// 与 ListActive 同一量级论证：台账只放人工逐条确认过的结论（第②段 12 条量级），行数以
// 人的核对带宽兜底 ⇒ 整表读合规（红线④的分批要求针对 590 万行派生表的全表聚合，非本表）；
// status 过滤由调用方内存做（行数小）。若未来台账涨到万级，本方法与 ListActive/Stats 再改按 match_key 分批。
func (r *EpidOverrideRepo) ListAll(ctx context.Context) ([]entity.MovieEpidOverride, error) {
	var out []entity.MovieEpidOverride
	err := dbFrom(ctx, r.db).
		Order("id ASC").
		Find(&out).Error
	return out, err
}

// OverrideImportReject 单行拒收明细（宁缺勿错的"并说明"）。
type OverrideImportReject struct {
	SrcRowId int64  `json:"srcRowId"`
	EpIndex  int    `json:"epIndex"`
	Reason   string `json:"reason"`
}

// OverrideImportResult UpsertPending 的逐行结果。error 只反映 DB 层故障；
// 业务性拒收（键冲突 / key_epid 对不上 / 枚举非法）一律进 Rejected，调用方按行处理。
type OverrideImportResult struct {
	Inserted int                    // 实际入库行数（status=pending）
	Deduped  int                    // 批内同 (src_row_id,ep_index) 被去重的行数（幂等）
	Rejected []OverrideImportReject // 被拒收的行与原因
}

// overrideImportChunk 单条 INSERT 的行批（台账行不宽，200 行/句同采集侧口径）。
const overrideImportChunk = 200

// UpsertPending 批量导入 pending 裁定（T5 录入 12 条 / T4 管理端 import 的数据面）。
//
// 校验链（任一不过 ⇒ 拒收该行并在 Rejected 里说明原因，绝不带病入库）：
//  1. **批内去重**：同 (src_row_id, ep_index) 只保留首条，其余计入 Deduped（去重幂等）；
//  2. **静态校验**：verdict/provenance/confidence 必须是合法枚举；verdict=corrected 必须给
//     new_epid；key_raw_name / key_epid 必填（三元组校验的后两键，缺了以后没法防错位）；
//  3. **唯一键冲突拒收**：台账已有同 (src_row_id, ep_index) 条目 ⇒ 拒收（无论内容异同、什么状态；
//     DB 层 uk_row_ep 也会兜底拦）。重复 import 同一条同样被拒 —— 幂等的边界在批内去重，
//     跨批冲突一律显式处理：要换裁定先对既有条 SetStatus(revoked) 再 import；
//  4. **key_epid 校验（宁缺勿错的核心）**：用派生表 movie_episode_epid 按 (src_row_id, ep_index)
//     **点查**当前规则 epid，与 key_epid 逐字符相等才收；不等 ⇒ 拒收并给出两边的值 ——
//     说明源行集序已漂移/已被重算，候选带着过期 key_epid 套用必然错位。派生表无此行同样拒收。
//     点查同时带回 match_key/mid，受控冗余由 repo 以派生表现值**回填**（覆盖调用方传值），
//     杜绝台账与派生表两处口径漂移。
//
// 通过校验的行在**一个事务**内插入：status 强制 pending、created_at 补当前毫秒、id 交自增。
func (r *EpidOverrideRepo) UpsertPending(ctx context.Context, rows []entity.MovieEpidOverride) (OverrideImportResult, error) {
	var res OverrideImportResult
	if len(rows) == 0 {
		return res, nil
	}

	// ① 批内去重（同键保留首条）
	type rowKey struct {
		src int64
		ep  int
	}
	seen := make(map[rowKey]struct{}, len(rows))
	deduped := make([]entity.MovieEpidOverride, 0, len(rows))
	for i := range rows {
		k := rowKey{rows[i].SrcRowId, rows[i].EpIndex}
		if _, dup := seen[k]; dup {
			res.Deduped++
			continue
		}
		seen[k] = struct{}{}
		deduped = append(deduped, rows[i])
	}

	// ② 静态校验
	static := make([]entity.MovieEpidOverride, 0, len(deduped))
	for i := range deduped {
		row := deduped[i]
		if reason := validateOverrideRow(row); reason != "" {
			res.Rejected = append(res.Rejected, OverrideImportReject{row.SrcRowId, row.EpIndex, reason})
			continue
		}
		static = append(static, row)
	}
	if len(static) == 0 {
		return res, nil
	}

	// ③ 唯一键冲突预检：一次按 src_row_id 收窄（走 uk_row_ep 前缀），(src_row_id, ep_index) 内存精确对
	srcIds := distinctOverrideSrcIds(static)
	var existing []entity.MovieEpidOverride
	if err := dbFrom(ctx, r.db).Model(&entity.MovieEpidOverride{}).
		Select("id, src_row_id, ep_index, status").
		Where("src_row_id IN ?", srcIds).
		Find(&existing).Error; err != nil {
		return res, err
	}
	occupied := make(map[rowKey]entity.MovieEpidOverride, len(existing))
	for _, e := range existing {
		occupied[rowKey{e.SrcRowId, e.EpIndex}] = e
	}
	unconflicted := make([]entity.MovieEpidOverride, 0, len(static))
	for i := range static {
		row := static[i]
		if prev, hit := occupied[rowKey{row.SrcRowId, row.EpIndex}]; hit {
			res.Rejected = append(res.Rejected, OverrideImportReject{row.SrcRowId, row.EpIndex,
				fmt.Sprintf("唯一键 (src_row_id=%d,ep_index=%d) 已存在裁定 id=%d status=%s —— 重复 import/键冲突一律拒收；要变更请先对既有条 SetStatus(revoked)",
					row.SrcRowId, row.EpIndex, prev.Id, prev.Status)})
			continue
		}
		unconflicted = append(unconflicted, row)
	}
	if len(unconflicted) == 0 {
		return res, nil
	}

	// ④ key_epid 校验：逐行按 (src_row_id, ep_index) 点查派生表当前规则 epid。
	// 低频人工导入（每批 ≤ 几十行）+ 唯一键点查 O(1) ⇒ 逐行点查不构成 N+1 风险
	// （性能铁律 3 约束的是派生/出参热路径，不含本入口）。
	valid := make([]entity.MovieEpidOverride, 0, len(unconflicted))
	for i := range unconflicted {
		row := unconflicted[i]
		var cur struct {
			Epid     string
			MatchKey string
			Mid      *int64 // 派生表 mid 可空（未关联片）
		}
		err := dbFrom(ctx, r.db).Model(&entity.MovieEpisodeEpid{}).
			Select("epid, match_key, mid").
			Where("src_row_id = ? AND ep_index = ?", row.SrcRowId, row.EpIndex).
			Take(&cur).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res.Rejected = append(res.Rejected, OverrideImportReject{row.SrcRowId, row.EpIndex,
				fmt.Sprintf("派生表无 (src_row_id=%d,ep_index=%d) 的行 —— key_epid 无法校验，宁缺勿错", row.SrcRowId, row.EpIndex)})
			continue
		}
		if err != nil {
			return res, err
		}
		if cur.Epid != row.KeyEpid {
			res.Rejected = append(res.Rejected, OverrideImportReject{row.SrcRowId, row.EpIndex,
				fmt.Sprintf("key_epid 与当前规则 epid 不一致（key_epid=%q, 当前=%q）—— 源行已漂移/被重算，拒收宁缺勿错", row.KeyEpid, cur.Epid)})
			continue
		}
		// 受控冗余以派生表现值回填（覆盖调用方传值，杜绝两处口径漂移）
		row.MatchKey = cur.MatchKey
		row.Mid = 0
		if cur.Mid != nil {
			row.Mid = *cur.Mid
		}
		valid = append(valid, row)
	}
	if len(valid) == 0 {
		return res, nil
	}

	// ⑤ 入库：单事务；status 强制 pending、created_at 补当前毫秒、id 交自增。
	now := nowMilli()
	for i := range valid {
		valid[i].Id = 0
		valid[i].Status = entity.OverrideStatusPending
		valid[i].CreatedAt = now
	}
	err := dbFrom(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		return tx.CreateInBatches(valid, overrideImportChunk).Error
	})
	if err != nil {
		return res, err
	}
	res.Inserted = len(valid)
	return res, nil
}

// validateOverrideRow import 静态校验：返回 "" 表示通过，否则返回给调用方可读的拒收原因。
func validateOverrideRow(row entity.MovieEpidOverride) string {
	switch {
	case row.SrcRowId <= 0:
		return "src_row_id 必填（>0）"
	case row.KeyRawName == "":
		return "key_raw_name 必填 —— 三元组校验键，防 ep_index 因重排/插集漂移后错位"
	case row.KeyEpid == "":
		return "key_epid 必填 —— 三元组校验第二键（裁定时的规则 epid）"
	case !entity.ValidOverrideVerdict(row.Verdict):
		return fmt.Sprintf("verdict 非法 %q（只收 corrected|kept）", row.Verdict)
	case row.Verdict == entity.OverrideVerdictCorrected && row.NewEpid == "":
		return "verdict=corrected 必须给 new_epid（目标 epid）"
	case !entity.ValidOverrideProvenance(row.Provenance):
		return fmt.Sprintf("provenance 非法 %q（只收 rule|external|agent|human）", row.Provenance)
	case !entity.ValidOverrideConfidence(row.Confidence):
		return fmt.Sprintf("confidence 非法 %q（只收 inferred|verified）", row.Confidence)
	}
	return ""
}

// SetStatus 批量改状态 —— 支持 active/stale/obsolete/revoked（方案 §4 状态机的人工侧迁移）。
//
//   - 状态合法性在此把关（非法值直接报错，不发 SQL）；pending/orphan 不在本入口：
//     pending 由 import 写入，orphan 由孤儿清扫写；
//   - 迁移合理性（如 revoked 能否复活为 active）留给管理端调用方把关，repo 不拦；
//   - 置 active 即"人工确认"，同步补 verified_at（审计列口径：人工确认时间）。
//
// 返回实际改到的行数（0 行 = ids 不存在，调用方自行判定）。
func (r *EpidOverrideRepo) SetStatus(ctx context.Context, ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if _, ok := setStatusAllowed[status]; !ok {
		return 0, fmt.Errorf("epid override: SetStatus 不支持状态 %q（只收 active|stale|obsolete|revoked；pending 由 import 写入，orphan 由孤儿清扫写）", status)
	}
	updates := map[string]any{"status": status}
	if status == entity.OverrideStatusActive {
		updates["verified_at"] = nowMilli()
	}
	res := dbFrom(ctx, r.db).Model(&entity.MovieEpidOverride{}).
		Where("id IN ?", ids).
		Updates(updates)
	return res.RowsAffected, res.Error
}

// OverrideStat 单分组计数行。
type OverrideStat struct {
	Name string `json:"name"`
	N    int64  `json:"n"`
}

// OverrideStats 按 verdict / provenance / status 三维分组计数（T11 管理端指标的数据源）。
//
// ⚠️ 红线④"全表聚合一律按 match_key 分批"针对的是**派生表**（590 万行、~3.5GB 内存机器上
// 全表表达式分组实测 Errno 28）。台账只放人工逐条确认过的结论（第②段 12 条量级），行数有
// 人的核对带宽兜底，整表按单列 GROUP BY 无内存风险 —— 若未来台账涨到万级，再按 match_key 分批。
type OverrideStats struct {
	ByVerdict    []OverrideStat `json:"byVerdict"`
	ByProvenance []OverrideStat `json:"byProvenance"`
	ByStatus     []OverrideStat `json:"byStatus"`
}

// Stats 三维分组计数（三次单列 GROUP BY，列名来自本文件内的固定白名单，无注入面）。
func (r *EpidOverrideRepo) Stats(ctx context.Context) (OverrideStats, error) {
	var out OverrideStats
	var err error
	if out.ByVerdict, err = r.statGroupBy(ctx, "verdict"); err != nil {
		return out, err
	}
	if out.ByProvenance, err = r.statGroupBy(ctx, "provenance"); err != nil {
		return out, err
	}
	out.ByStatus, err = r.statGroupBy(ctx, "status")
	return out, err
}

func (r *EpidOverrideRepo) statGroupBy(ctx context.Context, col string) ([]OverrideStat, error) {
	var out []OverrideStat
	err := dbFrom(ctx, r.db).Model(&entity.MovieEpidOverride{}).
		Select(col + " AS name, COUNT(*) AS n").
		Group(col).
		Order("n DESC").
		Scan(&out).Error
	return out, err
}

// distinctOverrideSrcIds 行集内去重后的 src_row_id（预检查询按它收窄，走 uk_row_ep 前缀）。
func distinctOverrideSrcIds(rows []entity.MovieEpidOverride) []int64 {
	seen := make(map[int64]struct{}, len(rows))
	out := make([]int64, 0, len(rows))
	for i := range rows {
		if _, ok := seen[rows[i].SrcRowId]; !ok {
			seen[rows[i].SrcRowId] = struct{}{}
			out = append(out, rows[i].SrcRowId)
		}
	}
	return out
}
