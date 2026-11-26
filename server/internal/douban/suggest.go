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
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	suggestEndpoint = "https://movie.douban.com/j/subject_suggest"
	detailEndpoint  = "https://m.douban.com/rexxar/api/v2/movie/"
	// abstractEndpoint 详情兜底/首选：**rexxar 详情接口对服务器 IP 会整段封禁**
	// （2026-11-26 实测 400 `subject_ip_rate_limit` code=1309，带随机 bid cookie 也无效），
	// 而 movie.douban.com 域的 j/subject_abstract 同 IP 仍 200，返回 subject.rate（10 分制字符串）。
	abstractEndpoint = "https://movie.douban.com/j/subject_abstract"
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
	// 首选 j/subject_abstract：movie.douban.com 域，实测未见 IP 限流
	score, ok, err := c.ratingFromAbstract(ctx, id)
	switch DecideRatingSource(err) {
	case RatingFromAbstract:
		if !ok {
			// 对端 200 但 rate 为空 = **豆瓣本来就没有评分**（欠账补查的目标全是这类片），
			// 这是"无评分"不是"失败"。**绝不能兜底到 rexxar** —— 它对本机 IP 已封（400），
			// 一兜底就把所有"无评分"变成"失败"，5 次即触发熔断（2026-11-26 实测：5/2000 就中止）。
			return 0, 0, nil
		}
		return score, 0, nil
	}
	return c.ratingFromRexxar(ctx, id)
}

// RatingSource 评分来源决策。
type RatingSource int

const (
	// RatingFromAbstract abstract 可用（200）：得分就用它，rate 空就当"无评分"，不兜底
	RatingFromAbstract RatingSource = iota
	// RatingFromRexxar 只有 abstract 真的失败（网络/非 200）才兜底 rexxar
	RatingFromRexxar
)

// DecideRatingSource 决定是否需要兜底 rexxar（纯函数，便于单测这条关键规则）。
// 只看 abstract 是否可用：**"有没有分"不影响这个决策**（无分就是无分，不该去撞 rexxar）。
func DecideRatingSource(abstractErr error) RatingSource {
	if abstractErr == nil {
		return RatingFromAbstract
	}
	return RatingFromRexxar
}

// ratingFromAbstract 走 j/subject_abstract 取 subject.rate（10 分制，字符串）。
// rate 缺失时**不猜**（star 是 5 分制星级，×2 只是粗略近似）—— 宁缺勿错，返回 ok=false。
func (c *Client) ratingFromAbstract(ctx context.Context, id int64) (float64, bool, error) {
	if err := c.wait(ctx); err != nil {
		return 0, false, err
	}
	u := abstractEndpoint + "?subject_id=" + strconv.FormatInt(id, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://movie.douban.com/")
	res, err := c.hc.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("豆瓣详情接口 HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return 0, false, err
	}
	score, ok := ParseAbstractRate(body)
	return score, ok, nil
}

// ParseAbstractRate 从 j/subject_abstract 的响应里取评分（纯函数，便于单测）。
// 形如 {"r":0,"subject":{"rate":"9.7","star":5.0,...}}；
// r!=0 或 rate 为空/非法 → ok=false（调用方跳过，不写 0 分）。
func ParseAbstractRate(body []byte) (float64, bool) {
	var raw struct {
		R       int `json:"r"`
		Subject struct {
			Rate string  `json:"rate"`
			Star float64 `json:"star"`
		} `json:"subject"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.R != 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw.Subject.Rate), 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// ratingFromRexxar 原有实现（保留作兜底）。
func (c *Client) ratingFromRexxar(ctx context.Context, id int64) (float64, int, error) {
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
