package handler

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/dto"
	"server/internal/service"
)

// ---- 仪表盘 / 站点配置 ----

func (h *Handlers) Dashboard(c *gin.Context) {
	d := h.Manage.Dashboard(c.Request.Context())
	d.PendingFails = h.Spider.PendingFailureCount(c.Request.Context())
	dto.OK(c, d)
}

// siteConfigResp 站点配置响应: 基础字段 + TMDB key 状态(只出掩码, 明文永不离开服务端)。
type siteConfigResp struct {
	entity.SiteConfig
	TmdbKeyMasked string `json:"tmdbKeyMasked"` // 已配置时的掩码, 如 0c06…676b; 未配置为空串
	TmdbKeySet    bool   `json:"tmdbKeySet"`
}

// maskKey 凭据掩码: 前 4 + … + 后 4; 过短(理论不可达)整串打码。
func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 12 {
		return "••••"
	}
	return key[:4] + "…" + key[len(key)-4:]
}

func (h *Handlers) GetSiteConfig(c *gin.Context) {
	cfg, err := h.Manage.GetSite(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, siteConfigResp{
		SiteConfig:    *cfg,
		TmdbKeyMasked: maskKey(cfg.TmdbAPIKey),
		TmdbKeySet:    cfg.TmdbAPIKey != "",
	})
}

func (h *Handlers) SaveSiteConfig(c *gin.Context) {
	var cfg entity.SiteConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.SaveSite(c.Request.Context(), &cfg); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, cfg)
}

// tmdbKeyReq POST /manage/tmdb-key 请求体。
type tmdbKeyReq struct {
	Key string `json:"key"`
}

// SetTMDBKey POST /manage/tmdb-key: 保存(验真)+热生效。
func (h *Handlers) SetTMDBKey(c *gin.Context) {
	var req tmdbKeyReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Key) == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.SetTMDBKey(c.Request.Context(), req.Key); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"saved": true})
}

// ClearTMDBKey DELETE /manage/tmdb-key: 清除凭据, 横图回填随下一轮停摆。
func (h *Handlers) ClearTMDBKey(c *gin.Context) {
	if err := h.Manage.ClearTMDBKey(c.Request.Context()); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"cleared": true})
}

// ---- 采集源 ----

func (h *Handlers) ListSources(c *gin.Context) {
	list, err := h.Manage.ListSources(c.Request.Context())
	respond(c, list, err)
}

func (h *Handlers) GetSource(c *gin.Context) {
	src, err := h.Manage.GetSource(c.Request.Context(), c.Param("id"))
	respond(c, src, err)
}

func (h *Handlers) UpsertSource(c *gin.Context) {
	var s entity.CollectSource
	if err := c.ShouldBindJSON(&s); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.UpsertSource(c.Request.Context(), &s); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, s)
}

func (h *Handlers) DeleteSource(c *gin.Context) {
	if err := h.Manage.DeleteSource(c.Request.Context(), c.Param("id")); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

func (h *Handlers) TestSource(c *gin.Context) {
	res, err := h.Manage.TestSource(c.Request.Context(), c.Param("id"))
	respond(c, res, err)
}

// SourceSampleM3u8 GET /manage/collect-sources/:id/sample-m3u8 端侧播放测速的样本输入。
func (h *Handlers) SourceSampleM3u8(c *gin.Context) {
	sample, err := h.Manage.SampleM3u8For(c.Request.Context(), c.Param("id"))
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"sampleM3u8": sample})
}

