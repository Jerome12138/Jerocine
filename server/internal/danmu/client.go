// Package danmu 弹幕数据客户端（P0：豆瓣 vendors 解析 + B站弹幕取数）。
//
// 链路：movie.db_id → 豆瓣 rexxar vendors（正版播放渠道，app scheme）→ 平台资源 id
// → B站 season → episodes（按集定位 cid）→ seg.so 弹幕。
// 请求纪律同 douban/collection.go：固定 UA + Referer；vendors 接口 POC 实测无限流，
// 但保守起见仍带 1 次重试；B站接口按平台频率纪律串行调用（见 service 层）。
package danmu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"server/internal/danmu/proto"
)

const (
	// doubanVendorsURL 豆瓣正版播放渠道（vendors）。POC 实测：需 Referer 指向该 subject 页。
	doubanVendorsURL = "https://m.douban.com/rexxar/api/v2/movie/%s?for_mobile=1"
	doubanReferer    = "https://m.douban.com/movie/subject/%s/?dt_dapp=1"
	// biliSeasonURL B站剧集/电影正片列表（含各集 cid）。
	biliSeasonURL = "https://api.bilibili.com/pgc/view/web/season?season_id=%s"
	// biliSegURL B站弹幕分片。一集约 8 段（segment_index 1..8），串行拉取。
	biliSegURL = "https://api.bilibili.com/x/v2/dm/web/seg.so?type=1&oid=%d&segment_index=%d"

	uaMobile = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
	uaPC     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Vendor 豆瓣返回的一个正版播放渠道（is_ad 广告位已过滤）。
type Vendor struct {
	ID  string `json:"id"`  // qq / iqiyi / youku / bilibili
	URI string `json:"uri"` // app scheme（如 txvideo://、bilibili://）
}

// PlatformResource 从 vendors URI 解析出的平台资源标识。
type PlatformResource struct {
	Provider   string `json:"provider"`   // tencent / iqiyi / youku / bilibili
	ResourceID string `json:"resourceId"` // 腾讯 cid / 爱奇艺 tvid / 优酷 showid / B站 seasonId
}

// Episode B站剧集条目（title 供分集匹配，cid 供弹幕定位）。
type Episode struct {
	No    int    `json:"no"`
	Cid   int64  `json:"cid"`
	Title string `json:"title"`
}

// Client 弹幕数据客户端：豆瓣 vendors + B站 season/seg.so。
type Client struct {
	hc *http.Client
}

// New 构造客户端。
func New() *Client {
	return &Client{hc: &http.Client{Timeout: 20 * time.Second}}
}

// ---- 豆瓣 vendors ----

// DoubanVendors 按豆瓣 id 取正版播放渠道（vendors，is_ad 已过滤）。无渠道返回空切片 + nil。
func (c *Client) DoubanVendors(ctx context.Context, doubanID string) ([]Vendor, error) {
	endpoint := fmt.Sprintf(doubanVendorsURL, url.PathEscape(doubanID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaMobile)
	req.Header.Set("Referer", fmt.Sprintf(doubanReferer, url.PathEscape(doubanID)))
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.hc.Do(req)
		if err != nil {
			lastErr = err
			if attempt == 0 {
				time.Sleep(time.Second)
			}
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			break
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("douban vendors status %d", resp.StatusCode)
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
				if attempt == 0 {
					time.Sleep(3 * time.Second)
					continue
				}
			}
			break
		}
		return parseVendors(body)
	}
	return nil, lastErr
}

