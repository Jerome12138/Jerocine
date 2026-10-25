package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// fakeSiteCfgRepo 测试用内存 site_config（弹幕配置注入）。
type fakeSiteCfgRepo struct {
	cfg *entity.SiteConfig
}

func (f *fakeSiteCfgRepo) Get(ctx context.Context) (*entity.SiteConfig, error) { return f.cfg, nil }
func (f *fakeSiteCfgRepo) Save(ctx context.Context, c *entity.SiteConfig) error {
	f.cfg = c
	return nil
}

// danmuSvc 构造"后台已启用指定平台"的 DanmuService。
// 后台配置默认关闭（danmu_enabled=0），自测试改造起所有路径都过配置白名单，
// 因此用例必须显式开平台，否则 validatePathBProvider 一律 400。
// DanmuService 直读 site_config repo（不经 ConfigService 缓存），无 Redis 依赖。
func danmuSvc(url string, providers ...string) *DanmuService {
	repo := &fakeSiteCfgRepo{cfg: &entity.SiteConfig{
		DanmuEnabled:   1,
		DanmuProviders: entity.StringSlice(providers),
	}}
	return NewDanmuService(nil, nil, url, repo)
}

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

	svc := danmuSvc(srv.URL, "bilibili")
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

	svc := danmuSvc(srv.URL, "bilibili")
	comments, err := svc.DanmuBilibili(context.Background(), "44847", 99)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0", len(comments))
	}
}

// TestDanmuBilibiliServiceDown danmu-service 不可用 → 返回 error（不写负缓存，发布窗口后自愈）；
// handler 对 error 返 500，前端 .catch 静默无弹幕（不阻塞播放）。
func TestDanmuBilibiliServiceDown(t *testing.T) {
	// 直接返回 500 的"不可用"服务
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := danmuSvc(srv.URL, "bilibili")
	comments, err := svc.DanmuBilibili(context.Background(), "44847", 1)
	if err == nil {
		t.Fatal("服务不可用应返回 error（调用失败不缓存，区别于业务无数据的负缓存）")
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0", len(comments))
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
				"data": []DanmuSegment{
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

	svc := danmuSvc(srv.URL, "tencent")

	matched, found, err := svc.DanmuEpisodes(context.Background(), "tencent", "mzc002003kpyd2m", 1)
	if err != nil {
		t.Fatalf("episodes err: %v", err)
	}
	if !found || matched.EpisodeID != episodeID {
		t.Fatalf("matched = %+v found=%v", matched, found)
	}

	segments, err := svc.DanmuSegments(context.Background(), "tencent", episodeID)
	if err != nil {
		t.Fatalf("segments err: %v", err)
	}
	if len(segments) != 3 || segments[1].StartTime != 201.5 || segments[1].SegmentID != "201" {
		t.Fatalf("segments 映射错误: %+v", segments)
	}

	comments, err := svc.DanmuComments(context.Background(), "tencent", episodeID, "201")
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

	svc := danmuSvc(srv.URL, "tencent")
	_, found, err := svc.DanmuEpisodes(context.Background(), "tencent", "mzc002003kpyd2m", 99)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if found {
		t.Fatalf("found = true, want false（无匹配集）")
	}
}

// TestDanmuTencentCommentsServiceDown 单段弹幕服务不可用 → 返回 error（不写负缓存）。
func TestDanmuTencentCommentsServiceDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := danmuSvc(srv.URL, "tencent")
	comments, err := svc.DanmuComments(context.Background(), "tencent", "cid=x&vid=y", "201")
	if err == nil {
		t.Fatal("服务不可用应返回 error（调用失败不缓存，区别于业务无数据的负缓存）")
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %d, want 0", len(comments))
	}
}

// TestDanmuPathBProviderWhitelist 路径 B provider 白名单（后台配置驱动）：
// 只启用 tencent 时，iqiyi/youku/mgtv/bilibili/空串 → ErrInvalidArgument（handler 经 dto.Fail
// 映射 400），且不发出任何 HTTP 请求 —— validatePathBProvider 在拼 URL 前拦停，
// 顺带封死"路由参数进请求路径"的注入面。白名单内的 tencent 不被拦。
// 另验：后台未启用任何平台（danmuSvc 不给 providers → 白名单空）时 tencent 也 400。
func TestDanmuPathBProviderWhitelist(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer srv.Close()

	svc := danmuSvc(srv.URL, "tencent")
	for _, p := range []string{"iqiyi", "youku", "mgtv", "bilibili", ""} {
		if _, _, err := svc.DanmuEpisodes(context.Background(), p, "x", 1); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("DanmuEpisodes(%q) err = %v, want ErrInvalidArgument", p, err)
		}
		if _, err := svc.DanmuSegments(context.Background(), p, "x"); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("DanmuSegments(%q) err = %v, want ErrInvalidArgument", p, err)
		}
		if _, err := svc.DanmuComments(context.Background(), p, "x", "201"); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("DanmuComments(%q) err = %v, want ErrInvalidArgument", p, err)
		}
	}
	if hits != 0 {
		t.Fatalf("白名单外的 provider 不应打到 danmu-service，实测 %d 次", hits)
	}
	// tencent 在白名单内：应穿过校验打到 fake（空响应 → 非 JSON 错误，但不是 ErrInvalidArgument）
	if _, _, err := svc.DanmuEpisodes(context.Background(), "tencent", "x", 1); errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatal("tencent 在白名单内，不应被 validatePathBProvider 拦停")
	}
	// 未启用任何平台（后台配置默认关闭）→ tencent 也 400
	off := danmuSvc(srv.URL)
	if _, _, err := off.DanmuEpisodes(context.Background(), "tencent", "x", 1); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("未启用任何平台时 tencent err = %v, want ErrInvalidArgument", err)
	}
}

