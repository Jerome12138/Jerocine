// Package botdetect 判定一次在线心跳是否来自机器人, 并给出命中原因。
//
// 背景(2026-10-21 线上取证): 某天 222 条会话里有 219 条是"正常 Chrome UA + 204 个不同 IP +
// 全部直达 /filmDetail + 每个只心跳一次", IP 集中在 43.128.0.0/10(腾讯云国际段)。
// 这类爬虫跑的是真浏览器内核, **UA 关键词规则完全失明**, 只靠 UA 过滤会让后台"访问数据"里
// 全是僵尸会话。故改为多信号判定, 命中即归类到具体原因, 便于后台核对规则是否生效:
//
//  1. UA 关键词           —— 爬虫/脚本/无头浏览器自报家门(最确定)
//  2. navigator.webdriver —— 客户端自报无头浏览器(headless 默认 true)
//  3. 缺浏览器特征头      —— Accept-Language 与 Sec-Fetch-* 都没有(真实浏览器必带)
//  4. 机房/云厂商 IP      —— ISP 关键词命中, 或落在云段 CIDR 内(线上僵尸会话即此)
//
// 口径取舍: 宁可多杀 —— 只影响"在线/访问"统计(进机器人计数, 从 UV/PV 与明细里剔除),
// 不影响鉴权与站点功能。用云主机当 VPN 的真实访客会被计入机器人, 这个代价是可接受的。
package botdetect

import (
	"net/netip"
	"strings"
)

// Reason 命中原因; 空串表示判定为真人。
type Reason string

const (
	// ReasonHuman 未命中任何规则。
	ReasonHuman Reason = ""
	// ReasonUA UA 含爬虫/脚本/无头浏览器关键词。
	ReasonUA Reason = "ua"
	// ReasonWebDriver 客户端上报 navigator.webdriver === true。
	ReasonWebDriver Reason = "webdriver"
	// ReasonHeader 请求头缺真实浏览器必然携带的特征头。
	ReasonHeader Reason = "header"
	// ReasonDatacenter 来源 IP 属于机房/云厂商。
	ReasonDatacenter Reason = "datacenter"
	// ReasonBehavior 行为异常: 整个会话只有一跳且唯一页面就是 /filmDetail(与 IP/UA 无关,
	// 专抓换云段/IP 池的爬虫)。误伤面 = 外链直达影片详情页且秒退的真人, 故单独分类。
	ReasonBehavior Reason = "behavior"
)

// AllReasons 全部机器人原因(后台按此顺序展示分布)。
var AllReasons = []Reason{ReasonUA, ReasonWebDriver, ReasonHeader, ReasonDatacenter, ReasonBehavior}

// Label 中文短标签(后台展示)。
func (r Reason) Label() string {
	switch r {
	case ReasonUA:
		return "UA"
	case ReasonWebDriver:
		return "无头浏览器"
	case ReasonHeader:
		return "缺浏览器头"
	case ReasonDatacenter:
		return "机房 IP"
	case ReasonBehavior:
		return "行为异常"
	default:
		return "真人"
	}
}

// Input 判定输入。只有 UA 是必填的, 其余缺省按"无信号"处理。
type Input struct {
	UA string
	// AcceptLanguage 请求头 Accept-Language; SecFetchMode 请求头 Sec-Fetch-Mode。
	AcceptLanguage string
	SecFetchMode   string
	// WebDriver 客户端自报的 navigator.webdriver。
	WebDriver bool
	// IP 来源 IP; ISP 为该 IP 的运营商名(geoip.SearchISP), 可为空。
	IP  string
	ISP string
}

// Classify 按 1→4 的顺序判定, 返回首个命中的原因; 都为否返回 ReasonHuman。
func Classify(in Input) Reason {
	if IsBotUA(in.UA) {
		return ReasonUA
	}
	if in.WebDriver {
		return ReasonWebDriver
	}
	if MissingBrowserHeaders(in.AcceptLanguage, in.SecFetchMode) {
		return ReasonHeader
	}
	if IsDatacenter(in.IP, in.ISP) {
		return ReasonDatacenter
	}
	return ReasonHuman
}

