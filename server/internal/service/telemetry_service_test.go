package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type fakeTelemetry struct {
	repository.TelemetryRepository
	inserted   []entity.TelemetryEvent
	upserted   *entity.TelemetryIssueResolution
	existing   *entity.TelemetryIssueResolution
	issues     []repository.TelemetryIssueAgg
	reso       map[string]*entity.TelemetryIssueResolution
}

func (f *fakeTelemetry) BatchInsert(_ context.Context, list []entity.TelemetryEvent) error {
	f.inserted = append(f.inserted, list...)
	return nil
}
func (f *fakeTelemetry) GetResolution(context.Context, string) (*entity.TelemetryIssueResolution, error) {
	if f.existing == nil {
		return nil, nil
	}
	return f.existing, nil
}
func (f *fakeTelemetry) UpsertResolution(_ context.Context, r *entity.TelemetryIssueResolution) error {
	f.upserted = r
	return nil
}
func (f *fakeTelemetry) ListErrorIssues(context.Context, int64, string) ([]repository.TelemetryIssueAgg, error) {
	return f.issues, nil
}
func (f *fakeTelemetry) GetResolutions(_ context.Context, hashes []string) (map[string]*entity.TelemetryIssueResolution, error) {
	m := make(map[string]*entity.TelemetryIssueResolution, len(hashes))
	for _, h := range hashes {
		if r, ok := f.reso[h]; ok {
			m[h] = r
		}
	}
	return m, nil
}

func TestIsNoise(t *testing.T) {
	svc := NewTelemetryService(&fakeTelemetry{}, nil, nil, []string{"jerocine.art"}, nil)
	cases := []struct {
		name                  string
		host, origin, ref, ua string
		want                  bool
	}{
		{"legit web", "jerocine.art", "https://jerocine.art", "https://jerocine.art/index", "Mozilla/5.0 Chrome", false},
		{"legit subdomain", "www.jerocine.art", "https://www.jerocine.art", "", "Mozilla/5.0", false},
		{"referer fallback legit", "jerocine.art", "", "https://jerocine.art/play", "Mozilla/5.0", false},
		{"撞IP via origin", "203.0.113.1", "http://203.0.113.1", "", "Mozilla/5.0", true},
		{"撞域名 via origin", "evil.com", "https://evil.com", "", "Mozilla/5.0", true},
		{"撞IP via host no-origin", "203.0.113.1", "", "", "Mozilla/5.0", true},
		{"bot ua googlebot", "jerocine.art", "https://jerocine.art", "", "Googlebot/2.1", true},
		{"empty ua", "jerocine.art", "https://jerocine.art", "", "", true},
		{"curl ua", "jerocine.art", "https://jerocine.art", "", "curl/8.0", true},
	}
	for _, c := range cases {
		if got := svc.IsNoise(context.Background(), c.host, c.origin, c.ref, c.ua); got != c.want {
			t.Errorf("%s: IsNoise=%v want %v", c.name, got, c.want)
		}
	}
}

// 变异验证锚点: 去掉 Ingest 里的 clampRunes 后本用例变红(复现 Error 1406 整批丢失)。
func TestIngest_ClampsOversizeFields(t *testing.T) {
	ft := &fakeTelemetry{}
	svc := NewTelemetryService(ft, nil, nil, []string{"jerocine.art"}, nil)
	longLabel := strings.Repeat("VIDEOJS: ERROR: (CODE:4 MEDIA_ERR_SRC_NOT_SUPPORTED) ", 10) // 510 runes
	longUA := strings.Repeat("Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X)", 16) // 832 runes
	longSession := strings.Repeat("s_abcdef0123456789", 6)                                 // 120 runes
	longCJK := strings.Repeat("弹", 300)                                                    // 300 runes, 多字节按字符计
	err := svc.Ingest(context.Background(), []IngestEvent{
		{Category: "error", Path: "/play", Label: longLabel, SessionId: longSession},
		{Category: longCJK, Path: "/play", Label: "x"},
	}, nil, "1.2.3.4", longUA, "jerocine.art")
	if err != nil {
		t.Fatalf("ingest err: %v", err)
	}
	if len(ft.inserted) != 2 {
		t.Fatalf("want 2 rows, got %d", len(ft.inserted))
	}
	e := ft.inserted[0]
	if runeLen(e.Label) != telemetryLabelMax || !strings.HasSuffix(e.Label, "…") {
		t.Fatalf("label not clamped to %d with … suffix: %d runes", telemetryLabelMax, runeLen(e.Label))
	}
	if runeLen(e.SessionId) != telemetrySessionMax {
		t.Fatalf("session not clamped: %d", runeLen(e.SessionId))
	}
	if runeLen(ft.inserted[0].Ua) != telemetryUaMax {
		t.Fatalf("ua not clamped: %d", runeLen(ft.inserted[0].Ua))
	}
	if runeLen(ft.inserted[1].Category) != telemetryCategoryMax || !strings.HasSuffix(ft.inserted[1].Category, "…") {
		t.Fatalf("cjk category not clamped by runes: %d runes", runeLen(ft.inserted[1].Category))
	}
	// 边界: 恰好等于列宽不截断、无 … 后缀
	ft2 := &fakeTelemetry{}
	svc2 := NewTelemetryService(ft2, nil, nil, nil, nil)
	exact := strings.Repeat("a", telemetryLabelMax)
	_ = svc2.Ingest(context.Background(), []IngestEvent{{Category: "error", Path: "/play", Label: exact}}, nil, "1.2.3.4", "UA", "")
	if got := ft2.inserted[0].Label; got != exact {
		t.Fatalf("exact-width label should pass through unchanged")
	}
}

