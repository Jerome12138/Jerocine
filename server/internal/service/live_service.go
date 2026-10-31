package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/live"
)

// 直播源频道解析缓存：源站清单低频变更，30min 足够新鲜；空解析（拉不到/解析为空）
// 负缓存 5min，避免源站抖动时每请求都打源站。
const (
	liveChannelsTTL    = 30 * time.Minute
	liveChannelsNegTTL = 5 * time.Minute
	liveChanPatFmt     = "v1:live:ch:%d:*" // 单源频道缓存失效 pattern（SCAN）
)

// LiveSourceBrief 公开直播源（不含 url/remark —— 频道地址属于内部信息）。
type LiveSourceBrief struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// LiveSourcesResp 公开直播源列表。
type LiveSourcesResp struct {
	Sources []LiveSourceBrief `json:"sources"`
}

// LiveChannelView 公开直播频道。
// Alive/CORS 为探源结果（nil = 未探测）：CORS=false → 浏览器跨域拉不到流（前台置灰"跨域受限"）；
// Alive=false → 流不可达（前台置灰"离线"）。两项独立。
type LiveChannelView struct {
	Name  string `json:"name"`
	Group string `json:"group"`
	Logo  string `json:"logo"`
	URL   string `json:"url"`
	Alive *bool  `json:"alive,omitempty"`
	CORS  *bool  `json:"cors,omitempty"`
}

// LiveChannelsResp 直播源频道列表（解析自 m3u，带缓存）。
type LiveChannelsResp struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	Channels []LiveChannelView `json:"channels"`
}

// LiveSourceInput 管理端直播源写入（Create 全字段 / Update 整体覆盖）。
type LiveSourceInput struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Remark string `json:"remark"`
	Sort   int    `json:"sort"`
	State  *int8  `json:"state"` // nil = 默认启用(0)；与站内开关同口径 0 启用 / 1 停用
}

// LiveService 直播源服务（公开频道解析 + 管理端 CRUD + 频道覆盖层/探源）。
type LiveService struct {
	repo    repository.LiveSourceRepository
	state   repository.LiveChannelStateRepository
	client  *live.Client
	probeHC *http.Client // 探源基 client(transport 复用连接池; 每次探源套独立超时)
}

// NewLiveService 构造直播源服务。
func NewLiveService(repo repository.LiveSourceRepository, state repository.LiveChannelStateRepository, client *live.Client) *LiveService {
	return &LiveService{
		repo:    repo,
		state:   state,
		client:  client,
		probeHC: &http.Client{Timeout: liveProbeTimeout},
	}
}

// liveProbeTimeout 探源超时(与 live.ProbeChannel 内部一致, 由 handler 层总超时兜底)。
const liveProbeTimeout = 10 * time.Second

// liveHealthTTL 探源结果缓存: 手动触发式, 7d 足够覆盖一次运维周期。
const liveHealthTTL = 7 * 24 * time.Hour

// liveProbeConcurrency 批量探源并发上限: 150 频道 × 8s 超时 / 24 ≈ 最坏 50s(在 manage 前端 80s 超时内)。
const liveProbeConcurrency = 24

// LiveSources 公开：启用的直播源列表（行数极小，直查不做缓存）。
func (s *LiveService) LiveSources(ctx context.Context) (LiveSourcesResp, error) {
	list, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return LiveSourcesResp{}, err
	}
	out := make([]LiveSourceBrief, 0, len(list))
	for i := range list {
		out = append(out, LiveSourceBrief{ID: list[i].Id, Name: list[i].Name})
	}
	return LiveSourcesResp{Sources: out}, nil
}

