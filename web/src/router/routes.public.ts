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
    path: '/live',
    name: 'live',
    component: () => import('@/views/public/LiveView.vue'),
    meta: { layout: 'public', title: '直播' }
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
    // 独立热榜页(8 榜并集/单榜筛选 + 年份/地区/语言/分类/排序)。固定导航入口, 不进 category 表。
    path: '/hotBoard',
    name: 'hot-board',
    component: () => import('@/views/public/HotBoardView.vue'),
    meta: { layout: 'public', title: '热榜' }
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
  // /settings 于 2026-10-10 删除过（当时设置全收进原生抽屉）。2026-10-18 重新加入:
  // web 状态开关(广告过滤/取源方式/全量中转/自动连播)需要独立配置页,
  // 数据仍存 web localStorage(useWebSettings 单一数据源), 安卓抽屉经 window.__jcConfig 读写同一份。
  {
    path: '/settings',
    name: 'settings',
    component: () => import('@/views/public/SettingsView.vue'),
    meta: { layout: 'public', title: '设置' }
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/auth/LoginView.vue'),
    meta: { layout: 'auth', title: '登录' }
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('@/views/auth/RegisterView.vue'),
    meta: { layout: 'auth', title: '注册' }
  },
  {
    // 激活链接落地页: 邮件里的一次性 token → 设密码建号 → 自动登录
    path: '/activate',
    name: 'activate',
    component: () => import('@/views/auth/ActivateView.vue'),
    meta: { layout: 'auth', title: '激活账号' }
  },
  {
    // 忘记密码: 输邮箱 → 发一次性重置链接(30 分钟)
    path: '/forgot-password',
    name: 'forgot-password',
    component: () => import('@/views/auth/ForgotPasswordView.vue'),
    meta: { layout: 'auth', title: '忘记密码' }
  },
  {
    // 重置密码落地页: 邮件里的一次性 token → 设新密码 → 全部设备下线回登录
    path: '/reset-password',
    name: 'reset-password',
    component: () => import('@/views/auth/ResetPasswordView.vue'),
    meta: { layout: 'auth', title: '重置密码' }
  },
  {
    path: '/device',
    name: 'device-confirm',
    component: () => import('@/views/auth/DeviceConfirmView.vue'),
    meta: { layout: 'auth', title: '扫码登录确认' }
  }
]
