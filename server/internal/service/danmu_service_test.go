package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"server/internal/cache"
)

// TestDanmuBilibiliViaService 模拟 danmu-service 两跳响应，验证转发、契约映射与降级。
//
// 覆盖：
//  1. episodes 返回 matched.episodeId → comments 整集聚合 → []DanmuComment 毫秒原样映射；
//  2. danmu-service 无匹配集（matched=null）→ 空弹幕 + 无错误（静默降级）；
//  3. danmu-service 不可用（500）→ 空弹幕 + 无错误（不阻塞播放）。
//
// 注意：测试走真实 Redis 缓存层 GetOrLoadTTL；无 Redis 环境时若 test 包已有
// 全局初始化跳过逻辑则相应调整（本地门禁以 go build/vet/test 为准）。
func TestDanmuBilibiliViaService(t *testing.T) {
	episodeID := "seasonId=44847&aid=867288412&cid=1091578122"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/episodes"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"provider": "bilibili",
					"episodes": []any{},
					"matched": map[string]any{
						"episodeId": episodeID,
					},
				},
			})
		case strings.Contains(r.URL.Path, "/comments"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": []DanmuComment{
					{Text: "中国科幻电影的新征程！", Time: 49953, Color: 16777215, Mode: 1},
					{Text: "好耶", Time: 50100, Color: 0, Mode: 4},
				},
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	svc := NewDanmuService(nil, nil, srv.URL)
	comments, err := svc.DanmuBilibili(context.Background(), "44847", 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("comments = %d, want 2", len(comments))
	}
	if comments[0].Text != "中国科幻电影的新征程！" || comments[0].Time != 49953 {
		t.Fatalf("first comment 映射错误: %+v", comments[0])
	}
	if comments[1].Mode != 4 {
		t.Fatalf("mode 映射错误: %+v", comments[1])
	}
}

func TestDanmuBilibiliNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"provider": "bilibili",
				"episodes": []any{},
				"matched":  nil, // 无匹配集
			},
		})
	}))
	defer srv.Close()

	svc := NewDanmuService(nil, nil, srv.URL)
	comments, err := svc.DanmuBilibili(context.Background(), "44847", 99)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0", len(comments))
	}
}

func TestDanmuBilibiliServiceDown(t *testing.T) {
	// 直接返回 500 的"不可用"服务
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := NewDanmuService(nil, nil, srv.URL)
	comments, err := svc.DanmuBilibili(context.Background(), "44847", 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 (降级为空弹幕)", len(comments))
	}
}

// TestEpKeyOf 复合 episodeId → 稳定 16 字符键片段（8.6：不得直接拼 Redis 键）。
func TestEpKeyOf(t *testing.T) {
	id := "seasonId=44847&aid=867288412&cid=1091578122"
	k := cache.EpKeyOf(id)
	if len(k) != 16 {
		t.Fatalf("EpKeyOf 长度 = %d, want 16", len(k))
	}
	if k2 := cache.EpKeyOf(id); k2 != k {
		t.Fatalf("EpKeyOf 不稳定: %s vs %s", k, k2)
	}
	seg := cache.KeyDanmuSeg("tencent", k, "1")
	if !strings.HasPrefix(seg, "v1:danmu:seg:tencent:") {
		t.Fatalf("KeyDanmuSeg 前缀错误: %s", seg)
	}
}

// TestDanmuTencentChain 腾讯路径 B 三段链路：episodes 匹配 → segments 索引 → comments 单段。
// 验证复合串 encodeURIComponent 透传、秒(startTime)字段原样、单段内容映射毫秒契约。
func TestDanmuTencentChain(t *testing.T) {
	episodeID := "cid=mzc002003kpyd2m&vid=s4102pomt63"
	var gotEpisodesPath, gotSegmentsPath, gotCommentsPath bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/episodes"):
			gotEpisodesPath = true
			// 复合串必须被 encodeURIComponent：query 里不能出现裸 '&' 分隔 episodeId 结构
			q := r.URL.Query()
			if q.Get("episode") != "1" || q.Get("resourceId") != "mzc002003kpyd2m" {
				t.Errorf("episodes query 解析错误: %v", q)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"provider": "tencent",
					"episodes": []any{},
					"matched":  map[string]any{"episodeId": episodeID},
				},
			})
		case strings.Contains(r.URL.Path, "/segments"):
			gotSegmentsPath = true
			q := r.URL.Query()
			if q.Get("episodeId") != episodeID {
				t.Errorf("segments episodeId 未 encodeURIComponent 透传: %q", q.Get("episodeId"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": []TencentSegment{
					{Provider: "tencent", StartTime: 0, SegmentID: "0"},
					{Provider: "tencent", StartTime: 201.5, SegmentID: "201"},
					{Provider: "tencent", StartTime: 401.0, SegmentID: "401"},
				},
			})
		case strings.Contains(r.URL.Path, "/comments"):
			gotCommentsPath = true
			q := r.URL.Query()
			if q.Get("episodeId") != episodeID || q.Get("segmentId") != "201" {
				t.Errorf("comments 参数透传错误: episodeId=%q segmentId=%q", q.Get("episodeId"), q.Get("segmentId"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"data": []DanmuComment{{Text: "腾讯弹幕", Time: 201500, Color: 16777215, Mode: 1}},
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	svc := NewDanmuService(nil, nil, srv.URL)

	matched, found, err := svc.DanmuTencentEpisodes(context.Background(), "mzc002003kpyd2m", 1)
	if err != nil {
		t.Fatalf("episodes err: %v", err)
	}
	if !found || matched.EpisodeID != episodeID {
		t.Fatalf("matched = %+v found=%v", matched, found)
	}

	segments, err := svc.DanmuTencentSegments(context.Background(), episodeID)
	if err != nil {
		t.Fatalf("segments err: %v", err)
	}
	if len(segments) != 3 || segments[1].StartTime != 201.5 || segments[1].SegmentID != "201" {
		t.Fatalf("segments 映射错误: %+v", segments)
	}

	comments, err := svc.DanmuTencentComments(context.Background(), episodeID, "201")
	if err != nil {
		t.Fatalf("comments err: %v", err)
	}
	if len(comments) != 1 || comments[0].Text != "腾讯弹幕" || comments[0].Time != 201500 {
		t.Fatalf("comments 映射错误: %+v", comments)
	}
	if !gotEpisodesPath || !gotSegmentsPath || !gotCommentsPath {
		t.Fatalf("未全部打到 fake danmu-service: eps=%v seg=%v cmt=%v", gotEpisodesPath, gotSegmentsPath, gotCommentsPath)
	}
}

// TestDanmuTencentNoMatch 无匹配集 → found=false（前端静默无弹幕，非错误）。
func TestDanmuTencentNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "data": map[string]any{"provider": "tencent", "episodes": []any{}, "matched": nil},
		})
	}))
	defer srv.Close()

	svc := NewDanmuService(nil, nil, srv.URL)
	_, found, err := svc.DanmuTencentEpisodes(context.Background(), "mzc002003kpyd2m", 99)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if found {
		t.Fatalf("found = true, want false（无匹配集）")
	}
}

// TestDanmuTencentCommentsServiceDown 单段弹幕服务不可用 → 空弹幕 + 无错误（不阻塞播放）。
func TestDanmuTencentCommentsServiceDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := NewDanmuService(nil, nil, srv.URL)
	comments, err := svc.DanmuTencentComments(context.Background(), "cid=x&vid=y", "201")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0 (降级为空弹幕)", len(comments))
	}
}
