package mysql

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"server/internal/domain/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// domainReplaceRepo 域名替换规则表读写（2026-11-25）。
// 挂在 searchRepo 上（它已持有 db 且被全站注入），避免为一个新表再加一条组合根接线。
//
// **只增不改**（用户要求）：
//
//	· (field, from_host, to_host) 完全一致 ⇒ 跳过（一个字段都不写、状态不动）
//	· 同 (field, from_host, scope, source_code) 但目标变了 ⇒ 新增一条，旧 pending 行标 merged
//	· 人工修正走 UpdateDomainRule（按 id 原地改，不产生新行）
//
// 插入与合并放在**同一事务**：要么"新增 + 旧行归档"一起生效，要么都不生效。
func (r *searchRepo) InsertDomainRuleWithMerge(ctx context.Context, rule repository.DomainRule) (bool, int64, int64, error) {
	var created bool
	var id, merged int64
	err := dbFrom(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		now := nowMilli()
		po := domainReplaceRulePO{
			Field: rule.Field, FieldLabel: rule.FieldLabel, RuleType: rule.RuleType,
			Scope: drOrDefault(rule.Scope, "master"), SourceCode: rule.SourceCode,
			FromHost: rule.FromHost, ToHost: rule.ToHost,
			SourceKey: rule.SourceKey, SourceURL: rule.SourceURL,
			AnnouncedAt: rule.AnnouncedAt, State: "pending", SourceNote: rule.SourceNote,
			CreatedAt: now, UpdatedAt: now,
		}
		// 命中 uk_field_from_to ⇒ 什么都不做（DoNothing 在 MySQL 下生成 ON DUPLICATE KEY UPDATE col=col）
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&po)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			var cur domainReplaceRulePO
			if err := tx.Where("field = ? AND from_host = ? AND to_host = ?",
				rule.Field, rule.FromHost, rule.ToHost).First(&cur).Error; err != nil {
				// 冲突却没查到行 = 不该发生；宁可报错，也不要把这条规则静默当成"已存在"丢掉
				return fmt.Errorf("规则写入未生效且查不到已有行（%s %s → %s）: %w",
					rule.Field, rule.FromHost, rule.ToHost, err)
			}
			id = cur.ID
			// **目标回退**：源站把目标改回了历史值（之前这条被判"已被取代"）⇒ 复活为待处理，
			// 并把别的待处理行归档 —— 否则库里会留一条指向旧目标的 pending，误导执行。
			// 人工决策（skipped/rejected/applied）一律尊重，不复活。
			if cur.State == "merged" {
				if err := tx.Model(&domainReplaceRulePO{}).Where("id = ?", id).
					Updates(map[string]interface{}{
						"state":      "pending",
						"note":       "目标恢复为本次公告值",
						"updated_at": now,
					}).Error; err != nil {
					return err
				}
				mres := tx.Model(&domainReplaceRulePO{}).
					Where("field = ? AND from_host = ? AND scope = ? AND source_code = ? AND id <> ? AND state = ? AND to_host <> ?",
						rule.Field, rule.FromHost, drOrDefault(cur.Scope, "master"), cur.SourceCode, id, "pending", rule.ToHost).
					Updates(map[string]interface{}{
						"state":      "merged",
						"note":       fmt.Sprintf("目标已变更，被 #%d 取代（历史保留）", id),
						"updated_at": now,
					})
				if mres.Error != nil {
					return mres.Error
				}
				merged = mres.RowsAffected
			}
			return nil
		}
		created = true
		id = po.ID
		// 目标变了：同 (field, from_host, scope, source_code) 的更早待处理行归档为 merged。
		// 必须带 scope/source_code 一起匹配 —— 同一个旧域名可能既有主站规则、也有某个从站的人工规则。
		mres := tx.Model(&domainReplaceRulePO{}).
			Where("field = ? AND from_host = ? AND scope = ? AND source_code = ? AND id <> ? AND state = ?",
				rule.Field, rule.FromHost, drOrDefault(rule.Scope, "master"), rule.SourceCode, id, "pending").
			Updates(map[string]interface{}{
				"state":      "merged",
				"note":       fmt.Sprintf("目标已变更，被 #%d 取代（历史保留）", id),
				"updated_at": now,
			})
		if mres.Error != nil {
			return mres.Error
		}
		merged = mres.RowsAffected
		return nil
	})
	if err != nil {
		return false, 0, 0, err
	}
	return created, id, merged, nil
}

