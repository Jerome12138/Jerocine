import { isNative, jerocine } from '@/utils/jerocineNative'

/**
 * TV 模式「设置」入口的统一语义（2026-10-10 随 SPA `/settings` 删除而修订）。
 *
 * 设置入口现在**只有原生抽屉一处**（设备级设置 + 账号行都在抽屉里）：
 *   - **原生壳(APK)**: 打开抽屉（顶栏齿轮 / 首页金刚区「设置」卡都走这里）;
 *   - **纯网页 TV 模式**(?mode=tv, bridge 不在场): 没有抽屉、也没有 SPA 设置页了 ——
 *     顶栏只显示「刷新」，设置按钮不渲染（TvHeaderActions 按 isIntercepted() 隐藏），
 *     首页「设置」卡同理（v-if="showTvSettingsCard"，setup 期取 isIntercepted()）。
 *
 * 抽成 composable 是为了让"该不该开抽屉"可单测, 且顶栏(TvHeaderActions)与首页金刚区
 * (HomeView 的"设置"卡)共用同一份判断, 避免两处各写一遍而漂移。
 */

export interface TvSettingsEntry {
  /** 打开设置（= 原生抽屉）。纯网页没有设置入口，调用是 no-op（UI 层应先用 isIntercepted 隐藏按钮） */
  open: () => void
  /**
   * 给 RouterLink 用的点击守卫: 原生壳里拦掉默认跳转(改用抽屉),
   * 纯网页里什么都不做。
   *
   * ⚠️ **必须绑在捕获阶段**(`@click.capture`) —— RouterLink 自身也在这个 <a> 上绑了
   * navigate, 且 Vue 合并同名 handler 时组件自身的排在后面(冒泡阶段)才执行我们这份;
   * 冒泡阶段的 preventDefault 拦不住 navigate(实测 push 仍发生)。捕获阶段先跑 ⇒ 有效。
   */
  guard: (e?: { preventDefault: () => void }) => void
  /** 当前是否会走原生抽屉(每次实时判断, 不用快照, 避免 bridge 注入时序导致误判) */
  isIntercepted: () => boolean
}

export function useTvSettingsEntry(): TvSettingsEntry {
  const isIntercepted = (): boolean => isNative()

  function open(): void {
    if (isNative()) {
      jerocine.openSettings()
    }
    // 非原生: no-op。SPA /settings 已删(2026-10-10), UI 层用 isIntercepted() 把
    // 设置入口藏掉, 这里只是兜底防误调。
  }

  function guard(e?: { preventDefault: () => void }): void {
    if (!isNative()) return
    if (e) e.preventDefault()
    jerocine.openSettings()
  }

  return { open, guard, isIntercepted }
}

/**
 * 顶栏右侧「刷新 / 设置」按钮组是否显示（2026-10-10 修订）。
 *
 * - **TV 模式 ⇒ 恒显示按钮组**：纯网页 TV 模式（`?mode=tv`，无桥）也保留「刷新」；
 *   「设置」按钮由 TvHeaderActions 内部按 isIntercepted() 二次过滤（无抽屉可开就不渲染）。
 * - **非 TV 模式 ⇒ 仅"在原生壳里且能调起设置抽屉"时显示**：桌面布局下首页金刚区的
 *   「设置」卡不渲染（`v-if="isTV"`），顶栏是用户唯一的回退入口 —— 藏掉它，用户在壳里
 *   切到桌面模式后就再也切不回 TV 了。
 *
 * 拆成纯函数是为了把这条产品决策钉在单测里（见 useTvSettingsEntry.spec.ts）。
 */
export function shouldShowHeaderActions(
  isTV: boolean,
  canOpenNativeSettings: boolean
): boolean {
  return isTV || canOpenNativeSettings
}
