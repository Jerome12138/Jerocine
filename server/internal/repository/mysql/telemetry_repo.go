package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// 埋点分析查询。server_ts 为 datetime(3); sinceUnix>0 时加时间下界。

func (r *telemetryRepo) sinceQuery(ctx context.Context, sinceUnix int64) *gorm.DB {
	q := dbFrom(ctx, r.db).Model(&entity.TelemetryEvent{})
	if sinceUnix > 0 {
		q = q.Where("server_ts >= ?", time.Unix(sinceUnix, 0))
	}
	return q
}

func (r *telemetryRepo) CountSince(ctx context.Context, sinceUnix int64) (int64, error) {
	var n int64
	err := r.sinceQuery(ctx, sinceUnix).Count(&n).Error
	return n, err
}

func (r *telemetryRepo) CountByCategory(ctx context.Context, sinceUnix int64) ([]repository.CategoryCount, error) {
	var out []repository.CategoryCount
	err := r.sinceQuery(ctx, sinceUnix).
		Select("category, COUNT(*) AS count").Group("category").Order("count DESC").Scan(&out).Error
	return out, err
}

func (r *telemetryRepo) TopPaths(ctx context.Context, sinceUnix int64, limit int) ([]repository.PathCount, error) {
	if limit <= 0 {
		limit = 20
	}
	var out []repository.PathCount
	err := r.sinceQuery(ctx, sinceUnix).
		Where("path <> ''").Select("path, COUNT(*) AS count").
		Group("path").Order("count DESC").Limit(limit).Scan(&out).Error
	return out, err
}

// TopFilmViews pv 事件按 label 分组计数(label 含 /filmDetail?link= 或 /play?id=)。
// 不在 SQL 解析 mid(label 形态各异), 交 service 用 url.Parse 聚合。
func (r *telemetryRepo) TopFilmViews(ctx context.Context, sinceUnix int64, limit int) ([]repository.PathCount, error) {
	if limit <= 0 {
		limit = 500
	}
	var out []repository.PathCount
	err := r.sinceQuery(ctx, sinceUnix).
		Where("category = ? AND (label LIKE ? OR label LIKE ?)", "pv", "/filmDetail%", "/play%").
		Select("label AS path, COUNT(*) AS count").
		Group("label").Order("count DESC").Limit(limit).Scan(&out).Error
	return out, err
}

func (r *telemetryRepo) ApiPerf(ctx context.Context, sinceUnix int64, limit int) ([]repository.ApiPerf, error) {
	if limit <= 0 {
		limit = 20
	}
	var out []repository.ApiPerf
	// 约定: 接口性能事件 category='api', duration 为耗时(ms)。
	err := r.sinceQuery(ctx, sinceUnix).
		Where("category = ? AND path <> ''", "api").
		Select("path, COUNT(*) AS count, AVG(duration) AS avg_ms, MAX(duration) AS max_ms").
		Group("path").Order("count DESC").Limit(limit).Scan(&out).Error
	return out, err
}

// OverviewStats 看板顶部卡片原始聚合。category=pv/api 直接计数; error 用 extra.type;
// avgApiMs 取 api 事件 value 均值(value=耗时 ms), 剔除 >60s 异常样本 —
// 正常接口调用远小于该值, 挂起/断连/超时请求(可达数百秒)会把算术均值拉爆, 不参与统计。
func (r *telemetryRepo) OverviewStats(ctx context.Context, sinceUnix int64) (repository.OverviewStats, error) {
	var o repository.OverviewStats
	if err := r.sinceQuery(ctx, sinceUnix).Where("category = ?", "pv").Count(&o.PV).Error; err != nil {
		return o, err
	}
	if err := r.sinceQuery(ctx, sinceUnix).Where("category = ?", "api").Count(&o.ApiCount).Error; err != nil {
		return o, err
	}
	if err := r.sinceQuery(ctx, sinceUnix).Where("extra->>'$.type' = ?", "error").Count(&o.ErrorCount).Error; err != nil {
		return o, err
	}
	r.sinceQuery(ctx, sinceUnix).Where("session_id <> ''").Distinct("session_id").Count(&o.UV)
	r.sinceQuery(ctx, sinceUnix).Where("user_id IS NOT NULL AND user_id <> 0").Distinct("user_id").Count(&o.UniqueUsers)
	const maxApiMs = 60_000 // 异常样本阈值: >60s 视为挂起/超时请求, 不参与均值
	var avg struct{ V float64 }
	if err := r.sinceQuery(ctx, sinceUnix).
		Where("category = ?", "api").
		Select("COALESCE(AVG(CASE WHEN value >= 0 AND value <= ? THEN value END),0) AS v", maxApiMs).
		Scan(&avg).Error; err == nil {
		o.AvgApiMs = avg.V
	}
	return o, nil
}

// ApiPerfByAction 按 api 接口路径(extra.action) 聚合 count/avg, 取 Top。
func (r *telemetryRepo) ApiPerfByAction(ctx context.Context, sinceUnix int64, limit int) ([]repository.ActionPerfAgg, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []repository.ActionPerfAgg
	err := r.sinceQuery(ctx, sinceUnix).
		Where("category = ? AND extra->>'$.action' IS NOT NULL AND extra->>'$.action' <> ''", "api").
		Select("extra->>'$.action' AS action, COUNT(*) AS count, AVG(value) AS avg").
		Group("action").Order("count DESC").Limit(limit).Scan(&out).Error
	return out, err
}

