package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/platform/safehttp"
)

// M3u8Service 代理 m3u8 播放列表: SSRF 防护 + 相对地址绝对化 + 跨域广告分片剔除 + 结果缓存。
type M3u8Service struct {
	client       *http.Client
	streamClient *http.Client
	streamKey    []byte
	streamKeyID  string
}

func NewM3u8Service(signingMaterial []byte) *M3u8Service {
	// 防 SSRF 客户端: dial 期拦内网 IP, 覆盖重定向跳转与 DNS rebinding。
	h := sha256.New()
	_, _ = h.Write([]byte("jerocine-m3u8-stream-v1\x00"))
	_, _ = h.Write(signingMaterial)
	key := h.Sum(nil)
	return &M3u8Service{
		client:       safehttp.NewClient(10 * time.Second),
		streamClient: safehttp.NewClient(0),
		streamKey:    key,
		streamKeyID:  hex.EncodeToString(key[:6]),
	}
}

const m3u8MaxBytes = 5 << 20

// statsMaxDepth 限定 stats 下钻层数(master→变体→媒体列表), 防恶意嵌套放大回源。
const statsMaxDepth = 2

// M3u8Stats 广告过滤统计: Total=本播放链路分片总数, Filtered=被判为广告剔除的分片数。
// Cross/Num/HasDisc/Sample 仅供排查日志用, json:"-" 不序列化(不入缓存 JSON、不出 API)。
type M3u8Stats struct {
	Filtered int      `json:"filtered"`
	Total    int      `json:"total"`
	Cross    int      `json:"-"` // 跨域 host 命中剔除数
	Num      int      `json:"-"` // 编号跳脱命中剔除数
	HasDisc  bool     `json:"-"` // 列表是否含 #EXT-X-DISCONTINUITY(广告常用插片标记)
	Sample   []string `json:"-"` // DISCONTINUITY 后首个分片 URI 样本(疑似漏过滤时定位广告形态)
}

// m3u8SampleMax 排查样本上限: 只留前若干个"插片标记后分片"URI, 够定位广告形态即可。
const m3u8SampleMax = 5

// m3u8Cached 缓存载荷: 代理输出 + 过滤统计 + 第一个子列表原始 URL(供 stats 下钻),
// 一份缓存同时服务 Proxy 与 Stats, 避免二者各自回源。
type m3u8Cached struct {
	Content    string `json:"c"`
	Filtered   int    `json:"f"`
	Total      int    `json:"t"`
	FirstChild string `json:"fc"`
}