// DomainRuleStats 全量计数（列表分页后仍要给出各状态总量与超期告警）。
// staleBefore：公告时间早于该时间戳的 pending 计为超期（0 表示不统计）。
func (r *searchRepo) DomainRuleStats(ctx context.Context, staleBefore int64) (counts map[string]int64, total, stale int64, err error) {
	counts = map[string]int64{}
	var rows []struct {
		State string
		N     int64
		Stale int64
	}
	// 一条 SQL 同时拿到各状态计数与超期数（避免两次全表聚合）
	q := dbFrom(ctx, r.db).Model(&domainReplaceRulePO{}).
		Select("state, COUNT(*) AS n, SUM(CASE WHEN state = ? AND announced_at > 0 AND announced_at < ? THEN 1 ELSE 0 END) AS stale",
			"pending", staleBefore).
		Group("state")
	if err = q.Scan(&rows).Error; err != nil {
		return
	}
	for _, x := range rows {
		counts[x.State] = x.N
		total += x.N
		stale += x.Stale
	}
	return
}

// ListDomainRules 分页列出规则（state 空 = 全部；keyword 匹配新旧域名/备注），按公告时间倒序。
func (r *searchRepo) ListDomainRules(ctx context.Context, state, keyword string, page, size int) ([]repository.DomainRule, int64, error) {
	if size <= 0 || size > 200 {
		size = 24
	}
	if page <= 0 {
		page = 1
	}
	base := func() *gorm.DB {
		q := dbFrom(ctx, r.db).Model(&domainReplaceRulePO{})
		if strings.TrimSpace(state) != "" {
			q = q.Where("state = ?", state)
		}
		if kw := strings.TrimSpace(keyword); kw != "" {
			like := likeContains(kw)
			q = q.Where("from_host LIKE ? OR to_host LIKE ? OR source_note LIKE ?", like, like, like)
		}
		return q
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var pos []domainReplaceRulePO
	if err := base().Order("announced_at DESC, id DESC").
		Offset((page - 1) * size).Limit(size).Find(&pos).Error; err != nil {
		return nil, 0, err
	}
	out := make([]repository.DomainRule, 0, len(pos))
	for _, p := range pos {
		out = append(out, p.toDomain())
	}
	return out, total, nil
}

// AnnotateSiblingRules 执行替换后，给同域名的**其它**待处理规则加一条提示。
// 只写"备注为空"的行：管理员写过备注说明已经人工看过，绝不覆盖。
func (r *searchRepo) AnnotateSiblingRules(ctx context.Context, field, fromHost, appliedTo string, keepID int64) (int64, error) {
	note := fmt.Sprintf("注意：同域名已按 %s 执行（#%d），本规则目标不同，请复核", appliedTo, keepID)
	res := dbFrom(ctx, r.db).Model(&domainReplaceRulePO{}).
		Where("field = ? AND from_host = ? AND id <> ? AND state = ? AND to_host <> ? AND note = ''",
			field, fromHost, keepID, "pending", appliedTo).
		Updates(map[string]interface{}{"note": note, "updated_at": nowMilli()})
	return res.RowsAffected, res.Error
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

// coverBareLike 不带协议头的形态（少数历史入库值形如 "p.lztutu.com/upload/..."）：
// 用**前缀**匹配，只有从开头就是该域名才算，避免误伤子域名。
func coverBareLike(host string) string {
	return host + "/%"
}

// replaceCoverHost 预览用：按两种形态各替换一次（真实替换在 SQL 里分两轮跑）。
func replaceCoverHost(v, fromHost, toHost string) string {
	if s := "://" + fromHost + "/"; strings.Contains(v, s) {
		return strings.Replace(v, s, "://"+toHost+"/", 1)
	}
	if p := fromHost + "/"; strings.HasPrefix(v, p) {
		return toHost + "/" + strings.TrimPrefix(v, p)
	}
	return v
}

func (r *searchRepo) CountCoverHost(ctx context.Context, host string) (int64, error) {
	var n int64
	// 统计改读**真相源 movie**：movie_search 是影子表（由 movie 重建），
	// 只统计影子表会漏掉"尚未重建"的行，也会与执行范围不一致。
	err := dbFrom(ctx, r.db).Model(&moviePO{}).
		Where("cover LIKE ? OR cover LIKE ?", coverHostLike(host), coverBareLike(host)).
		Count(&n).Error
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
	err := dbFrom(ctx, r.db).Model(&moviePO{}).
		Select("mid, cover").
		Where("cover LIKE ? OR cover LIKE ?", coverHostLike(fromHost), coverBareLike(fromHost)).
		Order("mid").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]repository.PicPreview, 0, len(rows))
	for _, row := range rows {
		out = append(out, repository.PicPreview{
			Mid:    row.Mid,
			Before: row.Cover,
			After:  replaceCoverHost(row.Cover, fromHost, toHost),
		})
	}
	return out, nil
}