// MissingBrowserHeaders 真实浏览器(含 SPA 的 fetch)一定会带 Accept-Language,
// 现代浏览器还会带 Sec-Fetch-*; 两者同时缺失基本只能是脚本/老旧代理。
// 只判空, 不校验内容 —— 内容各浏览器差异大, 校验会误伤。
func MissingBrowserHeaders(acceptLanguage, secFetchMode string) bool {
	return strings.TrimSpace(acceptLanguage) == "" && strings.TrimSpace(secFetchMode) == ""
}

// datacenterISPs 机房/云厂商 ISP 关键词(小写子串, 数据来自 ip2region 第 5 段)。
// 家宽/移动运营商(电信/联通/移动/广电/长城宽带 …)不在此列。
var datacenterISPs = []string{
	"阿里云", "腾讯云", "华为云", "金山云", "青云", "ucloud", "百度云", "移动云", "天翼云",
	"amazon", "aws", "google", "microsoft", "azure", "cloudflare", "akamai",
	"digitalocean", "vultr", "linode", "ovh", "oracle", "hetzner", "contabo",
	"level3", "cogent", "leaseweb", "choopa", "m247",
}

// datacenterCIDRs 云厂商 IPv4 段(故意保持精简: 只收"几乎不会分配给家宽"的大段)。
// 43.128.0.0/10 是腾讯云国际段 —— 线上那批僵尸会话全部落在这里, 也是加这张表的原因。
// 124.156.0.0/16 是腾讯云新加坡/香港段 —— 2026-10-24 线上取证: 老段被拉黑后对方换了
// 这个新段, 28 条"单跳 /filmDetail"僵尸会话全部落在这里(ip2region 只报新加坡/香港,
// ISP 关键词抓不到, 只能靠 CIDR)。
// 35.80.0.0/14 是 AWS 美国段 —— 同日取证的单条漏网(Linux Chrome + 首页 + 0s)。
// 扩充原则: 能确认归属单一云厂商且不含家宽分配的大段才加, 拿不准就不加。
var datacenterCIDRs = []string{
	// 腾讯云
	"43.128.0.0/10", "49.51.0.0/16", "101.32.0.0/12", "119.28.0.0/16",
	"129.226.0.0/16", "150.109.0.0/16", "162.62.0.0/16", "170.106.0.0/16",
	"175.24.0.0/14",
	// 阿里云
	"47.0.0.0/8", "8.208.0.0/12",
	// 华为云
	"114.115.0.0/16", "121.36.0.0/16", "122.112.0.0/16", "139.9.0.0/16", "159.138.0.0/16",
	// AWS / Azure / GCP
	"3.0.0.0/9", "18.128.0.0/9", "52.0.0.0/8", "54.0.0.0/8",
	"20.0.0.0/8", "40.64.0.0/10",
	"34.64.0.0/10", "35.184.0.0/13",
	// 中小云 / VPS
	"164.90.0.0/16", "167.99.0.0/16", "45.32.0.0/16", "108.61.0.0/16",
	"45.33.0.0/16", "172.104.0.0/15", "104.16.0.0/13",
	// 腾讯云新加坡/香港(2026-10-24 取证: 老段被拉黑后的换段目标)
	"124.156.0.0/16",
	// AWS 美国(2026-10-24 取证)
	"35.80.0.0/14",
}

var datacenterPrefixes = mustParsePrefixes(datacenterCIDRs)

func mustParsePrefixes(cidrs []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			panic("botdetect: 非法的内置 CIDR " + c)
		}
		out = append(out, p)
	}
	return out
}

