package live

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProbeResult 单频道探源结果。
// Alive = 流可达且响应像直播清单/媒体（m3u8 前缀或 media content-type）；
// CORS = 最终响应带 Access-Control-Allow-Origin（浏览器端能否拉流的判据，
// 与 Alive 相互独立：源活着但无 ACAO → 浏览器跨域拒, 前台置灰"跨域受限"）。
type ProbeResult struct {
	Alive bool `json:"alive"`
	CORS  bool `json:"cors"`
}

// probeTimeout 单频道探源超时: IPTV 流常见慢启动, 8s 兜住绝大多数; 批量并发摊平总耗时。
const probeTimeout = 8 * time.Second

// probeRedirects 跟随重定向上限（部分源 302 → CDN）。
const probeRedirects = 6

// ProbeChannel 探测单频道: 以浏览器视角(带 Origin 头)发 GET, 读响应头 + 前 64 字节判定。
// 只看头部即断开 —— 不拉流体（m3u8 清单几 KB, ts 直链会无限流, 必须 early close）。
func ProbeChannel(ctx context.Context, client *http.Client, url, origin string) ProbeResult {
	cctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{}
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("User-Agent", "Mozilla/5.0 (JerocineLiveProbe/1.0)")

	hc := client
	if hc == nil {
		hc = &http.Client{Timeout: probeTimeout}
	}
	// 探源用独立 transport(限重定向次数), 不复用服务全局 client(避免污染其重定向策略)
	hc = &http.Client{
		Timeout:       probeTimeout,
		Transport:     hc.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= probeRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	resp, err := hc.Do(req)
	if err != nil {
		return ProbeResult{}
	}
	defer func() { _ = resp.Body.Close() }()

	cors := resp.Header.Get("Access-Control-Allow-Origin") != ""
	alive := false
	// m3u8 清单: 200 + #EXTM3U 前缀; ts/其它媒体直链: 200 + media content-type。
	// 只读 64 字节即断开。
	if resp.StatusCode == http.StatusOK {
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		head, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		prefix := strings.TrimSpace(string(head))
		switch {
		case strings.HasPrefix(prefix, "#EXTM3U"):
			alive = true
		case strings.Contains(ct, "mpegurl"),
			strings.Contains(ct, "mp2t"),
			strings.Contains(ct, "video/"),
			strings.Contains(ct, "octet-stream"):
			alive = true
		}
	}
	return ProbeResult{Alive: alive, CORS: cors}
}
