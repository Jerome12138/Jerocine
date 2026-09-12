package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/repository"
	"server/internal/douban"
)

// HotService 榜单热度刷新 —— 把豆瓣"当下热门"落到本地热度列(hot_rank / hot_rank_at / hot_score)。
//
// 方案: docs/榜单热度方案-2026-09-11.md(§3.1 合成分 / §3.3 刷新与失效 / §3.4 三重门匹配)。
//
// # 为什么必须"全量替换"而不是"位次变了才写"
//
// 每轮先把上一轮在榜的行整体回落兜底分, 再写新榜 —— 掉榜的片因此自动归零。
// 若按"位次变化才 UPDATE", 掉榜片没人清位次, 快照会一层层叠成僵尸数据(榜首永远是三个月前的老片)。
//
// # 四层防护, 保证"抓不到"绝不被误读成"不热"
//
//  1. 客户端层: 一次响应必须 HTTP 200 + JSON 含 subject_collection_items 才算有效(见 douban 包);
//  2. 服务层·抓取侧: 任一集合失败 / 首页空返回 / 条目数低到不可能 → **整轮不落库**, 保留上轮快照。
//     全量替换下, "不完整的榜"会被误读成"这些片掉榜了", 比不刷新糟得多;
//  3. 服务层·匹配侧: 抓取"结构合法但语义漂移"时前面几道都拦不住, 但匹配数会塌向个位数 ——
//     新榜比上轮缩水一半以上同样整轮不落库(见 Refresh 里的第四道判据);
//  4. 兜底层: 快照超过 3 天没刷新(豆瓣长期不可用)才主动回落兜底分, 不让作废位次长期霸榜。
//
// 失败退避 1 小时(不与对端死磕, 也不至于整天不重试); 每天最多成功一次。
type HotService struct {
	movie  hotRepo
	client *douban.Client
	cols   []douban.Collection

	mu      sync.Mutex
	nextRun time.Time // 失败退避到期时间(进程内; 跨副本由 Redis 锁收敛)
}

// hotRepo 榜单刷新实际用到的仓储能力 —— 刻意只声明这 4 个方法(依赖倒置收窄),
// repository.MovieRepository 天然满足, 单测可只实现这 4 个。
type hotRepo interface {
	HotCandidatesByDbIds(ctx context.Context, dbIds []int64) ([]repository.HotCandidate, error)
	HotCandidatesByKeyword(ctx context.Context, keyword string, limit int) ([]repository.HotCandidate, error)
	ListHotBoard(ctx context.Context) ([]repository.HotCandidate, error)
	ApplyHot(ctx context.Context, rows []repository.HotRow) (int, error)
}

// 刷新节奏与阈值。
const (
	hotRefreshHour   = 4                // 每日刷新时刻(服务器本地时区, 部署为北京时间)
	hotInitialDelay  = 3 * time.Minute  // 启动后首次检查延迟(避开冷启动抖动)
	hotSweepInterval = 10 * time.Minute // 调度轮询间隔
	hotRetryBackoff  = time.Hour        // 失败后的重试间隔
	hotStaleAfter    = 72 * time.Hour   // 快照超期 → 主动回落兜底分(3 天)
	hotRunLockTTL    = 30 * time.Minute // 刷新互斥锁 TTL(单轮实测 <2 分钟)
	hotDailyLockTTL  = 25 * time.Hour   // 每日一次的成功标记(>24h, 覆盖跨天)
	hotMinBoardItems = 300              // 一轮至少应抓到的条目数(正常约 675); 低于此值判为限流/异常
	hotNameCandLimit = 30               // 片名兜底匹配的候选上限
	hotTitleMaxRunes = 12               // 片名兜底用的检索关键词长度上限(太长反而不召回)
)

// ErrHotBusy 已有一轮榜单刷新在跑(每日调度与后台手动触发撞车)。
var ErrHotBusy = errors.New("榜单刷新正在进行中，请稍后再试")

