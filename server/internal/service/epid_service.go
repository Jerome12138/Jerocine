package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"server/internal/danmu"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/repository/mysql"
)

// EpidService 集身份派生（P1-1, 2026-11-10）。
//
// 职责：把 `movie_play_source.episodes_json`（**唯一真相源，永不改**）**单向派生**成
// `movie_episode_epid`（一集一行）。口径见 docs/方案-集身份epid落地路线与弹幕准确性-2026-11-09.md（v5.0 §5.4）。
//
// 设计要点（均为踩过的坑）：
//   - **整源行重算**（DELETE+INSERT 同事务），**绝不按 `ep_index` 打补丁**（集序会漂）
//   - **幂等是特性**：只换 link/清晰度 ⇒ 重建出的 epid 一模一样 ⇒ 下游零感知
//   - **抖动守卫**（F-07 + 60% 阈值）：疑似抖动**不重建**，等下一轮；`force=true` 可人工强制
//   - **孤儿清理**（F-06）：源行整行没了 ⇒ 级联删派生行
type EpidService struct {
	repo *mysql.EpidRepo
	// ov 裁定台账 repo（epid 源头校正层 T3, 2026-11-12，迁移 000049）。**可选装配**
	//（SetEpidOverrideRepo）：未装配 ⇒ A 点跳过、B/C 报错 —— 行为与校正层上线前完全一致（安全降级）。
	ov *mysql.EpidOverrideRepo
}

func NewEpidService(repo *mysql.EpidRepo) *EpidService { return &EpidService{repo: repo} }

// SetEpidOverrideRepo 装配裁定台账 repo（main 启动时一行；构造签名不变，避免扩大既有调用面的回归）。
func (s *EpidService) SetEpidOverrideRepo(r *mysql.EpidOverrideRepo) { s.ov = r }

// ---- U1：触发接线（2026-11-10）----
//
// 取向：**采集任务收尾后 best-effort 异步触发**（方案 §5.4 的推荐路径"采集批次后异步批量"）。
// 为什么用"包级默认实例"而不是给 SpiderService 加依赖：SpiderService 的构造签名被多处测试使用，
// 加参数会扩大回归面；包级装配 + 显式 Set 只需在 main 里一行，**且未装配时是 no-op**（安全降级）。
var (
	defaultEpidMu sync.RWMutex
	defaultEpid   *EpidService
	deriveRunning atomic.Bool
)

// SetDefaultEpidService 装配默认派生服务（应用启动时调用一次）。未装配 ⇒ 后台触发为 no-op。
func SetDefaultEpidService(s *EpidService) {
	defaultEpidMu.Lock()
	defer defaultEpidMu.Unlock()
	defaultEpid = s
}

func currentEpidService() *EpidService {
	defaultEpidMu.RLock()
	defer defaultEpidMu.RUnlock()
	return defaultEpid
}

// 派生游标（U1-1, 2026-11-11 评审）：采集只会打脏"**本次采到的行**"，而原实现每轮跑 `DeriveAll`
// （全表 keyset 扫）⇒ 成本与收益严重不匹配 ✗。现改为**每轮只跑一批**（`DeriveBatch`，定量），
// 游标在批间前进、扫到末尾自动回 0（下一轮从头复核被守卫拦下的行）。
var deriveCursor atomic.Int64

// 孤儿清理节流（补审 U1-2）：孤儿只在"整片下架 / 站点下线 / Truncate"后产生 ⇒ **低频**任务；
// 挂在采集收尾却每次跑属**频率错配** ✗ ⇒ 每 24h 最多一次。
// deriveHotPhase 热门优先阶段（H6, 2026-11-11 评审）：true = **热门趟**（只取 `mid` 非空的源行），
// false = **长尾趟**（全表）。§5.5 铁律 5 要求"限速 + 热门优先"——纯 `ORDER BY` 在 keyset 分页下
// **不改变候选集合**（见 repo `DirtySourceRowsHot` 的说明）⇒ 必须拆两趟。**内存态**：重启后从热门趟
// 重来（热门趟更快、也更该先跑）✓ 零新增持久状态。
var deriveHotPhase atomic.Bool

// epidOverrideEnabled epid 源头校正**生效开关**（D3 · T4, 2026-11-12）：**默认关**。
//
// 关着时（T4 管理端入口的把守语义，待办-epid源头校正层实施-2026-11-12 T4）：
//   - **A 点替换不生效**：loadOverrideLedger 直接返回 nil ⇒ DeriveBatch 照常派生、只是不套用裁定；
//   - **B/C 显式报错**：ApplyOverrides / RecomputeForOverrides 在入口处拒绝（apply 接口 → 403）；
//   - **import 与 GET 不受影响**：录入/查看先行（先录入核对、后手动打开）。
//
// 实现对齐仓内现成的两种"配置/开关"模式里的**运行时开关**一支（deriveHotPhase/deriveRunning 同款
// atomic.Bool）：持久式（site_config 加列，仿 DanmuEnabled）需要新迁移，第②段迁移只有 000049，
// 不在 T4 范围 ⇒ 取内存态。重启回默认关 = fail-safe（已落库的校正值不动；A 停止套用，不会写错值）。
// 打开入口：PUT /manage/epid/overrides/enabled（handler 层）。
var epidOverrideEnabled atomic.Bool

// EpidOverrideEnabled 生效开关当前状态（GET /manage/epid/overrides 回显 + apply/recompute 把守）。
func EpidOverrideEnabled() bool { return epidOverrideEnabled.Load() }

// SetEpidOverrideEnabled 打开/关闭生效开关（管理端 PUT /manage/epid/overrides/enabled 调用）。
func SetEpidOverrideEnabled(on bool) { epidOverrideEnabled.Store(on) }

// ---- U5 / H2：一键全量派生（管理端入口, 2026-11-11）----
//
// 与"采集收尾自动触发一批"的关系：**共用同一把锁**（`deriveRunning`）⇒ 互斥防重入；
// 本全量任务由管理端**显式**发起（补缺 / 清空重建），跑完自己打验收指标日志。
var (
	deriveFullRunning   atomic.Bool  // 是否有全量任务在跑（供状态查询）
	deriveFullStartedAt atomic.Int64 // 全量任务的起始毫秒
	deriveFullMode      atomic.Value // fill | rebuild
)

// 采集轮次进行中的标记（2026-11-11，需求 3"与采集错峰/让路"）。
//
// 为什么需要：全量派生与采集**都在写同一张库**；两者重叠会推高死锁概率（项目里已记录过
// "主站并发过高会批量死锁"）。原先只与"派生自身的自动触发"互斥，**不与采集互斥** ⇒ 全量重建
// 期间必然撞上 1~3 轮采集。现由采集链路（cron 轮次 起/止）置位，全量派生在**每批前**让路。
//
// ⚠️ 2026-11-11 审查（N2）：原来是 `atomic.Bool`，且**只挂在 cron 闭包** ⇒ 手动采集
// （`StartCollect` / `CollectFilm` / `AutoCollect` / `BatchCollect` / 恢复补采）期间不让路 ✗。
// 现改为**可重入计数**（`atomic.Int32`）：cron 与各手动入口都进出一次，嵌套/并发也都算"在采集" ✓
var collectRoundBusy atomic.Int32

// SetCollectRoundRunning 由采集链路在"轮次开始/结束"时调用（cron 闭包）。
// 语义 = 计数 +1/-1，可重入（见上方说明）。
func SetCollectRoundRunning(on bool) {
	if on {
		collectRoundBusy.Add(1)
		return
	}
	collectRoundBusy.Add(-1)
}

// CollectEnter / CollectLeave 手动采集入口用的显式进出（与 SetCollectRoundRunning 同一计数）。
func CollectEnter() { collectRoundBusy.Add(1) }
func CollectLeave() { collectRoundBusy.Add(-1) }

// CollectRoundRunning 当前是否有采集在跑（含手动入口）。
func CollectRoundRunning() bool { return collectRoundBusy.Load() > 0 }

// waitCollectIdle 若采集轮次在跑则等待，最多等 `max`；返回是否等到了空闲。
//
// 为什么要设上限：采集一轮可能跑几分钟~十几分钟，无限等会让全量任务被**饿死**；
// 等满就放行（并在日志里说明），把"完全串行"换成"尽量错峰"。
func waitCollectIdle(ctx context.Context, max time.Duration) bool {
	if !CollectRoundRunning() {
		return true
	}
	log.Printf("[epid] 采集轮次进行中 ⇒ 本批让路，最多等 %s", max)
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
		if !CollectRoundRunning() {
			log.Printf("[epid] 采集轮次已结束 ⇒ 继续派生")
			return true
		}
	}
	log.Printf("[epid] 等待采集结束超时（%s）⇒ 为免饿死，本批继续执行", max)
	return false
}

var lastOrphanCleanMs atomic.Int64

const (
	deriveBatchPerTrigger = 2000             // 每次触发处理的行数上限
	deriveTriggerTimeout  = 10 * time.Minute // U1-3：只跑一批 ⇒ 不需要 30 分钟
	orphanCleanIntervalMs = 24 * 60 * 60 * 1000
)

