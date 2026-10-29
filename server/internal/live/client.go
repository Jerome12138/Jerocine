package live

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client 直播源拉取客户端（m3u 播放列表文本）。
type Client struct {
	hc *http.Client
}

// NewClient 构造。清单文件小（几十 KB 级）+ 源站响应慢是常态，给 20s。
func NewClient() *Client {
	return &Client{hc: &http.Client{Timeout: 20 * time.Second}}
}

// Fetch 拉取 m3u 文本（body 上限 16MB，防异常源拖垮内存）。
func (c *Client) Fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// 部分直播源站校验 UA，伪装常规播放器 UA 提高兼容面
	req.Header.Set("User-Agent", "Lavf/60.16.100")
	req.Header.Set("Accept", "*/*")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("live playlist status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
