package spider

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"server/internal/domain/entity"
	"server/internal/platform/safehttp"
)

const fetchMaxBytes = 16 << 20 // 单页响应上限 16MB

// Fetcher 采集站 HTTP 客户端(防 SSRF)。
type Fetcher struct {
	client *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{client: safehttp.NewClient(20 * time.Second)}
}

// NewFetcherWithTimeout 短超时探测/测速专用(正式采集用 20s, 实测延时用更短)。
func NewFetcherWithTimeout(d time.Duration) *Fetcher {
	return &Fetcher{client: safehttp.NewClient(d)}
}

// ProbeResult 单次探测结果。
type ProbeResult struct {
	LatencyMs  int64  // 采集 API 请求往返耗时
	Films      int    // ac=detail 首页解析到的影片数(0 = 连通但无可采集数据)
	PageCount  int    // 分页总数(资源最全判定)
	Total      int    // 目录总片数(maccms total, 缺省 0)
	SampleM3u8 string // 首页首部影片的第一条 m3u8 线路(抽样播放延时用, 可能为空)
}

// Probe 打一次真实采集请求(ac=detail&pg=1)并解析, 验证站点连通且能产出可采集数据,
// 同时取目录大小与一条 m3u8 样本(供上层量播放延时)。
func (f *Fetcher) Probe(ctx context.Context, src *entity.CollectSource) (ProbeResult, error) {
	start := time.Now()
	body, err := f.fetch(ctx, src.Uri, detailParams(1, 0))
	lat := time.Since(start).Milliseconds()
	r := ProbeResult{LatencyMs: lat}
	if err != nil {
		return r, err
	}
	if src.ResultModel == entity.ResultXML {
		r.Films, r.PageCount, err = parseDetailXML(body)
		return r, err
	}
	r.Films, r.PageCount, r.Total, r.SampleM3u8, err = parseDetailJSON(body)
	return r, err
}

// parseDetailJSON 纯函数: 解析 JSON 详情页, 返回片数/分页数/总数/首条 m3u8 样本(便于单测)。
func parseDetailJSON(body []byte) (films, pageCount, total int, sampleM3u8 string, err error) {
	var dp filmDetailPage
	if err = json.Unmarshal(body, &dp); err != nil {
		return
	}
	films = len(dp.List)
	pageCount = dp.PageCount
	total = dp.Total
	if films > 0 {
		sampleM3u8 = firstM3u8(dp.List[0])
	}
	return
}

// parseDetailXML 纯函数: 解析 XML 详情页, 返回片数/分页数(XML 不取 total/m3u8 样本)。
func parseDetailXML(body []byte) (films, pageCount int, err error) {
	var rss rssDetail
	if err = xml.Unmarshal(body, &rss); err != nil {
		return
	}
	return len(rss.List.Videos), rss.List.PageCount, nil
}

// firstM3u8 取一部影片里第一条含 .m3u8 的播放地址(复用 maccms 播放串解析)。
func firstM3u8(d filmDetail) string {
	for _, g := range parsePlayGroups(d.VodPlayFrom, d.VodPlayURL, d.VodPlayNote) {
		for _, ep := range g.Episodes {
			if strings.Contains(ep.Link, ".m3u8") {
				return ep.Link
			}
		}
	}
	return ""
}

// MeasureURL 计时拉取一个 URL(抽样 m3u8 播放延时), 走 safehttp 防 SSRF, 仅读首段。
func (f *Fetcher) MeasureURL(ctx context.Context, rawURL string) (int64, error) {
	if rawURL == "" {
		return 0, errors.New("empty url")
	}
	if _, err := safehttp.ValidateURL(rawURL); err != nil {
		return 0, err
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) // m3u8 通常几 KB
	lat := time.Since(start).Milliseconds()
	if resp.StatusCode != http.StatusOK {
		return lat, errors.New("status " + strconv.Itoa(resp.StatusCode))
	}
	return lat, nil
}

// fetch GET uri?params, 校验目标为公网后读取(限大小)。
func (f *Fetcher) fetch(ctx context.Context, uri string, params url.Values) ([]byte, error) {
	full := uri + "?" + params.Encode()
	if _, err := safehttp.ValidateURL(full); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("spider: upstream status " + strconv.Itoa(resp.StatusCode))
	}
	return io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes))
}

func detailParams(pg, hours int) url.Values {
	p := url.Values{}
	p.Set("ac", "detail")
	p.Set("pg", strconv.Itoa(pg))
	if hours > 0 {
		p.Set("h", strconv.Itoa(hours))
	}
	return p
}

