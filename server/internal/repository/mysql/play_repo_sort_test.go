package mysql

import (
	"testing"

	"server/internal/domain/entity"
)

// TestSortPlaySourcesForKey 防呆：写侧必须先按 upsert 唯一键 (site_id, match_key, play_from)
// 排序，否则并发 worker 取锁顺序不一致 ⇒ MySQL Error 1213 死锁（见 sortPlaySourcesForKey 注释）。
func TestSortPlaySourcesForKey(t *testing.T) {
	in := []entity.MoviePlaySource{
		{SiteId: "src_b", MatchKey: "2", PlayFrom: "a"},
		{SiteId: "src_a", MatchKey: "9", PlayFrom: "z"},
		{SiteId: "src_a", MatchKey: "9", PlayFrom: "a"},
		{SiteId: "src_a", MatchKey: "1", PlayFrom: "b"},
	}
	sortPlaySourcesForKey(in)

	want := []string{
		"src_a|1|b",
		"src_a|9|a",
		"src_a|9|z",
		"src_b|2|a",
	}
	for i, w := range want {
		got := in[i].SiteId + "|" + in[i].MatchKey + "|" + in[i].PlayFrom
		if got != w {
			t.Fatalf("第 %d 项 = %q, want %q（排序后：%v）", i, got, w, dump(in))
		}
	}
}

// TestSortPlaySourcesForKeyStableForEqualKeys 同一唯一键的多行（同 key 不同线路不会重复，
// 但同 site+key+from 只应有一行）—— 排序不得改变元素个数，空列表安全。
func TestSortPlaySourcesForKeyEmpty(t *testing.T) {
	sortPlaySourcesForKey(nil)
	sortPlaySourcesForKey([]entity.MoviePlaySource{})
}

func dump(l []entity.MoviePlaySource) []string {
	out := make([]string, 0, len(l))
	for _, x := range l {
		out = append(out, x.SiteId+"|"+x.MatchKey+"|"+x.PlayFrom)
	}
	return out
}
