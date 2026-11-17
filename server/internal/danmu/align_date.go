package danmu

import "strings"

// ===== 综艺「日期对齐」口径（2026-11-17 立项；指标层已接线，读侧不接线）=====
//
// 背景（实测 2026-11-17，生产库只读；证据见 docs/epid-集身份/索引-观察项总账-2026-11-17.md §4.1）：
//   - 综艺（`pid=3`）占全库「首段差异 = `d`」分歧槽的 **75.9%**（19,936 / 26,256）；
//   - 综艺 289,666 行中 **136,306 行（47.1%）**是「纯日期型」（`s/v/n` 全空、`d` 非空），涉及 1,613 片；
//     这些行现在**没有可用身份**——跨源判等要求逐段全等 ⇒ 日期稍有出入就落进「分歧」。
//
// 口径：对纯日期型集，把「跨源是否同排期」的判据从 **位置（`ep_index`）** 换成 **日期值（`d`）**，
// 并以 **`(日期, part)` 复合键** 支持「同日多集」：两源只要复合键集合**仅差一侧多出 / 缺失**即判相容
// ⇒ 跨源**位序错位**不再产出伪分歧（与 M4 容忍口径同构，差别只在"身份取什么"）。
//
// 守卫（用户 2026-11-17 拍板，均由实测支撑）：
//  1. **≥ 2 个有日期的源**才判（实测单源组 42.2%，无跨源收益）；
//  2. **同源内复合键必须唯一** —— 同日多行**允许**，只要各行 `part` 互不相同（上 / 中 / 下 / 数字）；
//     只有「同一 (日期, part) 出现 ≥ 2 行」才**整组放弃**（实测原歧义组 9.1%；食尚玩家 17.1% 偏高）；
//  3. 空日期 / 空源一律**不判**（宁缺勿错；不命中不回退到位置口径）。
//
// ⚠️ 已知代价：**同日不同期且 part 缺失**（一天两期但两边都没标上/中/下）会在复合键上撞成同一键 ⇒
// 触发守卫(2) 放弃，这正是「宁缺勿错」的期望行为；若某源整列无 part、另一源带 part，判**不齐**
// （不做"无 part 即通配"的猜测）。
//
// ⚠️ **读侧不接线**（用户 2026-11-17 拍板：「读侧不做特殊处理」）：本口径只供**指标层**
// （`GET /manage/epid/alignment/date`）与单测消费；聚合/挑集路径一律不变。

// DateAlignItem 一行的对齐身份：日期 + 可选 part（`上/中/下` 或 1~2 位数字，可复合如 `中上`）。
type DateAlignItem struct {
	Date string
	Part string
}

// Key 复合键：无 part 即日期本身；有 part 用 `#` 连接（日期是 8 位数字，不会与 `#` 混淆）。
func (it DateAlignItem) Key() string {
	if it.Part == "" {
		return it.Date
	}
	return it.Date + "#" + it.Part
}

// dateAlignItemsFromDates 把纯日期序列包成 items（无 part）。
func dateAlignItemsFromDates(dates []string) []DateAlignItem {
	out := make([]DateAlignItem, 0, len(dates))
	for _, d := range dates {
		out = append(out, DateAlignItem{Date: d})
	}
	return out
}

// dateAlignCleanItems 归一化一侧源行：任一项日期不合法（空 / `-` / 非纯数字）⇒ 返回 nil（该源不参与判定）。
// 这样"某源整列无日期"与"某源部分行无日期"都不会被当成"日期不同"而误判成不齐。
//
// ⚠️ **要求日期为纯数字**（2026-11-17 审查）：`Key()` 以 `#` 拼接，只有保证 `Date` 里不含 `#`，
// 复合键才是**单射**（第一个 `#` 即分隔点）⇒ 否则脏值（如 `d=a#b`）可能与另一组 `(Date,Part)` 撞键，
// 判成"齐"而虚增对齐率 ✗。不满足者该源不参与（宁缺勿错，方向保守）。
func dateAlignCleanItems(items []DateAlignItem) []DateAlignItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]DateAlignItem, 0, len(items))
	for _, it := range items {
		d := strings.TrimSpace(it.Date)
		if !isDigits(d) {
			return nil
		}
		out = append(out, DateAlignItem{Date: d, Part: strings.TrimSpace(it.Part)})
	}
	return out
}

// isDigits 全 ASCII 数字且非空。
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigitByte(s[i]) {
			return false
		}
	}
	return true
}