// RecordPlayLatency POST /manage/collect-sources/:id/play-latency 浏览器端播放测速结果回传落库。
func (h *Handlers) RecordPlayLatency(c *gin.Context) {
	var body struct {
		Ms int64 `json:"ms"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Ms < 0 || body.Ms > 600_000 {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.RecordPlayLatency(c.Request.Context(), c.Param("id"), body.Ms); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

func (h *Handlers) TestAllSources(c *gin.Context) {
	res, err := h.Manage.TestAllSources(c.Request.Context())
	respond(c, res, err)
}

func (h *Handlers) ListSourceHealth(c *gin.Context) {
	res, err := h.Manage.ListHealth(c.Request.Context())
	respond(c, res, err)
}

// ---- cron 任务 ----

func (h *Handlers) ListCrons(c *gin.Context) {
	list, err := h.Manage.ListCrons(c.Request.Context())
	respond(c, list, err)
}

func (h *Handlers) UpsertCron(c *gin.Context) {
	var t entity.CronTask
	if err := c.ShouldBindJSON(&t); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.UpsertCron(c.Request.Context(), &t); err != nil {
		dto.Fail(c, err)
		return
	}
	h.Spider.ReloadCron(c.Request.Context()) // 即时生效
	dto.OK(c, t)
}

func (h *Handlers) DeleteCron(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Manage.DeleteCron(c.Request.Context(), id); err != nil {
		dto.Fail(c, err)
		return
	}
	h.Spider.ReloadCron(c.Request.Context())
	dto.NoContent(c)
}

// ---- 分类 ----

func (h *Handlers) ListCategories(c *gin.Context) {
	list, err := h.Manage.ListCategories(c.Request.Context())
	respond(c, list, err)
}

func (h *Handlers) UpsertCategory(c *gin.Context) {
	var cat entity.Category
	if err := c.ShouldBindJSON(&cat); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.UpsertCategory(c.Request.Context(), &cat); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, cat)
}

func (h *Handlers) DeleteCategory(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Manage.DeleteCategory(c.Request.Context(), id); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

// ---- 文件 / 版本(含上传) ----

func (h *Handlers) ListFiles(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.Manage.ListFiles(c.Request.Context(), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	np := page.Normalize(39)
	dto.Page(c, list, np.Current, np.Size, total)
}

func (h *Handlers) DeleteFile(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Manage.DeleteFile(c.Request.Context(), id); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

func (h *Handlers) UploadFile(c *gin.Context) {
	data, name, ok := readUpload(c)
	if !ok {
		return
	}
	f, err := h.Manage.UploadImage(c.Request.Context(), name, data, queryInt64(c, "relevanceId"))
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Created(c, gin.H{"id": f.Id, "link": f.Link, "objectKey": f.ObjectKey})
}

func (h *Handlers) ListVersions(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.Manage.ListVersions(c.Request.Context(), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	np := page.Normalize(20)
	dto.Page(c, list, np.Current, np.Size, total)
}

func (h *Handlers) CreateVersion(c *gin.Context) {
	var v entity.AppVersion
	if err := c.ShouldBindJSON(&v); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.CreateVersion(c.Request.Context(), &v); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Created(c, v)
}

func (h *Handlers) DeleteVersion(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Manage.DeleteVersion(c.Request.Context(), id); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}

func (h *Handlers) UploadApk(c *gin.Context) {
	data, name, ok := readUpload(c)
	if !ok {
		return
	}
	url, err := h.Manage.UploadApk(c.Request.Context(), name, data)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Created(c, gin.H{"url": url})
}

// ---- 榜单热度 ----

// HotRefresh POST /manage/hot/refresh 手动跑一轮豆瓣榜单刷新。
//
// 与每日 04:00 的自动刷新共用一把锁: 撞车时返 409(而不是排队, 免得后台连点把豆瓣打爆)。
// 响应体是这一轮的观测值(fetched/matched/fellOff/dbIdFilled/scoreFilled/applied/...), 抓取不可信时
// 返回 502 且**不落库** —— 保留上一轮快照。
func (h *Handlers) HotRefresh(c *gin.Context) {
	if h.Hot == nil {
		dto.Error(c, http.StatusServiceUnavailable, "hot service unavailable")
		return
	}
	rep, err := h.Hot.Refresh(c.Request.Context())
	if err != nil {
		if errors.Is(err, service.ErrHotBusy) {
			dto.Error(c, http.StatusConflict, err.Error())
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"message": err.Error(), "report": rep})
		return
	}
	dto.OK(c, rep)
}

// ---- 用户 ----

func (h *Handlers) CreateUser(c *gin.Context) {
	var req struct {
		UserName string `json:"userName"`
		Password string `json:"password"`
		Role     int    `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserName == "" || req.Password == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "userName/password required")
		return
	}
	u, err := h.Manage.CreateUser(c.Request.Context(), req.UserName, req.Password, req.Role)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Created(c, gin.H{"id": u.ID, "userName": u.UserName, "role": u.Role})
}

