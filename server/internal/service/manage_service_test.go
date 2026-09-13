package service

import (
	"context"
	"errors"
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
}

func (f *fakeSearchKW) Filter(context.Context, repository.FilterSpec, repository.Page) ([]entity.MovieSearch, int64, error) {
	f.filterCalls++
	return nil, 0, nil
}
func (f *fakeSearchKW) SearchKeyword(_ context.Context, kw string, _ int, _ repository.Page) ([]entity.MovieSearch, int64, error) {
	f.keywordCalls++
	f.lastKeyword = kw
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

// 无关键字 → 走分类/筛选(Filter)。
func TestSearchFilms_NoKeywordUsesFilter(t *testing.T) {
	fs := &fakeSearchKW{}
	ms := &ManageService{search: fs}
	if _, err := ms.SearchFilms(context.Background(), repository.FilterSpec{Pid: 2}, repository.Page{}); err != nil {
		t.Fatal(err)
	}
	if fs.filterCalls != 1 || fs.keywordCalls != 0 {
		t.Fatalf("应只调 Filter: filter=%d keyword=%d", fs.filterCalls, fs.keywordCalls)
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
// 样本 m3u8 / 广告过滤可达性(成功与失败都写)只在有值时更新; 浏览器端播放字段不受影响。
func TestApplyHealth_SplitFields(t *testing.T) {
	now := int64(1000)
	ok := true
	prev := &entity.SourceHealth{
		PlayLatencyWeb: 234, PlayCheckedAt: 999, SampleM3u8: "http://old/x.m3u8",
	}
	// 成功探测 + m3u8 可达
	res := CollectTestResult{Ok: true, LatencyMs: 120, PlayLatencyMs: 55, SampleM3u8: "http://new/x.m3u8", AdFilterOk: &ok}
	h := applyHealth(prev, res, 3, now)
	if h.ApiCheckedAt != now {
		t.Fatalf("ApiCheckedAt = %d, want %d", h.ApiCheckedAt, now)
	}
	if h.SampleM3u8 != "http://new/x.m3u8" || h.PlayLatencyMs != 55 || h.AdFilterOk == nil || !*h.AdFilterOk {
		t.Fatalf("广告过滤可达性未正确写入: %+v", h)
	}
	if h.PlayLatencyWeb != 234 || h.PlayCheckedAt != 999 {
		t.Fatalf("浏览器端播放字段不应被采集测速覆盖: %+v", h)
	}
	// m3u8 不可达: AdFilterOk=false 也必须落(失败同样有价值)
	bad := false
	h2 := applyHealth(nil, CollectTestResult{Ok: true, SampleM3u8: "http://new/x.m3u8", AdFilterOk: &bad}, 3, now)
	if h2.AdFilterOk == nil || *h2.AdFilterOk {
		t.Fatalf("m3u8 不可达应写 AdFilterOk=false: %+v", h2)
	}
	if h2.AdFilterCheckedAt != now {
		t.Fatalf("AdFilterCheckedAt = %d, want %d", h2.AdFilterCheckedAt, now)
	}
	// 无样本: 不动上次可达性
	h3 := applyHealth(prev, CollectTestResult{Ok: true}, 3, now)
	if h3.AdFilterOk != nil {
		t.Fatalf("无样本不应写 AdFilterOk: %+v", h3)
	}
}

// TestRecordAdFilter: 运行时兜底上报只接受失败(ok=true 直接忽略), 失败立即落库。
func TestRecordAdFilter(t *testing.T) {
	hr := &fakeHealthRepo{}
	ms := &ManageService{health: hr}
	if err := ms.RecordAdFilter(context.Background(), "lz", true); err != nil {
		t.Fatal(err)
	}
	if hr.upserts != 0 {
		t.Fatalf("ok=true 不应写库, got upserts=%d", hr.upserts)
	}
	if err := ms.RecordAdFilter(context.Background(), "lz", false); err != nil {
		t.Fatal(err)
	}
	if hr.upserts != 1 || hr.last == nil || hr.last.AdFilterOk == nil || *hr.last.AdFilterOk {
		t.Fatalf("失败上报应写 AdFilterOk=false, got %+v", hr.last)
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
