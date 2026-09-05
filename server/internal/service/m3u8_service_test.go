package service

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestFilterText_ClientSide 端侧混合过滤: 子列表绝对化、不改写成代理(设备直连)。
func TestFilterText_ClientSide(t *testing.T) {
	s := NewM3u8Service([]byte("test-signing-key"))
	master := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=2000000\n2000k/mixed.m3u8\n"
	out, _, err := s.FilterText(master, "https://p.bf.com/v/x/index.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "/m3u8/proxy") {
		t.Fatalf("client-side 不应改写成代理:\n%s", out)
	}
	if !strings.Contains(out, "https://p.bf.com/v/x/2000k/mixed.m3u8") {
		t.Fatalf("子列表应绝对化:\n%s", out)
	}
	// 同域编号跳脱广告应被剔
	base := "https://p.bf.com/v/x/"
	_ = base
	if _, _, err := s.FilterText("bad-url-no-host", ""); err == nil {
		t.Fatal("空 src 应报错")
	}
}

// buildSeqM3u8 按给定编号序列生成同域(全相对路径)媒体播放列表, 用于编号跳脱过滤测试。
func buildSeqM3u8(nums []int) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, n := range nums {
		b.WriteString("#EXTINF:4.0,\n")
		fmt.Fprintf(&b, "seg%d.ts\n", n)
	}
	return b.String()
}

func TestRewriteM3u8_Absolutize(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/play/index.m3u8")
	body := "#EXTM3U\n#EXTINF:5.0,\nseg0.ts\n#EXTINF:5.0,\n/abs/seg1.ts\n"
	out := rewriteM3u8(body, base, false)
	if !strings.Contains(out, "https://cdn.example.com/play/seg0.ts") {
		t.Fatalf("relative seg not absolutized:\n%s", out)
	}
	if !strings.Contains(out, "https://cdn.example.com/abs/seg1.ts") {
		t.Fatalf("root-relative seg not absolutized:\n%s", out)
	}
}

func TestRewriteM3u8_ProxiesMediaResources(t *testing.T) {
	s := NewM3u8Service([]byte("test-signing-key"))
	base, _ := url.Parse("https://cdn.example.com/play/index.m3u8")
	body := strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-STREAM-INF:BANDWIDTH=2000000", "child.m3u8",
		`#EXT-X-KEY:METHOD=AES-128,URI="key.bin"`,
		`#EXT-X-MAP:URI="init.mp4"`,
		"#EXTINF:5.0,", "seg0.ts",
		"",
	}, "\n")
	out, _, _ := s.rewriteM3u8WithProxy(body, base, true, true)
	if strings.Contains(out, "https://cdn.example.com/play/seg0.ts") ||
		strings.Contains(out, `URI="https://cdn.example.com/play/key.bin"`) {
		t.Fatalf("media resources must not remain direct CDN URLs:\n%s", out)
	}
	if strings.Count(out, "/api/v1/m3u8/stream?") != 3 {
		t.Fatalf("segment, key and map should use stream proxy:\n%s", out)
	}
	if !strings.Contains(out, "sig=") || !strings.Contains(out, "exp=") {
		t.Fatalf("stream proxy URLs must be signed:\n%s", out)
	}
	if !strings.Contains(out, "/api/v1/m3u8/proxy?") || !strings.Contains(out, "proxyMedia=1") {
		t.Fatalf("child playlist must preserve media proxy mode:\n%s", out)
	}
}

func TestM3u8StreamSignature(t *testing.T) {
	s := NewM3u8Service([]byte("test-signing-key"))
	raw := "https://cdn.example.com/play/seg0.ts?token=a"
	exp := time.Now().Add(time.Hour).Unix()
	sig := s.signStream(raw, exp)
	if !s.VerifyStream(raw, strconv.FormatInt(exp, 10), sig) {
		t.Fatal("valid stream signature rejected")
	}
	if s.VerifyStream(raw+"x", strconv.FormatInt(exp, 10), sig) {
		t.Fatal("tampered source accepted")
	}
	if s.VerifyStream(raw, strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10), sig) {
		t.Fatal("expired signature accepted")
	}
}

