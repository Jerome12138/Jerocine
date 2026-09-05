import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { publicRoutes } from './routes.public'
import { manageRoutes } from './routes.manage'
import { registerGuards } from './guards'
import { scrollBehavior } from './scrollBehavior'

/** 扩展 RouteMeta 类型 */
declare module 'vue-router' {
  interface RouteMeta {
    /** 布局壳 */
    layout?: 'public' | 'manage' | 'auth' | 'minimal'
    /** 是否需要登录 */
    requiresAuth?: boolean
    /** 是否需要管理员（role=1），自动隐含 requiresAuth */
    requiresAdmin?: boolean
    /** 浏览器标题 */
    title?: string
    /** keepAlive 缓存 */
    keepAlive?: boolean
  }
}

const routes: RouteRecordRaw[] = [
  ...publicRoutes,
  ...manageRoutes,
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/error/NotFoundView.vue'),
    meta: { layout: 'minimal', title: '页面不存在' }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior
})

registerGuards(router)

/**
 * Native (Jerocine APK) 模式拦截 /play 路由 — 不渲染 web 播放页, 直接拉 detail 数据
 * 然后 jerocine.playPlaylist 拉起原生播放器. 取消时若 detail 拉到了就跳详情页,
 * 否则放行 web /play 兜底. 浏览器模式不受影响 (isNative false).
 *
 * 关键: 用 beforeEach 是因为这个判断要在 Vue 组件 setup 之前发生, 避免 PlayView
 * mount 再 detect native 的"先闪一下空白 web 页"体验. FilmDetailView 的"立即播放"
 * 已直接跳过 /play 不进路由; 这条拦截兜的是: 外部链接 / 历史记录 / 收藏点击 等
 * 仍然进 /play 的入口.
 */
router.beforeEach(async (to) => {
  if (to.path !== '/play') return
  // 异步 import 避免增加首屏 router bundle
  const { isNative } = await import('@/utils/jerocineNative')
  if (!isNative()) return
  const id = String(to.query.id ?? '')
  const source = String(to.query.source ?? '')
  const episode = Number(to.query.episode ?? 0)
  const currentTime = Number(to.query.currentTime ?? 0)
  if (!id) return // 缺 id, 让 PlayView 兜底渲染报错
  try {
    const { filmApi } = await import('@/api')
    const resp = await filmApi.getFilmDetail(id)
    const detail = resp?.detail
    if (!detail?.sources?.length) {
      return { path: '/filmDetail', query: { link: id }, replace: true }
    }
    // 统一派发(映射源+建历史+skip+proxyBase 全在一处, 避免多入口漏字段)
    const { dispatchNativePlaylist } = await import('@/utils/nativePlay')
    dispatchNativePlaylist(
      detail,
      source,
      Number.isFinite(episode) ? episode : 0,
      currentTime > 0 ? currentTime : 0
    )
    // 无论成功与否都落详情页: 成功→退出 native 后落详情; 数据不全→详情兜底, 不留空白 /play
    return { path: '/filmDetail', query: { link: id }, replace: true }
  } catch {
    // 拉 detail 失败, 退到详情页兜底
    return { path: '/filmDetail', query: { link: id }, replace: true }
  }
})

// Telemetry: 路由变化记 PV (含 query 串)
router.afterEach((to) => {
  void import('@/utils/telemetry').then(({ telemetry }) =>
    telemetry.trackPv(to.fullPath)
  )
})

/**
 * 动态 import 失败自动 reload — 新 deploy 把 dist/assets/XXX-[hash].js 文件全部换名,
 * 但 WebView 缓存里 index.html 还指向旧 hash. 用户跳路由 → 懒加载 component 取旧 chunk
 * → 404 → nginx 默认返 index.html (text/html), 触发 "'text/html' is not a valid
 * JavaScript MIME type" 的 unhandled-rejection. 一旦命中就 location.reload() 拿新
 * index.html + 新 chunk URLs, 错误自愈.
 */
router.onError((err) => {
  const msg = err instanceof Error ? err.message : String(err)
  const isChunkFail =
    msg.includes('dynamically imported module') ||
    msg.includes('text/html') ||
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('Importing a module script failed')
  if (isChunkFail && typeof window !== 'undefined') {
    void import('@/utils/telemetry').then(({ telemetry }) => {
      telemetry.trackError(err as Error, 'chunk-load-reload', { msg })
    })
    // 给 telemetry 50ms flush 机会, 然后硬刷
    window.setTimeout(() => window.location.reload(), 50)
  }
})

export default router
