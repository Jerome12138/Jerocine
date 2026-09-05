import type { RouteRecordRaw } from 'vue-router'

/**
 * 后台管理路由
 * 全部 layout='manage'，requiresAuth=true，懒加载
 */
export const manageRoutes: RouteRecordRaw[] = [
  {
    path: '/manage',
    redirect: { path: '/manage/index' }
  },
  {
    path: '/manage/index',
    name: 'manage-dashboard',
    component: () => import('@/views/manage/DashboardView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '后台首页' }
  },
  {
    path: '/manage/telemetry',
    name: 'manage-telemetry',
    component: () => import('@/views/manage/telemetry/TelemetryView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '埋点监控' }
  },
  {
    path: '/manage/collect/index',
    name: 'manage-collect',
    component: () => import('@/views/manage/collect/CollectListView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '采集源管理' }
  },
  {
    path: '/manage/collect/jobs',
    name: 'manage-collect-jobs',
    component: () => import('@/views/manage/collect/CollectJobsView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '采集任务监控' }
  },
  {
    path: '/manage/cron/index',
    name: 'manage-cron',
    component: () => import('@/views/manage/cron/CronListView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '定时任务' }
  },
  {
    path: '/manage/film',
    name: 'manage-film-list',
    component: () => import('@/views/manage/film/FilmListView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '影片管理' }
  },
  {
    path: '/manage/film/class',
    name: 'manage-film-class',
    component: () => import('@/views/manage/film/FilmClassView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '影片分类' }
  },
  {
    path: '/manage/film/add',
    name: 'manage-film-add',
    component: () => import('@/views/manage/film/FilmAddView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '新增影片' }
  },
  {
    path: '/manage/film/detail',
    name: 'manage-film-detail',
    component: () => import('@/views/manage/film/FilmDetailView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '影片详情' }
  },
  {
    path: '/manage/file/upload',
    name: 'manage-file-upload',
    component: () => import('@/views/manage/file/FileUploadView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '文件上传' }
  },
  {
    path: '/manage/file/gallery',
    name: 'manage-file-gallery',
    component: () => import('@/views/manage/file/FileGalleryView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '文件库' }
  },
  {
    path: '/manage/system/webSite',
    name: 'manage-system-config',
    component: () => import('@/views/manage/system/SiteConfigView.vue'),
    meta: { layout: 'manage', requiresAuth: true, requiresAdmin: true, title: '站点配置' }
  }
]
