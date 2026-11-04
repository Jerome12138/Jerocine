import { onScopeDispose, watch, type Ref } from 'vue'

/**
 * 浮层关闭逻辑（规格 §3.1）：点击 anchor 外部 / ESC 时触发 onClose。
 * anchor 是「包含触发钮 + 浮层」的容器 —— 点击容器内(含触发钮)不动作，
 * 开关交给触发钮自己的 click toggle。
 *
 * 监听口径：isOpen 为 true 时挂 window 的 pointerdown(capture) 与 keydown(capture)；
 * false 时卸载；组件卸载时清理。同一浮层生命周期内反复开合只做增减监听。
 * ESC 在捕获段消费并 stopPropagation：浮层开着时按 ESC 只关浮层，不穿透到外层弹窗。
 */
export function useDismiss(
  anchor: Ref<Element | null>,
  isOpen: Ref<boolean>,
  onClose: () => void
): void {
  function handlePointerDown(e: Event): void {
    if (!isOpen.value) return
    const el = anchor.value
    if (!el) return
    // 目标在 anchor 内 → 忽略（触发钮自己的 click 负责 toggle）
    if (e.target instanceof Node && el.contains(e.target)) return
    onClose()
  }

  function handleKeyDown(e: Event): void {
    if (!isOpen.value) return
    if ((e as KeyboardEvent).key !== 'Escape') return
    // ESC 归属浮层自身: 捕获段先于弹窗(BaseDialog/ManageSheet 的 window/document bubble)
    // 触发, 处理后阻断传播 —— 否则「弹窗内浮层开着按 ESC」会把浮层和整个弹窗一起关掉
    // (R1 评审 #ESC 穿透; v1 原生 select 的 OS 弹层即在自身层级消费 ESC, 页面级关闭不触发)。
    // 同节点同段的其他监听(TV 空间导航 window capture)不受 stopPropagation 影响, 照常收到事件。
    e.stopPropagation()
    onClose()
  }

  function attach(): void {
    // pointerdown/keydown 都用捕获段: 前者保证浮层内部 stopPropagation 的指针事件逃不过外点判定,
    // 后者保证 ESC 先于弹窗 bubble handler 被消费(见 handleKeyDown 注释)
    window.addEventListener('pointerdown', handlePointerDown, true)
    window.addEventListener('keydown', handleKeyDown, true)
  }

  function detach(): void {
    window.removeEventListener('pointerdown', handlePointerDown, true)
    window.removeEventListener('keydown', handleKeyDown, true)
  }

  // immediate: setup 时即已打开的场景不用等到下次翻转才挂监听
  watch(
    isOpen,
    (open) => {
      if (open) attach()
      else detach()
    },
    { immediate: true }
  )

  // 组件卸载时清理（从未打开过则 removeEventListener 幂等 no-op）
  onScopeDispose(detach)
}
