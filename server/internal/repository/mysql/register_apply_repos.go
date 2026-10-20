package mysql

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ---- register_apply (申请注册审核, P2) ----

type registerApplyRepo struct{ db *gorm.DB }

func NewRegisterApplyRepository(db *gorm.DB) repository.RegisterApplyRepository {
	return &registerApplyRepo{db: db}
}

func (r *registerApplyRepo) Create(ctx context.Context, a *entity.RegisterApply) error {
	return dbFrom(ctx, r.db).Create(a).Error
}

func (r *registerApplyRepo) GetById(ctx context.Context, id int64) (*entity.RegisterApply, error) {
	var a entity.RegisterApply
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *registerApplyRepo) GetByEmail(ctx context.Context, email string) (*entity.RegisterApply, error) {
	var a entity.RegisterApply
	err := dbFrom(ctx, r.db).Where("email = ?", email).Order("id DESC").First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// LatestByAccount 登录申请态反馈: 账号可能是申请用户名, 也可能是申请邮箱。
func (r *registerApplyRepo) LatestByAccount(ctx context.Context, account string) (*entity.RegisterApply, error) {
	var a entity.RegisterApply
	err := dbFrom(ctx, r.db).
		Where("user_name = ? OR email = ?", account, account).
		Order("id DESC").First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *registerApplyRepo) List(ctx context.Context, status int8, page repository.Page) ([]entity.RegisterApply, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.RegisterApply{})
	if status != entity.ApplyStatusAny {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.RegisterApply
	err := q.Order("id DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}

// Approve pending → approved。WHERE status = pending 保证幂等: 重复审批影响 0 行 → ErrConflict。
func (r *registerApplyRepo) Approve(ctx context.Context, id int64, reviewedBy int64) error {
	res := dbFrom(ctx, r.db).Model(&entity.RegisterApply{}).
		Where("id = ? AND status = ?", id, entity.ApplyStatusPending).
		Updates(map[string]any{
			"status":        entity.ApplyStatusApproved,
			"reject_reason": "",
			"reviewed_by":   reviewedBy,
			"reviewed_at":   time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

// Reject pending → rejected + 理由。同样以 status = pending 做幂等闸。
func (r *registerApplyRepo) Reject(ctx context.Context, id int64, reviewedBy int64, reason string) error {
	res := dbFrom(ctx, r.db).Model(&entity.RegisterApply{}).
		Where("id = ? AND status = ?", id, entity.ApplyStatusPending).
		Updates(map[string]any{
			"status":        entity.ApplyStatusRejected,
			"reject_reason": reason,
			"reviewed_by":   reviewedBy,
			"reviewed_at":   time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *registerApplyRepo) MarkActivated(ctx context.Context, id int64, at time.Time) error {
	return dbFrom(ctx, r.db).Model(&entity.RegisterApply{}).Where("id = ?", id).
		Update("activated_at", at).Error
}