// NewHotService 构造榜单刷新服务(L1 集合清单取 douban.Collections)。
func NewHotService(movie hotRepo, client *douban.Client) *HotService {
	return &HotService{movie: movie, client: client, cols: douban.Collections}
}

// HotRefreshReport 一轮刷新的观测值 —— 日志 + 后台手动触发的回显。
type HotRefreshReport struct {
	At          int64  `json:"at"`          // 本轮时间(Unix 秒)
	Fetched     int    `json:"fetched"`     // 榜单条目数(含跨榜重复)
	Matched     int    `json:"matched"`     // 匹配到本地影片的部数 = 本轮在榜部数(去重后, 未按"是否落库"区分)
	FellOff     int    `json:"fellOff"`     // 掉榜回落的部数
	DbIdFilled  int    `json:"dbIdFilled"`  // 回填 db_id 的部数
	ScoreFilled int    `json:"scoreFilled"` // 回填 db_score 的部数
	Cleared     int    `json:"cleared"`     // 快照超期被动回落的部数
	Applied     int    `json:"applied"`     // 实际写入行数(movie 表) = Matched + FellOff(未落库时为 0)
	Pages       int    `json:"pages"`       // 有效响应页数
	Empty       int    `json:"empty"`       // 首页空返回次数(限流特征)
	Skipped     int    `json:"skipped"`     // 重试用尽仍失败的页数
	ElapsedMs   int64  `json:"elapsedMs"`
	Err         string `json:"err,omitempty"`
}

// Start 起后台调度循环(阻塞 goroutine, 由组合根 go 出去)。
func (s *HotService) Start(ctx context.Context) {
	if s.client == nil || s.movie == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[hot] 调度器 panic: %v", r)
			}
		}()
		s.loop(ctx)
	}()
}

