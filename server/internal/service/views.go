package service

import (
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// 服务层读模型(可缓存/可被 handler 直接映射为 DTO)。

// HomeRow 首页一个分类区块。
type HomeRow struct {
	Nav    entity.Category      `json:"nav"`
	Latest []entity.MovieSearch `json:"latest"`
	Hot    []entity.MovieSearch `json:"hot"`
}

// HomeData 首页聚合(后端完成 merge/排序, 前端不再补偿)。
type HomeData struct {
	Categories []*entity.CategoryNode `json:"categories"`
	// Hot 全站**跨类别**热榜(首页「🔥 热门榜单」行, 不按 pid)。这是全站唯一的跨类别榜单 ——
	// Rows[].Hot 是"该分类的热榜", 两者口径不同, 不可互相替代(见方案 §8.1①)。
	Hot  []entity.MovieSearch `json:"hot"`
	Rows []HomeRow            `json:"rows"`
}

// ClassifyData 分类页各榜。
type ClassifyData struct {
	News   []entity.MovieSearch `json:"news"`
	Top    []entity.MovieSearch `json:"top"`
	Recent []entity.MovieSearch `json:"recent"`
	// Score 高分榜(db_score > 0 且排除解说)。ScoredCount 为该分类有评分的影片总数:
	// 为 0 时 Score 必为空, 前端据 ScoredCount 决定要不要渲染这个分区。
	Score       []entity.MovieSearch `json:"score"`
	ScoredCount int64                `json:"scoredCount"`
}

// PlaySourceView 一个播放源(对齐 OpenAPI PlaySource: id/name/episodes)。
// AdFilterOk 服务端 m3u8 可达性(nil=未测): false 时播放页跳过服务端代理过滤链路直接走直链。
type PlaySourceView struct {
	Id         string           `json:"id"`
	Name       string           `json:"name"`
	Episodes   []entity.Episode `json:"episodes"`
	AdFilterOk *bool            `json:"adFilterOk,omitempty"`
}

// FilmDetailData 影片详情(详情主体 + 多源播放)。
type FilmDetailData struct {
	Movie   entity.Movie     `json:"movie"`
	Sources []PlaySourceView `json:"sources"`
}

// CardPage 卡片分页。
type CardPage struct {
	List  []entity.MovieSearch `json:"list"`
	Total int64                `json:"total"`
	Page  repository.Page      `json:"page"`
}
