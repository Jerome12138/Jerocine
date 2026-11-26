package mysql

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// taskSettingRepo 内置任务配置表（统一任务中心）。
//
// 语义：表里只保留**用户改过的值**；没有行 = 全部用注册表默认值。
// 因此 List 只返回存在的行，调用方自行与默认值合并。
type taskSettingRepo struct{ db *gorm.DB }

func NewTaskSettingRepo(db *gorm.DB) repository.TaskSettingRepository {
	return &taskSettingRepo{db: db}
}

func (r *taskSettingRepo) List(ctx context.Context) (map[string]entity.TaskSetting, error) {
	var rows []entity.TaskSetting
	if err := dbFrom(ctx, r.db).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]entity.TaskSetting, len(rows))
	for _, s := range rows {
		out[s.TaskKey] = s
	}
	return out, nil
}

func (r *taskSettingRepo) Get(ctx context.Context, key string) (entity.TaskSetting, bool, error) {
	var s entity.TaskSetting
	err := dbFrom(ctx, r.db).Where("task_key = ?", key).First(&s).Error
	if err == gorm.ErrRecordNotFound {
		return entity.TaskSetting{}, false, nil
	}
	if err != nil {
		return entity.TaskSetting{}, false, err
	}
	return s, true, nil
}

// Upsert 只覆盖传入的非零字段（空串/0 视为"不改"），避免把备注清空之类的手误。
func (r *taskSettingRepo) Upsert(ctx context.Context, s entity.TaskSetting) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"enabled":     s.Enabled,
			"schedule":    gorm.Expr("IF(VALUES(schedule) <> '', VALUES(schedule), schedule)"),
			"remark":      gorm.Expr("IF(VALUES(remark) <> '', VALUES(remark), remark)"),
			"interval_ms": gorm.Expr("IF(VALUES(interval_ms) > 0, VALUES(interval_ms), interval_ms)"),
			"updated_at":  s.UpdatedAt,
		}),
	}).Create(&s).Error
}

// SetEnabled 单独改启用位（与 Upsert 的"非空才覆盖"语义区分开：这里 false 也要生效）。
func (r *taskSettingRepo) SetEnabled(ctx context.Context, key string, enabled bool, now int64) error {
	return dbFrom(ctx, r.db).Exec(
		"INSERT INTO task_setting(task_key, enabled, schedule, remark, interval_ms, updated_at) "+
			"VALUES(?, ?, '', '', 0, ?) "+
			"ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), updated_at = VALUES(updated_at)",
		key, enabled, now).Error
}
