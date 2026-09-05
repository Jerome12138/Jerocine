package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/dto"
	"server/internal/middleware"
)

// Login POST /auth/login
func (h *Handlers) Login(c *gin.Context) {
	var req struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Account == "" || req.Password == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "account/password required")
		return
	}
	res, err := h.User.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, res)
}

// Logout POST /auth/logout
func (h *Handlers) Logout(c *gin.Context) {
	h.User.Logout(c.Request.Context(), c.GetUint(middleware.CtxUserID), c.GetString(middleware.CtxToken))
	dto.NoContent(c)
}

// DeviceCode POST /auth/device/code (公开) — 生成扫码登录设备码。
func (h *Handlers) DeviceCode(c *gin.Context) {
	dc, uc, exp, err := h.User.DeviceCode(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"deviceCode": dc, "userCode": uc, "expiresIn": exp, "interval": 3})
}

// DeviceConfirm POST /auth/device/confirm (需登录) — 手机端确认授权 userCode。
func (h *Handlers) DeviceConfirm(c *gin.Context) {
	var req struct {
		UserCode string `json:"userCode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserCode == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "userCode required")
		return
	}
	if err := h.User.DeviceConfirm(c.Request.Context(), req.UserCode, c.GetUint(middleware.CtxUserID)); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"ok": true})
}

// DevicePoll POST /auth/device/poll (公开) — 待登录端轮询, authorized 时回 token。
func (h *Handlers) DevicePoll(c *gin.Context) {
	var req struct {
		DeviceCode string `json:"deviceCode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.DeviceCode == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "deviceCode required")
		return
	}
	res, status, err := h.User.DevicePoll(c.Request.Context(), req.DeviceCode)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if status != "ok" {
		dto.OK(c, gin.H{"status": status})
		return
	}
	dto.OK(c, gin.H{
		"status": "ok", "userName": res.UserName, "token": res.Token,
		"expires": res.Expires, "role": res.Role,
	})
}

// UserInfo GET /me
func (h *Handlers) UserInfo(c *gin.Context) {
	u, err := h.User.Me(c.Request.Context(), c.GetUint(middleware.CtxUserID))
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": u.ID, "userName": u.UserName, "role": u.Role})
}

// ChangePassword PATCH /me/password
func (h *Handlers) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.NewPassword == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "password required")
		return
	}
	if err := h.User.ChangePassword(c.Request.Context(), c.GetUint(middleware.CtxUserID), req.OldPassword, req.NewPassword); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

// HistoryUpsert POST /me/histories
func (h *Handlers) HistoryUpsert(c *gin.Context) {
	var req struct {
		Mid      int64  `json:"mid"`
		PlayFrom string `json:"playFrom"`
		Episode  int    `json:"episode"`
		Progress int    `json:"progress"`
		Duration int    `json:"duration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Mid == 0 {
		dto.Error(c, http.StatusUnprocessableEntity, "mid required")
		return
	}
	hh := &entity.UserHistory{
		UserId: currentUserID(c), Mid: req.Mid, PlayFrom: req.PlayFrom,
		Episode: req.Episode, Progress: req.Progress, Duration: req.Duration,
	}
	if err := h.User.HistoryUpsert(c.Request.Context(), hh); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, hh)
}

// HistoryList GET /me/histories
func (h *Handlers) HistoryList(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.User.HistoryList(c.Request.Context(), currentUserID(c), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Page(c, list, page.Normalize(20).Current, page.Normalize(20).Size, total)
}

// HistoryDelete DELETE /me/histories?mid=
func (h *Handlers) HistoryDelete(c *gin.Context) {
	mid := queryInt64(c, "mid")
	if mid == 0 {
		dto.Error(c, http.StatusBadRequest, "mid required")
		return
	}
	if err := h.User.HistoryDelete(c.Request.Context(), currentUserID(c), mid); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

// HistoryClear DELETE /me/histories/clear
func (h *Handlers) HistoryClear(c *gin.Context) {
	if err := h.User.HistoryClear(c.Request.Context(), currentUserID(c)); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

// FavoriteAdd POST /me/favorites
func (h *Handlers) FavoriteAdd(c *gin.Context) {
	var req struct {
		Mid int64 `json:"mid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Mid == 0 {
		dto.Error(c, http.StatusUnprocessableEntity, "mid required")
		return
	}
	if err := h.User.FavoriteAdd(c.Request.Context(), currentUserID(c), req.Mid); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Created(c, gin.H{"mid": req.Mid})
}

// FavoriteRemove DELETE /me/favorites?mid=
func (h *Handlers) FavoriteRemove(c *gin.Context) {
	mid := queryInt64(c, "mid")
	if mid == 0 {
		dto.Error(c, http.StatusBadRequest, "mid required")
		return
	}
	if err := h.User.FavoriteRemove(c.Request.Context(), currentUserID(c), mid); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

// FavoriteList GET /me/favorites
func (h *Handlers) FavoriteList(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.User.FavoriteList(c.Request.Context(), currentUserID(c), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	np := page.Normalize(20)
	dto.Page(c, list, np.Current, np.Size, total)
}

// FavoriteCheck GET /me/favorites/:mid
func (h *Handlers) FavoriteCheck(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	fav, err := h.User.FavoriteCheck(c.Request.Context(), currentUserID(c), mid)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"favorited": fav})
}

// SkipList GET /me/skip-settings  (账号级片头/片尾跳过设置, 全量)
func (h *Handlers) SkipList(c *gin.Context) {
	list, err := h.User.SkipList(c.Request.Context(), currentUserID(c))
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, list)
}

// SkipSave PUT /me/skip-settings/:mid  {intro, outro}
func (h *Handlers) SkipSave(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	var req struct {
		Intro int `json:"intro"`
		Outro int `json:"outro"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.User.SkipSave(c.Request.Context(), currentUserID(c), mid, req.Intro, req.Outro); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"mid": mid, "intro": req.Intro, "outro": req.Outro})
}

// SkipReset DELETE /me/skip-settings/:mid
func (h *Handlers) SkipReset(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	if err := h.User.SkipReset(c.Request.Context(), currentUserID(c), mid); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}
