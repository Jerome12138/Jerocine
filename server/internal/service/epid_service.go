package service

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"server/internal/danmu"
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
}

func NewEpidService(repo *mysql.EpidRepo) *EpidService { return &EpidService{repo: repo} }

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

// ---- U5 / H2：一键全量派生（管理端入口, 2026-11-11）----
//
// 与"采集收尾自动触发一批"的关系：**共用同一把锁**（`deriveRunning`）⇒ 互斥防重入；
// 本全量任务由管理端**显式**发起（补缺 / 清空重建），跑完自己打验收指标日志。
var (
	deriveFullRunning   atomic.Bool   // 是否有全量任务在跑（供状态查询）
	deriveFullStartedAt atomic.Int64  // 全量任务的起始毫秒
	deriveFullMode      atomic.Value  // fill | rebuild
)

// 采集轮次进行中的标记（2026-11-11，需求 3"与采集错峰/让路"）。
//
// 为什么需要：全量派生与采集**都在写同一张库**；两者重叠会推高死锁概率（项目里已记录过
// "主站并发过高会批量死锁"）。原先只与"派生自身的自动触发"互斥，**不与采集互斥** ⇒ 全量重建
// 期间必然撞上 1~3 轮采集。现由采集链路（cron 轮次 起/止）置位，全量派生在**每批前**让路。
var collectRoundRunning atomic.Bool

// SetCollectRoundRunning 由采集链路在"轮次开始/结束"时调用（见 spider_service 的 cron 闭包）。
func SetCollectRoundRunning(on bool) { collectRoundRunning.Store(on) }

// CollectRoundRunning 当前是否有采集轮次在跑。
func CollectRoundRunning() bool { return collectRoundRunning.Load() }

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
		log.Printf("[epid] 派生一批完成: 趟=%s 游标 %d→%d 扫=%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d 来源=%v",
			phaseName, after, deriveCursor.Load(), st.Scanned, st.Rebuilt, st.Rows, st.Skipped, st.EmptyEpid, st.SkipWhy)
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
	Scanned   int            // 本批**扫过**的源行数（== 窗口大小；< limit ⇒ 已到表尾）
	Rebuilt   int            // 实际重建的源行数
	Rows      int64          // 写入的派生行数
	Skipped   int            // 被抖动守卫拦下的源行数
	SkipWhy   map[string]int // 原因分布（suspect-shrink / suspect-disjoint）
	EmptyEpid int64          // 派生行里 epid 为空串的条数（L6：不产身份）
	LastId    int64          // 本批最后一个源行 id（下次 keyset 从这里继续）
}

// derivFlushRows 一次 flush 的派生行上限（批级写入缓冲）：≈几 MB 内存，且把"一次事务"的规模
// 控制在"几十~几百个源行"这一档 —— 对照采集侧的写粒度（200 行/语句、整页一次调用）。
const derivFlushRows = 5000

// DeriveBatch 派生一批脏源行（keyset 分页；`afterId` 传上一批的 `LastId`，从头开始传 0）。
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

	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.Id)
	}
	anchors, err := s.repo.AnchorsBySrcRowIds(ctx, ids)
	if err != nil {
		return st, err
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
			return st, perr
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
	)
	flush := func() error {
		if len(pendingIDs) == 0 {
			return nil
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
					return st, err
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
		out := make([]entity.MovieEpisodeEpid, 0, len(titles))
		for i, t := range titles {
			if derived[i][0] == "" {
				st.EmptyEpid++
			}
			out = append(out, entity.MovieEpisodeEpid{
				SrcRowId:   src.Id,
				Mid:        src.Mid,
				MatchKey:   src.MatchKey,
				SiteId:     src.SiteId,
				PlayFrom:   src.PlayFrom,
				EpIndex:    i,
				Epid:       derived[i][0],
				FromSrc:    derived[i][1],
				RawName:    t,
				SrcEpHash:  src.EpHash,
				ComputedAt: now,
			})
		}
		pendingIDs = append(pendingIDs, src.Id)
		pendingRows = append(pendingRows, out...)
		st.Rebuilt++
		st.Rows += int64(len(out))
		if len(pendingRows) >= derivFlushRows {
			if err := flush(); err != nil {
				return st, err
			}
		}
	}
	if err := flush(); err != nil {
		return st, err
	}
	return st, nil
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
}

// DeriveStatus 读当前派生状态。
func DeriveStatus() DeriveState {
	m := ""
	if v := deriveFullMode.Load(); v != nil {
		m, _ = v.(string)
	}
	return DeriveState{
		Running:   deriveFullRunning.Load(),
		Mode:      m,
		StartedAt: deriveFullStartedAt.Load(),
		Cursor:    deriveCursor.Load(),
		HotPhase:  deriveHotPhase.Load(),
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
			log.Printf("[epid] 全量派生 第%d批: 趟=%s 游标 %d→%d 扫=%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d",
				n+1, phase, after, deriveCursor.Load(), st.Scanned, st.Rebuilt, st.Rows, st.Skipped, st.EmptyEpid)
			// 热门趟扫完 ⇒ 继续跑长尾趟；长尾趟也扫完 ⇒ 整趟结束
			if finished && !runHot {
				break
			}
		}
		log.Printf("[epid] 全量派生结束（mode=%s）: 扫=%d 重建=%d 派生行=%d 守卫跳过=%d 空epid=%d 原因=%v",
			mode, total.Scanned, total.Rebuilt, total.Rows, total.Skipped, total.EmptyEpid, total.SkipWhy)
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

// EpidReport 管理端只读报告：状态 + 验收指标（对齐率/覆盖率/来源分布）。
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
}

// ReportEpid 汇总验收指标（只读）。覆盖率走离线口径（要扫巨列，**不在热路径**）。
func ReportEpid(ctx context.Context) (EpidReport, error) {
	out := EpidReport{Status: DeriveStatus()}
	s := currentEpidService()
	if s == nil {
		return out, nil
	}
	rep, err := s.AlignmentReport(ctx)
	if err != nil {
		return out, err
	}
	out.DerivedRows, out.EmptyEpidRows = rep.TotalDerivedRows, rep.EmptyEpidRows
	out.AlignedGroups, out.TotalGroups = rep.AlignedGroups, rep.TotalGroups
	out.AlignmentPct, out.ByFromSrc = rep.AlignmentPct, rep.ByFromSrc

	dr, se, cerr := s.repo.CoverageFull(ctx)
	switch {
	case cerr != nil:
		out.CoverageError = cerr.Error()
	case se <= 0:
		out.CoverageError = "源侧集数为 0（无源行？）"
	default:
		out.Coverage = &EpidCoverage{DerivedRows: dr, SourceEpisodes: se, Pct: float64(dr) * 100 / float64(se)}
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
