package service

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"server/internal/cache"
	"server/internal/config"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ---- 内存假仓储(嵌入接口, 只覆写被测方法; 未覆写方法被调用会 nil panic, 起到"未预期调用"断言) ----

type fakeSearch struct {
	repository.SearchRepository
	topCalls int
	rows     []entity.MovieSearch
}

func (f *fakeSearch) TopByPidSorted(_ context.Context, _ int64, _ repository.ClassifySort, limit int) ([]entity.MovieSearch, error) {
	f.topCalls++
	if len(f.rows) > limit {
		return f.rows[:limit], nil
	}
	return f.rows, nil
}

type fakeMovie struct {
	repository.MovieRepository
	m *entity.Movie
}

func (f *fakeMovie) GetByMid(_ context.Context, mid int64) (*entity.Movie, error) {
	if f.m == nil || f.m.Mid != mid {
		return nil, domain.ErrMovieNotFound
	}
	return f.m, nil
}

type fakePlay struct {
	repository.PlaySourceRepository
	byMid   []entity.MoviePlaySource
	matched []entity.MoviePlaySource
}

func (f *fakePlay) ListByMid(context.Context, int64) ([]entity.MoviePlaySource, error) {
	return f.byMid, nil
}
func (f *fakePlay) GetByMatchKeys(context.Context, string, []string) ([]entity.MoviePlaySource, error) {
	return f.matched, nil
}

type fakeCategory struct {
	repository.CategoryRepository
	cats []entity.Category
}

func (f *fakeCategory) All(context.Context) ([]entity.Category, error) { return f.cats, nil }

func setupCache(t *testing.T) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache.Init(rdb, rdb)
}

func newFilmService(s repository.SearchRepository, m repository.MovieRepository, p repository.PlaySourceRepository, c repository.CategoryRepository) *FilmService {
	return NewFilmService(s, m, p, c, nil, nil, config.PageSizes{Home: 14, Classify: 21, Filter: 49, Search: 10})
}

