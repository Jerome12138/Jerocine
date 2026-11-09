package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/bloburl"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// ---- collect_source ----

type sourceRepo struct{ db *gorm.DB }

func NewCollectSourceRepository(db *gorm.DB) repository.CollectSourceRepository {
	return &sourceRepo{db: db}
}

func (r *sourceRepo) List(ctx context.Context, onlyEnabled bool) ([]entity.CollectSource, error) {
	q := dbFrom(ctx, r.db).Model(&entity.CollectSource{})
	if onlyEnabled {
		q = q.Where("state = ?", entity.StateEnabled)
	}
	var out []entity.CollectSource
	err := q.Order("grade ASC, grade_score DESC").Find(&out).Error
	return out, err
}

func (r *sourceRepo) Get(ctx context.Context, id string) (*entity.CollectSource, error) {
	var s entity.CollectSource
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *sourceRepo) ExistsByUri(ctx context.Context, uri, excludeId string) (bool, error) {
	q := dbFrom(ctx, r.db).Model(&entity.CollectSource{}).Where("uri = ?", uri)
	if excludeId != "" {
		q = q.Where("id <> ?", excludeId)
	}
	var n int64
	err := q.Count(&n).Error
	return n > 0, err
}

// Upsert 冲突(同 id 已存在, 即编辑/启停走 PUT)时只更新管理页可编辑列。
// 此前 UpdateAll:true 全列覆盖, 而前端 PUT DTO 不透传 grade_score/created_at:
//   - grade_score 是同等级源的人工优先级(List 按 grade ASC, grade_score DESC 排序消费),
//     绑定 JSON 里缺该字段 → 落 0, 任何一次编辑/启停都会把已调好的优先级静默清零;
//   - created_at 是建档时间, 不应随编辑刷新。
// 两者均为服务端管辖字段, 在仓储层排除, 不依赖各调用方自觉透传。
func (r *sourceRepo) Upsert(ctx context.Context, s *entity.CollectSource) error {
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "uri", "site_url", "result_model", "grade",
			"sync_pictures", "collect_type", "interval_ms", "state", "updated_at",
		}),
	}).Create(s).Error
}

func (r *sourceRepo) Delete(ctx context.Context, id string) error {
	return dbFrom(ctx, r.db).Where("id = ?", id).Delete(&entity.CollectSource{}).Error
}

// ---- cron_task ----

type cronRepo struct{ db *gorm.DB }

func NewCronTaskRepository(db *gorm.DB) repository.CronTaskRepository { return &cronRepo{db: db} }

func (r *cronRepo) List(ctx context.Context) ([]entity.CronTask, error) {
	var out []entity.CronTask
	err := dbFrom(ctx, r.db).Order("id ASC").Find(&out).Error
	return out, err
}

func (r *cronRepo) Get(ctx context.Context, id int64) (*entity.CronTask, error) {
	var t entity.CronTask
	err := dbFrom(ctx, r.db).Where("id = ?", id).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *cronRepo) Upsert(ctx context.Context, t *entity.CronTask) error {
	return dbFrom(ctx, r.db).Save(t).Error
}

func (r *cronRepo) Delete(ctx context.Context, id int64) error {
	return dbFrom(ctx, r.db).Where("id = ?", id).Delete(&entity.CronTask{}).Error
}

// ---- site_config (单行 id=1) ----

type siteConfigRepo struct{ db *gorm.DB }

func NewSiteConfigRepository(db *gorm.DB) repository.SiteConfigRepository {
	return &siteConfigRepo{db: db}
}

func (r *siteConfigRepo) Get(ctx context.Context) (*entity.SiteConfig, error) {
	var c entity.SiteConfig
	err := dbFrom(ctx, r.db).Where("id = ?", 1).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &entity.SiteConfig{Id: 1}, nil // 兜底空配置, 不报错
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *siteConfigRepo) Save(ctx context.Context, c *entity.SiteConfig) error {
	c.Id = 1
	logo := c.Logo // 落库存裸 key, 写完还原为 URL 视角(见 internal/bloburl)
	c.Logo = bloburl.ToKey(logo)
	err := dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}}, UpdateAll: true,
	}).Create(c).Error
	c.Logo = logo
	return err
}

// ---- app_version ----

type appVersionRepo struct{ db *gorm.DB }

func NewAppVersionRepository(db *gorm.DB) repository.AppVersionRepository {
	return &appVersionRepo{db: db}
}

func (r *appVersionRepo) Latest(ctx context.Context, channel int8) (*entity.AppVersion, error) {
	var v entity.AppVersion
	err := dbFrom(ctx, r.db).Where("channel = ?", channel).Order("version_code DESC").First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *appVersionRepo) List(ctx context.Context, page repository.Page) ([]entity.AppVersion, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.AppVersion{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.AppVersion
	err := q.Order("id DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}

func (r *appVersionRepo) Create(ctx context.Context, v *entity.AppVersion) error {
	apkURL := v.ApkUrl // 落库存裸 key, 写完还原(见 internal/bloburl)
	v.ApkUrl = bloburl.ToKey(apkURL)
	err := dbFrom(ctx, r.db).Create(v).Error
	v.ApkUrl = apkURL
	return err
}

func (r *appVersionRepo) Delete(ctx context.Context, id int64) error {
	return dbFrom(ctx, r.db).Where("id = ?", id).Delete(&entity.AppVersion{}).Error
}

// ---- files ----

type fileRepo struct{ db *gorm.DB }

func NewFileRepository(db *gorm.DB) repository.FileRepository { return &fileRepo{db: db} }

func (r *fileRepo) GetCoversByRelevance(ctx context.Context, relevanceIds []int64) (map[int64]string, error) {
	out := make(map[int64]string)
	if len(relevanceIds) == 0 {
		return out, nil
	}
	var files []entity.FileInfo
	// 命中 idx_relevance(relevance_id, type), 根治封面回填全表扫
	if err := dbFrom(ctx, r.db).Where("relevance_id IN ? AND type = ?", relevanceIds, entity.FileTypeCover).
		Find(&files).Error; err != nil {
		return nil, err
	}
	for _, f := range files {
		out[f.RelevanceId] = f.Link
	}
	return out, nil
}

func (r *fileRepo) Create(ctx context.Context, f *entity.FileInfo) error {
	link := f.Link // 落库存裸 key, 写完还原为 URL 视角(见 internal/bloburl)
	f.Link = bloburl.ToKey(link)
	err := dbFrom(ctx, r.db).Create(f).Error
	f.Link = link
	return err
}

func (r *fileRepo) Delete(ctx context.Context, id int64) error {
	return dbFrom(ctx, r.db).Where("id = ?", id).Delete(&entity.FileInfo{}).Error
}

func (r *fileRepo) List(ctx context.Context, page repository.Page) ([]entity.FileInfo, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.FileInfo{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.FileInfo
	err := q.Order("id DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error
	return out, total, err
}
