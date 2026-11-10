package mysql

import (
	"testing"

	"server/internal/domain/entity"
)

// P1-1 / F-05（2026-11-10）：ep_hash 只做"变更检测"——
// 内容相同必须同指纹（否则派生任务会反复重算）、内容变了必须不同（否则静默漏派）。
func TestEpisodesHash(t *testing.T) {
	a := entity.EpisodeList{{Episode: "第1集", Link: "u1"}, {Episode: "第2集", Link: "u2"}}
	b := entity.EpisodeList{{Episode: "第1集", Link: "u1"}, {Episode: "第2集", Link: "u2"}}
	if EpisodesHash(a) != EpisodesHash(b) {
		t.Fatal("同内容应得到同指纹（否则派生任务反复重算）")
	}
	if len(EpisodesHash(a)) != 16 {
		t.Fatalf("应为 16 位十六进制, got %q", EpisodesHash(a))
	}
	// link 变（源站换 CDN / 防盗链签名）⇒ 指纹必须变，否则派生任务不会重建
	c := entity.EpisodeList{{Episode: "第1集", Link: "u1"}, {Episode: "第2集", Link: "u9"}}
	if EpisodesHash(a) == EpisodesHash(c) {
		t.Fatal("link 变了指纹应随之改变")
	}
	// 集名变（真正影响身份）⇒ 指纹必须变
	d := entity.EpisodeList{{Episode: "第1集", Link: "u1"}, {Episode: "第3集", Link: "u2"}}
	if EpisodesHash(a) == EpisodesHash(d) {
		t.Fatal("集名变了指纹应随之改变")
	}
	// 顺序变 ⇒ 指纹变（数组顺序即集序，属实质变化）
	e := entity.EpisodeList{{Episode: "第2集", Link: "u2"}, {Episode: "第1集", Link: "u1"}}
	if EpisodesHash(a) == EpisodesHash(e) {
		t.Fatal("集序变了指纹应随之改变")
	}
	// 空集也要有稳定非空值（派生任务需要"空 vs 未派生"可区分）
	if EpisodesHash(nil) == "" || EpisodesHash(nil) != EpisodesHash(entity.EpisodeList{}) {
		t.Fatal("空集应得到稳定非空指纹")
	}
}
