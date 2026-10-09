import { useRouter } from 'vue-router'
import { isNative, jerocine } from '@/utils/jerocineNative'

/**
 * TV 模式「设置」入口的统一语义(方案 §6 + 抽屉重设计 §5/§6)。
 *
 * 两种宿主必须给不同答案:
 *   - **原生壳(APK)**: 设备级设置都在原生抽屉里(能用 Java 直读写原生偏好), 打开抽屉;
 *     此时首页/顶栏那个链接指向的 SPA `/settings` 只剩账号级设置, 不该把用户送到那儿。
 *   - **纯网页 TV 模式**(?mode=tv, bridge 不在场): 没有抽屉可开, 老实走 SPA `/settings` 路由。
 *
 * 抽成 composable 是为了让"该走哪条路"可单测, 且顶栏(TvHeaderActions)与首页金刚区
 * (HomeView 的"设置"卡)共用同一份判断, 避免两处各写一遍而漂移。
 */

export interface TvSettingsEntry {
  /** 打开设置: 原生壳 → 开原生抽屉; 纯网页 → 跳 SPA /settings */
  open: () => void
  /**
   * 给 RouterLink 用的点击守卫: 原生壳里拦掉默认跳转(改用抽屉),
   * 纯网页里什么都不做(让 <a> 正常导航)。
   */
  guard: (e?: { preventDefault: () => void }) => void
  /** 当前是否会走原生抽屉(每次实时判断, 不用快照, 避免 bridge 注入时序导致误判) */
  isIntercepted: () => boolean
}

export function useTvSettingsEntry(): TvSettingsEntry {
  const router = useRouter()

  const isIntercepted = (): boolean => isNative()

  function open(): void {
    if (isNative()) {
      jerocine.openSettings()
      return
    }
    void router.push('/settings')
  }

  function guard(e?: { preventDefault: () => void }): void {
    if (!isNative()) return
    if (e) e.preventDefault()
    jerocine.openSettings()
  }

  return { open, guard, isIntercepted }
}