// LiveChannels 公开：解析指定直播源的 m3u 出频道列表（缓存 30min/负 5min）。
// 停用/不存在的源一律 404（不泄露存在性）。
func (s *LiveService) LiveChannels(ctx context.Context, id int64) (LiveChannelsResp, error) {
	src, err := s.repo.Get(ctx, id)
	if err != nil {
		return LiveChannelsResp{}, err
	}
	if src.State != entity.LiveSourceEnabled {
		return LiveChannelsResp{}, domain.ErrNotFound
	}
	urlKey := cache.EpKeyOf(src.URL)
	resp, _, err := cache.GetOrLoadTTL(ctx, cache.KeyLiveChannels(src.Id, urlKey), liveChannelsTTL, liveChannelsNegTTL,
		func(ctx context.Context) (LiveChannelsResp, bool, error) {
			text, err := s.client.Fetch(ctx, src.URL)
			if err != nil {
				// 拉取失败也计入负缓存(5min)：死源不回源（否则每请求最长等 20s），
				// TTL 到期自动重试，无需人工干预。日志留痕便于排障
				log.Printf("[live] source %d(%s) 拉取失败(负缓存 5min): %v", src.Id, src.Name, err)
				return LiveChannelsResp{}, false, nil
			}
			chs := live.ParsePlaylist(text)
			out := make([]LiveChannelView, 0, len(chs))
			for i := range chs {
				out = append(out, LiveChannelView{
					Name:  chs[i].Name,
					Group: chs[i].Group,
					Logo:  chs[i].Logo,
					URL:   chs[i].URL,
				})
			}
			return LiveChannelsResp{ID: src.Id, Name: src.Name, Channels: out}, len(out) > 0, nil
		})
	if err != nil {
		return LiveChannelsResp{}, err
	}
	// 负缓存命中回零值结构：channels:null 违背契约, 归一为空数组（id/name 回填源信息）
	if resp.Channels == nil {
		resp.Channels = []LiveChannelView{}
		resp.ID = src.Id
		resp.Name = src.Name
	}
	// 合并覆盖层(隐藏/改名/自定义频道追加)。覆盖层读取失败仅降级(不挡频道列表)。
	// 探源交由浏览器端侧执行——服务器无 IPv6 出口导致 IPv6-only 源全标记不可达，
	// 但客户端浏览器通常有 IPv6 直连能力，服务端探测反而误伤可用频道。
	s.applyChannelOverrides(ctx, src.Id, &resp)
	return resp, nil
}

