// Package service 编排业务(原 logic 单例搬入), 依赖 repository 端口 + cache 旁路。
package service

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"server/internal/cache"
	"server/internal/config"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// 缓存 TTL(写入时 cache 包自动叠 ±10% 抖动)。
const (
	ttlHome     = 10 * time.Minute
	ttlClassify = 10 * time.Minute
	ttlDetail   = 24 * time.Hour
	ttlRelate   = 30 * time.Minute
	ttlTagOpts  = time.Hour
	ttlCatTree  = time.Hour
)

const relateCandidatePool = 200

// FilmService 影片读路径编排。
type FilmService struct {
	search   repository.SearchRepository
	movie    repository.MovieRepository
	play     repository.PlaySourceRepository
	category repository.CategoryRepository
	health   repository.SourceHealthRepository    // 读: 播放线路按延时排序(可空)
	sources  repository.CollectSourceRepository    // 读: 播放源 tab 名用采集源维护的 name(可空)
	pages    config.PageSizes
}

func NewFilmService(s repository.SearchRepository, m repository.MovieRepository, p repository.PlaySourceRepository, c repository.CategoryRepository, health repository.SourceHealthRepository, sources repository.CollectSourceRepository, pages config.PageSizes) *FilmService {
	return &FilmService{search: s, movie: m, play: p, category: c, health: health, sources: sources, pages: pages}
}

// sourceNameMap 采集源 id → 维护的 name. 仓储缺省/出错 → 空表(退化为用 PlayFrom)。
func (s *FilmService) sourceNameMap(ctx context.Context) map[string]string {
	if s.sources == nil {
		return nil
	}
	list, err := s.sources.List(ctx, false)
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(list))
	for _, cs := range list {
		if cs.Name != "" {
			m[cs.Id] = cs.Name
		}
	}
	return m
}

// CategoryTree 返回完整分类树(缓存)。
func (s *FilmService) CategoryTree(ctx context.Context) ([]*entity.CategoryNode, error) {
	tree, _, err := cache.GetOrLoad(ctx, cache.KeyCatTree, ttlCatTree, func(ctx context.Context) ([]*entity.CategoryNode, bool, error) {
		cats, err := s.category.All(ctx)
		if err != nil {
			return nil, false, err
		}
		return buildTree(cats), true, nil
	})
	return tree, err
}

// NavCategories 返回导航用的一级可展示分类。
func (s *FilmService) NavCategories(ctx context.Context) ([]*entity.CategoryNode, error) {
	tree, err := s.CategoryTree(ctx)
	if err != nil {
		return nil, err
	}
	var nav []*entity.CategoryNode
	for _, n := range tree {
		if n.Show {
			nav = append(nav, n)
		}
	}
	return nav, nil
}

