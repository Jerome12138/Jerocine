package mysql

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// R2 验证（2026-11-11，评审「最终报告」§一 R2）：
//
// `DirtySourceRows` 用 `Raw(...).Scan(&[]entity.MoviePlaySource)` 读 `episodes_json`。
// **GORM 的 `serializer:json` 在 `Raw().Scan` 路径是否生效，静态不可判定** ⇒ 本用例把结论钉死：
//   - 若生效 ⇒ `Episodes` 被正确反序列化 ⇒ 派生可用 ✓
//   - 若不生效 ⇒ `Episodes` 恒空 ⇒ **派生出 0 行且静默**（整套 P1-1 白做）✗ ⇒ 本用例失败即报警
//
// 纯 sqlmock，不需真库；若将来 `DirtySourceRows` 的 SQL 首行变化，本用例同步更新。
func TestDirtySourceRows_EpisodesJSON反序列化必须生效(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewEpidRepository(gdb)

	const jsonPayload = `[{"episode":"第1期上","link":"https://x/1.m3u8"},{"episode":"第2期上","link":"https://x/2.m3u8"}]`
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT s.id, s.mid, s.match_key, s.site_id, s.play_from, s.episodes_json, s.ep_hash")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "mid", "match_key", "site_id", "play_from", "episodes_json", "ep_hash",
		}).AddRow(9, 100, "mk-1", "src_lz", "lzm3u8", jsonPayload, "0123456789abcdef"))

	rows, err := repo.DirtySourceRows(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("DirtySourceRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应返回 1 行, got %d", len(rows))
	}
	r := rows[0]
	// ① 关键断言：episodes_json 必须被反序列化（否则派生池为空 ⇒ 零产出）
	if len(r.Episodes) != 2 {
		t.Fatalf("`episodes_json` 未被反序列化 —— `serializer:json` 在 `Raw().Scan` 路径**未生效**，"+
			"派生会零产出且静默；需要改用 `Find()`（基于 schema）或显式 json.Unmarshal。got Episodes=%#v", r.Episodes)
	}
	if r.Episodes[0].Episode != "第1期上" || r.Episodes[0].Link != "https://x/1.m3u8" {
		t.Fatalf("第 1 集反序列化内容不对: %#v", r.Episodes[0])
	}
	if r.Episodes[1].Episode != "第2期上" {
		t.Fatalf("第 2 集反序列化内容不对: %#v", r.Episodes[1])
	}
	// ② 其余列也必须映射正确（派生要用它们做受控冗余）
	if r.Id != 9 || r.MatchKey != "mk-1" || r.SiteId != "src_lz" || r.PlayFrom != "lzm3u8" || r.EpHash != "0123456789abcdef" {
		t.Fatalf("其它列映射不对: %#v", r)
	}
	if r.Mid == nil || *r.Mid != 100 {
		t.Fatalf("mid 映射不对: %v", r.Mid)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