// ApplyCoverReplace 分批执行封面域名替换（见接口注释）。返回累计影响行数。
//
// ⚠️ **关键（P0，2026-11-25 审查发现）**：`movie_search` 是从 `movie` 重建的**影子表**，
// 只改影子表会在下次重建时被**写回旧域名**（等于白做）。故必须：
//
//	① 审计记 **movie.cover** 的 before/after（movie 是真相源）
//	② 先更新 **movie.cover**
//	③ 再更新 movie_search.cover（让线上立即可见；即使不更新，重建后也会跟随）
//
// 游标由 **movie** 驱动（两表 mid 一一对应，命中集合一致）。
// 分两轮：先处理带协议的 `://host/`，再处理极少数不带协议的 `host/` 前缀形态。
func (r *searchRepo) ApplyCoverReplace(ctx context.Context, ruleID int64, fromHost, toHost string, batch, sleepMs int) (int64, error) {
	if batch <= 0 || batch > 5000 {
		batch = 2000
	}
	var total int64

	// ---- 第一轮：带协议 ----
	like := coverHostLike(fromHost)
	fromPat := "://" + fromHost + "/"
	toPat := "://" + toHost + "/"
	var cursor int64
	for {
		var next int64
		if err := dbFrom(ctx, r.db).Raw(
			"SELECT COALESCE(MAX(mid),0) FROM (SELECT mid FROM movie WHERE mid > ? AND cover LIKE ? ORDER BY mid LIMIT ?) t",
			cursor, like, batch).Scan(&next).Error; err != nil {
			return total, err
		}
		if next == 0 {
			break
		}
		if err := dbFrom(ctx, r.db).Exec(
			"INSERT INTO domain_replace_audit(rule_id, field, mid, before_val, after_val, created_at) "+
				"SELECT ?, 'vod_pic', mid, cover, REPLACE(cover, ?, ?), ? FROM movie "+
				"WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			ruleID, fromPat, toPat, nowMilli(), cursor, next, like).Error; err != nil {
			return total, err
		}
		res := dbFrom(ctx, r.db).Exec(
			"UPDATE movie SET cover = REPLACE(cover, ?, ?) WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			fromPat, toPat, cursor, next, like)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if err := dbFrom(ctx, r.db).Exec(
			"UPDATE movie_search SET cover = REPLACE(cover, ?, ?) WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			fromPat, toPat, cursor, next, like).Error; err != nil {
			log.Printf("[macrep] 影子表 movie_search 更新失败（下次重建会跟随真相源）: %v", err)
		}
		cursor = next
		if !sleepCtxOr(ctx, sleepMs) {
			return total, ctx.Err()
		}
	}

	// ---- 第二轮：不带协议（前缀形态）----
	bareLike := coverBareLike(fromHost)
	toPrefix := toHost + "/"
	// SUBSTRING 是 1-based，跳过的正是 "from/"，所以从 len("from/")+1 开始
	skip := len(fromHost) + 1
	cursor = 0
	for {
		var next int64
		if err := dbFrom(ctx, r.db).Raw(
			"SELECT COALESCE(MAX(mid),0) FROM (SELECT mid FROM movie WHERE mid > ? AND cover LIKE ? ORDER BY mid LIMIT ?) t",
			cursor, bareLike, batch).Scan(&next).Error; err != nil {
			return total, err
		}
		if next == 0 {
			break
		}
		if err := dbFrom(ctx, r.db).Exec(
			"INSERT INTO domain_replace_audit(rule_id, field, mid, before_val, after_val, created_at) "+
				"SELECT ?, 'vod_pic', mid, cover, CONCAT(?, SUBSTRING(cover, ?)), ? FROM movie "+
				"WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			ruleID, toPrefix, skip+1, nowMilli(), cursor, next, bareLike).Error; err != nil {
			return total, err
		}
		res := dbFrom(ctx, r.db).Exec(
			"UPDATE movie SET cover = CONCAT(?, SUBSTRING(cover, ?)) WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			toPrefix, skip+1, cursor, next, bareLike)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if err := dbFrom(ctx, r.db).Exec(
			"UPDATE movie_search SET cover = CONCAT(?, SUBSTRING(cover, ?)) WHERE mid > ? AND mid <= ? AND cover LIKE ?",
			toPrefix, skip+1, cursor, next, bareLike).Error; err != nil {
			log.Printf("[macrep] 影子表 movie_search 基础形态更新失败（下次重建会跟随真相源）: %v", err)
		}
		cursor = next
		if !sleepCtxOr(ctx, sleepMs) {
			return total, ctx.Err()
		}
	}
	return total, nil
}