func (h *Handlers) ListUsers(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.Manage.ListUsers(c.Request.Context(), c.Query("keyword"), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	np := page.Normalize(20)
	dto.Page(c, list, np.Current, np.Size, total) // entity.User.Password json:"-" 已脱敏
}

// ---- 影片管理 ----

// ManageFilms GET /manage/films 后台影片搜索(分页 {list,page})。
// status 取 active(默认, 仅在架) | deleted(回收站) | all(全部)。
func (h *Handlers) ManageFilms(c *gin.Context) {
	// cid 兼容前端 cid / 旧 category 两种入参
	cid := queryInt64(c, "cid")
	if cid == 0 {
		cid = queryInt64(c, "category")
	}
	spec := repository.FilterSpec{
		Keyword: c.Query("keyword"), Pid: queryInt64(c, "pid"), Cid: cid,
		Deleted: deletedMode(c.Query("status")),
	}
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	res, err := h.Manage.SearchFilms(c.Request.Context(), spec, page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Page(c, dto.ToManageFilmRows(res.List), res.Page.Current, res.Page.Size, res.Total)
}

// deletedMode 把后台的 status 查询串映射成软删态过滤维度。
func deletedMode(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "deleted", "trash":
		return repository.DeletedOnly
	case "all", "any":
		return repository.DeletedInclude
	default:
		return repository.DeletedExclude
	}
}

// DeleteFilm DELETE /manage/films/:mid 软删影片(可在回收站恢复)。
func (h *Handlers) DeleteFilm(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	respond(c, nil, h.Manage.SoftDeleteFilm(c.Request.Context(), mid))
}

// RestoreFilm POST /manage/films/:mid/restore 恢复被软删的影片。
func (h *Handlers) RestoreFilm(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	respond(c, nil, h.Manage.RestoreFilm(c.Request.Context(), mid))
}

// ManageFilmDetail GET /manage/films/:mid/detail 后台影片详情(影片 + 全部源与集, 实时读库)。
func (h *Handlers) ManageFilmDetail(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	d, err := h.Manage.FilmDetail(c.Request.Context(), mid)
	respond(c, d, err)
}

// SpiderSearch POST /manage/spider/search {keyword, sources[]} 按片名搜索各源影片信息(只读, 不落库)。
func (h *Handlers) SpiderSearch(c *gin.Context) {
	var req struct {
		Keyword string   `json:"keyword"`
		Sources []string `json:"sources"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Keyword == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "keyword required")
		return
	}
	res, err := h.Spider.SearchSources(c.Request.Context(), req.Keyword, req.Sources)
	respond(c, res, err)
}

// CollectFilm POST /manage/spider/collect-film {sources[]|sourceId, keyword|mid, vodId} 智能采集。
// 传 mid: 按"已有影片匹配逻辑"用 NormalizeName(片名) 作搜索关键字(影片详情"按源采集该片"用);
// 传 keyword: 直接用该关键字(新增页"按名采集"用)。
// 智能采集分流:
//   - 传 sourceId(单源): 调 CollectOneSource(sourceId, keyword, vodId), 返回单个 SourceFilmResult。
//     vodId>0 时精确采该片(前端从候选选定后回调); vodId==0 时智能匹配(精确唯一即采, 多义返 candidates 待前端选)。
//   - 否则(sources[]/全部): 走多源 CollectFilm(各源独立智能匹配)。
func (h *Handlers) CollectFilm(c *gin.Context) {
	var req struct {
		Keyword  string   `json:"keyword"`
		Mid      int64    `json:"mid"`
		Sources  []string `json:"sources"`
		SourceId string   `json:"sourceId"`
		VodId    int64    `json:"vodId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	keyword := req.Keyword
	if req.Mid > 0 {
		m, err := h.Manage.GetFilm(c.Request.Context(), req.Mid)
		if err != nil {
			dto.Fail(c, err)
			return
		}
		keyword = domain.NormalizeName(m.Name)
	}
	// 单源: 智能采集(支持 vodId 精确采)。vodId>0 时无需 keyword(前端已选定具体片)。
	if req.SourceId != "" {
		if keyword == "" && req.VodId <= 0 {
			dto.Error(c, http.StatusUnprocessableEntity, "keyword or mid or vodId required")
			return
		}
		res, err := h.Spider.CollectOneSource(c.Request.Context(), req.SourceId, keyword, req.VodId)
		respond(c, res, err)
		return
	}
	// 多源/全部: 按片名智能采集。
	if keyword == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "keyword or mid required")
		return
	}
	res, err := h.Spider.CollectFilm(c.Request.Context(), keyword, req.Sources)
	respond(c, res, err)
}