// parseAndGuard 解析 + SSRF 防护, Proxy / Stats 共用。
func parseAndGuard(rawURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, domain.ErrInvalidArgument
	}
	if err := guardSSRF(u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

// load 回源 + 重写 + 统计, 结果以 JSON 缓存 10min(Proxy / Stats 共享)。
func (s *M3u8Service) load(ctx context.Context, rawURL string, u *url.URL, filterAds, proxyMedia bool) (m3u8Cached, error) {
	// 缓存版本号: 过滤逻辑变更需 bump。v4=签名媒体转发，key id 防密钥轮换后命中旧签名。
	cacheKey := fmt.Sprintf("m3u8:v4:%s:%s:%t:%t", s.streamKeyID, sha1hex(rawURL), filterAds, proxyMedia)
	blob, _, err := cache.GetOrLoad(ctx, cacheKey, 10*time.Minute, func(ctx context.Context) (string, bool, error) {
		body, err := s.fetch(ctx, rawURL)
		if err != nil {
			return "", false, err
		}
		content, st, firstChild := rewriteM3u8WithStats(body, u, filterAds, true)
		if proxyMedia {
			content, st, firstChild = s.rewriteM3u8WithProxy(body, u, filterAds, true)
		}
		if filterAds {
			logM3u8Filter("server", rawURL, st)
		}
		b, _ := json.Marshal(m3u8Cached{Content: content, Filtered: st.Filtered, Total: st.Total, FirstChild: firstChild})
		return string(b), true, nil
	})
	if err != nil {
		return m3u8Cached{}, err
	}
	var res m3u8Cached
	if e := json.Unmarshal([]byte(blob), &res); e != nil {
		// 兼容意外的非 JSON 旧缓存: 当作纯 m3u8 文本, 统计为 0。
		return m3u8Cached{Content: blob}, nil
	}
	return res, nil
}

// Proxy 拉取并处理 m3u8。filterAds=true 时剔除跨域(广告 CDN)分片。
func (s *M3u8Service) Proxy(ctx context.Context, rawURL string, filterAds, proxyMedia bool) (string, error) {
	u, err := parseAndGuard(rawURL)
	if err != nil {
		return "", err
	}
	res, err := s.load(ctx, rawURL, u, filterAds, proxyMedia)
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// Stats 返回该 m3u8 的广告过滤统计。master/变体列表本身无分片(Total=0)时,
// 下钻第一个子播放列表聚合其分片统计(限 statsMaxDepth 层), 让"播放链路实际过滤数"可见。
func (s *M3u8Service) Stats(ctx context.Context, rawURL string, filterAds bool) (M3u8Stats, error) {
	return s.statsRec(ctx, rawURL, filterAds, 0)
}

func (s *M3u8Service) statsRec(ctx context.Context, rawURL string, filterAds bool, depth int) (M3u8Stats, error) {
	if depth > statsMaxDepth {
		return M3u8Stats{}, nil
	}
	u, err := parseAndGuard(rawURL)
	if err != nil {
		return M3u8Stats{}, err
	}
	res, err := s.load(ctx, rawURL, u, filterAds, false)
	if err != nil {
		return M3u8Stats{}, err
	}
	st := M3u8Stats{Filtered: res.Filtered, Total: res.Total}
	// 本层无直接分片但有子列表 → 下钻第一个变体(播放器实际会选其一)聚合统计。
	if res.Total == 0 && res.FirstChild != "" {
		if child, e := s.statsRec(ctx, res.FirstChild, filterAds, depth+1); e == nil {
			st.Filtered += child.Filtered
			st.Total += child.Total
		}
	}
	return st, nil
}

func (s *M3u8Service) fetch(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", domain.ErrInvalidArgument
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("m3u8: upstream status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, m3u8MaxBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

const streamURLTTL = 12 * time.Hour

func (s *M3u8Service) signStream(rawURL string, exp int64) string {
	mac := hmac.New(sha256.New, s.streamKey)
	_, _ = fmt.Fprintf(mac, "%d\n%s", exp, rawURL)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyStream validates a server-generated media URL without exposing an open bandwidth proxy.
func (s *M3u8Service) VerifyStream(rawURL, expRaw, sig string) bool {
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || exp < time.Now().Unix() || rawURL == "" || sig == "" {
		return false
	}
	want := s.signStream(rawURL, exp)
	return hmac.Equal([]byte(want), []byte(sig))
}

func (s *M3u8Service) streamWrap(rawURL string) string {
	exp := time.Now().Add(streamURLTTL).Unix()
	q := url.Values{}
	q.Set("src", rawURL)
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", s.signStream(rawURL, exp))
	return "/api/v1/m3u8/stream?" + q.Encode()
}

// OpenStream relays a signed HLS segment, initialization map, or encryption key.
func (s *M3u8Service) OpenStream(ctx context.Context, rawURL, exp, sig, byteRange string) (*http.Response, error) {
	if !s.VerifyStream(rawURL, exp, sig) {
		return nil, domain.ErrForbidden
	}
	if _, err := parseAndGuard(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	req.Header.Set("User-Agent", "Jerocine-Media-Proxy/1.0")
	resp, err := s.streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("m3u8 stream: upstream status %d", resp.StatusCode)
	}
	return resp, nil
}

// guardSSRF 早期拒绝解析到内网/环回/链路本地的目标(dial 期 Control 是最终防线)。
func guardSSRF(host string) error {
	if err := safehttp.GuardHost(host); err != nil {
		return domain.ErrForbidden
	}
	return nil
}

// m3u8ProxyPath 本服务的代理路由(须与 router 注册一致)。
// 子播放列表回环代理用根相对路径, 由播放器按 manifest 所在 origin 解析, 适配 openresty 反代/任意域名。
const m3u8ProxyPath = "/api/v1/m3u8/proxy"

// rewriteM3u8 见 rewriteM3u8WithStats; 仅取重写后的文本(调用方不关心统计时)。
func rewriteM3u8(body string, base *url.URL, filterAds bool) string {
	out, _, _ := rewriteM3u8WithStats(body, base, filterAds, true)
	return out
}

// FilterText 端侧混合过滤: 对客户端送来的 m3u8 文本剔广告 + 绝对化(子列表/分片均绝对、不改写代理),
// 不联网抓源(srcURL 仅作相对地址解析基准)。供服务器抓不到的源(如 bf 地域封)在设备侧抓流后送来过滤。
func (s *M3u8Service) FilterText(content, srcURL string) (string, M3u8Stats, error) {
	base, err := url.Parse(srcURL)
	if err != nil || base.Host == "" {
		return "", M3u8Stats{}, domain.ErrInvalidArgument
	}
	out, st, _ := rewriteM3u8WithStats(content, base, true, false)
	logM3u8Filter("client", srcURL, st)
	return out, st, nil
}

// logM3u8Filter 过滤排查日志(stdout, 进 docker logs film_api)。
// 仅媒体列表(有分片)打印, master/空表(Total=0)跳过降噪;
// filtered==0 且有插片标记(#EXT-X-DISCONTINUITY)→ WARN「疑似漏过滤」+ 分片样本, 便于定位是哪部片哪集、广告何种形态。
func logM3u8Filter(via, srcURL string, st M3u8Stats) {
	if st.Total == 0 {
		return
	}
	host, path := "", srcURL
	if u, err := url.Parse(srcURL); err == nil {
		host, path = u.Hostname(), u.Path
	}
	if st.Filtered == 0 && st.Total >= adNumMinSegs && st.HasDisc {
		log.Printf("WARN m3u8 filter[%s] 疑似漏过滤 host=%s path=%s total=%d filtered=0 hasDisc=true sample=%v",
			via, host, path, st.Total, st.Sample)
		return
	}
	log.Printf("m3u8 filter[%s] host=%s path=%s total=%d filtered=%d cross=%d num=%d hasDisc=%t",
		via, host, path, st.Total, st.Filtered, st.Cross, st.Num, st.HasDisc)
}

// rewriteM3u8WithStats 把相对 URI 绝对化; filterAds 时剔除与主域不同 host 的分片(常见广告插入手法),
// 并把子播放列表(变体/媒体列表)回环走代理, 使其内部分片同样被过滤(否则过滤只在 master 层生效)。
// 返回: 重写后文本、过滤统计(Total=分片总数, Filtered=剔除的广告分片数)、第一个子播放列表绝对 URL(供 stats 下钻, 无则空)。
// proxyChild=true: 子播放列表回环走本服务代理(服务端过滤链路); false: 仅绝对化, 供端侧混合过滤
// (客户端抓流→本服务只过滤文本→设备直连子列表/分片, 用于服务器抓不到的源)。
func rewriteM3u8WithStats(body string, base *url.URL, filterAds, proxyChild bool) (string, M3u8Stats, string) {
	return rewriteM3u8Core(body, base, filterAds, proxyChild, nil)
}

func (s *M3u8Service) rewriteM3u8WithProxy(body string, base *url.URL, filterAds, proxyChild bool) (string, M3u8Stats, string) {
	return rewriteM3u8Core(body, base, filterAds, proxyChild, s.streamWrap)
}

var hlsURIAttr = regexp.MustCompile(`URI="([^"]+)"`)

func rewriteTagURI(line string, base *url.URL, proxyChild bool, wrapMedia func(string) string) string {
	idx := hlsURIAttr.FindStringSubmatchIndex(line)
	if idx == nil {
		return line
	}
	abs := absolutize(line[idx[2]:idx[3]], base)
	var rewritten string
	if proxyChild && isPlaylistURI(abs) {
		rewritten = proxyWrap(abs, wrapMedia != nil)
	} else if wrapMedia != nil {
		rewritten = wrapMedia(abs)
	} else {
		rewritten = abs
	}
	return line[:idx[2]] + rewritten + line[idx[3]:]
}

func rewriteM3u8Core(body string, base *url.URL, filterAds, proxyChild bool, wrapMedia func(string) string) (string, M3u8Stats, string) {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	// 广告判定基准 = 本列表内分片的"主导 host"(出现最多者 = 真实内容 CDN), 而非播放列表自身 host。
	// 多 CDN 源常把 m3u8 与分片放不同 host(合法), 旧的"跨播放列表 host=广告"会误杀整张表(如 huya)。
	dominant := dominantSegmentHost(lines, base)
	// 预扫: 按播放顺序收集分片(非子列表)绝对 URI, 供"编号跳脱"同域广告检测(lz/量子源把广告
	// 用异编号分片同域插进正片连续序列, 跨域过滤抓不到)。
	var adByIdx map[int]bool
	if filterAds {
		var segURIs []string
		for _, ln := range lines {
			t := strings.TrimSpace(ln)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			abs := absolutize(t, base)
			if isPlaylistURI(abs) {
				continue
			}
			segURIs = append(segURIs, abs)
		}
		adByIdx = numberOutlierAds(segURIs)
	}
	var pendingExtinf string
	var st M3u8Stats
	var firstChild string
	segIdx := -1
	justAfterDisc := false // 上一条有意义的标记是 #EXT-X-DISCONTINUITY(其后首个分片是广告插片首选嫌疑)
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			out = append(out, ln)
			continue
		}
		if strings.HasPrefix(t, "#EXTINF") {
			pendingExtinf = ln
			continue
		}
		if strings.HasPrefix(t, "#") {
			if strings.HasPrefix(t, "#EXT-X-DISCONTINUITY") {
				st.HasDisc = true
				justAfterDisc = true
			}
			out = append(out, rewriteTagURI(ln, base, proxyChild, wrapMedia))
			continue
		}
		// URI 行(分片或子播放列表)
		abs := absolutize(t, base)
		// 子播放列表: filterAds 时回环代理, 否则仅绝对化(分片跨域过滤逻辑不适用于子列表引用)
		if isPlaylistURI(abs) {
			if firstChild == "" {
				firstChild = abs // 原始绝对 URL(非代理包装), 供 stats 下钻重新过滤
			}
			if pendingExtinf != "" {
				out = append(out, pendingExtinf)
				pendingExtinf = ""
			}
			if filterAds && proxyChild {
				out = append(out, proxyWrap(abs, wrapMedia != nil))
			} else {
				out = append(out, abs)
			}
			continue
		}
		// 真实分片(非子播放列表)计入总数
		st.Total++
		segIdx++
		// 采样: DISCONTINUITY 后首个分片(无论是否剔除), 供疑似漏过滤时定位广告形态。
		if justAfterDisc {
			if len(st.Sample) < m3u8SampleMax {
				st.Sample = append(st.Sample, abs)
			}
			justAfterDisc = false
		}
		// 广告判定: ① 非主导 host 的零星分片(跨域广告 CDN); ② 编号跳脱正片连续序列的同域插片。
		if filterAds && pendingExtinf != "" {
			crossDrop := false
			if dominant != "" {
				if h := hostOf(abs); h != "" && h != dominant {
					crossDrop = true
				}
			}
			numDrop := !crossDrop && adByIdx[segIdx]
			if crossDrop || numDrop {
				pendingExtinf = "" // 连同其 EXTINF 丢弃
				st.Filtered++
				if crossDrop {
					st.Cross++
				} else {
					st.Num++
				}
				continue
			}
		}
		if pendingExtinf != "" {
			out = append(out, pendingExtinf)
			pendingExtinf = ""
		}
		if wrapMedia != nil {
			out = append(out, wrapMedia(abs))
		} else {
			out = append(out, abs)
		}
	}
	return strings.Join(out, "\n"), st, firstChild
}

// 编号跳脱广告检测参数。
const (
	adNumMinSegs     = 10   // 分片太少不做编号检测(避免短表误判)
	adNumMinCoverage = 0.80 // 至少这么大比例分片能解析出尾号, 否则视为非顺序命名, 跳过
	adNumMinContent  = 0.60 // 主内容簇至少占比, 否则编号分布太散, 不可信
	adNumMaxDropFrac = 0.30 // 剔除占比上限: 候选广告超此比例 → 判定误判, 整张表不剔
)

// numberOutlierAds 识别"编号跳脱正片连续序列"的同域插播广告。
// 原理: 正片分片文件名尾号在一个紧凑连续区间(主内容簇), 广告被插进来时编号远离该区间。
// 返回 segURIs 中判为广告的下标集; 不满足条件(命名非顺序/无主簇/超剔除上限)时返回 nil(不剔)。
func numberOutlierAds(segURIs []string) map[int]bool {
	n := len(segURIs)
	if n < adNumMinSegs {
		return nil
	}
	nums := make([]int, n)
	numbered := 0
	for i, u := range segURIs {
		nums[i] = segNumber(u)
		if nums[i] >= 0 {
			numbered++
		}
	}
	if float64(numbered) < float64(n)*adNumMinCoverage {
		return nil // 非顺序数字命名(哈希等), 不适用
	}
	// 排序去重
	seen := map[int]bool{}
	distinct := make([]int, 0, numbered)
	for _, x := range nums {
		if x >= 0 && !seen[x] {
			seen[x] = true
			distinct = append(distinct, x)
		}
	}
	if len(distinct) < 2 {
		return nil
	}
	sort.Ints(distinct)
	// 簇切分阈值 = max(1000, 1000×中位步长): 正片步长通常为 1, 广告编号跳变远超此阈值。
	thr := 1000 * medianGap(distinct)
	if thr < 1000 {
		thr = 1000
	}
	// 在排序值空间按 gap>thr 切簇, 取 segment 数最多者为主内容簇。
	lo, hi := distinct[0], distinct[0]
	bestLo, bestHi, bestCount := lo, hi, 0
	flush := func(l, h int) {
		cnt := 0
		for _, x := range nums {
			if x >= l && x <= h {
				cnt++
			}
		}
		if cnt > bestCount {
			bestLo, bestHi, bestCount = l, h, cnt
		}
	}
	for i := 1; i < len(distinct); i++ {
		if distinct[i]-distinct[i-1] > thr {
			flush(lo, hi)
			lo = distinct[i]
		}
		hi = distinct[i]
	}
	flush(lo, hi)
	if float64(bestCount) < float64(n)*adNumMinContent {
		return nil // 没有占主导的连续内容簇, 编号分布不可信
	}
	// 主簇外的有编号分片 = 候选广告
	ads := map[int]bool{}
	for i, x := range nums {
		if x >= 0 && (x < bestLo || x > bestHi) {
			ads[i] = true
		}
	}
	// 剔除占比上限: 超过则判定误判(可能主簇选错), 整张表不剔。
	if float64(len(ads)) > float64(n)*adNumMaxDropFrac {
		return nil
	}
	return ads
}

// segNumber 取分片文件名(去 query/扩展名后)末尾连续数字; 无则返回 -1。
func segNumber(rawURL string) int {
	p := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		p = u.Path
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		p = p[:i]
	}
	j := len(p)
	for j > 0 && p[j-1] >= '0' && p[j-1] <= '9' {
		j--
	}
	if j == len(p) {
		return -1
	}
	digits := p[j:]
	if len(digits) > 15 { // 防 int 溢出, 取末 15 位足够区分
		digits = digits[len(digits)-15:]
	}
	v, err := strconv.Atoi(digits)
	if err != nil {
		return -1
	}
	return v
}

// medianGap 排序数列相邻差值的中位数(用于估计正片编号步长)。
func medianGap(sorted []int) int {
	if len(sorted) < 2 {
		return 0
	}
	gaps := make([]int, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		gaps = append(gaps, sorted[i]-sorted[i-1])
	}
	sort.Ints(gaps)
	return gaps[len(gaps)/2]
}

// isPlaylistURI 判断 URI 是否指向 m3u8 子播放列表(忽略 query/fragment)。
func isPlaylistURI(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Path), ".m3u8")
}

