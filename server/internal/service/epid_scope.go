package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"server/internal/danmu"
)

// ===== mode=scope：按 `from_src` 档定向重算（2026-11-14）=====
//
// 背景：改解析规则**不会让已派生行变脏**（派生只按 `ep_hash` 判脏，`ShouldReplaceDerived` 只是抖动守卫）
// ⇒ 光发版规则不作用于存量；而全量 `rebuild` 要 3h37m。本批三条规则（R11/R12/R13）的**输入档都是 `skip`**
// ⇒ 按档圈即可（`skip` 档 190,125 源行 = 147,099 主份 + 43,026 副本，24% of 603,427；圈档**不滤** dup_copy，
// 见交接文档口径纪律 3）。
//
// 安全边界（缺一不可，见 Harness `审查-epid服务端规则与批量补采-2026-11-14.md` §3.1.1「判断流程」）：
//  1. 范围**只能从登记表推导**（`rule=` ⇒ `Scope`/`InFromSrc`/`FansOutMovie`）；`Scope=all` 直接拒绝 ✗；
//  2. **池扇出**（`FansOutMovie`）必须扩到同片的**池相关档**（`bridge`/`anchor`/`none`），
//     否则同片兄弟的 `n` 漏改 ✗；
//  3. **宁宽勿漏**：`from_src` 是**安全上界**，`Tighten` 只是省时间、不是正确性前提；
//  4. 跳过判脏与抖动守卫（`force`），但**台账必须 fail-closed**（否则把人工校正冲掉 ✗）。

// ErrDeriveBusy `RunScopedDerive` 的"锁被占"哨兵错误 —— 调用方据此回 **409**（与 `derive-all` 同款语义），
// 其它错误一律是入参非法（400）。
var ErrDeriveBusy = errors.New("已有派生任务在跑（含采集收尾的自动派生）")

// DeriveModeScope 定向重算（`mode=scope`）——只重算"范围里的源行"，**不动派生表其它行**。
const DeriveModeScope DeriveMode = "scope"

// ScopeRequest 定向重算的范围请求（二选一：给 `Rule`，或给 `FromSrc`）。
type ScopeRequest struct {
	Rule         string   // 规则编号（`D1`~`D8`，见 `danmu.DecisionRuleMetas()`）⇒ 从登记表推导下面三项
	FromSrc      []string // 显式档（`Rule` 为空时必填）
	ExpandMovies bool     // 池扇出（`Rule` 非空时由登记的 `FansOutMovie` 覆盖）
	Tighten      bool     // 用 `danmu.RuleSelectsRow` 过选行谓词（只对 `FansOutMovie=false` 的规则精确）
}

// Resolve 把请求解成「要重算哪些档 + 要不要扩片 + 要不要收紧」。**纯函数**（可单测，不碰 DB）。
func (r ScopeRequest) Resolve() (fromSrc []string, expandMovies, tighten bool, err error) {
	if strings.TrimSpace(r.Rule) != "" {
		id := danmu.RuleID(strings.TrimSpace(r.Rule))
		m := danmu.RuleMetaOf(id)
		if m.ID == "" {
			return nil, false, false, fmt.Errorf("未知规则编号 %q（合法值见 danmu.DecisionRuleMetas()）", r.Rule)
		}
		if m.Scope != danmu.ScopeFromSrc {
			return nil, false, false, fmt.Errorf(
				"规则 %s 改的是**所有行**（Scope=all）⇒ 没有更小的安全范围，只能用 mode=rebuild 全量", r.Rule)
		}
		// 池依赖规则（FansOutMovie）⇒ 不收紧：其判定需要"本片所有源"的池，`RuleSelectsRow` 不适用。
		return m.InFromSrc, m.FansOutMovie, !m.FansOutMovie, nil
	}
	if len(r.FromSrc) == 0 {
		return nil, false, false, fmt.Errorf("必须给 rule 或 fromSrc")
	}
	known := map[string]bool{}
	for _, m := range danmu.DecisionRuleMetas() {
		known[m.OutFromSrc] = true
	}
	for _, s := range r.FromSrc {
		if !known[s] {
			return nil, false, false, fmt.Errorf("未知 from_src 档 %q（合法值：self|single|skip|none|bridge|anchor）", s)
		}
	}
	// tighten 的选行谓词来自规则本体（danmu.RuleSelectsRow）—— 显式档没带 rule ⇒ 空谓词对任何行都
	// 不命中，任务会 200 启动后**静默空转**（kept=0）。显式拒绝优于静默吞掉；只要档级上界请去掉
	// tighten（宁宽勿漏，tighten 只是省时间、不是正确性前提）。
	if r.Tighten && strings.TrimSpace(r.Rule) == "" {
		return nil, false, false, fmt.Errorf(
			"fromSrc 显式档 + tighten 必须同时给 rule（选行谓词来自规则本体）；只要档级上界请去掉 tighten")
	}
	return r.FromSrc, r.ExpandMovies, r.Tighten, nil
}