func TestRewriteM3u8_AdFilterDropsCrossDomain(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/play/index.m3u8")
	body := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:5.0,", "seg0.ts",
		"#EXTINF:2.0,", "https://ads.evil.com/ad0.ts", // 跨域广告
		"#EXTINF:5.0,", "seg1.ts",
		"",
	}, "\n")
	out := rewriteM3u8(body, base, true)
	if strings.Contains(out, "ads.evil.com") {
		t.Fatalf("ad segment should be dropped:\n%s", out)
	}
	if !strings.Contains(out, "seg0.ts") || !strings.Contains(out, "seg1.ts") {
		t.Fatalf("legit segments should remain:\n%s", out)
	}
	// 广告分片的 EXTINF(2.0) 不应残留为孤立标签
	if strings.Count(out, "#EXTINF") != 2 {
		t.Fatalf("orphan EXTINF left:\n%s", out)
	}
}

// 多 CDN 源: m3u8 在 host A, 全部分片在 host B(合法分片 CDN) → 不得被当广告全部丢弃。
func TestRewriteM3u8_AdFilterKeepsMultiCdnSegments(t *testing.T) {
	base, _ := url.Parse("https://1080p.huyall.com/play/hls/x/index.m3u8")
	body := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:5.0,", "https://c.baisiweiting.com:18443/hls/p0.ts",
		"#EXTINF:5.0,", "https://c.baisiweiting.com:18443/hls/p1.ts",
		"#EXTINF:5.0,", "https://c.baisiweiting.com:18443/hls/p2.ts",
		"",
	}, "\n")
	out := rewriteM3u8(body, base, true)
	for _, seg := range []string{"p0.ts", "p1.ts", "p2.ts"} {
		if !strings.Contains(out, seg) {
			t.Fatalf("multi-CDN segment %s wrongly dropped:\n%s", seg, out)
		}
	}
	if strings.Count(out, "#EXTINF") != 3 {
		t.Fatalf("segments should be kept with their EXTINF:\n%s", out)
	}
}

// 主导 host 内容 + 零星异 host 广告分片 → 只丢广告分片。
func TestRewriteM3u8_AdFilterDropsMinorityHost(t *testing.T) {
	base, _ := url.Parse("https://play.x.com/hls/index.m3u8")
	body := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:5.0,", "https://cdnA.com/s0.ts",
		"#EXTINF:5.0,", "https://cdnA.com/s1.ts",
		"#EXTINF:2.0,", "https://ads.evil.com/ad.ts", // 少数派异 host → 广告
		"#EXTINF:5.0,", "https://cdnA.com/s2.ts",
		"",
	}, "\n")
	out := rewriteM3u8(body, base, true)
	if strings.Contains(out, "ads.evil.com") {
		t.Fatalf("minority-host ad should be dropped:\n%s", out)
	}
	if strings.Count(out, "cdnA.com") != 3 || strings.Count(out, "#EXTINF") != 3 {
		t.Fatalf("dominant-host segments should remain:\n%s", out)
	}
}

// 广告过滤统计: 剔除的分片计入 Filtered, 全部分片计入 Total。
func TestRewriteM3u8WithStats_CountsFilteredAndTotal(t *testing.T) {
	base, _ := url.Parse("https://play.x.com/hls/index.m3u8")
	body := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:5.0,", "https://cdnA.com/s0.ts",
		"#EXTINF:5.0,", "https://cdnA.com/s1.ts",
		"#EXTINF:2.0,", "https://ads.evil.com/ad.ts", // 少数派异 host → 广告
		"#EXTINF:5.0,", "https://cdnA.com/s2.ts",
		"",
	}, "\n")
	_, st, child := rewriteM3u8WithStats(body, base, true, true)
	if st.Filtered != 1 {
		t.Fatalf("Filtered=%d want 1", st.Filtered)
	}
	if st.Total != 4 {
		t.Fatalf("Total=%d want 4", st.Total)
	}
	if child != "" {
		t.Fatalf("media playlist 无子列表, 不应返回 firstChild, got %q", child)
	}
}