// proxyWrap 把绝对子播放列表 URL 包成本服务的根相对代理 URL(强制 filterAds=1), 使其被同样过滤。
func proxyWrap(abs string, proxyMedia bool) string {
	out := m3u8ProxyPath + "?src=" + url.QueryEscape(abs) + "&filterAds=1"
	if proxyMedia {
		out += "&proxyMedia=1"
	}
	return out
}

func absolutize(ref string, base *url.URL) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if r, err := url.Parse(ref); err == nil {
		return base.ResolveReference(r).String()
	}
	return ref
}

// dominantSegmentHost 统计本播放列表内分片(非子播放列表)的 host, 返回出现最多者(真实内容 CDN)。
// 全部分片在同一 host(即便≠播放列表 host) → 返回该 host, 不会被当广告; 零星异 host 分片才判广告。
func dominantSegmentHost(lines []string, base *url.URL) string {
	counts := map[string]int{}
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		abs := absolutize(t, base)
		if isPlaylistURI(abs) {
			continue // 子播放列表不计入分片 host 统计
		}
		if h := hostOf(abs); h != "" {
			counts[h]++
		}
	}
	best, bestN := "", 0
	for h, n := range counts {
		if n > bestN {
			best, bestN = h, n
		}
	}
	return best
}

func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Hostname()
	}
	return ""
}

func sha1hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}
