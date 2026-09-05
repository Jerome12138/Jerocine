package dto

import (
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/service"
)

// Card 影片卡片(裁剪字段, 跨国线路减重)。
type Card struct {
	Mid      int64   `json:"mid"`
	Name     string  `json:"name"`
	Cover    string  `json:"cover"`
	Cid      int64   `json:"cid"`
	Pid      int64   `json:"pid"`
	CName    string  `json:"cName"`
	SubTitle string  `json:"subTitle"`
	Area     string  `json:"area"`
	Year     int     `json:"year"`
	State    string  `json:"state"`
	Remarks  string  `json:"remarks"`
	DbScore  float64 `json:"dbScore"`
}

func ToCard(m entity.MovieSearch) Card {
	return Card{
		Mid: m.Mid, Name: m.Name, Cover: m.Cover, Cid: m.Cid, Pid: m.Pid, CName: m.CName,
		SubTitle: m.SubTitle, Area: m.Area, Year: m.Year, State: m.State, Remarks: m.Remarks, DbScore: m.DbScore,
	}
}

func ToCards(list []entity.MovieSearch) []Card {
	out := make([]Card, 0, len(list))
	for _, m := range list {
		out = append(out, ToCard(m))
	}
	return out
}

// Episode 单集。
type Episode struct {
	Episode string `json:"episode"`
	Link    string `json:"link"`
}

// PlaySource 一个播放源(契约: id/name/episodes)。
type PlaySource struct {
	Id       string    `json:"id"`
	Name     string    `json:"name"`
	Episodes []Episode `json:"episodes"`
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
		out = append(out, PlaySource{Id: s.Id, Name: s.Name, Episodes: toEpisodes(s.Episodes)})
	}
	return out
}

// FilmDetail 影片详情(契约: 含 sources)。
type FilmDetail struct {
	Mid      int64        `json:"mid"`
	Name     string       `json:"name"`
	Cover    string       `json:"cover"`
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
	Content  string       `json:"content"`
	PlayFrom []string     `json:"playFrom"`
	Sources  []PlaySource `json:"sources"`
}

func ToFilmDetail(d service.FilmDetailData) FilmDetail {
	m := d.Movie
	return FilmDetail{
		Mid: m.Mid, Name: m.Name, Cover: m.Cover, Cid: m.Cid, Pid: m.Pid, CName: m.CName,
		SubTitle: m.SubTitle, Actor: m.Actor, Director: m.Director, Area: m.Area, Language: m.Language,
		Year: m.Year, ClassTag: m.ClassTag, Remarks: m.Remarks, State: m.State, DbScore: m.DbScore,
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
	Rows       []HomeRow     `json:"rows"`
}

func ToHome(h service.HomeData) HomeResp {
	r := HomeResp{Categories: ToNavList(h.Categories)}
	for _, row := range h.Rows {
		r.Rows = append(r.Rows, HomeRow{
			Nav:    NavCategory{Id: row.Nav.Id, Pid: row.Nav.Pid, Name: row.Nav.Name},
			Latest: ToCards(row.Latest), Hot: ToCards(row.Hot),
		})
	}
	return r
}

// ClassifyResp 分类页三榜(含标题分类)。
type ClassifyResp struct {
	Title  *NavCategory `json:"title"`
	News   []Card       `json:"news"`
	Top    []Card       `json:"top"`
	Recent []Card       `json:"recent"`
}

func ToClassify(c service.ClassifyData, title *entity.CategoryNode) ClassifyResp {
	r := ClassifyResp{News: ToCards(c.News), Top: ToCards(c.Top), Recent: ToCards(c.Recent)}
	if title != nil {
		nc := toNav(title)
		r.Title = &nc
	}
	return r
}

// Filters /categories/{pid}/filters 响应(直接复用 repository.FilterOptions 的 json 形状)。
type Filters = repository.FilterOptions
