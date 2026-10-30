package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/domain/repository"
	"server/internal/dto"
	"server/internal/service"
)

// LiveSources GET /live/sources —— 启用的直播源列表（直播页源切换）。
func (h *Handlers) LiveSources(c *gin.Context) {
	resp, err := h.Live.LiveSources(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// LiveChannels GET /live/sources/:id/channels —— 解析指定直播源的 m3u 出频道列表（带缓存）。
// 停用/不存在的源一律 404。公开无鉴权，必须限流（路由层 RateLimit）。
func (h *Handlers) LiveChannels(c *gin.Context) {
	id, ok := pathInt64(c, "id")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid id")
		return
	}
	resp, err := h.Live.LiveChannels(c.Request.Context(), id)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DanmuAggregate GET /danmu/aggregate?title=&episode=&year=&from=&to= —— 按标题匹配的聚合弹幕。
// from/to（秒, 可选）: 分片过滤 time∈[from,to) —— 前端懒加载起播段+按进度预取；
// 缺省返回整集。响应 total = 整集总条数（与 comments 数对比判断是否拉完）。
// 公开无鉴权必须限流（路由层 RateLimit("danmu", 30, 10)）；查无作品/集 → 404。
func (h *Handlers) DanmuAggregate(c *gin.Context) {
	title := strings.TrimSpace(c.Query("title"))
	if title == "" {
		dto.Error(c, http.StatusBadRequest, "missing title")
		return
	}
	if len([]rune(title)) > 128 {
		dto.Error(c, http.StatusBadRequest, "title too long")
		return
	}
	episode := queryInt(c, "episode", 1)
	if episode < 1 || episode > 9999 {
		dto.Error(c, http.StatusBadRequest, "invalid episode")
		return
	}
	year := queryInt(c, "year", 0)
	// 年份仅作匹配打分: 越界值按 0 处理(避免 year=2 之类把标题含"2"的作品全部加分)
	if year < 1900 || year > 2100 {
		year = 0
	}
	from := queryInt(c, "from", 0)
	to := queryInt(c, "to", 0)
	// 分片参数合理性: from/to 负值按缺省(整集)处理; to 不得小于 from
	if from < 0 {
		from = 0
	}
	if to < from {
		to = 0
	}
	resp, err := h.Danmu.DanmuAggregate(c.Request.Context(), title, year, episode, from, to)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Home GET /home
func (h *Handlers) Home(c *gin.Context) {
	data, err := h.Film.Home(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.WriteCached(c, dto.ToHome(data), 300)
}

// Categories GET /categories
func (h *Handlers) Categories(c *gin.Context) {
	nav, err := h.Film.NavCategories(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.WriteCached(c, dto.ToNavList(nav), 300)
}

// Filters GET /categories/:pid/filters
func (h *Handlers) Filters(c *gin.Context) {
	pid, ok := pathInt64(c, "pid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid pid")
		return
	}
	opts, err := h.Film.TagOptions(c.Request.Context(), pid)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.WriteCached(c, opts, 3600)
}

// Films GET /films  (keyword 走全文检索, 否则多维筛选)
func (h *Handlers) Films(c *gin.Context) {
	ctx := c.Request.Context()
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	keyword := c.Query("keyword")
	var cp service.CardPage
	var err error
	if keyword != "" {
		cp, err = h.Film.Search(ctx, keyword, page)
	} else {
		spec := repository.FilterSpec{
			Pid: queryInt64(c, "pid"), Cid: queryInt64(c, "category"),
			Plot: c.Query("plot"), Area: c.Query("area"), Language: c.Query("language"),
			Year: queryInt(c, "year", 0), Sort: c.Query("sort"),
		}
		cp, err = h.Film.Filter(ctx, spec, page)
	}
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Page(c, dto.ToCards(cp.List), cp.Page.Current, cp.Page.Size, cp.Total)
}

// HotBoard GET /hotboard?board=&pid=&year=&area=&language=&sort=&page=&size=
// board 为空 = 全榜混排(按 hot_score, 枪版动态降级); board 指定 = 该榜视图(按榜位次)。
func (h *Handlers) HotBoard(c *gin.Context) {
	ctx := c.Request.Context()
	page := repository.Page{Current: queryInt(c, "page", 1), Size: queryInt(c, "size", 0)}
	spec := repository.FilterSpec{
		Pid:  queryInt64(c, "pid"),
		Area: c.Query("area"), Language: c.Query("language"),
		Year: queryInt(c, "year", 0),
	}
	cp, err := h.Film.HotBoard(ctx, c.Query("board"), spec, c.Query("sort"), page)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.Page(c, dto.ToCards(cp.List), cp.Page.Current, cp.Page.Size, cp.Total)
}

// HotBoardFilters GET /hotboard/filters —— 榜单固定 + 年份/地区/语言聚合。
func (h *Handlers) HotBoardFilters(c *gin.Context) {
	data, err := h.Film.HotBoardFilters(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.WriteCached(c, data, 3600)
}

// Classify GET /films/classify?pid=
func (h *Handlers) Classify(c *gin.Context) {
	ctx := c.Request.Context()
	pid := queryInt64(c, "pid")
	if pid <= 0 {
		dto.Error(c, http.StatusBadRequest, "missing pid")
		return
	}
	data, err := h.Film.Classify(ctx, pid)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	title, _ := h.Film.PidCategory(ctx, pid)
	dto.WriteCached(c, dto.ToClassify(data, title), 300)
}

// Detail GET /films/:mid
func (h *Handlers) Detail(c *gin.Context) {
	ctx := c.Request.Context()
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	d, found, err := h.Film.Detail(ctx, mid)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if !found {
		dto.Error(c, http.StatusNotFound, "movie not found")
		return
	}
	related, _ := h.Film.Related(ctx, mid)
	dto.WriteCached(c, dto.FilmDetailResp{Detail: dto.ToFilmDetail(d), Related: dto.ToCards(related)}, 300)
}

// Related GET /films/:mid/related
func (h *Handlers) Related(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	related, err := h.Film.Related(c.Request.Context(), mid)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.WriteCached(c, dto.ToCards(related), 300)
}

// Play GET /films/:mid/play?source=&episode=
func (h *Handlers) Play(c *gin.Context) {
	ctx := c.Request.Context()
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	d, found, err := h.Film.Detail(ctx, mid)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if !found {
		dto.Error(c, http.StatusNotFound, "movie not found")
		return
	}
	detail := dto.ToFilmDetail(d)
	source := c.Query("source")
	episode := queryInt(c, "episode", 0)
	if source == "" && len(detail.Sources) > 0 {
		source = detail.Sources[0].Id
	}
	var current dto.Episode
	for _, s := range detail.Sources {
		if s.Id == source {
			if episode >= 0 && episode < len(s.Episodes) {
				current = s.Episodes[episode]
			}
			break
		}
	}
	related, _ := h.Film.Related(ctx, mid)
	dto.WriteCached(c, dto.PlayInfoResp{
		Detail: detail, Current: current, CurrentSource: source, CurrentEpisode: episode,
		Related: dto.ToCards(related),
	}, 120)
}

// SiteConfig GET /config/site
func (h *Handlers) SiteConfig(c *gin.Context) {
	cfg, err := h.Config.SiteConfig(c.Request.Context())
	if err != nil {
		dto.Fail(c, err)
		return
	}
	dto.WriteCached(c, cfg, 600)
}

// VersionLatest GET /app/version/latest?channel=
func (h *Handlers) VersionLatest(c *gin.Context) {
	channel := int8(queryInt(c, "channel", 0))
	dto.OK(c, h.Config.LatestVersion(c.Request.Context(), channel))
}

// TelemetryIngest POST /telemetry/events  (公开)
func (h *Handlers) TelemetryIngest(c *gin.Context) {
	ua := c.GetHeader("User-Agent")
	// 反扫描/反撞IP域名: 来源域非本站 或 bot UA → 静默丢弃(204, 不回显给攻击者, 不落库)。
	if h.Telemetry.IsNoise(c.Request.Context(), c.Request.Host, c.GetHeader("Origin"), c.GetHeader("Referer"), ua) {
		c.Status(http.StatusNoContent)
		return
	}
	var body struct {
		Events []service.IngestEvent `json:"events"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		dto.Error(c, http.StatusBadRequest, "invalid body")
		return
	}
	srcHost := h.Telemetry.SourceHost(c.Request.Host, c.GetHeader("Origin"), c.GetHeader("Referer"))
	if err := h.Telemetry.Ingest(c.Request.Context(), body.Events, nil, c.ClientIP(), ua, srcHost); err != nil {
		dto.Fail(c, err)
		return
	}
	dto.NoContent(c)
}
