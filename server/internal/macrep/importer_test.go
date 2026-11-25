package macrep

import (
	"strings"
	"testing"
)

// 真实页面片段（取自 lzizy macrep.html 的结构：字段在箭头行之前，用小节标题分隔）
const samplePage = `
<h2>v.lzcdn28.com 的替换规则</h2>
<strong>视频播放地址</strong><code>vod_play_url</code>
<code>v.lzcdn28.com</code>→<code>v.cdnlz28.com</code>
<div>域名迁移 [ops:liangzi-play-cdnlz28]</div>
<h2>p.lztutu.com 的替换规则</h2>
<strong>视频封面替换</strong><code>vod_pic</code>
<code>p.lztutu.com</code>→<code>p.lzp261118.com</code>
<h2>https://pic.lzzypic.com/ 的替换规则</h2>
<strong>视频封面替换</strong><code>vod_pic</code>
<code>https://pic.lzzypic.com/</code>→<code>https://viptulz.com/</code>
<h2>坏例子：带路径</h2>
<code>vod_play_url</code>
<code>v.example.com/2022/x.m3u8</code>→<code>w.example.com</code>
<h2>坏例子：同域名</h2>
<code>vod_pic</code>
<code>same.example.com</code>→<code>same.example.com</code>
<h2>坏例子：无字段</h2>
<code>orphan.example.com</code>→<code>orphan2.example.com</code>
`

// 采信规则：必须**只有 host**，且字段配对正确；坏例子不得进入。
func TestParseRules_StrictHostOnly(t *testing.T) {
	rules, rejects := ParseRules(samplePage)

	got := map[string]Rule{}
	for _, r := range rules {
		got[r.Field+"|"+r.FromHost] = r
		if strings.ContainsAny(r.FromHost, "/:?") || strings.ContainsAny(r.ToHost, "/:?") {
			t.Errorf("采信规则里出现了非 host 内容: %+v", r)
		}
	}

	// 1) 播放地址
	if r, ok := got["vod_play_url|v.lzcdn28.com"]; !ok || r.ToHost != "v.cdnlz28.com" {
		t.Errorf("播放地址规则缺失或错误: %+v", r)
	}
	// 2) 封面
	if r, ok := got["vod_pic|p.lztutu.com"]; !ok || r.ToHost != "p.lzp261118.com" {
		t.Errorf("封面规则缺失或错误: %+v", r)
	}
	// 3) 带协议/尾部斜杠 ⇒ 应被规范化成裸 host（而不是整条拒绝）
	if r, ok := got["vod_pic|pic.lzzypic.com"]; !ok || r.ToHost != "viptulz.com" {
		t.Errorf("带协议的公告未被规范化为裸 host: %+v", r)
	}
	// 4) 坏例子不得采信
	if _, ok := got["vod_play_url|v.example.com"]; ok {
		t.Error("带路径的候选被误采信")
	}
	if _, ok := got["vod_pic|same.example.com"]; ok {
		t.Error("from==to 的候选被误采信")
	}
	if _, ok := got["|orphan.example.com"]; ok {
		t.Error("无字段归属的候选被误采信")
	}
	if len(rejects) == 0 {
		t.Error("坏例子应进入 rejects 以便人工核对")
	}
}

// 纯 host 校验的边界：单段、非法字符、超长、端口
func TestExtractHost(t *testing.T) {
	ok := []struct{ in, want string }{
		{"v.lzcdn28.com", "v.lzcdn28.com"},
		{"HTTPS://V.LZCDN28.COM/", "v.lzcdn28.com"},
		{"`p.lztutu.com`", "p.lztutu.com"},
	}
	for _, c := range ok {
		if got, v := extractHost(c.in); !v || got != c.want {
			t.Errorf("extractHost(%q) = (%q,%v), want (%q,true)", c.in, got, v, c.want)
		}
	}
	// 严格口径：带端口 / 带路径 / 多 token 一律**拒绝**（不截断猜测）
	bad := []string{"localhost", "a..b", "-a.com", "a-.com", "a.com b.com", "", "。",
		"a.com/path", "a.com:8080", "x.y.z/path?q=1"}
	for _, in := range bad {
		if got, v := extractHost(in); v {
			t.Errorf("extractHost(%q) 应拒绝，却得到 (%q,true)", in, got)
		}
	}
}
