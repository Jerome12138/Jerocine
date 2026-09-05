/**
 * API 桶导出
 *
 * 使用：
 *   import { filmApi, authApi, manageApi, http } from '@/api'
 *   const data = await filmApi.getIndex()
 */
export { http } from './http'
export * as filmApi from './film'
export * as authApi from './auth'
export * as manageApi from './manage'
export * as historyApi from './history'
export * as favoriteApi from './favorite'
