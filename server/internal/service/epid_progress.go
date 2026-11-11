package service

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"server/internal/repository/mysql"
)

// 进度读数层（2026-11-11 "卡片读数全是 0" 事故的修法）。
//
// 事故回顾：短路期（重建/补缺进行中）只算"轻量进度集"，但那三项在派生表涨到 **590 万行**后
// 全部撞穿各自的预算 —— `COUNT(*)` 19.2s（预算 3s）、`COUNT(DISTINCT src_row_id)` 8.6s（预算 5s）、
// `COUNT(*) movie_play_source` 5.3s ⇒ 被"每项独立超时 + 全项容错"逐个降级成 0
// ⇒ 管理员看到 `0 / 0`、`0.0%`，误以为数据丢了 ✗。
//
// 修法（本文件）：
//  1. **本次任务的内存计数**（扫过/重建/批次）—— 任务自己就知道，0 成本；
//  2. **全库计数走写侧计数器**（`epid_derive_stat` 单行表，O(1) 点查；见 repo 层与迁移 000048）；
//  3. **源行总数**（`COUNT(*)` 5.3s）与**影片完整度**（10~14s）—— 它俩几乎不变/慢，
//     一律走**后台快照 + 进程内缓存**，请求路径只读快照 ⇒ 永不阻塞、永不超时 ✓。
//
// 铁律：`buildEpidProgress` **只在内存与缓存上工作**，除了一次 O(1) 的计数器点查之外不碰任何大表。

// ---- 1) 本次全量任务的内存计数 ----
//
// 由 `RunFullDerive` 在每批后累加；任务结束不清零（保留"上一轮跑了多少"的印象分），
// 下次启动时重置。**重启即失**是刻意的：不新增持久状态。
var (
	deriveFullScanned atomic.Int64 // 累计**扫过**的源行数（keyset 窗口大小之和）
	deriveFullRebuilt atomic.Int64 // 累计**重建**的源行数
	deriveFullRows    atomic.Int64 // 累计写入的派生行数
	deriveFullBatches atomic.Int64 // 累计批数
)

// ResetDeriveRunCounters 重置本轮内存计数（`RunFullDerive` 启动时调用）。
func ResetDeriveRunCounters() {
	deriveFullScanned.Store(0)
	deriveFullRebuilt.Store(0)
	deriveFullRows.Store(0)
	deriveFullBatches.Store(0)
}

// ---- 2) 源行总数快照（`COUNT(*)` 实测 5.3s）----
//
// 为什么慢：`movie_play_source` 挂着 1.1GB 的 `episodes_json` 巨列，`COUNT(*)` 要扫聚簇索引页。
// 为什么可以缓存：它是"分母"，只在增删影片源时变化 ⇒ 1 小时 TTL 足够，且失败时退回上次值。
const srcRowTotalTTL = time.Hour

var (
	srcRowTotalMu      sync.Mutex
	srcRowTotalVal     int64
	srcRowTotalAt      time.Time
	srcRowTotalLoading bool
)

// srcRowTotalSnapshot 读源行总数快照；过期则**后台**刷新（不阻塞调用方）。
//
// 不接收 ctx：刷新有**自己的后台上下文**（调用方多在请求里，请求 ctx 一返回就取消 ⇒ 用它必然半途而废 ✗）。
func srcRowTotalSnapshot() int64 {
	srcRowTotalMu.Lock()
	val, at := srcRowTotalVal, srcRowTotalAt
	fresh := !at.IsZero() && time.Since(at) < srcRowTotalTTL
	needKick := !fresh && !srcRowTotalLoading
	if needKick {
		srcRowTotalLoading = true
	}
	srcRowTotalMu.Unlock()

	if needKick {
		s := currentEpidService()
		if s == nil {
			srcRowTotalMu.Lock()
			srcRowTotalLoading = false
			srcRowTotalMu.Unlock()
		} else {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[epid] 源行总数刷新 panic 已拦截: %v", r)
					}
					srcRowTotalMu.Lock()
					srcRowTotalLoading = false
					srcRowTotalMu.Unlock()
				}()
				bg, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				n, err := s.repo.SourceRowCount(bg)
				if err != nil {
					log.Printf("[epid] 源行总数刷新失败（保留上次值）: %v", err)
					return
				}
				srcRowTotalMu.Lock()
				srcRowTotalVal, srcRowTotalAt = n, time.Now()
				srcRowTotalMu.Unlock()
			}()
		}
	}
	return val
}

