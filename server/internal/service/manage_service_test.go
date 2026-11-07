package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/spider"
)

// fakeSearchKW 仅覆写 Filter / SearchKeyword 以验证 SearchFilms 的关键字分支(其余方法未实现, 被调用即 panic)。
type fakeSearchKW struct {
	repository.SearchRepository
	filterCalls  int
	keywordCalls int
	lastKeyword  string
	lastSort     string
	lastSpec     repository.FilterSpec
}

func (f *fakeSearchKW) Filter(_ context.Context, spec repository.FilterSpec, _ repository.Page) ([]entity.MovieSearch, int64, error) {
	f.filterCalls++
	f.lastSpec = spec
	return nil, 0, nil
}
func (f *fakeSearchKW) SearchKeyword(_ context.Context, kw string, _ int, _ bool, sort string, _ repository.Page) ([]entity.MovieSearch, int64, error) {
	f.keywordCalls++
	f.lastKeyword = kw
	f.lastSort = sort
	return nil, 0, nil
}

// 有关键字 → 走 FULLTEXT(SearchKeyword), 且关键字被 trim。
func TestSearchFilms_KeywordRoutesToFullText(t *testing.T) {
	fs := &fakeSearchKW{}
	ms := &ManageService{search: fs}
	if _, err := ms.SearchFilms(context.Background(), repository.FilterSpec{Keyword: "  凡人  "}, repository.Page{}); err != nil {
		t.Fatal(err)
	}
	if fs.keywordCalls != 1 || fs.filterCalls != 0 {
		t.Fatalf("应只调 SearchKeyword: keyword=%d filter=%d", fs.keywordCalls, fs.filterCalls)
	}
	if fs.lastKeyword != "凡人" {
		t.Fatalf("关键字应 trim, got %q", fs.lastKeyword)
	}
}

// 关键字路径的 sort: 未指定回退热度(与无关键字 defaultSort 同口径), 指定则原样透传。
func TestSearchFilms_KeywordSortFallbackAndPassthrough(t *testing.T) {
	fs := &fakeSearchKW{}
	ms := &ManageService{search: fs}
	if _, err := ms.SearchFilms(context.Background(), repository.FilterSpec{Keyword: "凡人"}, repository.Page{}); err != nil {
		t.Fatal(err)
	}
	if fs.lastSort != "hot" {
		t.Fatalf("未指定 sort 应回退 hot, got %q", fs.lastSort)
	}
	if _, err := ms.SearchFilms(context.Background(), repository.FilterSpec{Keyword: "凡人", Sort: "score"}, repository.Page{}); err != nil {
		t.Fatal(err)
	}
	if fs.lastSort != "score" {
		t.Fatalf("sort 应透传, got %q", fs.lastSort)
	}
}

// 无关键字 → 走分类/筛选(Filter), 且 FilterSpec 原样透传(含管理端口径 IncludeHidden)。
func TestSearchFilms_NoKeywordUsesFilter(t *testing.T) {
	fs := &fakeSearchKW{}
	ms := &ManageService{search: fs}
	spec := repository.FilterSpec{Pid: 2, IncludeHidden: true}
	if _, err := ms.SearchFilms(context.Background(), spec, repository.Page{}); err != nil {
		t.Fatal(err)
	}
	if fs.filterCalls != 1 || fs.keywordCalls != 0 {
		t.Fatalf("应只调 Filter: filter=%d keyword=%d", fs.filterCalls, fs.keywordCalls)
	}
	if !reflect.DeepEqual(fs.lastSpec, spec) {
		t.Fatalf("FilterSpec 应原样透传(含 IncludeHidden), got %+v want %+v", fs.lastSpec, spec)
	}
}

func okResult() CollectTestResult {
	return CollectTestResult{Ok: true, Probes: 3, OkCount: 3, LatencyMs: 200, BestMs: 100, Films: 20, Message: "连通正常"}
}
func failResult() CollectTestResult {
	return CollectTestResult{Ok: false, Probes: 3, OkCount: 0, Message: "spider: upstream status 403"}
}

