// Package tmdb TMDB(The Movie Database)检索客户端 —— 按片名(+年份)取影片横图(backdrop)。
//
// 背景: 采集源(MacCMS)的 vod_pic_slide 填充率≈0%, 上游 GoFilm 也不消费; 横图只能自建。
// 服务器直连 api.themoviedb.org 可达; 图片 image.tmdb.org 在大陆被墙 —— 所以只把
// backdrop_path 取回来, 图片本体由后台 worker 经 blobstore 下载到本地存储, 终端用户永远不直连 TMDB。
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MissMark 检索无果哨兵: 写回 movie.backdrop, 避免每轮 worker 都重查同一批查不到的片。
// 对外 DTO 会把它归一为空串, 前端永远见不到这个值。
const MissMark = "-"

// Client TMDB v3/v4 API 客户端。key 可运行期热替换(管理后台维护), 为空时检索调用返回错误
// (功能关闭), 调用方无需判空客户端本体。
type Client struct {
	mu      sync.RWMutex
	apiKey  string // 空串 = 未配置; SetKey 运行期热更新(后台保存即生效)
	lang    string
	baseURL string // 默认 https://api.themoviedb.org/3, 可被配置覆盖(自建反代)
	image   string // 图片 CDN 前缀, 如 https://image.tmdb.org/t/p/
	hc      *http.Client
}

// New 构造客户端。apiKey 可为空(功能待配置), 后续经 SetKey 热更新。
func New(apiKey, lang, apiBase, imageBase string) *Client {
	if lang == "" {
		lang = "zh-CN"
	}
	if apiBase == "" {
		apiBase = "https://api.themoviedb.org/3"
	}
	if imageBase == "" {
		imageBase = "https://image.tmdb.org/t/p/"
	}
	return &Client{
		apiKey:  apiKey,
		lang:    lang,
		baseURL: strings.TrimRight(apiBase, "/"),
		image:   strings.TrimRight(imageBase, "/") + "/",
		hc:      &http.Client{Timeout: 10 * time.Second},
	}
}

// SetKey 运行期替换凭据(空串 = 关闭)。供管理后台保存/清除后热生效, 无需重启。
func (c *Client) SetKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = key
}

// key 当前凭据快照。
func (c *Client) key() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiKey
}

// ImageURL 由 backdrop_path 拼出 w1280 规格(轮播/详情 hero 够用)的图片 URL。
func (c *Client) ImageURL(path string) string {
	if path == "" {
		return ""
	}
	return c.image + "w1280" + path
}

// isJWTToken 判别 v4 Read Access Token(eyJ...三段式 JWT)与 v3 API Key(32 位十六进制)。
func isJWTToken(key string) bool {
	return len(key) > 100 && strings.Count(key, ".") == 2
}

// searchResp 拉平 movie/tv 两类结果。
type searchResp struct {
	Results []struct {
		MediaType     string  `json:"media_type"`
		BackdropPath  *string `json:"backdrop_path"`
		Title         string  `json:"title"`
		Name          string  `json:"name"`
		ReleaseDate   string  `json:"release_date"`  // movie: YYYY-MM-DD
		FirstAirDate  string  `json:"first_air_date"` // tv: YYYY-MM-DD
	} `json:"results"`
}

// SearchBackdrop 按片名(+年份, 0=不限)检索, 返回第一个带 backdrop 的结果路径。
// 先按 movie 搜(年份可精确过滤), 再试 tv, 都没有时再各做一次不带年份的兜底 —— 采集源年份
// 常见错标, 宽松兜底比精确无果更符合"有横图比没有强"的目标。
func (c *Client) SearchBackdrop(ctx context.Context, name string, year int) (string, error) {
	if c.key() == "" {
		return "", fmt.Errorf("tmdb: api key not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("tmdb: empty query")
	}
	for _, withYear := range []bool{true, false} {
		for _, kind := range []string{"movie", "tv"} {
			path, err := c.searchOne(ctx, kind, name, year, withYear)
			if err != nil {
				return "", err
			}
			if path != "" {
				return path, nil
			}
		}
	}
	return MissMark, nil
}

// searchOne 单类目检索; withYear=false 时忽略年份过滤。无命中/无横图返回 ""。
func (c *Client) searchOne(ctx context.Context, kind, name string, year int, withYear bool) (string, error) {
	key := c.key()
	q := url.Values{}
	q.Set("query", name)
	q.Set("language", c.lang)
	q.Set("include_adult", "false")
	q.Set("page", "1")
	if withYear && year > 0 {
		if kind == "movie" {
			q.Set("year", strconv.Itoa(year))
		} else {
			q.Set("first_air_date_year", strconv.Itoa(year))
		}
	}
	// 鉴权二选一: v4 Read Access Token(三段式长 JWT)走 Bearer 头;
	// 常见的 v3 API Key(32 位十六进制)走 api_key 查询参数, 放 Bearer 头会 401。
	if !isJWTToken(key) {
		q.Set("api_key", key)
	}
	endpoint := c.baseURL + "/search/" + kind + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if isJWTToken(key) {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("tmdb: request %s: %w", kind, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		// 免费档限流: 交给调用方限速节奏消化, 不当致命错误
		log.Printf("tmdb: 429 rate limited on %s search", kind)
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tmdb: %s search status %d", kind, resp.StatusCode)
	}
	var out searchResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("tmdb: decode %s: %w", kind, err)
	}
	for _, r := range out.Results {
		if r.BackdropPath != nil && *r.BackdropPath != "" {
			return *r.BackdropPath, nil
		}
	}
	return "", nil
}

// authResp /authentication 的响应(只关心 success)。
type authResp struct {
	Success bool `json:"success"`
}

// Verify 校验候选凭据是否有效(v3 key / v4 token 均可), 供管理后台保存前验真,
// 避免存进一个错 key 之后横图静默不回填。网络错误原样返回, 由调用方提示重试。
func (c *Client) Verify(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("tmdb: empty key")
	}
	endpoint := c.baseURL + "/authentication"
	if !isJWTToken(key) {
		endpoint += "?" + url.Values{"api_key": {key}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if isJWTToken(key) {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: verify request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("tmdb: key 无效(401), 请检查后重试")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: verify status %d", resp.StatusCode)
	}
	var out authResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("tmdb: decode verify: %w", err)
	}
	if !out.Success {
		return fmt.Errorf("tmdb: key 无效, 请检查后重试")
	}
	return nil
}