// ApiActionValues 取某接口最近 limit 条耗时(value, ms), service 用于算百分位。
func (r *telemetryRepo) ApiActionValues(ctx context.Context, sinceUnix int64, action string, limit int) ([]float64, error) {
	if limit <= 0 {
		limit = 1000
	}
	var values []float64
	err := r.sinceQuery(ctx, sinceUnix).
		Where("category = ? AND extra->>'$.action' = ?", "api", action).
		Order("server_ts DESC").Limit(limit).Pluck("value", &values).Error
	return values, err
}

// ListErrorIssues 按 issue(category,path,label) 聚合错误事件, latest_ts 倒序。
// 两条 SQL: ①GROUP BY 出聚合(SUM(value) 累计次数 / COUNT(*) 批次数 / MAX(server_ts));
// ②行值 IN 子查询取每个 issue 最新一条原始事件(extra/userId/platform 明细取自它)。
// 同一 (category,path,label,server_ts) 恰好撞车时取一条, 看板展示无影响。
func (r *telemetryRepo) ListErrorIssues(ctx context.Context, sinceUnix int64, frontendType string) ([]repository.TelemetryIssueAgg, error) {
	typeQ := func(q *gorm.DB) *gorm.DB {
		if frontendType != "" {
			q = q.Where("extra->>'$.type' = ?", frontendType)
		}
		return q
	}
	var aggs []struct {
		Category   string
		Path       string
		Label      string
		TotalValue float64
		BatchCount int64
		LatestTs   time.Time
	}
	if err := typeQ(r.sinceQuery(ctx, sinceUnix)).
		Select("category, path, label, SUM(value) AS total_value, COUNT(*) AS batch_count, MAX(server_ts) AS latest_ts").
		Group("category, path, label").Order("latest_ts DESC").Scan(&aggs).Error; err != nil {
		return nil, err
	}
	out := make([]repository.TelemetryIssueAgg, 0, len(aggs))
	if len(aggs) == 0 {
		return out, nil
	}
	sub := typeQ(r.sinceQuery(ctx, sinceUnix)).
		Select("category, path, label, MAX(server_ts)").
		Group("category, path, label")
	var reps []entity.TelemetryEvent
	if err := typeQ(r.sinceQuery(ctx, sinceUnix)).
		Where("(category, path, label, server_ts) IN (?)", sub).
		Order("server_ts DESC").Find(&reps).Error; err != nil {
		return nil, err
	}
	type issueKey struct{ c, p, l string }
	latest := make(map[issueKey]entity.TelemetryEvent, len(reps))
	for _, e := range reps {
		k := issueKey{e.Category, e.Path, e.Label}
		if _, dup := latest[k]; !dup {
			latest[k] = e
		}
	}
	for _, a := range aggs {
		agg := repository.TelemetryIssueAgg{
			Category: a.Category, Path: a.Path, Label: a.Label,
			TotalValue: a.TotalValue, BatchCount: a.BatchCount,
		}
		if e, ok := latest[issueKey{a.Category, a.Path, a.Label}]; ok {
			agg.Latest = e
		}
		out = append(out, agg)
	}
	return out, nil
}

// DeleteChunkNoise 清理 chunk 加载失败类噪声错误(部署后旧 hash 自愈现象, 非真异常):
// extra.type='error' 且 category=chunk-load-reload 或 label 含动态 import/MIME/ChunkLoad 特征。返回删除条数。
func (r *telemetryRepo) DeleteChunkNoise(ctx context.Context) (int64, error) {
	res := dbFrom(ctx, r.db).
		Where("extra->>'$.type' = ?", "error").
		Where("(category = ? OR label LIKE ? OR label LIKE ? OR label LIKE ? OR label LIKE ? OR label LIKE ? OR label LIKE ?)",
			"chunk-load-reload",
			"%dynamically imported module%",
			"%Importing a module script failed%",
			"%valid JavaScript MIME type%",
			"%ChunkLoadError%",
			"%Loading chunk%",
			"%preload CSS%").
		Delete(&entity.TelemetryEvent{})
	return res.RowsAffected, res.Error
}

// DeleteErrorsBefore 清理 beforeUnix 之前的错误事件(extra.type='error' 且 server_ts < 截止时间)。返回删除条数。
func (r *telemetryRepo) DeleteErrorsBefore(ctx context.Context, beforeUnix int64) (int64, error) {
	res := dbFrom(ctx, r.db).
		Where("extra->>'$.type' = ? AND server_ts < ?", "error", time.Unix(beforeUnix, 0)).
		Delete(&entity.TelemetryEvent{})
	return res.RowsAffected, res.Error
}

// GetResolutions 批量取问题闭环记录, 按 key_hash 索引。
func (r *telemetryRepo) GetResolutions(ctx context.Context, keyHashes []string) (map[string]*entity.TelemetryIssueResolution, error) {
	m := make(map[string]*entity.TelemetryIssueResolution, len(keyHashes))
	if len(keyHashes) == 0 {
		return m, nil
	}
	var rows []entity.TelemetryIssueResolution
	if err := dbFrom(ctx, r.db).Where("key_hash IN ?", keyHashes).Find(&rows).Error; err != nil {
		return m, err
	}
	for i := range rows {
		m[rows[i].KeyHash] = &rows[i]
	}
	return m, nil
}

func (r *telemetryRepo) ListEvents(ctx context.Context, f repository.TelemetryFilter, page repository.Page) ([]entity.TelemetryEvent, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.TelemetryEvent{})
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.Path != "" {
		q = q.Where("path = ?", f.Path)
	}
	if f.SinceUnix > 0 {
		q = q.Where("server_ts >= ?", time.Unix(f.SinceUnix, 0))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.TelemetryEvent
	err := q.Order("server_ts DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}