// applyChannelOverrides 合并频道覆盖层到解析结果：
//   - display_name 覆盖、hidden 过滤（解析频道）；
//   - is_custom=1 的条目作为自定义频道追加到尾部（组缺省"自定义"）。
//
// 覆盖层读取失败 → 原样返回(不挡播放)。
func (s *LiveService) applyChannelOverrides(ctx context.Context, sourceID int64, resp *LiveChannelsResp) {
	if s.state == nil {
		return
	}
	states, err := s.state.ListBySource(ctx, sourceID)
	if err != nil {
		log.Printf("[live] source %d 覆盖层读取失败(降级原样返回): %v", sourceID, err)
		return
	}
	if len(states) == 0 {
		return
	}
	byURL := make(map[string]entity.LiveChannelState, len(states))
	for _, st := range states {
		byURL[st.URL] = st
	}
	out := make([]LiveChannelView, 0, len(resp.Channels)+len(states))
	for _, ch := range resp.Channels {
		st, ok := byURL[ch.URL]
		if ok && st.Hidden == entity.LiveChannelHidden {
			continue // 解析频道被后台隐藏
		}
		if ok && st.DisplayName != "" {
			ch.Name = st.DisplayName
		}
		out = append(out, ch)
	}
	// 自定义频道追加(追加序 = state.id 升序); 与解析条目同 URL 的自定义行已在上面按覆盖处理
	for _, st := range states {
		if st.IsCustom != entity.LiveChannelCustom || st.Hidden == entity.LiveChannelHidden {
			continue
		}
		dup := false
		for _, ch := range out {
			if ch.URL == st.URL {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		group := st.GroupName
		if group == "" {
			group = "自定义"
		}
		out = append(out, LiveChannelView{Name: st.Name, Group: group, URL: st.URL})
	}
	resp.Channels = out
}

// attachHealth 批量读探源健康(MGET 一次), 有记录的频道补 alive/cors 标记。
func (s *LiveService) attachHealth(ctx context.Context, sourceID int64, chs []LiveChannelView) {
	if len(chs) == 0 {
		return
	}
	srcKey := cache.EpKeyOf(fmt.Sprintf("%d", sourceID))
	urlKeys := make([]string, len(chs))
	for i, ch := range chs {
		urlKeys[i] = cache.EpKeyOf(ch.URL)
	}
	raws, err := cache.LiveHealthGetAll(ctx, srcKey, urlKeys)
	if err != nil {
		log.Printf("[live] source %d 健康读取失败(降级无标记): %v", sourceID, err)
		return
	}
	for i, str := range raws {
		if str == "" {
			continue
		}
		var h struct {
			Alive bool `json:"alive"`
			CORS  bool `json:"cors"`
		}
		if json.Unmarshal([]byte(str), &h) != nil {
			continue
		}
		a, c := h.Alive, h.CORS
		chs[i].Alive = &a
		chs[i].CORS = &c
	}
}

// ListLiveSources 管理端：全量直播源（含停用）。
func (s *LiveService) ListLiveSources(ctx context.Context) ([]entity.LiveSource, error) {
	return s.repo.ListAll(ctx)
}

// CreateLiveSource 管理端：新增直播源。
func (s *LiveService) CreateLiveSource(ctx context.Context, in LiveSourceInput) (*entity.LiveSource, error) {
	e, err := in.validate()
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// UpdateLiveSource 管理端：整体覆盖直播源（地址变更 → 频道缓存键自然更换）。
// state 必填：整体覆盖语义下，缺省会导致停用中的源被静默重新启用。
func (s *LiveService) UpdateLiveSource(ctx context.Context, id int64, in LiveSourceInput) error {
	if in.State == nil {
		return fmt.Errorf("缺少 state 字段: %w", domain.ErrInvalidArgument)
	}
	e, err := in.validate()
	if err != nil {
		return err
	}
	e.Id = id
	if err := s.repo.Update(ctx, e); err != nil {
		return err
	}
	// URL 变更走新键即可，但名称/停用等也会影响公开语义 → 该源频道缓存全清，立即生效
	cache.DelByPattern(context.WithoutCancel(ctx), fmt.Sprintf(liveChanPatFmt, id))
	return nil
}

// DeleteLiveSource 管理端：删除直播源并清其频道缓存。
func (s *LiveService) DeleteLiveSource(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.state != nil {
		_ = s.state.DeleteBySource(ctx, id) // 覆盖层级联清理(失败不影响源删除)
	}
	cache.DelByPattern(context.WithoutCancel(ctx), fmt.Sprintf(liveChanPatFmt, id))
	return nil
}

// ---- 频道覆盖层（管理端） ----

// LiveChannelManageView 后台频道视图（解析频道 + 覆盖层合并, 含隐藏项）。
type LiveChannelManageView struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	URL         string `json:"url"`
	Hidden      bool   `json:"hidden"`
	IsCustom    bool   `json:"isCustom"`
	DisplayName string `json:"displayName"`
	Alive       *bool  `json:"alive,omitempty"`
	CORS        *bool  `json:"cors,omitempty"`
}

// LiveChannelsManageResp 后台频道列表。
type LiveChannelsManageResp struct {
	ID       int64                   `json:"id"`
	Name     string                  `json:"name"`
	Channels []LiveChannelManageView `json:"channels"`
}

// ListChannelsManage 后台：解析频道 + 覆盖层合并（含隐藏项, 供后台开关/改名）。
func (s *LiveService) ListChannelsManage(ctx context.Context, id int64) (LiveChannelsManageResp, error) {
	src, err := s.repo.Get(ctx, id)
	if err != nil {
		return LiveChannelsManageResp{}, err
	}
	states := map[string]entity.LiveChannelState{}
	if s.state != nil {
		list, err := s.state.ListBySource(ctx, id)
		if err == nil {
			for _, st := range list {
				states[st.URL] = st
			}
		}
	}
	raw := s.parseChannels(ctx, src)
	out := make([]LiveChannelManageView, 0, len(raw)+len(states))
	for _, ch := range raw {
		st, has := states[ch.URL]
		v := LiveChannelManageView{Name: ch.Name, Group: ch.Group, URL: ch.URL}
		if has {
			v.Hidden = st.Hidden == entity.LiveChannelHidden
			v.IsCustom = st.IsCustom == entity.LiveChannelCustom
			v.DisplayName = st.DisplayName
			if st.Name != "" {
				v.Name = st.Name
			}
			if st.GroupName != "" {
				v.Group = st.GroupName
			}
		}
		out = append(out, v)
	}
	// 自定义频道(解析集里没有的)追加
	for url, st := range states {
		if st.IsCustom != entity.LiveChannelCustom {
			continue
		}
		dup := false
		for _, v := range out {
			if v.URL == url {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		group := st.GroupName
		if group == "" {
			group = "自定义"
		}
		out = append(out, LiveChannelManageView{
			Name: st.Name, Group: group, URL: url,
			Hidden: st.Hidden == entity.LiveChannelHidden, IsCustom: true,
		})
	}
	resp := LiveChannelsManageResp{ID: src.Id, Name: src.Name, Channels: out}
	s.attachHealthManage(ctx, id, resp.Channels)
	return resp, nil
}

// attachHealthManage 后台视图健康标记(复用公开 MGET 链路)。
func (s *LiveService) attachHealthManage(ctx context.Context, sourceID int64, chs []LiveChannelManageView) {
	if len(chs) == 0 {
		return
	}
	srcKey := cache.EpKeyOf(fmt.Sprintf("%d", sourceID))
	urlKeys := make([]string, len(chs))
	for i, ch := range chs {
		urlKeys[i] = cache.EpKeyOf(ch.URL)
	}
	raws, err := cache.LiveHealthGetAll(ctx, srcKey, urlKeys)
	if err != nil {
		return
	}
	for i, str := range raws {
		if str == "" {
			continue
		}
		var h struct {
			Alive bool `json:"alive"`
			CORS  bool `json:"cors"`
		}
		if json.Unmarshal([]byte(str), &h) != nil {
			continue
		}
		a, c := h.Alive, h.CORS
		chs[i].Alive = &a
		chs[i].CORS = &c
	}
}

// parseChannels 拉取并解析源 m3u（不走公开缓存, 供后台全集视图）。
func (s *LiveService) parseChannels(ctx context.Context, src *entity.LiveSource) []live.Channel {
	text, err := s.client.Fetch(ctx, src.URL)
	if err != nil {
		return nil
	}
	return live.ParsePlaylist(text)
}

// LiveChannelInput 频道覆盖层写入（hidden/display_name 用于解析频道; name/group_name 用于自定义频道）。
type LiveChannelInput struct {
	SourceID    int64  `json:"sourceId"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	GroupName   string `json:"groupName"`
	Hidden      *bool  `json:"hidden"`
	DisplayName string `json:"displayName"`
}

// UpsertChannelState 后台：写频道覆盖层（解析频道隐藏/改名; 自定义频道增改）。
func (s *LiveService) UpsertChannelState(ctx context.Context, in LiveChannelInput) error {
	url := strings.TrimSpace(in.URL)
	if in.SourceID <= 0 || url == "" {
		return fmt.Errorf("sourceId/url 必填: %w", domain.ErrInvalidArgument)
	}
	if len(url) > 1024 {
		return fmt.Errorf("频道地址过长: %w", domain.ErrInvalidArgument)
	}
	if _, err := s.repo.Get(ctx, in.SourceID); err != nil {
		return err
	}
	hidden := int8(entity.LiveChannelVisible)
	if in.Hidden != nil && *in.Hidden {
		hidden = entity.LiveChannelHidden
	}
	// 已有条目判断 is_custom 语义(解析频道上不覆盖 name/group_name 语义)
	existing, err := s.state.ListBySource(ctx, in.SourceID)
	if err != nil {
		return err
	}
	isCustom := int8(entity.LiveChannelParsed)
	for _, st := range existing {
		if st.URL == url && st.IsCustom == entity.LiveChannelCustom {
			isCustom = entity.LiveChannelCustom
		}
	}
	st := &entity.LiveChannelState{
		SourceId:    in.SourceID,
		URL:         url,
		Name:        strings.TrimSpace(in.Name),
		GroupName:   strings.TrimSpace(in.GroupName),
		Hidden:      hidden,
		DisplayName: strings.TrimSpace(in.DisplayName),
		IsCustom:    isCustom,
	}
	if st.Name == "" && isCustom == entity.LiveChannelCustom {
		return fmt.Errorf("自定义频道名称必填: %w", domain.ErrInvalidArgument)
	}
	if err := s.state.Upsert(ctx, st); err != nil {
		return err
	}
	s.invalidateChannelCache(ctx, in.SourceID)
	return nil
}

// DeleteChannel 后台：删除频道 —— 自定义条目物理删; 解析条目置 hidden（源重解析后覆盖层仍生效）。
func (s *LiveService) DeleteChannel(ctx context.Context, sourceID int64, url string) error {
	if sourceID <= 0 || strings.TrimSpace(url) == "" {
		return fmt.Errorf("sourceId/url 必填: %w", domain.ErrInvalidArgument)
	}
	isCustom := false
	existing, err := s.state.ListBySource(ctx, sourceID)
	if err == nil {
		for _, st := range existing {
			if st.URL == url && st.IsCustom == entity.LiveChannelCustom {
				isCustom = true
			}
		}
	}
	if isCustom {
		if err := s.state.Delete(ctx, sourceID, url); err != nil {
			return err
		}
	} else {
		if err := s.state.Upsert(ctx, &entity.LiveChannelState{
			SourceId: sourceID, URL: url, Hidden: entity.LiveChannelHidden,
		}); err != nil {
			return err
		}
	}
	s.invalidateChannelCache(ctx, sourceID)
	return nil
}

// invalidateChannelCache 清该源公开频道缓存(覆盖层变更立即生效)。
func (s *LiveService) invalidateChannelCache(ctx context.Context, sourceID int64) {
	cache.DelByPattern(context.WithoutCancel(ctx), fmt.Sprintf(liveChanPatFmt, sourceID))
}

// ---- 探源（管理端） ----

// ProbeResultView 单频道探源结果（管理端返回体）。
type ProbeResultView struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	Alive bool   `json:"alive"`
	CORS  bool   `json:"cors"`
}

// ProbeChannels 批量探源：并发探该源全部(可见)频道, 结果写 Redis(7d) 供公开列表/后台读取。
// origin 用于模拟浏览器跨域请求(响应头有无 ACAO 即 CORS 判据)。
func (s *LiveService) ProbeChannels(ctx context.Context, sourceID int64, origin string) ([]ProbeResultView, error) {
	if _, err := s.repo.Get(ctx, sourceID); err != nil {
		return nil, err
	}
	pub, err := s.LiveChannels(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if origin == "" {
		origin = "https://jerocine.art"
	}
	chs := pub.Channels
	results := make([]ProbeResultView, len(chs))
	sem := make(chan struct{}, liveProbeConcurrency)
	var wg sync.WaitGroup
	for i := range chs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := live.ProbeChannel(ctx, s.probeHC, chs[idx].URL, origin)
			results[idx] = ProbeResultView{URL: chs[idx].URL, Name: chs[idx].Name, Alive: r.Alive, CORS: r.CORS}
		}(i)
	}
	wg.Wait()
	srcKey := cache.EpKeyOf(fmt.Sprintf("%d", sourceID))
	for _, r := range results {
		b, _ := json.Marshal(map[string]bool{"alive": r.Alive, "cors": r.CORS})
		if err := cache.LiveHealthSet(ctx, srcKey, cache.EpKeyOf(r.URL), string(b), liveHealthTTL); err != nil {
			log.Printf("[live] source %d 健康写入失败: %v", sourceID, err)
			break
		}
	}
	return results, nil
}

// validate 输入校验：名称/地址必填，地址仅允许 http(s)（拉取客户端不支持其它协议）。
func (in LiveSourceInput) validate() (*entity.LiveSource, error) {
	name := strings.TrimSpace(in.Name)
	url := strings.TrimSpace(in.URL)
	if name == "" {
		return nil, fmt.Errorf("直播源名称不能为空: %w", domain.ErrInvalidArgument)
	}
	if len(name) > 64 {
		return nil, fmt.Errorf("直播源名称过长: %w", domain.ErrInvalidArgument)
	}
	lu := strings.ToLower(url)
	if !strings.HasPrefix(lu, "http://") && !strings.HasPrefix(lu, "https://") {
		return nil, fmt.Errorf("直播源地址仅支持 http(s): %w", domain.ErrInvalidArgument)
	}
	if len(url) > 1024 {
		return nil, fmt.Errorf("直播源地址过长: %w", domain.ErrInvalidArgument)
	}
	remark := strings.TrimSpace(in.Remark)
	if len([]rune(remark)) > 255 {
		return nil, fmt.Errorf("备注过长(≤255 字): %w", domain.ErrInvalidArgument)
	}
	state := entity.LiveSourceEnabled
	if in.State != nil {
		if *in.State != entity.LiveSourceEnabled && *in.State != entity.LiveSourceDisabled {
			return nil, fmt.Errorf("非法状态值 %d: %w", *in.State, domain.ErrInvalidArgument)
		}
		state = *in.State
	}
	return &entity.LiveSource{
		Name:   name,
		URL:    url,
		Remark: remark,
		Sort:   in.Sort,
		State:  state,
	}, nil
}
