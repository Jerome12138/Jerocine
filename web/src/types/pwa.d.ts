/**
 * vite-plugin-pwa 的虚拟模块类型声明 (`virtual:pwa-register` 等).
 *
 * 必须显式 reference: tsconfig.app.json 用了 `types: ["vite/client", "node"]`
 * (白名单模式), 不会自动加载插件自带的 client 类型; 缺了它 `pnpm build`
 * 的类型门禁 (vue-tsc --noEmit) 会因找不到 virtual 模块而报错.
 */
/// <reference types="vite-plugin-pwa/client" />
