package mysql

import (
	"context"
	"strings"
	"time"

	"server/internal/domain/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// domainReplaceRepo 域名替换规则表读写（2026-11-25）。
// 挂在 searchRepo 上（它已持有 db 且被全站注入），避免为一个新表再加一条组合根接线。
func (r *searchRepo) UpsertDomainRule(ctx context.Context, rule repository.DomainRule) (bool, error) {
	now := nowMilli()
	var existing int64
	if err := dbFrom(ctx, r.db).Model(&domainReplaceRulePO{}).
		Where("field = ? AND from_host = ?", rule.Field, rule.FromHost).
		Count(&existing).Error; err != nil {
		return false, err
	}
	po := domainReplaceRulePO{
		Field: rule.Field, FromHost: rule.FromHost, ToHost: rule.ToHost,
		OpsTag: rule.OpsTag, SourceURL: rule.SourceURL, AnnouncedAt: rule.AnnouncedAt,
		State: "pending", Note: rule.Note, CreatedAt: now, UpdatedAt: now,
	}
	err := dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "field"}, {Name: "from_host"}},
		// ⚠️ 必须用**有序** clause.Set（P1 修复）：ODKU 的 SET 从左到右求值，
		// 而 state 的表达式依赖 to_host 已被先赋值。用 clause.Assignments(map) 顺序不保证 ⇒
		// 会读到旧 to_host ⇒ 把所有规则（含已 applied / rejected）**每天重置回 pending**。
		// 语义：仅在 to_host 变化（新公告改了目标）时才需重新确认；否则保持原状态。
		DoUpdates: clause.Set{
			{Column: clause.Column{Name: "to_host"}, Value: rule.ToHost},
			{Column: clause.Column{Name: "ops_tag"}, Value: rule.OpsTag},
			{Column: clause.Column{Name: "source_url"}, Value: rule.SourceURL},
			{Column: clause.Column{Name: "announced_at"}, Value: rule.AnnouncedAt},
			{Column: clause.Column{Name: "state"}, Value: gorm.Expr("IF(to_host <> VALUES(to_host), 'pending', state)")},
			{Column: clause.Column{Name: "updated_at"}, Value: now},
		},
	}).Create(&po).Error
	return existing == 0, err
}

func (r *searchRepo) ListDomainRules(ctx context.Context, state string) ([]repository.DomainRule, error) {
	var pos []domainReplaceRulePO
	q := dbFrom(ctx, r.db).Model(&domainReplaceRulePO{})
	if strings.TrimSpace(state) != "" {
		q = q.Where("state = ?", state)
	}
	if err := q.Order("announced_at DESC, id DESC").Find(&pos).Error; err != nil {
		return nil, err
	}
	out := make([]repository.DomainRule, 0, len(pos))
	for _, p := range pos {
		out = append(out, p.toDomain())
	}
	return out, nil
}

func (r *searchRepo) GetDomainRule(ctx context.Context, id int64) (*repository.DomainRule, error) {
	var po domainReplaceRulePO
	err := dbFrom(ctx, r.db).Model(&domainReplaceRulePO{}).Where("id = ?", id).First(&po).Error
	if err != nil {
		return nil, err
	}
	d := po.toDomain()
	return &d, nil
}

func (r *searchRepo) SetDomainRuleState(ctx context.Context, id int64, state, note string) error {
	return dbFrom(ctx, r.db).Model(&domainReplaceRulePO{}).Where("id = ?", id).
		Updates(map[string]interface{}{"state": state, "note": note, "updated_at": nowMilli()}).Error
}

// coverHostLike 域名边界匹配：只匹配 "://<host>/" 形态，
// 避免 v.lzcdn28.com 误伤 v.lzcdn28.com.cn、或片名里恰好出现该串。
func coverHostLike(host string) string {
	return "%://" + host + "/%"
}

func (r *searchRepo) CountCoverHost(ctx context.Context, host string) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).Model(&movieSearchPO{}).
		Where("cover LIKE ?", coverHostLike(host)).Count(&n).Error
	return n, err
}

func (r *searchRepo) PreviewCoverReplace(ctx context.Context, fromHost, toHost string, limit int) ([]repository.PicPreview, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []struct {
		Mid   int64
		Cover string
	}
	err := dbFrom(ctx, r.db).Model(&movieSearchPO{}).
		Select("mid, cover").
		Where("cover LIKE ?", coverHostLike(fromHost)).
		Order("mid").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]repository.PicPreview, 0, len(rows))
	for _, row := range rows {
		out = append(out, repository.PicPreview{
			Mid:    row.Mid,
			Before: row.Cover,
			After:  strings.Replace(row.Cover, "://"+fromHost+"/", "://"+toHost+"/", 1),
		})
	}
	return out, nil
}

