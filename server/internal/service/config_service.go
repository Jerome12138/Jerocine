package service

import (
	"context"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ConfigService 公开配置读取(站点配置 / APK 版本), 带缓存。
type ConfigService struct {
	siteCfg  repository.SiteConfigRepository
	versions repository.AppVersionRepository
}

func NewConfigService(siteCfg repository.SiteConfigRepository, versions repository.AppVersionRepository) *ConfigService {
	return &ConfigService{siteCfg: siteCfg, versions: versions}
}

func (s *ConfigService) SiteConfig(ctx context.Context) (*entity.SiteConfig, error) {
	v, _, err := cache.GetOrLoad(ctx, cache.KeyCfgSite, time.Hour, func(ctx context.Context) (*entity.SiteConfig, bool, error) {
		c, err := s.siteCfg.Get(ctx)
		if err != nil {
			return nil, false, err
		}
		return c, true, nil
	})
	return v, err
}

// LatestVersion 取某渠道最新版本; 不存在则兜底一个 v1.0(契约要求不报错)。
func (s *ConfigService) LatestVersion(ctx context.Context, channel int8) *entity.AppVersion {
	v, err := s.versions.Latest(ctx, channel)
	if err != nil || v == nil {
		if err != nil && err != domain.ErrNotFound {
			// 真错误也兜底, 避免客户端升级检查 500
		}
		return &entity.AppVersion{Channel: channel, VersionCode: 1, VersionName: "1.0.0"}
	}
	return v
}
