package danmu

import (
	"strconv"
)

// ===== 池索引（2026-11-16，性能；2026-11-22 D10 改造为「双锚互鉴」两张映射）=====
//
// 问题：借用类规则在**片内**对"本片所有源的所有集名"（池）做**线性扫描**：
//   - `BridgeNum`（D4）：为找「同日 + 同栏目/变体 + 主次相容」的候选，逐个池元素判 ⇒ 片内 O(E×P)
// 池的**解析**已在 2026-11-15 缓存（`ruleCtx.poolKeysOf`），但**扫描**没缓存。
//
// 实测（真库只读，2026-11-16）：Σ(Eb×P) = **4.26 亿次**比较；改索引后 Σ(Eb+P) = **581 万** ⇒ **≈73×**；
// 最坏单片 **3,900 万次**（MEGA 综艺，正是"单行磨 1 分钟"的那批）。
//
// D10（2026-11-22）改造：**撤销 A2 锚点计数推 n**（`AnchorNumForKey`/`poolIndex.anchorNum` 删除——
// 审计可对照面乱对率 51.8%、96.6% 不可验证），替代规则 = **双锚互鉴**：片内「双锚行」（自带期号 ∧
// 自带日期）建**两张复合键映射**，构造时从池键一次建好（O(P)）：
//   - **正查** `bridge`：(Date, Series, Variant) 桶 → 双锚行（复合键第 4 维 Part 由条目携带，
//     查询按 `partCompatible` 相容；空 = 通配、上/下严格）—— 给 d-only 行借 n；
//     语义与既有桥接（D4）完全同构 ⇒ `bridgeNum` 即正查入口（方法名保留，口径不变）。
//   - **反查** `rev`：(Series, Variant, Num) 桶 → 双锚行（Part 由条目携带；v2 dual-tight 起
//     查询按 `partStrictEqual` 严格相等过滤，双侧空算相等）—— 给 n-only 行借 d；
//     入口 `dualAnchorDate`（新增）。
// 四守卫（两方向同款）：①同复合键 ≥2 个不同值 ⇒ 放弃；②part 进复合键（同日双期上下不互借错）；
// ③类隔离（同 Series 同 Variant，由桶键保证）；④片内无双锚行 ⇒ 映射为空 ⇒ 不借（宁缺勿错）。
// 等价性由 `TestPoolIndex_EquivalentToLinear`（随机池 × 随机查询，逐例与线性版对照）钉住。

// poolIndex 池索引（每个 `ruleCtx` 惰性建一次，与 `poolKeysOf` 同构）。
type poolIndex struct {
	// 双锚互鉴·正查映射：key = Date\x00Series\x00Variant → 桶内**双锚行**（自带期号且有日期）；
	// 复合键第 4 维 Part 在条目上，查询时按 partCompatible 相容过滤（与 BridgeNum 守卫逐字一致）。
	bridge map[string][]EpisodeKey
	// 双锚互鉴·反查映射：key = Series\x00Variant\x00Num → 桶内**双锚行**；
	// 查询（dualAnchorDate）在桶内按 partStrictEqual 严格相等过滤（v2 dual-tight），候选日期唯一才借。
	rev map[string][]EpisodeKey
}

// newPoolIndex 由已解析的池键建索引（O(P)）：**双锚行**（Num>0 ∧ Date≠""）同时进两张映射。
func newPoolIndex(poolKeys []EpisodeKey) *poolIndex {
	ix := &poolIndex{
		bridge: make(map[string][]EpisodeKey, len(poolKeys)/4+1),
		rev:    make(map[string][]EpisodeKey, len(poolKeys)/4+1),
	}
	for _, k := range poolKeys {
		if k.Num > 0 && k.Date != "" {
			// 正查桶：与 BridgeNum 的守卫逐字一致（自带期号 ∧ 有日期）
			key := k.Date + "\x00" + k.Series + "\x00" + k.Variant
			ix.bridge[key] = append(ix.bridge[key], k)
			// 反查桶：复合键 (Series, Variant, Num)
			rkey := k.Series + "\x00" + k.Variant + "\x00" + strconv.Itoa(k.Num)
			ix.rev[rkey] = append(ix.rev[rkey], k)
		}
	}
	return ix
}

// bridgeNum = 双锚互鉴·**正查**，与 `BridgeNum(cur, poolKeys)` **逐字等价**（只少了"扫全池"）：
// 候选只可能来自同 (Date, Series, Variant) 桶；主次相容仍在桶内逐条判；**候选期号不唯一 ⇒ 0**。
// （0 = 不借；调用方回退 D6-none —— A2 锚点已于 D10 退役。）
func (ix *poolIndex) bridgeNum(cur EpisodeKey) int {
	if ix == nil || cur.Date == "" {
		return 0
	}
	bucket := ix.bridge[cur.Date+"\x00"+cur.Series+"\x00"+cur.Variant]
	if len(bucket) == 0 {
		return 0
	}
	seen := make(map[int]struct{}, len(bucket))
	for _, k := range bucket {
		if !partCompatible(cur, k) {
			continue
		}
		seen[k.Num] = struct{}{}
	}
	if len(seen) == 1 {
		for n := range seen {
			return n
		}
	}
	return 0
}

// dualAnchorDate 双锚互鉴·**反查**（D10 新增），与线性版 `DualAnchorDate(cur, poolKeys)` **逐字等价**：
// n-only 行（自带期号、无日期）按复合键 (Series, Variant, Num) 桶找双锚行，桶内按 partStrictEqual
// 严格相等过滤（v2 dual-tight，2026-11-23：v1 partCompatible 的「空=通配」会把 p 空 borrower
// 错配到 p=上/下 donor）；**候选日期不唯一 ⇒ 放弃（""）**。返回 "" = 不借（调用方保留 self 身份，n 不变）。
func (ix *poolIndex) dualAnchorDate(cur EpisodeKey) string {
	if ix == nil || cur.Num <= 0 || cur.Date != "" {
		return ""
	}
	bucket := ix.rev[cur.Series+"\x00"+cur.Variant+"\x00"+strconv.Itoa(cur.Num)]
	if len(bucket) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(bucket))
	for _, k := range bucket {
		if !partStrictEqual(cur, k) {
			continue
		}
		seen[k.Date] = struct{}{}
	}
	if len(seen) == 1 {
		for d := range seen {
			return d
		}
	}
	return ""
}