func TestApplyHealth_FirstOk(t *testing.T) {
	h := applyHealth(nil, okResult(), 3, 1000)
	if h.Status != entity.HealthHealthy || h.Suppressed || h.ConsecutiveFails != 0 {
		t.Fatalf("got status=%s suppressed=%v fails=%d", h.Status, h.Suppressed, h.ConsecutiveFails)
	}
	if h.CheckedAt != 1000 || h.LatencyMs != 200 {
		t.Fatalf("snapshot not copied: checkedAt=%d latency=%d", h.CheckedAt, h.LatencyMs)
	}
}

func TestApplyHealth_FirstFailDegradedNotSuppressed(t *testing.T) {
	h := applyHealth(nil, failResult(), 3, 1)
	if h.Status != entity.HealthDegraded || h.Suppressed || h.ConsecutiveFails != 1 {
		t.Fatalf("got status=%s suppressed=%v fails=%d", h.Status, h.Suppressed, h.ConsecutiveFails)
	}
}

func TestApplyHealth_ReachThresholdSuppresses(t *testing.T) {
	prev := &entity.SourceHealth{ConsecutiveFails: 2, Status: entity.HealthDegraded}
	h := applyHealth(prev, failResult(), 3, 1)
	if h.Status != entity.HealthDown || !h.Suppressed || h.ConsecutiveFails != 3 {
		t.Fatalf("got status=%s suppressed=%v fails=%d", h.Status, h.Suppressed, h.ConsecutiveFails)
	}
}

func TestApplyHealth_RecoverUnsuppresses(t *testing.T) {
	prev := &entity.SourceHealth{ConsecutiveFails: 5, Status: entity.HealthDown, Suppressed: true}
	h := applyHealth(prev, okResult(), 3, 1)
	if h.Status != entity.HealthHealthy || h.Suppressed || h.ConsecutiveFails != 0 {
		t.Fatalf("recover failed: status=%s suppressed=%v fails=%d", h.Status, h.Suppressed, h.ConsecutiveFails)
	}
}

func TestApplyHealth_StaySuppressedWhileFailing(t *testing.T) {
	prev := &entity.SourceHealth{ConsecutiveFails: 3, Status: entity.HealthDown, Suppressed: true}
	h := applyHealth(prev, failResult(), 3, 1)
	if h.Status != entity.HealthDown || !h.Suppressed || h.ConsecutiveFails != 4 {
		t.Fatalf("got status=%s suppressed=%v fails=%d", h.Status, h.Suppressed, h.ConsecutiveFails)
	}
}

func TestSummarizeProbes_AllFail(t *testing.T) {
	r := summarizeProbes(3, nil, 0, errors.New("dial timeout"))
	if r.Ok {
		t.Fatal("Ok should be false on all-fail")
	}
	if r.OkCount != 0 {
		t.Fatalf("OkCount = %d, want 0", r.OkCount)
	}
	if r.Message != "dial timeout" {
		t.Fatalf("Message = %q, want last error text", r.Message)
	}
}

func TestSummarizeProbes_AllFailNoErr(t *testing.T) {
	r := summarizeProbes(3, nil, 0, nil)
	if r.Ok || r.Message != "全部探测失败" {
		t.Fatalf("got ok=%v msg=%q", r.Ok, r.Message)
	}
}

func TestSummarizeProbes_AllOk(t *testing.T) {
	r := summarizeProbes(3, []int64{300, 100, 200}, 20, nil)
	if !r.Ok {
		t.Fatal("Ok should be true")
	}
	if r.OkCount != 3 {
		t.Fatalf("OkCount = %d, want 3", r.OkCount)
	}
	if r.BestMs != 100 {
		t.Fatalf("BestMs = %d, want 100 (fastest)", r.BestMs)
	}
	if r.LatencyMs != 200 {
		t.Fatalf("LatencyMs = %d, want 200 (median)", r.LatencyMs)
	}
	if r.Message != "连通正常" {
		t.Fatalf("Message = %q, want 连通正常", r.Message)
	}
}

