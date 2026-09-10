package handler

import (
	"net/http"

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

// ListBanners GET /manage/banners 后台全量列表。
func (h *Handlers) ListBanners(c *gin.Context) {
	list, err := h.Banner.ManageList(c.Request.Context())
	respond(c, list, err)
}

// UpsertBanner POST /manage/banners 新建或按 id 更新(整体覆盖)。
func (h *Handlers) UpsertBanner(c *gin.Context) {
	var b entity.Banner
	if err := c.ShouldBindJSON(&b); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	// 一张没有任何图的轮播没有意义(前台会渲染成纯色块), 直接挡在入口。
	if b.Image == "" && b.Poster == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "banner 至少需要横图或竖图")
		return
	}
	if b.Link == "" && b.Mid <= 0 {
		dto.Error(c, http.StatusUnprocessableEntity, "banner 需要配置跳转(关联影片或自定义链接)")
		return
	}
	if b.StartAt > 0 && b.EndAt > 0 && b.EndAt < b.StartAt {
		dto.Error(c, http.StatusUnprocessableEntity, "结束时间不能早于开始时间")
		return
	}
	if err := h.Banner.Save(c.Request.Context(), &b); err != nil {
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
