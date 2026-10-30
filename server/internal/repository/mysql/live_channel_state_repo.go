package mysql

import (
	"context"

	"gorm.io/gorm"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type liveChannelStateRepo struct{ db *gorm.DB }

// NewLiveChannelStateRepository 构造直播频道状态仓储。
func NewLiveChannelStateRepository(db *gorm.DB) repository.LiveChannelStateRepository {
	return &liveChannelStateRepo{db: db}
}

func (r *liveChannelStateRepo) ListBySource(ctx context.Context, sourceID int64) ([]entity.LiveChannelState, error) {
	var out []entity.LiveChannelState
	err := dbFrom(ctx, r.db).
		Where("source_id = ?", sourceID).
		Order("id ASC").
		Find(&out).Error
	return out, err
}

// Upsert 按 (source_id, url) 覆盖写入。gorm 的 OnConflict 在前缀唯一键(url(191))上
// 不可靠 —— 先查后写（该表写入频率极低, 无并发热点）。
func (r *liveChannelStateRepo) Upsert(ctx context.Context, st *entity.LiveChannelState) error {
	db := dbFrom(ctx, r.db)
	var n int64
	if err := db.Model(&entity.LiveChannelState{}).
		Where("source_id = ? AND url = ?", st.SourceId, st.URL).
		Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return db.Model(&entity.LiveChannelState{}).
			Where("source_id = ? AND url = ?", st.SourceId, st.URL).
			Updates(map[string]any{
				"name":         st.Name,
				"group_name":   st.GroupName,
				"hidden":       st.Hidden,
				"display_name": st.DisplayName,
				"is_custom":    st.IsCustom,
			}).Error
	}
	return db.Create(st).Error
}

func (r *liveChannelStateRepo) Delete(ctx context.Context, sourceID int64, url string) error {
	res := dbFrom(ctx, r.db).
		Where("source_id = ? AND url = ?", sourceID, url).
		Delete(&entity.LiveChannelState{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *liveChannelStateRepo) DeleteBySource(ctx context.Context, sourceID int64) error {
	return dbFrom(ctx, r.db).
		Where("source_id = ?", sourceID).
		Delete(&entity.LiveChannelState{}).Error
}
