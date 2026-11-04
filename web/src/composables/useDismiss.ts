import { onScopeDispose, watch, type Ref } from 'vue'

/**
 * 浮层关闭逻辑（规格 §3.1）：点击 anchor 外部 / ESC 时触发 onClose。
 * anchor 是「包含触发钮 + 浮层」的容器 —— 点击容器内(含触发钮)不动作，
 * 开关交给触发钮自己的 click toggle。
 *
 * 监听口径：isOpen 为 true 时挂 window 的 pointerdown(capture) 与 keydown；
 * false 时卸载；组件卸载时清理。同一浮层生命周期内反复开合只做增减监听。
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
    onClose()
  }

  function attach(): void {
    // pointerdown 用捕获段: 在浮层内部 stopPropagation 的 click/pointerdown 也逃不过外点判定
    window.addEventListener('pointerdown', handlePointerDown, true)
    window.addEventListener('keydown', handleKeyDown)
  }

  function detach(): void {
    window.removeEventListener('pointerdown', handlePointerDown, true)
    window.removeEventListener('keydown', handleKeyDown)
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