func TestSummarizeProbes_Unstable(t *testing.T) {
	r := summarizeProbes(3, []int64{150}, 10, nil)
	if !r.Ok {
		t.Fatal("Ok should be true when at least one probe parsed films")
	}
	if r.OkCount != 1 || r.BestMs != 150 || r.LatencyMs != 150 {
		t.Fatalf("got okCount=%d best=%d median=%d", r.OkCount, r.BestMs, r.LatencyMs)
	}
	if !strings.Contains(r.Message, "不稳定") {
		t.Fatalf("Message = %q, want 不稳定", r.Message)
	}
}

func TestSummarizeProbes_ConnectedButNoFilms(t *testing.T) {
	r := summarizeProbes(3, []int64{100, 200}, 0, nil)
	if r.Ok {
		t.Fatal("Ok should be false when films == 0")
	}
	if !strings.Contains(r.Message, "未解析到影片") {
		t.Fatalf("Message = %q, want 未解析到影片", r.Message)
	}
}

// ---- 测速拆分: 采集(服务端 API) / 播放(浏览器端) / 广告过滤可达性 ----

// fakeSourceRepo 仅按 id 返回预置源, List 返回全部(其余方法被调用即 panic, 测试不应触发)。
type fakeSourceRepo struct {
	repository.CollectSourceRepository
	byId map[string]entity.CollectSource
	all  []entity.CollectSource
}

func (f *fakeSourceRepo) Get(_ context.Context, id string) (*entity.CollectSource, error) {
	s, ok := f.byId[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &s, nil
}
func (f *fakeSourceRepo) List(context.Context, bool) ([]entity.CollectSource, error) {
	return f.all, nil
}

// fakeHealthRepo 记录 Upsert 调用次数与健康行, 验证健康度写入行为。
type fakeHealthRepo struct {
	upserts int
	last    *entity.SourceHealth
}

func (f *fakeHealthRepo) Get(context.Context, string) (*entity.SourceHealth, error) {
	return nil, nil
}
func (f *fakeHealthRepo) List(context.Context) ([]entity.SourceHealth, error) { return nil, nil }
func (f *fakeHealthRepo) Upsert(_ context.Context, h *entity.SourceHealth) error {
	f.upserts++
	f.last = h
	return nil
}

// TestAllSources: 探测失败的源写一次健康度(计失败), Ok=false。
func TestTestAllSources_FailedSourceRecordsHealth(t *testing.T) {
	// 源 uri 指向回环非法端点, Probe 会快速失败 → recordHealth 写一次(失败计数)。
	sr := &fakeSourceRepo{all: []entity.CollectSource{
		{Id: "norm", Name: "Norm", Uri: "http://127.0.0.1:1/api"},
	}}
	hr := &fakeHealthRepo{}
	ms := &ManageService{sources: sr, health: hr, prober: spider.NewFetcherWithTimeout(time.Second)}
	out, err := ms.TestAllSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 result, got %d", len(out))
	}
	if out[0].Ok {
		t.Fatalf("失败源不应 Ok=true")
	}
	if hr.upserts != 1 {
		t.Fatalf("应为失败源写 1 次健康度, got %d", hr.upserts)
	}
}

// TestApplyHealth_SplitFields: 采集测速结果合并 —— ApiCheckedAt 必写;
// 样本 m3u8 / 播放延时只在有值时更新; 浏览器端播放字段不受影响。
func TestApplyHealth_SplitFields(t *testing.T) {
	now := int64(1000)
	prev := &entity.SourceHealth{
		PlayLatencyWeb: 234, PlayCheckedAt: 999, SampleM3u8: "http://old/x.m3u8",
	}
	res := CollectTestResult{Ok: true, LatencyMs: 120, PlayLatencyMs: 55, SampleM3u8: "http://new/x.m3u8"}
	h := applyHealth(prev, res, 3, now)
	if h.ApiCheckedAt != now {
		t.Fatalf("ApiCheckedAt = %d, want %d", h.ApiCheckedAt, now)
	}
	if h.SampleM3u8 != "http://new/x.m3u8" || h.PlayLatencyMs != 55 {
		t.Fatalf("样本/延时应正确写入: %+v", h)
	}
	if h.PlayLatencyWeb != 234 || h.PlayCheckedAt != 999 {
		t.Fatalf("浏览器端播放字段不应被采集测速覆盖: %+v", h)
	}
	// 无样本: 不动上次样本
	h3 := applyHealth(prev, CollectTestResult{Ok: true}, 3, now)
	if h3.SampleM3u8 != "http://old/x.m3u8" {
		t.Fatalf("无样本不应覆盖上次样本: %+v", h3)
	}
}

