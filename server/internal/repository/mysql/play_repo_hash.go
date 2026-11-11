package mysql

import (
	"fmt"
	"hash/fnv"

	"server/internal/domain/entity"
)

// EpisodesHash 源行**集名序列**的 16 位十六进制指纹（fnv64a，对 `episode \x01` 序列化）。
//
// 用途（P1-1 / F-05）：派生任务只比 `movie_play_source.ep_hash` 就知道"该源行的**集名**变没变"，
// 从而**不回表读 1GB 的 episodes_json 巨列**。
//
// ⚠️ R4（2026-11-11 评审）：**不把 `link` 计入指纹** —— 源站换 CDN / 防盗链签名轮换会让 URL 天天变，
// 若计入 ⇒ **一次重采把全部 44 万源行同时打脏** ⇒ 派生风暴 ✗。而派生表的内容（`epid`/`raw_name`）
// 完全**不依赖 link** ⇒ link 变化本就不需要重建 ✓
//
// 注意：fingerprint 只用于**变更检测**，不承载身份语义；空集也返回稳定值（非空串）。
func EpisodesHash(eps entity.EpisodeList) string {
	h := fnv.New64a()
	for _, e := range eps {
		_, _ = h.Write([]byte(e.Episode))
		_, _ = h.Write([]byte{0x01})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}
