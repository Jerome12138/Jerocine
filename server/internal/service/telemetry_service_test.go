package service

import (
	"context"
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
		{"撞IP via origin", "43.156.77.237", "http://43.156.77.237", "", "Mozilla/5.0", true},
		{"撞域名 via origin", "evil.com", "https://evil.com", "", "Mozilla/5.0", true},
		{"撞IP via host no-origin", "43.156.77.237", "", "", "Mozilla/5.0", true},
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

func TestIngest_CapsAndStamps(t *testing.T) {
	ft := &fakeTelemetry{}
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
	if err := svc.ResolveIssue(context.Background(), "api", "/films", "slow", "abc123"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ft.upserted == nil || !ft.upserted.Resolved || ft.upserted.ResolvedCommitId != "abc123" {
		t.Fatalf("resolve upsert: %+v", ft.upserted)
	}
	// unresolve 时 reopened_count 应基于已存在记录 +1
	ft.existing = &entity.TelemetryIssueResolution{ReopenedCount: 2}
	if err := svc.UnresolveIssue(context.Background(), "api", "/films", "slow"); err != nil {
		t.Fatalf("unresolve: %v", err)
	}
	if ft.upserted.Resolved || ft.upserted.ReopenedCount != 3 {
		t.Fatalf("unresolve upsert: %+v", ft.upserted)
	}
}
