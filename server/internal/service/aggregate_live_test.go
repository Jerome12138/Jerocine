package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"server/internal/danmu"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/live"
)

// writeJSON 测试假上游统一 JSON 输出。
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("write json: %v", err)
	}
}

// aggregateSvc 构造只测聚合链路的 DanmuService（movie/client/danmu-service 均不参与）。
func aggregateSvc(base, token string, danmuEnabled int) *DanmuService {
	return NewDanmuService(nil, nil, danmu.NewDandan(base, token), "http://unused",
		&fakeSiteCfgRepo{cfg: &entity.SiteConfig{DanmuEnabled: danmuEnabled}})
}

// TestDanmuAggregate 聚合弹幕（dandanplay v2 兼容）服务级链路：
// 假上游依次处理 /{token}/api/v2/search/anime、/bangumi/{id}、/comment/{id}。
//
// 覆盖：
//
//	a) 正常命中：搜索→作品打分→分集定位→弹幕规范化（毫秒/mode/color），三段缓存后二调不再打上游；
//	b) 查无作品（search 空 animes）→ domain.ErrNotFound，只打 search；
//	c) 查无集（bangumi 有列表但 episode 越界，PickEpisode 三路全落空）→ ErrNotFound，不打 comment；
//	d) 后台 danmu_enabled=0 → 空弹幕（非 nil 空数组）且不打上游；
//	e) 未配置 token → Enabled()=false → 空弹幕不报错且不打上游。
//
// 负缓存 2h：b/c 用不同 title+year（kwKey = 归一化标题|year），c 用独立 animeId，
// 避免同键互串。cache 包全局，经 setupCache(t) 指向 miniredis（同 danmu_service_test.go 范式）。
func TestDanmuAggregate(t *testing.T) {
	setupCache(t)
	ctx := context.Background()

	// ---- a) 正常命中 + 三段缓存 ----
	var searchHits, bangumiHits, commentHits int
	srvHit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/tok/") {
			t.Errorf("token 未作路径前缀: %s", r.URL.Path)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v2/search/anime"):
			if kw := r.URL.Query().Get("keyword"); kw != "庆余年" {
				t.Errorf("search keyword = %q, want 庆余年", kw)
			}
			searchHits++
			writeJSON(t, w, map[string]any{"success": true, "animes": []map[string]any{
				{"animeId": 6101, "animeTitle": "庆余年 第一季", "episodeCount": 46},
				{"animeId": 6102, "animeTitle": "庆余年（2024）第二季", "episodeCount": 36},
			}})
		case strings.HasSuffix(r.URL.Path, "/api/v2/bangumi/6102"):
			bangumiHits++
			writeJSON(t, w, map[string]any{"success": true, "bangumi": map[string]any{
				"episodes": []map[string]any{
					{"episodeId": 910001, "episodeTitle": "第1集", "episodeNumber": "1"},
					{"episodeId": 910002, "episodeTitle": "第2集", "episodeNumber": "2"},
					{"episodeId": 910003, "episodeTitle": "第3集", "episodeNumber": "3"},
				},
			}})
		case strings.HasSuffix(r.URL.Path, "/api/v2/comment/910003"):
			commentHits++
			writeJSON(t, w, map[string]any{"success": true, "count": 3, "comments": []map[string]any{
				{"p": "12.5,1,16777215,0", "m": "前方高能"},
				{"p": "65,4,16738740,0", "m": "名场面"},
				{"p": "abc,1,2,3", "m": "坏数据"}, // 时间解析失败 → 跳过
			}})
		default:
			t.Errorf("未预期的上游请求: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srvHit.Close()

	svc := aggregateSvc(srvHit.URL, "tok", 1)
	resp, err := svc.DanmuAggregate(ctx, "庆余年", 2024, 3)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(resp.Comments) != 2 {
		t.Fatalf("comments = %d, want 2（坏数据行应被跳过）", len(resp.Comments))
	}
	if c := resp.Comments[0]; c.Text != "前方高能" || c.Time != 12500 || c.Mode != 1 || c.Color != 16777215 {
		t.Fatalf("comment[0] 映射错误（秒→毫秒/mode/color）: %+v", c)
	}
	if c := resp.Comments[1]; c.Text != "名场面" || c.Time != 65000 || c.Mode != 4 || c.Color != 16738740 {
		t.Fatalf("comment[1] 映射错误: %+v", c)
	}
	if searchHits != 1 || bangumiHits != 1 || commentHits != 1 {
		t.Fatalf("首次调用应打满三跳上游: search=%d bangumi=%d comment=%d", searchHits, bangumiHits, commentHits)
	}

	// 第二次调用：搜索/分集/内容三段正缓存全命中，不再打上游
	resp2, err := svc.DanmuAggregate(ctx, "庆余年", 2024, 3)
	if err != nil {
		t.Fatalf("second call err: %v", err)
	}
	if len(resp2.Comments) != 2 || resp2.Comments[0].Text != "前方高能" {
		t.Fatalf("second call comments = %+v, want 与首调一致", resp2.Comments)
	}
	if searchHits != 1 || bangumiHits != 1 || commentHits != 1 {
		t.Fatalf("缓存命中后不应再打上游: search=%d bangumi=%d comment=%d", searchHits, bangumiHits, commentHits)
	}

	// ---- b) 查无作品 → ErrNotFound（只打 search） ----
	var hitsNoAnime int
	srvNoAnime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsNoAnime++
		writeJSON(t, w, map[string]any{"success": true, "animes": []any{}})
	}))
	defer srvNoAnime.Close()

	if _, err := aggregateSvc(srvNoAnime.URL, "tok", 1).
		DanmuAggregate(ctx, "查无此剧", 2021, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("查无作品 err = %v, want ErrNotFound", err)
	}
	if hitsNoAnime != 1 {
		t.Fatalf("查无作品应只打 search 一次，实测 %d 次", hitsNoAnime)
	}

	// ---- c) 查无集（episode 越界，PickEpisode 精确/标题/下标三路全落空）→ ErrNotFound ----
	var hitsNoEp int
	srvNoEp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsNoEp++
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v2/search/anime"):
			writeJSON(t, w, map[string]any{"success": true, "animes": []map[string]any{
				{"animeId": 6201, "animeTitle": "单集试播剧", "episodeCount": 1},
			}})
		case strings.HasSuffix(r.URL.Path, "/api/v2/bangumi/6201"):
			writeJSON(t, w, map[string]any{"success": true, "bangumi": map[string]any{
				"episodes": []map[string]any{
					{"episodeId": 920001, "episodeTitle": "第1集", "episodeNumber": "1"},
				},
			}})
		default:
			t.Errorf("查无集不应打到 comment 接口: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srvNoEp.Close()

	// episode=99：episodeNumber 精确不中、"第1集" 标题不中、99 > len(eps)=1 下标回退不中
	if _, err := aggregateSvc(srvNoEp.URL, "tok", 1).
		DanmuAggregate(ctx, "单集试播剧", 2022, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("查无集 err = %v, want ErrNotFound", err)
	}
	if hitsNoEp != 2 {
		t.Fatalf("查无集应只打 search+bangumi 两跳，实测 %d 次", hitsNoEp)
	}

	// ---- d) 后台总开关关闭 → 空弹幕且不打上游 ----
	var hitsDisabled int
	srvDisabled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsDisabled++
	}))
	defer srvDisabled.Close()

	respOff, err := aggregateSvc(srvDisabled.URL, "tok", 0).
		DanmuAggregate(ctx, "开关关闭剧", 2023, 1)
	if err != nil {
		t.Fatalf("danmu_enabled=0 err = %v, want nil（静默降级）", err)
	}
	if respOff.Comments == nil || len(respOff.Comments) != 0 {
		t.Fatalf("danmu_enabled=0 comments = %#v, want 非 nil 空数组（JSON 契约 comments:[]）", respOff.Comments)
	}
	if hitsDisabled != 0 {
		t.Fatalf("后台关闭不应打上游，实测 %d 次", hitsDisabled)
	}

	// ---- e) 未配置 token → 空弹幕不报错且不打上游 ----
	var hitsNoToken int
	srvNoToken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsNoToken++
	}))
	defer srvNoToken.Close()

	respNoTok, err := aggregateSvc(srvNoToken.URL, "", 1).
		DanmuAggregate(ctx, "无令牌剧", 2020, 1)
	if err != nil {
		t.Fatalf("未配置 token err = %v, want nil", err)
	}
	if respNoTok.Comments == nil || len(respNoTok.Comments) != 0 {
		t.Fatalf("未配置 token comments = %#v, want 非 nil 空数组", respNoTok.Comments)
	}
	if hitsNoToken != 0 {
		t.Fatalf("未配置 token 不应打上游，实测 %d 次", hitsNoToken)
	}
}