func (h *Handlers) ManageGetFilm(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	m, err := h.Manage.GetFilm(c.Request.Context(), mid)
	respond(c, m, err)
}

func (h *Handlers) AddFilm(c *gin.Context) {
	var m entity.Movie
	if err := c.ShouldBindJSON(&m); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	mid, err := h.Manage.AddFilm(c.Request.Context(), &m)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Created(c, gin.H{"mid": mid})
}

// ---- 采集控制 ----

func (h *Handlers) SpiderJobs(c *gin.Context) {
	jobs, err := h.Spider.Jobs(c.Request.Context())
	respond(c, jobs, err)
}

func (h *Handlers) SpiderStart(c *gin.Context) {
	var req struct {
		SourceId string `json:"sourceId"`
		Duration int    `json:"duration"` // -1 全量 / >0 增量小时
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SourceId == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "sourceId required")
		return
	}
	if err := h.Spider.StartCollect(req.SourceId, req.Duration); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Accepted(c, gin.H{"sourceId": req.SourceId, "accepted": true})
}

func (h *Handlers) SpiderPause(c *gin.Context) {
	h.Spider.PauseJob(c.Request.Context(), c.Param("id"))
	dto.OK(c, gin.H{"ok": true})
}
func (h *Handlers) SpiderResume(c *gin.Context) {
	h.Spider.ResumeJob(c.Request.Context(), c.Param("id"))
	dto.OK(c, gin.H{"ok": true})
}
func (h *Handlers) SpiderCancel(c *gin.Context) {
	h.Spider.CancelJob(c.Request.Context(), c.Param("id"))
	dto.OK(c, gin.H{"ok": true})
}

func (h *Handlers) SpiderReset(c *gin.Context) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "confirm required")
		return
	}
	// 常量时间比较二次确认 token, 防破坏性误触/爆破
	if subtle.ConstantTimeCompare([]byte(req.Confirm), []byte(h.ResetToken)) != 1 {
		dto.Error(c, http.StatusUnprocessableEntity, "确认令牌不匹配")
		return
	}
	if err := h.Spider.FilmZero(c.Request.Context()); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Accepted(c, gin.H{"ok": true})
}