// TestSiteURLRe: 站点网址规则(选填, http/https)。
func TestSiteURLRe(t *testing.T) {
	for _, ok := range []string{"https://a.com", "http://a.com/x?y=1"} {
		if !siteURLRe.MatchString(ok) {
			t.Fatalf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"ftp://a.com", "a.com", "https://"} {
		if siteURLRe.MatchString(bad) {
			t.Fatalf("%q 应非法", bad)
		}
	}
}

// ---- UpsertSource 采集源 id 命名规则 ----

// TestCollectSourceIDRe: 规则边界 —— 小写字母开头, 仅 [a-z0-9_], 2~32 字符。
func TestCollectSourceIDRe(t *testing.T) {
	for _, ok := range []string{"src_lz", "src_huya", "ab", "a1_b2", strings.Repeat("a", 32)} {
		if !collectSourceIDRe.MatchString(ok) {
			t.Fatalf("%q 应合法", ok)
		}
	}
	for _, bad := range []string{"", "A", "1src", "src-lz", "src lz", "大写", strings.Repeat("a", 33)} {
		if collectSourceIDRe.MatchString(bad) {
			t.Fatalf("%q 应不合法", bad)
		}
	}
}

// TestUpsertSource_RejectsInvalidID: 不合法 id 在触达仓储前被拒绝(ErrInvalidSourceID)。
// 合法路径会调用 ExistsByUri(本 fake 未实现, 调用即 panic)与 Redis 缓存失效, 单测环境不覆盖。
func TestUpsertSource_RejectsInvalidID(t *testing.T) {
	ms := &ManageService{sources: &fakeSourceRepo{byId: map[string]entity.CollectSource{}}}
	for _, bad := range []string{"", "SRC", "1abc", "ab-cd", "a", "has space"} {
		err := ms.UpsertSource(context.Background(), &entity.CollectSource{Id: bad})
		if !errors.Is(err, domain.ErrInvalidSourceID) {
			t.Fatalf("id=%q 应返回 ErrInvalidSourceID, got %v", bad, err)
		}
	}
}

// fakeCatAll 仅覆写 All: ListCategories 的分类树来源。
type fakeCatAll struct {
	repository.CategoryRepository
	cats []entity.Category
}

func (f *fakeCatAll) All(context.Context) ([]entity.Category, error) { return f.cats, nil }

// fakeCountFilms 仅覆写 CountFilmsByCategory: 分类影片计数来源。
type fakeCountFilms struct {
	repository.SearchRepository
	pids, cids map[int64]int64
}

func (f *fakeCountFilms) CountFilmsByCategory(context.Context) (map[int64]int64, map[int64]int64, error) {
	return f.pids, f.cids, nil
}

// TestListCategories_TopCountIncludesDirect 顶级分类计数 = pids(子分类影片, pid 恒为父顶级
// id) + cids(直挂影片, XML 源/maccms type_id_1=0 写法下 pid=0 只有 cid), 与公开列表
// (pid=X OR cid=X) 同口径 —— 修复"AI漫剧计数 0 但影片在架"的显示口径。
func TestListCategories_TopCountIncludesDirect(t *testing.T) {
	ms := &ManageService{
		category: &fakeCatAll{cats: []entity.Category{
			{Id: 1, Pid: 0, Name: "电影"},
			{Id: 2, Pid: 1, Name: "动作"},
		}},
		search: &fakeCountFilms{
			pids: map[int64]int64{1: 30},             // 子分类影片(pid=1)
			cids: map[int64]int64{1: 12, 2: 30},      // 直挂(顶级1) + 子分类2
		},
	}
	tree, err := ms.ListCategories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].Id != 1 {
		t.Fatalf("树形不符: %v", tree)
	}
	if tree[0].FilmCount == nil || *tree[0].FilmCount != 42 {
		t.Fatalf("顶级计数应为 30+12=42, got %v", tree[0].FilmCount)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].FilmCount == nil || *tree[0].Children[0].FilmCount != 30 {
		t.Fatalf("子分类计数应为 30, got %v", tree[0].Children)
	}
}