func TestSortPlayLines(t *testing.T) {
	lines := []playLine{
		{View: PlaySourceView{Id: "a:line"}, SiteId: "a"},
		{View: PlaySourceView{Id: "b:line"}, SiteId: "b"},
		{View: PlaySourceView{Id: "c:line"}, SiteId: "c"}, // 0 → 未知
		{View: PlaySourceView{Id: "d:line"}, SiteId: "d"}, // 缺省 → 未知
	}
	sortPlayLines(lines, map[string]int64{"a": 300, "b": 50, "c": 0})
	got := []string{lines[0].SiteId, lines[1].SiteId, lines[2].SiteId, lines[3].SiteId}
	want := []string{"b", "a", "c", "d"} // 升序; 未知 c/d 排末尾并保持原序
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestSortPlayLines_EmptyMap(t *testing.T) {
	lines := []playLine{
		{View: PlaySourceView{Id: "x:1"}, SiteId: "x"},
		{View: PlaySourceView{Id: "y:1"}, SiteId: "y"},
	}
	sortPlayLines(lines, nil) // 健康仓储缺省 → 保持原序(主站在前)
	if lines[0].SiteId != "x" || lines[1].SiteId != "y" {
		t.Fatalf("empty map should preserve order, got %v", []string{lines[0].SiteId, lines[1].SiteId})
	}
}

// fakeCollectSource 仅实现本测用到的 List, 其余端口方法 panic 防误用。
type fakeCollectSource struct {
	list []entity.CollectSource
	err  error
}

func (f *fakeCollectSource) List(context.Context, bool) ([]entity.CollectSource, error) {
	return f.list, f.err
}
func (f *fakeCollectSource) Get(context.Context, string) (*entity.CollectSource, error) {
	panic("unused")
}
func (f *fakeCollectSource) ExistsByUri(context.Context, string, string) (bool, error) {
	panic("unused")
}
func (f *fakeCollectSource) Upsert(context.Context, *entity.CollectSource) error { panic("unused") }
func (f *fakeCollectSource) Delete(context.Context, string) error                { panic("unused") }

func TestSourceNameMap(t *testing.T) {
	// 仓储为空 → nil(退化用 play_from)
	svc := NewFilmService(nil, nil, nil, nil, nil, nil, config.PageSizes{})
	if m := svc.sourceNameMap(context.Background()); m != nil {
		t.Fatalf("nil sources repo should yield nil map, got %v", m)
	}

	// 出错 → nil
	svcErr := NewFilmService(nil, nil, nil, nil, nil, &fakeCollectSource{err: context.DeadlineExceeded}, config.PageSizes{})
	if m := svcErr.sourceNameMap(context.Background()); m != nil {
		t.Fatalf("repo error should yield nil map, got %v", m)
	}

	// 正常 → id→name, 空 name 跳过
	svcOk := NewFilmService(nil, nil, nil, nil, nil, &fakeCollectSource{list: []entity.CollectSource{
		{Id: "s1", Name: "极速线路"},
		{Id: "s2", Name: ""}, // 空名跳过
		{Id: "s3", Name: "备用线路"},
	}}, config.PageSizes{})
	m := svcOk.sourceNameMap(context.Background())
	if m["s1"] != "极速线路" || m["s3"] != "备用线路" {
		t.Fatalf("name map wrong: %v", m)
	}
	if _, ok := m["s2"]; ok {
		t.Fatalf("empty name should be skipped, got %v", m)
	}
}

func TestHome_CacheAsideAndCompose(t *testing.T) {
	setupCache(t)
	fs := &fakeSearch{rows: []entity.MovieSearch{{Mid: 1, Name: "A", Cover: "c1"}, {Mid: 2, Name: "B"}}}
	fc := &fakeCategory{cats: []entity.Category{
		{Id: 1, Pid: 0, Name: "电影", Show: true},
		{Id: 11, Pid: 1, Name: "动作", Show: true},
	}}
	svc := newFilmService(fs, &fakeMovie{}, &fakePlay{}, fc)

	hd, err := svc.Home(context.Background())
	if err != nil {
		t.Fatalf("Home err: %v", err)
	}
	if len(hd.Categories) != 1 || hd.Categories[0].Name != "电影" {
		t.Fatalf("nav categories: %+v", hd.Categories)
	}
	if len(hd.Rows) != 1 || len(hd.Rows[0].Latest) == 0 {
		t.Fatalf("home rows: %+v", hd.Rows)
	}
	callsAfterFirst := fs.topCalls // 1 nav × (latest+hot) = 2

	// 第二次应命中缓存, 不再触发 repo
	if _, err := svc.Home(context.Background()); err != nil {
		t.Fatalf("Home2 err: %v", err)
	}
	if fs.topCalls != callsAfterFirst {
		t.Fatalf("second Home should hit cache, topCalls %d → %d", callsAfterFirst, fs.topCalls)
	}
}

func TestDetail_FoundAndNotFound(t *testing.T) {
	setupCache(t)
	mv := &entity.Movie{Mid: 7, Name: "复仇者联盟", Cid: 11, Pid: 1, DbId: 100}
	fp := &fakePlay{
		byMid:   []entity.MoviePlaySource{{SiteId: "src_lz", PlayFrom: "lzm3u8", Mid: ptr(int64(7)), Episodes: entity.EpisodeList{{Episode: "01", Link: "u1"}}}},
		matched: []entity.MoviePlaySource{{SiteId: "src_sn", PlayFrom: "snm3u8", Episodes: entity.EpisodeList{{Episode: "01", Link: "u2"}}}},
	}
	svc := newFilmService(&fakeSearch{}, &fakeMovie{m: mv}, fp, &fakeCategory{})

	d, found, err := svc.Detail(context.Background(), 7)
	if err != nil || !found {
		t.Fatalf("Detail found: found=%v err=%v", found, err)
	}
	if d.Movie.Mid != 7 || len(d.Sources) != 2 {
		t.Fatalf("detail compose: %+v", d)
	}

	_, found, err = svc.Detail(context.Background(), 999)
	if err != nil || found {
		t.Fatalf("missing movie should be not-found: found=%v err=%v", found, err)
	}
}

func ptr[T any](v T) *T { return &v }