// TestDanmuBilibiliDisabled 后台未启用 bilibili → 路径 A（整集聚合）也 400（与路径 B 同一白名单）。
func TestDanmuBilibiliDisabled(t *testing.T) {
	svc := danmuSvc("http://127.0.0.1:1", "tencent")
	if _, err := svc.DanmuBilibili(context.Background(), "44847", 1); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bilibili 未启用时 DanmuBilibili err = %v, want ErrInvalidArgument", err)
	}
}

// countingMovie 计数 fake：只实现 GetByMid（其余嵌接口 nil，调了会 panic）。
type countingMovie struct {
	repository.MovieRepository
	onGet func()
	m     *entity.Movie
}

func (m *countingMovie) GetByMid(_ context.Context, _ int64) (*entity.Movie, error) {
	if m.onGet != nil {
		m.onGet()
	}
	if m.m == nil {
		return &entity.Movie{Mid: 1}, nil
	}
	return m.m, nil
}

// TestDanmuSourceDisabledSkipsUpstream 后台未启用弹幕（总开关关 / 白名单空或 nil）→
// danmuEnabledProviders 返回 nil → loadDanmuSource 早退：不打 movie repo、不打豆瓣。
// client 传 nil：若代码越过早退走到 DoubanVendors 会 panic —— 顺带证明分支已拦停。
func TestDanmuSourceDisabledSkipsUpstream(t *testing.T) {
	setupCache(t)
	var hits int
	movie := &countingMovie{onGet: func() { hits++ }}
	for _, cfg := range []*entity.SiteConfig{
		{DanmuEnabled: 0, DanmuProviders: entity.StringSlice{"tencent"}}, // 总开关关
		{DanmuEnabled: 1, DanmuProviders: entity.StringSlice{}},          // 空白名单
		{DanmuEnabled: 1, DanmuProviders: nil},                           // nil 白名单
	} {
		svc := NewDanmuService(movie, nil, "http://127.0.0.1:1", &fakeSiteCfgRepo{cfg: cfg})
		if _, err := svc.DanmuSource(context.Background(), 1); err != nil {
			t.Fatalf("DanmuSource(未启用) err = %v, want nil（静默无弹幕）", err)
		}
	}
	if hits != 0 {
		t.Fatalf("未启用时不应打 movie repo，实测 %d 次", hits)
	}
}