// sleepCtxOr 批间让路；ctx 取消返回 false。
func sleepCtxOr(ctx context.Context, ms int) bool {
	if ms <= 0 {
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return true
	}
}

// UpdateDomainRule 人工编辑单条规则（原地改，不新增行）。
// 若改后的 (field, from_host, to_host) 与另一条重复 ⇒ 报错让用户去编辑那一条。
func (r *searchRepo) UpdateDomainRule(ctx context.Context, id int64, rule repository.DomainRule) error {
	var dup int64
	if err := dbFrom(ctx, r.db).Model(&domainReplaceRulePO{}).
		Where("field = ? AND from_host = ? AND to_host = ? AND id <> ?", rule.Field, rule.FromHost, rule.ToHost, id).
		Count(&dup).Error; err != nil {
		return err
	}
	if dup > 0 {
		return fmt.Errorf("已存在相同规则（%s %s → %s），请直接编辑那一条", rule.Field, rule.FromHost, rule.ToHost)
	}
	return dbFrom(ctx, r.db).Model(&domainReplaceRulePO{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"field":        rule.Field,
			"field_label":  rule.FieldLabel,
			"rule_type":    rule.RuleType,
			"scope":        drOrDefault(rule.Scope, "master"),
			"source_code":  rule.SourceCode,
			"from_host":    rule.FromHost,
			"to_host":      rule.ToHost,
			"source_key":   rule.SourceKey,
			"source_url":   rule.SourceURL,
			"announced_at": rule.AnnouncedAt,
			"source_note":  rule.SourceNote,
			"state":        rule.State,
			"updated_at":   nowMilli(),
		}).Error
}

// likeContains 转义 LIKE 通配符（用户搜 "100%" 时不该变成任意匹配）。
func likeContains(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	return "%" + r.Replace(s) + "%"
}

// drOrDefault 空串取默认值（scope 的默认是 master）。
func drOrDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// domainReplaceRulePO 规则表映射（只含我们真正存储的列）。
type domainReplaceRulePO struct {
	ID          int64  `gorm:"column:id;primaryKey"`
	Field       string `gorm:"column:field"`
	FieldLabel  string `gorm:"column:field_label"`
	RuleType    string `gorm:"column:rule_type"`
	Scope       string `gorm:"column:scope"`
	SourceCode  string `gorm:"column:source_code"`
	FromHost    string `gorm:"column:from_host"`
	ToHost      string `gorm:"column:to_host"`
	SourceKey   string `gorm:"column:source_key"`
	SourceURL   string `gorm:"column:source_url"`
	AnnouncedAt int64  `gorm:"column:announced_at"`
	State       string `gorm:"column:state"`
	Note        string `gorm:"column:note"`
	SourceNote  string `gorm:"column:source_note"`
	CreatedAt   int64  `gorm:"column:created_at"`
	UpdatedAt   int64  `gorm:"column:updated_at"`
}

func (domainReplaceRulePO) TableName() string { return "domain_replace_rule" }

func (p domainReplaceRulePO) toDomain() repository.DomainRule {
	return repository.DomainRule{
		ID: p.ID, Field: p.Field, FieldLabel: p.FieldLabel, RuleType: p.RuleType,
		Scope: p.Scope, SourceCode: p.SourceCode,
		FromHost: p.FromHost, ToHost: p.ToHost,
		SourceKey: p.SourceKey, SourceURL: p.SourceURL,
		AnnouncedAt: p.AnnouncedAt,
		State:       p.State, Note: p.Note, SourceNote: p.SourceNote,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

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

// moviePO 复用 movie 表只读扫描（mid/cover）
type moviePO struct {
	Mid   int64  `gorm:"column:mid"`
	Cover string `gorm:"column:cover"`
}

func (moviePO) TableName() string { return "movie" }
