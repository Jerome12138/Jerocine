import { ref, type Ref } from 'vue'

/** useListNavigation 选项（规格 §3.2） */
export interface ListNavOptions {
  itemCount: () => number
  /** 打开时初始 active（通常=当前选中项索引）；默认 0 */
  initial?: () => number
  onConfirm: (index: number) => void
  onClose: () => void
}

export interface ListNavHandle {
  activeIndex: Ref<number>
  /**
   * 挂在触发钮（或浮层）上的 keydown 处理：
   * ↑↓ 循环移动、Home/End、Enter/Space 确认、Esc 关闭；已处理的方向键 preventDefault。
   */
  onKeydown: (e: KeyboardEvent) => void
  /** 打开浮层时调用：重置 activeIndex = opts.initial() */
  open: () => void
}

/**
 * 列表浮层键盘导航（写一次、全端复用）：只管「索引游标 + 键盘语义」，
 * 不感知浮层开关状态与 DOM —— 调用方把 onKeydown 绑到触发钮/浮层上即可。
 */
export function useListNavigation(opts: ListNavOptions): ListNavHandle {
  const activeIndex = ref(0)

  /** 循环移动 delta 步: 末→首、首→末（空列表不动作） */
  function move(delta: number): void {
    const count = opts.itemCount()
    if (count <= 0) return
    // 两次取模把负数折回正区间, 避免依赖 Array.prototype.at 的浏览器地板
    activeIndex.value = (((activeIndex.value + delta) % count) + count) % count
  }

  function onKeydown(e: KeyboardEvent): void {
    const count = opts.itemCount()
    switch (e.key) {
      case 'ArrowDown':
        if (count > 0) {
          e.preventDefault()
          move(1)
        }
        break
      case 'ArrowUp':
        if (count > 0) {
          e.preventDefault()
          move(-1)
        }
        break
      case 'Home':
        if (count > 0) {
          e.preventDefault()
          activeIndex.value = 0
        }
        break
      case 'End':
        if (count > 0) {
          e.preventDefault()
          activeIndex.value = count - 1
        }
        break
      case 'Enter':
      case ' ':
        // 空列表无有效索引可确认
        if (count > 0) opts.onConfirm(activeIndex.value)
        break
      case 'Escape':
        // 关闭不受列表是否为空限制——空浮层也要能退出
        opts.onClose()
        break
    }
  }

  function open(): void {
    activeIndex.value = opts.initial?.() ?? 0
  }

  return { activeIndex, onKeydown, open }
}
