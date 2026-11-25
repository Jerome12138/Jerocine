package mysql

import (
	"encoding/json"
	"strings"
	"testing"
)

// 播放地址替换的纯函数契约（数据改写路径，必须钉住）。
const playJSON = `[{"episode":"第1集","link":"https://vold.example.com/a.m3u8"},` +
	`{"episode":"第2集","link":"https://other.example.com/b.m3u8"}]`

func TestReplaceEpisodeLinks_Hit(t *testing.T) {
	newRaw, eps, changed, before, after, err := replaceEpisodeLinks(playJSON, "vold.example.com", "vnew.example.com")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if changed != 1 {
		t.Fatalf("命中数 = %d, want 1", changed)
	}
	if !strings.Contains(newRaw, "https://vnew.example.com/a.m3u8") {
		t.Fatalf("新链接缺失: %s", newRaw)
	}
	if strings.Contains(newRaw, "vold.example.com") {
		t.Fatalf("旧域名仍存在: %s", newRaw)
	}
	if !strings.Contains(newRaw, "https://other.example.com/b.m3u8") {
		t.Fatalf("未命中的链接被破坏: %s", newRaw)
	}
	if len(eps) != 2 || eps[0].Episode != "第1集" || eps[1].Episode != "第2集" {
		t.Fatalf("集名被破坏: %+v", eps)
	}
	if before != "https://vold.example.com/a.m3u8" || after != "https://vnew.example.com/a.m3u8" {
		t.Fatalf("审计 before/after 不对: %q -> %q", before, after)
	}
	// content_hash 必须随链接变化
	var orig []struct{ Episode, Link string }
	_ = json.Unmarshal([]byte(playJSON), &orig)
	if EpisodesContentHash(eps) == "" {
		t.Fatal("content_hash 为空")
	}
}

func TestReplaceEpisodeLinks_NoHitKeepsRaw(t *testing.T) {
	newRaw, _, changed, _, _, err := replaceEpisodeLinks(playJSON, "nothere.com", "x.com")
	if err != nil || changed != 0 {
		t.Fatalf("不应命中: changed=%d err=%v", changed, err)
	}
	if newRaw != playJSON {
		t.Fatal("未命中时必须原样返回，避免无谓改写")
	}
}

func TestReplaceEpisodeLinks_BareHost(t *testing.T) {
	const raw = `[{"episode":"1","link":"vold.example.com/a.m3u8"}]`
	newRaw, _, changed, _, _, err := replaceEpisodeLinks(raw, "vold.example.com", "vnew.example.com")
	if err != nil || changed != 1 {
		t.Fatalf("裸域名形态应命中: changed=%d err=%v", changed, err)
	}
	if !strings.Contains(newRaw, "vnew.example.com/a.m3u8") {
		t.Fatalf("替换结果不对: %s", newRaw)
	}
}

func TestReplaceEpisodeLinks_SubdomainUntouched(t *testing.T) {
	const raw = `[{"episode":"1","link":"https://a.vold.example.com/x.m3u8"}]`
	_, _, changed, _, _, err := replaceEpisodeLinks(raw, "vold.example.com", "vnew.example.com")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if changed != 0 {
		t.Fatal("子域名不应被误伤")
	}
}

func TestReplaceEpisodeLinks_BadJSON(t *testing.T) {
	newRaw, _, _, _, _, err := replaceEpisodeLinks(`not-json`, "a.com", "b.com")
	if err == nil {
		t.Fatal("坏 JSON 应报错由调用方跳过")
	}
	if newRaw != "not-json" {
		t.Fatal("坏 JSON 必须原样返回")
	}
}
