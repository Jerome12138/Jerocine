package mysql

import (
	"fmt"
	"hash/fnv"

	"server/internal/domain/entity"
)

// EpisodesHash 源行集内容的 16 位十六进制指纹（fnv64a，对 `episode \x01 link \x02` 序列化）。
//
// 用途（P1-1 / F-05，2026-11-10）：派生任务只比 `movie_play_source.ep_hash` 就知道
// "该源行的集内容变没变"，从而**不回表读 1GB 的 episodes_json 巨列**（现有工程惯例：
// 列表查询一律只投影四列绕开巨列）。
//
// 注意：fingerprint 只用于**变更检测**，不承载身份语义；空集也返回稳定值（非空串）。
func EpisodesHash(eps entity.EpisodeList) string {
	h := fnv.New64a()
	for _, e := range eps {
		_, _ = h.Write([]byte(e.Episode))
		_, _ = h.Write([]byte{0x01})
		_, _ = h.Write([]byte(e.Link))
		_, _ = h.Write([]byte{0x02})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}
