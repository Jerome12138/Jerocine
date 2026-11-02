# web e2e 测试

基于 **Playwright** 的端到端 UI 测试。

> ⚠️ 前端 Mock 适配器已在重构中移除（见 `src/main.ts`，`src/mock/` 目录已删除，源码无任何
> `VITE_USE_MOCK` 消费点）。e2e 通过 dev proxy 打真实后端（`127.0.0.1:3601`），跑测试前需先
> 启动本地后端，否则断言列表数据的用例（首页 / 管理端等）会失败。

## 目录

```
tests/e2e/
├── helpers.ts                       # waitAppReady / fakeLogin / setViewMode / muteImages
├── public-home.spec.ts              # HomeView：Hero 自适应 + FilmRow + 评分角标
├── public-detail.spec.ts            # FilmDetailView：标题 / Hero / 多源 + 集数
├── public-play.spec.ts              # PlayView：标题 / 集数高亮 / 自动连播 / 下一集
├── public-search-classify.spec.ts   # SearchView + ClassifyView + ClassifySearchView
├── public-history.spec.ts           # HistoryView：空态 / 卡片 / 移除 / BaseConfirmDialog
├── manage-auth-dashboard.spec.ts    # 鉴权重定向 + Login + Dashboard
├── manage-collect-cron.spec.ts      # 采集源 + 定时任务 CRUD + 确认弹窗
├── manage-film-file.spec.ts         # 影片管理 + 文件库
└── manage-system-sessions.spec.ts   # 在线会话(页面级路由拦截, 不依赖本地后端)
```

## 运行

```bash
# 全矩阵（6 视口 × 47 用例）
pnpm exec playwright test

# 仅桌面
pnpm exec playwright test --project=desktop

# 单文件 / 单测试
pnpm exec playwright test public-home.spec.ts
pnpm exec playwright test --grep "Hero 区域"

# 调试 / 录像
pnpm exec playwright test --headed --debug
pnpm exec playwright show-report
```

## 项目矩阵（playwright.config.ts）

| project          | viewport      | UA / device         | 用途                     |
|------------------|---------------|---------------------|--------------------------|
| mobile-portrait  | 390 × 844     | iPhone 12 (Chromium)| 手机竖屏（4:5 Hero）     |
| mobile-landscape | 844 × 390    | iPhone 12 横屏      | 手机横屏（max-height）   |
| tablet           | 768 × 1024    | Chromium + touch    | 平板（16:9 Hero）        |
| desktop          | 1366 × 768    | Desktop Chrome      | 主流桌面（21:9 Hero）    |
| desktop-2k       | 2560 × 1440   | Desktop Chrome      | 2K 大屏（clamp max）     |
| tv               | 1920 × 1080   | Desktop Chrome      | TV 模式 / 焦点导航      |

> 全部用 Chromium 引擎跑，避免 WebKit 二次下载（200+MB）。

## 运行前提

dev server 由 playwright `webServer` 自动拉起，API 请求经 vite proxy（见 `vite.config.ts`
`server.proxy`）转发到本地后端 `127.0.0.1:3601`（可用 `JEROCINE_DEV_PROXY` 覆盖目标）。

## helpers 用法

```ts
// 等"应用就绪"（#app 可见 + 网络空闲）
import { waitAppReady } from './helpers'
await waitAppReady(page)

// 模拟登录（写真实 token 结构到 localStorage['auth']）
import { fakeLogin } from './helpers'
await fakeLogin(page)

// 强制 TV 模式
import { setViewMode } from './helpers'
await setViewMode(page, 'tv')
```

## 故障排查

- **ERR_CONNECTION_REFUSED**：dev server 未起，手动 `pnpm dev` 后再 `playwright test`，或杀掉占用 3600 端口的旧进程。
- **列表/接口数据为空或断言失败**：确认本地后端（127.0.0.1:3601）已启动——前端 mock 已移除，e2e 打的是真实后端。
- **WebKit 启动失败**：本仓库强制 `browserName: 'chromium'`，不需要 webkit。

## 报告

```bash
pnpm exec playwright show-report   # 打开 HTML 报告（失败会附截图 / 视频）
```
