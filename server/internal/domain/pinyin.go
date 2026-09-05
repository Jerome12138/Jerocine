package domain

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

// NameInitials 计算影片名的"拼音首字母串"(大写), 供首字母搜索(如 凡人修仙传 → FRXXZ)。
//   - 汉字 → 拼音首字母(大写)
//   - ASCII 字母 → 原样大写(兼容英文名, 如 Friends → FRIENDS)
//   - 数字 → 保留
//   - 空格/标点等 → 忽略
//
// 与 ProjectMovieToSearch 配合: 采集/手动加片投影时一并算好存入 movie_search.name_pinyin。
func NameInitials(name string) string {
	a := pinyin.NewArgs()
	a.Style = pinyin.FirstLetter
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.Is(unicode.Han, r):
			py := pinyin.SinglePinyin(r, a)
			if len(py) > 0 && py[0] != "" {
				b.WriteString(strings.ToUpper(py[0]))
			}
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 32) // → 大写
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}
