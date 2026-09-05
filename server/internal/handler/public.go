package handler

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"server/internal/domain/repository"
	"server/internal/dto"
	"server/internal/service"
)

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

// M3u8Proxy GET /m3u8/proxy?src=<url>&filterAds=1
// 必须是 GET: <video>/hls.js 直接把本接口 URL 当 manifest src 拉取。
// filterAds 默认开启(前端仅在广告过滤开启时才走代理); 传 filterAds=0 可关闭。
func (h *Handlers) M3u8Proxy(c *gin.Context) {
	raw := c.Query("src")
	if raw == "" {
		dto.Error(c, http.StatusBadRequest, "invalid url")
		return
	}
	filterAds := c.Query("filterAds") != "0"
	proxyMedia := c.Query("proxyMedia") == "1"
	content, err := h.M3u8.Proxy(c.Request.Context(), raw, filterAds, proxyMedia)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=120")
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl; charset=utf-8", []byte(content))
}

// M3u8Stream GET /m3u8/stream?src=&exp=&sig=
// 转发 manifest 生成的签名媒体 URL，支持 Range；不能作为任意 URL 开放代理使用。
func (h *Handlers) M3u8Stream(c *gin.Context) {
	resp, err := h.M3u8.OpenStream(
		c.Request.Context(),
		c.Query("src"),
		c.Query("exp"),
		c.Query("sig"),
		c.GetHeader("Range"),
	)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	defer resp.Body.Close()
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := resp.Header.Get(name); value != "" {
			c.Header(name, value)
		}
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// M3u8Filter POST /m3u8/filter?src=<原始m3u8 URL>  body=raw m3u8 文本  (公开, 端侧混合过滤)
// 不联网抓源, 只对文本剔广告 + 绝对化, 供服务器抓不到的源(如 bf 地域封)在设备侧抓流后送来过滤。
func (h *Handlers) M3u8Filter(c *gin.Context) {
	src := c.Query("src")
	if src == "" {
		dto.Error(c, http.StatusBadRequest, "src required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 8<<20)) // 限 8MB 防滥用
	if err != nil {
		dto.Error(c, http.StatusBadRequest, "read body")
		return
	}
	out, st, err := h.M3u8.FilterText(string(body), src)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	c.Header("X-Ad-Filtered", strconv.Itoa(st.Filtered))
	c.Header("X-Ad-Total", strconv.Itoa(st.Total))
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl; charset=utf-8", []byte(out))
}

// M3u8Stats GET /m3u8/stats?src=<url>&filterAds=1
// 返回该 m3u8 的广告过滤统计 {filtered,total}, 供前端"广告过滤成功"提示判定(filtered>0)。
// 与 M3u8Proxy 共用缓存, 不额外回源。
func (h *Handlers) M3u8Stats(c *gin.Context) {
	raw := c.Query("src")
	if raw == "" {
		dto.Error(c, http.StatusBadRequest, "invalid url")
		return
	}
	filterAds := c.Query("filterAds") != "0"
	st, err := h.M3u8.Stats(c.Request.Context(), raw, filterAds)
	if err != nil {
		dto.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=120")
	dto.OK(c, st)
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