// master(变体)列表无直接分片, Total=0, 但应捕获第一个子播放列表绝对 URL 供 stats 下钻。
func TestRewriteM3u8WithStats_CapturesFirstChild(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/path/master.m3u8")
	body := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=2000000\n2000k/mixed.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=4000000\n4000k/mixed.m3u8\n"
	_, st, child := rewriteM3u8WithStats(body, base, true, true)
	if st.Total != 0 || st.Filtered != 0 {
		t.Fatalf("master 无直接分片, st=%+v want {0 0}", st)
	}
	if child != "https://cdn.example.com/path/2000k/mixed.m3u8" {
		t.Fatalf("firstChild=%q", child)
	}
}

// filterAds 关闭: 不剔除任何分片(Filtered=0), Total 仍统计全部分片。
func TestRewriteM3u8WithStats_NoFilter(t *testing.T) {
	base, _ := url.Parse("https://play.x.com/hls/index.m3u8")
	body := "#EXTM3U\n#EXTINF:5.0,\nhttps://ads.evil.com/ad.ts\n#EXTINF:5.0,\nhttps://cdnA.com/s0.ts\n"
	_, st, _ := rewriteM3u8WithStats(body, base, false, true)
	if st.Filtered != 0 {
		t.Fatalf("过滤关闭 → Filtered=%d want 0", st.Filtered)
	}
	if st.Total != 2 {
		t.Fatalf("Total=%d want 2", st.Total)
	}
}

// 同域插播广告(编号跳脱正片连续序列)→ 应被剔除, 正片保留。模拟 lz 源真实形态。
func TestRewriteM3u8WithStats_DropsNumberOutlierAds(t *testing.T) {
	base, _ := url.Parse("https://v.lzfile26.com/x/2000k/hls/mixed.m3u8")
	content := []int{}
	for i := 5000000; i <= 5000019; i++ { // 20 段正片, 连续编号
		content = append(content, i)
	}
	// 在第 10 段后插入两段异编号广告(同域)
	seq := append([]int{}, content[:10]...)
	seq = append(seq, 50184765, 50184766)
	seq = append(seq, content[10:]...)
	out, st, _ := rewriteM3u8WithStats(buildSeqM3u8(seq), base, true, true)
	if st.Filtered != 2 {
		t.Fatalf("应剔除 2 段异编号广告, got Filtered=%d", st.Filtered)
	}
	if st.Total != 22 {
		t.Fatalf("Total=%d want 22", st.Total)
	}
	if strings.Contains(out, "seg50184765.ts") || strings.Contains(out, "seg50184766.ts") {
		t.Fatalf("广告分片应被剔除:\n%s", out)
	}
	if !strings.Contains(out, "seg5000000.ts") || !strings.Contains(out, "seg5000019.ts") {
		t.Fatalf("正片应保留:\n%s", out)
	}
}

// 离群分片占比超上限 → 判定为误判, 整张表不做编号过滤(全保留)。
func TestRewriteM3u8WithStats_NumberFilterCapKeepsAll(t *testing.T) {
	base, _ := url.Parse("https://h/x/mixed.m3u8")
	seq := []int{}
	for i := 100; i <= 109; i++ { // 10 段"正片"
		seq = append(seq, i)
	}
	for i := 900000; i <= 900005; i++ { // 6 段离群 → 6/16=37.5% 超过 30% 上限
		seq = append(seq, i)
	}
	_, st, _ := rewriteM3u8WithStats(buildSeqM3u8(seq), base, true, true)
	if st.Filtered != 0 {
		t.Fatalf("剔除占比超上限应全保留, got Filtered=%d", st.Filtered)
	}
	if st.Total != 16 {
		t.Fatalf("Total=%d want 16", st.Total)
	}
}

// 哈希命名(无可解析编号)→ 跳过编号过滤, 不误杀。
func TestRewriteM3u8WithStats_NumberFilterSkipsHashedNames(t *testing.T) {
	base, _ := url.Parse("https://h/x/mixed.m3u8")
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, h := range []string{"aa", "bb", "cc", "dd", "ee", "ff", "gg", "hh", "ii", "jj", "kk", "ll"} {
		b.WriteString("#EXTINF:4.0,\n")
		b.WriteString("seg_" + h + ".ts\n")
	}
	_, st, _ := rewriteM3u8WithStats(b.String(), base, true, true)
	if st.Filtered != 0 {
		t.Fatalf("无数字命名应跳过编号过滤, got Filtered=%d", st.Filtered)
	}
}