// ---- 3) 影片完整度快照（实测 10.1~13.1s）----

const (
	movieStatTTL        = 60 * time.Second // 任务进行中：60s 一份足够（用户要的是"跑到哪了"）
	movieStatRetryFloor = 30 * time.Second // 刷新失败后的最小重试间隔，避免失败时被高频重试打爆
)

var (
	movieStatMu        sync.Mutex
	movieStatVal       mysql.MovieCompletenessStat
	movieStatAt        time.Time
	movieStatAttemptAt time.Time
	movieStatLoading   bool
)

// 计数器自愈的单飞开关（见 reconcileDeriveStat）。
var (
	reconcileMu      sync.Mutex
	reconcileLoading bool
)

// movieCompletenessSnapshot 读影片完整度快照；过期则**后台**刷新。不接收 ctx（理由同上）。
// 返回 (快照, 采样时间, 是否陈旧)。`采样时间` 为零 ⇒ 还没有任何快照（调用方显示"计算中"）。
func movieCompletenessSnapshot() (mysql.MovieCompletenessStat, time.Time, bool) {
	movieStatMu.Lock()
	val, at := movieStatVal, movieStatAt
	fresh := !at.IsZero() && time.Since(at) < movieStatTTL
	canRetry := movieStatAttemptAt.IsZero() || time.Since(movieStatAttemptAt) >= movieStatRetryFloor
	needKick := !fresh && !movieStatLoading && canRetry
	if needKick {
		movieStatLoading = true
		movieStatAttemptAt = time.Now()
	}
	movieStatMu.Unlock()

	if needKick {
		s := currentEpidService()
		if s == nil {
			movieStatMu.Lock()
			movieStatLoading = false
			movieStatMu.Unlock()
		} else {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[epid] 影片完整度刷新 panic 已拦截: %v", r)
					}
					movieStatMu.Lock()
					movieStatLoading = false
					movieStatMu.Unlock()
				}()
				bg, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				st, err := s.repo.MovieCompleteness(bg)
				if err != nil {
					log.Printf("[epid] 影片完整度刷新失败（保留上次快照）: %v", err)
					return
				}
				movieStatMu.Lock()
				movieStatVal, movieStatAt = st, time.Now()
				movieStatMu.Unlock()
			}()
		}
	}
	return val, at, !at.IsZero() && !fresh
}

// ---- 4) 进度集组装 ----

// EpidProgress 进度集：**任务进行中也给**，且成本与表规模无关（O(1) 计数 + 缓存快照 + 内存计数）。
type EpidProgress struct {
	// 全库计数（写侧计数器 O(1)）。`CountingAvailable=false` ⇒ 计数器未初始化且兜底计数超时 ⇒ 别当 0 看。
	CountingAvailable bool  `json:"countingAvailable"`
	DerivedRows       int64 `json:"derivedRows"`
	DerivedSrcRows    int64 `json:"derivedSrcRows"`
	SrcRows           int64 `json:"srcRows"`
	RemainSrcRows     int64 `json:"remainSrcRows"` // = SrcRows − DerivedSrcRows（"还缺多少源行"）

	// 本次任务的内存计数（无任务时全 0）
	Scanned     int64   `json:"scanned"`
	Rebuilt     int64   `json:"rebuilt"`
	Batches     int64   `json:"batches"`
	ElapsedSec  int64   `json:"elapsedSec"`
	ScanRate    float64 `json:"scanRate"`    // 扫过源行/秒
	RebuildRate float64 `json:"rebuildRate"` // 重建源行/秒
	EtaSec      int64   `json:"etaSec"`      // 按 RebuildRate 粗估的剩余秒数（0 = 估不出；口径为**下界**）

	// 影片完整度（后台 60s 快照）—— 进度主口径
	MovieComplete  int64 `json:"movieComplete"`
	MovieTotal     int64 `json:"movieTotal"`
	MissingSrcRows int64 `json:"missingSrcRows"` // 不完整影片里一共还缺多少源行
	MovieStatAt    int64 `json:"movieStatAt"`    // 快照采样时间（毫秒；0 = 还没算出来）
	MovieStatStale bool  `json:"movieStatStale"` // true = 后台正在刷新，展示的是上一份快照
}

