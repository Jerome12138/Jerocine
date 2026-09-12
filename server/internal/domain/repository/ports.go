package repository

import (
	"context"

	"server/internal/domain/entity"
)

// 仓储端口 (port): service 依赖这些接口, 由 repository/mysql 实现 (依赖倒置)。
// 所有方法只接收 context (事务经 ctx 传播, 见 tx.go), 不泄漏 *gorm.DB。

// MovieRepository 影片详情主表。
type MovieRepository interface {
	GetByMid(ctx context.Context, mid int64) (*entity.Movie, error) // 未命中返回 domain.ErrMovieNotFound; 已软删视为未命中
	// GetByMidIncludingDeleted 同 GetByMid, 但不排除已软删的影片。后台查看/恢复需要能读到它们。
	GetByMidIncludingDeleted(ctx context.Context, mid int64) (*entity.Movie, error)
	Upsert(ctx context.Context, m *entity.Movie) error
	BatchUpsert(ctx context.Context, list []entity.Movie) error
	Delete(ctx context.Context, mid int64) error
	// SoftDelete 打软删标记(deletedAt 毫秒时间戳, 供后台展示删除时间)。
	SoftDelete(ctx context.Context, mid, deletedAt int64) error
	// Restore 清除软删标记。
	Restore(ctx context.Context, mid int64) error
	// ListMissingBackdropsByMids 取指定 mid 中尚未回填横图的影片(TMDB worker 用, 范围仅首页轮播集合)。
	// 排除未软删外的行(软删/空片名)与 backdrop 非空(含 '-' 哨兵)的行。
	ListMissingBackdropsByMids(ctx context.Context, mids []int64) ([]entity.Movie, error)
	// UpdateBackdrop 回填横图(url 可为 tmdb.MissMark 哨兵)。返回是否有行被更新。
	UpdateBackdrop(ctx context.Context, mid int64, url string) (bool, error)
	Truncate(ctx context.Context) error

	// ---- 榜单热度(豆瓣榜单刷新任务用, 见 service/hot_service.go) ----
	// 注意: db_id 只存在于 movie 表 —— 按 db_id 匹配必须查主表, 读模型 movie_search 无该列。

	// HotCandidatesByDbIds 按豆瓣 subject id 批量取本地候选行(首选匹配: 精确)。
	// 同一 db_id 可能对应多行(库里 16,755 组重复, 多为"正片 + 电影解说"配对), 由调用方择优。
	HotCandidatesByDbIds(ctx context.Context, dbIds []int64) ([]HotCandidate, error)
	// HotCandidatesByKeyword 用 FULLTEXT 取与该片名相关的本地候选行(兜底匹配)。
	// 只是**候选**: 调用方还要做归一化片名 + 年份 + 编导的三重校验, 唯一命中才采用。
	HotCandidatesByKeyword(ctx context.Context, keyword string, limit int) ([]HotCandidate, error)
	// ListHotBoard 当前在榜行(hot_rank > 0), 刷新任务据此把掉榜片回落到兜底分。
	ListHotBoard(ctx context.Context) ([]HotCandidate, error)
	// ApplyHot 写入榜单热度(db_id/db_score 仅在入参非 0 时回填), 事务内双写 movie 与 movie_search。
	// 返回 movie 表实际更新的行数。
	ApplyHot(ctx context.Context, rows []HotRow) (int, error)
}

