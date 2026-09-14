// Package douban 豆瓣榜单客户端 —— 只做一件事: 抓"当下热门"的有序排行。
//
// # 为什么是 subject_collection
//
// 实测排除了另外两条路: `new_search_subjects?sort=T` 是"综合排序", 本质由评分主导
// (≈ 本地已有的 db_score), 查它零增量; `search_subjects?tag=热门` 与
// `subject_collection/movie_hot_gaia` 是同一份数据, 且不返回 count/year/编导。
// 只有移动站 rexxar 的 subject_collection 一次给出 subject id / title / year /
// release_date / rating(含评价人数) / 编导, 深度也够(热门电影 308 条)。
//
// # 限流的真实形态
//
// 豆瓣限流不是 403, 而是**静默返回空数组**。实测 0.9s 间隔连发约 30 次触发, 75s 内恢复。
// 也就是说"空结果"既可能是"榜到底了", 也可能是"被限流了" —— 混淆两者会把大批热门片
// 错误地判成不热。故这里的判定纪律是: 一次响应必须同时满足 HTTP 200 + JSON 含预期字段
// 才算有效(否则重试一次, 仍失败则跳过该集合并计数); 首页(start=0)返回空**不算有效抓取**,
// 只记 Empty 并跳过该集合 —— 此时多半正被限流, 再打只会加重, 交给调用方按 Empty/Skipped
// 判定"本轮不可信"(见 service/hot_service.go), 绝不把它当成"热度 0"写进库。
package douban

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultInterval 相邻两次请求的最小间隔。实测 3s 间隔可连续拉 8 页无碍, 这里再放宽到 5s
	// —— L1 全部集合单日只有约 18 次请求, 离触发阈值有很大余量。
	DefaultInterval = 5 * time.Second
	// pageSize 单次请求条数。实测 count 传 100 也只返回 50, 故按 50 取。
	pageSize = 50
	// maxPages 单集合翻页上限, 防对端异常时无限翻。
	maxPages = 12
	// maxAttempts 单页尝试次数(首次 + 1 次重试)。
	maxAttempts = 2
	// itemsKey 榜单条目的字段名 —— 结构校验的判据。
	itemsKey = "subject_collection_items"
	// baseURL 移动站 rexxar 榜单接口前缀。
	baseURL = "https://m.douban.com/rexxar/api/v2/subject_collection"
	// referer 必须是 m.douban.com: 实测换其他 Referer 会被拒。
	referer = "https://m.douban.com/"
	// userAgent 与手动验证时一致, 不引入额外变量。
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Collection 一个榜单集合的定义。
type Collection struct {
	Name  string // 豆瓣集合名
	Label string // 中文名(日志展示)
	Depth int    // 实测深度, 用于位次分归一化与"抓全了没"的对照
}

// Collections L1 榜单清单 —— 8 个集合合计约 675 条 / 18 次请求 / 约 1.5 分钟。
//
// 榜单清单集中在这里: 加榜只改本切片, 不动逻辑; 各集合深度是实测值(movie_hot_gaia 308 /
// tv_hot 192 / tv_variety_show 61 / tv_animation 34 / movie_showing 50 / 三个周榜各 10)。
// 曾经设想过的 "按大类 × 年份拉综合榜"(L2) 已证伪: 那拿到的是评分序不是热度, 且本地
// db_score 已覆盖该信息。
var Collections = []Collection{
	{Name: "movie_hot_gaia", Label: "热门电影", Depth: 308},
	{Name: "tv_hot", Label: "热门剧集", Depth: 192},
	{Name: "tv_variety_show", Label: "热门综艺", Depth: 61},
	{Name: "tv_animation", Label: "热门动漫", Depth: 34},
	{Name: "movie_showing", Label: "正在上映", Depth: 50},
	{Name: "movie_weekly_best", Label: "一周口碑榜", Depth: 10},
	{Name: "tv_chinese_best_weekly", Label: "华语口碑剧集榜", Depth: 10},
	{Name: "tv_global_best_weekly", Label: "全球口碑剧集榜", Depth: 10},
}

