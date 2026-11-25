package macrep

import (
	"html"
	"testing"
)

// repPage 用 data-rep-data 拼一个最小公告页（属性值按 HTML 转义）。
func repPage(jsonBody string) string {
	return `<html><body><div class="rep-wrap" data-rep-data="` + html.EscapeString(jsonBody) + `"></div></body></html>`
}

// 源站内嵌 JSON 才是权威来源：早期文本扫描拿不到日期/分类/备注/executable。
func TestParsePageReadsEmbeddedJSON(t *testing.T) {
	body := `[{"id":13,"time":1795009251,"original":"v.lzcdn28.com","applied":true,
	  "rules":[{"id":13,"type":"视频播放地址","original":"v.lzcdn28.com","replacement":"v.cdnlz28.com",
	    "published_target":"v.cdnlz28.com","note":"域名迁移 [ops:liangzi-play-cdnlz28]",
	    "warning":"","executable":true,
	    "fields":[{"table":"vod","field":"vod_play_url","label":"视频播放"}]}]}]`

	rules, rejects := ParsePage(repPage(body))
	if len(rejects) != 0 {
		t.Fatalf("不应有拒收: %+v", rejects)
	}
	if len(rules) != 1 {
		t.Fatalf("规则数 = %d, want 1", len(rules))
	}
	r := rules[0]
	if r.Field != FieldPlay || r.FieldLabel != "视频播放" || r.Type != "视频播放地址" {
		t.Fatalf("字段信息不对: %+v", r)
	}
	if r.FromHost != "v.lzcdn28.com" || r.ToHost != "v.cdnlz28.com" {
		t.Fatalf("域名不对: %+v", r)
	}
	if r.AnnouncedAt != 1795009251*1000 {
		t.Fatalf("公告时间 = %d, want 秒转毫秒", r.AnnouncedAt)
	}
	if r.Note != "域名迁移" {
		t.Fatalf("备注应剥掉 [ops:xxx] 片段, got %q", r.Note)
	}
	// 源站自用字段（executable/applied/warning/published_target/id）按用户要求不采集。
}

// 带协议与尾部斜杠的原文要按同一口径归一到裸 host。
func TestParsePageNormalizesSchemeAndSlash(t *testing.T) {
	body := `[{"id":6,"time":1707753600,"original":"https://pic.lzzypic.com/","applied":false,
	  "rules":[{"id":6,"type":"视频封面替换","original":"https://pic.lzzypic.com/","replacement":"https://viptulz.com/",
	    "published_target":"https://viptulz.com/","note":"原站 rep.html 历史公告；仅迁移展示，不在本库执行",
	    "executable":true,"fields":[{"table":"vod","field":"vod_pic","label":"视频封面"}]}]}]`

	rules, _ := ParsePage(repPage(body))
	if len(rules) != 1 {
		t.Fatalf("规则数 = %d", len(rules))
	}
	if rules[0].FromHost != "pic.lzzypic.com" || rules[0].ToHost != "viptulz.com" {
		t.Fatalf("归一化失败: %+v", rules[0])
	}
	if rules[0].Note == "" {
		t.Fatal("备注未采集")
	}
}

// 带路径的原文必须拒收（宁可漏娶，不可配错）。
func TestParsePageRejectsPath(t *testing.T) {
	body := `[{"id":1,"time":1705680000,"original":"a.com","applied":false,
	  "rules":[{"id":1,"type":"视频封面替换","original":"a.com","replacement":"b.com/x/y",
	    "executable":true,"fields":[{"table":"vod","field":"vod_pic","label":"视频封面"}]}]}]`

	rules, rejects := ParsePage(repPage(body))
	if len(rules) != 0 || len(rejects) != 1 {
		t.Fatalf("rules=%d rejects=%d, want 0/1", len(rules), len(rejects))
	}
}

// 未知目标字段（不在三个目标列里）不采信。
func TestParsePageRejectsUnknownField(t *testing.T) {
	body := `[{"id":1,"time":1705680000,"original":"a.com","applied":false,
	  "rules":[{"id":1,"type":"其它","original":"a.com","replacement":"b.com","executable":true,
	    "fields":[{"table":"vod","field":"vod_area","label":"地区"}]}]}]`

	rules, rejects := ParsePage(repPage(body))
	if len(rules) != 0 || len(rejects) != 1 {
		t.Fatalf("rules=%d rejects=%d, want 0/1", len(rules), len(rejects))
	}
}