// parseVendors 解析 vendors 列表（is_ad 广告位剔除）。结构异常返回空 + 错误。
func parseVendors(body []byte) ([]Vendor, error) {
	var raw struct {
		Vendors []struct {
			ID    string `json:"id"`
			IsAd  bool   `json:"is_ad"`
			URI   string `json:"uri"`
		} `json:"vendors"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("vendors 响应非 JSON: %w", err)
	}
	out := make([]Vendor, 0, len(raw.Vendors))
	for _, v := range raw.Vendors {
		if v.IsAd || v.ID == "" || v.URI == "" {
			continue
		}
		out = append(out, Vendor{ID: v.ID, URI: v.URI})
	}
	return out, nil
}

// ParsePlatformResource 从 vendors URI（app scheme）解析平台资源标识。
// 解析不出返回 ok=false（该渠道弃用，不影响其它渠道）。
func ParsePlatformResource(uri string) (PlatformResource, bool) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return PlatformResource{}, false
	}
	q := parsed.Query()
	get := func(k string) string { return q.Get(k) }
	switch {
	case strings.HasPrefix(uri, "txvideo://") || strings.Contains(parsed.Host, "qq.com"):
		return PlatformResource{Provider: "tencent", ResourceID: get("cid")}, get("cid") != ""
	case strings.HasPrefix(uri, "iqiyi://") || strings.Contains(parsed.Host, "iqiyi.com"):
		return PlatformResource{Provider: "iqiyi", ResourceID: get("tvid")}, get("tvid") != ""
	case strings.HasPrefix(uri, "youku://") || strings.Contains(parsed.Host, "youku.com"):
		return PlatformResource{Provider: "youku", ResourceID: get("showid")}, get("showid") != ""
	case strings.HasPrefix(uri, "bilibili://") || strings.Contains(parsed.Host, "bilibili.com"):
		m := seasonRe.FindStringSubmatch(parsed.Path)
		if len(m) == 2 && m[1] != "" {
			return PlatformResource{Provider: "bilibili", ResourceID: m[1]}, true
		}
	}
	return PlatformResource{}, false
}

var seasonRe = regexp.MustCompile(`/season/(\d+)`)

// ---- B站 ----

// BiliEpisodes 按 seasonId 取剧集列表（含各集 cid 与 title）。
func (c *Client) BiliEpisodes(ctx context.Context, seasonID string) ([]Episode, error) {
	endpoint := fmt.Sprintf(biliSeasonURL, url.QueryEscape(seasonID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaPC)
	req.Header.Set("Referer", "https://www.bilibili.com/")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bili season status %d", resp.StatusCode)
	}
	var raw struct {
		Code   int    `json:"code"`
		Result struct {
			Episodes []struct {
				ID    int64  `json:"id"`
				Cid   int64  `json:"cid"`
				Title string `json:"title"`
			} `json:"episodes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("bili season 响应非 JSON: %w", err)
	}
	if raw.Code != 0 {
		return nil, fmt.Errorf("bili season code %d", raw.Code)
	}
	out := make([]Episode, 0, len(raw.Result.Episodes))
	for _, e := range raw.Result.Episodes {
		if e.Cid <= 0 {
			continue
		}
		out = append(out, Episode{Cid: e.Cid, Title: strings.TrimSpace(e.Title)})
	}
	return out, nil
}

// BiliSeg 拉取 cid 的指定弹幕分片并解析。返回的弹幕时间轴相对该视频（毫秒）。
func (c *Client) BiliSeg(ctx context.Context, cid int64, segment int) ([]danmupb.Comment, error) {
	endpoint := fmt.Sprintf(biliSegURL, cid, segment)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaPC)
	req.Header.Set("Referer", "https://www.bilibili.com/")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bili seg status %d", resp.StatusCode)
	}
	return danmupb.ParseSeg(body)
}

// ParseEpisodeNo 从剧集标题提取集号（评审 7.3：标题格式混乱，需统一提取）。
//
// 支持形态：`第3集` / `第 03 话` / `EP3` / `ep 03` / 纯数字 / 中文数字（一~九九九）/ 罗马数字（I~X）。
// 电影"正片/预告片"等无集号标题返回 0（调用方按"单集片"处理）；解析不出返回 0。
func ParseEpisodeNo(title string) int {
	t := strings.TrimSpace(title)
	if t == "" {
		return 0
	}
	// 第X集/话/回 与 EPx / Epx
	if m := zhEpRe.FindStringSubmatch(t); len(m) == 2 {
		return cnNum(m[1])
	}
	if m := epRe.FindStringSubmatch(t); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	// 纯数字 / 数字开头
	if n, err := strconv.Atoi(t); err == nil {
		return n
	}
	// 中文数字
	if n := cnNum(t); n > 0 {
		return n
	}
	// 罗马数字（整串）
	if n := romanNum(t); n > 0 {
		return n
	}
	return 0
}

var (
	zhEpRe = regexp.MustCompile(`第\s*([0-9一二三四五六七八九十百千零两]+)\s*[集话回]`)
	epRe   = regexp.MustCompile(`(?i)^ep\s*0*(\d+)$`)
)

var cnDigits = map[rune]int{
	'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
	'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
}

// cnNum 中文数字 → 阿拉伯数字（支持 1~9999：一二三…九 + 十/百/千 位；兼容 ASCII 数字混排，
// 如"第03集"的 03）。
func cnNum(s string) int {
	total := 0
	section := 0
	for _, r := range s {
		switch {
		case r == '十':
			if section == 0 {
				section = 1
			}
			total += section * 10
			section = 0
		case r == '百':
			total += section * 100
			section = 0
		case r == '千':
			total += section * 1000
			section = 0
		case r == '零':
			continue
		default:
			if d, ok := cnDigits[r]; ok {
				section = section*10 + d
			} else if r >= '0' && r <= '9' {
				section = section*10 + int(r-'0')
			} else {
				return 0 // 含非数字字符，按无法解析处理
			}
		}
	}
	return total + section
}

// romanNum 罗马数字（仅支持单字符 I~X 组合，足够剧集号场景）→ 阿拉伯数字；不支持返回 0。
func romanNum(s string) int {
	roman := map[rune]int{'I': 1, 'V': 5, 'X': 10}
	total := 0
	prev := 0
	for i := len(s) - 1; i >= 0; i-- {
		v, ok := roman[rune(s[i])]
		if !ok {
			return 0
		}
		if v < prev {
			total -= v
		} else {
			total += v
			prev = v
		}
	}
	return total
}
