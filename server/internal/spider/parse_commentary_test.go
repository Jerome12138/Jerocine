package spider

import (
	"testing"

	"server/internal/domain"
)

// TestBuildPlaySourcesCommentary 钉住 O6（2026-11-18 用户定）：
//
// 解说片**不得**与正片合并 ——
//  1. 不生成豆瓣 key（否则会按正片 douban id 归并，实测 300/324 解说 mk 带正片豆瓣 id）；
//  2. 不写 `source_douban_id`（清掉"正片豆瓣号"，防后续按豆瓣 id 再次污染）；
//  3. 只保留名字键 ⇒ 解说独立成片（作为单独片名呈现）。
//
// 判定同时看 **TypeName 与 VodName**（`[电影解说]` 有时只在片名里）。
func TestBuildPlaySourcesCommentary(t *testing.T) {
	playURL := "第01集$https://x.tv/1.m3u8#第02集$https://x.tv/2.m3u8"

	cases := []struct {
		name     string
		vodName  string
		typeName string
	}{
		{name: "TypeName 含解说", vodName: "某片", typeName: "电影解说"},
		{name: "片名含解说（TypeName 是普通电影）", vodName: "某片[电影解说]", typeName: "电影"},
		{name: "片名含 [解说] 方括号", vodName: "某片[解说]", typeName: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := filmDetail{
				VodID:       123,
				VodName:     c.vodName,
				TypeName:    c.typeName,
				VodDouBanID: 456780,
				VodPlayFrom: "lzm3u8",
				VodPlayURL:  playURL,
			}
			rows := buildPlaySources(d, "src_test", nil)
			if len(rows) == 0 {
				t.Fatalf("解说（slave）应保留名字键行，实际 0 行")
			}
			nameKey := domain.GenerateHashKey(d.VodName)
			dbKey := domain.GenerateHashKey(int64(d.VodDouBanID))
			for _, r := range rows {
				if r.MatchKey == dbKey {
					t.Fatalf("解说不得生成豆瓣 key（会与正片合并）: %s", r.MatchKey)
				}
				if r.MatchKey != nameKey {
					t.Fatalf("解说的 match_key 应只有名字键: got=%s want=%s", r.MatchKey, nameKey)
				}
				if r.SourceDoubanID != nil {
					t.Fatalf("解说必须清空 source_douban_id，实际 = %d", *r.SourceDoubanID)
				}
			}
		})
	}

	// master 场景（传入 mid）：解说仍按**独立片**处理 ⇒ mid 必须被置空，不得挂正片
	dMid := filmDetail{
		VodID: 123, VodName: "某片[电影解说]", TypeName: "电影", VodDouBanID: 456780,
		VodPlayFrom: "lzm3u8", VodPlayURL: playURL,
	}
	mid := int64(7)
	rowsMid := buildPlaySources(dMid, "src_lz", &mid)
	if len(rowsMid) == 0 {
		t.Fatal("解说不应被丢弃（必须作为独立片存在）")
	}
	for _, r := range rowsMid {
		if r.Mid == nil || *r.Mid != 7 {
			t.Fatalf("解说若本就有自己的影片(mid)，应保留归属，实际 %v", r.Mid)
		}
		if r.SourceDoubanID != nil {
			t.Fatalf("解说必须清空 source_douban_id")
		}
	}

	// 对照组：非解说仍保持双写（名字键 + 豆瓣键）与豆瓣字段
	d := filmDetail{
		VodID:       123,
		VodName:     "普通片",
		TypeName:    "电影",
		VodDouBanID: 456780,
		VodPlayFrom: "lzm3u8",
		VodPlayURL:  playURL,
	}
	rows := buildPlaySources(d, "src_test", nil)
	if len(rows) != 2 {
		t.Fatalf("非解说（slave + 豆瓣ID）应仍有 2 行，实际 %d", len(rows))
	}
	foundDB := false
	for _, r := range rows {
		if r.SourceDoubanID == nil || *r.SourceDoubanID != 456780 {
			t.Fatalf("非解说应保留 source_douban_id，实际 %v", r.SourceDoubanID)
		}
		if r.MatchKey == domain.GenerateHashKey(int64(456780)) {
			foundDB = true
		}
	}
	if !foundDB {
		t.Fatal("非解说应仍生成豆瓣 key（对照被误伤）")
	}
}