// PageCount 取分页总页数。
func (f *Fetcher) PageCount(ctx context.Context, src *entity.CollectSource, hours int) (int, error) {
	body, err := f.fetch(ctx, src.Uri, detailParams(1, hours))
	if err != nil {
		return 0, err
	}
	if src.ResultModel == entity.ResultXML {
		var rss rssDetail
		if err := xml.Unmarshal(body, &rss); err != nil {
			return 0, err
		}
		return rss.List.PageCount, nil
	}
	var cp commonPage
	if err := json.Unmarshal(body, &cp); err != nil {
		return 0, err
	}
	return cp.PageCount, nil
}

// JSONDetails 拉取并解析一页 JSON 详情。
func (f *Fetcher) JSONDetails(ctx context.Context, src *entity.CollectSource, pg, hours int) ([]filmDetail, error) {
	body, err := f.fetch(ctx, src.Uri, detailParams(pg, hours))
	if err != nil {
		return nil, err
	}
	var dp filmDetailPage
	if err := json.Unmarshal(body, &dp); err != nil {
		return nil, err
	}
	return dp.List, nil
}

// XMLDetails 拉取并解析一页 XML 详情。
func (f *Fetcher) XMLDetails(ctx context.Context, src *entity.CollectSource, pg, hours int) ([]videoDetail, error) {
	body, err := f.fetch(ctx, src.Uri, detailParams(pg, hours))
	if err != nil {
		return nil, err
	}
	var rss rssDetail
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, err
	}
	return rss.List.Videos, nil
}

// searchParams 按片名搜索详情(maccms ac=detail&wd=<关键字>&pg=N)。
func searchParams(pg int, wd string) url.Values {
	p := url.Values{}
	p.Set("ac", "detail")
	p.Set("pg", strconv.Itoa(pg))
	p.Set("wd", wd)
	return p
}

// SearchJSONDetails 按片名搜索一页 JSON 详情(不写库, 供预览/按名采集)。
func (f *Fetcher) SearchJSONDetails(ctx context.Context, src *entity.CollectSource, wd string, pg int) ([]filmDetail, error) {
	body, err := f.fetch(ctx, src.Uri, searchParams(pg, wd))
	if err != nil {
		return nil, err
	}
	var dp filmDetailPage
	if err := json.Unmarshal(body, &dp); err != nil {
		return nil, err
	}
	return dp.List, nil
}

// SearchXMLDetails 按片名搜索一页 XML 详情。
func (f *Fetcher) SearchXMLDetails(ctx context.Context, src *entity.CollectSource, wd string, pg int) ([]videoDetail, error) {
	body, err := f.fetch(ctx, src.Uri, searchParams(pg, wd))
	if err != nil {
		return nil, err
	}
	var rss rssDetail
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, err
	}
	return rss.List.Videos, nil
}

// idsParams 按 vodId 精确取详情(maccms ac=detail&ids=<id 逗号分隔>)。
// 上游对同名多版本只能靠精确 id 区分, 故按名搜出候选后采集走 ids 锁定具体那部。
func idsParams(ids string) url.Values {
	p := url.Values{}
	p.Set("ac", "detail")
	p.Set("ids", ids)
	return p
}

// IdsJSONDetails 按 ids 拉取一页 JSON 详情(不分页, 仿 SearchJSONDetails)。
func (f *Fetcher) IdsJSONDetails(ctx context.Context, src *entity.CollectSource, ids string) ([]filmDetail, error) {
	body, err := f.fetch(ctx, src.Uri, idsParams(ids))
	if err != nil {
		return nil, err
	}
	var dp filmDetailPage
	if err := json.Unmarshal(body, &dp); err != nil {
		return nil, err
	}
	return dp.List, nil
}

// IdsXMLDetails 按 ids 拉取一页 XML 详情(仿 SearchXMLDetails)。
func (f *Fetcher) IdsXMLDetails(ctx context.Context, src *entity.CollectSource, ids string) ([]videoDetail, error) {
	body, err := f.fetch(ctx, src.Uri, idsParams(ids))
	if err != nil {
		return nil, err
	}
	var rss rssDetail
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, err
	}
	return rss.List.Videos, nil
}

// Categories 拉取分类列表(JSON; XML 站点分类暂不支持)。
func (f *Fetcher) Categories(ctx context.Context, src *entity.CollectSource) ([]filmClass, error) {
	if src.ResultModel == entity.ResultXML {
		return nil, errors.New("spider: XML 分类采集暂未支持")
	}
	p := url.Values{}
	p.Set("ac", "list")
	p.Set("pg", "1")
	body, err := f.fetch(ctx, src.Uri, p)
	if err != nil {
		return nil, err
	}
	var lp filmListPage
	if err := json.Unmarshal(body, &lp); err != nil {
		return nil, err
	}
	return lp.Class, nil
}