// buildEpidProgress 组装进度集。
//
// 只做：① 一次 O(1) 计数器点查；② 读两个缓存快照；③ 读内存计数。
// **绝不**在这条路径上做任何全表计数（那是本次事故的根因）。
// 唯一可能慢的分支是"计数器未初始化时的兜底精确计数"——由调用方的独立超时兜住（失败只降级本项）。
func buildEpidProgress(ctx context.Context) (*EpidProgress, error) {
	s := currentEpidService()
	if s == nil {
		return nil, nil
	}
	p := &EpidProgress{}

	// ① 派生行数 / 已派生源行：读写侧计数器（O(1)）
	st, err := s.repo.DeriveStatFast(ctx)
	if err != nil {
		return nil, err
	}
	if st.UpdatedAt > 0 {
		p.CountingAvailable = true
		p.DerivedRows, p.DerivedSrcRows = st.DerivedRows, st.DerivedSrcRows
	} else {
		// 兜底（迁移未跑 / 空库 / 被手工清过）：退回精确计数。表大时这里会超时 ⇒ 本项整体降级
		//（并**在后台**把计数器补上，见 reconcileDeriveStat），绝不长期依赖它。
		// 自愈是**异步**的：这一次请求仍然可能降级，但下一个请求就走 O(1) 了 ✓
		go reconcileDeriveStat()
		dr, e1 := s.repo.CountDerivedRows(ctx)
		ds, e2 := s.repo.DerivedSourceRowCount(ctx)
		if e1 != nil || e2 != nil {
			return nil, e1
		}
		p.CountingAvailable = true
		p.DerivedRows, p.DerivedSrcRows = dr, ds
	}

	// ② 源行总数：读 1h 缓存快照（首次未就绪时为 0 ⇒ RemainSrcRows 暂不可给）
	p.SrcRows = srcRowTotalSnapshot()
	if p.SrcRows > 0 {
		p.RemainSrcRows = p.SrcRows - p.DerivedSrcRows
		if p.RemainSrcRows < 0 {
			p.RemainSrcRows = 0
		}
	}

	// ③ 本次任务的速率 / 预计剩余（内存）
	ds := DeriveStatus()
	p.Scanned, p.Rebuilt, p.Batches = ds.Scanned, ds.Rebuilt, ds.Batches
	p.ElapsedSec = ds.ElapsedSec
	if p.ElapsedSec > 0 {
		p.ScanRate = float64(p.Scanned) / float64(p.ElapsedSec)
		p.RebuildRate = float64(p.Rebuilt) / float64(p.ElapsedSec)
		if p.RebuildRate > 0 && p.RemainSrcRows > 0 {
			p.EtaSec = int64(float64(p.RemainSrcRows)/p.RebuildRate) + 1
		}
	}

	// ④ 影片完整度：读后台快照（过期只触发后台刷新，本请求拿到的是上一份）
	ms, at, stale := movieCompletenessSnapshot()
	if !at.IsZero() {
		p.MovieComplete, p.MovieTotal, p.MissingSrcRows = ms.CompleteMovies, ms.Movies, ms.MissingSrcRows
		p.MovieStatAt = at.UnixMilli()
	}
	p.MovieStatStale = stale
	return p, nil
}

// WarmProgressSnapshots 预热两个缓存快照（`RunFullDerive` 启动时调一次）。
//
// 目的：任务一开始就把"源行总数"和"影片完整度基线"备好 ⇒ 卡片立刻能显示"还缺多少 / 从哪开始"，
// 而不是等用户第一次点刷新才开始算（首次请求会白等一轮）。
func WarmProgressSnapshots() {
	s := currentEpidService()
	if s == nil {
		return
	}
	srcRowTotalSnapshot()
	movieCompletenessSnapshot()
}

// RefreshMovieCompleteness 同步刷新一次影片完整度快照（任务收尾时调用，让空闲态读数立刻是新值）。
func RefreshMovieCompleteness(ctx context.Context) (mysql.MovieCompletenessStat, bool) {
	s := currentEpidService()
	if s == nil {
		return mysql.MovieCompletenessStat{}, false
	}
	st, err := s.repo.MovieCompleteness(ctx)
	if err != nil {
		log.Printf("[epid] 影片完整度收尾刷新失败: %v", err)
		return mysql.MovieCompletenessStat{}, false
	}
	movieStatMu.Lock()
	movieStatVal, movieStatAt = st, time.Now()
	movieStatMu.Unlock()
	return st, true
}

