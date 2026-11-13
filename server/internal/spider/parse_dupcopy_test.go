package spider

import (
	"testing"

	"server/internal/domain"
)

// TestBuildPlaySourcesMarksDupCopy 防呆：双写两份必须打上正确的 `dup_copy`
// （0 = 片名键**主份** / 1 = 豆瓣键**副本**）。
//
// 为什么必须钉住：对齐率系列指标靠 `WHERE dup_copy = 0` **排除双写副本**来消除"虚高"
// （见 docs/epid-集身份/分析-采集侧键口径与匹配度-2026-11-13.md §10.5/§11，
// 实测含副本 35.16% vs 排除副本 32.83%）。一旦标记打反或漏打，口径静默失真 ✗。
func TestBuildPlaySourcesMarksDupCopy(t *testing.T) {
	playURL := "第01集$https://x.tv/1.m3u8#第02集$https://x.tv/2.m3u8"
	base := filmDetail{
		VodID:       123,
		VodName:     "某某剧",
		VodPlayFrom: "lzm3u8",
		VodPlayURL:  playURL,
	}

	nameKey := domain.GenerateHashKey(base.VodName)

	// ① slave 且上游带豆瓣ID ⇒ 双写两行（片名键主份 + 豆瓣键副本）
	withDB := base
	withDB.VodDouBanID = 456780
	rows := buildPlaySources(withDB, "src_test", nil)
	if len(rows) != 2 {
		t.Fatalf("slave + 豆瓣ID 应产生 2 行，得到 %d 行", len(rows))
	}
	dbKey := domain.GenerateHashKey(int64(456780))
	got := map[string]int8{}
	for _, r := range rows {
		got[r.MatchKey] = r.DupCopy
		if r.SiteId != "src_test" || r.SourceVodID != 123 {
			t.Errorf("行字段不对：site=%q vod=%d", r.SiteId, r.SourceVodID)
		}
	}
	if d, ok := got[nameKey]; !ok {
		t.Fatalf("缺少片名键 %s（实际键：%v）", nameKey, got)
	} else if d != 0 {
		t.Errorf("片名键主份 dup_copy = %d, want 0", d)
	}
	if d, ok := got[dbKey]; !ok {
		t.Fatalf("缺少豆瓣键 %s（实际键：%v）", dbKey, got)
	} else if d != 1 {
		t.Errorf("豆瓣键副本 dup_copy = %d, want 1", d)
	}

	// ② master（mid != nil）⇒ 只写片名键一份，且是主份
	mid := int64(9)
	master := buildPlaySources(base, "src_lz", &mid)
	if len(master) != 1 {
		t.Fatalf("master 应只写 1 行，得到 %d 行", len(master))
	}
	if master[0].DupCopy != 0 {
		t.Errorf("master 行 dup_copy = %d, want 0", master[0].DupCopy)
	}

	// ③ slave 但上游**没有**豆瓣ID ⇒ 只有片名键一份（无副本）
	noDB := base
	noDB.VodDouBanID = 0
	single := buildPlaySources(noDB, "src_test", nil)
	if len(single) != 1 {
		t.Fatalf("slave 无豆瓣ID 应只写 1 行，得到 %d 行", len(single))
	}
	if single[0].DupCopy != 0 {
		t.Errorf("无豆瓣ID 行 dup_copy = %d, want 0", single[0].DupCopy)
	}

	// ④ 多线路时"键 × 线路"各自标记正确（每键每线路一行）
	multi := withDB
	multi.VodPlayFrom = "lzm3u8$$$liangzi"
	multi.VodPlayURL = playURL + "$$$" + playURL
	mrows := buildPlaySources(multi, "src_test", nil)
	if len(mrows) != 4 {
		t.Fatalf("2 键 × 2 线路 应产生 4 行，得到 %d 行", len(mrows))
	}
	for _, r := range mrows {
		want := int8(0)
		if r.MatchKey == dbKey {
			want = 1
		}
		if r.DupCopy != want {
			t.Errorf("key=%s line=%s dup_copy = %d, want %d", r.MatchKey, r.PlayFrom, r.DupCopy, want)
		}
	}
}
