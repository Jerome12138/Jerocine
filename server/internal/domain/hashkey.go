package domain

import (
	"fmt"
	"hash/fnv"
	"regexp"
)

// 多站点播放源匹配: 把片名/dbId 归一化后做 fnv32a, 作为 movie_play_source.match_key,
// 提高各站点间同一影片的匹配度。
var (
	// 空白字符(半角/全角/制表)一律丢弃: 源站片名的空格写法极不统一。
	reHashSpace = regexp.MustCompile(`\s|\x{3000}`)
	// 尾部"～别名～": 源站常用它挂副标题。
	reHashAlias = regexp.MustCompile(`～.*～$`)
	// 首尾标点/括号: 半角标点 + 中文常用成对符号。
	reHashPunct = regexp.MustCompile(`^[\s[:punct:]【】《》「」『』〔〕（）［］｛｝、，。；：·…—]+|[\s[:punct:]【】《》「」『』〔〕（）［］｛｝、，。；：·…—]+$`)
	// 季后缀截断: "季"之后的内容(如各季副标题)不再参与匹配, 保留"季"这个分季区分位。
	// ⚠ 与 reHashNoise 的分工: 这里**刻意不看管季**, 见 reHashNoise 注释。
	reHashSeason = regexp.MustCompile(`季.*`)
)

// reHashNoise 片名噪声词 —— 采集源普遍把发行标记直接拼进片名, 这些标记与"作品本体"无关,
// 却让同一部片在不同源上算出不同的 match_key, 于是聚合时挂不上线。
// 归一化阶段整体剔除, 使"某某 1080P 中字"与"某某"落到同一个 key。
//
// 刻意**不含**"第N季"/"季": 季是分季区分位, 由 reHashSeason 单独收敛。
// 若把"第N季"当噪声删掉, "某某第一季"与"某某第二季"会被合并成同一部片。
var reHashNoise = regexp.MustCompile(`(?i)` +
	// 发布组标记: 整段方括号(半角/全角)含这些词则连括号一并剔除
	`\[[^\]]*(?:字幕组|字幕社|汉化|压制|发布组|资源组)[^\]]*\]|` +
	`【[^】]*(?:字幕组|字幕社|汉化|压制|发布组|资源组)[^】]*】|` +
	// 集数标记(不动"季")
	`第\s*\d+\s*[话話集期回]|` +
	`\b[Ss]\d{1,2}(?:[Ee]\d{1,3})?\b|` +
	`\b[Ee][Pp]?\s*\d{1,3}\b|` +
	`\b\d{1,2}x\d{1,3}\b|` +
	// 分辨率 / 画质 / 编码 / 封装 / 体积。
	// 分辨率组(1080P/4K)刻意**两侧都不锚定边界**: 源站几乎总把它与画质/编码连写(如 "4KHDR"、
	// "1080Px265"), 一加边界就整段漏剔; 数字+P/数字+K 的组合在片名里没有别的含义。
	// 其余标记保留 \b: 它们短(HD/BD/AVI...), 不锚定会误伤英文词(如 Aviation 被剔成 ation)。
	`(?:2160|1440|1080|720|480)[Pp]|` +
	`[48][Kk]|` +
	`\b(?:REMUX|BluRay|Blu-ray|BDRip|BRRip|BDMV|WEB-?DL|WEBRip|DVDRip|HDTV|FHD|UHD|HDR|SDR|HD|BD)\b|` +
	`\b(?:HEVC|AVC|AV1|x264|x265|H\.?264|H\.?265|10bit|8bit)\b|` +
	`\b(?:TrueHD|DDP|AAC|AC3|DTS)\b|` +
	`\b(?:MP4|MKV|AVI|RMVB|FLV)\b|` +
	// 体积: 左边界不锚定(常紧跟封装标记, 如 "MKV2.5GB", 锚了就锚不到)
	`\d+(?:\.\d+)?[GgMm][Bb]\b|` +
	// 字幕 / 语言包装
	`(?:国语中字|中英双字|双语字幕|中文字幕|简繁字幕|简体|繁体|中字|无字|生肉|熟肉)|` +
	// 更新进度
	`(?:更新至\d+[话話集]?|全集|合集|连载)`,
)

// NormalizeName 片名归一化: 去空白/尾部别名 → 剔噪声词 → 去首尾标点 → 季后缀截断为"季"。
// 即匹配键的归一化部分, 不含哈希。按片名采集时用它作为上游 wd 搜索关键字,
// 保证"搜索口径"与"匹配口径"一致。
func NormalizeName(name string) string {
	name = reHashSpace.ReplaceAllString(name, "")
	name = reHashAlias.ReplaceAllString(name, "")
	name = stripNoise(name)
	name = reHashPunct.ReplaceAllString(name, "")
	name = reHashSeason.ReplaceAllString(name, "季")
	return name
}

// stripNoise 反复剔除噪声词直到不再变化。
// 迭代而非单趟的原因: 源站常把多个标记连写(如 "1080Px265"), 第一趟剔掉 "1080P" 之后,
// "x265" 前面才空出可锚定的边界, 需要再跑一趟才能剔净。上限 4 趟(每趟必缩短串, 不会不收敛)。
func stripNoise(name string) string {
	for i := 0; i < 4; i++ {
		next := reHashNoise.ReplaceAllString(name, "")
		if next == name {
			break
		}
		name = next
	}
	return name
}

// GenerateHashKey 归一化 key(去空白/噪声词/首尾标点/季后缀)后取 fnv32a 十进制串。
func GenerateHashKey[K string | ~int | int64](key K) string {
	name := NormalizeName(fmt.Sprint(key))
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return fmt.Sprint(h.Sum32())
}
