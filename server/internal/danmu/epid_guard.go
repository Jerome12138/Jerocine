package danmu

// 抖动守卫（P1-1 / F-07，2026-11-10）。
//
// 背景：采集偶发只抓到一部分集（分页失败/源站抽风）⇒ 若照单重建派生行，会造成**某源 epid 大面积消失**
// ⇒ 弹幕突然全空、切源候选骤减。守卫规则（「宁缺勿错」的延伸）：
//
//	新 N < 旧 N × 60%  **或**  新集名集合与旧**完全不相交**  ⇒  判**疑似抖动** ⇒ **不重建**
//	（标记 + 告警，等下一轮确认：连续 2 轮一致才采纳）
//
// ⚠️ **F-07 首次派生分支**：`oldN == 0`（该源行从未派生过）⇒ **直接采纳**，不触发守卫。
// 否则"旧 N = 0"会把**所有新源行**误判为抖动 ⇒ 派生任务永远空转（静默失效）。
const derivedShrinkNumerator = 6   // 60% = 6/10
const derivedShrinkDenominator = 10

// ShouldReplaceDerived 是否采纳本次重建。
//   - oldN：该源行现有派生行数（0 = 首次派生）
//   - newN：本次重算得到的行数
//   - hasOverlap：新旧集名是否存在交集（调用方查库得出；首次派生时可传 false）
//
// 返回 (是否采纳, 原因)。原因仅用于日志/打点。
//
// ⚠️ 阈值用**整数运算**（`newN*10 < oldN*6`）而非浮点：`int(float64(oldN)*0.6)` 在 `oldN=1`
// 时会下取整成 0 ⇒ "0 < 0" 恒假 ⇒ **小 N 的源行（单集片/刚采到的短片）60% 阈值完全失效** ✗
// （实测由单测 `1→0` 用例捕获）。
func ShouldReplaceDerived(oldN, newN int, hasOverlap bool) (bool, string) {
	if oldN == 0 {
		return true, "first-derive" // F-07：首次派生直接采纳
	}
	if newN*derivedShrinkDenominator < oldN*derivedShrinkNumerator {
		return false, "suspect-shrink" // 新集数骤减 ⇒ 疑似抖动
	}
	if !hasOverlap {
		return false, "suspect-disjoint" // 新旧集名完全不相交 ⇒ 疑似抓错源/换片
	}
	return true, "ok"
}
