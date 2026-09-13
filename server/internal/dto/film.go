package dto

import (
	"server/internal/douban"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/service"
	"server/internal/tmdb"
)

// normBackdrop 对外输出前归一化: 检索无果哨兵不外泄, 统一为空串。
func normBackdrop(b string) string {
	if b == tmdb.MissMark {
		return ""
	}
	return b
}

// Card 影片卡片(裁剪字段, 跨国线路减重)。
type Card struct {
	Mid     int64   `json:"mid"`
	Name    string  `json:"name"`
	Cover   string  `json:"cover"`
	// Poster 横图(16:9)。数据列是 movie_search.backdrop; JSON 字段名沿用前端 TS 契约的
	// poster(前端早已按"有则用、无则回退 cover"预留了该字段), 首页兜底轮播零改动点亮。
	Poster  string  `json:"poster,omitempty"`
	Cid     int64   `json:"cid"`
	Pid     int64   `json:"pid"`
	CName   string  `json:"cName"`
	SubTitle string  `json:"subTitle"`
	Area     string  `json:"area"`
	Year     int     `json:"year"`
	State    string  `json:"state"`
	Remarks  string  `json:"remarks"`
	DbScore  float64 `json:"dbScore"`
	// PubDate 上映日期(ISO 前缀串: 2026-09-11 / 2026-07 / 2007), 空 = 源站未提供。
	// 源站覆盖率低且随机, 故 omitempty —— 没值就不占 payload。
	PubDate string `json:"pubDate,omitempty"`
	// HotRank 当前豆瓣榜位(1 起), 0 = 不在榜。前端据此显示 "Hot No.N" 角标;
	// omitempty 让 15 万部榜外影片的卡片不带这个字段。
	HotRank int `json:"hotRank,omitempty"`
}

func ToCard(m entity.MovieSearch) Card {
	return Card{
		Mid: m.Mid, Name: m.Name, Cover: m.Cover, Poster: normBackdrop(m.Backdrop), Cid: m.Cid,
		Pid: m.Pid, CName: m.CName,
		SubTitle: m.SubTitle, Area: m.Area, Year: m.Year, State: m.State, Remarks: m.Remarks, DbScore: m.DbScore,
		PubDate: m.PubDate, HotRank: m.HotRank,
	}
}

func ToCards(list []entity.MovieSearch) []Card {
	out := make([]Card, 0, len(list))
	for _, m := range list {
		out = append(out, ToCard(m))
	}
	return out
}

// ManageFilmRow 后台影片列表行: 公开 Card + 软删标记。
// 单独定义而不给 Card 加字段, 是为了不把 deletedAt 泄漏到公开接口契约里。
type ManageFilmRow struct {
	Card
	DeletedAt int64 `json:"deletedAt"`
}

func ToManageFilmRow(m entity.MovieSearch) ManageFilmRow {
	return ManageFilmRow{Card: ToCard(m), DeletedAt: m.DeletedAt}
}

// ToManageFilmRows 后台影片列表批量转换。
func ToManageFilmRows(list []entity.MovieSearch) []ManageFilmRow {
	out := make([]ManageFilmRow, 0, len(list))
	for _, m := range list {
		out = append(out, ToManageFilmRow(m))
	}
	return out
}

// Episode 单集。
type Episode struct {
	Episode string `json:"episode"`
	Link    string `json:"link"`
}

// PlaySource 一个播放源(契约: id/name/episodes; adFilterOk 服务端 m3u8 可达性, false 时播放页跳过代理过滤)。
type PlaySource struct {
	Id         string    `json:"id"`
	Name       string    `json:"name"`
	Episodes   []Episode `json:"episodes"`
	AdFilterOk *bool     `json:"adFilterOk,omitempty"`
}

func toEpisodes(eps []entity.Episode) []Episode {
	out := make([]Episode, 0, len(eps))
	for _, e := range eps {
		out = append(out, Episode{Episode: e.Episode, Link: e.Link})
	}
	return out
}

func toSources(srcs []service.PlaySourceView) []PlaySource {
	out := make([]PlaySource, 0, len(srcs))
	for _, s := range srcs {
		out = append(out, PlaySource{Id: s.Id, Name: s.Name, Episodes: toEpisodes(s.Episodes), AdFilterOk: s.AdFilterOk})
	}
	return out
}