// PidCategory 按一级分类 pid 取分类节点(分类页标题用)。
func (s *FilmService) PidCategory(ctx context.Context, pid int64) (*entity.CategoryNode, error) {
	tree, err := s.CategoryTree(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range tree {
		if n.Id == pid {
			return n, nil
		}
	}
	return nil, nil
}

// Home 首页聚合(缓存)。
func (s *FilmService) Home(ctx context.Context) (HomeData, error) {
	data, _, err := cache.GetOrLoad(ctx, cache.KeyHomeAgg, ttlHome, func(ctx context.Context) (HomeData, bool, error) {
		nav, err := s.NavCategories(ctx)
		if err != nil {
			return HomeData{}, false, err
		}
		topN := s.pages.Home
		pool := topN * 4 // 多取候选, 优先挑有封面的(避免首页各分类卡出现无图灰块)
		hd := HomeData{Categories: nav}
		for _, n := range nav {
			latest, err := s.search.TopByPidSorted(ctx, n.Id, repository.SortRecent, pool)
			if err != nil {
				return HomeData{}, false, err
			}
			hot, err := s.search.TopByPidSorted(ctx, n.Id, repository.SortHot, pool)
			if err != nil {
				return HomeData{}, false, err
			}
			hd.Rows = append(hd.Rows, HomeRow{Nav: n.Category, Latest: coverFirst(latest, topN), Hot: coverFirst(hot, topN)})
		}
		return hd, true, nil
	})
	return data, err
}

// coverFirst 把有封面的影片排前(无封面的排后补足), 取前 n 条 —— 让首页各分类 top 尽量带封面图。
func coverFirst(items []entity.MovieSearch, n int) []entity.MovieSearch {
	withCover := make([]entity.MovieSearch, 0, len(items))
	without := make([]entity.MovieSearch, 0, len(items))
	for _, it := range items {
		if it.Cover != "" {
			withCover = append(withCover, it)
		} else {
			without = append(without, it)
		}
	}
	out := append(withCover, without...)
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// Classify 分类页三榜(缓存)。
func (s *FilmService) Classify(ctx context.Context, pid int64) (ClassifyData, error) {
	data, _, err := cache.GetOrLoad(ctx, cache.KeyClassify(pid), ttlClassify, func(ctx context.Context) (ClassifyData, bool, error) {
		n := s.pages.Classify
		news, err := s.search.TopByPidSorted(ctx, pid, repository.SortLatest, n)
		if err != nil {
			return ClassifyData{}, false, err
		}
		top, err := s.search.TopByPidSorted(ctx, pid, repository.SortHot, n)
		if err != nil {
			return ClassifyData{}, false, err
		}
		recent, err := s.search.TopByPidSorted(ctx, pid, repository.SortRecent, n)
		if err != nil {
			return ClassifyData{}, false, err
		}
		return ClassifyData{News: news, Top: top, Recent: recent}, true, nil
	})
	return data, err
}

// Detail 影片详情 + 多源播放(缓存)。found=false 表示影片不存在。
func (s *FilmService) Detail(ctx context.Context, mid int64) (FilmDetailData, bool, error) {
	return cache.GetOrLoad(ctx, cache.KeyMovieDetail(mid), ttlDetail, func(ctx context.Context) (FilmDetailData, bool, error) {
		m, err := s.movie.GetByMid(ctx, mid)
		if err == domain.ErrMovieNotFound {
			return FilmDetailData{}, false, nil
		}
		if err != nil {
			return FilmDetailData{}, false, err
		}
		sources, err := s.assembleSources(ctx, m)
		if err != nil {
			return FilmDetailData{}, false, err
		}
		return FilmDetailData{Movie: *m, Sources: sources}, true, nil
	})
}

// Related 相关推荐(缓存 + 内存均匀抽样, 去 ORDER BY RAND)。found=false 表示影片不存在。
func (s *FilmService) Related(ctx context.Context, mid int64) ([]entity.MovieSearch, error) {
	cards, _, err := cache.GetOrLoad(ctx, cache.KeyRelate(mid), ttlRelate, func(ctx context.Context) ([]entity.MovieSearch, bool, error) {
		seedRow, err := s.search.GetByMid(ctx, mid)
		if err == domain.ErrNotFound {
			return []entity.MovieSearch{}, true, nil // 影片无检索行: 空推荐(仍缓存, 避免反复重算)
		}
		if err != nil {
			return nil, false, err
		}
		seed := repository.RelatedSeed{
			Mid: seedRow.Mid, Cid: seedRow.Cid, Name: relateName(seedRow.Name),
			ClassTag: seedRow.ClassTag, Area: seedRow.Area, Language: seedRow.Language,
		}
		cand, err := s.search.Related(ctx, seed, relateCandidatePool)
		if err != nil {
			return nil, false, err
		}
		return pickSpread(cand, s.pages.Home), true, nil
	})
	return cards, err
}

// Filter 多维筛选分页(不缓存: 组合维度多)。
func (s *FilmService) Filter(ctx context.Context, spec repository.FilterSpec, page repository.Page) (CardPage, error) {
	page = page.Normalize(s.pages.Filter)
	list, total, err := s.search.Filter(ctx, spec, page)
	if err != nil {
		return CardPage{}, err
	}
	return CardPage{List: list, Total: total, Page: page}, nil
}

// Search 关键字检索分页(FULLTEXT)。
func (s *FilmService) Search(ctx context.Context, keyword string, page repository.Page) (CardPage, error) {
	page = page.Normalize(s.pages.Search)
	list, total, err := s.search.SearchKeyword(ctx, keyword, page)
	if err != nil {
		return CardPage{}, err
	}
	return CardPage{List: list, Total: total, Page: page}, nil
}

// TagOptions 7 维筛选标签(缓存)。
func (s *FilmService) TagOptions(ctx context.Context, pid int64) (*repository.FilterOptions, error) {
	opts, _, err := cache.GetOrLoad(ctx, cache.KeyTagOpts(pid), ttlTagOpts, func(ctx context.Context) (*repository.FilterOptions, bool, error) {
		o, err := s.search.TagOptions(ctx, pid)
		if err != nil {
			return nil, false, err
		}
		return o, true, nil
	})
	return opts, err
}

// playLine 带源标识的播放线路(供按播放延时排序)。
type playLine struct {
	View   PlaySourceView
	SiteId string
}

// assembleSources 组合多源播放: 主站(按 mid) + 各附属站(按 match_key 命中), 按 (siteId, playFrom) 去重,
// 再按各源实测播放延时升序排(默认线路最快; 与主站解耦)。
func (s *FilmService) assembleSources(ctx context.Context, m *entity.Movie) ([]PlaySourceView, error) {
	rows, err := s.play.ListByMid(ctx, m.Mid)
	if err != nil {
		return nil, err
	}
	keys := []string{domain.GenerateHashKey(m.Name)}
	if m.DbId > 0 {
		keys = append(keys, domain.GenerateHashKey(m.DbId))
	}
	matched, err := s.play.GetByMatchKeys(ctx, "", keys)
	if err != nil {
		return nil, err
	}
	nameMap := s.sourceNameMap(ctx)
	seen := make(map[string]struct{})
	var lines []playLine
	add := func(ps entity.MoviePlaySource) {
		dedup := ps.SiteId + "\x00" + ps.PlayFrom
		if _, ok := seen[dedup]; ok {
			return
		}
		seen[dedup] = struct{}{}
		// tab 名优先用采集源维护的 name(按 siteId), 缺省回退到 play_from 原始标识
		name := ps.PlayFrom
		if nm := nameMap[ps.SiteId]; nm != "" {
			name = nm
		}
		lines = append(lines, playLine{
			View:   PlaySourceView{Id: ps.SiteId + ":" + ps.PlayFrom, Name: name, Episodes: ps.Episodes},
			SiteId: ps.SiteId,
		})
	}
	for _, r := range rows {
		add(r)
	}
	for _, r := range matched {
		add(r)
	}
	sortPlayLines(lines, s.playLatencyMap(ctx))
	out := make([]PlaySourceView, len(lines))
	for i := range lines {
		out[i] = lines[i].View
	}
	return out, nil
}

// playLatencyMap 各源抽样播放延时(0/未测的源不入表 → 排末尾)。健康仓储缺省/出错 → 空表(退化为原序)。
func (s *FilmService) playLatencyMap(ctx context.Context) map[string]int64 {
	if s.health == nil {
		return nil
	}
	hs, err := s.health.List(ctx)
	if err != nil {
		return nil
	}
	m := make(map[string]int64, len(hs))
	for _, h := range hs {
		if h.PlayLatencyMs > 0 {
			m[h.SourceId] = h.PlayLatencyMs
		}
	}
	return m
}

// sortPlayLines 按播放延时升序稳定排序(未知/0 用 MaxInt 排末尾, 保留主站在前的原序)。纯函数, 便于单测。
func sortPlayLines(lines []playLine, lat map[string]int64) {
	sort.SliceStable(lines, func(i, j int) bool {
		return effPlayLat(lat[lines[i].SiteId]) < effPlayLat(lat[lines[j].SiteId])
	})
}

func effPlayLat(v int64) int64 {
	if v <= 0 {
		return math.MaxInt64
	}
	return v
}

// buildTree 把扁平分类行装配成树(输入按 sort,id 有序 → 输出保持稳定顺序)。
func buildTree(cats []entity.Category) []*entity.CategoryNode {
	byId := make(map[int64]*entity.CategoryNode, len(cats))
	for i := range cats {
		c := cats[i]
		byId[c.Id] = &entity.CategoryNode{Category: c}
	}
	var roots []*entity.CategoryNode
	for i := range cats {
		n := byId[cats[i].Id]
		if cats[i].Pid == 0 {
			roots = append(roots, n)
		} else if p := byId[cats[i].Pid]; p != nil {
			p.Children = append(p.Children, n)
		}
	}
	return roots
}

// pickSpread 在候选里均匀间隔取 n 个(候选顺序无意义, 间隔取样避免聚集)。
func pickSpread(list []entity.MovieSearch, n int) []entity.MovieSearch {
	if n <= 0 {
		n = 14
	}
	if len(list) <= n {
		return list
	}
	step := len(list) / n
	if step < 1 {
		step = 1
	}
	out := make([]entity.MovieSearch, 0, n)
	for i := 0; i < len(list) && len(out) < n; i += step {
		out = append(out, list[i])
	}
	return out
}

// relateName 截取片名主干作为相似匹配条件(去季/数字/剧场版等)。
func relateName(name string) string {
	if i := strings.IndexAny(name, " 第"); i > 0 {
		return name[:i]
	}
	return name
}