// Item 榜单条目 —— 只保留排序与匹配需要的字段。
//
// ⚠ 数值字段宽松解析(见 UnmarshalJSON / Rating): 豆瓣不同端点对同一字段会在
// number / string 之间漂移 —— 线上实测 subject_collection_items 的 id 是**字符串**
// (`"id": "35423605"`), 强类型 int64 直接让整页 unmarshal 失败、整轮抓取作废
// (2026-09-12 首次部署即踩中)。原则同 Directors/Actors 的 RawMessage:
// 单字段形态漂移绝不拖垮整页。
type Item struct {
	Id           int64  `json:"id"` // 豆瓣 subject id(与本地 movie.db_id 同源)
	Title        string `json:"title"`
	Year         string `json:"year"`         // 豆瓣用字符串给年份, 可能为空
	ReleaseDate  string `json:"release_date"` // 形如 "09.04"(月.日), 年份在 Year
	CardSubtitle string `json:"card_subtitle"`
	Rating       Rating `json:"rating"`
	// Directors / Actors 用 RawMessage 收: 豆瓣在不同集合给出的形态不完全一致
	// (对象数组 / 字符串数组), 强类型一旦不匹配会让整个响应解析失败、拖垮整轮抓取。
	Directors json.RawMessage `json:"directors"`
	Actors    json.RawMessage `json:"actors"`
}

// UnmarshalJSON 定制 Id 的宽松解析, 其余字段走默认逻辑(经 alias 避开递归)。
func (it *Item) UnmarshalJSON(b []byte) error {
	type alias Item
	var aux struct {
		Id json.RawMessage `json:"id"`
		*alias
	}
	aux.alias = (*alias)(it)
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	it.Id = flexInt(aux.Id)
	return nil
}

// Rating 豆瓣评分块 —— count/value 同样按"可能是字符串"宽松解析。
type Rating struct {
	Count int     `json:"count"` // 评价人数(真实片级热度标量, 可与位次分互相印证)
	Value float64 `json:"value"`
}

