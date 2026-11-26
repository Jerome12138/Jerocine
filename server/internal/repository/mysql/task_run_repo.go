package mysql

import (
	"context"

	"gorm.io/gorm"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// taskRunRepo 任务运行台账 (table: task_run)。
type taskRunRepo struct{ db *gorm.DB }

// NewTaskRunRepository 构造任务台账仓储。
func NewTaskRunRepository(db *gorm.DB) repository.TaskRunRepository {
	return &taskRunRepo{db: db}
}

func (r *taskRunRepo) Create(ctx context.Context, t *entity.TaskRun) error {
	if t.CreatedAt == 0 {
		t.CreatedAt = nowMilli()
	}
	if t.UpdatedAt == 0 {
		t.UpdatedAt = t.CreatedAt
	}
	return dbFrom(ctx, r.db).Create(t).Error
}

// Update 更新可变动字段。调用方负责填 Id; 其余字段整体覆盖(台账行由服务层独占更新)。
func (r *taskRunRepo) Update(ctx context.Context, t *entity.TaskRun) error {
	return dbFrom(ctx, r.db).Model(&entity.TaskRun{}).Where("id = ?", t.Id).
		Updates(map[string]any{
			"status": t.Status, "total": t.Total, "done": t.Done, "failed": t.Failed,
			"message": t.Message, "error": t.Error, "ended_at": t.EndedAt,
			"updated_at": nowMilli(),
		}).Error
}

func (r *taskRunRepo) Get(ctx context.Context, id int64) (*entity.TaskRun, error) {
	var t entity.TaskRun
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *taskRunRepo) List(ctx context.Context, f repository.TaskRunFilter, page repository.Page) ([]entity.TaskRun, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.TaskRun{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		q = q.Where("task_type = ?", f.Type)
	}
	if f.Ignored != nil {
		if *f.Ignored {
			q = q.Where("ignored = 1")
		} else {
			q = q.Where("ignored = 0")
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.TaskRun
	err := q.Order("id DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}

func (r *taskRunRepo) LatestByCron(ctx context.Context, cronIds []int64) (map[int64]entity.TaskRun, error) {
	out := make(map[int64]entity.TaskRun, len(cronIds))
	if len(cronIds) == 0 {
		return out, nil
	}
	var rows []entity.TaskRun
	// 每个 cron_id 只取最新一条: 子查询按 (cron_id, id) 分组取最大 id。
	err := dbFrom(ctx, r.db).Raw(
		`SELECT t.* FROM task_run t
		 JOIN (SELECT cron_id, MAX(id) AS mid FROM task_run WHERE kind = ? AND cron_id IN (?) GROUP BY cron_id) x
		   ON t.cron_id = x.cron_id AND t.id = x.mid`,
		entity.TaskRunKindCron, cronIds,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CronId] = r
	}
	return out, nil
}

func (r *taskRunRepo) CountSince(ctx context.Context, sinceMs int64) (success, failed int64, err error) {
	q := dbFrom(ctx, r.db).Model(&entity.TaskRun{}).Where("started_at >= ?", sinceMs)
	q = q.Select("SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS success, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS failed",
		entity.TaskRunSuccess, entity.TaskRunFailed)
	var row struct {
		Success int64
		Failed  int64
	}
	if err = q.Scan(&row).Error; err != nil {
		return 0, 0, err
	}
	return row.Success, row.Failed, nil
}

// CountRunning 统计 running 行数; excludeSourceIds 非空时剔除同名源的行
// (那些源已有 Redis 活跃 job, 统计卡另一口径已计入, 不剔会双计)。
// 非采集行(hot_refresh/recover 等)source_id 为空串, 不在任何排除名单里, 照常计入。
func (r *taskRunRepo) CountRunning(ctx context.Context, excludeSourceIds []string) (int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.TaskRun{}).Where("status = ?", entity.TaskRunRunning)
	if len(excludeSourceIds) > 0 {
		q = q.Where("source_id NOT IN ?", excludeSourceIds)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

// MarkInterrupted 服务重启时调用: 未收尾的 running 行标记为 failed(中断)。
// CountRunningByType 统计某任务类型（cronId>0 时再按 cron 限定）当前在跑的条数。
func (r *taskRunRepo) CountRunningByType(ctx context.Context, taskType string, cronId int64) (int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.TaskRun{}).Where("status = ?", entity.TaskRunRunning)
	if taskType != "" {
		q = q.Where("task_type = ?", taskType)
	}
	if cronId > 0 {
		q = q.Where("cron_id = ?", cronId)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

// SetIgnored 标记/取消"已忽略"（异常处理；不删行，保留审计）。
func (r *taskRunRepo) SetIgnored(ctx context.Context, id int64, ignored bool) error {
	return dbFrom(ctx, r.db).Model(&entity.TaskRun{}).Where("id = ?", id).
		Update("ignored", ignored).Error
}

// ListUnhandledFailed 未处理的失败：failed 且未忽略，且其后没有同一任务的**成功**运行。
//
// 同一任务的判定 = (task_type, cron_id, source_id) 三者相同（内置任务 cron_id/source_id 为 0/空）。
// 语义意图：只要后来又成功跑过一次，说明问题已自愈/已重跑过 → 不再打扰人。
// 索引：外层走 idx_status_ignored_started；子查询走 idx_task_run_cron(kind,cron_id) + PK 回表，
// 台账保留 30 天（万级行）实测秒级。
func (r *taskRunRepo) ListUnhandledFailed(ctx context.Context, limit int) ([]entity.TaskRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []entity.TaskRun
	err := dbFrom(ctx, r.db).Raw(
		`SELECT t.* FROM task_run t
		 WHERE t.status = ? AND t.ignored = 0
		   AND NOT EXISTS (
		     SELECT 1 FROM task_run s
		     WHERE s.status = ?
		       AND s.id > t.id
		       AND s.task_type = t.task_type
		       AND s.cron_id = t.cron_id
		       AND s.source_id = t.source_id
		   )
		 ORDER BY t.id DESC
		 LIMIT ?`,
		entity.TaskRunFailed, entity.TaskRunSuccess, limit,
	).Scan(&rows).Error
	return rows, err
}

// PruneBefore 删除早于 beforeMs 的**终态**记录（running 不删，避免误删活任务）。
func (r *taskRunRepo) PruneBefore(ctx context.Context, beforeMs int64, limit int) (int64, error) {
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	res := dbFrom(ctx, r.db).Exec(
		"DELETE FROM task_run WHERE started_at < ? AND status IN (?, ?, ?) LIMIT ?",
		beforeMs, entity.TaskRunSuccess, entity.TaskRunFailed, entity.TaskRunCanceled, limit)
	return res.RowsAffected, res.Error
}

func (r *taskRunRepo) MarkInterrupted(ctx context.Context) error {
	return dbFrom(ctx, r.db).Model(&entity.TaskRun{}).
		Where("status = ?", entity.TaskRunRunning).
		Updates(map[string]any{
			"status": entity.TaskRunFailed, "error": "服务重启, 任务中断",
			"ended_at": nowMilli(), "updated_at": nowMilli(),
		}).Error
}
