package service

import (
	"testing"

	"server/internal/spider"
)

// pickExactHits: 仅保留归一化片名(NormalizeName)与关键字完全相同的命中。
func TestPickExactHits(t *testing.T) {
	hits := []spider.SearchHit{
		{SourceVodID: 1, Name: "凡人修仙传"},
		{SourceVodID: 2, Name: "凡人修仙传 第一季"}, // 季后缀归一化为"凡人修仙传季" → 不等
		{SourceVodID: 3, Name: "凡人修仙传（重制版）"}, // 别名/括号 → 不等
		{SourceVodID: 4, Name: "凡人修仙传 "},     // 仅尾空格 → 归一化后相等
	}
	got := pickExactHits(hits, "凡人修仙传")
	if len(got) != 2 {
		t.Fatalf("精确命中应为 2(id 1 与 4), got %d: %+v", len(got), got)
	}
	if got[0].SourceVodID != 1 || got[1].SourceVodID != 4 {
		t.Fatalf("精确命中顺序/内容不符: %+v", got)
	}
}

// 关键字本身带空格也应归一化后比较(搜索口径与匹配口径一致)。
func TestPickExactHits_KeywordNormalized(t *testing.T) {
	hits := []spider.SearchHit{
		{SourceVodID: 1, Name: "庆余年"},
		{SourceVodID: 2, Name: "庆余年2"},
	}
	got := pickExactHits(hits, "  庆余年  ")
	if len(got) != 1 || got[0].SourceVodID != 1 {
		t.Fatalf("带空格关键字应归一化后精确匹配 id 1, got %+v", got)
	}
}

// 无任何精确命中(仅模糊)→ 空(上层据此返回 candidates 交前端选)。
func TestPickExactHits_NoneExact(t *testing.T) {
	hits := []spider.SearchHit{
		{SourceVodID: 1, Name: "斗罗大陆动画版"},
		{SourceVodID: 2, Name: "斗罗大陆真人版"},
	}
	if got := pickExactHits(hits, "斗罗大陆"); len(got) != 0 {
		t.Fatalf("无精确命中应为空, got %+v", got)
	}
}
