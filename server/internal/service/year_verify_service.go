package service

import (
	"context"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"server/internal/domain/repository"
	"server/internal/tmdb"
)

// YearVerifyService 年份脏值**异步核实** worker（2026-11-25，第 3 层）。
//
// 背景：源站偶发脏年（2028/2030/9999…）会破坏「最新上线」的真实性。前两层已做
// 排序钳制与写入三分支；本层负责把**库里已存的**越界年份逐条按片名到 TMDB 核实：
//   - 命中且年份合理（>0 且 <= 今年）→ 回填真实年份
//   - 查不到 / 结果仍是未来年      → 判定为脏年，置 0（年份未知）
//
// 自限且幂等：置 0 后该片自动移出候选集，无需建表、无需退避表；
// 网络/限流类错误**本轮跳过、不置 0**（避免把好数据误清零）。
// 与 backdrop worker 同款：无 key 时整体 no-op，事后配 key 下一轮自动生效。
type YearVerifyService struct {
	movie  repository.MovieRepository
	client *tmdb.Client
	kick   chan struct{}
	batch  int
}

const (
	yearVerifySweepInterval = 30 * time.Minute
	yearVerifyBatch         = 100
)

// NewYearVerifyService 构造。client 为 nil 时 Start 直接返回（功能整体关闭）。
func NewYearVerifyService(movie repository.MovieRepository, client *tmdb.Client) *YearVerifyService {
	return &YearVerifyService{movie: movie, client: client, kick: make(chan struct{}, 1), batch: yearVerifyBatch}
}

// Kick 事件触发一轮扫描（非阻塞且幂等：已有待处理触发时合并为一次）。
func (s *YearVerifyService) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Start 起后台循环（阻塞 goroutine，由组合根 go 出去）。
func (s *YearVerifyService) Start(ctx context.Context) {
	if s.client == nil || s.movie == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[year-verify] worker panic: %v", r)
			}
		}()
		s.loop(ctx)
	}()
}

func (s *YearVerifyService) loop(ctx context.Context) {
	s.tick(ctx) // 启动即跑一轮
	sweep := time.NewTicker(yearVerifySweepInterval)
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.kick:
		case <-sweep.C:
		}
		runSafely("[year-verify]", func() { s.tick(ctx) })
	}
}

// tick 一轮：取越界候选 → 逐条按片名核实 → 仅在**同名命中且年份不同**时纠正。
//
// ⚠️ 实测（2026-11-25）修正了本层策略，两条守卫必须同时成立才写库：
//
//	① **同名才采信**：TMDB 中文名检索会误命中（实测「勇者时代」→「勇敢者的预算超支时代」2017），
//	   不做同名校验会直接污染数据；
//	② **查不到就不动**：中文片 TMDB 覆盖率低（实测 4 个真实候选 3 个 NO_RESULT），
//	   原先"查不到就置 0"会抹掉**正确数据**（实测「流浪地球3」在 TMDB 就是 2027-02-06，真实待映）。
//	   宁缺勿错 ⇒ 无法证实一律保持原值。
//
// 叠加"未来年份不参与「最新上线」排序"（第 1 层）后，真实性问题已由排序侧解决；
// 本层只负责用外部权威**纠正确凿的错误年份**。
func (s *YearVerifyService) tick(ctx context.Context) {
	rows, err := s.movie.ListYearSuspicious(ctx, s.batch)
	if err != nil {
		log.Printf("[year-verify] 取脏年候选失败: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	thisYear := time.Now().Year()
	fixed, unchanged, untouched := 0, 0, 0
	for _, m := range rows {
		if ctx.Err() != nil {
			return
		}
		query := yearVerifyQueryName(m.Name)
		if query == "" {
			untouched++
			continue
		}
		y, title, serr := s.client.SearchYearNamed(ctx, query)
		if serr != nil || y <= 0 || !yearVerifySameName(m.Name, title) {
			untouched++ // 网络错误 / 无命中 / 误命中：一律不动
			continue
		}
		// ⚠️ P1 修复（2026-11-25 审查）：SetYear 是**直接 UPDATE**，不走 yearUpsertExpr 的守卫，
		// 故必须在此再守一道，否则 TMDB 给出未来年会**绕过**第 1/2 层建立的钳制。
		// 取"<= 今年"的保守口径：未来年份属于"待映"，写进库没有业务意义，宁可不动。
		if y < 1900 || y > thisYear {
			untouched++
			continue
		}
		if y == m.Year {
			unchanged++ // 外部权威证实现值正确
			continue
		}
		if e := s.movie.SetYear(ctx, m.Mid, y); e == nil {
			fixed++
		}
	}
	log.Printf("[year-verify] 候选 %d：纠正 %d（同名命中且年份不同），证实 %d，未采信 %d",
		len(rows), fixed, unchanged, untouched)
}

// yearVerifyQueryName 去掉 [预告片]/[电影解说] 这类方括号后缀再检索（提高 TMDB 命中率）。
func yearVerifyQueryName(name string) string {
	name = strings.TrimSpace(name)
	for {
		i := strings.LastIndex(name, "[")
		if i < 0 || !strings.HasSuffix(name, "]") {
			break
		}
		name = strings.TrimSpace(name[:i])
	}
	return name
}

// yearVerifySameName 同名判定：规范化后相等或互相包含。
// 用于挡住 TMDB 模糊检索的误命中（实测「勇者时代」→「勇敢者的预算超支时代」2017）。
// 守卫：包含关系只在**长度接近**时采信 —— 否则本地「超人」会被 TMDB「超人总动员」误命中
// （实测反例：Contains 判定会放行跨作品匹配，从而把别人的年份写进来）。
// 阈值 0.6：挡住 超人/超人总动员(2/5=0.4)，放行 流浪地球3/流浪地球3(上)(5/8=0.625，
// 注意全角括号只是转半角、**不去掉**，故长度按 8 计)。
const yearVerifyNameMinRatio = 0.6

func yearVerifySameName(local, remote string) bool {
	a, b := yearVerifyNorm(local), yearVerifyNorm(remote)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	la, lb := utf8.RuneCountInString(a), utf8.RuneCountInString(b)
	if la == 0 || lb == 0 {
		return false
	}
	minL, maxL := la, lb
	if minL > maxL {
		minL, maxL = maxL, minL
	}
	if float64(minL)/float64(maxL) < yearVerifyNameMinRatio {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func yearVerifyNorm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.Index(s, "["); i >= 0 {
		s = s[:i]
	}
	return strings.NewReplacer(" ", "", "\u3000", "", "（", "(", "）", ")", "·", "", "：", ":").Replace(s)
}
