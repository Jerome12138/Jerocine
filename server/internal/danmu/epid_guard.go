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

// derivedSuspectGraceMs 「疑似」宽限期（L4, 2026-11-11 评审）。
//
// 问题：守卫判"疑似抖动"后不采纳，且不写状态 ⇒ 下一轮仍判脏、仍拦 ⇒ **合法改名 / 下架半季的源行
// 派生数据永久停更** ✗。
// 解法（**零新增状态**）：借已有的 `computed_at`（上次重建时间）—— 距上次重建**已超过本宽限期**仍判
// 疑似 ⇒ 认定是**真实变化**，采纳 ✓；否则继续拦（等下轮）。相当于"等一天仍如此 ⇒ 就是真的" ✓
const derivedSuspectGraceMs = 24 * 60 * 60 * 1000

// ShouldReplaceDerived 是否采纳本次重建。
//   - oldN / newN：该源行现有派生行数（0 = 首次派生）/ 本次重算得到的行数
//   - hasOverlap：新旧集名是否存在交集（调用方查库得出；首次派生可传 false）
//   - lastComputedAt：该源行**上次重建时间**（0 = 未知）   - now：当前毫秒时间戳
//
// 返回 (是否采纳, 原因)。原因仅用于日志/打点（`*-grace` 后缀 = 走宽限期采纳）。
//
// ⚠️ 阈值用**整数运算**（`newN*10 < oldN*6`）而非浮点：`int(float64(oldN)*0.6)` 在 `oldN=1`
// 时会下取整成 0 ⇒「0 < 0」恒假 ⇒ **小 N 源行（单集片/刚采到的短片）60% 阈值完全失效** ✗。
func ShouldReplaceDerived(oldN, newN int, hasOverlap bool, lastComputedAt, now int64) (bool, string) {
	if oldN == 0 {
		return true, "first-derive" // F-07：首次派生直接采纳
	}
	suspect := ""
	if newN*derivedShrinkDenominator < oldN*derivedShrinkNumerator {
		suspect = "suspect-shrink" // 新集数骤减 ⇒ 疑似抖动
	} else if !hasOverlap {
		suspect = "suspect-disjoint" // 新旧集名完全不相交 ⇒ 疑似抓错源/换片
	}
	if suspect == "" {
		return true, "ok"
	}
	if lastComputedAt > 0 && now-lastComputedAt >= derivedSuspectGraceMs {
		return true, suspect + "-grace" // L4：已等过宽限期 ⇒ 认作真变化
	}
	return false, suspect
}
