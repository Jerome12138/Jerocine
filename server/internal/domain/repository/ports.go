package repository

import (
	"context"

	"server/internal/domain/entity"
)

// 仓储端口 (port): service 依赖这些接口, 由 repository/mysql 实现 (依赖倒置)。
// 所有方法只接收 context (事务经 ctx 传播, 见 tx.go), 不泄漏 *gorm.DB。

// MovieRepository 影片详情主表。
type MovieRepository interface {
	GetByMid(ctx context.Context, mid int64) (*entity.Movie, error) // 未命中返回 domain.ErrMovieNotFound
	Upsert(ctx context.Context, m *entity.Movie) error
	BatchUpsert(ctx context.Context, list []entity.Movie) error
	Delete(ctx context.Context, mid int64) error
	Truncate(ctx context.Context) error
}

// SearchRepository 物化卡片/检索宽表 (读模型) + 影子表重建。
// 接口隔离便于未来把检索换成 meilisearch 等外部引擎。
type SearchRepository interface {
	GetByMid(ctx context.Context, mid int64) (*entity.MovieSearch, error)
	GetByMids(ctx context.Context, mids []int64) ([]entity.MovieSearch, error)
	// TopByPidSorted 取某一级分类下某排序维度的前 N 条 (首页区块 / 分类三榜用, 不分页不 count → 根治首页无谓 COUNT)。
	TopByPidSorted(ctx context.Context, pid int64, sort ClassifySort, limit int) ([]entity.MovieSearch, error)
	// Filter 多维筛选分页 (含 pid+sort 的可翻页浏览; 返回数据与总数)。
	Filter(ctx context.Context, spec FilterSpec, page Page) ([]entity.MovieSearch, int64, error)
	// SearchKeyword 关键字检索 (FULLTEXT ngram)。
	SearchKeyword(ctx context.Context, keyword string, page Page) ([]entity.MovieSearch, int64, error)
	// CountCreatedSince 统计 created_at(毫秒) >= sinceMillis 的影片数 (仪表盘今日/近一周新增)。
	CountCreatedSince(ctx context.Context, sinceMillis int64) (int64, error)
	// Related 相关推荐候选 (按 cid + 名称/标签, 内存抽样在 service 层)。
	Related(ctx context.Context, seed RelatedSeed, candidateLimit int) ([]entity.MovieSearch, error)
	// TagOptions 7 维筛选标签 (查询期 GROUP BY 聚合, 不再靠采集期 ZIncrBy)。
	TagOptions(ctx context.Context, pid int64) (*FilterOptions, error)

	Upsert(ctx context.Context, m *entity.MovieSearch) error
	BatchUpsert(ctx context.Context, list []entity.MovieSearch) error
	Delete(ctx context.Context, mid int64) error

	// 全量重采无空窗影子表生命周期: Begin(建 movie_search_next) → Write(批量灌) → Commit(RENAME 原子切换 + drop old)。
	ShadowBegin(ctx context.Context) error
	ShadowWrite(ctx context.Context, list []entity.MovieSearch) error
	ShadowCommit(ctx context.Context) error
	Truncate(ctx context.Context) error
}

// PlaySourceRepository 多源播放。
type PlaySourceRepository interface {
	UpsertBatch(ctx context.Context, list []entity.MoviePlaySource) error
	ListByMid(ctx context.Context, mid int64) ([]entity.MoviePlaySource, error)
	GetByMatchKeys(ctx context.Context, siteId string, matchKeys []string) ([]entity.MoviePlaySource, error)
	Truncate(ctx context.Context) error
	// CountBySite 各采集源已采集播放源行数(已采集片数), 一次 GROUP BY site_id。
	CountBySite(ctx context.Context) (map[string]int64, error)
}

// CategoryRepository 分类。
type CategoryRepository interface {
	All(ctx context.Context) ([]entity.Category, error)
	Get(ctx context.Context, id int64) (*entity.Category, error)
	Upsert(ctx context.Context, c *entity.Category) error
	BatchUpsert(ctx context.Context, list []entity.Category) error
	Delete(ctx context.Context, id int64) error
}

// CollectSourceRepository 采集源。
type CollectSourceRepository interface {
	List(ctx context.Context, onlyEnabled bool) ([]entity.CollectSource, error)
	Get(ctx context.Context, id string) (*entity.CollectSource, error)
	ExistsByUri(ctx context.Context, uri string, excludeId string) (bool, error)
	Upsert(ctx context.Context, s *entity.CollectSource) error
	Delete(ctx context.Context, id string) error
}

// SourceHealthRepository 采集源健康度 (一源一行快照)。
type SourceHealthRepository interface {
	Get(ctx context.Context, sourceId string) (*entity.SourceHealth, error) // 未命中返回 domain.ErrNotFound
	List(ctx context.Context) ([]entity.SourceHealth, error)
	Upsert(ctx context.Context, h *entity.SourceHealth) error
}

// CronTaskRepository 定时任务。
type CronTaskRepository interface {
	List(ctx context.Context) ([]entity.CronTask, error)
	Get(ctx context.Context, id int64) (*entity.CronTask, error)
	Upsert(ctx context.Context, t *entity.CronTask) error
	Delete(ctx context.Context, id int64) error
}

// SiteConfigRepository 站点配置 (单行)。
type SiteConfigRepository interface {
	Get(ctx context.Context) (*entity.SiteConfig, error)
	Save(ctx context.Context, c *entity.SiteConfig) error
}