// reconcileDeriveStat 计数器自愈：当计数器未初始化（`updated_at = 0`）时，用一次精确计数把它补上。
//
// 什么时候会未初始化：迁移 000048 之前的老库、被手工清过的库、或迁移里的基线校准失败。
// 为什么必须自愈而不能靠请求路径兜底：兜底那次精确计数要 ~20s（就是本次事故的根因）✗。
//
// 三个必须：① **单飞**（并发请求只起一个）；② **自己的后台上下文**（请求的 ctx 在响应返回时就被
// 取消了，用它做 20s 的计数必然半途而废 ✗）；③ **recover**（未捕获 panic 会终止整个进程）。
func reconcileDeriveStat() {
	reconcileMu.Lock()
	if reconcileLoading {
		reconcileMu.Unlock()
		return
	}
	reconcileLoading = true
	reconcileMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[epid] 计数器自愈 panic 已拦截: %v", r)
		}
		reconcileMu.Lock()
		reconcileLoading = false
		reconcileMu.Unlock()
	}()

	s := currentEpidService()
	if s == nil {
		return
	}
	bg, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	st, err := s.repo.DeriveStatFast(bg)
	if err == nil && st.UpdatedAt > 0 {
		return // 期间已被别的路径补上
	}
	n, err := s.repo.RecountDeriveStat(bg)
	if err != nil {
		log.Printf("[epid] 计数器自愈失败（下次请求再试）: %v", err)
		return
	}
	log.Printf("[epid] 计数器已自愈（精确计数）: 派生行=%d 已派生源行=%d", n.DerivedRows, n.DerivedSrcRows)
}

// ---- 5) 重型指标快照（T2：单条 10~33s，**绝不能进请求路径**）----
//
// 2026-11-11 真库实测（派生表 598 万行、且任务同时在跑 ⇒ 含并发写竞争，偏慢）：
//
//	全库口径对齐率（GROUP BY match_key,epid + COUNT(DISTINCT site_id)） ≈ 32s（已吃 000046 覆盖索引，index-only）
//	≥2 site 子查询                                                      ≈ 33s（**仍回表** ⇒ 等 000047 索引）
//	按分类有身份率                                                      ≈ 22s（派生表 **全表扫 + Using temporary** ✗）
//	成熟片 / 影片完整度 / 成熟度                                        ≈ 15s / 10s / 10s
//
// ⇒ 全套加起来近 **2 分钟** —— 正是此前网关 524 的根因，也是"卡片全是 0"那次事故的同一根源。
// 关键判断：**它们本来就不该进请求路径**。用户要的是"现在什么状态 / 还差多少"，
// 不是"每次点刷新都跑一遍全库体检" ⇒ 一律**后台算、请求读快照**（5 分钟一份，并给出采样时间）。
const (
	heavyStatTTL        = 5 * time.Minute
	heavyStatRetryFloor = time.Minute
)

// heavyMetrics 重型指标的一份快照。
type heavyMetrics struct {
	MetricsError      string
	DerivedRows       int64
	EmptyEpidRows     int64
	AlignedGroups     int64
	TotalGroups       int64
	AlignmentPct      float64
	ByFromSrc         map[string]int64
	ComparableAligned int64
	ComparableTotal   int64
	ComparablePct     float64
	Maturity          []mysql.MaturityStat
	ByCategory        []mysql.CategoryStat
	EmptyByShape      []mysql.ShapeStat
}

var (
	heavyMu        sync.Mutex
	heavyVal       heavyMetrics
	heavyAt        time.Time
	heavyLoading   bool
	heavyAttemptAt time.Time
)

// heavyMetricsSnapshot 读重型指标快照；过期则触发后台刷新（**本请求只拿上一份**）。
// 返回 (快照, 采样时间, 是否陈旧)；采样时间为零 ⇒ 从未算过（调用方应显示"后台计算中"）。
func heavyMetricsSnapshot() (heavyMetrics, time.Time, bool) {
	heavyMu.Lock()
	val, at := heavyVal, heavyAt
	heavyMu.Unlock()
	fresh := !at.IsZero() && time.Since(at) < heavyStatTTL
	if !fresh {
		KickHeavyMetricsRefresh()
	}
	return val, at, !at.IsZero() && !fresh
}