// 同一 (field, from) 重复出现时保留首条（页面时间倒序 ⇒ 最新的在前）。
func TestParsePageDedupKeepsFirst(t *testing.T) {
	body := `[{"id":2,"time":1700000000,"original":"a.com","applied":false,
	  "rules":[{"id":2,"type":"视频封面替换","original":"a.com","replacement":"new.com","executable":true,
	    "fields":[{"table":"vod","field":"vod_pic","label":"视频封面"}]}]},
	 {"id":1,"time":1600000000,"original":"a.com","applied":false,
	  "rules":[{"id":1,"type":"视频封面替换","original":"a.com","replacement":"old.com","executable":true,
	    "fields":[{"table":"vod","field":"vod_pic","label":"视频封面"}]}]}]`

	rules, _ := ParsePage(repPage(body))
	if len(rules) != 1 || rules[0].ToHost != "new.com" {
		t.Fatalf("去重应保留最新一条: %+v", rules)
	}
}

// 页面没有 data-rep-data（改版）时，必须回退到文本扫描而不是直接失败。
func TestParsePageFallsBackToText(t *testing.T) {
	page := `<div>vod_pic<br>` + "\n" + `p.lztutu.com → p.lzp261118.com</div>`
	rules, _ := ParsePage(page)
	if len(rules) != 1 || rules[0].Field != FieldPic || rules[0].FromHost != "p.lztutu.com" {
		t.Fatalf("文本兜底失败: %+v", rules)
	}
}

// 规则要挂到来源站上（本次来源是 lz），便于按站归类与跳原文。
func TestSourceKeyFromURL(t *testing.T) {
	cases := map[string]string{
		"https://www.lzizy.com/index.php/macrep.html": "lz",
		"http://lzizy.com/index.php/macrep.html":      "lz",
		"https://lzizy.com:443/x/y":                   "lz",
		"https://other.com/ann.html":                  "other.com",
		"":                                            "",
	}
	for in, want := range cases {
		if got := SourceKeyFromURL(in); got != want {
			t.Fatalf("SourceKeyFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// ops 标记可能出现在备注里（甚至多段），写库前全部剥掉。
func TestStripOpsTag(t *testing.T) {
	cases := map[string]string{
		"域名迁移 [ops:liangzi-play-cdnlz28]": "域名迁移",
		"[ops:a-1] 两个 [ops:b-2]":          "两个",
		"无标记的备注":                          "无标记的备注",
		"":                                "",
	}
	for in, want := range cases {
		if got := stripOpsTag(in); got != want {
			t.Fatalf("stripOpsTag(%q) = %q, want %q", in, got, want)
		}
	}
}

// 单条链接换域名：带协议 / 裸前缀 / 不命中 / 子域名不误伤。
func TestExchangeHost(t *testing.T) {
	cases := []struct {
		in, from, to, want string
		ok                 bool
	}{
		{"https://v.lzcdn28.com/hls/1.m3u8", "v.lzcdn28.com", "v.cdnlz28.com",
			"https://v.cdnlz28.com/hls/1.m3u8", true},
		{"http://a.b.com:8080/x", "a.b.com", "c.com", "http://c.com:8080/x", true},
		{"v.lzcdn28.com/hls/1.m3u8", "v.lzcdn28.com", "v.cdnlz28.com",
			"v.cdnlz28.com/hls/1.m3u8", true},
		{"https://v.lzcdn28.com.cn/x", "v.lzcdn28.com", "a.com", "https://v.lzcdn28.com.cn/x", false},
		{"https://other.com/x", "v.lzcdn28.com", "a.com", "https://other.com/x", false},
	}
	for _, c := range cases {
		got, ok := ExchangeHost(c.in, c.from, c.to)
		if got != c.want || ok != c.ok {
			t.Fatalf("ExchangeHost(%q,%q,%q) = (%q,%v), want (%q,%v)", c.in, c.from, c.to, got, ok, c.want, c.ok)
		}
	}
}
