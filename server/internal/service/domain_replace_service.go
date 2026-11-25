package service

import (
	"context"
	"fmt"
	"io"
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
	// 拉取后把**图片**域名（旧+新）并入前端图床兜底列表 site_config.img_fallback_hosts：
	// 前端 BaseImage 会把第一项当主图床、其余当备用（onerror 切 host、路径原样保留），
	// 于是"旧图 404 → 自动切新域名"，替换期间也不会裂图。
	// 只并 vod_pic；播放/下载域名与图片无关，不进该列表。
	var picHosts []string
	for _, r := range rules {
		if r.Field == macrep.FieldPic {
			picHosts = append(picHosts, r.FromHost, r.ToHost)
		}
	}
	if len(picHosts) > 0 {
		cur, herr := s.search.GetImgFallbackHosts(ctx)
		if herr == nil {
			if merged := mergeImgHosts(cur, picHosts); merged != cur {
				_ = s.search.SetImgFallbackHosts(ctx, merged)
			}
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

// ListDomainRules 列出规则（state 空=全部）
func (s *FilmService) ListDomainRules(ctx context.Context, state string) ([]DomainRuleView, error) {
	rows, err := s.search.ListDomainRules(ctx, state)
	if err != nil {
		return nil, err
	}
	out := make([]DomainRuleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainRuleView(r))
	}
	return out, nil
}

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

// mergeImgHosts 合并图床 host 列表（逗号分隔）：
//   - 保持**原有顺序**（第一项是主图床，语义不能变）
//   - 追加新 host 时**去重**（忽略大小写与首尾空白）、忽略空串、只接受裸 host
//   - 上限 24 个，避免随公告无限增长
//
// 这一条同时满足"自动过滤掉相同的"：同一域名出现多次只保留一次。
func mergeImgHosts(current string, add []string) string {
	const maxHosts = 24
	seen := map[string]bool{}
	out := make([]string, 0, maxHosts)
	push := func(h string) {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || seen[h] || len(out) >= maxHosts {
			return
		}
		seen[h] = true
		out = append(out, h)
	}
	for _, p := range strings.Split(current, ",") {
		push(p)
	}
	for _, h := range add {
		push(h)
	}
	return strings.Join(out, ",")
}