// DeriveEpidInBackground 采集批次结束后**非阻塞**触发一轮派生（带可重入保护 + 超时 + 游标）。
//
// 铁律：**失败只记日志，绝不影响采集**；已在跑 / 未装配 ⇒ 直接返回（并记一条跳过日志，便于发现"长期跑不完"）。
func DeriveEpidInBackground() {
	s := currentEpidService()
	if s == nil {
		return
	}
	if !deriveRunning.CompareAndSwap(false, true) {
		log.Printf("[epid] 上一轮派生尚未结束 ⇒ 本轮跳过（U1-6：若长期如此说明单轮远超采集间隔）")
		return
	}
	go func() {
		defer deriveRunning.Store(false)
		// R1（2026-11-11 评审，High）：**必须 recover** —— 本 goroutine 挂在**采集任务收尾**，
		// 而 Go 的未捕获 panic **会终止整个进程**（不局限于该 goroutine）⇒ 一条脏数据（JSON 异常/
		// SQL 扫描异常/下标越界）就能把**全站服务**打挂 ✗。派生是 best-effort，必须"只记账、不致命"。
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[epid] 派生 panic 已拦截（不影响采集与线上服务）: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), deriveTriggerTimeout)
		defer cancel()
		after := deriveCursor.Load()
		runHot := deriveHotPhase.Load() // H6：本趟是"热门趟"（只扫 mid 非空）还是"长尾趟"（全表）
		st, err := s.DeriveBatch(ctx, after, deriveBatchPerTrigger, false)
		if err != nil {
			log.Printf("[epid] 派生失败（不影响采集）: %v", err)
			return
		}
		finishedPass := st.Scanned < deriveBatchPerTrigger
		if finishedPass {
			// H6：本趟扫到表尾 ⇒ 切趟。热门趟结束 ⇒ 进长尾趟（从 0 开始）；长尾趟结束 ⇒ 回热门趟
			// （热门行的集内容变动最频繁，优先复核）。
			deriveHotPhase.Store(!runHot)
			deriveCursor.Store(0)
		} else {
			deriveCursor.Store(st.LastId)
		}
		phaseName := "长尾趟"
		if runHot {
			phaseName = "热门趟"
		}
		// U1-5：**批次末汇总一行**（原实现逐行打日志 ⇒ 日志量 = 采集频率 × 被拦行数 ✗）
		log.Printf("[epid] 派生一批完成: 趟=%s 游标 %d→%d 扫=%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d 来源=%v 校正(套用/登记/标stale)=%d/%d/%d",
			phaseName, after, deriveCursor.Load(), st.Scanned, st.Rebuilt, st.Rows, st.Skipped, st.EmptyEpid, st.SkipWhy,
			st.OverrideApplied, st.OverrideKept, st.OverrideStale)
		// H2（2026-11-11 评审，High）：**验收指标必须有人消费** —— `AlignmentReport` / `Coverage` 原来
		// **没有任何生产调用点** ⇒ §4.8 门槛与 §6.1 指标永远算不出来（P1-2 的启动条件也就无法判定）✗。
		// 挂到**每轮收尾**（零接口、零前端改动）：轻量指标每轮一行；代价高的"完整覆盖率"（要扫
		// `Σ JSON_LENGTH(episodes_json)` 巨列，与性能铁律无关但很慢）只在**整趟结束**时算一次。
		if rep, rerr := s.AlignmentReport(ctx); rerr == nil {
			log.Printf("[epid] 验收指标: 派生行=%d 空epid=%d 对齐组=%d/%d (%.1f%%) 来源=%v 门槛(≥85%%)=%v",
				rep.TotalDerivedRows, rep.EmptyEpidRows, rep.AlignedGroups, rep.TotalGroups,
				rep.AlignmentPct, rep.ByFromSrc, rep.AlignmentPct >= 85)
			if finishedPass && rep.TotalDerivedRows > 0 {
				if dr, se, cerr := s.repo.CoverageFull(ctx); cerr == nil && se > 0 {
					log.Printf("[epid] 完整覆盖率(§6.1 口径, 空epid也算行): %d/%d ⇒ %.2f%%（目标 ≈100%%）",
						dr, se, float64(dr)*100/float64(se))
				}
			}
		}
		// 孤儿清理：24h 节流（CAS 保证同一轮只有一个 goroutine 拿到执行权）
		nowMs := time.Now().UnixMilli()
		if last := lastOrphanCleanMs.Load(); nowMs-last >= orphanCleanIntervalMs &&
			lastOrphanCleanMs.CompareAndSwap(last, nowMs) {
			if n, err := s.CleanOrphans(ctx, 500); err == nil && n > 0 {
				log.Printf("[epid] 孤儿清理：%d 个源行（级联删派生行）", n)
			}
		}
	}()
}

// DeriveStats 一次派生批的统计（可直接打日志/落监控）。
//
// ⚠️ `Scanned` 的语义（2026-11-11 修正）：**本批"扫过"的源行数**（keyset 窗口大小），
// **不是**"脏行数"。调用方据此判"是否到表尾"（`Scanned < limit`）—— 用脏行数判会误判 ✗。
type DeriveStats struct {
	Scanned   int            `json:"scanned"`   // 本批**扫过**的源行数（== 窗口大小；< limit ⇒ 已到表尾）
	Rebuilt   int            `json:"rebuilt"`   // 实际重建的源行数
	Rows      int64          `json:"rows"`      // 写入的派生行数
	Skipped   int            `json:"skipped"`   // 被抖动守卫拦下的源行数
	SkipWhy   map[string]int `json:"skipWhy"`   // 原因分布（suspect-shrink / suspect-disjoint）
	EmptyEpid int64          `json:"emptyEpid"` // 派生行里 epid 为空串的条数（L6：不产身份）
	LastId    int64          `json:"lastId"`    // 本批最后一个源行 id（下次 keyset 从这里继续）

	// ---- epid 源头校正层 · A 点（T3, 2026-11-12）----
	OverrideApplied int `json:"overrideApplied"` // 本批套用 corrected 的派生行数（epid 被替换为 new_epid）
	OverrideKept    int `json:"overrideKept"`    // 本批登记"已裁定（kept）"的条数（值不变）
	OverrideStale   int `json:"overrideStale"`   // 本批三元组校验失败被标 stale 的台账条数（重回清单复核）
	// FilmNameStripped S-去前缀（2026-11-19）：s 以片名为前缀 ⇒ 去掉（含季号），计改动行数。
	FilmNameStripped int `json:"filmNameStripped"`
}

// derivFlushRows 一次 flush 的派生行上限（批级写入缓冲）：≈几 MB 内存，且把"一次事务"的规模
// 控制在"几十~几百个源行"这一档 —— 对照采集侧的写粒度（200 行/语句、整页一次调用）。
const derivFlushRows = 5000

// DeriveBatch 派生一批脏源行（keyset 分页；`afterId` 传上一批的 `LastId`，从头开始传 0）。
//
// epid 源头校正层 **A 点**（方案 §8.2，T3 2026-11-12）：派生循环外预加载 active 台账一次，
// 写库（ReplaceForSourceRows）前对 pendingRows 做**内存命中替换** —— 增量派生与全量重建
// （RunFullDerive 也走本函数）两条路径都自动套用校正，重建后校正自动回来（§5"清空重建"行）。
//
// 结束条件：返回的 `Scanned < limit` 表示这一轮已经扫完（但**不代表没有脏行**：被守卫拦下的仍脏）。
func (s *EpidService) DeriveBatch(ctx context.Context, afterId int64, limit int, force bool) (DeriveStats, error) {
	st := DeriveStats{SkipWhy: map[string]int{}}
	if limit <= 0 {
		limit = 200
	}
	// H6（2026-11-11 评审）：按阶段取候选 —— 热门趟只扫 `mid IS NOT NULL`（主站行/已关联行），
	// 长尾趟才扫全表。**不能**靠 `ORDER BY` 实现（keyset 分页下页内排序不改变候选集合）。
	//
	// ⚠️ **2026-11-11 实测 bug 修复**：repo 这里返回的是**过滤后的脏行** ⇒ 必须同时拿回
	// "本批扫过的源行数 `scanCount`"与"窗口末端 id `scanLast`"：**表尾按 `scanCount < limit` 判、游标按 `scanLast` 推进**。
	// 原先用"脏行数 < limit"判表尾 ⇒ 一段没有脏行的区间就被误判为表尾 ⇒ 游标归零、原地空转 ✗✗
	// （线上表现：重启后「补缺」每批 `扫=0` 秒结束，派生表不再增长）。
	var (
		rows      []entity.MoviePlaySource
		scanLast  int64
		scanCount int
		err       error
	)
	if deriveHotPhase.Load() {
		rows, scanLast, scanCount, err = s.repo.DirtySourceRowsHot(ctx, afterId, limit)
	} else {
		rows, scanLast, scanCount, err = s.repo.DirtySourceRows(ctx, afterId, limit)
	}
	if err != nil {
		return st, err
	}
	st.Scanned = scanCount
	st.LastId = scanLast
	if len(rows) == 0 {
		// 本批没有脏行：**不代表到表尾** —— 游标仍按窗口末端推进（调用方据 `Scanned < limit` 决定是否归零/切趟）。
		return st, nil
	}

	// ---- A 点预加载（方案 §8.2）：台账 ListActive **整表一次**，批内零重复查询 ----
	// 台账只放人工逐条确认过的结论（第②段 12 条量级小表）⇒ 整表读合规（红线④的分批要求针对
	// 590 万行派生表的**全表聚合**；本批对派生表的访问仍全部 uk_row_ep 键查）。
	// 失败 ⇒ **本批中止**（fail-closed）：deriveRows 会整源行重写派生行，套用层缺席会把已生效的
	// 校正冲掉（且要等该行再变脏才会重算回来）⇒ 宁可整批重试，也不带病落库。
	var ledger *overrideLedger
	if s.ov != nil {
		l, lerr := s.loadOverrideLedger(ctx)
		if lerr != nil {
			return st, fmt.Errorf("epid override: 台账预加载失败，本批中止（防已生效校正被重建冲掉）: %w", lerr)
		}
		ledger = l
	}
	// 未装配台账 repo ⇒ ledger 为 nil ⇒ A 点自动跳过，行为与校正层上线前一致（安全降级）。
	staleIds, err := s.deriveRows(ctx, rows, force, &st, ledger)
	if err != nil {
		return st, err
	}
	st.OverrideStale = len(staleIds)
	if len(staleIds) > 0 {
		s.markOverridesStale(ctx, staleIds)
	}
	return st, nil
}

