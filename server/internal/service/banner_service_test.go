package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ---- 轮播服务测试假仓储 ----

type fakeBannerRepo struct {
	repository.BannerRepository
	rows  []entity.Banner
	moved map[int64]int // UpdateSort 记录
}

func (f *fakeBannerRepo) ListAll(context.Context) ([]entity.Banner, error) {
	out := append([]entity.Banner(nil), f.rows...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sort < out[j].Sort })
	return out, nil
}

func (f *fakeBannerRepo) Create(_ context.Context, b *entity.Banner) error {
	b.Id = int64(len(f.rows) + 100)
	f.rows = append(f.rows, *b)
	return nil
}

func (f *fakeBannerRepo) Update(_ context.Context, b *entity.Banner) error {
	for i := range f.rows {
		if f.rows[i].Id == b.Id {
			f.rows[i] = *b
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeBannerRepo) UpdateSort(_ context.Context, id int64, sort int) error {
	if f.moved == nil {
		f.moved = map[int64]int{}
	}
	f.moved[id] = sort
	for i := range f.rows {
		if f.rows[i].Id == id {
			f.rows[i].Sort = sort
		}
	}
	return nil
}

// bannerFilms 构造一个 Home 可用的 FilmService: nav 一个分类, 榜单 n 部影片,
// 并注入 8 榜各前 2 的轮播候选(主路径, 见 docs/热榜页面与轮播候选方案 §2)。
func bannerFilms(t *testing.T, n int) *FilmService {
	t.Helper()
	setupCache(t)
	pool := hotPool(n)
	fs := newFilmService(
		&fakeSearch{rows: pool, hotAll: pool, cands: bannerCandsFrom(pool)},
		&fakeMovie{}, &fakePlay{},
		&fakeCategory{cats: []entity.Category{{Id: 1, Pid: 0, Name: "电影", Show: true}}},
	)
	return fs
}

func hotPool(n int) []entity.MovieSearch {
	return hotPoolFrom(1000, n)
}

// testBoardOrder 8 榜权重顺序(与 service.candidateWeightOrder 对齐, 测试用字面量)。
var testBoardOrder = []string{
	"movie_hot_gaia", "tv_hot", "movie_showing", "movie_weekly_best",
	"tv_chinese_best_weekly", "tv_global_best_weekly", "tv_variety_show", "tv_animation",
}

// bannerCandsFrom 造 8 榜各前 2 共 16 个候选(按 weight_pos 升序), mid 取 pool 前 16。
func bannerCandsFrom(pool []entity.MovieSearch) []repository.HotBoardCandidate {
	out := make([]repository.HotBoardCandidate, 0, 16)
	for i := 0; i < 16 && i < len(pool); i++ {
		out = append(out, repository.HotBoardCandidate{
			WeightPos: i, Board: testBoardOrder[i%8], Rank: i%2 + 1,
			Mid: pool[i].Mid, Score: 1000 - i,
		})
	}
	return out
}

// hotPoolFrom 生成 mid = base+1 .. base+n 的影片池(测试用不相交池模拟"热榜换新")。
func hotPoolFrom(base, n int) []entity.MovieSearch {
	out := make([]entity.MovieSearch, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, entity.MovieSearch{
			Mid: int64(base + i), Name: fmt.Sprintf("影片%d", base+i),
			Cover: fmt.Sprintf("cover-%d-%d.jpg", base, i), Pid: 1, Cid: 11,
		})
	}
	return out
}

// GetByMids enrichMeta(Public/Board) 的元信息补齐走这里; 按 mid 命中榜单池即回。
func (f *fakeSearch) GetByMids(_ context.Context, mids []int64) ([]entity.MovieSearch, error) {
	idx := make(map[int64]entity.MovieSearch, len(f.rows))
	for _, r := range f.rows {
		idx[r.Mid] = r
	}
	out := make([]entity.MovieSearch, 0, len(mids))
	for _, m := range mids {
		if r, ok := idx[m]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// TestDisableByMidIdempotent 幂等屏蔽: 已有行 → 置停用不新增; 无行 → 只建一条屏蔽行;
// 重复禁用(含已屏蔽后再次调用)永远只产生一行。
func TestDisableByMidIdempotent(t *testing.T) {
	setupCache(t)
	fs := bannerFilms(t, 30)
	svc := NewBannerService(&fakeBannerRepo{}, fs)
	repo := svc.repo.(*fakeBannerRepo)

	// 场景 1: 从未出现过的 mid → 新建一条屏蔽行
	mid := int64(7001)
	if err := svc.DisableByMid(context.Background(), mid); err != nil {
		t.Fatalf("disable new mid: %v", err)
	}
	if len(repo.rows) != 1 || repo.rows[0].Mid != mid || repo.rows[0].State != entity.BannerDisabled {
		t.Fatalf("expect 1 disabled row for new mid, got %+v", repo.rows)
	}

	// 场景 2: 重复禁用同一 mid → 幂等, 仍只有一行
	if err := svc.DisableByMid(context.Background(), mid); err != nil {
		t.Fatalf("disable again: %v", err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("expect still 1 row after re-disable, got %d", len(repo.rows))
	}

	// 场景 3: 已有启用行(手动位) → 置停用, 不新增
	active := entity.Banner{Id: 55, Title: "手动位", Mid: 7002, Image: "x.jpg", State: entity.BannerEnabled, Sort: 0}
	repo.rows = append(repo.rows, active)
	if err := svc.DisableByMid(context.Background(), 7002); err != nil {
		t.Fatalf("disable existing manual row: %v", err)
	}
	if len(repo.rows) != 2 {
		t.Fatalf("expect 2 rows total, got %d", len(repo.rows))
	}
	var found *entity.Banner
	for i := range repo.rows {
		if repo.rows[i].Mid == 7002 {
			found = &repo.rows[i]
		}
	}
	if found == nil || found.State != entity.BannerDisabled || found.Id != 55 {
		t.Fatalf("expect existing row 55 toggled to disabled, got %+v", found)
	}
}

// TestBoardManualRowCarriesBannerAndImageFallback Board 手动行必须携带完整配置行
// (前端删除/停用/编辑按钮依赖 s.banner), 且 Image 为空时用影片已回填的 backdrop 兜底。
// TestBoardManualRowCarriesBannerAndImageFallback Board 手动行必须携带完整配置行
// (前端删除/停用/编辑按钮依赖 s.banner); 自动候选位 Image 为空时用影片已回填的
// backdrop 兜底(修复"回填成功但页面显示无横图" —— 回填写 movie 表, 轮播位 Image
// 来自 banner.image/榜单快照, 两者不同源)。
func TestBoardManualRowCarriesBannerAndImageFallback(t *testing.T) {
	setupCache(t)
	repo := &fakeBannerRepo{rows: []entity.Banner{
		{Id: 1, Title: "手动1", Mid: 1001, Image: "manual-1.jpg", Poster: "", State: entity.BannerEnabled, Sort: 0},
	}}
	pool := []entity.MovieSearch{
		{Mid: 1001, Name: "手动1", Pid: 1, Cid: 11},
		{Mid: 1002, Name: "候选2", Pid: 1, Cid: 11, Backdrop: "backdrop-1002.jpg"},
	}
	// 候选首位 = 1002, 快照不带图(Backdrop 空) → 依赖影片侧 backdrop 兜底
	cands := []repository.HotBoardCandidate{
		{WeightPos: 0, Board: testBoardOrder[0], Rank: 1, Mid: 1002, Score: 1000},
	}
	fs := newFilmService(
		&fakeSearch{rows: pool, hotAll: pool, cands: cands},
		&fakeMovie{}, &fakePlay{},
		&fakeCategory{cats: []entity.Category{{Id: 1, Pid: 0, Name: "电影", Show: true}}},
	)
	svc := NewBannerService(repo, fs)
	board, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	var manual, auto *EffectiveSlide
	for i := range board.Active {
		switch board.Active[i].Source {
		case SlideSourceManual:
			manual = &board.Active[i]
		case SlideSourceAuto:
			auto = &board.Active[i]
		}
	}
	if manual == nil {
		t.Fatal("expect a manual slide in board")
	}
	if manual.Banner == nil || manual.Banner.Id != 1 {
		t.Fatalf("expect manual slide carries full banner row, got %+v", manual.Banner)
	}
	if manual.Image != "manual-1.jpg" {
		t.Fatalf("expect manual image preserved, got %q", manual.Image)
	}
	if auto == nil {
		t.Fatal("expect an auto candidate slide in board")
	}
	if auto.Image != "backdrop-1002.jpg" {
		t.Fatalf("expect auto image fallback to movie backdrop, got %q", auto.Image)
	}
}

// TestBoardShowsTenLiveFive 管理视图展示 10 位、前 5 位生效; 前台仍只出 5 位。
func TestBoardShowsTenLiveFive(t *testing.T) {
	repo := &fakeBannerRepo{}
	svc := NewBannerService(repo, bannerFilms(t, 30))

	pub, err := svc.Public(context.Background())
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	if len(pub) != heroSlideCount {
		t.Fatalf("前台应恒为 %d 位, got %d", heroSlideCount, len(pub))
	}

	board, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	if len(board.Active) != heroBoardCount {
		t.Fatalf("管理视图应展示 %d 位, got %d", heroBoardCount, len(board.Active))
	}
	if board.LiveCount != heroSlideCount {
		t.Fatalf("liveCount 应为 %d, got %d", heroSlideCount, board.LiveCount)
	}
	// 前 5 位与前台完全同源(同序同 mid)
	for i := range pub {
		if board.Active[i].Mid != pub[i].Mid || board.Active[i].Source != pub[i].Source {
			t.Fatalf("第 %d 位管理与前台不同源: board=%+v pub=%+v", i, board.Active[i], pub[i])
		}
	}
	// 第 6-10 位为候选(自动位), 不在对外生效的前 5 里
	for i := heroSlideCount; i < heroBoardCount; i++ {
		if board.Active[i].Mid == pub[heroSlideCount-1].Mid {
			t.Fatalf("候选位 %d 不应与生效区末位重复", i)
		}
	}
}

// TestBoardManualOrderSurvivesRefresh 管理员已采纳(手动行)的顺序持久化:
// 热榜刷新(fakeSearch 换一批新片)后, 手动行仍按原 sort 排前, 候选位换新。
func TestBoardManualOrderSurvivesRefresh(t *testing.T) {
	pool := hotPool(30)
	fs := &fakeSearch{rows: pool, hotAll: pool, cands: bannerCandsFrom(pool)}
	svc := NewBannerService(&fakeBannerRepo{}, newFilmService(fs, &fakeMovie{}, &fakePlay{},
		&fakeCategory{cats: []entity.Category{{Id: 1, Pid: 0, Name: "电影", Show: true}}}))

	// 落两条手动行(模拟管理员已采纳并排序)
	repo := svc.repo.(*fakeBannerRepo)
	repo.rows = append(repo.rows,
		entity.Banner{Id: 1, Title: "手动A", Mid: 9001, Image: "a.jpg", State: entity.BannerEnabled, Sort: 0},
		entity.Banner{Id: 2, Title: "手动B", Mid: 9002, Image: "b.jpg", State: entity.BannerEnabled, Sort: 1},
	)

	board1, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	if board1.Active[0].Mid != 9001 || board1.Active[1].Mid != 9002 {
		t.Fatalf("手动行应排前且保持顺序: %+v", board1.Active[:2])
	}

	// 热榜刷新: 换一批**不相交**的全新影片(Home 聚合有缓存, 模拟刷新需连缓存一起失效);
	// 候选快照也随每日榜单换新。
	newPool := hotPoolFrom(5000, 50)
	fs.rows = newPool
	fs.hotAll = newPool
	fs.cands = bannerCandsFrom(newPool)
	cache.Del(context.Background(), cache.KeyHomeAgg)
	board2, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("Board(刷新后): %v", err)
	}
	if board2.Active[0].Mid != 9001 || board2.Active[1].Mid != 9002 {
		t.Fatalf("刷新后手动行顺序必须保持: %+v", board2.Active[:2])
	}
	// 候选区(生效区之后的自动位)换成新片
	seen := map[int64]bool{9001: true, 9002: true}
	fresh := 0
	for i := heroSlideCount; i < len(board2.Active); i++ {
		m := board2.Active[i].Mid
		if m == 0 || seen[m] {
			continue
		}
		if m < 5000 {
			t.Fatalf("候选位 %d 出现刷新前的旧候选 %d —— 自动位应随热榜换新", i, m)
		}
		fresh++
	}
	if fresh == 0 {
		t.Fatalf("刷新后候选区没有新影片")
	}
}

// TestMoveAdoptFromBoardRegion 管理视图 10 位口径下, 从候选区(第 6-10 位)采纳自动位:
// 钉入手动区末尾(进入生效区), 而不是按原 slot 落在生效区之外。
func TestMoveAdoptFromBoardRegion(t *testing.T) {
	repo := &fakeBannerRepo{}
	svc := NewBannerService(repo, bannerFilms(t, 30))

	board, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	candidate := board.Active[heroBoardCount-1] // 第 10 位(候选区末)
	if candidate.Source != SlideSourceAuto {
		t.Fatalf("第 %d 位应为自动位, got %s", heroBoardCount-1, candidate.Source)
	}
	// 候选位上移(逐位)最终采纳; 这里直接验证: 对候选位上移一次 = 采纳钉入手动末位
	if err := svc.Move(context.Background(), heroBoardCount-1, "up"); err != nil {
		t.Fatalf("候选区上移采纳: %v", err)
	}
	board2, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("Board(采纳后): %v", err)
	}
	if board2.Active[0].Source != SlideSourceManual || board2.Active[0].Mid != candidate.Mid {
		t.Fatalf("采纳后应成为手动位第 1 位: %+v", board2.Active[0])
	}
	if board2.Active[0].Image != candidate.Image || board2.Active[0].Poster != candidate.Poster {
		t.Fatalf("采纳应把当前生效图 pin 住: %+v vs %+v", board2.Active[0], candidate)
	}
}

// TestExcludedMidNeverAutoFilled 屏蔽铁律: banner 表出现过的 mid(无论启用与否)
// 永不由自动补位产生 —— 禁用一条候选后, 热榜刷新它也不应重现。
func TestExcludedMidNeverAutoFilled(t *testing.T) {
	repo := &fakeBannerRepo{rows: []entity.Banner{
		{Id: 1, Title: "屏蔽位", Mid: 1005, State: entity.BannerDisabled, Sort: 0}, // 禁用行=屏蔽
	}}
	svc := NewBannerService(repo, bannerFilms(t, 30))

	board, err := svc.Board(context.Background())
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	for i, sl := range board.Active {
		if sl.Mid == 1005 {
			t.Fatalf("被屏蔽的 mid=%d 出现在第 %d 位(禁用行应永久排除出自动补位)", 1005, i)
		}
	}
	// 手动行(禁用)不出现在 Active, 应以 disabled 原因出现在未生效区
	found := false
	for _, row := range board.Inactive {
		if row.Mid == 1005 && row.Reason == "disabled" {
			found = true
		}
	}
	if !found {
		t.Fatalf("禁用行应出现在未生效区(reason=disabled): %+v", board.Inactive)
	}
}

// TestMoveSlotOutOfRange 管理视图 10 位口径: slot 越界仍返回 ErrInvalidMove。
func TestMoveSlotOutOfRange(t *testing.T) {
	svc := NewBannerService(&fakeBannerRepo{}, bannerFilms(t, 30))
	if err := svc.Move(context.Background(), heroBoardCount, "up"); !errors.Is(err, domain.ErrInvalidMove) {
		t.Fatalf("slot=%d 应越界, got %v", heroBoardCount, err)
	}
}
