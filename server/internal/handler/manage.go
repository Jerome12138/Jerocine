package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/dto"
	"server/internal/platform/mask"
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

func (h *Handlers) GetSiteConfig(c *gin.Context) {
	cfg, err := h.Manage.GetSite(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, siteConfigResp{
		SiteConfig:    *cfg,
		TmdbKeyMasked: mask.Middle(cfg.TmdbAPIKey),
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

// ---- 弹幕服务后台配置 ----

// ---- 直播源管理（/manage/live-sources） ----

// ListLiveSources GET /manage/live-sources — 全量直播源（含停用）。
func (h *Handlers) ListLiveSources(c *gin.Context) {
	list, err := h.Live.ListLiveSources(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if list == nil {
		list = []entity.LiveSource{}
	}
	c.JSON(http.StatusOK, gin.H{"sources": list})
}

// CreateLiveSource POST /manage/live-sources {name,url,remark,sort,state}
func (h *Handlers) CreateLiveSource(c *gin.Context) {
	var req service.LiveSourceInput
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	src, err := h.Live.CreateLiveSource(c.Request.Context(), req)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	respond(c, src, nil)
}

// UpdateLiveSource PUT /manage/live-sources/:id {name,url,remark,sort,state} — 整体覆盖。
func (h *Handlers) UpdateLiveSource(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req service.LiveSourceInput
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	respond(c, nil, h.Live.UpdateLiveSource(c.Request.Context(), id, req))
}

// DeleteLiveSource DELETE /manage/live-sources/:id — 删除并清该源频道缓存。
func (h *Handlers) DeleteLiveSource(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	respond(c, nil, h.Live.DeleteLiveSource(c.Request.Context(), id))
}

// GetDanmuConfig GET /manage/danmu-config — 弹幕总开关 + 开放平台白名单 + 平台全集。
func (h *Handlers) GetDanmuConfig(c *gin.Context) {
	v, err := h.Manage.GetDanmuConfig(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, v)
}

// SaveDanmuConfig PUT /manage/danmu-config {enabled, providers} — 保存后清弹幕来源缓存, 切换立即生效。
func (h *Handlers) SaveDanmuConfig(c *gin.Context) {
	var req service.DanmuConfigInput
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.SaveDanmuConfig(c.Request.Context(), req); err != nil {
		dto.Fail(c, err)
		return
	}
	v, err := h.Manage.GetDanmuConfig(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, v)
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

// cronTaskResp 定时任务响应: 原字段 + 最近一次运行信息(任务台账回填)。
// 定时任务页据此展示"上次运行时间/状态/失败原因", 不再是无历史的黑盒。
type cronTaskResp struct {
	entity.CronTask
	LastRunAt   int64  `json:"lastRunAt"`
	LastStatus  string `json:"lastStatus"`
	LastError   string `json:"lastError"`
	LastMessage string `json:"lastMessage"`
}

func (h *Handlers) ListCrons(c *gin.Context) {
	list, err := h.Manage.ListCrons(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	ids := make([]int64, 0, len(list))
	for i := range list {
		ids = append(ids, list[i].Id)
	}
	latest := map[int64]entity.TaskRun{}
	if h.Tasks != nil && len(ids) > 0 {
		if m, e := h.Tasks.LatestByCron(c.Request.Context(), ids); e == nil {
			latest = m
		}
	}
	out := make([]cronTaskResp, 0, len(list))
	for _, t := range list {
		r := cronTaskResp{CronTask: t}
		if lr, ok := latest[t.Id]; ok {
			r.LastRunAt, r.LastStatus, r.LastError, r.LastMessage = lr.StartedAt, lr.Status, lr.Error, lr.Message
		}
		out = append(out, r)
	}
	dto.OK(c, out)
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

// CronRun POST /manage/cron-tasks/:id/run 手动立即执行一次定时任务(不改变调度表)。
func (h *Handlers) CronRun(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Spider.TriggerCron(c.Request.Context(), id); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Accepted(c, gin.H{"accepted": true})
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
// 返回 502 且**不落库** —— 保留上一轮快照。每次触发都会登记一条任务台账(hot_refresh)。
func (h *Handlers) HotRefresh(c *gin.Context) {
	if h.Hot == nil {
		dto.Error(c, http.StatusServiceUnavailable, "hot service unavailable")
		return
	}
	rep, err := h.runHotRefresh(c.Request.Context())
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

// runHotRefresh 执行一轮榜单刷新并登记任务台账(手动刷新 / 历史任务重跑共用)。
//
// 刷新实测 1-2 分钟, 必须与 HTTP 请求生命周期解耦: 网关默认 60s 读超时会先掐连接,
// 请求 ctx 随之取消, 抓取半路夭折、零落库(2026-10-23 线上事故)。WithoutCancel 保留
// 请求上的取值, 只剥离取消信号, 另加 10min 硬上限防悬挂 —— 客户端断开只影响响应送达,
// 服务端照常跑完落库, 结果看任务台账。
func (h *Handlers) runHotRefresh(ctx context.Context) (service.HotRefreshReport, error) {
	ctx = context.WithoutCancel(ctx)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	runId := h.Tasks.Start(entity.TaskRunKindManual, entity.TaskRunHotRefresh, 0, "", "榜单热度刷新", 0)
	rep, err := h.Hot.Refresh(ctx)
	if err != nil {
		status := entity.TaskRunFailed
		if errors.Is(err, service.ErrHotBusy) {
			status = entity.TaskRunCanceled // 上一轮未结束, 本轮未执行
		}
		h.Tasks.Finish(runId, status, 0, 0, 0, "", err.Error())
		return rep, err
	}
	msg := fmt.Sprintf("抓取 %d 条, 匹配 %d 部, 应用 %d 行, 耗时 %dms",
		rep.Fetched, rep.Matched, rep.Applied, rep.ElapsedMs)
	h.Tasks.Finish(runId, entity.TaskRunSuccess, 0, 0, 0, msg, "")
	return rep, nil
}

// ---- 任务管理(运行台账) ----

// TaskRuns GET /manage/tasks/runs?status=&type=&page=&size= 任务运行台账分页。
// status: running | success | failed | canceled; type: collect | recover | category_cover | hot_refresh。
func (h *Handlers) TaskRuns(c *gin.Context) {
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	list, total, err := h.Tasks.List(c.Request.Context(), c.Query("status"), c.Query("type"), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	np := page.Normalize(20)
	dto.Page(c, list, np.Current, np.Size, total)
}

// TaskOverview GET /manage/tasks/overview 任务管理页统计卡。
// Running = Redis 活跃采集 job(在跑/暂停) + 台账 running 行数。
func (h *Handlers) TaskOverview(c *gin.Context) {
	jobs, _ := h.Spider.Jobs(c.Request.Context())
	active := 0
	for _, j := range jobs {
		if j.State == cache.JobRunning || j.State == cache.JobPaused {
			active++
		}
	}
	o := h.Tasks.Overview(c.Request.Context(), active, h.Spider.PendingFailureCount(c.Request.Context()))
	dto.OK(c, o)
}

// TaskRerun POST /manage/tasks/:id/rerun 重跑一条历史任务(按其类型重放动作, 会登记一条新记录)。
func (h *Handlers) TaskRerun(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok || id <= 0 {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	run, err := h.Tasks.Get(c.Request.Context(), id)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	switch run.Type {
	case entity.TaskRunCollect:
		if err := h.Spider.StartCollect(run.SourceId, run.Hours); err != nil {
			dto.Fail(c, err)
			return
		}
	case entity.TaskRunRecover:
		h.Spider.RecoverAsync(nil) // 重跑补采 = 补采全部待处理
	case entity.TaskRunCategoryCover:
		if err := h.Spider.CategoryCover(c.Request.Context(), run.SourceId); err != nil {
			dto.Fail(c, err)
			return
		}
	case entity.TaskRunHotRefresh:
		if h.Hot == nil {
			dto.Error(c, http.StatusServiceUnavailable, "hot service unavailable")
			return
		}
		if _, err := h.runHotRefresh(c.Request.Context()); err != nil {
			dto.Fail(c, err)
			return
		}
	default:
		dto.Error(c, http.StatusBadRequest, "unsupported task type: "+run.Type)
		return
	}
	dto.Accepted(c, gin.H{"accepted": true, "type": run.Type})
}

// ---- 运行参数(后台「系统设置」: 账号与安全 / 采集性能) ----

// GetRuntimeSettings GET /manage/settings — 登录策略与采集并发。
func (h *Handlers) GetRuntimeSettings(c *gin.Context) {
	v, err := h.Manage.RuntimeSettings(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, v)
}

// SaveRuntimeSettings PUT /manage/settings — 保存(1h~1年 / 1~50 设备 / 1~256 并发)。
// 都是"下次生效"的项: 登录策略在下次登录应用, 采集并发在下一轮采集应用。
func (h *Handlers) SaveRuntimeSettings(c *gin.Context) {
	var req service.RuntimeSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.Error(c, http.StatusUnprocessableEntity, "invalid body")
		return
	}
	if err := h.Manage.SaveRuntimeSettings(c.Request.Context(), req); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, req)
}

// ---- 在线会话(当前生效的登录凭据) ----

// ManageSessions GET /manage/sessions?keyword=&limit= — 最近活跃会话(按最近活跃倒序)。
func (h *Handlers) ManageSessions(c *gin.Context) {
	list := h.User.ListLoginSessions(c.Request.Context(), c.Query("keyword"), queryInt(c, "limit", 0))
	dto.OK(c, gin.H{"list": list, "total": len(list)})
}

// ManageRevokeSession DELETE /manage/sessions/:id — 踢掉单个会话(其它设备不受影响)。
func (h *Handlers) ManageRevokeSession(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		dto.Error(c, http.StatusBadRequest, "bad id")
		return
	}
	if err := h.User.RevokeLoginSession(c.Request.Context(), id); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "revoked": true})
}

// ---- 用户 ----

func (h *Handlers) CreateUser(c *gin.Context) {
	var req struct {
		UserName string `json:"userName"`
		Password string `json:"password"`
		Role     int    `json:"role"`
		Email    string `json:"email"` // 可选: 手工建号也可带邮箱(email_verified=0, 不强制验证)
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserName == "" || req.Password == "" {
		dto.Error(c, http.StatusUnprocessableEntity, "userName/password required")
		return
	}
	u, err := h.Manage.CreateUser(c.Request.Context(), req.UserName, req.Password, req.Role, req.Email)
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
// email 选填: 空串 = 解绑邮箱; 填了服务端校验格式与唯一性(改地址会把 email_verified 重置为 0)。
type manageUserUpdateReq struct {
	UserName string `json:"userName"`
	Role     int    `json:"role"`
	Email    string `json:"email"`
}

// ManageUserUpdate 编辑用户(用户名/角色/邮箱)。角色变化后该用户全部设备下线; 不能改自己的角色。
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
	if err := h.User.ManageUpdateUser(c.Request.Context(), uint(currentUserID(c)), uint(id), req.UserName, req.Role, req.Email); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.OK(c, gin.H{"id": id, "userName": req.UserName, "role": req.Role, "email": req.Email})
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
