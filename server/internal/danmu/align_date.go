package danmu

import "strings"

// ===== 综艺「日期对齐」口径（2026-11-17 立项；**暂未接线**）=====
//
// 背景（实测 2026-11-17，生产库只读；证据见 docs/epid-集身份/索引-观察项总账-2026-11-17.md §4.1）：
//   - 综艺（`pid=3`）占全库「首段差异 = `d`」分歧槽的 **75.9%**（19,936 / 26,256）；
//   - 综艺 289,666 行中 **136,306 行（47.1%）**是「纯日期型」（`s/v/n` 全空、`d` 非空），涉及 1,613 片；
//     这些行现在**没有可用身份**——跨源判等要求逐段全等 ⇒ 日期稍有出入就落进「分歧」。
//
// 口径：对综艺纯日期型集，把「跨源是否同排期」的判据从 **位置（`ep_index`）** 换成 **日期值（`d`）**：
// 两源只要日期集合**仅差一侧多出 / 缺失**即判相容 ⇒ 跨源**位序错位**不再产出伪分歧。
// 这与 M4 容忍口径（`align_tolerant.go`）是同构的——差别只在"身份取什么"：M4 吃值序列，
// 本口径吃日期值。
//
// 守卫（用户 2026-11-17 拍板，均由实测支撑）：
//  1. **组内每源同日 ≤ 1 行** —— 否则**整组放弃**（实测歧义组 9.1%；食尚玩家 17.1% 显著偏高）；
//  2. **≥ 2 个有日期的源**才判（实测单源组 42.2%，无跨源收益）；
//  3. 空日期 / 空源一律**不判**（宁缺勿错；不命中不回退到位置）。
//
// ⚠️ 已知代价：**同日不同期**（一天两期）会被日期合并 ⇒ 需与「同槽 `n` 缺失」联合判定，
// 或在歧义组内回落。在接线前仅作口径登记。
//
// ⚠️ **本口径暂不接线**（用户 2026-11-17 拍板：「读侧不做特殊处理」）：
// 目前只有单测消费；正式启用前需先决定消费层（指标层 or 读侧），并做零迁移评估。

// dateAlignClean 归一化一侧源行的日期序列：任一项为空 / `-`（无日期）⇒ 返回 nil（该源不参与判定）。
// 这样"某源整列无日期"与"某源部分行无日期"都不会被当成"日期不同"而误判成不齐。
func dateAlignClean(seq []string) []string {
	out := make([]string, 0, len(seq))
	for _, d := range seq {
		d = strings.TrimSpace(d)
		if d == "" || d == "-" {
			return nil
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hasDupDate 同一源内是否出现重复日期（= 该源同日 ≥ 2 行）。
func hasDupDate(seq []string) bool {
	seen := make(map[string]struct{}, len(seq))
	for _, d := range seq {
		if _, ok := seen[d]; ok {
			return true
		}
		seen[d] = struct{}{}
	}
	return false
}

// dateSubset 判 short 的每个日期都出现在 long 里（**值**语义，与位置无关）。
func dateSubset(short, long []string) bool {
	set := make(map[string]struct{}, len(long))
	for _, d := range long {
		set[d] = struct{}{}
	}
	for _, d := range short {
		if _, ok := set[d]; !ok {
			return false
		}
	}
	return true
}

// DateAligned 判两源的日期序列是否「同排期」：只看日期**值**，允许一方多出 / 缺失。
// 语义 = 较短一方的日期集合是较长一方的子集。
//   - 一侧多出头部/中间/尾部若干集 ⇒ **相容**（源不全，与 M4 的"插入/删除"一致）；
//   - 交错缺集（A 缺 x / B 缺 y，`x≠y`）⇒ **不相容**（不放过"替换"）；
//   - 任一侧为空 / 含无日期项 ⇒ false（无从判定，宁缺勿错）。
func DateAligned(a, b []string) bool {
	ca, cb := dateAlignClean(a), dateAlignClean(b)
	if len(ca) == 0 || len(cb) == 0 {
		return false
	}
	if len(ca) <= len(cb) {
		return dateSubset(ca, cb)
	}
	return dateSubset(cb, ca)
}

// DateAlignUsable 守卫：组内**每源同日 ≤ 1 行** ∧ **≥ 2 个有日期的源**。
// 不满足 ⇒ 该组整体放弃（不猜、不回退位置口径）。
func DateAlignUsable(perSource [][]string) bool {
	nonEmpty := 0
	for _, seq := range perSource {
		c := dateAlignClean(seq)
		if len(c) == 0 {
			continue // 无日期的源不计数、也不否决
		}
		if hasDupDate(c) {
			return false
		}
		nonEmpty++
	}
	return nonEmpty >= 2
}

// DateAlignVerdict 完整判定：先过守卫，再要求**任两源**都相容。
// ⚠️ 逐对判定而非"与最长源比对"——日期是集合语义，三源两两相容才安全。
func DateAlignVerdict(perSource [][]string) bool {
	if !DateAlignUsable(perSource) {
		return false
	}
	var kept [][]string
	for _, seq := range perSource {
		if c := dateAlignClean(seq); len(c) > 0 {
			kept = append(kept, c)
		}
	}
	for i := 0; i < len(kept); i++ {
		for j := i + 1; j < len(kept); j++ {
			if !DateAligned(kept[i], kept[j]) {
				return false
			}
		}
	}
	return true
}
