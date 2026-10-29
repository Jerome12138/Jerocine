package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/domain/repository"
	"server/internal/dto"
	"server/internal/service"
)

// DanmuSource GET /films/:mid/danmu/source —— 该影片的弹幕来源候选（全平台）。
// 评审 7.7：前端不碰 dbId，此处只返回平台候选；db_id 为空/无命中由 service 负缓存 2h。
func (h *Handlers) DanmuSource(c *gin.Context) {
	mid, ok := pathInt64(c, "mid")
	if !ok {
		dto.Error(c, http.StatusBadRequest, "invalid mid")
		return
	}
	resp, err := h.Danmu.DanmuSource(c.Request.Context(), mid)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if resp.Providers == nil {
		resp.Providers = []service.DanmuProvider{}
	}
	c.JSON(http.StatusOK, resp)
}

// DanmuBilibili GET /danmu/bilibili?seasonId=&episode= —— 指定 B站剧集整集弹幕。
// 评审 7.2：无鉴权公开接口，必须限流（路由层 RateLimit("danmu", 30, 10)）。
func (h *Handlers) DanmuBilibili(c *gin.Context) {
	seasonID := strings.TrimSpace(c.Query("seasonId"))
	if seasonID == "" {
		dto.Error(c, http.StatusBadRequest, "missing seasonId")
		return
	}
	episode := queryInt(c, "episode", 1)
	comments, err := h.Danmu.DanmuBilibili(c.Request.Context(), seasonID, episode)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if comments == nil {
		comments = []service.DanmuComment{}
	}
	c.JSON(http.StatusOK, gin.H{"comments": comments})
}

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

// DanmuAggregate GET /danmu/aggregate?title=&episode=&year= —— 按标题匹配的整集聚合弹幕。
// 与平台链路（/danmu/:provider/*）相互独立：按影片名搜索作品 → 定位集 → 整集弹幕。
// 公开无鉴权必须限流（路由层 RateLimit("danmu", 30, 10)）；查无作品/集 → 404。
func (h *Handlers) DanmuAggregate(c *gin.Context) {
	title := strings.TrimSpace(c.Query("title"))
	if title == "" {
		dto.Error(c, http.StatusBadRequest, "missing title")
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
	resp, err := h.Danmu.DanmuAggregate(c.Request.Context(), title, year, episode)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// DanmuEpisodes GET /danmu/:provider/episodes?resourceId=&episode= —— 路径 B 分集匹配。
// provider 白名单在 service（未启用 → 400 ErrInvalidArgument，dto.Fail 自动映射）。
// 路径 B 分段点播第一步：拿 matched.episodeId（复合串，前端 encodeURIComponent 后透传）。
// 无匹配集 → 200 { matched: null }（前端静默无弹幕）；danmu-service 不可用 → 500。
func (h *Handlers) DanmuEpisodes(c *gin.Context) {
	resourceID := strings.TrimSpace(c.Query("resourceId"))
	if resourceID == "" {
		dto.Error(c, http.StatusBadRequest, "missing resourceId")
		return
	}
	episode := queryInt(c, "episode", 1)
	matched, found, err := h.Danmu.DanmuEpisodes(c.Request.Context(), c.Param("provider"), resourceID, episode)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if !found {
		c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"matched": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"matched": matched}})
}

// DanmuSegments GET /danmu/:provider/segments?episodeId= —— 路径 B 弹幕分段索引。
// episodeId 为复合串（cid=..&vid=..），必须 encodeURIComponent 透传。
func (h *Handlers) DanmuSegments(c *gin.Context) {
	episodeID := strings.TrimSpace(c.Query("episodeId"))
	if episodeID == "" {
		dto.Error(c, http.StatusBadRequest, "missing episodeId")
		return
	}
	segments, err := h.Danmu.DanmuSegments(c.Request.Context(), c.Param("provider"), episodeID)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if segments == nil {
		segments = []service.DanmuSegment{}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": segments})
}

// DanmuComments GET /danmu/:provider/comments?episodeId=&segmentId= —— 路径 B 单段弹幕。
func (h *Handlers) DanmuComments(c *gin.Context) {
	episodeID := strings.TrimSpace(c.Query("episodeId"))
	if episodeID == "" {
		dto.Error(c, http.StatusBadRequest, "missing episodeId")
		return
	}
	segmentID := strings.TrimSpace(c.Query("segmentId"))
	if segmentID == "" {
		dto.Error(c, http.StatusBadRequest, "missing segmentId")
		return
	}
	comments, err := h.Danmu.DanmuComments(c.Request.Context(), c.Param("provider"), episodeID, segmentID)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	if comments == nil {
		comments = []service.DanmuComment{}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": comments})
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
