package douban

import "testing"

// j/subject_abstract 是 rexxar 详情被 IP 封禁后的首选来源（2026-11-26 实测）：
// rate 是 10 分制字符串，star 是 5 分制星级 —— 只认 rate，缺失就跳过（宁缺勿错）。
func TestParseAbstractRate(t *testing.T) {
	cases := []struct {
		name string
		body string
		want float64
		ok   bool
	}{
		{"正常", `{"r":0,"subject":{"rate":"9.7","star":5.0}}`, 9.7, true},
		{"新片低分", `{"r":0,"subject":{"rate":"5.3","star":2.5}}`, 5.3, true},
		{"整数分", `{"r":0,"subject":{"rate":"8"}}`, 8, true},
		{"rate 空（豆瓣无评分）", `{"r":0,"subject":{"rate":"","star":0}}`, 0, false},
		{"只有 star 不猜", `{"r":0,"subject":{"rate":"","star":4.5}}`, 0, false},
		{"无此条(r!=0)", `{"r":1,"subject":{"rate":"9.1"}}`, 0, false},
		{"rate 非法", `{"r":0,"subject":{"rate":"abc"}}`, 0, false},
		{"非 JSON", `<html>403</html>`, 0, false},
		{"空体", ``, 0, false},
	}
	for _, c := range cases {
		got, ok := ParseAbstractRate([]byte(c.body))
		if ok != c.ok || got != c.want {
			t.Fatalf("%s: got (%v,%v) want (%v,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// 兜底决策：abstract 只要可用（err==nil）就**绝不**兜底 rexxar —— 否则"豆瓣无评分"
// 会被 rexxar 的 400 变成"失败"，5 次即熔断（2026-11-26 实测欠账补查 5/2000 就中止）。
func TestDecideRatingSource(t *testing.T) {
	if got := DecideRatingSource(nil, true); got != RatingFromAbstract {
		t.Fatalf("abstract 有分应直接用, got %v", got)
	}
	if got := DecideRatingSource(nil, false); got != RatingFromAbstract {
		t.Fatalf("abstract 200 但无分也应视为'无评分'(不兜底), got %v", got)
	}
	if got := DecideRatingSource(errTest, false); got != RatingFromRexxar {
		t.Fatalf("abstract 真的失败才兜底, got %v", got)
	}
}

type testErr struct{}

func (testErr) Error() string { return "boom" }

var errTest = testErr{}