// ---- 直播源 service 层 ----

// fakeLiveSourceRepo 内存直播源仓储（嵌入接口，只覆写被测方法；未覆写被调会 nil panic）。
type fakeLiveSourceRepo struct {
	repository.LiveSourceRepository
	byID    map[int64]*entity.LiveSource
	updates []*entity.LiveSource // Update 调用留痕
}

func (f *fakeLiveSourceRepo) Get(_ context.Context, id int64) (*entity.LiveSource, error) {
	s, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (f *fakeLiveSourceRepo) Update(_ context.Context, s *entity.LiveSource) error {
	cp := *s
	f.updates = append(f.updates, &cp)
	f.byID[s.Id] = &cp
	return nil
}

// TestLiveServiceChannels 直播源频道解析服务级链路（LiveService + live.Client + 假 m3u 源）：
//
//	a) LiveChannels 解析两组 EXTINF（分组/台标/URL 正确）+ 二调缓存命中不打上游；
//	b) 停用源（state=1）→ ErrNotFound 且不打上游（Get 在缓存/拉取之前拦停）；
//	c) 上游 500 → 空频道数组无错误（负缓存 5min），二调也不打上游；负缓存命中回填源 id/name；
//	d) UpdateLiveSource 缺 state → ErrInvalidArgument；带 state → 整体覆盖落 repo。
func TestLiveServiceChannels(t *testing.T) {
	setupCache(t)
	ctx := context.Background()

	m3u := "#EXTM3U\n" +
		"#EXTINF:-1 tvg-name=\"CCTV1\" tvg-logo=\"http://logo.example.com/cctv1.png\" group-title=\"央视\",CCTV-1 综合\n" +
		"http://stream.example.com/cctv1.m3u8\n" +
		"#EXTINF:-1 group-title=\"卫视\",湖南卫视\n" +
		"http://stream.example.com/hntv.m3u8\n"

	var m3uHits int
	m3uSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m3uHits++
		_, _ = w.Write([]byte(m3u))
	}))
	defer m3uSrv.Close()

	var deadHits int
	deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadHits++
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer deadSrv.Close()

	repo := &fakeLiveSourceRepo{byID: map[int64]*entity.LiveSource{
		1: {Id: 1, Name: "聚合源", URL: m3uSrv.URL + "/list.m3u", State: entity.LiveSourceEnabled},
		2: {Id: 2, Name: "停用源", URL: m3uSrv.URL + "/off.m3u", State: entity.LiveSourceDisabled},
		3: {Id: 3, Name: "死源", URL: deadSrv.URL + "/dead.m3u", State: entity.LiveSourceEnabled},
	}}
	svc := NewLiveService(repo, live.NewClient())

	// ---- a) 正常解析 + 缓存 ----
	resp, err := svc.LiveChannels(ctx, 1)
	if err != nil {
		t.Fatalf("LiveChannels: %v", err)
	}
	if resp.ID != 1 || resp.Name != "聚合源" {
		t.Fatalf("源信息回填错误: %+v", resp)
	}
	if len(resp.Channels) != 2 {
		t.Fatalf("channels = %d, want 2: %+v", len(resp.Channels), resp.Channels)
	}
	c0, c1 := resp.Channels[0], resp.Channels[1]
	if c0.Name != "CCTV-1 综合" || c0.Group != "央视" || c0.URL != "http://stream.example.com/cctv1.m3u8" ||
		c0.Logo != "http://logo.example.com/cctv1.png" {
		t.Fatalf("channel[0] 解析错误: %+v", c0)
	}
	if c1.Name != "湖南卫视" || c1.Group != "卫视" || c1.URL != "http://stream.example.com/hntv.m3u8" {
		t.Fatalf("channel[1] 解析错误: %+v", c1)
	}
	if m3uHits != 1 {
		t.Fatalf("首次调用应拉一次源站，实测 %d 次", m3uHits)
	}

	resp2, err := svc.LiveChannels(ctx, 1)
	if err != nil {
		t.Fatalf("second LiveChannels: %v", err)
	}
	if len(resp2.Channels) != 2 || resp2.Channels[0].Name != "CCTV-1 综合" {
		t.Fatalf("second channels = %+v, want 与首调一致", resp2.Channels)
	}
	if m3uHits != 1 {
		t.Fatalf("30min 正缓存命中后不应再拉源站，实测 %d 次", m3uHits)
	}

	// ---- b) 停用源 → ErrNotFound 且不打上游 ----
	if _, err := svc.LiveChannels(ctx, 2); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("停用源 err = %v, want ErrNotFound", err)
	}
	if m3uHits != 1 {
		t.Fatalf("停用源不应拉上游，实测 m3u 源被打 %d 次", m3uHits)
	}

	// ---- c) 上游 500 → 空频道数组（负缓存），二调也不打上游 ----
	respDead, err := svc.LiveChannels(ctx, 3)
	if err != nil {
		t.Fatalf("死源 err = %v, want nil（拉取失败负缓存，不阻塞）", err)
	}
	if respDead.Channels == nil || len(respDead.Channels) != 0 {
		t.Fatalf("死源 channels = %#v, want 非 nil 空数组", respDead.Channels)
	}
	if respDead.ID != 3 || respDead.Name != "死源" {
		t.Fatalf("死源响应应回填源信息: %+v", respDead)
	}
	if deadHits != 1 {
		t.Fatalf("死源首次调用应拉一次上游，实测 %d 次", deadHits)
	}

	respDead2, err := svc.LiveChannels(ctx, 3)
	if err != nil {
		t.Fatalf("死源二调 err = %v, want nil（负缓存命中）", err)
	}
	if respDead2.Channels == nil || len(respDead2.Channels) != 0 {
		t.Fatalf("死源二调 channels = %#v, want 非 nil 空数组", respDead2.Channels)
	}
	if deadHits != 1 {
		t.Fatalf("负缓存命中后不应再拉源站，实测 %d 次", deadHits)
	}

	// ---- d) UpdateLiveSource：缺 state → ErrInvalidArgument；带 state → 整体覆盖 ----
	if err := svc.UpdateLiveSource(ctx, 1, LiveSourceInput{Name: "改名", URL: "http://x/new.m3u"}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("缺 state err = %v, want ErrInvalidArgument", err)
	}
	if len(repo.updates) != 0 {
		t.Fatalf("非法输入不得落 repo: %+v", repo.updates)
	}

	off := int8(entity.LiveSourceDisabled)
	in := LiveSourceInput{Name: "改名源", URL: "http://x/new.m3u", Remark: " 备注 ", Sort: 7, State: &off}
	if err := svc.UpdateLiveSource(ctx, 1, in); err != nil {
		t.Fatalf("UpdateLiveSource: %v", err)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("Update 应落 repo 一次，实测 %d 次", len(repo.updates))
	}
	got := repo.byID[1]
	if got.Id != 1 || got.Name != "改名源" || got.URL != "http://x/new.m3u" ||
		got.State != entity.LiveSourceDisabled || got.Sort != 7 || got.Remark != "备注" {
		t.Fatalf("整体覆盖字段不符: %+v", got)
	}
}
