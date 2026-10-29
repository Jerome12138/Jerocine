package service

import (
	"context"
	"fmt"
	"log"
	"strings"
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
type LiveChannelView struct {
	Name  string `json:"name"`
	Group string `json:"group"`
	Logo  string `json:"logo"`
	URL   string `json:"url"`
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

// LiveService 直播源服务（公开频道解析 + 管理端 CRUD）。
type LiveService struct {
	repo   repository.LiveSourceRepository
	client *live.Client
}

// NewLiveService 构造直播源服务。
func NewLiveService(repo repository.LiveSourceRepository, client *live.Client) *LiveService {
	return &LiveService{repo: repo, client: client}
}

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
	return resp, nil
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
	cache.DelByPattern(context.WithoutCancel(ctx), fmt.Sprintf(liveChanPatFmt, id))
	return nil
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
