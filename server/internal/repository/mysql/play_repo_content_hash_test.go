package mysql

import (
	"testing"

	"server/internal/domain/entity"
)

// TestEpisodesContentHashVsEpisodesHash 防呆：两枚指纹**各司其职，不可互换**
//   - `EpisodesHash`（→ `ep_hash`）**不含 link** ⇒ CDN 轮换不该把它算"变了"（否则一次重采把全库打脏，
//     R4 实测教训：派生风暴）；
//   - `EpisodesContentHash`（→ `content_hash`）**含 link** ⇒ 服务「内容未变不写」判重。
//
// 看护点：一旦有人把它们合并/换用，会分别导致"派生风暴"或"跳过该写的修复型写入"✗。
func TestEpisodesContentHashVsEpisodesHash(t *testing.T) {
	base := entity.EpisodeList{
		{Episode: "第01集", Link: "https://a.tv/1.m3u8"},
		{Episode: "第02集", Link: "https://a.tv/2.m3u8"},
	}

	// ① 完全一致 ⇒ 两枚指纹都相同
	same := entity.EpisodeList{
		{Episode: "第01集", Link: "https://a.tv/1.m3u8"},
		{Episode: "第02集", Link: "https://a.tv/2.m3u8"},
	}
	if EpisodesHash(base) != EpisodesHash(same) {
		t.Error("ep_hash：内容一致却不同")
	}
	if EpisodesContentHash(base) != EpisodesContentHash(same) {
		t.Error("content_hash：内容一致却不同")
	}

	// ② **只换 CDN 链接**：ep_hash **必须不变**（不脏派生）；content_hash **必须变**（要重写）
	relinked := entity.EpisodeList{
		{Episode: "第01集", Link: "https://b.tv/1.m3u8"},
		{Episode: "第02集", Link: "https://b.tv/2.m3u8"},
	}
	if EpisodesHash(base) != EpisodesHash(relinked) {
		t.Error("ep_hash 把 link 变化算进去了（会引发派生风暴）")
	}
	if EpisodesContentHash(base) == EpisodesContentHash(relinked) {
		t.Error("content_hash 没把 link 变化算进去（会跳过该写的更新 ⇒ 链接停在旧的）")
	}

	// ③ 集名变化：两枚都要变
	renamed := entity.EpisodeList{
		{Episode: "第01集", Link: "https://a.tv/1.m3u8"},
		{Episode: "第02集（下）", Link: "https://a.tv/2.m3u8"},
	}
	if EpisodesHash(base) == EpisodesHash(renamed) {
		t.Error("ep_hash 没把集名变化算进去")
	}
	if EpisodesContentHash(base) == EpisodesContentHash(renamed) {
		t.Error("content_hash 没把集名变化算进去")
	}

	// ④ 空集也返回稳定非空串（只作变更检测，不承载身份语义）
	if EpisodesHash(nil) == "" || EpisodesContentHash(nil) == "" {
		t.Error("空集应返回稳定的非空指纹")
	}
}
