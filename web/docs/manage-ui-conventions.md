# 管理端 UI 组件规范（manage-ui conventions）

> 2026-11-05 ｜ 提炼自 `docs/specs/manage-controls-spec.md`（唯一契约）§0/§3/§5/§6/§7，供后续新增/修改管理端控件时速查。

## 路线（§0）

- 自建组件皮 + 控件逻辑下沉共享 composables（写一次、全端复用），**不引 styled 组件库**。
- 未来遇到逻辑真正复杂的控件（日期选择器、异步搜索 combobox、虚拟滚动大表）：**按控件**引入 headless 库（reka-ui 类，逻辑来自库、样式 100% 自管），仍不引 styled 库。

## 尺寸档位（§3.6/§5）

- 表单控件（input/select/checkbox/switch/button）尺寸一律走 `--jc-ctrl-*`/`--jc-switch-*` 令牌：四档（移动 <768 = 32/28/36；常规 = 36/30/42；宽屏 ≥1280 = 40/32/46；TV = 48/40/56）自动生效，**禁止新写固定 px 高度**。
- ManageInput/ManageSelect 用 `size?: 'sm' | 'default' | 'lg'`（默认 default）；与 sm 按钮同行的工具栏控件用 `sm`。
- 刻意定值例外（保留，不算违规）：segment 胶囊 28px、BasePagination 页码 chip 44px、ManageTable 表头筛选钮 24px、Dashboard 刷新按钮 44px。
- 浮层统一 `max-h-[320px]` 内滚动，四档一致。

## 交互与可访问性（§3/§7）

- 新交互控件（button/input/option/浮层项…）必带 `data-focusable="true"` —— TV 空间导航按 DOM 几何寻焦。
- 浮层定位 = absolute 内联（同级联/排序面板范式）；不做通用翻转；弹窗内实测裁剪时用组件预留的 `teleport` prop（Teleport+fixed）。
- 弹窗面板一律 `max-width: 94vw` 兜底（CSS `min()` 在 legacy chrome61 不可用）；调用点 width 保持固定 px。
- 浮层关闭逻辑用 `composables/useDismiss`（外点 + ESC），键盘循环/确认用 `composables/useListNavigation`；页面不得再手写内联 listener。

## 组件复用门槛（§6）

- 提案组合组件：**≥3 处重复**，或 2 处且存在漂移风险；且实现成本 ≤1 个新文件 + ≤3 个页面触点；否则只记 `deferred` 建议，不实施。
- 单批至多实施 2 个提案，防范围膨胀。

## 测试与门禁（§7）

- 组件新能力必须带 vitest spec，**TDD 先红后绿**；页面行为断言优先，不断言内部类名（视觉契约本身除外，如 segment 激活类）。
- 纯逻辑下沉为独立 `.ts` 模块（同目录放 spec），不挂在 SFC 上测。
- 铁律：不 push、不部署；e2e 不写不跑；公共区（views/public、views/auth、PublicHeader/PublicFooter/MobileTabbar/AuthLayout）不动；样式走 `--jc-*` 令牌与 UnoCSS 语义 shortcuts；新代码中文注释说明设计意图。
- 门禁：`pnpm run type-check` + `pnpm run test`（最终交付再加 `pnpm run build`），全绿才提交；每批次/每轮修复各一笔 commit（`feat|refactor|fix|docs(manage-ui)`，中文主题）。