func TestIngest_CapsAndStamps(t *testing.T) {	ft := &fakeTelemetry{}
	svc := NewTelemetryService(ft, nil, nil, []string{"jerocine.art"}, nil)
	events := make([]IngestEvent, 250)
	for i := range events {
		events[i] = IngestEvent{Category: "page", Path: "/x"}
	}
	uid := int64(10000)
	if err := svc.Ingest(context.Background(), events, &uid, "1.2.3.4", "UA", "jerocine.art"); err != nil {
		t.Fatalf("ingest err: %v", err)
	}
	if len(ft.inserted) != maxIngestBatch {
		t.Fatalf("should cap at %d, got %d", maxIngestBatch, len(ft.inserted))
	}
	e := ft.inserted[0]
	if e.Ip != "1.2.3.4" || e.Ua != "UA" || e.UserId == nil || *e.UserId != 10000 || e.ServerTs.IsZero() {
		t.Fatalf("server-side stamping missing: %+v", e)
	}
}

func TestIssueKeyStable(t *testing.T) {
	k1 := IssueKey("api", "/films", "slow")
	k2 := IssueKey("api", "/films", "slow")
	k3 := IssueKey("api", "/films", "other")
	if k1 != k2 || k1 == k3 || len(k1) != 40 {
		t.Fatalf("issue key: %s %s %s", k1, k2, k3)
	}
}

func TestPercentile(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if percentile(v, 0.50) != 5 || percentile(v, 0.95) != 9 || percentile(v, 0.99) != 9 {
		t.Fatalf("p50=%v p95=%v p99=%v", percentile(v, 0.50), percentile(v, 0.95), percentile(v, 0.99))
	}
	if percentile(nil, 0.5) != 0 {
		t.Fatal("空数组应为 0")
	}
}

func TestClassifyIssueStatus(t *testing.T) {
	now := time.Now()
	if classifyIssueStatus(nil, now, now) != "unresolved" {
		t.Fatal("无 resolution → unresolved")
	}
	if classifyIssueStatus(&entity.TelemetryIssueResolution{Resolved: false}, now, now) != "unresolved" {
		t.Fatal("已撤销 → unresolved")
	}
	// 刚解决 1h, 事件在解决前(灰度期内)→ resolved
	r := &entity.TelemetryIssueResolution{Resolved: true, UpdatedAt: now.Add(-1 * time.Hour).UnixMilli()}
	if got := classifyIssueStatus(r, now.Add(-90*time.Minute), now); got != "resolved" {
		t.Fatalf("灰度期内应 resolved, got %s", got)
	}
	// 解决 >30 天 → expired
	old := &entity.TelemetryIssueResolution{Resolved: true, UpdatedAt: now.Add(-31 * 24 * time.Hour).UnixMilli()}
	if got := classifyIssueStatus(old, now, now); got != "expired" {
		t.Fatalf(">30天应 expired, got %s", got)
	}
	// 解决 48h 前, 事件发生在解决+24h 灰度期之后 → reopened
	r2 := &entity.TelemetryIssueResolution{Resolved: true, UpdatedAt: now.Add(-48 * time.Hour).UnixMilli()}
	if got := classifyIssueStatus(r2, now.Add(-1*time.Hour), now); got != "reopened" {
		t.Fatalf("灰度期后再触发应 reopened, got %s", got)
	}
}

func TestStatusMatches(t *testing.T) {
	// 默认/active = 未解决 + 重启
	for _, s := range []string{"", "active"} {
		if !statusMatches(s, "unresolved") || !statusMatches(s, "reopened") {
			t.Fatalf("%q 应含 unresolved/reopened", s)
		}
		if statusMatches(s, "resolved") || statusMatches(s, "expired") {
			t.Fatalf("%q 不应含 resolved/expired", s)
		}
	}
	if !statusMatches("all", "resolved") || !statusMatches("all", "expired") {
		t.Fatal("all 应全含")
	}
	if !statusMatches("resolved", "resolved") || statusMatches("resolved", "unresolved") {
		t.Fatal("resolved 精确匹配")
	}
	if !statusMatches("expired", "expired") {
		t.Fatal("expired 精确匹配")
	}
}

