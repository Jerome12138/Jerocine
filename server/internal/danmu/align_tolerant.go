package danmu

// ===== M4 · 错位容忍对齐判据（2026-11-16）=====
//
// 严格口径（现状）按 `ep_index` **位置**比较 ⇒ 某个源"头部多一集 / 中间缺几集"会让整片
// 每一集都算分歧（**整体错位型**：实测 `dv≥3` 的失败片里 76.8% 无集名病症，属这类）。
//
// 本文件的**容忍口径**只允许「插入 / 删除」（多出 / 缺失），**绝不允许「替换」**
// —— 这是为挡住"A 缺 x、B 缺 y"被误判成对齐（用户 2026-11-16 明确要求）：
//
//	A=[1..N]    B=[2..N+1]   整体偏移      ⇒ 对齐（h=1 后重叠区完全一致，边界各多一条）
//	A=[1,2,3,4] B=[1,2,4]    中间缺失      ⇒ 对齐（跳过后能重新对齐 ⇒ 判为单边缺失）
//	A=[1,2,3]   B=[1,2,4]    A缺x/B缺y     ⇒ **不齐**（`3` 与 `4` 互不存在于对方剩余 ⇒ 同位置替换）
//
// ⚠️ 判据只吃"值序列"，**不含日期/期号语义**：调度方决定身份用什么（建议直接给 `epid` 串）。
// ⚠️ 骨架取**最长源**而不是主源 —— 主源常常不是最长，拿短的主源当骨架会把其它源全判成"多出"
// 而"多出无害" ⇒ 判据失效、漏判（用户 2026-11-16 拍板）。主源另作"必备相容方"由调用方保证。

// maxTolerantShift 头部/尾部允许的**整体位移**上限（集）。
const maxTolerantShift = 5

// TolerantAligned 判定两条身份序列是否"仅差插入/删除"（多出 / 缺失），实现 = 枚举位移 + 双指针。
// 任一侧为空 ⇒ false（无从判定；调用方应跳过空序列）。
func TolerantAligned(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	tol := tolerantBudget(len(a), len(b))
	for h := -maxTolerantShift; h <= maxTolerantShift; h++ {
		skipped := 0
		var x, y []string
		switch {
		case h > 0:
			if h >= len(a) {
				continue
			}
			x, y, skipped = a[h:], b, h
		case h < 0:
			if -h >= len(b) {
				continue
			}
			x, y, skipped = a, b[-h:], -h
		default:
			x, y = a, b
		}
		if d := insertDeleteDiffs(x, y); d >= 0 && skipped+d <= tol {
			return true
		}
	}
	return false
}

// insertDeleteDiffs 双指针走两条序列，返回"插/删步数"；出现**同位置替换**时返回 -1。
//
//	相同                              ⇒ 前进
//	**B 当前值在 A 的剩余里**（B[j] ∈ a[i+1:]）⇒ B 只是"晚一步" ⇒ 判 **A 多出 a[i]**（跳过 A）
//	**A 当前值在 B 的剩余里**（a[i] ∈ b[j+1:]）⇒ A 只是"晚一步" ⇒ 判 **B 多出 b[j]**（跳过 B）
//	两者都不在对方剩余里（**互不存在，即"同位置替换"**）⇒ -1（不齐）
//
// ⚠️ 顺序要紧：先看"对方剩余里有没有我"，否则 `[n1,n2,n3,n4] vs [n1,n2,n4]`（B 缺 n3）会被
// 误判为"B 多出 n4" ⇒ 差异翻倍、好片被判不齐（单测 `中间缺失` 抓过这个方向 bug）。
// 而 `[n1,n2,n3] vs [n1,n2,n4]`（A 缺 x / B 缺 y）两侧都"对方剩余里没有" ⇒ 落到最后一条 ⇒ 判不齐 ✓
func insertDeleteDiffs(a, b []string) int {
	i, j, diffs := 0, 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		if containsFrom(a, i+1, b[j]) {
			i++ // B 的当前值在 A 后面 ⇒ A 这一条是多出来的
			diffs++
			continue
		}
		if containsFrom(b, j+1, a[i]) {
			j++ // A 的当前值在 B 后面 ⇒ B 这一条是多出来的
			diffs++
			continue
		}
		return -1 // 同位置替换 ⇒ 冲突
	}
	return diffs + (len(a) - i) + (len(b) - j)
}

func containsFrom(s []string, from int, v string) bool {
	for i := from; i < len(s); i++ {
		if s[i] == v {
			return true
		}
	}
	return false
}

// tolerantBudget 允许的差异额度：`max(2, ceil(5% × max(|a|,|b|)))`。
// 至少 2：头部多一集 + 尾部多一集是常见形态（偏移 1 会同时产生两侧各 1 条边界差）；
// 上限 5%：防止"两边几乎完全不同但恰好相容"的退化被算成对齐。
func tolerantBudget(la, lb int) int {
	n := la
	if lb > n {
		n = lb
	}
	t := (n*5 + 99) / 100
	if t < 2 {
		t = 2
	}
	return t
}

// TolerantMkAligned 判**一个片**（多源）是否"容忍对齐"（M4 的指标侧用法）。
//
// 入参：
//
//	seqs    —— 每个源一条身份序列（按**该源自己的** ep_index 升序；元素建议直接用 epid 串）。
//	           空序列（该源没有身份行）不参与判定。
//	mainIdx —— "主源"在 seqs 中的下标（**无主源传 -1**）。
//
// 规则（用户 2026-11-16 拍板）：
//  1. **骨架取最长源**（并列时取下标最小者 ⇒ 结果确定、可复现），**不是**取主源 ——
//     主源常常不是最长，拿短的主源当骨架会把其它源全判"多出"，而"多出无害"⇒ 判据失效、漏判。
//  2. 其余**每个非空源**都必须与骨架 `TolerantAligned`（`mainIdx ≥ 0` 时主源也在其中
//     ⇒ "主源必备相容方"自动成立；主源为空则同其它空源一并跳过）。
//  3. 非空源少于 2 个 ⇒ **判不齐**（单源无"跨源对齐"可言，容忍口径不替它背书 —— 这点与
//     严格口径的"单源槽默认对齐"有意不同，调用方若要与严格口径可比，请只统计有主源的片）。
//
// 返回 `(对齐?, 骨架下标, 未通过的源下标)`。
func TolerantMkAligned(seqs [][]string, mainIdx int) (bool, int, []int) {
	skeleton, nonEmpty := -1, 0
	for i, s := range seqs {
		if len(s) == 0 {
			continue
		}
		nonEmpty++
		if skeleton < 0 || len(s) > len(seqs[skeleton]) {
			skeleton = i
		}
	}
	if nonEmpty < 2 || skeleton < 0 {
		return false, skeleton, nil
	}
	bad := make([]int, 0, len(seqs))
	for i, s := range seqs {
		if i == skeleton || len(s) == 0 {
			continue
		}
		if !TolerantAligned(seqs[skeleton], s) {
			bad = append(bad, i)
		}
	}
	_ = mainIdx // 见 doc 第 2 条：主源已在上面被逐一比较，无需单独分支
	return len(bad) == 0, skeleton, bad
}
