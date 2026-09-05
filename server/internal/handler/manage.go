package handler

import (
	"crypto/subtle"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/dto"
)

// ---- 仪表盘 / 站点配置 ----

func (h *Handlers) Dashboard(c *gin.Context) { dto.OK(c, h.Manage.Dashboard(c.Request.Context())) }

func (h *Handlers) GetSiteConfig(c *gin.Context) {
	cfg, err := h.Manage.GetSite(c.Request.Context())
	respond(c, cfg, err)
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
	list, total, err := h.Manage.ListUsers(c.Request.Context(), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	np := page.Normalize(20)
	dto.Page(c, list, np.Current, np.Size, total) // entity.User.Password json:"-" 已脱敏
}

// ---- 影片管理 ----

func (h *Handlers) ManageFilms(c *gin.Context) {
	// cid 兼容前端 cid / 旧 category 两种入参
	cid := queryInt64(c, "cid")
	if cid == 0 {
		cid = queryInt64(c, "category")
	}
	spec := repository.FilterSpec{
		Keyword: c.Query("keyword"), Pid: queryInt64(c, "pid"), Cid: cid,
	}
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	res, err := h.Manage.SearchFilms(c.Request.Context(), spec, page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Page(c, dto.ToCards(res.List), res.Page.Current, res.Page.Size, res.Total)
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
