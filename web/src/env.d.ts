/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface ImportMetaEnv {
  readonly VITE_API_BASE: string
  readonly VITE_APP_TITLE: string
}

/** 构建期注入的构建号(= 部署脚本的 JC_BUILD_TS, 见 vite.config.ts define); 未注入时为空串 */
declare const __APP_BUILD__: string

interface ImportMeta {
  readonly env: ImportMetaEnv
}
