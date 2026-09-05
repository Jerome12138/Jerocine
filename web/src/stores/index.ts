/**
 * Pinia store 桶导出
 *
 * 注意：在 view / 组件 setup 内调用 useXxxStore() 会自动注册。
 * 不要在 main.ts 顶层提前 use 全部 store，避免 tree-shaking 失效。
 */
export { useUIStore } from './ui'
export { useUserStore } from './user'
export { useSiteStore } from './site'
export { useNavStore } from './nav'
export { useHistoryStore } from './history'
export { useFavoriteStore } from './favorite'