// AppVersionRepository APK 版本。
type AppVersionRepository interface {
	Latest(ctx context.Context, channel int8) (*entity.AppVersion, error)
	List(ctx context.Context, page Page) ([]entity.AppVersion, int64, error)
	Create(ctx context.Context, v *entity.AppVersion) error
	Delete(ctx context.Context, id int64) error
}

// FileRepository 图库元数据 (二进制走 BlobStore)。
type FileRepository interface {
	// GetCoversByRelevance 批量取影片封面 (type=0), 命中 idx_relevance, 根治全表扫。
	GetCoversByRelevance(ctx context.Context, relevanceIds []int64) (map[int64]string, error)
	Create(ctx context.Context, f *entity.FileInfo) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, page Page) ([]entity.FileInfo, int64, error)
}

// UserRepository 用户。
type UserRepository interface {
	GetByName(ctx context.Context, name string) (*entity.User, error) // 未命中返回 domain.ErrUserNotFound
	GetById(ctx context.Context, id uint) (*entity.User, error)
	Create(ctx context.Context, u *entity.User) error
	List(ctx context.Context, page Page) ([]entity.User, int64, error)
	UpdatePassword(ctx context.Context, id uint, hashed string) error
}

// HistoryRepository 观看历史。
type HistoryRepository interface {
	Upsert(ctx context.Context, h *entity.UserHistory) error
	List(ctx context.Context, userId int64, page Page) ([]entity.UserHistory, int64, error)
	Delete(ctx context.Context, userId, mid int64) error
	Clear(ctx context.Context, userId int64) error
}

// FavoriteRepository 收藏。
type FavoriteRepository interface {
	Add(ctx context.Context, f *entity.UserFavorite) error // 幂等
	Remove(ctx context.Context, userId, mid int64) error
	List(ctx context.Context, userId int64, page Page) ([]entity.UserFavorite, int64, error)
	Exists(ctx context.Context, userId, mid int64) (bool, error)
	// TopFavorited 收藏最多的影片 (按 mid 计数 Top N)。
	TopFavorited(ctx context.Context, limit int) ([]MidCount, error)
}

// SkipSettingRepository 每用户每片片头/片尾跳过设置。
type SkipSettingRepository interface {
	List(ctx context.Context, userId int64) ([]entity.UserSkipSetting, error)
	Upsert(ctx context.Context, s *entity.UserSkipSetting) error // (user_id,mid) 冲突则更新
	Delete(ctx context.Context, userId, mid int64) error
}

// TelemetryRepository 埋点入库 + 分析查询 + 问题闭环。
type TelemetryRepository interface {
	BatchInsert(ctx context.Context, list []entity.TelemetryEvent) error
	// DeleteNoise 清理历史脏数据: ua 为空/含任一 bot 子串, 或 path 属 junkPaths 的行。返回删除条数。
	DeleteNoise(ctx context.Context, botUASubstrings, junkPaths []string) (int64, error)
	// DeleteApiErrors 清理误记的 API HTTP 错误: extra.type='error' 且 category 以 'api-' 开头(如 api-4xx/api-5xx)。
	// 前端已停止把 HTTP 4xx/5xx 记成 error 事件, 此处一次性清历史。返回删除条数。
	DeleteApiErrors(ctx context.Context) (int64, error)
	// DeleteErrorsBefore 清理 beforeUnix 之前的错误事件(按龄保留): extra.type='error' 且 server_ts < 截止。返回删除条数。
	DeleteErrorsBefore(ctx context.Context, beforeUnix int64) (int64, error)
	// DeleteChunkNoise 清理 chunk 加载失败类噪声错误(部署自愈现象, 非真异常)。返回删除条数。
	DeleteChunkNoise(ctx context.Context) (int64, error)
	GetResolution(ctx context.Context, keyHash string) (*entity.TelemetryIssueResolution, error)
	UpsertResolution(ctx context.Context, r *entity.TelemetryIssueResolution) error

	// 分析查询 (sinceUnix=server_ts 秒下界, 0 不限)
	CountSince(ctx context.Context, sinceUnix int64) (int64, error)
	CountByCategory(ctx context.Context, sinceUnix int64) ([]CategoryCount, error)
	TopPaths(ctx context.Context, sinceUnix int64, limit int) ([]PathCount, error)
	// TopFilmViews pv 事件按 label(含 /filmDetail?link= 或 /play?id=) 分组计数, service 再解析 mid 聚合。
	TopFilmViews(ctx context.Context, sinceUnix int64, limit int) ([]PathCount, error)
	ApiPerf(ctx context.Context, sinceUnix int64, limit int) ([]ApiPerf, error)
	ListEvents(ctx context.Context, f TelemetryFilter, page Page) ([]entity.TelemetryEvent, int64, error)

	// 以下为前端看板契约(category=pv/api 计数, extra.type/extra.action 派生)
	OverviewStats(ctx context.Context, sinceUnix int64) (OverviewStats, error)
	ApiPerfByAction(ctx context.Context, sinceUnix int64, limit int) ([]ActionPerfAgg, error)
	ApiActionValues(ctx context.Context, sinceUnix int64, action string, limit int) ([]float64, error)
	ListEventsByType(ctx context.Context, sinceUnix int64, frontendType string, limit, offset int) ([]entity.TelemetryEvent, int64, error)
	GetResolutions(ctx context.Context, keyHashes []string) (map[string]*entity.TelemetryIssueResolution, error)
}
