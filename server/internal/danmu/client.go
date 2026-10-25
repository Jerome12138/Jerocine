// Package danmu 弹幕数据客户端（D2 收敛后：仅保留豆瓣 vendors 解析）。
//
// 链路：movie.db_id → 豆瓣 rexxar vendors（正版播放渠道，app scheme）→ 平台资源 id。
// 分集映射/弹幕拉取自 D2 起全部交给 danmu-service（Node 壳，复用上游 scraper-kit）：
// Go 侧不再直连 B站 season/seg.so（client.go/proto/* 已于 D2 删除）。
// 请求纪律同 douban/collection.go：固定 UA + Referer；vendors 接口 POC 实测无限流，
// 但保守起见仍带 1 次重试。
package danmu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// doubanVendorsURL 豆瓣正版播放渠道（vendors）。POC 实测：需 Referer 指向该 subject 页。
	doubanVendorsURL = "https://m.douban.com/rexxar/api/v2/movie/%s?for_mobile=1"
	doubanReferer    = "https://m.douban.com/movie/subject/%s/?dt_dapp=1"

	uaMobile = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
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

// Client 弹幕数据客户端（D2：豆瓣 vendors 解析；弹幕取数走 danmu-service）。
type Client struct {
	hc *http.Client
}

// New 构造客户端。
func New() *Client {
	return &Client{hc: &http.Client{Timeout: 20 * time.Second}}
}

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
			ID   string `json:"id"`
			IsAd bool   `json:"is_ad"`
			URI  string `json:"uri"`
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
