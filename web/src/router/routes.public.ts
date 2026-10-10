import type { RouteRecordRaw } from 'vue-router'

/**
 * 用户端路由（含登录）
 * 全部 layout='public' / 'auth'，懒加载
 * URL 完全兼容旧站
 */
export const publicRoutes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: { path: '/index' }
  },
  {
    path: '/index',
    name: 'home',
    component: () => import('@/views/public/HomeView.vue'),
    meta: { layout: 'public', title: '首页' }
  },
  {
    path: '/filmDetail',
    name: 'film-detail',
    component: () => import('@/views/public/FilmDetailView.vue'),
    meta: { layout: 'public', title: '影片详情' }
  },
  {
    path: '/play',
    name: 'play',
    component: () => import('@/views/public/PlayView.vue'),
    meta: { layout: 'public', title: '播放' }
  },
  {
    path: '/search',
    name: 'search',
    component: () => import('@/views/public/SearchView.vue'),
    meta: { layout: 'public', title: '搜索' }
  },
  {
    path: '/filmClassify',
    name: 'classify',
    component: () => import('@/views/public/ClassifyView.vue'),
    meta: { layout: 'public', title: '分类' }
  },
  {
    path: '/filmClassifySearch',
    name: 'classify-search',
    component: () => import('@/views/public/ClassifySearchView.vue'),
    meta: { layout: 'public', title: '分类筛选' }
  },
  {
    path: '/history',
    name: 'history',
    component: () => import('@/views/public/HistoryView.vue'),
    meta: { layout: 'public', title: '观看历史' }
  },
  {
    path: '/favorites',
    name: 'favorites',
    component: () => import('@/views/public/FavoritesView.vue'),
    meta: { layout: 'public', title: '我的收藏' }
  },
  // /settings 已删（2026-10-10）：TV 的设备级/账号级设置都收进原生抽屉（账号行经
  // window.__jcAuth 直接读写 web 登录态），SPA 设置页没有存在意义了。
  // 片头片尾的"按片"设置仍在播放页（useSkipSettings），账号级默认值的数据协议保留备用。
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/auth/LoginView.vue'),
    meta: { layout: 'auth', title: '登录' }
  },
  {
    path: '/device',
    name: 'device-confirm',
    component: () => import('@/views/auth/DeviceConfirmView.vue'),
    meta: { layout: 'auth', title: '扫码登录确认' }
  }
]
