package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"server/internal/domain/repository"
	"server/internal/macrep"
)

// DomainReplaceDefaultSource 默认公告来源。
// 页面/域名都会换 ⇒ 作为**默认值**而非硬编码，管理端可按次覆盖。
const DomainReplaceDefaultSource = "https://www.lzizy.com/index.php/macrep.html"

// DomainFetchResult 拉取结果
type DomainFetchResult struct {
	Source    string `json:"source"`
	New       int    `json:"new"`
	Updated   int    `json:"updated"`
	Rejected  int    `json:"rejected"`
	FetchedAt int64  `json:"fetchedAt"`
}

// DomainPreviewResult 影响面预览（**只读**）
type DomainPreviewResult struct {
	Rule   DomainRuleView `json:"rule"`
	Hits   int64          `json:"hits"`
	Sample []PicSample    `json:"sample"`
}

// DomainRuleView 出参视图
type DomainRuleView struct {
	ID          int64  `json:"id"`
	Field       string `json:"field"`
	FromHost    string `json:"fromHost"`
	ToHost      string `json:"toHost"`
	OpsTag      string `json:"opsTag"`
	AnnouncedAt int64  `json:"announcedAt"`
	State       string `json:"state"`
	Note        string `json:"note"`
}

// PicSample 封面替换样例
type PicSample struct {
	Mid    int64  `json:"mid"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// FetchDomainRules 拉公告页 → 解析（严格 host→host）→ **只写规则表**，不动业务数据。
func (s *FilmService) FetchDomainRules(ctx context.Context, sourceURL string) (DomainFetchResult, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		sourceURL = DomainReplaceDefaultSource
	}
	res := DomainFetchResult{Source: sourceURL, FetchedAt: time.Now().UnixMilli()}
	cli := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return res, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; jerocine-macrep/1.0)")
	resp, err := cli.Do(req)
	if err != nil {
		return res, fmt.Errorf("拉取公告失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("拉取公告失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return res, err
	}
	rules, rejects := macrep.ParseRules(string(body))
	res.Rejected = len(rejects)
	// 图床兜底链（site_config.img_fallback_hosts）：
	// 前端 BaseImage 把第一项当主图床、其余当备用（onerror 切 host、路径原样保留）。
	// 用户要求（2026-11-25）：**只保留最新 2 个**，每次拉取直接整链替换（不累积历史域名）：
	//   [ 新域名(主), 旧域名(备) ] —— 取**最新一条 vod_pic 规则**（按公告时间）。
	// 只处理 vod_pic；播放/下载域名与图片无关，不进该列表。
	var newest *macrep.Rule
	for i := range rules {
		if rules[i].Field != macrep.FieldPic {
			continue
		}
		if newest == nil || rules[i].AnnouncedAt > newest.AnnouncedAt {
			newest = &rules[i]
		}
	}
	if newest != nil {
		if v := buildImgHosts(newest.ToHost, newest.FromHost); v != "" {
			_ = s.search.SetImgFallbackHosts(ctx, v)
		}
	}
	now := time.Now().UnixMilli()
	for _, r := range rules {
		isNew, uerr := s.search.UpsertDomainRule(ctx, domainRuleFromParsed(r, sourceURL, now))
		if uerr != nil {
			return res, uerr
		}
		if isNew {
			res.New++
		} else {
			res.Updated++
		}
	}
	return res, nil
}

// DomainRuleStaleAfter 待处理规则超过该时长即计入"告警"
const DomainRuleStaleAfter = 7 * 24 * time.Hour

// ListDomainRules 列出规则（state 空=全部），并返回**告警计数**：
//
//	pending —— 待处理规则数（前端红点）
//	stale   —— 其中公告已超过 DomainRuleStaleAfter 仍未处理（强提醒）
//
// 计数在内存里算，不额外查库；"命中行数"属于执行影响面，走 Preview 按需拉取。
func (s *FilmService) ListDomainRules(ctx context.Context, state string) ([]DomainRuleView, int, int, error) {
	rows, err := s.search.ListDomainRules(ctx, state)
	if err != nil {
		return nil, 0, 0, err
	}
	out := make([]DomainRuleView, 0, len(rows))
	pending, stale := 0, 0
	cutoff := time.Now().Add(-DomainRuleStaleAfter).UnixMilli()
	for _, r := range rows {
		out = append(out, toDomainRuleView(r))
		if r.State == "pending" {
			pending++
			if r.AnnouncedAt > 0 && r.AnnouncedAt < cutoff {
				stale++
			}
		}
	}
	return out, pending, stale, nil
}

// StartDomainRuleFetcher 每日一次拉取公告（**只入库，绝不执行替换**）。
// 首次等 DomainFetchInitialDelay 再跑（避免重启风暴打公告站），之后 24h 一轮。
func (s *FilmService) StartDomainRuleFetcher(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[macrep] fetcher panic: %v", r)
			}
		}()
		if !sleepCtx(ctx, DomainFetchInitialDelay) {
			return
		}
		for {
			if res, err := s.FetchDomainRules(ctx, ""); err != nil {
				log.Printf("[macrep] 每日拉取失败: %v", err)
			} else {
				log.Printf("[macrep] 每日拉取完成: 新增 %d / 覆盖 %d / 拒收 %d（仅入库，未执行替换）",
					res.New, res.Updated, res.Rejected)
			}
			if !sleepCtx(ctx, DomainFetchInterval) {
				return
			}
		}
	}()
}

// DomainFetchInitialDelay / DomainFetchInterval 拉取节奏（与 hot worker 同款：启动不立即跑）
var (
	DomainFetchInitialDelay = 10 * time.Minute
	DomainFetchInterval     = 24 * time.Hour
)

// PreviewDomainRule 影响面预览：**只读**。目前仅实现 vod_pic（封面，实测的主战场）。
func (s *FilmService) PreviewDomainRule(ctx context.Context, id int64) (DomainPreviewResult, error) {
	var out DomainPreviewResult
	rule, err := s.search.GetDomainRule(ctx, id)
	if err != nil {
		return out, err
	}
	out.Rule = toDomainRuleView(*rule)
	if rule.Field != macrep.FieldPic {
		// 播放地址实测"新采已换好"，且位于 JSON 大列内，需按 JSON 逐 url 处理
		// ⇒ 执行侧单独实现，本预览只覆盖封面
		out.Hits = 0
		out.Sample = []PicSample{}
		return out, nil
	}
	hits, err := s.search.CountCoverHost(ctx, rule.FromHost)
	if err != nil {
		return out, err
	}
	out.Hits = hits
	rows, err := s.search.PreviewCoverReplace(ctx, rule.FromHost, rule.ToHost, 20)
	if err != nil {
		return out, err
	}
	out.Sample = make([]PicSample, 0, len(rows))
	for _, r := range rows {
		out.Sample = append(out.Sample, PicSample{Mid: r.Mid, Before: r.Before, After: r.After})
	}
	return out, nil
}

// domainRuleFromParsed 把解析结果转成仓储实体（补来源与时间戳）
func domainRuleFromParsed(r macrep.Rule, sourceURL string, now int64) repository.DomainRule {
	return repository.DomainRule{
		Field: r.Field, FromHost: r.FromHost, ToHost: r.ToHost,
		OpsTag: r.OpsTag, SourceURL: sourceURL, AnnouncedAt: r.AnnouncedAt,
		CreatedAt: now, UpdatedAt: now,
	}
}

func toDomainRuleView(r repository.DomainRule) DomainRuleView {
	return DomainRuleView{
		ID: r.ID, Field: r.Field, FromHost: r.FromHost, ToHost: r.ToHost,
		OpsTag: r.OpsTag, AnnouncedAt: r.AnnouncedAt, State: r.State, Note: r.Note,
	}
}

// buildImgHosts 图床兜底链**只保留最新 2 个**：[新域名(主), 旧域名(备)]。
//
// 用户要求（2026-11-25）：不再累积历史域名，每次拉取把整链换成最近的一对。
// 去重：新==旧 时只留一个。空值忽略（不写空链，避免把已有配置擦掉）。
func buildImgHosts(primary, backup string) string {
	p := strings.ToLower(strings.TrimSpace(primary))
	b := strings.ToLower(strings.TrimSpace(backup))
	out := make([]string, 0, 2)
	if p != "" {
		out = append(out, p)
	}
	if b != "" && b != p {
		out = append(out, b)
	}
	return strings.Join(out, ",")
}

// SetDomainRuleState 人工标记规则状态（不改业务数据，只改规则表）
func (s *FilmService) SetDomainRuleState(ctx context.Context, id int64, state, note string) error {
	return s.search.SetDomainRuleState(ctx, id, state, note)
}

// ApplyDomainRule **执行替换**（会改动业务数据），守卫从严：
//  1. 仅支持 vod_pic：播放/下载地址在 episodes_json（JSON 大列）里，必须按 JSON 逐 url 处理；
//  2. **二次确认**：confirmFrom/confirmTo 必须与规则逐字一致（防误触）；
//  3. 已 applied 的规则拒绝重复执行；
//  4. 分批 + 批间 sleep，全程可被 ctx 中断；每批先写审计再更新。
//
// 注（P2-4）：审计 INSERT 与 UPDATE **不在同一事务** —— 若 UPDATE 失败会多记一批审计，
// 但审计"只多不少"且替换本身幂等，重复执行会 0 行命中，故不额外引入事务复杂度。
func (s *FilmService) ApplyDomainRule(ctx context.Context, id int64, confirmFrom, confirmTo string, batch, sleepMs int) (int64, error) {
	rule, err := s.search.GetDomainRule(ctx, id)
	if err != nil {
		return 0, err
	}
	if rule.Field != macrep.FieldPic {
		return 0, fmt.Errorf("暂只支持封面(vod_pic)执行；%s 位于 JSON 大列，需按 url 逐条处理，尚未开放", rule.Field)
	}
	if rule.State == "applied" {
		return 0, fmt.Errorf("该规则已标记为已执行")
	}
	if strings.TrimSpace(confirmFrom) != rule.FromHost || strings.TrimSpace(confirmTo) != rule.ToHost {
		return 0, fmt.Errorf("二次确认不匹配：期望 %s → %s", rule.FromHost, rule.ToHost)
	}
	n, err := s.search.ApplyCoverReplace(ctx, id, rule.FromHost, rule.ToHost, batch, sleepMs)
	if err != nil {
		return n, err
	}
	if serr := s.search.SetDomainRuleState(ctx, id, "applied", fmt.Sprintf("已替换 %d 行", n)); serr != nil {
		log.Printf("[macrep] 规则 %d 已替换 %d 行，但状态更新失败: %v", id, n, serr)
	}
	return n, nil
}