// TestDanmuSourceDbIdZeroSkipsUpstream db_id<=0 → 早退（不打豆瓣）。
func TestDanmuSourceDbIdZeroSkipsUpstream(t *testing.T) {
	setupCache(t)
	svc := danmuSvc("http://127.0.0.1:1", "tencent")
	svc.movie = &countingMovie{} // Mid=1, DbId=0
	if _, err := svc.DanmuSource(context.Background(), 1); err != nil {
		t.Fatalf("DanmuSource(db_id=0) err = %v, want nil", err)
	}
}

// TestSaveDanmuConfig 弹幕后台配置读写（manage 端点 service 层，铁律三同步第三点）：
// 入参校验（enabled 仅 0/1 / 未知平台拒绝 / 开而空白名单拒绝 / 去重）+ 落库读回，
// 并验证保存后 v1:danmu:src:* 负缓存被清（否则旧"未启用"负缓存继续挡 2h）。
func TestSaveDanmuConfig(t *testing.T) {
	setupCache(t)
	repo := &fakeSiteCfgRepo{cfg: &entity.SiteConfig{Id: 1}}
	m := &ManageService{siteCfg: repo}
	ctx := context.Background()

	v, err := m.GetDanmuConfig(ctx)
	if err != nil {
		t.Fatalf("GetDanmuConfig: %v", err)
	}
	if v.Enabled != 0 || len(v.Providers) != 0 {
		t.Fatalf("默认应关+空白名单: %+v", v)
	}
	if strings.Join(v.All, ",") != "bilibili,tencent,iqiyi,youku" {
		t.Fatalf("平台全集与壳 ID_PREFIX 不同步: %v", v.All)
	}

	bad := []DanmuConfigInput{
		{Enabled: 2, Providers: []string{"tencent"}}, // enabled 越界
		{Enabled: 1, Providers: []string{"mgtv"}},    // 未知平台
		{Enabled: 1, Providers: []string{}},          // 开而空白名单
		{Enabled: 1, Providers: nil},
	}
	for _, in := range bad {
		if err := m.SaveDanmuConfig(ctx, in); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("SaveDanmuConfig(%+v) err = %v, want ErrInvalidArgument", in, err)
		}
	}
	if repo.cfg.DanmuEnabled != 0 || len(repo.cfg.DanmuProviders) != 0 {
		t.Fatalf("非法输入不得落库: %+v", repo.cfg)
	}

	if err := m.SaveDanmuConfig(ctx, DanmuConfigInput{
		Enabled: 1, Providers: []string{"tencent", "bilibili", "tencent"}, // 去重
	}); err != nil {
		t.Fatalf("SaveDanmuConfig: %v", err)
	}
	v, err = m.GetDanmuConfig(ctx)
	if err != nil {
		t.Fatalf("GetDanmuConfig: %v", err)
	}
	if v.Enabled != 1 || strings.Join([]string(v.Providers), ",") != "tencent,bilibili" {
		t.Fatalf("落库读回不一致: %+v", v)
	}

	// 负缓存清理：先写一条"未启用"负缓存，保存后应被 DelByPattern 清掉
	if err := cache.Set(ctx, cache.KeyDanmuSource(1), danmuEmptyTTL, DanmuSourceResp{}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if err := m.SaveDanmuConfig(ctx, DanmuConfigInput{Enabled: 0}); err != nil {
		t.Fatalf("SaveDanmuConfig(关): %v", err)
	}
	var loaderCalled bool
	if _, _, _ = cache.GetOrLoadTTL(ctx, cache.KeyDanmuSource(1), danmuSourceTTL, danmuEmptyTTL,
		func(context.Context) (DanmuSourceResp, bool, error) {
			loaderCalled = true
			return DanmuSourceResp{}, false, nil
		}); !loaderCalled {
		t.Fatal("负缓存未清：GetOrLoadTTL 仍命中旧值（SaveDanmuConfig 应 DelByPattern v1:danmu:src:*）")
	}
}