// fakeCatUpsert 记录 Upsert 调用: 验证三开关强制联动的级联隐藏。
type fakeCatUpsert struct {
	repository.CategoryRepository
	cats    []entity.Category
	upserts []entity.Category
}

func (f *fakeCatUpsert) All(context.Context) ([]entity.Category, error) { return f.cats, nil }

func (f *fakeCatUpsert) Upsert(_ context.Context, c *entity.Category) error {
	// 模拟 DB 行为: 按 id 覆盖(新行 append)。
	found := false
	for i := range f.cats {
		if f.cats[i].Id == c.Id {
			f.cats[i] = *c
			found = true
			break
		}
	}
	if !found {
		f.cats = append(f.cats, *c)
	}
	f.upserts = append(f.upserts, *c)
	return nil
}

// TestUpsertCategory_FilmInvisibleForcesNavOff 影片可见性置 false 是最狠一档:
// 服务端必须强制联动 —— 本节点 show/sidebar_show 置 false, 且全部子孙的
// show/sidebar_show 也一并置 false(前端开关同步禁用, 这里是服务端兜底)。
// 子级 filmVisible 不动(顶级 film_visible=0 时 pid NOT IN 已剔除子级影片,
// 顶级恢复可见时子级影片行为不突变)。
func TestUpsertCategory_FilmInvisibleForcesNavOff(t *testing.T) {
	fr := &fakeCatUpsert{cats: []entity.Category{
		{Id: 1, Pid: 0, Name: "顶级", Show: true, SidebarShow: true, FilmVisible: true},
		{Id: 11, Pid: 1, Name: "子类A", Show: true, SidebarShow: true, FilmVisible: true},
		{Id: 12, Pid: 1, Name: "子类B", Show: false, SidebarShow: false, FilmVisible: true},
	}}
	ms := &ManageService{category: fr}

	// 管理端提交: 影片不可见(其余字段被前端照旧传上来, 可能仍是 true)
	err := ms.UpsertCategory(context.Background(), &entity.Category{
		Id: 1, Pid: 0, Name: "顶级", Show: true, SidebarShow: true, FilmVisible: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	top := fr.cats[0]
	if top.Show || top.SidebarShow {
		t.Fatalf("filmVisible=false 应强制 show/sidebarShow=false: %+v", top)
	}
	byId := map[int64]entity.Category{}
	for _, c := range fr.cats {
		byId[c.Id] = c
	}
	if byId[11].Show || byId[11].SidebarShow {
		t.Fatalf("子类A 应被级联隐藏: %+v", byId[11])
	}
	if byId[11].FilmVisible != true {
		t.Fatalf("子类A filmVisible 不应被改: %+v", byId[11])
	}
	if byId[12].Show || byId[12].SidebarShow {
		t.Fatalf("子类B 本已隐藏: %+v", byId[12])
	}
}

// TestUpsertCategory_VisibleNoCascade 影片可见的分类保存不做级联, 正常落库。
func TestUpsertCategory_VisibleNoCascade(t *testing.T) {
	fr := &fakeCatUpsert{cats: []entity.Category{
		{Id: 1, Pid: 0, Name: "顶级", Show: false, SidebarShow: false, FilmVisible: true},
	}}
	ms := &ManageService{category: fr}
	if err := ms.UpsertCategory(context.Background(), &entity.Category{
		Id: 1, Pid: 0, Name: "顶级", Show: true, SidebarShow: true, FilmVisible: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(fr.upserts) != 1 {
		t.Fatalf("可见分类保存不应触发级联 Upsert: %d 次", len(fr.upserts))
	}
	if !fr.cats[0].Show || !fr.cats[0].SidebarShow {
		t.Fatalf("开关应按提交值落库: %+v", fr.cats[0])
	}
}

// fakePlaySlim 仅覆写 ListSlimByMidsOrKeys, 验证 CountSources 的口径(主源行 ∪ 名字键行, site+playFrom 去重)。
type fakePlaySlim struct {
	repository.PlaySourceRepository
	rows []repository.SlimSourceRow
}

func (f *fakePlaySlim) ListSlimByMidsOrKeys(_ context.Context, _ []int64, _ []string) ([]repository.SlimSourceRow, error) {
	return f.rows, nil
}

// CountSources 口径对齐详情页: 补充源行 mid=NULL 靠 match_key 关联也要计入, 同 site+playFrom 去重。
// 回归背景: 旧口径(只 GROUP BY mid)在线上 74% 源行 mid=NULL 时几乎恒报 1(2026-11-07 修复)。
func TestCountSources_MatchesDetailSemantics(t *testing.T) {
	key := domain.GenerateHashKey("完美世界")
	fp := &fakePlaySlim{rows: []repository.SlimSourceRow{
		{Mid: ptr[int64](1), MatchKey: key, SiteId: "src_a", PlayFrom: "线路1"},  // 主源行
		{MatchKey: key, SiteId: "src_b", PlayFrom: "线路1"},                      // 补充源行(mid=NULL)
		{MatchKey: key, SiteId: "src_b", PlayFrom: "线路2"},                      // 补充源行
		{MatchKey: key, SiteId: "src_a", PlayFrom: "线路1"},                      // 豆瓣键双写行: 同 site+playFrom → 去重不计
		{MatchKey: domain.GenerateHashKey("别的片"), SiteId: "src_c", PlayFrom: "线路1"}, // 别的片的键 → 不计入
	}}
	ms := &ManageService{play: fp}
	counts, err := ms.CountSources(context.Background(), []entity.MovieSearch{{Mid: 1, Name: "完美世界"}})
	if err != nil {
		t.Fatal(err)
	}
	if counts[1] != 3 {
		t.Fatalf("应数出 3 条去重线路(主源1 + 补充2), got %d", counts[1])
	}
}

// 同名影片共享名字键桶: 每片的计数都含对方的主源行(与详情装配 GetByMatchKeys 同行为)。
func TestCountSources_SameNameFilmsShareKeyRows(t *testing.T) {
	key := domain.GenerateHashKey("狩猎")
	fp := &fakePlaySlim{rows: []repository.SlimSourceRow{
		{Mid: ptr[int64](1), MatchKey: key, SiteId: "src_a", PlayFrom: "线路1"},
		{Mid: ptr[int64](2), MatchKey: key, SiteId: "src_b", PlayFrom: "线路1"},
		{MatchKey: key, SiteId: "src_c", PlayFrom: "线路1"},
	}}
	ms := &ManageService{play: fp}
	counts, err := ms.CountSources(context.Background(), []entity.MovieSearch{
		{Mid: 1, Name: "狩猎"}, {Mid: 2, Name: "狩猎"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts[1] != 3 || counts[2] != 3 {
		t.Fatalf("同名两片各应数出 3 条去重线路, got %d/%d", counts[1], counts[2])
	}
}

// 无名影片只按 mid 行计; 空入参返回空表。
func TestCountSources_NamelessFilmAndEmptyInput(t *testing.T) {
	fp := &fakePlaySlim{rows: []repository.SlimSourceRow{
		{Mid: ptr[int64](3), MatchKey: "whatever", SiteId: "src_a", PlayFrom: "线路1"},
		{Mid: ptr[int64](3), MatchKey: "whatever", SiteId: "src_a", PlayFrom: "线路2"},
	}}
	ms := &ManageService{play: fp}
	counts, err := ms.CountSources(context.Background(), []entity.MovieSearch{{Mid: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if counts[3] != 2 {
		t.Fatalf("无名影片应只按 mid 行计 2, got %d", counts[3])
	}
	empty, err := ms.CountSources(context.Background(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("空入参应返回空表, got %v / %v", empty, err)
	}
}