// dateAlignDupKey 同一源内是否出现重复**复合键**（= 同源同日同 part ≥ 2 行）。
// 同日不同 part 不算重复（这正是"一天两集"的合法形态）。
func dateAlignDupKey(items []DateAlignItem) bool {
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		k := it.Key()
		if _, ok := seen[k]; ok {
			return true
		}
		seen[k] = struct{}{}
	}
	return false
}

// DatePartAligned 判两源的「日期(+part)」序列是否同排期：只看复合键**值**，允许一方多出 / 缺失。
// 语义 = 较短一方的键集合是较长一方的子集。
//   - 一侧多出头部/中间/尾部若干集 ⇒ **相容**（源不全，与 M4 的"插入/删除"一致）；
//   - 交错缺集（A 缺 x / B 缺 y，`x≠y`）⇒ **不相容**（不放过"替换"）；
//   - 一侧带 part、另一侧同日无 part ⇒ **不相容**（不做通配猜测）；
//   - 任一侧为空 / 含非法日期项 ⇒ false（无从判定，宁缺勿错）。
//
// 方向选择按**去重后的集合大小**（2026-11-17 审查）：若按原始长度选，含重复键的一侧会被判成"长"，
// 从而对另一侧施加更严的子集检验 ⇒ 产出假"不齐"（方向保守但属误判）。
func DatePartAligned(a, b []DateAlignItem) bool {
	ca, cb := dateAlignCleanItems(a), dateAlignCleanItems(b)
	if len(ca) == 0 || len(cb) == 0 {
		return false
	}
	sa, sb := dateAlignKeySet(ca), dateAlignKeySet(cb)
	if len(sa) <= len(sb) {
		return dateAlignSetSubset(sa, sb)
	}
	return dateAlignSetSubset(sb, sa)
}

// dateAlignKeySet 复合键集合（天然去重）。
func dateAlignKeySet(items []DateAlignItem) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, it := range items {
		set[it.Key()] = struct{}{}
	}
	return set
}

// dateAlignSetSubset 判 sa ⊆ sb。
func dateAlignSetSubset(sa, sb map[string]struct{}) bool {
	for k := range sa {
		if _, ok := sb[k]; !ok {
			return false
		}
	}
	return true
}

// DateAligned 纯日期（无 part）版本 —— 等价于把输入包成无 part 的 items。
func DateAligned(a, b []string) bool {
	return DatePartAligned(dateAlignItemsFromDates(a), dateAlignItemsFromDates(b))
}

// DateAlignUsableItems 守卫：**≥ 2 个有日期的源** ∧ **每源复合键唯一**。
// 不满足 ⇒ 该组整体放弃（不猜、不回退位置口径）。
func DateAlignUsableItems(perSource [][]DateAlignItem) bool {
	nonEmpty := 0
	for _, items := range perSource {
		c := dateAlignCleanItems(items)
		if len(c) == 0 {
			continue // 无日期的源不计数、也不否决
		}
		if dateAlignDupKey(c) {
			return false
		}
		nonEmpty++
	}
	return nonEmpty >= 2
}

// DateAlignUsable 纯日期版本守卫。
func DateAlignUsable(perSource [][]string) bool {
	out := make([][]DateAlignItem, 0, len(perSource))
	for _, seq := range perSource {
		out = append(out, dateAlignItemsFromDates(seq))
	}
	return DateAlignUsableItems(out)
}

// DateAlignVerdictItems 完整判定：先过守卫，再要求**任两源**都相容。
// ⚠️ 逐对判定而非"与最长源比对"——日期是集合语义，三源两两相容才安全。
func DateAlignVerdictItems(perSource [][]DateAlignItem) bool {
	if !DateAlignUsableItems(perSource) {
		return false
	}
	var kept [][]DateAlignItem
	for _, items := range perSource {
		if c := dateAlignCleanItems(items); len(c) > 0 {
			kept = append(kept, c)
		}
	}
	for i := 0; i < len(kept); i++ {
		for j := i + 1; j < len(kept); j++ {
			if !DatePartAligned(kept[i], kept[j]) {
				return false
			}
		}
	}
	return true
}

// DateAlignVerdict 纯日期版本完整判定。
func DateAlignVerdict(perSource [][]string) bool {
	out := make([][]DateAlignItem, 0, len(perSource))
	for _, seq := range perSource {
		out = append(out, dateAlignItemsFromDates(seq))
	}
	return DateAlignVerdictItems(out)
}
