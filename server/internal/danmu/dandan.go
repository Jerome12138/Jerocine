package danmu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DandanClient 聚合弹幕服务（dandanplay v2 兼容协议）客户端。
// 与 scraper-kit 链路（Client → danmu-service 壳）相互独立：本客户端按标题搜索
// 匹配剧集，不依赖豆瓣 vendors。鉴权为路径前缀 /{token}/api/v2/*。
type DandanClient struct {
	base  string // 形如 http://danmu_api:9321（无尾斜杠）
	token string
	hc    *http.Client
}

// NewDandan 构造聚合弹幕客户端。base 为服务基址，token 为访问令牌（空 = 未配置）。
func NewDandan(base, token string) *DandanClient {
	return &DandanClient{
		base:  strings.TrimRight(base, "/"),
		token: token,
		hc:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Enabled 未配置基址/令牌时为 false（上层据此静默降级为无弹幕）。
func (c *DandanClient) Enabled() bool { return c.base != "" && c.token != "" }

// dandanResp dandanplay v2 响应公共头。
type dandanResp struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"errorMessage"`
}

// DandanAnime 搜索命中的一部作品。
type DandanAnime struct {
	AnimeID      int64  `json:"animeId"`
	AnimeTitle   string `json:"animeTitle"`
	EpisodeCount int    `json:"episodeCount"`
}

// DandanEpisode 一个可取弹幕的分集。
type DandanEpisode struct {
	EpisodeID     int64  `json:"episodeId"`
	EpisodeTitle  string `json:"episodeTitle"`
	EpisodeNumber string `json:"episodeNumber"`
}

// searchResp GET /api/v2/search/anime。
type searchResp struct {
	dandanResp
	Animes []DandanAnime `json:"animes"`
}

// bangumiResp GET /api/v2/bangumi/{animeId}。
type bangumiResp struct {
	dandanResp
	Bangumi struct {
		Episodes []DandanEpisode `json:"episodes"`
	} `json:"bangumi"`
}

// DandanRawComment 评论原始条目（p = "时间秒,模式,颜色,来源"）。
type DandanRawComment struct {
	P string `json:"p"`
	M string `json:"m"`
}

// commentResp GET /api/v2/comment/{episodeId}。
type commentResp struct {
	dandanResp
	Count    int                `json:"count"`
	Comments []DandanRawComment `json:"comments"`
}

// get 统一请求：token 路径前缀 + JSON 解码（body 上限 48MB —— 整集聚合弹幕实测 4-5MB）。
// transport 错误统一脱敏（url.Error 的 Error() 会内嵌完整 URL 含 token，绝不能透传出去）。
func (c *DandanClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/"+url.PathEscape(c.token)+path, nil)
	if err != nil {
		// 构造失败(parse error)的 err 内嵌完整 URL 含 token, 一并脱敏
		return fmt.Errorf("danmu aggregate request failed: %w", errors.Unwrap(err))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		// 只保留底层原因（超时/连接拒绝等），剥离含 token 的 URL
		return fmt.Errorf("danmu aggregate request failed: %w", errors.Unwrap(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 48<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("danmu aggregate status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("danmu aggregate 响应非 JSON: %w", err)
	}
	return nil
}

// SearchAnime 按关键词搜索作品（服务端对空结果也返回 success=true）。
func (c *DandanClient) SearchAnime(ctx context.Context, keyword string) ([]DandanAnime, error) {
	var out searchResp
	err := c.get(ctx, "/api/v2/search/anime?keyword="+url.QueryEscape(keyword), &out)
	if err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("danmu aggregate search 失败: %s", out.ErrorMessage)
	}
	return out.Animes, nil
}

// Bangumi 取作品分集列表。
func (c *DandanClient) Bangumi(ctx context.Context, animeID int64) ([]DandanEpisode, error) {
	var out bangumiResp
	err := c.get(ctx, "/api/v2/bangumi/"+strconv.FormatInt(animeID, 10), &out)
	if err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("danmu aggregate bangumi 失败: %s", out.ErrorMessage)
	}
	return out.Bangumi.Episodes, nil
}

// Comments 取单集弹幕（withRelated 聚合关联源；chConvert=1 简体）。
func (c *DandanClient) Comments(ctx context.Context, episodeID int64) ([]DandanRawComment, error) {
	var out commentResp
	err := c.get(ctx, "/api/v2/comment/"+strconv.FormatInt(episodeID, 10)+"?withRelated=true&chConvert=1", &out)
	if err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, fmt.Errorf("danmu aggregate comment 失败: %s", out.ErrorMessage)
	}
	return out.Comments, nil
}

// NormalizeTitle 标题归一（匹配打分用）：去空白 + 小写。
func NormalizeTitle(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s))
}

// PickAnime 从搜索结果里选目标作品（确定性打分，无命中返回 nil）：
//   - 标题包含关键词（归一后）才算候选；
//   - 候选内加分：标题含年份 +2、集数 >= episode +1；同分取靠前（服务端排序稳定）。
func PickAnime(animes []DandanAnime, keyword string, year, episode int) *DandanAnime {
	kw := NormalizeTitle(keyword)
	if kw == "" {
		return nil
	}
	var best *DandanAnime
	bestScore := -1
	for i := range animes {
		a := &animes[i]
		title := NormalizeTitle(a.AnimeTitle)
		if !strings.Contains(title, kw) {
			continue
		}
		score := 0
		if year > 0 && strings.Contains(title, strconv.Itoa(year)) {
			score += 2
		}
		if episode > 0 && (a.EpisodeCount == 0 || a.EpisodeCount >= episode) {
			score++
		}
		if score > bestScore {
			best, bestScore = a, score
		}
	}
	return best
}

// PickEpisode 从分集列表里定位目标集：
//   - episodeNumber 精确等于集号（服务端给的是字符串数字）；
//   - 回退标题「第N集/话/期」或纯数字段；
//   - 再回退按 1 起下标。
//
// 全部未命中返回 nil（调用方标 empty）。
func PickEpisode(eps []DandanEpisode, episode int) *DandanEpisode {
	if episode <= 0 || len(eps) == 0 {
		return nil
	}
	want := strconv.Itoa(episode)
	for i := range eps {
		if eps[i].EpisodeNumber == want {
			return &eps[i]
		}
	}
	for i := range eps {
		if matchEpisodeNo(eps[i].EpisodeTitle, episode) {
			return &eps[i]
		}
	}
	if episode <= len(eps) {
		return &eps[episode-1]
	}
	return nil
}

// matchEpisodeNo 标题集号匹配：第(\d+)[集话期] / 开头或独立数字段 == episode。
func matchEpisodeNo(title string, episode int) bool {
	t := []rune(title)
	n := len(t)
	for i := 0; i < n; i++ {
		// 「第N…」形态：第 后跟数字，数字段结尾允许 集话期季 任一汉字
		if t[i] == '第' && i+1 < n && t[i+1] >= '0' && t[i+1] <= '9' {
			j := i + 1
			v := 0
			for j < n && t[j] >= '0' && t[j] <= '9' {
				v = v*10 + int(t[j]-'0')
				j++
			}
			if v == episode && j-i <= 6 && j < n {
				switch t[j] {
				case '集', '话', '期':
					return true
				}
			}
			continue
		}
		// 纯数字段（前一位不是数字才起算，避免把长数字串中段误当集号）
		if t[i] >= '0' && t[i] <= '9' && (i == 0 || t[i-1] < '0' || t[i-1] > '9') {
			j := i
			v := 0
			for j < n && t[j] >= '0' && t[j] <= '9' {
				v = v*10 + int(t[j]-'0')
				j++
			}
			// 数字段限长(≤6 位): 超长数字串(时间戳/日期样式)不参与集号判定, 也防溢出回绕
			if j-i <= 6 && v == episode && (j >= n || t[j] == ' ' || t[j] == ']' || t[j] == '】') {
				return true
			}
		}
	}
	return false
}

// DandanComment 规范化弹幕条目（时间毫秒；mode 原样透传，前端统一归一）。
type DandanComment struct {
	Text  string
	Time  int64
	Color uint32
	Mode  int32
}

// ParseDandanComments 把 p="时间秒,模式,颜色,来源" 列表规范化。
// 时间/颜色解析失败的条目跳过。
func ParseDandanComments(raw []DandanRawComment) []DandanComment {
	out := make([]DandanComment, 0, len(raw))
	for _, r := range raw {
		if r.M == "" {
			continue
		}
		parts := strings.SplitN(r.P, ",", 4)
		if len(parts) < 3 {
			continue
		}
		sec, err := strconv.ParseFloat(parts[0], 64)
		if err != nil || sec < 0 {
			continue
		}
		mode, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		color64, err := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 32)
		if err != nil {
			continue
		}
		out = append(out, DandanComment{
			Text:  r.M,
			Time:  int64(sec * 1000),
			Color: uint32(color64),
			Mode:  int32(mode),
		})
	}
	return out
}
