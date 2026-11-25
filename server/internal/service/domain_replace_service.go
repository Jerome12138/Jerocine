package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"server/internal/domain/repository"
	"server/internal/macrep"
)

// DomainReplaceDefaultSource 默认公告来源。
// 页面/域名都会换 ⇒ 作为**默认值**而非硬编码，管理端可按次覆盖。
const DomainReplaceDefaultSource = "https://www.lzizy.com/index.php/macrep.html"

// DomainSource 一个可拉取的公告来源。
// 未来会有多个站点/类型（封面站、播放站…），管理端「立即拉取公告」弹窗就按这个清单渲染。
type DomainSource struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	Type       string `json:"type"` // macrep=替换助手公告 / custom=自定义地址
	URL        string `json:"url"`
	FieldHint  string `json:"fieldHint"`  // 该来源通常会带来哪类字段的规则
	TargetHint string `json:"targetHint"` // 这些字段落到我们库的哪里（后端口径，前端只展示）
	Note       string `json:"note"`
}

// DomainSources 当前支持的公告来源。**新增站点只改这里**，前端无需改代码。
var DomainSources = []DomainSource{
	{
		Key: "lz", Name: "lz 量子资源", Type: "macrep", URL: DomainReplaceDefaultSource,
		FieldHint:  "封面 / 播放 / 下载",
		TargetHint: "封面→movie.cover（可执行）；播放→movie_play_source.episodes_json（只展示）；下载→本库不存（自动跳过）",
		Note:       "替换助手公告页（data-rep-data 结构化数据）",
	},
}

// ListDomainSources 返回支持的来源清单（返回副本，避免调用方改到全局）。
func (s *FilmService) ListDomainSources() []DomainSource {
	out := make([]DomainSource, len(DomainSources))
	copy(out, DomainSources)
	return out
}

// ResolveDomainSource 把「来源 key」或「完整 URL」解析成实际拉取地址。
func ResolveDomainSource(in string) (DomainSource, string) {
	in = strings.TrimSpace(in)
	if in == "" {
		if len(DomainSources) > 0 {
			return DomainSources[0], DomainSources[0].URL
		}
		return DomainSource{}, ""
	}
	for _, src := range DomainSources {
		if strings.EqualFold(src.Key, in) {
			return src, src.URL
		}
	}
	key := macrep.SourceKeyFromURL(in)
	name := key
	if name == "" {
		name = in
	}
	return DomainSource{Key: key, Name: name, Type: "custom", URL: in, FieldHint: "未知（按公告页解析）"}, in
}

// DomainFetchResult 拉取结果
type DomainFetchResult struct {
	Source    string `json:"source"`
	SourceKey string `json:"sourceKey"`
	New       int    `json:"new"`
	Merged    int    `json:"merged"`  // 目标变更、旧行被标 merged 的条数
	Skipped   int    `json:"skipped"` // 与库内完全一致，跳过
	Rejected  int    `json:"rejected"`
	FetchedAt int64  `json:"fetchedAt"`
}

// DomainPreviewResult 影响面预览（**只读**）
type DomainPreviewResult struct {
	Rule   DomainRuleView `json:"rule"`
	Hits   int64          `json:"hits"`
	Sample []PicSample    `json:"sample"`
	Sites  []string       `json:"sites"` // 播放规则：命中的采集源
}

// DomainRuleView 出参视图
type DomainRuleView struct {
	ID           int64  `json:"id"`
	Field        string `json:"field"`
	FieldLabel   string `json:"fieldLabel"`
	RuleType     string `json:"ruleType"`
	Scope        string `json:"scope"`      // master 主站 / slave 从站
	SourceCode   string `json:"sourceCode"` // 从站时的采集源 id
	SourceKey    string `json:"sourceKey"`  // 来源站标识：lz / manual …
	SourceURL    string `json:"sourceUrl"`  // 来源页 URL（卡片上可点开原文）
	FromHost     string `json:"fromHost"`
	ToHost       string `json:"toHost"`
	TargetColumn string `json:"targetColumn"` // 在我们库里的落点（如 movie.cover），空=本库不存储
	TargetNote   string `json:"targetNote"`   // 落点说明（为什么能/不能自动执行）
	AnnouncedAt  int64  `json:"announcedAt"`
	State        string `json:"state"`
	Note         string `json:"note"`       // 本工具的操作备注
	SourceNote   string `json:"sourceNote"` // 源站备注
	Supported    bool   `json:"supported"`  // 本工具能否自动执行（当前仅 vod_pic）
}

