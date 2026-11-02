/**
 * 弹层栈 — 修复叠层弹窗两个结构性问题(复核 #7):
 *   1) ESC 全关: 多层弹窗同时开着时, 每层各自监听 Escape 全部响应 → 改为
 *      「ESC 只归属最顶层」, 由 isTopOverlay(seq) 判定。
 *   2) body 单槽互踩: data-jc-modal-open 标志与 overflow 滚动锁由各弹窗直接
 *      set/remove → 底层关闭会把顶层的锁一起摘掉。改为计数式: 首层申请时加锁,
 *      全部释放(计数归零)才解锁; 乱序释放计数仍正确。
 *
 * 接入约定(三个弹层组件同规则):
 *   打开时   mySeq = acquireOverlay()
 *   关闭时   releaseOverlay(mySeq); mySeq = null   (onBeforeUnmount 同样兜底)
 *   keydown  e.key === 'Escape' && visible && closeOnEsc && isTopOverlay(mySeq)
 *   (归属判断由栈承担, 组件内不再 stopPropagation)
 */

/** 未释放的层序集合(有序性不依赖插入顺序, 用最大序号判顶层) */
const active = new Set<number>()
/** 层序号发牌器, 单调递增: 后申请者序号恒更大 */
let nextSeq = 0

function lockBody(): void {
  if (typeof document === 'undefined') return
  document.body.setAttribute('data-jc-modal-open', '1')
  document.body.style.overflow = 'hidden'
}

function unlockBody(): void {
  if (typeof document === 'undefined') return
  document.body.removeAttribute('data-jc-modal-open')
  document.body.style.overflow = ''
}

/** 申请一层弹层: 计数 +1; 首层申请时加 body 标志与滚动锁(已锁时不重复设置)。返回本层序号(从 1 开始) */
export function acquireOverlay(): number {
  nextSeq += 1
  const seq = nextSeq
  const wasEmpty = active.size === 0
  active.add(seq)
  if (wasEmpty) lockBody()
  return seq
}

/** 释放本层: 计数 -1; 归零时清理 body 标志与滚动锁。未持有的 seq 为幂等 no-op */
export function releaseOverlay(seq: number): void {
  if (!active.has(seq)) return
  active.delete(seq)
  if (active.size === 0) unlockBody()
}

/** 当前未释放层数 */
export function overlayCount(): number {
  return active.size
}

/** seq 是否为当前最顶层(未释放层中序号最大者); 未持有的 seq 恒 false */
export function isTopOverlay(seq: number): boolean {
  if (!active.has(seq)) return false
  let max = 0
  for (const s of active) {
    if (s > max) max = s
  }
  return seq === max
}