func (s *HotService) loop(ctx context.Context) {
	if !sleepCtx(ctx, hotInitialDelay) {
		return
	}
	s.tick(ctx)
	ticker := time.NewTicker(hotSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick 到点才刷: 当天没成功跑过(Redis 标记) + 已过刷新时刻 + 退避已到期。
func (s *HotService) tick(ctx context.Context) {
	if !s.due(time.Now()) {
		return
	}
	dayKey := cache.KeyLockHotDaily(time.Now().Format("2006-01-02"))
	token, ok, err := cache.TryLock(ctx, dayKey, hotDailyLockTTL)
	if err != nil {
		log.Printf("[hot] 抢占当日刷新标记失败: %v", err)
		return
	}
	if !ok {
		return // 今天已经成功跑过(或另一副本正在跑)
	}
	rep, err := s.Refresh(ctx)
	if err != nil {
		// 释放当日标记 → 退避后可重试(否则一次网络抖动就整天不再试)。
		cache.Unlock(ctx, dayKey, token)
		s.backoff(time.Now())
		log.Printf("[hot] 刷新失败(保留上轮快照): %v; report=%+v", err, rep)
		return
	}
	s.mu.Lock()
	s.nextRun = time.Time{}
	s.mu.Unlock()
	log.Printf("[hot] %+v", rep)
}

// due 是否到了该刷新的时刻: 已过 04:00 且不在失败退避期内。
// "当天是否已跑过"由 Redis 标记负责(跨副本、跨重启一致), 这里只管时刻。
func (s *HotService) due(now time.Time) bool {
	if now.Hour() < hotRefreshHour {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !now.Before(s.nextRun)
}

func (s *HotService) backoff(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRun = now.Add(hotRetryBackoff)
}

// Refresh 跑一轮刷新。可被每日调度与后台手动触发调用(同一把锁互斥)。
func (s *HotService) Refresh(ctx context.Context) (rep HotRefreshReport, err error) {
	start := time.Now()
	rep.At = start.Unix()
	defer func() {
		rep.ElapsedMs = time.Since(start).Milliseconds()
		if err != nil {
			rep.Err = err.Error()
		}
	}()

	token, ok, lerr := cache.TryLock(ctx, cache.KeyLockHotRun, hotRunLockTTL)
	if lerr != nil {
		return rep, lerr
	}
	if !ok {
		return rep, ErrHotBusy
	}
	defer cache.Unlock(ctx, cache.KeyLockHotRun, token)

	ranked, ferr := s.client.Fetch(ctx, s.cols)
	st := s.client.Stats()
	rep.Pages, rep.Empty, rep.Skipped, rep.Fetched = st.Pages, st.Empty, st.Skipped, len(ranked)

	// 可信性判定(见类型注释第 2 条): 只有"整轮干净"才允许落库。
	if ferr != nil || rep.Skipped > 0 || rep.Empty > 0 || rep.Fetched < hotMinBoardItems {
		if n, _ := s.clearStaleBoard(ctx, start); n > 0 {
			rep.Cleared = n
		}
		return rep, fmt.Errorf("榜单抓取不可信: err=%v fetched=%d skipped=%d empty=%d",
			ferr, rep.Fetched, rep.Skipped, rep.Empty)
	}

	matches, err := s.match(ctx, ranked)
	if err != nil {
		return rep, err
	}
	rep.Matched = len(matches)
	rows, fell, dbIdFilled, scoreFilled, prevBoard, err := s.buildRows(ctx, matches, start)
	if err != nil {
		return rep, err
	}
	rep.FellOff, rep.DbIdFilled, rep.ScoreFilled = fell, dbIdFilled, scoreFilled

	// 第四道判据(相对量, 兜住前面三道抓不到的情形): 响应"结构合法但语义漂移"时
	// (对端换了榜单内容 / 库里 db_id 大面积失效), 匹配数会塌向个位数 —— 全量替换下这等于
	// "把整个榜清空", 比不刷新糟得多。新榜比上轮缩水一半以上即判异常。
	// 只在上一轮有成规模的榜时启用(prevBoard < 10 时样本太小, 不值得据此否决)。
	if len(matches) == 0 || (prevBoard >= 10 && len(matches)*2 < prevBoard) {
		// 仍然走一次超期检查: 若这种异常持续, 72h 后把作废快照回落到兜底分, 不至于永久卡住。
		if n, _ := s.clearStaleBoard(ctx, start); n > 0 {
			rep.Cleared = n
		}
		return rep, fmt.Errorf("榜单匹配数异常萎缩: matched=%d, 上轮在榜=%d", len(matches), prevBoard)
	}
	if rep.Applied, err = s.movie.ApplyHot(ctx, rows); err != nil {
		return rep, err
	}
	// 热度列参与首页/分类页/筛选页排序, 必须主动失效(否则最多滞后一个 TTL)。
	cache.InvalidateAfterCollect(ctx)
	return rep, nil
}

// clearStaleBoard 快照超期(>72h 未刷新)才把在榜片回落到兜底分。
//
// 豆瓣连续多日不可用时, 榜位分就是"作废快照" —— 留着会让几天前的位置一直霸榜,
// 不如回落到"最近上映 + 高分"的兜底序, 至少与站内内容同步变化。
func (s *HotService) clearStaleBoard(ctx context.Context, now time.Time) (int, error) {
	prev, err := s.movie.ListHotBoard(ctx)
	if err != nil {
		log.Printf("[hot] 读取在榜快照失败: %v", err)
		return 0, err
	}
	if len(prev) == 0 {
		return 0, nil
	}
	newest := int64(0)
	for _, p := range prev {
		if p.HotRankAt > newest {
			newest = p.HotRankAt
		}
	}
	if newest > 0 && now.Unix()-newest < int64(hotStaleAfter/time.Second) {
		return 0, nil // 快照还新鲜, 保留(等退避后重试抓取)
	}
	rows := make([]repository.HotRow, 0, len(prev))
	for _, p := range prev {
		rows = append(rows, repository.HotRow{
			Mid:      p.Mid,
			HotScore: domain.HotScore(0, p.Year, p.Remarks, p.DbScore, now),
		})
	}
	n, err := s.movie.ApplyHot(ctx, rows)
	if err != nil {
		return 0, err
	}
	cache.InvalidateAfterCollect(ctx)
	log.Printf("[hot] 榜单快照已超过 %v 未刷新, %d 部片回落到兜底分", hotStaleAfter, n)
	return n, nil
}

// hotMatch 榜单条目 ↔ 本地影片的一对一匹配结果。
type hotMatch struct {
	Mid   int64
	Rank  int    // 榜单内位次(1 起)
	Depth int    // 该榜单深度(与 Rank 一起决定多榜并列时的代表榜)
	Board string // 来源集合名(如 movie_hot_gaia), 详情页展示"哪个榜的 No.X"
	Label string // 集合中文名
	Item  douban.Item
	Local repository.HotCandidate
}

// betterMatch 与 betterBoard 同规则, 作用在 hotMatch 上(buildRows 按 mid 去重时用)。
func betterMatch(a, b hotMatch) hotMatch {
	if a.Rank != b.Rank {
		if a.Rank < b.Rank {
			return a
		}
		return b
	}
	if a.Depth != b.Depth {
		if a.Depth > b.Depth {
			return a
		}
		return b
	}
	if oa, ob := hotBoardOrder[a.Board], hotBoardOrder[b.Board]; oa != ob {
		if oa < ob {
			return a
		}
		return b
	}
	return a
}

// hotBoardOrder 集合名 → Collections 清单序号, 多榜并列时保证选择结果稳定可复现
// (不依赖 map 迭代序)。
var hotBoardOrder = func() map[string]int {
	m := make(map[string]int, len(douban.Collections))
	for i, c := range douban.Collections {
		m[c.Name] = i
	}
	return m
}()

// betterBoard 多榜命中同一部片时选"代表榜": 位次小者优先; 同位次取更深的榜
// (大榜的第 2 名比 10 条小榜的第 2 名含金量高); 再同按 Collections 清单顺序。
func betterBoard(a, b douban.Ranked) douban.Ranked {
	if a.Rank != b.Rank {
		if a.Rank < b.Rank {
			return a
		}
		return b
	}
	if a.Depth != b.Depth {
		if a.Depth > b.Depth {
			return a
		}
		return b
	}
	if oa, ob := hotBoardOrder[a.Collection], hotBoardOrder[b.Collection]; oa != ob {
		if oa < ob {
			return a
		}
		return b
	}
	return a
}

// match 三重门匹配(方案 §3.4):
//  1. db_id 精确命中(首选);
//  2. 归一化片名在本库唯一命中;
//  3. 年份 ±1 且导演/主演有交集。
//
// 任一不满足 → 该条只跳过, 绝不猜(猜错的代价是给不相干的片挂上榜位与豆瓣分)。
func (s *HotService) match(ctx context.Context, ranked []douban.Ranked) ([]hotMatch, error) {
	best := make(map[int64]douban.Ranked, len(ranked))
	noID := make([]douban.Ranked, 0)
	for _, r := range ranked {
		if r.Id <= 0 {
			noID = append(noID, r)
			continue
		}
		if cur, ok := best[r.Id]; !ok {
			best[r.Id] = r
		} else {
			best[r.Id] = betterBoard(cur, r) // 命中多榜取代表榜(见 betterBoard)
		}
	}

	ids := make([]int64, 0, len(best))
	for id := range best {
		ids = append(ids, id)
	}
	cands, err := s.movie.HotCandidatesByDbIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	byDbId := make(map[int64][]repository.HotCandidate, len(cands))
	for _, c := range cands {
		if c.DbId > 0 {
			byDbId[c.DbId] = append(byDbId[c.DbId], c)
		}
	}

	out := make([]hotMatch, 0, len(best))
	unmatched := make([]douban.Ranked, 0, len(noID))
	for id, r := range best {
		group := byDbId[id]
		if len(group) == 0 {
			unmatched = append(unmatched, r)
			continue
		}
		local, ok := pickByDbId(r.Item, group)
		if !ok {
			continue
		}
		if strings.Contains(local.Name, "解说") {
			// 组内**全是**解说行(线上实测: 源站把正片的 db_id 填到解说行上, 正片行缺 db_id)
			// —— db_id 精确只证明"源站这么标的", 证明不了这行是本体, 直接挂会让
			// 「杀死比尔：血色全传[电影解说]」站上热度榜首。降级走片名兜底:
			// 库里有正片就挂正片并回填 db_id, 没有就丢弃(绝不挂解说)。
			unmatched = append(unmatched, r)
			continue
		}
		out = append(out, hotMatch{Mid: local.Mid, Rank: r.Rank, Depth: r.Depth, Board: r.Collection, Label: r.Label, Item: r.Item, Local: local})
	}
	unmatched = append(unmatched, noID...)

	// 兜底: 片名归一化唯一命中 + 年份 ±1 且编导有交集。
	// 逐条一次 FULLTEXT 检索, 单日候选量在百级, 成本可忽略。
	for _, r := range unmatched {
		if ctx.Err() != nil {
			break
		}
		kw := hotKeyword(r.Item.Title)
		if kw == "" {
			continue
		}
		cands, err := s.movie.HotCandidatesByKeyword(ctx, kw, hotNameCandLimit)
		if err != nil {
			log.Printf("[hot] 片名候选检索失败 %q: %v", kw, err)
			continue
		}
		local, ok := pickNameMatch(r.Item, cands)
		if !ok {
			continue
		}
		out = append(out, hotMatch{Mid: local.Mid, Rank: r.Rank, Depth: r.Depth, Board: r.Collection, Label: r.Label, Item: r.Item, Local: local})
	}
	return out, nil
}

// pickByDbId 在同一个 db_id 的多行里挑"本体"。
//
// 库里实测 16,755 组重复 db_id(多出 18,768 行), 绝大多数是"正片 + 电影解说"配对,
// **而豆瓣分往往落在解说行上** —— 实测「漫长的季节」正片(58589) db_score=0、解说行(65268)=9.4。
// 所以择本体这一步直接决定榜单挂到哪一行。判据(优先级从高到低):
// 片名归一化后与豆瓣标题一致 → 片名不含"解说" → mid 最小(调用方已按 mid 升序, 稳定可复现)。
func pickByDbId(it douban.Item, group []repository.HotCandidate) (repository.HotCandidate, bool) {
	if len(group) == 0 {
		return repository.HotCandidate{}, false
	}
	best, bestKey := group[0], hotPickKey(group[0], it.Title)
	for _, c := range group[1:] {
		if k := hotPickKey(c, it.Title); k < bestKey {
			best, bestKey = c, k
		}
	}
	return best, true
}

// hotPickKey 择优键: 0 = 片名与豆瓣标题一致且非解说 / 1 = 非解说 / 2 = 解说。
func hotPickKey(c repository.HotCandidate, title string) int {
	if strings.Contains(c.Name, "解说") {
		return 2
	}
	if want := domain.NormalizeName(title); want != "" && domain.NormalizeName(c.Name) == want {
		return 0
	}
	return 1
}

// pickNameMatch 兜底匹配: 归一化片名在候选里**唯一命中**, 且年份 ±1 且(导演或主演有交集)。
// 一对多一律不猜(同名不同片在采集站很常见, 错了会把榜位挂到不相干的片上)。
func pickNameMatch(it douban.Item, cands []repository.HotCandidate) (repository.HotCandidate, bool) {
	want := domain.NormalizeName(it.Title)
	if want == "" {
		return repository.HotCandidate{}, false
	}
	var hits []repository.HotCandidate
	for _, c := range cands {
		if domain.NormalizeName(c.Name) == want {
			hits = append(hits, c)
		}
	}
	if len(hits) != 1 {
		return repository.HotCandidate{}, false
	}
	c := hits[0]
	dy, y := it.YearInt(), c.Year
	if dy <= 0 || y <= 0 || absInt(dy-y) > 1 {
		return repository.HotCandidate{}, false
	}
	people := it.Names()
	if len(people) == 0 || !hotPeopleOverlap(people, c.Director, c.Actor) {
		return repository.HotCandidate{}, false
	}
	return c, true
}

// hotPeopleOverlap 本地 actor/director(逗号分隔的姓名串)里是否出现豆瓣给出的姓名。
// 两侧都没数据时返回 false —— 无法校验就不认(防同名不同片)。
func hotPeopleOverlap(people []string, fields ...string) bool {
	blob := strings.Join(fields, ",")
	if strings.TrimSpace(blob) == "" {
		return false
	}
	for _, p := range people {
		if p = strings.TrimSpace(p); p != "" && strings.Contains(blob, p) {
			return true
		}
	}
	return false
}

// hotKeyword 片名兜底检索的关键词: 太长的标题截断(ngram 全文检索里长句反而召回差)。
func hotKeyword(title string) string {
	title = strings.TrimSpace(title)
	r := []rune(title)
	if len(r) > hotTitleMaxRunes {
		return string(r[:hotTitleMaxRunes])
	}
	return title
}

// buildRows 组装本轮要写的行:
//   - 在榜片: 榜位分 + 兜底分(榜单 rating 可顺带补本地为 0 的 db_score);
//   - 掉榜片: hot_rank / hot_rank_at 归 0, hot_score 回落到兜底分。
//
// 第 5 个返回值是上一轮的在榜部数 —— 调用方据此判断"新榜是否异常萎缩"(第四道可信性判据)。
func (s *HotService) buildRows(ctx context.Context, matches []hotMatch, now time.Time) (
	rows []repository.HotRow, fellOff, dbIdFilled, scoreFilled, prevBoard int, err error,
) {
	byMid := make(map[int64]hotMatch, len(matches))
	for _, m := range matches {
		if cur, ok := byMid[m.Mid]; !ok {
			byMid[m.Mid] = m
		} else {
			byMid[m.Mid] = betterMatch(cur, m)
		}
	}
	prev, err := s.movie.ListHotBoard(ctx)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	prevBoard = len(prev)

	nowUnix := now.Unix()
	rows = make([]repository.HotRow, 0, len(byMid)+len(prev))
	for _, m := range byMid {
		row := repository.HotRow{Mid: m.Mid, HotRank: m.Rank, HotRankAt: nowUnix, HotBoard: m.Board}
		score := m.Local.DbScore
		if m.Local.DbId <= 0 && m.Item.Id > 0 {
			row.DbId = m.Item.Id // 源站覆盖 59.2% → 剩下的靠榜单 id 精确回填
			dbIdFilled++
		}
		if score <= 0 && m.Item.Rating.Value > 0 {
			row.DbScore = m.Item.Rating.Value // 只在本地为 0 时补, 不覆盖源站给的豆瓣分
			score = m.Item.Rating.Value
			scoreFilled++
		}
		row.HotScore = domain.HotScore(m.Rank, m.Local.Year, m.Local.Remarks, score, now)
		rows = append(rows, row)
	}
	for _, p := range prev {
		if _, onBoard := byMid[p.Mid]; onBoard {
			continue
		}
		rows = append(rows, repository.HotRow{
			Mid:      p.Mid,
			HotScore: domain.HotScore(0, p.Year, p.Remarks, p.DbScore, now),
		})
		fellOff++
	}
	return rows, fellOff, dbIdFilled, scoreFilled, prevBoard, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
