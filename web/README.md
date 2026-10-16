# jerocine-web

Jerocine影视 Web 客户端（用户端 + 管理端 + TV 模式），并经 Capacitor 打包安卓 APK。

## 技术栈

- Vue 3.5 + Vite 5 + TypeScript 5
- Pinia 2 + Vue Router 4
- UnoCSS（preset-uno + attributify + icons）
- axios + @vueuse/core + video.js

## 开发

```bash
pnpm install
pnpm dev          # 启动 :3600，代理 /api → 127.0.0.1:3601
pnpm type-check   # 类型检查
pnpm build        # 产物输出 dist/（含 vue-tsc 严格类型门禁，CI/部署以此为准）
```

## 目录约定

- `src/api/`     接口模块（业务域拆分，以 `server/openapi/openapi.yaml` 为契约真相）
- `src/stores/`  Pinia 全局状态
- `src/router/`  路由（public + manage 双布局）
- `src/views/`   页面视图（按业务域归档）
- `src/components/{base,layout,film}/` 自动注册的 base 组件 + 布局壳 + 影视业务组件
- `src/composables/` 组合式工具
- `src/types/`   DTO / 接口类型
- `src/utils/`   通用工具

视觉与交互规范见仓库根 `docs/design/`（design-tokens / components-spec / ux-design-v2）。

## 三套视图模式

通过 `<html data-mode="mobile|desktop|tv">` 切换：

- 自动检测：UA + 视口宽度 + hover 能力
- 强制：`localStorage.setItem('jc-mode','tv')` 或 URL `?mode=tv`
- 详见 `src/composables/useViewMode.ts`

## 编码守则

- 遵循分层与命名约定；不做"兼容旧字段"的冗余映射。
- 严格模式（`noUncheckedIndexedAccess`）下注意索引访问的 `T | undefined`。
- 组件必须补齐 空/错/loading 三态；视觉 token 一律引用 `--jc-*` CSS 变量，不写裸值。

## Android 打包（Capacitor）

```bash
pnpm cap:sync     # cap sync android：把 android/web-stub 占位页 + 插件配置同步进壳工程（不跑 vite build）
pnpm cap:open     # 在 Android Studio 打开 android/ 工程
pnpm cap:build    # 等价于 cap:sync（合并后 APK 不再携带前端产物，前端构建已解耦）
```

> ⚠️ 单包合并后 **APK 与前端构建解耦**：`capacitor.config` 的 `webDir` 指向 `../android/web-stub`
> （只有占位 index.html），WebShell 运行时远程加载线上 SPA。故 `cap:*` 不再执行 `vite build`
> —— 需要部署前端时单独跑 `pnpm build`（产物进 `dist`，由 nginx 发布）。
> 日常出包直接用仓库根 `scripts/build-android.sh all`。

**前置条件**：JDK 17、Android Studio、Android SDK 36（compileSdk；AGP 8.13 + Capacitor 8 要求）。

- 签名：release 签名凭据从 `keystore.properties`（gitignored）或环境变量读取，`*.keystore`/`*.jks` **严禁提交**。
- **单包双壳（2026-10-11 合并）**：打包入口为**顶层 `android/` 工程**（2026-10-15 从 `web/android` 上移，
  本目录下的 `android` 只是本地 junction → `../android`，Capacitor 平台目录硬约定），launcher 为 `EntryRouterActivity`，按设备能力路由到
  `WebShellActivity`（本壳）或内嵌的 `:tvshell` `NativeShellActivity`。APK **远程加载**线上站点 URL
  （WebShell 起 WebView 打开远端 SPA），故前端改动部署后对本壳即时生效、无需重发 APK；
  包内 `assets/public` 只留占位页。**出包用仓库根 `scripts/build-android.sh all`**（不再跑 vite build）。