// 无单一主内容簇(多簇均不达占比下限)→ 不可信, 跳过。
func TestRewriteM3u8WithStats_NumberFilterSkipsWhenNoDominantCluster(t *testing.T) {
	base, _ := url.Parse("https://h/x/mixed.m3u8")
	seq := []int{}
	for i := 100; i < 105; i++ {
		seq = append(seq, i)
	}
	for i := 500000; i < 500005; i++ {
		seq = append(seq, i)
	}
	for i := 900000; i < 900005; i++ {
		seq = append(seq, i)
	}
	_, st, _ := rewriteM3u8WithStats(buildSeqM3u8(seq), base, true, true)
	if st.Filtered != 0 {
		t.Fatalf("无主内容簇应跳过, got Filtered=%d", st.Filtered)
	}
}

func TestGuardSSRF_BlocksLoopback(t *testing.T) {
	// localhost 解析到环回, 必须拒绝
	if err := guardSSRF("localhost"); err == nil {
		t.Fatal("loopback host should be blocked")
	}
}

func TestRewriteM3u8_VariantPlaylistReproxiedWhenFilter(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/path/master.m3u8")
	body := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=2000000\n2000k/mixed.m3u8\n"

	// filterAds 开: 子播放列表回环代理(强制 filterAds=1), 使其内部分片同样被过滤
	out := rewriteM3u8(body, base, true)
	want := m3u8ProxyPath + "?src=" + url.QueryEscape("https://cdn.example.com/path/2000k/mixed.m3u8") + "&filterAds=1"
	if !strings.Contains(out, want) {
		t.Fatalf("variant playlist not re-proxied:\n got: %s\n want substr: %s", out, want)
	}

	// filterAds 关: 仅绝对化, 不回环
	out2 := rewriteM3u8(body, base, false)
	if !strings.Contains(out2, "https://cdn.example.com/path/2000k/mixed.m3u8") {
		t.Fatalf("variant playlist not absolutized:\n%s", out2)
	}
	if strings.Contains(out2, m3u8ProxyPath) {
		t.Fatalf("should not re-proxy when filter off:\n%s", out2)
	}
}

func TestIsPlaylistURI(t *testing.T) {
	cases := map[string]bool{
		"https://h/a/index.m3u8":         true,
		"https://h/a/index.m3u8?token=x": true,
		"https://h/a/seg1.ts":            false,
		"https://h/a/seg1.ts?x=1":        false,
	}
	for in, want := range cases {
		if got := isPlaylistURI(in); got != want {
			t.Errorf("isPlaylistURI(%q)=%v want %v", in, got, want)
		}
	}
}

// 复现 bf ep146 真实形态: 同域 + #EXT-X-DISCONTINUITY 包裹 + 20 位时间戳编号的 /video/adjump 插片。
// 断言: 9 段广告被编号跳脱剔除, Num 计数, HasDisc=true, Sample 采到插片首段(供排查日志)。
func TestRewriteM3u8WithStats_BfEp146Shape(t *testing.T) {
	base, _ := url.Parse("https://v.fengbao8.com/video/x/ep146/index.m3u8")
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for i := 0; i < 30; i++ { // 30 段正片(同域相对), 主内容簇
		fmt.Fprintf(&b, "#EXTINF:2,\n%07d.ts\n", i)
	}
	b.WriteString("#EXT-X-DISCONTINUITY\n")
	for i := 0; i < 9; i++ { // 9 段广告: 根相对 /video/adjump, 20 位时间戳编号(末15位 ≈ 9.5e14, 离群)
		fmt.Fprintf(&b, "#EXTINF:3,\n/video/adjump/time/1776695243547000000%d.ts\n", i)
	}
	b.WriteString("#EXT-X-DISCONTINUITY\n")
	for i := 30; i < 60; i++ { // 正片续
		fmt.Fprintf(&b, "#EXTINF:2,\n%07d.ts\n", i)
	}
	out, st, _ := rewriteM3u8WithStats(b.String(), base, true, false)
	if st.Filtered != 9 {
		t.Fatalf("应剔 9 段 adjump 广告, got Filtered=%d (Num=%d Cross=%d)", st.Filtered, st.Num, st.Cross)
	}
	if st.Num != 9 || st.Cross != 0 {
		t.Fatalf("应全部由编号跳脱命中: Num=%d Cross=%d want 9/0", st.Num, st.Cross)
	}
	if !st.HasDisc {
		t.Fatalf("含 #EXT-X-DISCONTINUITY, HasDisc 应为 true")
	}
	if len(st.Sample) == 0 || !strings.Contains(st.Sample[0], "/video/adjump/time/17766952435470000000.ts") {
		t.Fatalf("Sample 首条应是插片后首个分片, got %v", st.Sample)
	}
	if strings.Contains(out, "adjump") {
		t.Fatalf("过滤后不应残留 adjump 分片:\n%s", out)
	}
	if !strings.Contains(out, "0000000.ts") || !strings.Contains(out, "0000059.ts") {
		t.Fatalf("正片应保留首尾段:\n%s", out)
	}
}

