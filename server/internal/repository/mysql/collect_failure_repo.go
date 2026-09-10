package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type collectFailureRepo struct{ db *gorm.DB }

// NewCollectFailureRepository 构造采集失败台账仓储。
func NewCollectFailureRepository(db *gorm.DB) repository.CollectFailureRepository {
	return &collectFailureRepo{db: db}
}

// Record 落一条失败记录。同一(源, 页, 参数)若还在待处理状态, 只累加 attempts 并刷新原因,
// 不再插新行 —— 一个页连续失败几十次不该把台账刷成几十行。
func (r *collectFailureRepo) Record(ctx context.Context, f *entity.CollectFailure) error {
	db := dbFrom(ctx, r.db)
	var exist entity.CollectFailure
	err := db.Where("source_id = ? AND page_no = ? AND hours = ? AND status = ?",
		f.SourceId, f.PageNo, f.Hours, entity.FailurePending).First(&exist).Error
	if err == nil {
		return db.Model(&entity.CollectFailure{}).Where("id = ?", exist.Id).
			Updates(map[string]any{
				"cause":      f.Cause,
				"attempts":   exist.Attempts + 1,
				"updated_at": f.CreatedAt,
			}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Create(f).Error
}

// ListPending 取待补采记录, 按失败时间升序(先失败先补)。ids 非空时只取这些 id。
func (r *collectFailureRepo) ListPending(ctx context.Context, ids []int64, limit int) ([]entity.CollectFailure, error) {
	if limit <= 0 {
		limit = 200
	}
	q := dbFrom(ctx, r.db).Where("status = ?", entity.FailurePending)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var out []entity.CollectFailure
	err := q.Order("created_at ASC, id ASC").Limit(limit).Find(&out).Error
	return out, err
}

func (r *collectFailureRepo) List(ctx context.Context, status int8, page repository.Page) ([]entity.CollectFailure, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.CollectFailure{})
	if status != entity.FailureStatusAny {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.CollectFailure
	err := q.Order("created_at DESC, id DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}

func (r *collectFailureRepo) MarkHandled(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).Model(&entity.CollectFailure{}).
		Where("id IN ?", ids).
		Updates(map[string]any{"status": entity.FailureHandled, "updated_at": nowMilli()}).Error
}

func (r *collectFailureRepo) MarkHandledIncrementalBefore(ctx context.Context, sourceId string, maxId int64, maxHours int) (int64, error) {
	res := dbFrom(ctx, r.db).Model(&entity.CollectFailure{}).
		Where("source_id = ? AND id <= ? AND status = ? AND hours > 0 AND hours <= ?",
			sourceId, maxId, entity.FailurePending, maxHours).
		Updates(map[string]any{"status": entity.FailureHandled, "updated_at": nowMilli()})
	return res.RowsAffected, res.Error
}

func (r *collectFailureRepo) DeleteHandled(ctx context.Context) (int64, error) {
	res := dbFrom(ctx, r.db).Where("status = ?", entity.FailureHandled).Delete(&entity.CollectFailure{})
	return res.RowsAffected, res.Error
}

func (r *collectFailureRepo) CountPending(ctx context.Context) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).Model(&entity.CollectFailure{}).
		Where("status = ?", entity.FailurePending).Count(&n).Error
	return n, err
}