// UnmarshalJSON 逐字段宽松解析: 单个评分字段形态漂移只损失该字段, 不拖垮整页。
func (r *Rating) UnmarshalJSON(b []byte) error {
	var raw struct {
		Count json.RawMessage `json:"count"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	r.Count = int(flexFloat(raw.Count))
	r.Value = flexFloat(raw.Value)
	return nil
}

// flexInt 宽松整数解析: 接受 123 与 "123"; 空/null/解析失败一律 0(缺省语义)。
func flexInt(raw json.RawMessage) int64 {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// flexFloat 宽松浮点解析: 接受 8.5 与 "8.5"; 空/null/解析失败一律 0。
func flexFloat(raw json.RawMessage) float64 {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// YearInt 年份整数; 明显不合理或无法解析时返回 0。
func (it Item) YearInt() int {
	n, err := strconv.Atoi(strings.TrimSpace(it.Year))
	if err != nil || n < 1900 || n > 2200 {
		return 0
	}
	return n
}

// Names 条目里的导演 + 主演姓名(供同名片的二次校验)。解析不出返回 nil, 不报错。
func (it Item) Names() []string {
	out := parsePeople(it.Directors)
	out = append(out, parsePeople(it.Actors)...)
	return out
}

// parsePeople 宽松解析豆瓣的人员字段, 兼容 [{"name":"…"}] 与 ["…"] 两种形态。
func parsePeople(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var objs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &objs); err == nil {
		out := make([]string, 0, len(objs))
		for _, o := range objs {
			if n := strings.TrimSpace(o.Name); n != "" {
				out = append(out, n)
			}
		}
		return out
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		return strs
	}
	return nil
}

// Ranked 带榜位与来源的条目。
type Ranked struct {
	Item
	Collection string // 来源集合名
	Label      string // 来源集合中文名
	Rank       int    // 在该集合内的位次(1 起)
	Depth      int    // 该集合深度 —— 供 domain.HotScore 归一化位次分
}

// 位次分不在这里算: 本包只负责"取到榜单、给定位次", 折算成分数属领域计算,
// 唯一实现在 domain.HotScore(内部按 0.1 分单位换算, 避免公式分叉)。

// BoardLabel 集合名 → 中文榜单名(详情页展示"豆瓣·热门电影 No.2"用)。
// 未知集合名返回原名 —— 库里可能存着清单下线前的历史名, 不至于展示成空白。
func BoardLabel(name string) string {
	for _, c := range Collections {
		if c.Name == name {
			return c.Label
		}
	}
	return name
}

// pidClassHotBoard 一级分类 → 该分类对应的"分类热榜"集合名。
// pid 口径同 nav 表: 1 电影片 / 2 连续剧 / 3 综艺片 / 4 动漫片。
// 这里只写集合名, 中文名一律走 BoardLabel(Collections) —— 榜单名有唯一来源, 不重复写死。
var pidClassHotBoard = map[int64]string{
	1: "movie_hot_gaia",  // 电影片 → 热门电影
	2: "tv_hot",          // 连续剧 → 热门剧集
	3: "tv_variety_show", // 综艺片 → 热门综艺
	4: "tv_animation",    // 动漫片 → 热门动漫
}

// ClassHotBoardLabel 按一级分类取"分类热榜"的中文名(pid=4 → "热门动漫")。取不到返回空串。
func ClassHotBoardLabel(pid int64) string {
	if name, ok := pidClassHotBoard[pid]; ok {
		return BoardLabel(name)
	}
	return ""
}

// HotBoardLabel 对外展示的榜单名: 优先用落库的 hot_board, 缺失但有位次时按分类热榜兜底。
//
// 为什么需要兜底: hot_board 正常与 hot_rank 同批落库(hot_service 全量替换), 但历史行 /
// 异常行可能只有位次没有榜名 —— 那时前台只剩"热门"两个字, 到底是电影/剧集/综艺/动漫的
// 热榜全丢了。用户明确要求"要显示全, 比如热门动漫, 不要只显示热门"。
//
// 注意这是**兜底猜测**: 4 个分类热榜占榜单条目约 88%, 其余来自口碑榜 / 正在上映, 那些行若缺
// 榜名会被归到分类热榜上。要精确只能靠采集侧把 hot_board 写全。
func HotBoardLabel(pid int64, raw string, rank int) string {
	if l := BoardLabel(raw); l != "" {
		return l
	}
	if rank <= 0 {
		return "" // 不在榜就没有"哪个榜"的问题, 不要凭分类硬造一个
	}
	return ClassHotBoardLabel(pid)
}

// Stats 一轮抓取的观测值 —— 供日志与后台排查(抓了多少页 / 空返回几次 / 跳过几页)。
type Stats struct {
	Pages   int           // 有效响应页数
	Empty   int           // 首页空返回次数(限流或异常, 已重试)
	Skipped int           // 重试用尽仍失败而跳过的页数
	Items   int           // 收到的条目总数
	Elapsed time.Duration // 总耗时
}

// Client 豆瓣榜单客户端: 单线程、内置限速、无需 key。
type Client struct {
	hc       *http.Client
	interval time.Duration
	base     string // 接口前缀; 默认 baseURL, 单测用 SetBaseURL 指向 httptest

	mu   sync.Mutex
	last time.Time // 上次请求发起时间(限速锚点)

	stats Stats // 由 Fetch 写入; 调用方串行使用
}

// New 构造客户端(限速取 DefaultInterval)。
func New() *Client {
	return &Client{
		hc:       &http.Client{Timeout: 20 * time.Second},
		interval: DefaultInterval,
		base:     baseURL,
	}
}

// SetInterval 覆盖限速间隔(测试与手工排障用)。
func (c *Client) SetInterval(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interval = d
}

// SetBaseURL 覆盖接口前缀(测试与手工排障用; 传空串忽略)。
func (c *Client) SetBaseURL(u string) {
	if u == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = u
}

// endpoint 当前接口前缀(锁内读, 与 SetBaseURL 并发安全)。
func (c *Client) endpoint() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base
}

// Stats 最近一轮抓取的观测值。
func (c *Client) Stats() Stats { return c.stats }

// Fetch 抓取全部集合, 返回带榜位的扁平列表 —— 同一部片命中多个集合会保留多条,
// 由调用方取最好位次。
//
// 单个集合失败只跳过该集合(记日志 + 计数), 绝不让整轮失败: 排序宁可少几条榜内命中,
// 也不能因为一个集合抖动就整体退化成兜底分。
func (c *Client) Fetch(ctx context.Context, cols []Collection) ([]Ranked, error) {
	start := time.Now()
	c.stats = Stats{}
	var out []Ranked

	for _, col := range cols {
		if ctx.Err() != nil {
			break
		}
		rank := 0
		for page := 0; page < maxPages; page++ {
			items, err := c.fetchPage(ctx, col.Name, page*pageSize)
			if err != nil {
				log.Printf("[douban] %s(%s) start=%d 抓取失败, 跳过该集合: %v",
					col.Label, col.Name, page*pageSize, err)
				c.stats.Skipped++
				break
			}
			if len(items) == 0 {
				// 有效响应但 0 条: 首页为空说明对端拿不到数据(限流/异常), 后续页为空才是正常到底。
				if page == 0 {
					c.stats.Empty++
				}
				break
			}
			c.stats.Pages++
			for _, it := range items {
				rank++
				out = append(out, Ranked{
					Item: it, Collection: col.Name, Label: col.Label, Rank: rank, Depth: col.Depth,
				})
			}
			c.stats.Items += len(items)
			if len(items) < pageSize {
				break // 不满一页 → 已到底
			}
		}
	}

	c.stats.Elapsed = time.Since(start)
	return out, nil
}

// fetchPage 取一页, 内含"有效性判定 + 重试一次"。
// 返回 (条目, nil) 且条目为空 = 该页确实到底; 返回 error = 重试用尽仍无效。
func (c *Client) fetchPage(ctx context.Context, name string, start int) ([]Item, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		items, valid, err := c.doFetchPage(ctx, name, start)
		if err == nil && valid {
			return items, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = errors.New("响应结构异常(缺少 " + itemsKey + ")")
		}
		if attempt+1 < maxAttempts && !sleepCtx(ctx, c.interval) {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

// doFetchPage 单次请求。第二个返回值为"结构是否有效" —— 只有它成立, 空数组才能被解释成"到底了"。
func (c *Client) doFetchPage(ctx context.Context, name string, start int) ([]Item, bool, error) {
	if err := c.wait(ctx); err != nil {
		return nil, false, err
	}
	endpoint := fmt.Sprintf("%s/%s/items?start=%d&count=%d&items_only=1",
		c.endpoint(), url.PathEscape(name), start, pageSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, false, err
	}

	// 结构校验先行: 必须确认 items 字段存在, 才能把"空数组"解释成"到底了"。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, false, fmt.Errorf("响应非 JSON: %w", err)
	}
	blob, ok := raw[itemsKey]
	if !ok {
		return nil, false, errors.New("响应缺少 " + itemsKey + " 字段")
	}
	var items []Item
	if err := json.Unmarshal(blob, &items); err != nil {
		return nil, false, fmt.Errorf("解析 %s: %w", itemsKey, err)
	}
	return items, true, nil
}

// wait 保证相邻两次请求间隔不小于 interval(以请求发起时间为锚点)。
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	gap := c.interval - time.Since(c.last)
	c.mu.Unlock()
	if gap > 0 && !sleepCtx(ctx, gap) {
		return ctx.Err()
	}
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