func TestResolveUnresolve(t *testing.T) {
	ft := &fakeTelemetry{}
	svc := NewTelemetryService(ft, nil, nil, []string{"jerocine.art"}, nil)
	if err := svc.ResolveIssue(context.Background(), "api", "/films", "slow", "abc123", "改了重试逻辑"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ft.upserted == nil || !ft.upserted.Resolved || ft.upserted.ResolvedCommitId != "abc123" || ft.upserted.Note != "改了重试逻辑" {
		t.Fatalf("resolve upsert: %+v", ft.upserted)
	}
	// unresolve 时 reopened_count 应基于已存在记录 +1, 且保留原 note
	ft.existing = &entity.TelemetryIssueResolution{ReopenedCount: 2, Note: "改了重试逻辑"}
	if err := svc.UnresolveIssue(context.Background(), "api", "/films", "slow"); err != nil {
		t.Fatalf("unresolve: %v", err)
	}
	if ft.upserted.Resolved || ft.upserted.ReopenedCount != 3 || ft.upserted.Note != "改了重试逻辑" {
		t.Fatalf("unresolve upsert: %+v", ft.upserted)
	}
}

// 变异验证锚点: 把 ErrorEvents 的 statusMatches 过滤删掉 / total 改回过滤前计数 /
// 行的 Value 改回单批值, 本用例分别变红。
func TestErrorEvents_AggregatesFiltersPaginates(t *testing.T) {
	ft := &fakeTelemetry{}
	now := time.Now()
	ft.issues = []repository.TelemetryIssueAgg{
		// unresolved: 累计 5 次 / 3 批
		{Category: "js-error", Label: "boom", TotalValue: 5, BatchCount: 3,
			Latest: entity.TelemetryEvent{Category: "js-error", Label: "boom", ServerTs: now.Add(-1 * time.Hour)}},
		// resolved(解决 24h 内未再触发)
		{Category: "js-error", Label: "fixed", TotalValue: 9, BatchCount: 2,
			Latest: entity.TelemetryEvent{Category: "js-error", Label: "fixed", ServerTs: now.Add(-2 * time.Hour)}},
		// expired(解决 >30 天)
		{Category: "js-error", Label: "old", TotalValue: 1, BatchCount: 1,
			Latest: entity.TelemetryEvent{Category: "js-error", Label: "old", ServerTs: now.Add(-1 * time.Hour)}},
	}
	ft.reso = map[string]*entity.TelemetryIssueResolution{
		IssueKey("js-error", "", "fixed"): {Resolved: true, UpdatedAt: now.Add(-24 * time.Hour).UnixMilli()},
		IssueKey("js-error", "", "old"):   {Resolved: true, UpdatedAt: now.Add(-40 * 24 * time.Hour).UnixMilli()},
	}
	svc := NewTelemetryService(ft, nil, nil, nil, nil)

	// active(默认) 只含 unresolved+reopened
	rows, total, err := svc.ErrorEvents(context.Background(), 7, "error", "active", 100, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Label != "boom" {
		t.Fatalf("active: total=%d rows=%+v", total, rows)
	}
	if rows[0].Value != 5 || rows[0].BatchCount != 3 {
		t.Fatalf("聚合值: value=%v batchCount=%v want 5/3", rows[0].Value, rows[0].BatchCount)
	}

	// all: total=过滤后总数(3), 与返回行数解耦
	rows, total, err = svc.ErrorEvents(context.Background(), 7, "error", "all", 2, 0)
	if err != nil || total != 3 || len(rows) != 2 {
		t.Fatalf("all page1: err=%v total=%d len=%d", err, total, len(rows))
	}
	rows, total, err = svc.ErrorEvents(context.Background(), 7, "error", "all", 2, 2)
	if err != nil || total != 3 || len(rows) != 1 || rows[0].Label != "old" {
		t.Fatalf("all page2: err=%v total=%d rows=%+v", err, total, rows)
	}
	// offset 越界 → 空页但 total 保留
	rows, total, err = svc.ErrorEvents(context.Background(), 7, "error", "all", 2, 99)
	if err != nil || total != 3 || len(rows) != 0 {
		t.Fatalf("all offset 越界: err=%v total=%d len=%d", err, total, len(rows))
	}

	// resolved / expired 精确过滤
	for _, c := range []struct{ status, label string }{
		{"resolved", "fixed"}, {"expired", "old"},
	} {
		rows, total, err := svc.ErrorEvents(context.Background(), 7, "error", c.status, 100, 0)
		if err != nil || total != 1 || len(rows) != 1 || rows[0].Label != c.label {
			t.Fatalf("%s: err=%v total=%d rows=%+v", c.status, err, total, rows)
		}
	}
}
