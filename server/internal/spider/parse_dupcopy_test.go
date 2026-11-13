package spider

import (
	"testing"

	"server/internal/domain"
)

// TestBuildPlaySourcesMarksDupCopy 防呆：双写两份必须打上正确的 `dup_copy`
//   - **0 = 片名键**（`hash(d.VodName)` 生成的 `keys[0]`）⇒ 主份；
//   - **1 = 豆瓣键**（`hash(d.VodDouBanID)` 生成）⇒ 副本。
//
// 为什么必须钉住：对齐率指标靠派生表继承来的 `dup_copy = 0` **排除双写副本**，才能消除"虚高"
// （实测含副本 35.16% vs 排除副本 32.83%）。写侧**不做任何判断**，只是把"这个键的来源"顺手记下来；
// 一旦打反或漏打，口径静默失真 ✗（存量由离线脚本按 `movie.db_id` 近似补齐，判不了的不猜）。
func TestBuildPlaySourcesMarksDupCopy(t *testing.T) {
	playURL := "第01集$https://x.tv/1.m3u8#第02集$https://x.tv/2.m3u8"
	base := filmDetail{
		VodID:       123,
		VodName:     "某某剧",
		VodPlayFrom: "lzm3u8",
		VodPlayURL:  playURL,
	}
	nameKey := domain.GenerateHashKey(base.VodName)

	// ① slave 且上游带豆瓣ID ⇒ 双写两行：片名键 dup=0、豆瓣键 dup=1
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
	if len(master) != 1 || master[0].DupCopy != 0 {
		t.Fatalf("master 应只写片名键一份且 dup=0（实际 %d 行）", len(master))
	}

	// ③ slave 但上游**没有**豆瓣ID ⇒ 只有片名键一份（无副本）
	noDB := base
	noDB.VodDouBanID = 0
	single := buildPlaySources(noDB, "src_test", nil)
	if len(single) != 1 || single[0].DupCopy != 0 {
		t.Fatalf("slave 无豆瓣ID 应只写 1 行且 dup=0（实际 %d 行）", len(single))
	}

	// ④ 多线路：键 × 线路各自成行，标记按"键的来源"（与线路无关）
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
