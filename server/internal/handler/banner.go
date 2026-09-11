package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/domain/entity"
	"server/internal/dto"
)

// ---- 首页轮播 Banner ----

// HomeBanners GET /banners 前台轮播(仅启用 + 生效窗口内)。
func (h *Handlers) HomeBanners(c *gin.Context) {
	list, err := h.Banner.Public(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.WriteCached(c, list, 120)
}

// BoardBanners GET /manage/banners/effective 后台管理视图 —— 生效位(前 5, 与首页同源,
// 自动补位带出已回填的 TMDB 横图) + 未生效配置行(带原因)。
func (h *Handlers) BoardBanners(c *gin.Context) {
	board, err := h.Banner.Board(c.Request.Context())
	respond(c, board, err)
}

// moveReq POST /manage/banners/move 请求体。slot 为生效列表下标(0 起)。
type moveReq struct {
	Slot int    `json:"slot"`
	Dir  string `json:"dir"` // up | down
}

// MoveBanner POST /manage/banners/move 生效位排序: 手动位换位 / 自动位上移转手动。
// 成功后直接返回新的 Board, 前端免二次请求。
func (h *Handlers) MoveBanner(c *gin.Context) {
	var req moveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Banner.Move(c.Request.Context(), req.Slot, req.Dir); err != nil {
		dto.Fail(c, err)
		return
	}
	board, err := h.Banner.Board(c.Request.Context())
	respond(c, board, err)
}

// bannerReq 后台轮播写请求。
//
// 独立于 entity.Banner 而不是直接绑定实体: 直接绑实体等于把 createdAt/updatedAt 这类
// 审计字段也开放给客户端(mass assignment), 而这正是 updated_at 被写成 0 的成因 ——
// 前端 Banner 类型根本没有 updatedAt 字段, 绑出来恒为 0。
type bannerReq struct {
	Id       int64  `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Image    string `json:"image"`
	Poster   string `json:"poster"`
	Mid      int64  `json:"mid"`
	Link     string `json:"link"`
	Sort     int    `json:"sort"`
	State    int8   `json:"state"`
	StartAt  int64  `json:"startAt"`
	EndAt    int64  `json:"endAt"`
	// Slot 新建时的钉入位置(生效列表下标); nil = 追加到手动位末尾。仅 Create 路径生效。
	Slot *int `json:"slot"`
}

// toEntity 只拷贝白名单字段 —— 审计字段(createdAt/updatedAt)由数据库负责(见
// entity.Banner 上的 autoCreateTime/autoUpdateTime), 请求体里带了也不会被采用。
func (r bannerReq) toEntity() entity.Banner {
	return entity.Banner{
		Id:       r.Id,
		Title:    strings.TrimSpace(r.Title),
		Subtitle: strings.TrimSpace(r.Subtitle),
		Image:    strings.TrimSpace(r.Image),
		Poster:   strings.TrimSpace(r.Poster),
		Mid:      r.Mid,
		Link:     strings.TrimSpace(r.Link),
		Sort:     r.Sort,
		State:    r.State,
		StartAt:  r.StartAt,
		EndAt:    r.EndAt,
	}
}

// validBannerLink 轮播跳转链接白名单: 站内路径(以 / 开头) 或 http(s) 外链。
//
// 前端对 http(s) 外链走 window.open、其余按站内路径 router.push, 后端把住入口是为了
// 不让 javascript:/data: 这类伪协议进库 —— 前端一旦漏判就是 XSS。
func validBannerLink(link string) bool {
	switch {
	case link == "":
		return true
	case strings.HasPrefix(link, "//"):
		return false // 协议相对地址("//evil.com")实际会跳出站外, 不认
	case strings.HasPrefix(link, "/"):
		return true // 站内路径
	default:
		u, err := url.Parse(link)
		return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
	}
}

// UpsertBanner POST /manage/banners 新建; PUT /manage/banners/:id 按路径 id 更新(整体覆盖)。
func (h *Handlers) UpsertBanner(c *gin.Context) {
	var req bannerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	b := req.toEntity()

	// PUT 路径上的 :id 是唯一权威, body 里的 id 被忽略。
	// 否则 PUT /manage/banners/5 配一个不带 id 的 body 会静默走到 Create —— 新建一条,
	// 而调用方以为改的是 #5。
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			dto.Error(c, http.StatusBadRequest, "invalid id")
			return
		}
		b.Id = id
	}

	// 启用中的轮播一张图都没有没有意义(前台会渲染成纯色块), 直接挡在入口。
	// 停用行放宽: "禁用自动位"落地为一条只有 mid 的屏蔽行, 无图是合法形态。
	if b.State == entity.BannerEnabled && b.Image == "" && b.Poster == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "banner 至少需要横图或竖图")
		return
	}
	if b.Link == "" && b.Mid <= 0 {
		dto.Error(c, http.StatusUnprocessableEntity, "banner 需要配置跳转(关联影片或自定义链接)")
		return
	}
	if b.Link != "" && !validBannerLink(b.Link) {
		dto.Error(c, http.StatusUnprocessableEntity, "link 只支持站内路径(以 / 开头)或 http(s) 外链")
		return
	}
	if b.State != entity.BannerEnabled && b.State != entity.BannerDisabled {
		dto.Error(c, http.StatusUnprocessableEntity, "state 只能是 0(启用)或 1(停用)")
		return
	}
	if b.StartAt > 0 && b.EndAt > 0 && b.EndAt < b.StartAt {
		dto.Error(c, http.StatusUnprocessableEntity, "结束时间不能早于开始时间")
		return
	}
	if err := h.Banner.Save(c.Request.Context(), &b, req.Slot); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, b)
}

// DeleteBanner DELETE /manage/banners/:id
func (h *Handlers) DeleteBanner(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Banner.Delete(c.Request.Context(), id); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}