// IsDatacenter 来源是否机房/云厂商: 先看 ISP 关键词, 再看 CIDR 表。
func IsDatacenter(ip, isp string) bool {
	if isp != "" {
		low := strings.ToLower(isp)
		for _, kw := range datacenterISPs {
			if strings.Contains(low, kw) {
				return true
			}
		}
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.Is4() {
		return false // IPv6/非法地址不判(在线统计只按 IPv4 归属地)
	}
	for _, p := range datacenterPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// botUASubstrings 爬虫/脚本/无头浏览器 UA 关键词(小写子串)。
// 只加"不会出现在正常浏览器 UA 里"的词: 例如 okhttp(安卓壳网络栈)、whatsapp(应用内浏览器)
// 都会出现在真实用户 UA 中, 不能加。
var botUASubstrings = []string{
	// ── 通用爬虫/扫描器 ──
	"bot", "spider", "crawl", "slurp", "curl", "wget", "python", "go-http",
	"scrapy", "masscan", "zgrab", "nmap", "censys",
	"semrush", "ahrefs", "mj12", "dotbot", "bytespider", "facebookexternalhit",
	// ── 搜索引擎/收录爬虫 ──
	"googlebot", "googleother", "adsbot-google", "mediapartners-google",
	"bingbot", "bingpreview", "baiduspider", "360spider", "yisouspider",
	"yandexbot", "duckduckbot", "petalbot", "ia_archiver", "applebot",
	"twitterbot", "linkedinbot",
	// ── AI 爬虫 ──
	"gptbot", "oai-searchbot", "claudebot", "ccbot", "amazonbot",
	// ── 监控/连通性检查 ──
	"uptimerobot", "pingdom", "statuscake", "gtmetrix", "monitis",
	// ── 即时通讯/社交平台爬虫(勿加 whatsapp: 应用内浏览器是真实用户) ──
	"telegrambot", "slackbot", "discordbot",
	// ── 无头浏览器/自动化框架 ──
	"headless", "phantom", "puppeteer", "playwright", "selenium", "webdriver",
	"lighthouse", "slimerjs",
	// ── 脚本 HTTP 客户端 ──
	"node", "axios", "http-client", "lwp", "mechanize", "httrack", "libwww",
	"exabot", "gigabot", "java/", "aiohttp", "httpx",
	"postmanruntime", "insomnia",
}

// IsBotUA UA 是否机器人(空 UA 不判 —— 可能是异常客户端而非爬虫, 由请求头规则兜底)。
func IsBotUA(ua string) bool {
	u := strings.ToLower(strings.TrimSpace(ua))
	if u == "" {
		return false
	}
	for _, s := range botUASubstrings {
		if strings.Contains(u, s) {
			return true
		}
	}
	return false
}

// IsKnownBot 只凭已存储的字段(UA + IP)判定历史会话是否机器人。
//
// 请求头与 navigator.webdriver 不入库, 所以读取历史明细只能用这两项兜底;
// 有了机房 IP 规则, 规则上线/扩充后**旧数据也会立刻被过滤掉**(线上那批 200+ 僵尸会话
// 就是靠这条在读取时清掉的, 否则要等 8 天 TTL 自然过期)。
func IsKnownBot(ua, ip string) bool {
	return IsBotUA(ua) || IsDatacenter(ip, "")
}

// behaviorAliveMaxSec 行为判定允许的最大存活秒数: 只放过"一跳就走"的会话
// (真人停留稍长就会有第二跳, 30s 心跳间隔 ⇒ 存活 ≤2s 等价于全程只有一次心跳)。
const behaviorAliveMaxSec int64 = 2

// behaviorPath 行为判定的目标页面: 爬虫从站内任何入口都直奔 /filmDetail
// (详情页数据是它们唯一目标), 而真人从站内浏览进来 paths 会累积多个页面。
const behaviorPath = "/filmDetail"

// IsBehaviorBot 行为启发式: 整个会话只有一跳、且唯一页面就是 /filmDetail。
// 与 IP/UA 无关 —— 专门抓"换云段/IP 池"的爬虫: 2026-10-24 线上取证, 腾讯云新段
// 124.156.0.0/16 那 28 条僵尸会话全部是"单跳 /filmDetail + 存活 0s + visits=1",
// 而 UA/请求头/IP 规则对它们全部失明(换成新段即可绕过任何黑名单)。
//
// 误伤面: 外链/分享直达影片详情页且几秒内离开的真人 —— 概率低且只影响"在线/访问"
// 统计(单独计入 behavior 分类, 后台可核对), 与其它原因一样不动鉴权与站点功能。
func IsBehaviorBot(paths []string, aliveSec int64, visits int) bool {
	if len(paths) != 1 || paths[0] != behaviorPath {
		return false
	}
	return aliveSec >= 0 && aliveSec <= behaviorAliveMaxSec && visits <= 1
}
