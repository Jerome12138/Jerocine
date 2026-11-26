package douban

// 手动"按片名取豆瓣 ID + 评分"（2026-11-25）
//
// 与榜单抓取（collection.go）不同：这里走**搜索建议 + 详情**两个公开接口，单次人工触发、低频，
// 所以不做翻页与重试风暴，但保留客户端的限速与 UA/Referer（豆瓣对 Referer 敏感）。
//
// 为什么分两步：suggest 只给 id/标题/年份，**不带评分**；评分要再取一次详情。
// 类型上 movie 与 tv 共用 rexxar 接口（会 301 到 /tv/），客户端默认跟随重定向即可。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	suggestEndpoint = "https://movie.douban.com/j/subject_suggest"
	detailEndpoint  = "https://m.douban.com/rexxar/api/v2/movie/"
)

// Suggestion 一条搜索建议
type Suggestion struct {
	Id    int64  `json:"id"`
	Title string `json:"title"`
	Year  int    `json:"year"`
	Type  string `json:"type"` // movie / tv
}

// Suggest 按片名取候选（豆瓣建议接口，最多约 10 条）。
func (c *Client) Suggest(ctx context.Context, name string) ([]Suggestion, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("片名为空")
	}
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	u := suggestEndpoint + "?q=" + url.QueryEscape(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://movie.douban.com/")

	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("豆瓣建议接口 HTTP %d", res.StatusCode)
	}

	var raw []struct {
		Id    json.RawMessage `json:"id"`
		Title string          `json:"title"`
		Year  string          `json:"year"`
		Type  string          `json:"type"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析建议失败: %w", err)
	}
	out := make([]Suggestion, 0, len(raw))
	for _, it := range raw {
		id := flexInt(it.Id)
		if id <= 0 {
			continue
		}
		year, _ := strconv.Atoi(strings.TrimSpace(it.Year))
		out = append(out, Suggestion{Id: id, Title: strings.TrimSpace(it.Title), Year: year, Type: it.Type})
	}
	return out, nil
}

// SubjectRating 取某条目的评分与评价人数（0 表示暂无评分）。
func (c *Client) SubjectRating(ctx context.Context, id int64) (float64, int, error) {
	if id <= 0 {
		return 0, 0, fmt.Errorf("豆瓣 id 无效")
	}
	if err := c.wait(ctx); err != nil {
		return 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailEndpoint+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://m.douban.com/")

	res, err := c.hc.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("豆瓣详情接口 HTTP %d", res.StatusCode)
	}
	var d struct {
		Rating Rating `json:"rating"`
	}
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		return 0, 0, fmt.Errorf("解析详情失败: %w", err)
	}
	return d.Rating.Value, d.Rating.Count, nil
}

// PickSuggestion 从候选里挑最可信的一条（宁缺勿错）：
//   - 片名必须**归一化后完全一致**，或"包含片名且年份差值 ≤1"；
//   - 年份都有值时要求 |差| ≤ 1（跨年播出的剧集很常见）；
//   - 同分时 movie/tv 优先于其它类型。
func PickSuggestion(list []Suggestion, name string, year int) (Suggestion, bool) {
	want := normalizeTitle(name)
	if want == "" {
		return Suggestion{}, false
	}
	best := Suggestion{}
	bestScore := -1
	for _, it := range list {
		if it.Id <= 0 {
			continue
		}
		got := normalizeTitle(it.Title)
		if got == "" {
			continue
		}
		exact := got == want
		if !exact && !strings.Contains(got, want) && !strings.Contains(want, got) {
			continue
		}
		yearDiff := 999
		if year > 0 && it.Year > 0 {
			yearDiff = absInt(it.Year - year)
		}
		if !exact && yearDiff > 1 {
			continue // 只是"名字包含"，必须年份对得上才算
		}
		if exact && year > 0 && it.Year > 0 && yearDiff > 1 {
			continue // 同名不同年（重启/翻拍）也算错配
		}
		score := 0
		if exact {
			score += 10
		}
		if yearDiff <= 1 {
			score += 5
		} else if yearDiff == 999 {
			score += 1
		}
		if it.Type == "movie" || it.Type == "tv" {
			score += 2
		}
		if score > bestScore {
			best, bestScore = it, score
		}
	}
	if bestScore < 0 {
		return Suggestion{}, false
	}
	return best, true
}

func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "　", "")
	s = strings.ReplaceAll(s, "：", ":")
	s = strings.ReplaceAll(s, "·", "")
	return strings.ToLower(s)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
