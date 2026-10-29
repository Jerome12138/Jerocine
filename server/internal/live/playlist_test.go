package live

import (
	"strings"
	"testing"
)

func TestParsePlaylistStandard(t *testing.T) {
	text := strings.Join([]string{
		"#EXTM3U",
		`#EXTINF:-1 tvg-id="cctv1" tvg-name="CCTV1" tvg-logo="http://logo/1.png" group-title="央视",CCTV-1 综合`,
		"http://stream/cctv1.m3u8",
		`#EXTINF:-1 tvg-logo="http://logo/5.png" group-title="央视" tvg-name="CCTV5",CCTV-5 体育`,
		"http://stream/cctv5.m3u8",
		"#EXTINF:-1,地方新闻",
		"#EXTGRP:地方",
		"http://stream/local.flv",
	}, "\n")
	chs := ParsePlaylist(text)
	if len(chs) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(chs), chs)
	}
	if chs[0].Name != "CCTV-1 综合" || chs[0].Group != "央视" || chs[0].Logo != "http://logo/1.png" {
		t.Fatalf("ch0: %+v", chs[0])
	}
	if chs[1].Name != "CCTV-5 体育" || chs[1].Group != "央视" {
		t.Fatalf("ch1: %+v", chs[1])
	}
	if chs[2].Name != "地方新闻" || chs[2].Group != "地方" || chs[2].URL != "http://stream/local.flv" {
		t.Fatalf("ch2: %+v", chs[2])
	}
}

func TestParsePlaylistDefaultsAndEdgeCases(t *testing.T) {
	text := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:-1,无名频道",
		"http://stream/a.m3u8",
		// 属性值含空格（引号包裹不切分）
		`#EXTINF:-1 group-title="央视 综合",带空格分组`,
		"https://stream/b.m3u8?token=abc",
		// EXTINF 后跟注释行再跟地址（注释不打断配对）
		"#EXTINF:-1,跨注释频道",
		"#EXTVLCOPT:http-user-agent=xxx",
		"http://stream/c.m3u8",
		// 非 http 地址（udp 组播）也收录
		"#EXTINF:-1,组播",
		"udp://@239.3.1.1:5000",
	}, "\n")
	chs := ParsePlaylist(text)
	if len(chs) != 4 {
		t.Fatalf("len = %d: %+v", len(chs), chs)
	}
	if chs[0].Group != DefaultGroup {
		t.Fatalf("无分组应归默认组: %+v", chs[0])
	}
	if chs[1].Group != "央视 综合" {
		t.Fatalf("引号内空格不应切分: %+v", chs[1])
	}
	if chs[2].URL != "http://stream/c.m3u8" {
		t.Fatalf("注释行不应打断配对: %+v", chs[2])
	}
	if chs[3].URL != "udp://@239.3.1.1:5000" {
		t.Fatalf("udp 地址应收录: %+v", chs[3])
	}
}

func TestParsePlaylistDropsDanglingAndDup(t *testing.T) {
	text := strings.Join([]string{
		"#EXTM3U",
		"#EXTINF:-1,只有标题没有地址", // 悬挂条目 → 丢弃
		"#EXTINF:-1,重复地址A",
		"http://stream/dup.m3u8",
		"#EXTINF:-1,重复地址B",
		"http://stream/dup.m3u8", // 同址去重（后者整条丢弃）
		"#EXTINF:-1,正常频道",
		"http://stream/ok.m3u8",
		"random-garbage-line", // 无 EXTINF 的裸行 → 丢弃
	}, "\n")
	chs := ParsePlaylist(text)
	if len(chs) != 2 {
		t.Fatalf("len = %d: %+v", len(chs), chs)
	}
	if chs[0].Name != "重复地址A" || chs[1].Name != "正常频道" {
		t.Fatalf("去重应保留首个: %+v", chs)
	}
}

func TestParsePlaylistEmpty(t *testing.T) {
	if chs := ParsePlaylist(""); len(chs) != 0 {
		t.Fatalf("空文本应返回空切片: %+v", chs)
	}
	if chs := ParsePlaylist("#EXTM3U\n"); len(chs) != 0 {
		t.Fatalf("仅头行应返回空切片: %+v", chs)
	}
}