// PicSample 封面替换样例
type PicSample struct {
	Mid    int64  `json:"mid"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// FetchDomainRules 拉公告页 → 解析（严格 host→host）→ **只写规则表**，不动业务数据。
func (s *FilmService) FetchDomainRules(ctx context.Context, sourceURL string) (DomainFetchResult, error) {
	src, resolved := ResolveDomainSource(sourceURL)
	if resolved == "" {
		return DomainFetchResult{}, fmt.Errorf("未指定公告来源")
	}
	sourceURL = resolved
	res := DomainFetchResult{Source: sourceURL, SourceKey: src.Key, FetchedAt: time.Now().UnixMilli()}
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
	rules, rejects := parseDomainPage(src, string(body))
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
		dr := domainRuleFromParsed(r, sourceURL, now)
		// 插入 + 旧行归档在同一事务里：要么一起生效，要么都不生效。
		created, _, merged, uerr := s.search.InsertDomainRuleWithMerge(ctx, dr)
		if uerr != nil {
			return res, uerr
		}
		if !created {
			// 完全一致 ⇒ 跳过（不写任何字段、不动状态）
			res.Skipped++
			continue
		}
		res.New++
		res.Merged += int(merged)
	}
	invalidateDomainStats()
	return res, nil
}

// 规则统计缓存：全量聚合在规则表变大后会变贵，列表页每 10s 最多算一次；
// 任何写操作（拉取/新增/编辑/改状态/执行）都会立即让它失效。
const domainStatsTTL = 10 * time.Second

var domainStatsCache struct {
	mu     sync.Mutex
	at     time.Time
	counts map[string]int64
	total  int64
	stale  int64
}

func invalidateDomainStats() {
	domainStatsCache.mu.Lock()
	domainStatsCache.at = time.Time{}
	domainStatsCache.counts = nil
	domainStatsCache.mu.Unlock()
}

func (s *FilmService) domainStats(ctx context.Context) (map[string]int64, int64, int64, error) {
	domainStatsCache.mu.Lock()
	if !domainStatsCache.at.IsZero() && time.Since(domainStatsCache.at) < domainStatsTTL {
		c := make(map[string]int64, len(domainStatsCache.counts))
		for k, v := range domainStatsCache.counts {
			c[k] = v
		}
		total, stale := domainStatsCache.total, domainStatsCache.stale
		domainStatsCache.mu.Unlock()
		return c, total, stale, nil
	}
	domainStatsCache.mu.Unlock()

	cutoff := time.Now().Add(-DomainRuleStaleAfter).UnixMilli()
	counts, total, stale, err := s.search.DomainRuleStats(ctx, cutoff)
	if err != nil {
		return nil, 0, 0, err
	}
	domainStatsCache.mu.Lock()
	domainStatsCache.at = time.Now()
	domainStatsCache.counts = counts
	domainStatsCache.total = total
	domainStatsCache.stale = stale
	domainStatsCache.mu.Unlock()
	return counts, total, stale, nil
}

// DomainRuleStaleAfter 待处理规则超过该时长即计入"告警"
const DomainRuleStaleAfter = 7 * 24 * time.Hour

// ListDomainRules 列出规则（state 空=全部），并返回**告警计数**：
//
//	pending —— 待处理规则数（前端红点）
//	stale   —— 其中公告已超过 DomainRuleStaleAfter 仍未处理（强提醒）
//
// 计数在内存里算，不额外查库；"命中行数"属于执行影响面，走 Preview 按需拉取。
func (s *FilmService) ListDomainRules(ctx context.Context, state, keyword string, page, size int) ([]DomainRuleView, int64, map[string]int64, int, error) {
	rows, total, err := s.search.ListDomainRules(ctx, state, keyword, page, size)
	if err != nil {
		return nil, 0, nil, 0, err
	}
	out := make([]DomainRuleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainRuleView(r))
	}
	// 计数走全量聚合（带 10s 缓存），不受分页/筛选影响
	counts, _, stale, err := s.domainStats(ctx)
	if err != nil {
		return nil, 0, nil, 0, err
	}
	return out, total, counts, int(stale), nil
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
				log.Printf("[macrep] 每日拉取完成: 新增 %d / 跳过(一致) %d / 目标变更 %d / 拒收 %d（仅入库，未执行替换）",
					res.New, res.Skipped, res.Merged, res.Rejected)
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
	out.Sample = []PicSample{}
	switch rule.Field {
	case macrep.FieldPic:
		if out.Hits, err = s.search.CountCoverHost(ctx, rule.FromHost); err != nil {
			return out, err
		}
		rows, perr := s.search.PreviewCoverReplace(ctx, rule.FromHost, rule.ToHost, 20)
		if perr != nil {
			return out, perr
		}
		for _, r := range rows {
			out.Sample = append(out.Sample, PicSample{Mid: r.Mid, Before: r.Before, After: r.After})
		}
	case macrep.FieldPlay:
		sites, serr := s.search.PlaySiteIDs(ctx, rule.Scope != "slave", rule.SourceCode)
		if serr != nil {
			return out, serr
		}
		out.Sites = sites
		if out.Hits, err = s.search.CountPlayHost(ctx, sites, rule.FromHost); err != nil {
			return out, err
		}
		rows, perr := s.search.PreviewPlayReplace(ctx, sites, rule.FromHost, rule.ToHost, 20)
		if perr != nil {
			return out, perr
		}
		for _, r := range rows {
			out.Sample = append(out.Sample, PicSample{Mid: r.Mid, Before: r.Before, After: r.After})
		}
	}
	return out, nil
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// domainRuleFromParsed 把解析结果转成仓储实体（补来源与时间戳）
func domainRuleFromParsed(r macrep.Rule, sourceURL string, now int64) repository.DomainRule {
	// 封面只可能来自主站（用户要求）；播放/下载默认也先记主站，管理员可在页面上改成从站。
	// 本库根本不存这个字段（如下载地址）⇒ 入库即了结，不占"待处理"
	state, stateNote := "pending", ""
	if tg := fieldTarget(r.Field); tg.Column == "" && !tg.Executable {
		state, stateNote = "skipped", tg.Note
	}
	return repository.DomainRule{
		Field: r.Field, FieldLabel: clipRunes(r.FieldLabel, 32), RuleType: clipRunes(r.Type, 32),
		Scope: "master", SourceCode: "",
		SourceKey:   clipRunes(macrep.SourceKeyFromURL(sourceURL), 32),
		SourceURL:   clipRunes(sourceURL, 512),
		FromHost:    r.FromHost,
		ToHost:      r.ToHost,
		AnnouncedAt: r.AnnouncedAt,
		SourceNote:  clipRunes(r.Note, 255),
		State:       state,
		Note:        stateNote,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func toDomainRuleView(r repository.DomainRule) DomainRuleView {
	scope := r.Scope
	if scope == "" {
		scope = "master"
	}
	return DomainRuleView{
		ID: r.ID, Field: r.Field, FieldLabel: r.FieldLabel, RuleType: r.RuleType,
		Scope: scope, SourceCode: r.SourceCode,
		SourceKey: r.SourceKey, SourceURL: r.SourceURL,
		FromHost: r.FromHost, ToHost: r.ToHost,
		TargetColumn: fieldTarget(r.Field).Column, TargetNote: fieldTarget(r.Field).Note,
		AnnouncedAt: r.AnnouncedAt, State: r.State,
		Note: r.Note, SourceNote: r.SourceNote,
		Supported: fieldTarget(r.Field).Executable,
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
// domainRuleStates 允许的状态（skipped = 我们库里没有该旧域名，无需替换）。
var domainRuleStates = map[string]bool{
	"pending": true, "applied": true, "merged": true, "rejected": true, "skipped": true,
}

func (s *FilmService) SetDomainRuleState(ctx context.Context, id int64, state, note string) error {
	state = strings.TrimSpace(state)
	if !domainRuleStates[state] {
		return fmt.Errorf("未知状态: %s", state)
	}
	err := s.search.SetDomainRuleState(ctx, id, state, note)
	if err == nil {
		invalidateDomainStats()
	}
	return err
}

// DomainManualRule 管理端**手工录入/修改**一条规则（只写规则表，不动业务数据）。
type DomainManualRule struct {
	Field       string `json:"field"`
	Scope       string `json:"scope"`
	SourceCode  string `json:"sourceCode"`
	FromHost    string `json:"fromHost"`
	ToHost      string `json:"toHost"`
	AnnouncedAt int64  `json:"announcedAt"`
	SourceNote  string `json:"sourceNote"`
}

// fieldTargetInfo 源站字段 → 我们库里的落点。**后端维护**：公告源怎么写字段由这里翻译，
// 前端不做任何配置，只展示。
type fieldTargetInfo struct {
	Column     string // 我们的落点（空 = 本库不存该字段）
	Executable bool   // 本工具能否自动执行
	Note       string
}

var fieldTargetMap = map[string]fieldTargetInfo{
	macrep.FieldPic:  {Column: "movie.cover", Executable: true, Note: "封面：真相源 movie.cover，可自动执行（同步 movie_search.cover）"},
	macrep.FieldPlay: {Column: "movie_play_source.episodes_json", Executable: true, Note: "播放地址：按源写入 episodes_json，可自动替换并重算内容指纹"},
	macrep.FieldDown: {Note: "下载地址：本库不存储，入库即标记跳过"},
}

func fieldTarget(field string) fieldTargetInfo {
	if v, ok := fieldTargetMap[field]; ok {
		return v
	}
	return fieldTargetInfo{Note: "未知字段：需在后端补充落点映射"}
}

// parseDomainPage 解析**跟着公告源走**：不同来源的页面结构/字段口径可能不同，
// 由来源 Type 决定用哪个解析器（新增站点 = 后端加一个 case + 解析器，前端不动）。
func parseDomainPage(src DomainSource, body string) ([]macrep.Rule, []macrep.Reject) {
	switch src.Type {
	case "macrep":
		return macrep.ParsePage(body)
	default:
		log.Printf("[macrep] 来源 %s 的类型 %q 未注册专用解析器，回退通用解析", src.Key, src.Type)
		return macrep.ParsePage(body)
	}
}

// fieldLabel 目标列的中文名（与源站 label 对齐）。
func fieldLabel(field string) string {
	switch field {
	case macrep.FieldPic:
		return "视频封面"
	case macrep.FieldPlay:
		return "视频播放"
	case macrep.FieldDown:
		return "视频下载"
	}
	return field
}

// ruleType 公告分类的中文名（与源站 type 对齐）。
func ruleTypeOf(field string) string {
	switch field {
	case macrep.FieldPic:
		return "视频封面替换"
	case macrep.FieldPlay:
		return "视频播放地址"
	case macrep.FieldDown:
		return "视频下载地址"
	}
	return ""
}

// UpsertManualDomainRule 手工录入规则：与解析器**同一口径**（严格 host → host），
// 且封面只允许主站（用户要求：pic 只有主站），播放/下载可选主站或从站。
func (s *FilmService) UpsertManualDomainRule(ctx context.Context, in DomainManualRule) error {
	field := strings.TrimSpace(in.Field)
	switch field {
	case macrep.FieldPic, macrep.FieldPlay, macrep.FieldDown:
	default:
		return fmt.Errorf("字段只能是 vod_pic / vod_play_url / vod_down_url")
	}
	scope := strings.TrimSpace(in.Scope)
	if scope == "" {
		scope = "master"
	}
	if scope != "master" && scope != "slave" {
		return fmt.Errorf("范围只能是 master(主站) / slave(从站)")
	}
	if field == macrep.FieldPic && scope != "master" {
		return fmt.Errorf("封面(vod_pic)只可能来自主站，不能选从站")
	}
	from, ok := macrep.NormalizeHost(in.FromHost)
	if !ok {
		return fmt.Errorf("旧域名不合法：只填域名本身（不带 http://、路径、端口）")
	}
	to, ok := macrep.NormalizeHost(in.ToHost)
	if !ok {
		return fmt.Errorf("新域名不合法：只填域名本身（不带 http://、路径、端口）")
	}
	if from == to {
		return fmt.Errorf("新旧域名相同，无需替换")
	}
	at := in.AnnouncedAt
	if at <= 0 {
		at = time.Now().UnixMilli()
	}
	if !validAnnouncedAt(at) {
		return fmt.Errorf("公告日期超出合理范围")
	}
	code := strings.TrimSpace(in.SourceCode)
	if field == macrep.FieldPic {
		code = ""
	}
	now := time.Now().UnixMilli()
	// 与拉取同一口径：本库根本不存的字段（如下载地址）入库即了结
	state, note := "pending", ""
	if tg := fieldTarget(field); tg.Column == "" && !tg.Executable {
		state, note = "skipped", tg.Note
	}
	created, _, _, err := s.search.InsertDomainRuleWithMerge(ctx, repository.DomainRule{
		Field: field, FieldLabel: fieldLabel(field), RuleType: ruleTypeOf(field),
		Scope: scope, SourceCode: code,
		SourceKey: "manual", SourceURL: "",
		FromHost: from, ToHost: to, AnnouncedAt: at,
		SourceNote: clipRunes(strings.TrimSpace(in.SourceNote), 255),
		State:      state, Note: note,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return err
	}
	if !created {
		return fmt.Errorf("已存在相同规则（%s %s → %s），如需改目标请编辑那一条", field, from, to)
	}
	invalidateDomainStats()
	return nil
}

// EditDomainRule 手工**编辑**单条规则（原地改，不新增行）。
// 改动 from/to 等同于换了目标 ⇒ 状态回到 pending，需要重新预览/执行。
func (s *FilmService) EditDomainRule(ctx context.Context, id int64, in DomainManualRule) error {
	cur, err := s.search.GetDomainRule(ctx, id)
	if err != nil {
		return err
	}
	field := strings.TrimSpace(in.Field)
	switch field {
	case macrep.FieldPic, macrep.FieldPlay, macrep.FieldDown:
	default:
		return fmt.Errorf("字段只能是 vod_pic / vod_play_url / vod_down_url")
	}
	scope := strings.TrimSpace(in.Scope)
	if scope == "" {
		scope = "master"
	}
	if scope != "master" && scope != "slave" {
		return fmt.Errorf("范围只能是 master(主站) / slave(从站)")
	}
	if field == macrep.FieldPic && scope != "master" {
		return fmt.Errorf("封面(vod_pic)只可能来自主站，不能选从站")
	}
	from, ok := macrep.NormalizeHost(in.FromHost)
	if !ok {
		return fmt.Errorf("旧域名不合法：只填域名本身（不带 http://、路径、端口）")
	}
	to, ok := macrep.NormalizeHost(in.ToHost)
	if !ok {
		return fmt.Errorf("新域名不合法：只填域名本身（不带 http://、路径、端口）")
	}
	if from == to {
		return fmt.Errorf("新旧域名相同，无需替换")
	}
	at := in.AnnouncedAt
	if at <= 0 {
		at = cur.AnnouncedAt
	}
	if !validAnnouncedAt(at) {
		return fmt.Errorf("公告日期超出合理范围")
	}
	code := strings.TrimSpace(in.SourceCode)
	if field == macrep.FieldPic {
		code = ""
	}
	state := cur.State
	if field != cur.Field || from != cur.FromHost || to != cur.ToHost {
		state = "pending"
	}
	err = s.search.UpdateDomainRule(ctx, id, repository.DomainRule{
		Field: field, FieldLabel: fieldLabel(field), RuleType: ruleTypeOf(field),
		Scope: scope, SourceCode: code,
		SourceKey: clipRunes(cur.SourceKey, 32), SourceURL: clipRunes(cur.SourceURL, 512),
		FromHost: from, ToHost: to,
		AnnouncedAt: at, SourceNote: clipRunes(strings.TrimSpace(in.SourceNote), 255), State: state,
	})
	if err == nil {
		invalidateDomainStats()
	}
	return err
}

// validAnnouncedAt 公告日期只接受 2000-01-01 ~ 现在+1 天，挡住明显的手误（如 20261-01-01）。
func validAnnouncedAt(ms int64) bool {
	min := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	max := time.Now().Add(24 * time.Hour).UnixMilli()
	return ms >= min && ms <= max
}

// DomainRuleProgress 替换进度快照（异步任务；前端 1s 轮询）。
type DomainRuleProgress struct {
	RuleID    int64  `json:"ruleId"`
	Field     string `json:"field"`
	State     string `json:"state"` // running / done / failed
	Total     int64  `json:"total"`
	Done      int64  `json:"done"`
	Updated   int64  `json:"updated"`
	Message   string `json:"message"`
	StartedAt int64  `json:"startedAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

var domainProgress = struct {
	mu sync.Mutex
	m  map[int64]*DomainRuleProgress
}{m: map[int64]*DomainRuleProgress{}}

func snapshotDomainProgress(id int64) (DomainRuleProgress, bool) {
	domainProgress.mu.Lock()
	defer domainProgress.mu.Unlock()
	p, ok := domainProgress.m[id]
	if !ok {
		return DomainRuleProgress{}, false
	}
	return *p, true
}

func setDomainProgress(p DomainRuleProgress) {
	p.UpdatedAt = time.Now().UnixMilli()
	domainProgress.mu.Lock()
	domainProgress.m[p.RuleID] = &p
	domainProgress.mu.Unlock()
}

// DomainRuleProgress 查进度（没有记录时 ok=false，前端按"未执行"处理）。
func (s *FilmService) DomainRuleProgress(id int64) (DomainRuleProgress, bool) {
	return snapshotDomainProgress(id)
}

// StartDomainRuleApply 启动**异步**替换（封面 / 播放地址），立即返回，前端轮询进度。
//
// 守卫（照旧从严）：
//  1. 只接受可执行字段（封面 / 播放；下载地址本库不存，直接拒绝并给出原因）；
//  2. 封面只可能来自主站；
//  3. 已 applied 的规则不重复执行；
//  4. confirmFrom/confirmTo 必须与规则逐字一致；
//  5. 同一规则正在执行时拒绝重复触发。
func (s *FilmService) StartDomainRuleApply(ctx context.Context, id int64, confirmFrom, confirmTo string, batch, sleepMs int) error {
	rule, err := s.search.GetDomainRule(ctx, id)
	if err != nil {
		return err
	}
	tg := fieldTarget(rule.Field)
	if !tg.Executable {
		return fmt.Errorf("%s", tg.Note)
	}
	if rule.Field == macrep.FieldPic && rule.Scope != "" && rule.Scope != "master" {
		return fmt.Errorf("封面只可能来自主站，这条规则的范围是 %s，请先改成主站", rule.Scope)
	}
	if rule.State == "applied" {
		return fmt.Errorf("该规则已标记为已执行")
	}
	if strings.TrimSpace(confirmFrom) != rule.FromHost || strings.TrimSpace(confirmTo) != rule.ToHost {
		return fmt.Errorf("二次确认不匹配：期望 %s → %s", rule.FromHost, rule.ToHost)
	}
	if p, ok := snapshotDomainProgress(id); ok && p.State == "running" {
		return fmt.Errorf("该规则正在执行中（%d/%d），请稍候", p.Done, p.Total)
	}

	var sites []string
	var total int64
	switch rule.Field {
	case macrep.FieldPic:
		if total, err = s.search.CountCoverHost(ctx, rule.FromHost); err != nil {
			return err
		}
	case macrep.FieldPlay:
		if sites, err = s.search.PlaySiteIDs(ctx, rule.Scope != "slave", rule.SourceCode); err != nil {
			return err
		}
		if len(sites) == 0 {
			// 找不到源就停：否则会"执行 0 行"后把规则标成已执行，看起来像做过了
			return fmt.Errorf("没有匹配的采集源（范围=%s，来源=%q），请先确认站点配置", rule.Scope, rule.SourceCode)
		}
		if total, err = s.search.CountPlayHost(ctx, sites, rule.FromHost); err != nil {
			return err
		}
	default:
		return fmt.Errorf("暂不支持 %s 的自动执行", rule.Field)
	}

	setDomainProgress(DomainRuleProgress{
		RuleID: id, Field: rule.Field, State: "running", Total: total,
		StartedAt: time.Now().UnixMilli(), Message: "开始执行",
	})
	go s.runDomainRuleApply(*rule, sites, batch, sleepMs)
	return nil
}

// runDomainRuleApply 后台执行（不带请求 ctx：页面关掉也要跑完）。
func (s *FilmService) runDomainRuleApply(rule repository.DomainRule, sites []string, batch, sleepMs int) {
	defer func() {
		if r := recover(); r != nil {
			p, _ := snapshotDomainProgress(rule.ID)
			p.State, p.Message = "failed", fmt.Sprintf("执行异常: %v", r)
			setDomainProgress(p)
			log.Printf("[macrep] 规则 %d 执行 panic: %v", rule.ID, r)
		}
	}()
	bg := context.Background()
	onProgress := func(done, updated int64) {
		p, ok := snapshotDomainProgress(rule.ID)
		if !ok {
			return
		}
		p.Done, p.Updated = done, updated
		setDomainProgress(p)
	}
	var n int64
	var err error
	switch rule.Field {
	case macrep.FieldPlay:
		n, err = s.search.ApplyPlayReplace(bg, rule.ID, sites, rule.FromHost, rule.ToHost, batch, sleepMs, onProgress)
	default:
		n, err = s.search.ApplyCoverReplace(bg, rule.ID, rule.FromHost, rule.ToHost, batch, sleepMs, onProgress)
	}
	p, _ := snapshotDomainProgress(rule.ID)
	if err != nil {
		p.State, p.Message = "failed", err.Error()
		setDomainProgress(p)
		log.Printf("[macrep] 规则 %d 执行失败: %v", rule.ID, err)
		return
	}
	if serr := s.search.SetDomainRuleState(bg, rule.ID, "applied", fmt.Sprintf("已替换 %d 行", n)); serr != nil {
		log.Printf("[macrep] 规则 %d 已替换 %d 行，但状态更新失败: %v", rule.ID, n, serr)
	}
	invalidateDomainStats()
	if m, aerr := s.search.AnnotateSiblingRules(bg, rule.Field, rule.FromHost, rule.ToHost, rule.ID); aerr != nil {
		log.Printf("[macrep] 规则 %d 执行后给同域名规则加提示失败: %v", rule.ID, aerr)
	} else if m > 0 {
		log.Printf("[macrep] 规则 %d 执行后，%d 条同域名规则被标注(目标不同，待复核)", rule.ID, m)
	}
	p.State, p.Updated, p.Message = "done", n, fmt.Sprintf("已替换 %d 行", n)
	p.Done = p.Total
	setDomainProgress(p)
}