// FilmDetail 影片详情(契约: 含 sources)。
type FilmDetail struct {
	Mid      int64        `json:"mid"`
	Name     string       `json:"name"`
	Cover    string       `json:"cover"`
	Backdrop string       `json:"backdrop,omitempty"` // 横图(16:9), 详情页 hero 背景用
	Cid      int64        `json:"cid"`
	Pid      int64        `json:"pid"`
	CName    string       `json:"cName"`
	SubTitle string       `json:"subTitle"`
	Actor    string       `json:"actor"`
	Director string       `json:"director"`
	Area     string       `json:"area"`
	Language string       `json:"language"`
	Year     int          `json:"year"`
	ClassTag string       `json:"classTag"`
	Remarks  string       `json:"remarks"`
	State    string       `json:"state"`
	DbScore  float64      `json:"dbScore"`
	// PubDate 上映日期(ISO 前缀串, 空 = 源站未提供); HotRank 豆瓣榜位(0 = 不在榜),
	// 详情页据此显示「上映日期」与「豆瓣热门 No.N」。
	// HotBoard 榜位来源榜单中文名(如 "热门电影"), 展示成「豆瓣·热门电影 No.2」;
	// 同一部片会在多个集合出现(多分类各有 No.2), 光看位次分不清是哪个榜的。
	PubDate  string       `json:"pubDate,omitempty"`
	HotRank  int          `json:"hotRank,omitempty"`
	HotBoard string       `json:"hotBoard,omitempty"`
	Content  string       `json:"content"`
	PlayFrom []string     `json:"playFrom"`
	Sources  []PlaySource `json:"sources"`
}

func ToFilmDetail(d service.FilmDetailData) FilmDetail {
	m := d.Movie
	return FilmDetail{
		Mid: m.Mid, Name: m.Name, Cover: m.Cover, Backdrop: normBackdrop(m.Backdrop), Cid: m.Cid, Pid: m.Pid, CName: m.CName,
		SubTitle: m.SubTitle, Actor: m.Actor, Director: m.Director, Area: m.Area, Language: m.Language,
		Year: m.Year, ClassTag: m.ClassTag, Remarks: m.Remarks, State: m.State, DbScore: m.DbScore,
		PubDate: m.PubDate, HotRank: m.HotRank, HotBoard: douban.BoardLabel(m.HotBoard),
		Content: m.Content, PlayFrom: []string(m.PlayFrom), Sources: toSources(d.Sources),
	}
}

// FilmDetailResp /films/{mid} 响应。
type FilmDetailResp struct {
	Detail  FilmDetail `json:"detail"`
	Related []Card     `json:"related"`
}

// PlayInfoResp /films/{mid}/play 响应(契约: currentSource 为 string sourceId)。
type PlayInfoResp struct {
	Detail         FilmDetail `json:"detail"`
	Current        Episode    `json:"current"`
	CurrentSource  string     `json:"currentSource"`
	CurrentEpisode int        `json:"currentEpisode"`
	Related        []Card     `json:"related"`
}

// HomeRow 首页区块。
type HomeRow struct {
	Nav    NavCategory `json:"nav"`
	Latest []Card      `json:"latest"`
	Hot    []Card      `json:"hot"`
}

// NavCategory 导航分类(带子分类)。
type NavCategory struct {
	Id       int64         `json:"id"`
	Pid      int64         `json:"pid"`
	Name     string        `json:"name"`
	Children []NavCategory `json:"children,omitempty"`
}

func toNav(n *entity.CategoryNode) NavCategory {
	nc := NavCategory{Id: n.Id, Pid: n.Pid, Name: n.Name}
	for _, ch := range n.Children {
		nc.Children = append(nc.Children, toNav(ch))
	}
	return nc
}

func ToNavList(nodes []*entity.CategoryNode) []NavCategory {
	out := make([]NavCategory, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, toNav(n))
	}
	return out
}

// HomeResp 首页。
type HomeResp struct {
	Categories []NavCategory `json:"categories"`
	// Hot 全站跨类别热榜(「🔥 热门榜单」行) —— 与 rows[].hot(该分类热榜)口径不同。
	Hot  []Card    `json:"hot"`
	Rows []HomeRow `json:"rows"`
}

func ToHome(h service.HomeData) HomeResp {
	r := HomeResp{Categories: ToNavList(h.Categories), Hot: ToCards(h.Hot)}
	for _, row := range h.Rows {
		r.Rows = append(r.Rows, HomeRow{
			Nav:    NavCategory{Id: row.Nav.Id, Pid: row.Nav.Pid, Name: row.Nav.Name},
			Latest: ToCards(row.Latest), Hot: ToCards(row.Hot),
		})
	}
	return r
}

// ClassifyResp 分类页各榜(含标题分类)。
// ScoredCount 为该分类有评分的影片总数; 为 0 时 score 必为空, 前端不渲染高分榜分区。
type ClassifyResp struct {
	Title       *NavCategory `json:"title"`
	News        []Card       `json:"news"`
	Top         []Card       `json:"top"`
	Recent      []Card       `json:"recent"`
	Score       []Card       `json:"score"`
	ScoredCount int64        `json:"scoredCount"`
}

func ToClassify(c service.ClassifyData, title *entity.CategoryNode) ClassifyResp {
	r := ClassifyResp{
		News: ToCards(c.News), Top: ToCards(c.Top), Recent: ToCards(c.Recent),
		Score: ToCards(c.Score), ScoredCount: c.ScoredCount,
	}
	if title != nil {
		nc := toNav(title)
		r.Title = &nc
	}
	return r
}

// Filters /categories/{pid}/filters 响应(直接复用 repository.FilterOptions 的 json 形状)。
type Filters = repository.FilterOptions
