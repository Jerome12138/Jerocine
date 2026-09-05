package mysql

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ---- users ----

type userRepo struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) repository.UserRepository { return &userRepo{db: db} }

func (r *userRepo) GetByName(ctx context.Context, name string) (*entity.User, error) {
	var u entity.User
	err := dbFrom(ctx, r.db).Where("user_name = ?", name).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepo) GetById(ctx context.Context, id uint) (*entity.User, error) {
	var u entity.User
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepo) Create(ctx context.Context, u *entity.User) error {
	return dbFrom(ctx, r.db).Create(u).Error
}

func (r *userRepo) List(ctx context.Context, page repository.Page) ([]entity.User, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.User{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.User
	err := q.Order("id ASC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}

func (r *userRepo) UpdatePassword(ctx context.Context, id uint, hashed string) error {
	return dbFrom(ctx, r.db).Model(&entity.User{}).Where("id = ?", id).
		Update("password", hashed).Error
}

// ---- user_history ----

type historyRepo struct{ db *gorm.DB }

func NewHistoryRepository(db *gorm.DB) repository.HistoryRepository { return &historyRepo{db: db} }

func (r *historyRepo) Upsert(ctx context.Context, h *entity.UserHistory) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "mid"}},
		DoUpdates: clause.AssignmentColumns([]string{"play_from", "episode", "progress", "duration", "updated_at"}),
	}).Create(h).Error
}

func (r *historyRepo) List(ctx context.Context, userId int64, page repository.Page) ([]entity.UserHistory, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.UserHistory{}).Where("user_id = ?", userId)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.UserHistory
	err := q.Order("updated_at DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}

func (r *historyRepo) Delete(ctx context.Context, userId, mid int64) error {
	return dbFrom(ctx, r.db).Where("user_id = ? AND mid = ?", userId, mid).Delete(&entity.UserHistory{}).Error
}

func (r *historyRepo) Clear(ctx context.Context, userId int64) error {
	return dbFrom(ctx, r.db).Where("user_id = ?", userId).Delete(&entity.UserHistory{}).Error
}

// ---- user_favorite ----

type favoriteRepo struct{ db *gorm.DB }

func NewFavoriteRepository(db *gorm.DB) repository.FavoriteRepository { return &favoriteRepo{db: db} }

func (r *favoriteRepo) Add(ctx context.Context, f *entity.UserFavorite) error {
	// 幂等: 已存在则什么都不做
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "mid"}}, DoNothing: true,
	}).Create(f).Error
}

func (r *favoriteRepo) Remove(ctx context.Context, userId, mid int64) error {
	return dbFrom(ctx, r.db).Where("user_id = ? AND mid = ?", userId, mid).Delete(&entity.UserFavorite{}).Error
}

func (r *favoriteRepo) List(ctx context.Context, userId int64, page repository.Page) ([]entity.UserFavorite, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.UserFavorite{}).Where("user_id = ?", userId)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.UserFavorite
	err := q.Order("created_at DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}

func (r *favoriteRepo) Exists(ctx context.Context, userId, mid int64) (bool, error) {
	var n int64
	err := dbFrom(ctx, r.db).Model(&entity.UserFavorite{}).
		Where("user_id = ? AND mid = ?", userId, mid).Count(&n).Error
	return n > 0, err
}

// TopFavorited 收藏最多的影片 (按 mid 计数 Top N)。
func (r *favoriteRepo) TopFavorited(ctx context.Context, limit int) ([]repository.MidCount, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []repository.MidCount
	err := dbFrom(ctx, r.db).Model(&entity.UserFavorite{}).
		Select("mid, COUNT(*) AS count").
		Group("mid").Order("count DESC").Limit(limit).Scan(&out).Error
	return out, err
}

// ---- user_skip_setting ----

type skipRepo struct{ db *gorm.DB }

func NewSkipSettingRepository(db *gorm.DB) repository.SkipSettingRepository { return &skipRepo{db: db} }

func (r *skipRepo) List(ctx context.Context, userId int64) ([]entity.UserSkipSetting, error) {
	var out []entity.UserSkipSetting
	err := dbFrom(ctx, r.db).Where("user_id = ?", userId).Find(&out).Error
	return out, err
}

func (r *skipRepo) Upsert(ctx context.Context, s *entity.UserSkipSetting) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "mid"}},
		DoUpdates: clause.AssignmentColumns([]string{"intro_sec", "outro_sec", "updated_at"}),
	}).Create(s).Error
}

func (r *skipRepo) Delete(ctx context.Context, userId, mid int64) error {
	return dbFrom(ctx, r.db).Where("user_id = ? AND mid = ?", userId, mid).Delete(&entity.UserSkipSetting{}).Error
}

// ---- telemetry ----

type telemetryRepo struct{ db *gorm.DB }

func NewTelemetryRepository(db *gorm.DB) repository.TelemetryRepository { return &telemetryRepo{db: db} }

func (r *telemetryRepo) BatchInsert(ctx context.Context, list []entity.TelemetryEvent) error {
	if len(list) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).CreateInBatches(list, 200).Error
}

// DeleteNoise 清理历史脏数据: ua 为空 / 含任一 bot 子串(不分大小写) / path 属 junkPaths。
func (r *telemetryRepo) DeleteNoise(ctx context.Context, botUASubstrings, junkPaths []string) (int64, error) {
	clauses := []string{"ua = ''"}
	args := []any{}
	for _, s := range botUASubstrings {
		clauses = append(clauses, "LOWER(ua) LIKE ?")
		args = append(args, "%"+strings.ToLower(s)+"%")
	}
	if len(junkPaths) > 0 {
		clauses = append(clauses, "path IN ?")
		args = append(args, junkPaths)
	}
	res := dbFrom(ctx, r.db).Where("("+strings.Join(clauses, " OR ")+")", args...).
		Delete(&entity.TelemetryEvent{})
	return res.RowsAffected, res.Error
}

// DeleteApiErrors 清理误记的 API HTTP 错误历史: extra.type='error' 且 category 形如 api-4xx/api-5xx(api- 前缀)。
func (r *telemetryRepo) DeleteApiErrors(ctx context.Context) (int64, error) {
	res := dbFrom(ctx, r.db).
		Where("extra->>'$.type' = ? AND category LIKE ?", "error", "api-%").
		Delete(&entity.TelemetryEvent{})
	return res.RowsAffected, res.Error
}

func (r *telemetryRepo) GetResolution(ctx context.Context, keyHash string) (*entity.TelemetryIssueResolution, error) {
	var res entity.TelemetryIssueResolution
	err := dbFrom(ctx, r.db).Where("key_hash = ?", keyHash).First(&res).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *telemetryRepo) UpsertResolution(ctx context.Context, res *entity.TelemetryIssueResolution) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"resolved", "resolved_commit_id", "reopened_count", "updated_at"}),
	}).Create(res).Error
}