// domainReplaceRulePO / movieSearchPO 仅用于取表名与扫描，不引入业务逻辑。
type domainReplaceRulePO struct {
	ID          int64  `gorm:"column:id;primaryKey"`
	Field       string `gorm:"column:field"`
	FromHost    string `gorm:"column:from_host"`
	ToHost      string `gorm:"column:to_host"`
	OpsTag      string `gorm:"column:ops_tag"`
	SourceURL   string `gorm:"column:source_url"`
	AnnouncedAt int64  `gorm:"column:announced_at"`
	State       string `gorm:"column:state"`
	Note        string `gorm:"column:note"`
	CreatedAt   int64  `gorm:"column:created_at"`
	UpdatedAt   int64  `gorm:"column:updated_at"`
}

func (domainReplaceRulePO) TableName() string { return "domain_replace_rule" }

func (p domainReplaceRulePO) toDomain() repository.DomainRule {
	return repository.DomainRule{
		ID: p.ID, Field: p.Field, FromHost: p.FromHost, ToHost: p.ToHost,
		OpsTag: p.OpsTag, SourceURL: p.SourceURL, AnnouncedAt: p.AnnouncedAt,
		State: p.State, Note: p.Note, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// movieSearchPO 复用 movie_search 只读扫描（cover/mid）
type movieSearchPO struct {
	Mid   int64  `gorm:"column:mid"`
	Cover string `gorm:"column:cover"`
}

func (movieSearchPO) TableName() string { return "movie_search" }

var _ = gorm.ErrRecordNotFound

// GetImgFallbackHosts 读 site_config.img_fallback_hosts（单行站点配置）。
func (r *searchRepo) GetImgFallbackHosts(ctx context.Context) (string, error) {
	var v string
	err := dbFrom(ctx, r.db).Raw("SELECT COALESCE(img_fallback_hosts, '') FROM site_config ORDER BY id LIMIT 1").Scan(&v).Error
	return v, err
}

// SetImgFallbackHosts 写回 site_config.img_fallback_hosts（只更新现有行，不新建站点）。
func (r *searchRepo) SetImgFallbackHosts(ctx context.Context, v string) error {
	return dbFrom(ctx, r.db).Exec(
		"UPDATE site_config SET img_fallback_hosts = ?, updated_at = ?", v, nowMilli()).Error
}

// ApplyCoverReplace 分批执行封面域名替换（见接口注释）。返回累计影响行数。
func (r *searchRepo) ApplyCoverReplace(ctx context.Context, ruleID int64, fromHost, toHost string, batch, sleepMs int) (int64, error) {
	if batch <= 0 || batch > 5000 {
		batch = 2000
	}
	like := "%://" + fromHost + "/%"
	fromPat := "://" + fromHost + "/"
	toPat := "://" + toHost + "/"
	var cursor, total int64
	for {
		var next int64
		if err := dbFrom(ctx, r.db).Raw(
			"SELECT COALESCE(MAX(mid),0) FROM (SELECT mid FROM movie_search WHERE mid > ? AND cover LIKE ? ORDER BY mid LIMIT ?) t",
			cursor, like, batch).Scan(&next).Error; err != nil {
			return total, err
		}
		if next == 0 {
			return total, nil
		}
		// ① 先记审计（回滚依据）；② 再更新同一区间
		if err := dbFrom(ctx, r.db).Exec(
			"INSERT INTO domain_replace_audit(rule_id, field, mid, before_val, after_val, created_at) "+
				"SELECT ?, 'vod_pic', mid, cover, REPLACE(cover, ?, ?), ? FROM movie_search "+
				"WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			ruleID, fromPat, toPat, nowMilli(), cursor, next, like).Error; err != nil {
			return total, err
		}
		res := dbFrom(ctx, r.db).Exec(
			"UPDATE movie_search SET cover = REPLACE(cover, ?, ?) WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			fromPat, toPat, cursor, next, like)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		cursor = next
		if sleepMs > 0 {
			select {
			case <-ctx.Done():
				return total, ctx.Err()
			case <-time.After(time.Duration(sleepMs) * time.Millisecond):
			}
		}
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
}