// 跨域广告 → Cross 计数(非 Num); 无 DISCONTINUITY → HasDisc=false。
func TestRewriteM3u8WithStats_CrossDomainCounted(t *testing.T) {
	base, _ := url.Parse("https://play.x.com/hls/index.m3u8")
	body := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:5,", "https://cdnA.com/s0.ts",
		"#EXTINF:5,", "https://cdnA.com/s1.ts",
		"#EXTINF:2,", "https://ads.evil.com/ad.ts", // 跨域广告
		"#EXTINF:5,", "https://cdnA.com/s2.ts",
		"",
	}, "\n")
	_, st, _ := rewriteM3u8WithStats(body, base, true, false)
	if st.Filtered != 1 || st.Cross != 1 || st.Num != 0 {
		t.Fatalf("跨域命中应记 Cross: Filtered=%d Cross=%d Num=%d want 1/1/0", st.Filtered, st.Cross, st.Num)
	}
	if st.HasDisc {
		t.Fatalf("无 DISCONTINUITY, HasDisc 应为 false")
	}
}

// 排查日志: filtered==0 且有插片标记 → WARN 疑似漏过滤; 正常过滤 → info 明细; Total=0(master/空表)→ 不打印降噪。
func TestLogM3u8Filter_WarnVsInfoVsQuiet(t *testing.T) {
	var buf bytes.Buffer
	oldW, oldF := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(oldW); log.SetFlags(oldF) }()

	// ① 疑似漏网: total>=10, filtered=0, 有插片标记
	logM3u8Filter("client", "https://v.fengbao8.com/a/b/index.m3u8",
		M3u8Stats{Total: 613, Filtered: 0, HasDisc: true, Sample: []string{"https://v.fengbao8.com/video/adjump/time/x.ts"}})
	if !strings.Contains(buf.String(), "WARN") || !strings.Contains(buf.String(), "疑似漏过滤") || !strings.Contains(buf.String(), "adjump") {
		t.Fatalf("应 WARN 疑似漏过滤 + 样本, got: %s", buf.String())
	}

	// ② 正常过滤: filtered>0 → info 明细, 不 WARN
	buf.Reset()
	logM3u8Filter("client", "https://v.fengbao8.com/a/b/index.m3u8",
		M3u8Stats{Total: 613, Filtered: 9, Num: 9, HasDisc: true})
	if strings.Contains(buf.String(), "WARN") {
		t.Fatalf("正常过滤不应 WARN, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "filtered=9") || !strings.Contains(buf.String(), "num=9") {
		t.Fatalf("info 应含统计明细, got: %s", buf.String())
	}

	// ③ master/空表 Total=0 → 不打印
	buf.Reset()
	logM3u8Filter("server", "https://x/master.m3u8", M3u8Stats{Total: 0})
	if buf.Len() != 0 {
		t.Fatalf("Total=0 应降噪不打印, got: %s", buf.String())
	}
}
