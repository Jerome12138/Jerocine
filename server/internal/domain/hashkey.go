package domain

import (
	"fmt"
	"hash/fnv"
	"regexp"
)

// 多站点播放源匹配: 把片名/dbId 归一化后做 fnv32a, 作为 movie_play_source.match_key,
// 提高各站点间同一影片的匹配度。从旧 Movies.GenerateHashKey 平移。
var (
	reHashSpace  = regexp.MustCompile(`\s`)
	reHashAlias  = regexp.MustCompile(`～.*～$`)
	reHashPunct  = regexp.MustCompile(`^[[:punct:]]+|[[:punct:]]+$`)
	reHashSeason = regexp.MustCompile(`季.*`)
)

// NormalizeName 片名归一化(去空格/别名/首尾标点, 季后缀截断为"季"): 即匹配键的归一化部分,
// 不含哈希。按片名采集时用它作为上游 wd 搜索关键字, 保证"搜索口径"与"匹配口径"一致。
func NormalizeName(name string) string {
	name = reHashSpace.ReplaceAllString(name, "")
	name = reHashAlias.ReplaceAllString(name, "")
	name = reHashPunct.ReplaceAllString(name, "")
	name = reHashSeason.ReplaceAllString(name, "季")
	return name
}

// GenerateHashKey 归一化 key(去空格/别名/首尾标点/季后缀)后取 fnv32a 十进制串。
func GenerateHashKey[K string | ~int | int64](key K) string {
	name := NormalizeName(fmt.Sprint(key))
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return fmt.Sprint(h.Sum32())
}