// deriveRows 派生循环主体（DeriveBatch 与 C 点 RecomputeForOverrides **共用**，保证 A 点只有一处接线）：
// 锚/池预取 → 逐源行派生 → 攒 pendingIDs/pendingRows → flush（**写库前先做 A 点内存命中替换**）。
// `force=true` 跳过抖动守卫（C 点是管理端显式定向重算，守卫拦下会让校正落不了地）。
// 返回批内三元组校验失败、需标 stale 的台账 id（去重升序）。
func (s *EpidService) deriveRows(ctx context.Context, rows []entity.MoviePlaySource, force bool, st *DeriveStats, ledger *overrideLedger) ([]int64, error) {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Id)
	}
	anchors, err := s.repo.AnchorsBySrcRowIds(ctx, ids)
	// S-去前缀（2026-11-19）：C 口径——优先用本行 source_name；为空则按 mid 批量取影片名。
	midNames := map[int64]string{}
	{
		want := make([]int64, 0, len(rows))
		seenMid := map[int64]struct{}{}
		for _, r := range rows {
			if r.SourceName != nil && *r.SourceName != "" {
				continue
			}
			if r.Mid == nil || *r.Mid <= 0 {
				continue
			}
			if _, ok := seenMid[*r.Mid]; ok {
				continue
			}
			seenMid[*r.Mid] = struct{}{}
			want = append(want, *r.Mid)
		}
		const nameBatch = 200
		for i := 0; i < len(want); i += nameBatch {
			j := min(i+nameBatch, len(want))
			part, nerr := s.repo.MovieNamesByMids(ctx, want[i:j])
			if nerr != nil {
				return nil, nerr
			}
			for k, v := range part {
				midNames[k] = v
			}
		}
	}
	if err != nil {
		return nil, err
	}

	// H1（2026-11-11 评审；**同日更正**：上一批声称的"批内一次取"实际未落盘，生产上仍是逐行调用 ⇒
	// 单轮 ≤2000 次 `SourceTitlesByMatchKeys`，每次都拉"整片所有源"的 `episodes_json` 巨列
	// ⇒ 批处理跑满 10 分钟超时、该轮白跑 ✗，实测已复现）。
	// 现按 **200 个 match_key 一批**预取（≈10 次查询/批），并记录耗时便于持续观测。
	poolKeys := make([]string, 0, len(rows))
	seenKey := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		if r.MatchKey == "" {
			continue
		}
		if _, ok := seenKey[r.MatchKey]; ok {
			continue
		}
		seenKey[r.MatchKey] = struct{}{}
		poolKeys = append(poolKeys, r.MatchKey)
	}
	const poolKeyBatch = 200
	others := make(map[string][]string, len(poolKeys))
	for i := 0; i < len(poolKeys); i += poolKeyBatch {
		j := min(i+poolKeyBatch, len(poolKeys))
		t0 := time.Now()
		part, perr := s.repo.SourceTitlesByMatchKeys(ctx, poolKeys[i:j], nil)
		if perr != nil {
			return nil, perr
		}
		for k, v := range part {
			others[k] = append(others[k], v...)
		}
		if d := time.Since(t0); d > 500*time.Millisecond {
			log.Printf("[epid] 取借用池较慢: keys=%d 耗时=%s", j-i, d)
		}
	}

	now := time.Now().UnixMilli()
	// 批级写入缓冲（2026-11-11 优化，借采集侧 UpsertBatch 的批量思路）：
	// 原实现 **每源行一个事务**（DELETE + CreateInBatches）⇒ 单批 2000 事务，redo/binlog 与 4 个索引的
	// 维护被放大 2000 倍。现攒到 `derivFlushRows` 行或循环结束 flush 一次 ⇒ 事务数降到**个位数/批**。
	// `pendingIDs` 只放**本次真要重建**的源行（被守卫拦下的行不进来 —— 否则会被白删）。
	var (
		pendingIDs  []int64
		pendingRows []entity.MovieEpisodeEpid
		staleSeen   = map[int64]struct{}{}
	)
	flush := func() error {
		if len(pendingIDs) == 0 {
			return nil
		}
		// ---- A 点（方案 §8.2 "派生写库前"，必做/幂等保障）：对 pendingRows 做**内存命中替换**
		// —— 台账已在派生循环外预加载，这里零查询、纯内存命中，不额外取业务数据、不影响派生效率。
		applied, kept, stale := ledger.applyToPending(pendingRows)
		st.OverrideApplied += applied
		st.OverrideKept += kept
		for _, id := range stale {
			staleSeen[id] = struct{}{}
		}
		if err := s.repo.ReplaceForSourceRows(ctx, pendingIDs, pendingRows); err != nil {
			return err
		}
		pendingIDs = nil
		pendingRows = nil
		return nil
	}
	for _, src := range rows {
		st.LastId = src.Id
		titles := make([]string, 0, len(src.Episodes))
		for _, e := range src.Episodes {
			titles = append(titles, e.Episode)
		}
		// M2（2026-11-11 评审）：借用/锚点池必须是"**本片所有源**"的集名（web allEpisodeTitles 口径）——
		// 只放本源 ⇒ 桥接恒不可能命中 + 锚点在本源缺期时推错 ✗。
		// H1：他源部分已在循环外**按 200 key 一批**预取完（`others`），这里只做拼装；
		// 不传 exclude ⇒ 池 = 该片**全部**源行（含本批自身；重复集名对桥接/锚点无影响 —— 内部按 num/date 去重）。
		pool := append([]string{}, titles...)
		pool = append(pool, others[src.MatchKey]...)
		a := anchors[src.Id]
		if !force {
			hasOverlap := false
			if a.N > 0 {
				hasOverlap, err = s.repo.ExistsRawName(ctx, src.Id, titles)
				if err != nil {
					return nil, err
				}
			}
			ok, why := danmu.ShouldReplaceDerived(a.N, len(titles), hasOverlap, a.LastComputedAt, now)
			if !ok {
				st.Skipped++
				st.SkipWhy[why]++
				// U1-5（2026-11-11 评审）：**逐行打日志会让日志量 = 采集频率 × 被拦行数** ✗
				// ⇒ 改为只计数，批次末汇总一行（见 DeriveAll/DeriveEpidInBackground 的收尾日志）。
				continue
			}
		}

		// 派生（服务端口径：带 A2 期号，含锚点兜底；见 danmu.EpidFor）
		derived := danmu.EpidsForSourceRowWithPool(titles, pool)
		// S-去前缀（2026-11-19）：C 口径——source_name 优先，回退到 movie.name；
		// 仅当该行有 s 时才可能改动；去掉后若同源行内重复则不改（守卫在 danmu.StripFilmNamePrefix 内）。
		if fn := rowFilmName(src, midNames); fn != "" {
			if n := danmu.StripFilmNamePrefix(derived, fn); n > 0 {
				st.FilmNameStripped += n
			}
		}
		out := make([]entity.MovieEpisodeEpid, 0, len(titles))
		for i, t := range titles {
			if derived[i][0] == "" {
				st.EmptyEpid++
			}
			out = append(out, entity.MovieEpisodeEpid{
				SrcRowId:  src.Id,
				Mid:       src.Mid,
				MatchKey:  src.MatchKey,
				SiteId:    src.SiteId,
				PlayFrom:  src.PlayFrom,
				EpIndex:   i,
				Epid:      derived[i][0],
				FromSrc:   derived[i][1],
				RawName:   t,
				SrcEpHash: src.EpHash,
				// 双写副本标记**继承源行**（写侧落库 / 存量脚本补齐；派生**不做判定**）。
				DupCopy:    src.DupCopy,
				ComputedAt: now,
			})
		}
		pendingIDs = append(pendingIDs, src.Id)
		pendingRows = append(pendingRows, out...)
		st.Rebuilt++
		st.Rows += int64(len(out))
		if len(pendingRows) >= derivFlushRows {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(staleSeen) == 0 {
		return nil, nil
	}
	out := make([]int64, 0, len(staleSeen))
	for id := range staleSeen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// ===== epid 源头校正层 · T3：ApplyOverrides 统一入口 + 三处接线（2026-11-12）=====
//
// 口径：docs/epid-集身份/方案-epid源头校正层-2026-11-12.md §8 应用侧 / §5 变化矩阵。
// 台账（movie_epid_override，迁移 000049）只放"已判过"的结论；派生大表**零新增列**（红线①）——
// 校正不走行内状态（§8.3），全靠三处接线：
//   - **A** 派生写库前（DeriveBatch/deriveRows flush，增量与全量重建共用）：内存命中替换；
//   - **B** 手动快速应用（ApplyOverrides）：只点改命中行，幂等可重复调；
//   - **C** 定向重算（RecomputeForOverrides）：只重算受影响 src_row_id，不跑全量派生。
//
// 统一入口语义（§5 不变式，**宁缺勿错**）：targets=(src_row_id, ep_index) 集合；
// 命中且三元组 (src_row_id, key_raw_name, key_epid) 校验通过：
//   - verdict=corrected ⇒ epid=new_epid；verdict=kept ⇒ 不改值，只登记"已裁定"；
// 三元组对不上（集序漂移/插集/规则值已变）⇒ 该条标 stale 重回清单复核，**且不改值**。

// OverrideTarget 应用目标：(src_row_id, ep_index) —— 与台账唯一键 uk_row_ep 同口径（§8.1）。
type OverrideTarget struct {
	SrcRowId int64 `json:"srcRowId"`
	EpIndex  int   `json:"epIndex"`
}

// OverrideApplyResult ApplyOverrides（B 点）的汇总。error 只反映 DB 故障；
// 三元组校验失败/未命中等业务结果全部落在计数字段里（宁缺勿错 ≠ 失败）。
// ⚠️ 红线③：只有计数、不携带任何台账字段/evidence —— 裁定明细绝不进出参。
type OverrideApplyResult struct {
	Considered int `json:"considered"` // 参与裁定的 active 条数（targets 过滤后）
	Applied    int `json:"applied"`    // corrected 且三元组通过 ⇒ 已点改生效
	InEffect   int `json:"inEffect"`   // 派生表已是 new_epid ⇒ 重复 apply 的幂等命中（未写库）
	Kept       int `json:"kept"`       // kept 且三元组通过 ⇒ 值不变，登记"已裁定"
	Stale      int `json:"stale"`      // 三元组对不上 ⇒ 已标 stale 重回清单
	Missing    int `json:"missing"`    // 派生表已无对应行 ⇒ 不改台账（源行被删/重建未跑到；orphan 归清扫任务）
}

// ovKey 台账条目的内存命中键：(src_row_id, ep_index)。
type ovKey struct {
	src int64
	ep  int
}

// overrideLedger 已生效裁定（status=active）的内存索引 —— A 点一次性预加载（§8.2 "台账很小可
// 预加载"），批内**零重复查询**；applyToPending 纯内存命中替换。
type overrideLedger struct {
	byKey map[ovKey]*entity.MovieEpidOverride
}

// loadOverrideLedger 预加载 active 台账为内存索引。未装配 ⇒ (nil, nil)（A 点跳过）；
// 查询失败 ⇒ (nil, err)（调用方决定中止还是降级）。
func (s *EpidService) loadOverrideLedger(ctx context.Context) (*overrideLedger, error) {
	// D3 生效开关（默认关）：关着 ⇒ A 点跳过（nil ledger，派生照常、不套用裁定）。
	// B/C 不走这里的 nil 降级 —— 它们各自在入口先显式拒绝（见 ApplyOverrides/RecomputeForOverrides）。
	if s.ov == nil || !epidOverrideEnabled.Load() {
		return nil, nil
	}
	rows, err := s.ov.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	return newOverrideLedger(rows), nil
}

// loadOverrideLedgerForRewrite **整源行重写路径**（mode=scope）专用：**绕过 D3 生效开关**装载 active 台账。
// 开关只应管「新校正的应用」（A 点），不该管「已生效校正的保持」——整源行重写若不套台账，会把已生效
// 校正冲掉（2026-11-14 mode=fill 实际冲掉 34 条）。开关关着装载是安全方向：只读 active 重套用，
// 等价于把已套用过的校正原样保持。未装配 ⇒ (nil, nil)（无校正可冲）；查询失败 ⇒ (nil, err)
// （调用方必须中止 —— mode=scope 的台账 fail-closed）。
func (s *EpidService) loadOverrideLedgerForRewrite(ctx context.Context) (*overrideLedger, error) {
	if s.ov == nil {
		return nil, nil
	}
	rows, err := s.ov.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	return newOverrideLedger(rows), nil
}

func newOverrideLedger(rows []entity.MovieEpidOverride) *overrideLedger {
	l := &overrideLedger{byKey: make(map[ovKey]*entity.MovieEpidOverride, len(rows))}
	for i := range rows {
		l.byKey[ovKey{rows[i].SrcRowId, rows[i].EpIndex}] = &rows[i]
	}
	return l
}

// applyToPending **A 点核心**（方案 §8.2）：派生写库前对本批 pendingRows 做**内存命中替换**
// —— 台账已在派生循环外预加载，这里零查询、纯内存，不影响派生效率。
//
// 语义（§5 不变式，宁缺勿错）：
//   - 命中且三元组 (src_row_id, key_raw_name, key_epid) 逐字符对上：
//     verdict=corrected ⇒ epid=new_epid；verdict=kept ⇒ 不改值只登记；
//   - 三元组对不上（集序漂移/插集/规则值已变）⇒ 不改值，台账 id 记入 staleIds（调用方批末
//     一次性 SetStatus(stale) 重回清单复核）；
//   - 命中的源行在本批被重建、但其 ep_index 已不存在（集序缩短）⇒ 漂移确凿 ⇒ 同样 stale；
//   - 台账条目对应的源行不在本批重建集合 ⇒ 完全不碰（旧派生行还在，没有漂移证据，不下判）。
//
// ⚠️ 本函数只应见到**刚派生出的** pendingRows（flush 用完即清，一次性消费）—— 已套用过的行
// 再进来会因 epid≠key_epid 被判 stale；生产路径 flush 后立即清空，不存在二次套用。
func (l *overrideLedger) applyToPending(rows []entity.MovieEpisodeEpid) (corrected, kept int, staleIds []int64) {
	if l == nil || len(l.byKey) == 0 || len(rows) == 0 {
		return 0, 0, nil
	}
	stale := make(map[int64]struct{})
	// 本 flush 内"真被重建"的源行 → 其集下标集合（一个源行的行不会跨 flush 拆分：攒完整源行才可能触发 flush）
	rebuilt := make(map[int64]map[int]struct{})
	for i := range rows {
		eps, ok := rebuilt[rows[i].SrcRowId]
		if !ok {
			eps = make(map[int]struct{})
			rebuilt[rows[i].SrcRowId] = eps
		}
		eps[rows[i].EpIndex] = struct{}{}
	}
	// 集序缩短：命中的源行被重建但对应下标已不存在 ⇒ 三元组无从对上 ⇒ stale
	for k, ov := range l.byKey {
		if eps, inBatch := rebuilt[k.src]; inBatch {
			if _, present := eps[k.ep]; !present {
				stale[ov.Id] = struct{}{}
			}
		}
	}
	for i := range rows {
		ov, hit := l.byKey[ovKey{rows[i].SrcRowId, rows[i].EpIndex}]
		if !hit {
			continue
		}
		if rows[i].RawName != ov.KeyRawName || rows[i].Epid != ov.KeyEpid {
			stale[ov.Id] = struct{}{} // 不改值（宁缺勿错），批末统一标 stale
			continue
		}
		switch {
		case ov.Verdict == entity.OverrideVerdictCorrected && ov.NewEpid != "":
			rows[i].Epid = ov.NewEpid
			corrected++
		case ov.Verdict == entity.OverrideVerdictKept:
			kept++ // 不改值，只登记"已裁定"
		default:
			// corrected 却无 new_epid：import 静态校验本应拦下 ⇒ 防御性兜底，标 stale 交人工复核
			stale[ov.Id] = struct{}{}
		}
	}
	if len(stale) == 0 {
		return corrected, kept, nil
	}
	staleIds = make([]int64, 0, len(stale))
	for id := range stale {
		staleIds = append(staleIds, id)
	}
	sort.Slice(staleIds, func(i, j int) bool { return staleIds[i] < staleIds[j] })
	return corrected, kept, staleIds
}

// markOverridesStale 三元组校验失败的台账条目批末统一标 stale（§5：重回清单复核）。
// 失败只记日志：校正已"不套用"（值安全），stale 只是台账簿记，绝不能反过来影响派生主链路。
func (s *EpidService) markOverridesStale(ctx context.Context, ids []int64) {
	if s.ov == nil || len(ids) == 0 {
		return
	}
	n, err := s.ov.SetStatus(ctx, ids, entity.OverrideStatusStale)
	if err != nil {
		log.Printf("[epid] 台账标 stale 失败（不影响派生；条目保持 active，下批三元组校验会再次拦截）: ids=%v err=%v", ids, err)
		return
	}
	if n > 0 {
		log.Printf("[epid] 台账 %d 条三元组校验失败 ⇒ 已标 stale 重回清单复核（值未套用，宁缺勿错）", n)
	}
}

// ApplyOverrides **B 点：手动快速应用**（方案 §8.2）—— 统一入口的"生效"面（T4 管理端调用）。
//
// 只对命中行做**点改**（每行一条 UPDATE、一个事务提交），幂等可重复调：
//   - 已生效（派生表已是 new_epid）⇒ 记 InEffect，不再写库；
//   - 三元组 (src_row_id, key_raw_name, key_epid) 对不上 ⇒ 标 stale 且**不改值**（宁缺勿错）；
//   - 派生表无对应行 ⇒ 记 Missing、不动台账（源行被删归 orphan 清扫；与 A 点不同，B 没有
//     "该源行刚被重建"的确凿漂移证据，不妄判 stale）。
//
// targets 传 nil/空 ⇒ 应用**全部** active（§5"清空重建收尾跑一次 ApplyOverrides(all active)"、
// T4"一键应用"共用）；传具体 targets ⇒ 只裁定命中的条目。
// 与 C 的分工：B 不走规则链路；要连派生一起刷新用 RecomputeForOverrides。
func (s *EpidService) ApplyOverrides(ctx context.Context, targets []OverrideTarget) (OverrideApplyResult, error) {
	var res OverrideApplyResult
	if s.ov == nil {
		return res, errors.New("epid override: 裁定台账未装配（SetEpidOverrideRepo），无法应用")
	}
	// D3 生效开关（T4）：关着 ⇒ 显式拒绝（fail-closed）—— 不做"静默 0 计数"降级，
	// 让调用方/管理端明确知道"没生效"而不是"没命中"。
	if !EpidOverrideEnabled() {
		return res, errors.New("epid override: 生效开关未开启（D3 默认关；先录入核对，PUT /manage/epid/overrides/enabled 打开后再应用）")
	}
	// P1（2026-11-18）：pending → active 的**激活是 apply 的职责** —— import 写 pending，
	// 但此前没有任何端点能把 pending 转 active ⇒ 条目永远卡 pending（M5 取证批实测：
	// 台账 id=61 靠 SQL 直改绕过）。apply = 确认语义：先把全部 pending 升 active，
	// 再走既有校验/patch；KeyEpid 与当前规则 epid 不匹配者会被标 stale（宁缺勿错不变）。
	if promoted, err := s.ov.PromotePending(ctx); err != nil {
		return res, err
	} else if promoted > 0 {
		log.Printf("[epid] 台账 pending → active：%d 条（apply 激活）", promoted)
	}
	ledger, err := s.loadOverrideLedger(ctx)
	if err != nil {
		return res, err
	}
	if ledger == nil || len(ledger.byKey) == 0 {
		return res, nil
	}
	// targets 过滤：nil/空 = 全部 active
	candidates := make([]*entity.MovieEpidOverride, 0, len(ledger.byKey))
	if len(targets) == 0 {
		for _, ov := range ledger.byKey {
			candidates = append(candidates, ov)
		}
	} else {
		want := make(map[ovKey]struct{}, len(targets))
		for _, t := range targets {
			want[ovKey{t.SrcRowId, t.EpIndex}] = struct{}{}
		}
		for k, ov := range ledger.byKey {
			if _, ok := want[k]; ok {
				candidates = append(candidates, ov)
			}
		}
	}
	if len(candidates) == 0 {
		return res, nil
	}
	res.Considered = len(candidates)

	// 一次按 src_row_id 收窄取派生行现值（uk_row_ep 前缀键查，红线④；行数 = 命中源行的集数，有界）。
	srcIds := make([]int64, 0, len(candidates))
	seenSrc := make(map[int64]struct{}, len(candidates))
	for _, ov := range candidates {
		if _, ok := seenSrc[ov.SrcRowId]; !ok {
			seenSrc[ov.SrcRowId] = struct{}{}
			srcIds = append(srcIds, ov.SrcRowId)
		}
	}
	curRows, err := s.repo.EpidRowsBySrcRowIds(ctx, srcIds)
	if err != nil {
		return res, err
	}
	cur := make(map[ovKey]entity.MovieEpisodeEpid, len(curRows))
	for _, r := range curRows {
		cur[ovKey{r.SrcRowId, r.EpIndex}] = r
	}

	// 逐条三元组校验；需要点改的攒一批，一个事务提交（少一个条目失败不影响其余条目已校验的事实）。
	patches := make([]mysql.EpidRowPatch, 0, len(candidates))
	staleIds := make([]int64, 0)
	for _, ov := range candidates {
		row, ok := cur[ovKey{ov.SrcRowId, ov.EpIndex}]
		if !ok {
			res.Missing++
			continue
		}
		if row.RawName != ov.KeyRawName {
			staleIds = append(staleIds, ov.Id) // 集序漂移/插集 ⇒ 重回清单，值不变
			continue
		}
		switch ov.Verdict {
		case entity.OverrideVerdictCorrected:
			switch row.Epid {
			case ov.NewEpid:
				res.InEffect++ // 幂等：已生效（重复 apply / A 已套用）⇒ 不写库
			case ov.KeyEpid:
				patches = append(patches, mysql.EpidRowPatch{SrcRowId: ov.SrcRowId, EpIndex: ov.EpIndex, Epid: ov.NewEpid})
				res.Applied++
			default:
				// 现值既不是裁定时的规则值、也不是目标值 ⇒ 规则已变/被重算 ⇒ stale 重回清单
				staleIds = append(staleIds, ov.Id)
			}
		case entity.OverrideVerdictKept:
			if row.Epid != ov.KeyEpid {
				staleIds = append(staleIds, ov.Id)
				continue
			}
			res.Kept++ // 不改值，登记"已裁定"
		default:
			staleIds = append(staleIds, ov.Id)
		}
	}
	if len(patches) > 0 {
		if _, err := s.repo.UpdateEpidByRowKeys(ctx, patches); err != nil {
			return res, err
		}
	}
	res.Stale = len(staleIds)
	s.markOverridesStale(ctx, staleIds)
	return res, nil
}

// RecomputeForOverrides **C 点：定向重算**（方案 §8.2）—— 只对受影响的 src_row_id 走完整派生链路
// （解析 → 桥接/锚点 → A 点套用裁定 → ReplaceForSourceRows），**不跑全量派生**。
//
// 与 B 的分工：B 只点改命中行（秒级）；C 会把该源行**全部集**按当前规则重算一遍（整源行重算
// 铁律，绝不按 ep_index 打补丁），适用于"连规则值一起刷新"的场景。两者均幂等。
// force 语义：管理端**显式**定向 ⇒ 绕过抖动守卫（否则守卫可能拦下导致校正落不了地）。
func (s *EpidService) RecomputeForOverrides(ctx context.Context, targets []OverrideTarget) (DeriveStats, error) {
	st := DeriveStats{SkipWhy: map[string]int{}}
	if s.ov == nil {
		return st, errors.New("epid override: 裁定台账未装配（SetEpidOverrideRepo），无法定向重算")
	}
	// D3 生效开关（T4）：关着 ⇒ 显式拒绝。C 会**整源行重写**派生行，开关关着 ⇒ A 点缺席 ⇒
	// 现有校正在重算的行上会被规则值冲掉 —— 必须先打开开关再定向重算。
	if !EpidOverrideEnabled() {
		return st, errors.New("epid override: 生效开关未开启（D3 默认关）：定向重算会整源行重写，须先打开开关让 A 点套用裁定")
	}
	srcIds := make([]int64, 0, len(targets))
	seenSrc := make(map[int64]struct{}, len(targets))
	for _, t := range targets {
		if t.SrcRowId <= 0 {
			continue
		}
		if _, ok := seenSrc[t.SrcRowId]; ok {
			continue
		}
		seenSrc[t.SrcRowId] = struct{}{}
		srcIds = append(srcIds, t.SrcRowId)
	}
	if len(srcIds) == 0 {
		return st, nil
	}
	rows, err := s.repo.SourceRowsByIds(ctx, srcIds)
	if err != nil {
		return st, err
	}
	if len(rows) == 0 {
		return st, nil // 源行已不存在（下架）⇒ 无可重算；orphan 清理另行处理
	}
	ledger, err := s.loadOverrideLedger(ctx)
	if err != nil {
		// 台账读不到就**不重算**：C 会整源行重写派生行，套用层缺席 ⇒ 现有校正会被规则值冲掉。
		return st, fmt.Errorf("epid override: 台账预加载失败，已中止定向重算（防校正被冲掉）: %w", err)
	}
	staleIds, err := s.deriveRows(ctx, rows, true, &st, ledger)
	if err != nil {
		return st, err
	}
	st.OverrideStale = len(staleIds)
	s.markOverridesStale(ctx, staleIds)
	return st, nil
}

// ---- T3 管理端包级入口（同 RunFullDerive/ReportEpid 取向：走默认实例，未装配报错）----

// ApplyOverrides B 点包级入口（T4 管理端"手动快速应用"调用）。
func ApplyOverrides(ctx context.Context, targets []OverrideTarget) (OverrideApplyResult, error) {
	s := currentEpidService()
	if s == nil {
		return OverrideApplyResult{}, errors.New("epid override: 派生服务未装配")
	}
	return s.ApplyOverrides(ctx, targets)
}

// RecomputeForOverrides C 点包级入口（T4 管理端"定向重算"调用）。
func RecomputeForOverrides(ctx context.Context, targets []OverrideTarget) (DeriveStats, error) {
	s := currentEpidService()
	if s == nil {
		return DeriveStats{SkipWhy: map[string]int{}}, errors.New("epid override: 派生服务未装配")
	}
	return s.RecomputeForOverrides(ctx, targets)
}

// ---- T4 管理端入口（2026-11-12）：import / list 的服务级透传 + 出参白名单视图 ----

// OverrideImportInput import 端点（POST /manage/epid/overrides/import）的**单条候选**请求项。
//
// 请求体 = 本结构的 JSON 数组；字段名与类型（T5 生成候选文件按此格式）：
//
//	[{"srcRowId": int64, "epIndex": int,
//	  "keyRawName": string,  // 三元组校验键①（裁定时的集名），必填
//	  "keyEpid": string,     // 三元组校验键②（裁定时的规则 epid），必填
//	  "verdict": "corrected" | "kept",
//	  "newEpid": string,     // verdict=corrected 必填（目标 epid，5 段）
//	  "provenance": "rule" | "external" | "agent" | "human",
//	  "confidence": "inferred" | "verified",
//	  "reason": string（可选）, "evidence": object（可选）, "ruleVersionAt": string（可选）}]
//
// status / id / matchKey / mid / createdAt **由 repo 强制/回填**（status=pending、id 自增、
// match_key/mid 以派生表现值覆盖）—— 请求体传了也无效。逐行静态校验、批内去重、唯一键冲突
// 拒收、key_epid 与派生表现值对账全在 repo UpsertPending（T2，本任务未改）。
// Evidence 走独立 map 字段：entity.MovieEpidOverride.Evidence 是 json:"-"（红线③防外泄兜底），
// 直接绑实体收不到请求里的 evidence ⇒ 必须经本输入结构中转。
type OverrideImportInput struct {
	SrcRowId      int64          `json:"srcRowId"`
	EpIndex       int            `json:"epIndex"`
	KeyRawName    string         `json:"keyRawName"`
	KeyEpid       string         `json:"keyEpid"`
	Verdict       string         `json:"verdict"`
	NewEpid       string         `json:"newEpid"`
	Provenance    string         `json:"provenance"`
	Confidence    string         `json:"confidence"`
	Reason        string         `json:"reason,omitempty"`
	Evidence      map[string]any `json:"evidence,omitempty"`
	RuleVersionAt string         `json:"ruleVersionAt,omitempty"`
}

// OverrideView 台账行的**管理端出参视图**（GET /manage/epid/overrides 的 list 项）。
//
// ⚠️ 红线③：台账行出参必须**显式白名单映射**，绝不直接序列化 entity.MovieEpidOverride。
// 白名单 = 核对/确认/应用工作流要用的运营字段；**evidence**（外链/抓取记录）、**ruleVersionAt**
// 与审计三列（created_at / verified_at / verified_by）刻意不出 —— 出参是给"核对下一条要不要
// 确认/应用"用的，不是台账全量导出。id 保留：后续确认/撤销（SetStatus 按 ids）要用。
type OverrideView struct {
	Id         int64  `json:"id"`
	SrcRowId   int64  `json:"srcRowId"`
	EpIndex    int    `json:"epIndex"`
	MatchKey   string `json:"matchKey"`
	Mid        int64  `json:"mid"`
	KeyRawName string `json:"keyRawName"`
	KeyEpid    string `json:"keyEpid"`
	Verdict    string `json:"verdict"`
	NewEpid    string `json:"newEpid"`
	Provenance string `json:"provenance"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
}

// overrideView 显式映射（字段一一搬运，不反射、不整行透传 —— 红线③）。
func overrideView(r entity.MovieEpidOverride) OverrideView {
	return OverrideView{
		Id: r.Id, SrcRowId: r.SrcRowId, EpIndex: r.EpIndex,
		MatchKey: r.MatchKey, Mid: r.Mid,
		KeyRawName: r.KeyRawName, KeyEpid: r.KeyEpid,
		Verdict: r.Verdict, NewEpid: r.NewEpid,
		Provenance: r.Provenance, Confidence: r.Confidence,
		Reason: r.Reason, Status: r.Status,
	}
}

// ImportOverrides 管理端 import 入口（T4 POST /manage/epid/overrides/import）：T2 UpsertPending
// 的服务级透传 —— 校验/去重/键冲突/key_epid 对账/入库全在 repo，本层只做输入映射。
// **不受 D3 生效开关影响**（先录入核对、后手动打开）。rejected 等业务结果都在返回值里（非 error）。
func ImportOverrides(ctx context.Context, inputs []OverrideImportInput) (mysql.OverrideImportResult, error) {
	s := currentEpidService()
	if s == nil || s.ov == nil {
		return mysql.OverrideImportResult{}, errors.New("epid override: 裁定台账未装配（SetEpidOverrideRepo），无法导入")
	}
	rows := make([]entity.MovieEpidOverride, 0, len(inputs))
	for i := range inputs {
		in := inputs[i]
		rows = append(rows, entity.MovieEpidOverride{
			SrcRowId:      in.SrcRowId,
			EpIndex:       in.EpIndex,
			KeyRawName:    in.KeyRawName,
			KeyEpid:       in.KeyEpid,
			Verdict:       in.Verdict,
			NewEpid:       in.NewEpid,
			Provenance:    in.Provenance,
			Confidence:    in.Confidence,
			Reason:        in.Reason,
			Evidence:      entity.JSONMap(in.Evidence),
			RuleVersionAt: in.RuleVersionAt,
		})
	}
	return s.ov.UpsertPending(ctx, rows)
}

// ListOverrides 管理端 GET 入口（T4 GET /manage/epid/overrides）：台账整表读 + 三维统计。
// **不受 D3 生效开关影响**（查看先行）。台账小表整读合规（同 ListActive 的量级论证，见 repo 注释）；
// 出参经 overrideView 白名单映射（红线③），status 过滤由调用方按需做（行数小，内存过滤）。
func ListOverrides(ctx context.Context) ([]OverrideView, mysql.OverrideStats, error) {
	s := currentEpidService()
	if s == nil || s.ov == nil {
		return nil, mysql.OverrideStats{}, errors.New("epid override: 裁定台账未装配（SetEpidOverrideRepo），无法读取")
	}
	rows, err := s.ov.ListAll(ctx)
	if err != nil {
		return nil, mysql.OverrideStats{}, err
	}
	stats, err := s.ov.Stats(ctx)
	if err != nil {
		return nil, mysql.OverrideStats{}, err
	}
	out := make([]OverrideView, 0, len(rows))
	for i := range rows {
		out = append(out, overrideView(rows[i]))
	}
	return out, stats, nil
}

// ===== U5 / H2：一键全量派生（管理端入口, 2026-11-11）=====

// DeriveMode 一键派生的两种方式。
type DeriveMode string

const (
	// DeriveModeFill 补缺/更新：从**表头**起把"集内容变过"的源行补齐/更新，**不动已有**派生。
	DeriveModeFill DeriveMode = "fill"
	// DeriveModeRebuild 清空重建：**先 TRUNCATE 派生表**再全量重算（规则升级后用，方案 §5.4 ③）。
	DeriveModeRebuild DeriveMode = "rebuild"
)

// DeriveState 派生任务状态（管理端只读）。
type DeriveState struct {
	Running   bool   `json:"running"`   // 是否有全量任务在跑
	Mode      string `json:"mode"`      // fill | rebuild（无任务时为空）
	StartedAt int64  `json:"startedAt"` // 全量任务起始毫秒（0 = 无）
	Cursor    int64  `json:"cursor"`    // 当前游标（源行 id；0 = 刚到表尾）
	HotPhase  bool   `json:"hotPhase"`  // true = 正在跑"热门趟"（只扫 mid 非空的源行）

	// ---- 本轮任务的内存计数（2026-11-11）----
	// 为什么放这里：卡片需要"跑到哪了/还缺多少"，而任何全表计数在 590 万行下都要 9~19s ✗；
	// 任务自己就知道扫过多少、重建多少 ⇒ 0 成本，且**不随表规模增长** ✓
	Scanned     int64   `json:"scanned"`     // 累计扫过的源行数
	Rebuilt     int64   `json:"rebuilt"`     // 累计重建的源行数
	Batches     int64   `json:"batches"`     // 累计批数
	ElapsedSec  int64   `json:"elapsedSec"`  // 本轮已跑秒数（未在跑时为 0）
	ScanRate    float64 `json:"scanRate"`    // 扫过源行/秒
	RebuildRate float64 `json:"rebuildRate"` // 重建源行/秒
}

// DeriveStatus 读当前派生状态。
func DeriveStatus() DeriveState {
	m := ""
	if v := deriveFullMode.Load(); v != nil {
		m, _ = v.(string)
	}
	started := deriveFullStartedAt.Load()
	running := deriveFullRunning.Load()
	var elapsed int64
	if running && started > 0 {
		if e := (time.Now().UnixMilli() - started) / 1000; e > 0 {
			elapsed = e
		}
	}
	scanned, rebuilt := deriveFullScanned.Load(), deriveFullRebuilt.Load()
	var scanRate, rebuildRate float64
	if elapsed > 0 {
		scanRate = float64(scanned) / float64(elapsed)
		rebuildRate = float64(rebuilt) / float64(elapsed)
	}
	return DeriveState{
		Running:     running,
		Mode:        m,
		StartedAt:   started,
		Cursor:      deriveCursor.Load(),
		HotPhase:    deriveHotPhase.Load(),
		Scanned:     scanned,
		Rebuilt:     rebuilt,
		Batches:     deriveFullBatches.Load(),
		ElapsedSec:  elapsed,
		ScanRate:    scanRate,
		RebuildRate: rebuildRate,
	}
}

// RunFullDerive 启动一次全量派生（**异步**）。返回 false = 已有任务在跑 / 未装配。
//
// 设计要点：①**异步**（全库 63 万源行要跑几十分钟，不能占住 HTTP）；②**互斥**（与采集收尾的自动触发
// 共用 `deriveRunning`）；③**有上限**（`maxBatches` 批数 + `timeout` 总时长），防一次点错把机器跑满；
// ④进度可观测（每批一行日志 + `DeriveStatus()`）。
func RunFullDerive(mode DeriveMode, batch, maxBatches int, timeout time.Duration) bool {
	s := currentEpidService()
	if s == nil || !deriveRunning.CompareAndSwap(false, true) {
		return false
	}
	if batch <= 0 {
		batch = deriveBatchPerTrigger
	}
	if timeout <= 0 {
		timeout = time.Hour
	}
	if mode != DeriveModeRebuild {
		mode = DeriveModeFill
	}
	deriveFullMode.Store(string(mode))
	deriveFullStartedAt.Store(time.Now().UnixMilli())
	deriveFullRunning.Store(true)

	go func() {
		defer func() {
			deriveFullRunning.Store(false)
			deriveFullStartedAt.Store(0)
			deriveRunning.Store(false)
			// 与自动触发同款：**必须 recover**（一条脏数据不能把全站服务打挂）
			if r := recover(); r != nil {
				log.Printf("[epid] 全量派生 panic 已拦截（不影响线上服务）: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// 本轮内存计数归零 + 预热进度快照（源行总数 / 影片完整度基线）。
		// 目的：管理员点开卡片**立刻**能看到"跑到哪、还缺多少"，而不是等第一次刷新才开始算那两个慢查询。
		ResetDeriveRunCounters()
		WarmProgressSnapshots()

		if mode == DeriveModeRebuild {
			if err := s.repo.TruncateAll(ctx); err != nil {
				log.Printf("[epid] 清空派生表失败 ⇒ 任务中止（派生表未被改动）: %v", err)
				return
			}
			log.Printf("[epid] 已清空派生表（mode=rebuild）；期间出参无 epid，端侧回退实时解析，不影响可用性")
		}
		// 从表头起跑 + 热门趟优先（H6）
		deriveCursor.Store(0)
		deriveHotPhase.Store(true)

		total := DeriveStats{SkipWhy: map[string]int{}}
		for n := 0; maxBatches <= 0 || n < maxBatches; n++ {
			// 需求 3（2026-11-11）：**每批前让路采集** —— 两边都在写同一张库，重叠会推高死锁概率；
			// 最多等 3 分钟 ⇒ 不让全量任务被采集的长跑"饿死"。
			waitCollectIdle(ctx, 3*time.Minute)
			after := deriveCursor.Load()
			runHot := deriveHotPhase.Load()
			st, err := s.DeriveBatch(ctx, after, batch, false)
			if err != nil {
				log.Printf("[epid] 全量派生中断（已处理 %d 批，游标 %d）: %v", n, after, err)
				break
			}
			total.Scanned += st.Scanned
			total.Rebuilt += st.Rebuilt
			total.Rows += st.Rows
			total.Skipped += st.Skipped
			total.EmptyEpid += st.EmptyEpid
			total.OverrideApplied += st.OverrideApplied
			total.OverrideKept += st.OverrideKept
			total.OverrideStale += st.OverrideStale
			// 内存计数（供卡片实时读数；**不做任何全表计数** —— 那是 2026-11-11 线上事故的根因）
			deriveFullScanned.Add(int64(st.Scanned))
			deriveFullRebuilt.Add(int64(st.Rebuilt))
			deriveFullRows.Add(st.Rows)
			deriveFullBatches.Add(1)
			for k, v := range st.SkipWhy {
				total.SkipWhy[k] += v
			}
			finished := st.Scanned < batch
			if finished {
				deriveHotPhase.Store(!runHot) // 热门趟到底 ⇒ 进长尾趟；长尾趟到底 ⇒ 回热门趟
				deriveCursor.Store(0)
			} else {
				deriveCursor.Store(st.LastId)
			}
			phase := "长尾趟"
			if runHot {
				phase = "热门趟"
			}
			log.Printf("[epid] 全量派生 第%d批: 趟=%s 游标 %d→%d 扫=%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d 校正(套用/登记/标stale)=%d/%d/%d",
				n+1, phase, after, deriveCursor.Load(), st.Scanned, st.Rebuilt, st.Rows, st.Skipped, st.EmptyEpid,
				st.OverrideApplied, st.OverrideKept, st.OverrideStale)
			// 热门趟扫完 ⇒ 继续跑长尾趟；长尾趟也扫完 ⇒ 整趟结束
			if finished && !runHot {
				break
			}
		}
		log.Printf("[epid] 全量派生结束（mode=%s）: 扫=%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d 原因=%v 校正(套用/登记/标stale)=%d/%d/%d",
			mode, total.Scanned, total.Rebuilt, total.Rows, total.Skipped, total.EmptyEpid, total.SkipWhy,
			total.OverrideApplied, total.OverrideKept, total.OverrideStale)

		// ---- 收尾 1/2：重建后刷新优化器统计信息 ----
		// TRUNCATE + 全量 INSERT 会让统计信息明显失真 ⇒ 分组类指标（对齐率/成熟度）的查询计划可能退化。
		// 用**独立上下文**（任务本身可能已到超时，`ctx` 已失效）。
		if mode == DeriveModeRebuild {
			actx, acancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if err := s.repo.AnalyzeDerived(actx); err != nil {
				log.Printf("[epid] ANALYZE movie_episode_epid 失败（不影响可用性）: %v", err)
			} else {
				log.Printf("[epid] 已 ANALYZE movie_episode_epid（重建后刷新统计信息，避免优化器退化）")
			}
			acancel()
		}

		// ---- 收尾 2/2：刷新"影片完整度"快照 ----
		// 这是新的**进度主口径**（该片全部源行都已派生 / 影片总数），与 §4.8 门槛的成熟判据同一定义
		// ⇒ 收尾刷新一次，空闲态卡片立刻是新值，也顺手把"还能不能过门槛"打进行日志。
		{
			mctx, mcancel := context.WithTimeout(context.Background(), 3*time.Minute)
			if mc, ok := RefreshMovieCompleteness(mctx); ok {
				pct := 0.0
				if mc.Movies > 0 {
					pct = float64(mc.CompleteMovies) * 100 / float64(mc.Movies)
				}
				log.Printf("[epid] 影片完整度: %d/%d 完整 (%.1f%%)，还缺 %d 部影片 / %d 个源行",
					mc.CompleteMovies, mc.Movies, pct, mc.Movies-mc.CompleteMovies, mc.MissingSrcRows)
			}
			mcancel()
		}

		// ---- 收尾 3/3：触发重型指标快照刷新（后台）----
		// 跑完后（尤其 rebuild）全库指标才真正有意义 ⇒ 让空闲态读数尽快变成新值。
		KickHeavyMetricsRefresh()
		if rep, rerr := s.AlignmentReport(ctx); rerr == nil {
			log.Printf("[epid] 验收指标: 派生行=%d 空epid=%d 对齐组=%d/%d (%.1f%%) 来源=%v 门槛(≥85%%)=%v",
				rep.TotalDerivedRows, rep.EmptyEpidRows, rep.AlignedGroups, rep.TotalGroups,
				rep.AlignmentPct, rep.ByFromSrc, rep.AlignmentPct >= 85)
			if dr, se, cerr := s.repo.CoverageFull(ctx); cerr == nil && se > 0 {
				log.Printf("[epid] 完整覆盖率(§6.1 口径, 空epid也算行): %d/%d ⇒ %.2f%%（目标 ≈100%%）",
					dr, se, float64(dr)*100/float64(se))
			}
		}
	}()
	return true
}

// EpidCoverage 覆盖率（§6.1：派生行数 / Σ JSON_LENGTH(episodes_json)）。
type EpidCoverage struct {
	DerivedRows    int64   `json:"derivedRows"`
	SourceEpisodes int64   `json:"sourceEpisodes"`
	Pct            float64 `json:"pct"`
}

// EpidReport 管理端只读报告：状态 + 验收指标（分层 + 双口径对齐率 + 成熟度 + 空语义）。
type EpidReport struct {
	Status        DeriveState      `json:"status"`
	DerivedRows   int64            `json:"derivedRows"`
	EmptyEpidRows int64            `json:"emptyEpidRows"`
	AlignedGroups int64            `json:"alignedGroups"`
	TotalGroups   int64            `json:"totalGroups"`
	AlignmentPct  float64          `json:"alignmentPct"`
	ByFromSrc     map[string]int64 `json:"byFromSrc"`
	Coverage      *EpidCoverage    `json:"coverage,omitempty"`
	CoverageError string           `json:"coverageError,omitempty"`

	// ---- 口径层（2026-11-11 / 2026-11-15 修订）：分层 / 双口径 / 空语义 ----
	// ⚠️ 主口径（2026-11-15 用户拍板）：**mk 级对齐**（ClassAlignment）——对齐 = 该片非空 epid 只有一种
	//（单源片默认按对齐算；多源收敛同 epid 也对齐）；未对齐 = 多源分歧（P-1 冲突类）。
	// 组级口径（alignedGroups/totalGroups，site≥2 的组占比）**仅作对照**——51.3% 单源片结构性压制比率，
	// 且单源片是覆盖缺失而非机制失败，两种失败不可混计。
	// 2026-11-17：**可比口径对齐率（comparable*）已删除** —— 无任何 UI 消费方，且每 120s 跑一次
	// 全表聚合（纯浪费），字段易被误读成"对齐率"。组级字段保留仅作对照。
	ClassAlignment    []mysql.ClassAlignmentStat `json:"classAlignment"`     // 按分类的跨源对齐（mk 级）
	ClassAlignmentAll mysql.ClassAlignmentStat   `json:"classAlignmentAll"`  // 全库汇总（Category="全部"）
	Deriving          bool                       `json:"deriving"`           // true = 派生任务进行中（前端只渲染进度类指标）
	Progress          *EpidProgress              `json:"progress,omitempty"` // 任务进行中也给（O(1)/缓存）
	OnlyProgress      bool                       `json:"onlyProgress"`       // true = 本轮只算了进度集（重型指标未算，前端别把 0 当真值渲染）
	MetricsAt         int64                      `json:"metricsAt"`          // 重型指标快照的采样时间（毫秒；0 = 还没算出来）
	MetricsStale      bool                       `json:"metricsStale"`       // true = 后台正在刷新，展示的是上一份快照
	EmptyByShape      []mysql.ShapeStat          `json:"emptyByShape"`
	MetricsError      string                     `json:"metricsError,omitempty"`
}

// EpidProgress 已迁到 `epid_progress.go`（2026-11-11 线上事故修法）：
// 进度集改为「写侧计数器（O(1)）+ 后台缓存快照（源行总数 / 影片完整度）+ 内存计数」，
// 因为精确计数在 590 万行下分别要 19.2s / 8.6s，会撞穿接口的每项超时并被降级成 0 ✗。

// 指标结果缓存**已移除**（2026-11-11）。原设计是"5 分钟 TTL 的进程内缓存"，但**过期后的第一次请求
// 仍要现算全套**（近 2 分钟）⇒ 正好卡在网关超时上 ✗。现改为**后台快照 + 请求只读快照**：
//   · 重型指标 → `heavyMetricsSnapshot`（epid_progress.go，TTL 5 分钟，后台刷新）
//   · 影片完整度 → `movieCompletenessSnapshot`（TTL 60 秒，后台刷新）
//   · 源行总数 → `srcRowTotalSnapshot`（TTL 1 小时，后台刷新）
//   · 全库计数 → 写侧计数器 `epid_derive_stat`（O(1) 点查）
// ⇒ 请求路径上**再没有任何全表计数/聚合**，无论数据长到多大都不会变慢。

// EpidReport 管理端只读报告：状态 + 进度 + 验收指标（分层 + 双口径对齐率 + 成熟度 + 空语义）。

// ReportEpid 汇总验收指标（只读）。
//
// 性能设计（2026-11-11 **线上 524 + "读数全是 0"两次事故**后的定稿；全部真库实测，派生表 598 万行）：
//  1. **请求路径上不允许任何全表计数/聚合** —— 两次事故的共同根因：
//     · `COUNT(*)` 19.2s、`COUNT(DISTINCT src_row_id)` 8.6s ⇒ 撞穿各自超时被降级成 0（"读数全是 0"）；
//     · 全库口径 ~32s / ≥2 site 子查询 ~33s / 按分类 ~22s ⇒ 全套近 2 分钟 ⇒ 网关 **524**。
//  2. **三档分层**（"谁在哪算"是本接口的核心设计）：
//     · T0 内存/点查：本轮扫过/重建/批次/速率（内存计数）+ 全库计数（写侧计数器，O(1)）
//     · T1 缓存快照：源行总数（1h）、影片完整度（60s）—— 后台刷新，请求只读，并返回**采样时间**
//     · T2 后台快照：全库口径/可比口径/按分类/成熟度/空形态（5 分钟一份）
//     · 按需：覆盖率 §6.1（`?full=1`，7~13s）—— 唯一允许现算的重查询，它是一次性对账、不随进度变化
//  3. **任务进行中短路**：只返回 T0 + T1。重建期对齐率/分层/成熟度的分母"不成熟"，
//     读数会横跳、**本就没有判定意义**，且会把接口拖过网关超时 ✗
//     ⇒ 前端据此显示"进行中·未计算"，**不能**把 0 当真值渲染（那正是让人以为数据丢了的原因之一）。
//  4. **每项独立超时 + 全项容错**：任一项超时/失败只降级该项（写进 `MetricsError`），绝不整接口 500。
//  5. **null 兜底**：所有 map/slice 初始化为空值（`{}`/`[]`）—— Go 的 nil 会序列化成 `null`，
//     而前端 `Object.keys(null)` 抛错会把**整个卡片渲染打挂**（2026-11-11 线上事故）。
//  6. 配套迁移：`000046` 派生表覆盖索引（对齐率系列 index-only）；`000047` 源表 `(match_key, site_id)`
//     （≥2 site 子查询至今仍**回表**：EXPLAIN `index(idx_match)` 无 `Using index`，实测 33s）；
//     `000048` 进度计数器单行表。
//
// TolerantAlignReport 容忍口径（M4，2026-11-16）按分类的片级读数。
// ⚠️ 与严格口径的 `TotalMks` **不可直接相减**：容忍口径只统计"≥2 个源"的片（单源不背书）。
type TolerantAlignReport struct {
	ByCategory []mysql.TolerantAlignmentStat `json:"byCategory"`
	All        mysql.TolerantAlignmentStat   `json:"all"`
}

// ReportAlignmentTolerant 容忍口径报告 —— **按需 / 离线专用**（M4 的出口）：
// 不进 `heavyMetricsSnapshot` 的 600s 快照、也不进请求路径（用户 2026-11-16 定："算法时间长，离线算即可"），
// 只由管理端 `GET /manage/epid/alignment/tolerant` 显式触发。
func ReportAlignmentTolerant(ctx context.Context) (TolerantAlignReport, error) {
	// slice 一律初始化成空 slice（同 `ReportEpid` 的 null 兜底纪律：nil → JSON null 会打挂前端卡片）
	out := TolerantAlignReport{
		ByCategory: []mysql.TolerantAlignmentStat{},
		All:        mysql.TolerantAlignmentStat{Pid: -1, Category: "全部"},
	}
	s := currentEpidService()
	if s == nil {
		return out, errors.New("epid 派生服务未装配")
	}
	v, all, err := s.repo.ClassAlignmentTolerant(ctx)
	if err != nil {
		return out, err
	}
	if v != nil {
		out.ByCategory = v
	}
	out.All = all
	return out, nil
}

// DateAlignReport 综艺「日期对齐」口径报告（S 口径，2026-11-17）—— **按需 / 离线专用**。
//
// 与容忍口径（M4）同构：不进 600s 快照、不进请求路径，只由
// `GET /manage/epid/alignment/date` 显式触发（用户 2026-11-17：「读侧不做特殊处理」⇒ 只在指标层）。
//
// ⚠️ `totalMks` 只统计「≥2 个纯日期型源且过守卫」的片，**不能**与严格口径的片级直接相减；
// 同一分母下的严格读数在 `strictAligned` 里。`skippedMks` 是因守卫（同源同日同 part 重复）放弃的片。
type DateAlignReport struct {
	ByCategory []mysql.DateAlignmentStat `json:"byCategory"`
	All        mysql.DateAlignmentStat   `json:"all"`
}

// ReportAlignmentDate 日期对齐口径报告（指标层出口）。
func ReportAlignmentDate(ctx context.Context) (DateAlignReport, error) {
	// slice 一律初始化成空 slice（null → JSON null 会打挂前端卡片，同 ReportEpid 纪律）
	out := DateAlignReport{
		ByCategory: []mysql.DateAlignmentStat{},
		All:        mysql.DateAlignmentStat{Pid: -1, Category: "全部"},
	}
	s := currentEpidService()
	if s == nil {
		return out, errors.New("epid 派生服务未装配")
	}
	v, all, err := s.repo.ClassAlignmentDate(ctx)
	if err != nil {
		return out, err
	}
	if v != nil {
		out.ByCategory = v
	}
	out.All = all
	return out, nil
}

// FunnelReport 对齐率分层漏斗报告（L0~L5，2026-11-17）—— **按需 / 离线专用**。
//
// 总则：`docs/epid-集身份/方案-对齐率分层判定总则-2026-11-17.md`
// 只做判定、不改 epid；不进 600s 快照、不进请求路径。
//
// ⚠️ 不变量：`slotsTotal == l0Single + l1Slot + l2Tolerant + l3Date + l4Num + l5Mixed + leftover`。
// 分母恒为全部非空槽位（单源槽默认算对齐，覆盖差异不是解析问题）。
type FunnelReport struct {
	ByCategory []mysql.FunnelStat `json:"byCategory"`
	All        mysql.FunnelStat   `json:"all"`

	// ===== 快照元信息（P11：改为后台刷新 + 读快照）=====
	ComputedAt    time.Time `json:"computedAt"`    // 快照生成时间（零值 = 尚无快照）
	PolicyVersion string    `json:"policyVersion"` // 口径版本（L5/L6 语义变更时递增）
	Truncated     bool      `json:"truncated"`     // 1=计算被分页上限截断（结果不完整）
	Pending       bool      `json:"pending"`       // true = 还没有快照，需先 POST 刷新
}

// 漏斗快照刷新的单飞状态（同一时刻只允许一个后台任务）
var (
	funnelRefreshMu  sync.Mutex
	funnelRefreshing bool
)

// FunnelRefreshRunning 当前是否有刷新任务在跑（管理端展示用）。
func FunnelRefreshRunning() bool {
	funnelRefreshMu.Lock()
	defer funnelRefreshMu.Unlock()
	return funnelRefreshing
}

// TriggerAlignmentFunnelRefresh 异步触发一次漏斗快照刷新（单飞 + recover）。
//
// 为什么要异步：全量现算 ~9min，远超 HTTP WriteTimeout(120s)（P11）。
// 已有任务在跑时返回 (false, domain.ErrFunnelRefreshOngoing) ⇒ handler 映射 409。
func TriggerAlignmentFunnelRefresh() (started bool, err error) {
	funnelRefreshMu.Lock()
	if funnelRefreshing {
		funnelRefreshMu.Unlock()
		return false, domain.ErrFunnelRefreshOngoing
	}
	funnelRefreshing = true
	funnelRefreshMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[epid] 漏斗快照刷新 panic: %v", r)
			}
			funnelRefreshMu.Lock()
			funnelRefreshing = false
			funnelRefreshMu.Unlock()
		}()
		// 后台任务用独立上下文，不受触发请求的生命周期影响
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
		defer cancel()
		s := currentEpidService()
		if s == nil {
			log.Printf("[epid] 漏斗快照刷新失败：派生服务未装配")
			return
		}
		t0 := time.Now()
		rows, all, truncated, cerr := s.repo.ClassAlignmentFunnelWithMeta(ctx)
		if cerr != nil {
			log.Printf("[epid] 漏斗快照计算失败（耗时 %s）: %v", time.Since(t0).Round(time.Second), cerr)
			return
		}
		if serr := s.repo.SaveFunnelSnapshot(ctx, rows, all, truncated); serr != nil {
			log.Printf("[epid] 漏斗快照写入失败: %v", serr)
			return
		}
		log.Printf("[epid] 漏斗快照已更新（耗时 %s）集级=%.2f%% 片级=%.2f%% 槽=%d truncated=%v",
			time.Since(t0).Round(time.Second), all.SlotPct, all.MkPct, all.SlotsTotal, truncated)
	}()
	return true, nil
}

// ReportAlignmentFunnel 分层漏斗读数（管理端按需出口）。
func ReportAlignmentFunnel(ctx context.Context) (FunnelReport, error) {
	out := FunnelReport{
		ByCategory: []mysql.FunnelStat{},
		All:        mysql.FunnelStat{Pid: -1, Category: "全部"},
	}
	s := currentEpidService()
	if s == nil {
		return out, errors.New("epid 派生服务未装配")
	}
	snap, ok, err := s.repo.LoadFunnelSnapshot(ctx)
	if err != nil {
		return out, err
	}
	if !ok {
		// 尚无快照：不阻塞、不现算（现算必超时），提示调用方先 POST 刷新
		out.Pending = true
		return out, nil
	}
	if snap.ByCategory != nil {
		out.ByCategory = snap.ByCategory
	}
	out.All = snap.All
	out.ComputedAt = snap.ComputedAt
	out.PolicyVersion = snap.PolicyVersion
	out.Truncated = snap.Truncated
	return out, nil
}

func ReportEpid(ctx context.Context, full bool) (EpidReport, error) {
	st := DeriveStatus()

	// ⚠️ **null 兜底**（2026-11-11 线上事故）：Go 的 nil map/slice 序列化成 JSON 是 **`null`**，
	// 而前端 `Object.keys(report.byFromSrc)` 对 null 会抛 TypeError ⇒ **整个组件渲染失败、卡片消失** ✗
	// （表现为"点一下刷新指标卡片就没了"，且 production 构建不打 console ⇒ 看着像没报错）。
	// ⇒ 所有 map/slice 一律初始化为**空值**（`{}` / `[]`），从根上不让 JSON 出现 null。
	out := EpidReport{
		Status:            st,
		Deriving:          st.Running,
		ByFromSrc:         map[string]int64{},
		ClassAlignment:    []mysql.ClassAlignmentStat{},
		ClassAlignmentAll: mysql.ClassAlignmentStat{Category: "全部"},
		EmptyByShape:      []mysql.ShapeStat{},
	}
	s := currentEpidService()
	if s == nil {
		return out, nil
	}
	run := func(label string, d time.Duration, fn func(context.Context) error) {
		c, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		if err := fn(c); err != nil {
			if out.MetricsError != "" {
				out.MetricsError += "；"
			}
			out.MetricsError += label + "：" + err.Error()
		}
	}

	// ---- ① 进度集：**任务进行中也给**，且成本与表规模无关 ----
	//
	// 2026-11-11 线上实测（血的教训）：原实现用精确计数当"轻量进度集"，派生表涨到 590 万行后
	//   `COUNT(*)` = **19.2s**（预算 3s）、`COUNT(DISTINCT src_row_id)` = **8.6s**（预算 5s）、
	//   `COUNT(*) movie_play_source` = **5.3s**（表里挂着 1.1GB 的 episodes_json 巨列）
	// ⇒ 三项全部撞穿超时，被"每项独立超时 + 全项容错"逐个降级成 **0** ⇒ 管理员看到 `0 / 0`，
	// 以为数据丢了 ✗✗。
	// 现改为：写侧计数器（O(1) 主键点查）+ 两个后台缓存快照（源行总数 / 影片完整度）+ 内存计数
	// ⇒ 这条路径**不再有任何全表计数**，读数与表规模无关 ✓
	run("进度", 5*time.Second, func(c context.Context) error {
		p, err := buildEpidProgress(c)
		if err != nil {
			return err
		}
		out.Progress = p
		if p != nil && p.CountingAvailable {
			out.DerivedRows = p.DerivedRows
		}
		return nil
	})

	if st.Running {
		out.Deriving = true
		out.OnlyProgress = true
		out.MetricsError = "任务进行中：本轮只算进度类指标（对齐率/分层/成熟度/形态/覆盖率需跑完才有判定意义）"
		return out, nil
	}
	out.Deriving = st.Running // false：空闲（前端据此隐藏进度块、渲染终态指标）

	// ---- ② 重型指标：**只读后台快照**（每条 10~33s，绝不进请求路径）----
	//
	// 实测（2026-11-11，派生表 598 万行）：全库口径 ~32s、≥2 site 子查询 ~33s、按分类 ~22s
	//（派生表全表扫 + 临时表）、成熟片/成熟度 ~10~15s ⇒ 全套近 **2 分钟**（此前网关 524 的根因）。
	// 判断：用户要的是"现在什么状态 / 还差多少进度"，**不是**"每次点刷新都跑一遍全库体检"。
	// ⇒ 一律**后台算、请求读快照**（5 分钟一份），并把**采样时间**一起返回，让"数据多旧"可见 ✓
	hm, at, stale := heavyMetricsSnapshot()
	if at.IsZero() {
		out.MetricsError = "重型指标正在后台计算（每条 10~33s），稍后点「刷新指标」即可"
	} else {
		out.MetricsAt, out.MetricsStale = at.UnixMilli(), stale
		out.DerivedRows, out.EmptyEpidRows = hm.DerivedRows, hm.EmptyEpidRows
		out.AlignedGroups, out.TotalGroups = hm.AlignedGroups, hm.TotalGroups
		out.AlignmentPct = hm.AlignmentPct
		out.ByFromSrc = hm.ByFromSrc
		out.ClassAlignment, out.ClassAlignmentAll = hm.ClassAlignment, hm.ClassAlignmentAll
		out.EmptyByShape = hm.EmptyByShape
		out.MetricsError = hm.MetricsError
	}

	// ---- ③ 覆盖率：**按需**（唯一仍允许在请求里现算的重查询，实测 7~13s，独立 30s 超时兜底）----
	// 它是一次性的"对账"指标（派生行数 / Σ集数），不随进度变化，所以放按需最合适。
	if full {
		run("覆盖率(§6.1)", 30*time.Second, func(c context.Context) error {
			dr, se, err := s.repo.CoverageFull(c)
			if err != nil {
				return err
			}
			if se <= 0 {
				out.CoverageError = "源侧集数为 0（无源行？）"
				return nil
			}
			out.Coverage = &EpidCoverage{DerivedRows: dr, SourceEpisodes: se, Pct: float64(dr) * 100 / float64(se)}
			return nil
		})
	} else {
		out.CoverageError = "按需计算：加 ?full=1 拉取（要扫 Σ JSON_LENGTH 巨列，约 7~13s）"
	}
	return out, nil
}

// DeriveAll 从头扫完一轮（keyset 分页循环）。返回累计统计与"是否仍有被拦下的行"。
// `batch` 每批源行数；`maxBatches<=0` 表示不限批数。
func (s *EpidService) DeriveAll(ctx context.Context, batch int, maxBatches int, force bool) (DeriveStats, error) {
	total := DeriveStats{SkipWhy: map[string]int{}}
	afterId := int64(0)
	for n := 0; maxBatches <= 0 || n < maxBatches; n++ {
		st, err := s.DeriveBatch(ctx, afterId, batch, force)
		if err != nil {
			return total, err
		}
		total.Scanned += st.Scanned
		total.Rebuilt += st.Rebuilt
		total.Rows += st.Rows
		total.Skipped += st.Skipped
		total.EmptyEpid += st.EmptyEpid
		total.OverrideApplied += st.OverrideApplied
		total.OverrideKept += st.OverrideKept
		total.OverrideStale += st.OverrideStale
		for k, v := range st.SkipWhy {
			total.SkipWhy[k] += v
		}
		total.LastId = st.LastId
		if st.Scanned < batch || st.Scanned == 0 {
			break
		}
		afterId = st.LastId
	}
	return total, nil
}

// CleanOrphans 孤儿清理（F-06）：派生表里有、源表已不存在的行 ⇒ 级联删除。
// 返回删除的**源行数**。建议低频跑（整片下架/站点下线/Truncate 之后）。
func (s *EpidService) CleanOrphans(ctx context.Context, limit int) (int, error) {
	orphans, err := s.repo.OrphanSrcRowIds(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(orphans) == 0 {
		return 0, nil
	}
	if _, err := s.repo.DeleteBySrcRowIds(ctx, orphans); err != nil {
		return 0, err
	}
	log.Printf("[epid] 孤儿清理：删除 %d 个已消失源行的派生行", len(orphans))
	return len(orphans), nil
}

// AlignmentReport 全库对齐率报告（P1-1 验收主指标，配 §4.8 门槛）。
type AlignmentReport struct {
	TotalDerivedRows int64
	EmptyEpidRows    int64
	AlignedGroups    int64 // 同 (match_key, epid) 出现在 ≥2 个 site 的组数
	TotalGroups      int64 // 参与统计的非空 epid 组数
	AlignmentPct     float64
	ByFromSrc        map[string]int64
}

func (s *EpidService) AlignmentReport(ctx context.Context) (AlignmentReport, error) {
	var rep AlignmentReport
	total, empty, err := s.repo.Coverage(ctx)
	if err != nil {
		return rep, err
	}
	rep.TotalDerivedRows, rep.EmptyEpidRows = total, empty
	aligned, groups, err := s.repo.AlignmentRate(ctx)
	if err != nil {
		return rep, err
	}
	rep.AlignedGroups, rep.TotalGroups = aligned, groups
	if groups > 0 {
		rep.AlignmentPct = float64(aligned) * 100 / float64(groups)
	}
	rep.ByFromSrc = map[string]int64{}
	stats, err := s.repo.FromSrcStats(ctx)
	if err != nil {
		return rep, err
	}
	for _, x := range stats {
		rep.ByFromSrc[x.FromSrc] = x.N
	}
	return rep, nil
}

// rowFilmName（S-去前缀，2026-11-19）：C 口径取片名 —— 本行 source_name 优先，其次 mid → movie.name。
func rowFilmName(src entity.MoviePlaySource, midNames map[int64]string) string {
	if src.SourceName != nil && *src.SourceName != "" {
		return *src.SourceName
	}
	if src.Mid != nil {
		return midNames[*src.Mid]
	}
	return ""
}
