package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
func (f *fakeSearchKW) SearchKeyword(_ context.Context, kw string, _ repository.Page) ([]entity.MovieSearch, int64, error) {
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

// ---- ClientOnly(仅端侧可达)跳过测速/健康度 ----

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

// fakeHealthRepo 记录 Upsert 调用次数, 验证 ClientOnly 源不写健康度。
type fakeHealthRepo struct {
	upserts int
}

func (f *fakeHealthRepo) Get(context.Context, string) (*entity.SourceHealth, error) {
	return nil, nil
}
func (f *fakeHealthRepo) List(context.Context) ([]entity.SourceHealth, error) { return nil, nil }
func (f *fakeHealthRepo) Upsert(context.Context, *entity.SourceHealth) error {
	f.upserts++
	return nil
}

// TestSource: ClientOnly 源直接返回占位结果(Ok=true, 占位文案), 且不写健康度(不计失败/不停采)。
func TestTestSource_ClientOnlySkipsProbeAndHealth(t *testing.T) {
	sr := &fakeSourceRepo{byId: map[string]entity.CollectSource{
		"bf": {Id: "bf", Name: "BF", ClientOnly: true},
	}}
	hr := &fakeHealthRepo{}
	ms := &ManageService{sources: sr, health: hr}
	res, err := ms.TestSource(context.Background(), "bf")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Ok || res.Message != clientOnlyMessage {
		t.Fatalf("ClientOnly 应返回占位结果: ok=%v msg=%q", res.Ok, res.Message)
	}
	if res.Probes != 0 || res.OkCount != 0 {
		t.Fatalf("ClientOnly 不应有探测计数: probes=%d ok=%d", res.Probes, res.OkCount)
	}
	if hr.upserts != 0 {
		t.Fatalf("ClientOnly 源不应写健康度, got upserts=%d", hr.upserts)
	}
}

// TestAllSources: ClientOnly 源占位通过(不写健康度), 普通源仍照常测速并写健康度。
func TestTestAllSources_ClientOnlySkipped(t *testing.T) {
	// 普通源 uri 指向回环非法端点, Probe 会快速失败 → recordHealth 写一次(失败计数)。
	sr := &fakeSourceRepo{all: []entity.CollectSource{
		{Id: "bf", Name: "BF", ClientOnly: true},
		{Id: "norm", Name: "Norm", Uri: "http://127.0.0.1:1/api"},
	}}
	hr := &fakeHealthRepo{}
	ms := &ManageService{sources: sr, health: hr, prober: spider.NewFetcherWithTimeout(time.Second)}
	out, err := ms.TestAllSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 results, got %d", len(out))
	}
	var bf, norm SourceTestResult
	for _, r := range out {
		switch r.Id {
		case "bf":
			bf = r
		case "norm":
			norm = r
		}
	}
	if !bf.Ok || bf.Message != clientOnlyMessage {
		t.Fatalf("ClientOnly 源占位异常: ok=%v msg=%q", bf.Ok, bf.Message)
	}
	if norm.Ok {
		t.Fatalf("普通失败源不应 Ok=true")
	}
	// 仅普通源写了一次健康度, ClientOnly 源未写。
	if hr.upserts != 1 {
		t.Fatalf("应只为普通源写 1 次健康度, got %d", hr.upserts)
	}
}

// clientOnlyResult 占位: Ok=true(不计失败), 文案固定。
func TestClientOnlyResult(t *testing.T) {
	r := clientOnlyResult()
	if !r.Ok || r.Message != clientOnlyMessage {
		t.Fatalf("got ok=%v msg=%q", r.Ok, r.Message)
	}
}