// ScopeLabel 人读的范围标签（写进 `deriveFullMode`，进度卡片可见）。
func (r ScopeRequest) ScopeLabel() string {
	if strings.TrimSpace(r.Rule) != "" {
		return "scope:" + strings.TrimSpace(r.Rule)
	}
	return "scope:" + strings.Join(r.FromSrc, ",")
}

func dedupeSortedInt64(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// RunScopedDerive 启动一次**定向重算**（**异步**；与全量派生 / 采集收尾的自动派生共用 `deriveRunning` ⇒ 互斥）。
//
// 返回 (范围标签, nil) 表示已启动；( "", err) 表示入参非法 / 已有任务在跑 / 未装配。
// 进度看 `GET /manage/epid/report`（`mode` 会显示 `scope:<rule|档>`）与后端日志（每批一行 `[epid]`）。
func RunScopedDerive(req ScopeRequest, batch int, timeout time.Duration) (string, error) {
	fromSrc, expandMovies, tighten, err := req.Resolve()
	if err != nil {
		return "", err
	}
	s := currentEpidService()
	if s == nil {
		return "", fmt.Errorf("epid 派生服务未装配")
	}
	if !deriveRunning.CompareAndSwap(false, true) {
		return "", ErrDeriveBusy
	}
	if batch <= 0 || batch > 5000 {
		batch = 500
	}
	if timeout <= 0 {
		timeout = time.Hour
	}
	label := req.ScopeLabel()
	deriveFullMode.Store(label)
	deriveFullStartedAt.Store(time.Now().UnixMilli())
	deriveFullRunning.Store(true)

	go func() {
		defer func() {
			deriveFullRunning.Store(false)
			deriveFullStartedAt.Store(0)
			deriveRunning.Store(false)
			// 与全量派生同款：**必须 recover**（一条脏数据不能把全站服务打挂）
			if r := recover(); r != nil {
				log.Printf("[epid] 定向重算 panic 已拦截（不影响线上服务）: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		ResetDeriveRunCounters()
		WarmProgressSnapshots()

		// ---- 第 1 步：档级范围（安全上界）----
		ids, err := s.repo.SourceRowIdsByFromSrc(ctx, fromSrc)
		if err != nil {
			log.Printf("[epid] 定向重算：圈档失败（%s）: %v", label, err)
			return
		}
		// ---- 第 2 步：池扇出（FansOutMovie）—— ∪ 同片的池相关档 ----
		if expandMovies {
			pool := danmu.PoolRelevantFromSrc()
			extra, eerr := s.repo.SourceRowIdsInMoviesByFromSrc(ctx, fromSrc, pool)
			if eerr != nil {
				log.Printf("[epid] 定向重算：池扇出失败（%s）: %v", label, eerr)
				return
			}
			log.Printf("[epid] 定向重算：池扇出 ∪ %d 个池相关源行（档=%v，池档=%v）", len(extra), fromSrc, pool)
			ids = append(ids, extra...)
		}
		ids = dedupeSortedInt64(ids)
		// ---- 第 3 步：选行谓词（只省时间；只对非池依赖规则精确）----
		if tighten && len(ids) > 0 {
			titles, terr := s.repo.SourceRowTitlesByFromSrc(ctx, fromSrc)
			if terr != nil {
				log.Printf("[epid] 定向重算：取集名失败，跳过收紧（宁宽勿漏）: %v", terr)
			} else {
				id := danmu.RuleID(strings.TrimSpace(req.Rule))
				kept := make([]int64, 0, len(ids))
				for _, sid := range ids {
					if danmu.RuleSelectsRow(id, titles[sid]) {
						kept = append(kept, sid)
					}
				}
				log.Printf("[epid] 定向重算：收紧 %d → %d 源行（谓词=%s）", len(ids), len(kept), id)
				ids = kept
			}
		}
		if len(ids) == 0 {
			log.Printf("[epid] 定向重算：范围为空（%s），无事可做", label)
			return
		}
		log.Printf("[epid] 定向重算开始：%s 命中源行=%d batch=%d timeout=%s", label, len(ids), batch, timeout)

		total := DeriveStats{SkipWhy: map[string]int{}}
		batches := 0
		for i := 0; i < len(ids); i += batch {
			// 与全量派生同款：每批前让路采集（最多 3 分钟）—— 两边都在写同一张库，重叠会推高死锁概率。
			waitCollectIdle(ctx, 3*time.Minute)
			if ctx.Err() != nil {
				log.Printf("[epid] 定向重算到时限中止（已处理 %d/%d 源行）", i, len(ids))
				break
			}
			end := i + batch
			if end > len(ids) {
				end = len(ids)
			}
			rows, rerr := s.repo.SourceRowsByIds(ctx, ids[i:end])
			if rerr != nil {
				log.Printf("[epid] 定向重算中断（取源行失败，已处理 %d 批）: %v", batches, rerr)
				break
			}
			// 台账 **fail-closed（含 D3 开关关闭态）**：scope 整源行重写，若不套台账会把已生效校正
			// 冲掉（2026-11-14 mode=fill 实际冲掉 34 条）。开关只管「新校正的应用」（A 点），不该管
			// 「已生效校正的保持」—— 这里**绕过开关**强制装载（只读 active 重套用，安全方向）。
			var ledger *overrideLedger
			if s.ov != nil {
				if !EpidOverrideEnabled() {
					log.Printf("[epid] 定向重算：生效开关（D3）当前关闭 —— 本路径整源行重写，仍强制套用 active 台账以保持已生效校正（重算后校正值原样回归）")
				}
				l, lerr := s.loadOverrideLedgerForRewrite(ctx)
				if lerr != nil {
					log.Printf("[epid] 定向重算中止（台账预加载失败，防已生效校正被整源行冲掉）: %v", lerr)
					break
				}
				ledger = l
			}
			batchSt := DeriveStats{SkipWhy: map[string]int{}}
			// force=true：跳过"内容未变"判脏 **且** 跳过抖动守卫 —— 改规则场景下这些行本来就"没变"，
			// 守卫会把新规则拦下；与 C 点（RecomputeForOverrides）同款取舍。
			staleIds, derr := s.deriveRows(ctx, rows, true, &batchSt, ledger)
			if derr != nil {
				log.Printf("[epid] 定向重算中断（第 %d 批派生失败）: %v", batches+1, derr)
				break
			}
			batchSt.OverrideStale = len(staleIds)
			if len(staleIds) > 0 {
				s.markOverridesStale(ctx, staleIds)
			}
			batches++
			total.Scanned += batchSt.Scanned
			total.Rebuilt += batchSt.Rebuilt
			total.Rows += batchSt.Rows
			total.Skipped += batchSt.Skipped
			total.EmptyEpid += batchSt.EmptyEpid
			total.OverrideApplied += batchSt.OverrideApplied
			total.OverrideKept += batchSt.OverrideKept
			total.OverrideStale += batchSt.OverrideStale
			// 内存计数（供卡片实时读数；**不做任何全表计数**）
			deriveFullScanned.Add(int64(batchSt.Scanned))
			deriveFullRebuilt.Add(int64(batchSt.Rebuilt))
			deriveFullRows.Add(batchSt.Rows)
			deriveFullBatches.Add(1)
			for k, v := range batchSt.SkipWhy {
				total.SkipWhy[k] += v
			}
			log.Printf("[epid] 定向重算 第%d批: 源行 %d→%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d 校正(套用/登记/标stale)=%d/%d/%d",
				batches, i, end, batchSt.Rebuilt, batchSt.Rows, batchSt.Skipped, batchSt.EmptyEpid,
				batchSt.OverrideApplied, batchSt.OverrideKept, batchSt.OverrideStale)
		}
		log.Printf("[epid] 定向重算结束（%s 命中=%d 批=%d）: 扫=%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d 原因=%v 校正(套用/登记/标stale)=%d/%d/%d",
			label, len(ids), batches, total.Scanned, total.Rebuilt, total.Rows, total.Skipped, total.EmptyEpid,
			total.SkipWhy, total.OverrideApplied, total.OverrideKept, total.OverrideStale)

		// 收尾：刷新"影片完整度"快照（进度主口径；与全量派生同款）。**不做 ANALYZE** —— 定向重算不动全表，
		// 统计信息不会因此失真。
		{
			mctx, mcancel := context.WithTimeout(context.Background(), 3*time.Minute)
			if mc, ok := RefreshMovieCompleteness(mctx); ok {
				log.Printf("[epid] 影片完整度: %d/%d 完整，还缺 %d 部影片 / %d 个源行",
					mc.CompleteMovies, mc.Movies, mc.Movies-mc.CompleteMovies, mc.MissingSrcRows)
			}
			mcancel()
		}
		KickHeavyMetricsRefresh()
		WarmProgressSnapshots()
	}()
	return label, nil
}
