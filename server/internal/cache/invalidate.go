package cache

import "context"

// InvalidateAfterCollect 采集完成后统一失效内容缓存。
// 根治旧代码 clearCache 漏清 RelateMovie 的问题: 这里把首页/分类树/分类页/推荐/标签/详情/多源全部清掉。
func InvalidateAfterCollect(ctx context.Context) {
	Del(ctx, KeyHomeAgg, KeyCatTree, KeyCfgSources)
	DelByPattern(ctx, PatClassify)
	DelByPattern(ctx, PatRelate)
	DelByPattern(ctx, PatTagOpts)
	DelByPattern(ctx, PatMovie)
	DelByPattern(ctx, PatPlay)
}

// InvalidateMovie 单部影片变更后失效其详情/卡片/基本信息 + 受影响的聚合页。
func InvalidateMovie(ctx context.Context, mid int64) {
	Del(ctx, KeyMovieDetail(mid), KeyMovieBasic(mid), KeyMovieCard(mid), KeyRelate(mid), KeyHomeAgg)
}

// InvalidateConfig 站点配置变更后失效配置缓存。
func InvalidateConfig(ctx context.Context) {
	Del(ctx, KeyCfgSite, KeyCfgVersion, KeyCfgSources)
}

// InvalidateCategory 分类变更后失效分类树 + 首页。
func InvalidateCategory(ctx context.Context) {
	Del(ctx, KeyCatTree, KeyHomeAgg)
}
