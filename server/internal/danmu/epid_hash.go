package danmu

// ===== epid 短码（hash 形态，2026-11-16）=====
//
// 用途：给 epid 一个**稳定的纯 ASCII 短形态**，供
//  ① 端侧**优先用它判等/存储**（`epid` 原串备用）
//  ② 顺带解掉"epid 进 URL 被 percent-encode"的问题
//
// **判等语义与 epid 全等完全一致**：hash 是 epid 的纯函数 ⇒ 同串同码、异串异码；
// **不引入任何新的匹配规则**（不做前缀相容、不做单边通配 —— 那些是端侧判等简化要删掉的东西）。
//
// 算法：**FNV-1a 64-bit → base36 小写**（长度 ≤ 13、字符集 `[0-9a-z]`，URL 非保留）。
// ⚠️ **三端同规**：web（`web/src/utils/episodeKey.ts` 的 `epidHash`）、android
// （`player-core/EpisodeKey.java`）必须实现**同一算法**；改算法必须**带版本位**且双读兼容一版。
// 黄金值（跨端对齐锚点）见 `epid_hash_test.go` 的 `EPIDHASH` 日志。
//
// 碰撞：比较恒在**片内**（既有结论：epid 不要求全局唯一）⇒ 64-bit 在片内碰撞概率可忽略；
// 若要更严，可在派生侧加"同片同 (s,v,p) 内短码必须唯一"的校验（冲突即放弃该集身份，不猜）。
const (
	fnvOffset64 = uint64(14695981039346656037)
	fnvPrime64  = uint64(1099511628211)
)

// EpidHash 返回 epid 的短码（FNV-1a 64 → base36 小写）。
func EpidHash(epid string) string {
	h := fnvOffset64
	for i := 0; i < len(epid); i++ {
		h ^= uint64(epid[i])
		h *= fnvPrime64
	}
	return base36(h)
}

// base36 无符号 64 位 → base36 小写（最少 1 位；0 ⇒ "0"）。
func base36(v uint64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if v == 0 {
		return "0"
	}
	var buf [13]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v%36]
		v /= 36
	}
	return string(buf[i:])
}
