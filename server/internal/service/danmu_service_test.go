package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
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
	return NewDanmuService(nil, nil, repo)
}

func TestEpKeyOf(t *testing.T) {
	id := "seasonId=44847&aid=867288412&cid=1091578122"
	k := cache.EpKeyOf(id)
	if len(k) != 16 {
		t.Fatalf("EpKeyOf 长度 = %d, want 16", len(k))
	}
	if k2 := cache.EpKeyOf(id); k2 != k {
		t.Fatalf("EpKeyOf 不稳定: %s vs %s", k, k2)
	}
}

// TestSaveDanmuConfig 弹幕后台配置读写（manage 端点 service 层，铁律三同步第三点）：
// 入参校验（enabled 仅 0/1 / 未知平台拒绝 / 开而空白名单拒绝 / 去重）+ 落库读回。
// 注: danmu_providers 平台白名单随旧分段链路下线已无消费方, 保存语义保留兼容。
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
}
