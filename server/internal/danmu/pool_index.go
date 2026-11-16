package danmu

import (
	"sort"
	"strings"
)

// ===== 池索引（2026-11-16，性能）=====
//
// 问题：D4（桥接）/ D5（锚点）都在**片内**对"本片所有源的所有集名"（池）做**线性扫描**：
//   - `BridgeNum`：为找「同日 + 同栏目/变体 + 主次相容」的候选，逐个池元素判 ⇒ 片内 O(E×P)
//   - `AnchorNumForKey`：**每次调用都重建**「Series → 「上」的日期集合」（O(P) + 每次分配）⇒ 片内 O(E×P)
// 池的**解析**已在 2026-11-15 缓存（`ruleCtx.poolKeysOf`），但**扫描**没缓存。
//
// 实测（真库只读，2026-11-16）：Σ(Eb×P) = **4.26 亿次**比较；改索引后 Σ(Eb+P) = **581 万** ⇒ **≈73×**；
// 最坏单片 **3,900 万次**（MEGA 综艺，正是"单行磨 1 分钟"的那批）。
//
// 本文件只加索引、**不改语义**：桶内仍逐条 `partCompatible`；日期计数仍是"≤ 本集日期的个数"。
// 等价性由 `TestPoolIndex_EquivalentToLinear`（随机池 × 随机查询，逐例与线性版对照）钉住。

// poolIndex 池索引（每个 `ruleCtx` 惰性建一次，与 `poolKeysOf` 同构）。
type poolIndex struct {
	// bridge：key = Date\x00Series\x00Variant → 该桶里**自带期号且有日期**的池元素
	bridge map[string][]EpisodeKey
	// anchor：Series → 该 Series 下所有「上」的**日期**（升序去重）
	anchor map[string][]string
}

// newPoolIndex 由已解析的池键建索引（O(P)）。
func newPoolIndex(poolKeys []EpisodeKey) *poolIndex {
	ix := &poolIndex{
		bridge: make(map[string][]EpisodeKey, len(poolKeys)/4+1),
		anchor: make(map[string][]string, len(poolKeys)/8+1),
	}
	anchorSet := make(map[string]map[string]struct{}, len(poolKeys)/8+1)
	for _, k := range poolKeys {
		// 桥接候选：**自带期号 ∧ 有日期**（与 BridgeNum 的守卫逐字一致）
		if k.Num > 0 && k.Date != "" {
			key := k.Date + "\x00" + k.Series + "\x00" + k.Variant
			ix.bridge[key] = append(ix.bridge[key], k)
		}
		// 锚点日期：**无变体 ∧ 主次以「上」开头 ∧ 有日期**（与 AnchorNumForKey 的守卫一致）
		if k.Variant == "" && strings.HasPrefix(k.Part, "上") && k.Date != "" {
			set := anchorSet[k.Series]
			if set == nil {
				set = map[string]struct{}{}
				anchorSet[k.Series] = set
			}
			set[k.Date] = struct{}{}
		}
	}
	for series, set := range anchorSet {
		ds := make([]string, 0, len(set))
		for d := range set {
			ds = append(ds, d)
		}
		sort.Strings(ds)
		ix.anchor[series] = ds
	}
	return ix
}

// bridgeNum 与 `BridgeNum(cur, poolKeys)` **逐字等价**（只少了"扫全池"）：
// 候选只可能来自同 (Date, Series, Variant) 桶；主次相容仍在桶内逐条判；**候选期号不唯一 ⇒ 0**。
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

// anchorNum 与 `AnchorNumForKey(title, k, poolKeys)` 的池循环部分**逐字等价**：
// 前置守卫（无日期 / 有变体 / 裸日期行且无 p ⇒ 不猜）与"「上」日期 <2 个 ⇒ 不猜"保持一致；
// 计数 = 该 Series 下「上」的日期中 **≤ 本集日期** 的个数（日期已升序 ⇒ 二分）。
func (ix *poolIndex) anchorNum(title string, k EpisodeKey) int {
	if ix == nil || k.Date == "" || k.Variant != "" {
		return 0
	}
	// R9/R10：裸日期行（无 p）不猜 —— 与 AnchorNumForKey 同款守卫（宁缺勿错）
	if k.Part == "" && reBareDateRow.MatchString(strings.TrimSpace(title)) {
		return 0
	}
	dates := ix.anchor[k.Series]
	if len(dates) < 2 {
		return 0
	}
	// 第一个 > k.Date 的下标 = 「≤ k.Date」的个数
	return sort.Search(len(dates), func(i int) bool { return dates[i] > k.Date })
}