// SearchRepository 物化卡片/检索宽表 (读模型) + 影子表重建。
// 接口隔离便于未来把检索换成 meilisearch 等外部引擎。
type SearchRepository interface {
	GetByMid(ctx context.Context, mid int64) (*entity.MovieSearch, error)
	GetByMids(ctx context.Context, mids []int64) ([]entity.MovieSearch, error)
	// TopByPidSorted 取某一级分类下某排序维度的前 N 条 (首页区块 / 分类三榜用, 不分页不 count → 根治首页无谓 COUNT)。
	TopByPidSorted(ctx context.Context, pid int64, sort ClassifySort, limit int) ([]entity.MovieSearch, error)
	// TopHotAll 全站跨类别热榜(首页「热门榜单」行, 不按 pid) —— 与分类页排行榜共用同一排序键。
	TopHotAll(ctx context.Context, limit int) ([]entity.MovieSearch, error)
	// TopScoreByPid 某一级分类的高分榜(口径见 docs/榜单热度方案 §3.5: db_score > 0 且排除"解说")。
	TopScoreByPid(ctx context.Context, pid int64, limit int) ([]entity.MovieSearch, error)
	// CountScoredByPid 该一级分类有评分的影片数 —— 为 0 时分类页不返回高分榜分区(前端隐藏入口)。
	CountScoredByPid(ctx context.Context, pid int64) (int64, error)
	// Filter 多维筛选分页 (含 pid+sort 的可翻页浏览; 返回数据与总数)。
	Filter(ctx context.Context, spec FilterSpec, page Page) ([]entity.MovieSearch, int64, error)
	// SearchKeyword 关键字检索 (FULLTEXT ngram)。deleted 取 DeletedExclude/DeletedOnly/DeletedInclude。
	SearchKeyword(ctx context.Context, keyword string, deleted int, page Page) ([]entity.MovieSearch, int64, error)
	// CountCreatedSince 统计 created_at(毫秒) >= sinceMillis 的影片数 (仪表盘今日/近一周新增)。
	CountCreatedSince(ctx context.Context, sinceMillis int64) (int64, error)
	// Related 相关推荐候选 (按 cid + 名称/标签, 内存抽样在 service 层)。
	Related(ctx context.Context, seed RelatedSeed, candidateLimit int) ([]entity.MovieSearch, error)
	// TagOptions 7 维筛选标签 (查询期 GROUP BY 聚合, 不再靠采集期 ZIncrBy)。
	TagOptions(ctx context.Context, pid int64) (*FilterOptions, error)

	Upsert(ctx context.Context, m *entity.MovieSearch) error
	BatchUpsert(ctx context.Context, list []entity.MovieSearch) error
	Delete(ctx context.Context, mid int64) error
	// SoftDelete 给读模型打软删标记, 与 movie 同步。
	SoftDelete(ctx context.Context, mid, deletedAt int64) error
	// Restore 清除读模型的软删标记。
	Restore(ctx context.Context, mid int64) error
	// SyncDeletedFromMovie 把 movie.deleted_at 回灌到 movie_search。
	// 由 ShadowCommit 在**换表前**对影子表调用(换表会重建读模型、丢失删除态), 也可单独触发做修复。幂等。
	SyncDeletedFromMovie(ctx context.Context) error
	// UpdateBackdrop 回填横图读模型列(与 movie.backdrop 由 TMDB worker 双写同步)。
	UpdateBackdrop(ctx context.Context, mid int64, url string) error

	// 全量重采无空窗影子表生命周期: Begin(建 movie_search_next) → Write(批量灌) → Commit(回灌删除态 + RENAME 原子切换 + drop old)。
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

// CollectFailureRepository 采集页级失败台账。
// 记录页级失败 → 让"补采"成为可能(此前失败页只会丢, 事后连丢的是哪页都查不到)。
type CollectFailureRepository interface {
	// Record 落一条失败记录; 同源+同页+同参数(小时)若仍在待处理, 则累加 attempts 并刷新原因/时间, 不另起一行。
	Record(ctx context.Context, f *entity.CollectFailure) error
	// ListPending 取待补采记录, 按失败时间升序; ids 非空时只取这些 id。
	ListPending(ctx context.Context, ids []int64, limit int) ([]entity.CollectFailure, error)
	// List 后台列表(status 为 entity.FailureStatusAny 时不过滤), 按时间倒序分页。
	List(ctx context.Context, status int8, page Page) ([]entity.CollectFailure, int64, error)
	// MarkHandled 按 id 置为已处理。
	MarkHandled(ctx context.Context, ids []int64) error
	// MarkHandledIncrementalBefore 把同源、同为增量(hours>0 且 <=maxHours)、id 不晚于 maxId 的待处理记录
	// 一并置为已处理 —— 一次扩窗重扫已覆盖这些页, 不必再逐条重放。
	MarkHandledIncrementalBefore(ctx context.Context, sourceId string, maxId int64, maxHours int) (int64, error)
	// DeleteHandled 清空已处理记录, 返回删除条数。
	DeleteHandled(ctx context.Context) (int64, error)
	// CountPending 待补采条数。
	CountPending(ctx context.Context) (int64, error)
}

// SiteConfigRepository 站点配置 (单行)。
type SiteConfigRepository interface {
	Get(ctx context.Context) (*entity.SiteConfig, error)
	Save(ctx context.Context, c *entity.SiteConfig) error
}

// BannerRepository 首页轮播配置。
type BannerRepository interface {
	// ListEnabled 当前生效的 Banner(启用且在生效窗口内), 按 sort/id 升序。
	// now 由调用方传入, 便于测试固定时间。
	ListEnabled(ctx context.Context, now int64) ([]entity.Banner, error)
	// ListAll 后台全量列表, 按 sort/id 升序。
	ListAll(ctx context.Context) ([]entity.Banner, error)
	// Get 按 id 取单条; 不存在返回 domain.ErrNotFound。
	Get(ctx context.Context, id int64) (*entity.Banner, error)
	Create(ctx context.Context, b *entity.Banner) error
	// Update 按 id 整体覆盖可编辑字段。
	Update(ctx context.Context, b *entity.Banner) error
	// UpdateSort 只更新排序值(排序重排专用, 不碰其余字段)。
	UpdateSort(ctx context.Context, id int64, sort int) error
	Delete(ctx context.Context, id int64) error
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