// KickHeavyMetricsRefresh 触发一次**后台**重型指标刷新（单飞 + 失败重试下限 + recover）。
func KickHeavyMetricsRefresh() {
	heavyMu.Lock()
	canRetry := heavyAttemptAt.IsZero() || time.Since(heavyAttemptAt) >= heavyStatRetryFloor
	if heavyLoading || !canRetry {
		heavyMu.Unlock()
		return
	}
	heavyLoading = true
	heavyAttemptAt = time.Now()
	heavyMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[epid] 重型指标刷新 panic 已拦截: %v", r)
			}
			heavyMu.Lock()
			heavyLoading = false
			heavyMu.Unlock()
		}()
		if currentEpidService() == nil {
			return
		}
		bg, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		hm := refreshHeavyMetrics(bg)
		heavyMu.Lock()
		heavyVal, heavyAt = hm, time.Now()
		heavyMu.Unlock()
		if hm.MetricsError != "" {
			log.Printf("[epid] 重型指标快照已刷新（部分失败）: %s", hm.MetricsError)
		} else {
			log.Printf("[epid] 重型指标快照已刷新: 派生行=%d 对齐组=%d/%d (%.1f%%) 可比=%d/%d 覆盖率按需",
				hm.DerivedRows, hm.AlignedGroups, hm.TotalGroups, hm.AlignmentPct, hm.ComparableAligned, hm.ComparableTotal)
		}
	}()
}

// refreshHeavyMetrics 同步算一遍重型指标（**只在后台调用**）。
// 每项独立超时（后台可以给得比接口宽），任一项失败只记进 `MetricsError`、不整块丢弃。
func refreshHeavyMetrics(ctx context.Context) heavyMetrics {
	hm := heavyMetrics{
		ByFromSrc:    map[string]int64{},
		Maturity:     []mysql.MaturityStat{},
		ByCategory:   []mysql.CategoryStat{},
		EmptyByShape: []mysql.ShapeStat{},
	}
	s := currentEpidService()
	if s == nil {
		hm.MetricsError = "派生服务未装配"
		return hm
	}
	each := func(label string, d time.Duration, fn func(context.Context) error) {
		c, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		if err := fn(c); err != nil {
			if hm.MetricsError != "" {
				hm.MetricsError += "；"
			}
			hm.MetricsError += label + "：" + err.Error()
		}
	}
	each("全库口径", 90*time.Second, func(c context.Context) error {
		rep, err := s.AlignmentReport(c)
		if err != nil {
			return err
		}
		hm.DerivedRows, hm.EmptyEpidRows = rep.TotalDerivedRows, rep.EmptyEpidRows
		hm.AlignedGroups, hm.TotalGroups = rep.AlignedGroups, rep.TotalGroups
		hm.AlignmentPct = rep.AlignmentPct
		if rep.ByFromSrc != nil {
			hm.ByFromSrc = rep.ByFromSrc
		}
		return nil
	})
	each("可比口径对齐率", 120*time.Second, func(c context.Context) error {
		a, t, err := s.repo.AlignmentRateComparable(c)
		if err != nil {
			return err
		}
		hm.ComparableAligned, hm.ComparableTotal = a, t
		if t > 0 {
			hm.ComparablePct = float64(a) * 100 / float64(t)
		}
		return nil
	})
	each("按分类有身份率", 90*time.Second, func(c context.Context) error {
		v, err := s.repo.ByCategory(c)
		if err != nil {
			return err
		}
		if v != nil {
			hm.ByCategory = v
		}
		return nil
	})
	each("源行成熟度", 90*time.Second, func(c context.Context) error {
		v, err := s.repo.SourceRowMaturity(c)
		if err != nil {
			return err
		}
		if v != nil {
			hm.Maturity = v
		}
		return nil
	})
	each("空 epid 形态", 60*time.Second, func(c context.Context) error {
		v, err := s.repo.EmptyEpidByShape(c)
		if err != nil {
			return err
		}
		if v != nil {
			hm.EmptyByShape = v
		}
		return nil
	})
	return hm
}