func (h *Handlers) CategoryCover(c *gin.Context) {
	var req struct {
		SourceId string `json:"sourceId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SourceId == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "sourceId required")
		return
	}
	if err := h.Spider.CategoryCover(c.Request.Context(), req.SourceId); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Accepted(c, gin.H{"ok": true})
}

// ---- 采集失败台账 ----

// ListCollectFailures GET /manage/collect-failures?status=&page=&size=
// status 缺省 = -1(不限); 0 待补采 / 1 已处理。越界取值直接 400, 不静默截断。
func (h *Handlers) ListCollectFailures(c *gin.Context) {
	status, ok := failureStatus(c.Query("status"))
	if !ok {
		dto.Error(c, http.StatusBadRequest, "status 只能是 -1(不限)/0(待补采)/1(已处理)")
		return
	}
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.Spider.ListFailures(c.Request.Context(), status, page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	np := page.Normalize(service.FailurePageSize) // 与取数层同一默认值, 见 SpiderService.ListFailures
	dto.Page(c, list, np.Current, np.Size, total)
}

// failureStatus 解析 status 查询参数。缺省(空串) = FailureStatusAny。
//
// 早先写的是 int8(queryInt(c, "status", -1)): Atoi 出来的 int 直接窄化会**静默截断** ——
// status=999 被截成 -25、status=257 被截成 1(当成"已处理"), 前者查询恒空却不报错。
// 现在先按 int 判范围, 合法了再转 int8。
func failureStatus(raw string) (int8, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return entity.FailureStatusAny, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < int(entity.FailureStatusAny) || n > int(entity.FailureHandled) {
		return 0, false
	}
	return int8(n), true
}

// RecoverCollectFailures POST /manage/collect-failures/recover {ids?}
// ids 为空 → 补采全部待处理记录; 非空 → 只补这些。后台异步执行(逐源逐页可能很久), 立即返回 202。
func (h *Handlers) RecoverCollectFailures(c *gin.Context) {
	var req struct {
		Ids []int64 `json:"ids"`
	}
	// 允许没有请求体(等价于"补采全部待处理"); 但**有**请求体就必须能解析 ——
	// 早先是 `_ = c.ShouldBindJSON(&req)`, 把"body 写错/字段类型不对"也当成"没带 body",
	// 于是本意"只补这几条"会静默退化成"补采全部待处理", 破坏面被放大。
	if c.Request.ContentLength != 0 {
		switch err := c.ShouldBindJSON(&req); {
		case err == nil:
		case errors.Is(err, io.EOF): // ContentLength 未知(-1)时, 空 body 会走到这里 → 仍按"全部"处理
		default:
			dto.Error(c, http.StatusBadRequest, "invalid body")
			return
		}
	}
	ids := make([]int64, 0, len(req.Ids))
	for _, id := range req.Ids {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	h.Spider.RecoverAsync(ids)
	dto.Accepted(c, gin.H{
		"accepted": true,
		"pending":  h.Spider.PendingFailureCount(c.Request.Context()),
	})
}

// ClearHandledFailures DELETE /manage/collect-failures/handled 清理已处理记录。
func (h *Handlers) ClearHandledFailures(c *gin.Context) {
	n, err := h.Spider.ClearHandledFailures(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"deleted": n})
}

// ---- helpers ----

func respond(c *gin.Context, data any, err error) {
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, data)
}

// readUpload 读取 multipart 文件(限 64MB)。
func readUpload(c *gin.Context) ([]byte, string, bool) {
	fh, err := c.FormFile("file")
	if err != nil {
		dto.Error(c, http.StatusBadRequest, "missing file")
		return nil, "", false
	}
	if fh.Size > 64<<20 {
		dto.Error(c, http.StatusBadRequest, "file too large")
		return nil, "", false
	}
	f, err := fh.Open()
	if err != nil {
		dto.Error(c, http.StatusInternalServerError, "open file")
		return nil, "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		dto.Error(c, http.StatusInternalServerError, "read file")
		return nil, "", false
	}
	return data, fh.Filename, true
}

// ---- 用户管理 ----

// ManageUsers GET /manage/users?keyword=&page=&size= — 用户列表(用户名模糊搜索)。
func (h *Handlers) ManageUsers(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.User.ManageListUsers(c.Request.Context(), c.Query("keyword"), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	n := page.Normalize(20)
	dto.Page(c, list, n.Current, n.Size, total)
}

// manageUserDisabledReq PATCH /manage/users/:id/disabled 请求体。
type manageUserDisabledReq struct {
	Disabled bool `json:"disabled"`
}

// ManageUserSetDisabled 禁用/启用用户。禁用立即踢下线; 不能操作自己(防自锁)。
func (h *Handlers) ManageUserSetDisabled(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	var req manageUserDisabledReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusBadRequest, "bad body")
		return
	}
	if err := h.User.ManageSetUserDisabled(c.Request.Context(), uint(currentUserID(c)), uint(id), req.Disabled); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "disabled": req.Disabled})
}

// manageUserPasswordReq PATCH /manage/users/:id/password 请求体。
type manageUserPasswordReq struct {
	Password string `json:"password"`
}

// ManageUserResetPassword 管理员重置用户密码(不校验旧密码), 重置后该用户全部设备下线。
func (h *Handlers) ManageUserResetPassword(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	var req manageUserPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusBadRequest, "bad body")
		return
	}
	if err := h.User.ManageResetUserPassword(c.Request.Context(), uint(id), req.Password); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "reset": true})
}

// manageUserUpdateReq PUT /manage/users/:id 请求体。
type manageUserUpdateReq struct {
	UserName string `json:"userName"`
	Role     int    `json:"role"`
}

// ManageUserUpdate 编辑用户(用户名/角色)。角色变化后该用户全部设备下线; 不能改自己的角色。
func (h *Handlers) ManageUserUpdate(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	var req manageUserUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusBadRequest, "bad body")
		return
	}
	if err := h.User.ManageUpdateUser(c.Request.Context(), uint(currentUserID(c)), uint(id), req.UserName, req.Role); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "userName": req.UserName, "role": req.Role})
}

// ManageUserDelete 删除用户(硬删, 连同其历史/收藏/跳过设置), 删除后踢下线; 不能删除自己。
func (h *Handlers) ManageUserDelete(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	if err := h.User.ManageDeleteUser(c.Request.Context(), uint(currentUserID(c)), uint(id)); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "deleted": true})
}
