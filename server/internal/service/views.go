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
	Rows       []HomeRow              `json:"rows"`
}

// ClassifyData 分类页三榜。
type ClassifyData struct {
	News   []entity.MovieSearch `json:"news"`
	Top    []entity.MovieSearch `json:"top"`
	Recent []entity.MovieSearch `json:"recent"`
}

// PlaySourceView 一个播放源(对齐 OpenAPI PlaySource: id/name/episodes)。
type PlaySourceView struct {
	Id       string           `json:"id"`
	Name     string           `json:"name"`
	Episodes []entity.Episode `json:"episodes"`
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
