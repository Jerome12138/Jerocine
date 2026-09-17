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

func (r *taskRunRepo) CountRunning(ctx context.Context) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).Model(&entity.TaskRun{}).
		Where("status = ?", entity.TaskRunRunning).Count(&n).Error
	return n, err
}

// MarkInterrupted 服务重启时调用: 未收尾的 running 行标记为 failed(中断)。
func (r *taskRunRepo) MarkInterrupted(ctx context.Context) error {
	return dbFrom(ctx, r.db).Model(&entity.TaskRun{}).
		Where("status = ?", entity.TaskRunRunning).
		Updates(map[string]any{
			"status": entity.TaskRunFailed, "error": "服务重启, 任务中断",
			"ended_at": nowMilli(), "updated_at": nowMilli(),
		}).Error
}
