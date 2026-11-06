# 管理端 UI 组件规范（manage-ui conventions）

> 2026-11-05 ｜ 提炼自 `docs/specs/manage-controls-spec.md`（唯一契约）§0/§3/§5/§6/§7，供后续新增/修改管理端控件时速查。

## 路线（§0）

- 自建组件皮 + 控件逻辑下沉共享 composables（写一次、全端复用），**不引 styled 组件库**。
- 未来遇到逻辑真正复杂的控件（日期选择器、异步搜索 combobox、虚拟滚动大表）：**按控件**引入 headless 库（reka-ui 类，逻辑来自库、样式 100% 自管），仍不引 styled 库。

## 尺寸档位（§3.6/§5）

- **尺寸模型（2026-11-05 统一，见 `docs/方案-web控件三档标准统一-2026-11-05.md`）**：
  **标准档按端决定** —— 移动 <768 = **24**／平板（默认）= **28**／桌面 ≥1280 = **32**／TV `[data-mode='tv']` = **48**；
  组件**默认不标 size 即跟随**。`sm`/`lg` 是**相对标准的派生档**（`sm = 标准 −4`、`lg = 标准 +8`，只写公式、不逐端写值），
  语义与 EP/主流库一致（small = 小一档）。**移动端 `sm` 钉死 = 标准**（最小端本就全员紧凑，不派生 20px）。
- 所有控件（input/select/checkbox/switch/button/chip）尺寸一律走 `--jc-ctrl-*`／`--jc-switch-*`／`--jc-chip-*`，
  **禁止新写固定 px 高度**；端判断只用于形态（布局/焦点），**不用于尺寸**。
- ManageInput/ManageSelect 用 `size?: 'sm' | 'default' | 'lg'`（默认 default）；工具栏行控件**不标 size**（与归一后的同行按钮同高，见下节「size 使用规则」）。
- 唯一两档豁免：`BasePagination` 移动页号 chip = **28**（相邻数字触达密度；其余端随标准档 28/32）。
- 例外定值复核清单（违反即修）：segment 胶囊页签、ManageTable 表头筛选钮、Dashboard 刷新按钮 —— 一律绑 `--jc-ctrl-h`。
- 浮层统一 `max-h-[320px]` 内滚动，四档一致。

## size 使用规则（B5 归一，2026-11-06）

> 落地原则「大部份默认」：**不标 size 即标准档**；`sm`/`lg` 仅在下列场景出现。

- **不标 size（=标准档）**：页面工具栏/header 行（ManageTable `#toolbar` 槽、页面顶部 header 行内，含主操作与筛选/清除等次要钮）、弹窗/Sheet footer（确定/保存/取消/创建等）、表单字段内控件、页级错误/空态重试、列表级「加载更多」。
- **`sm`（标准 −4）**：表格行内（`#cell`/`#actions`/`#mobile-card` 模板内）；紧凑嵌套容器（级联面板 footer、候选位、卡片角部行内动作、ManageImageUpload 内部、表单字段内嵌错误行等）。
- **`lg`（标准 +8）**：公共区 CTA 预留（auth 提交钮等，ManageButton lg 分支）；管理端 `views/manage`/`components/manage` 内不使用。
- 存量归一：B5 一次性删除工具栏/footer/表单上的历史 `size="sm"` 共 77 处；新增代码按本节执行，行内与紧凑容器仍用 `sm`。

## 页面标题两种合法形态（2026-11-06 归一）

- **列表页**：标题在工具栏内，用 `<h2 class="text-lg font-[var(--jc-fw-semibold)]">`（12 个列表页一律不动）。
- **仪表盘 / 监控类**：标题在页面顶部，用 `<h1 class="text-2xl font-[var(--jc-fw-semibold)]">`。
- 落地样例：`DashboardView`（h2→h1 + 2xl/semibold）、`LiveSourceListView`（去 `jc-livemgr__title` 私有样式改工具类）；
  `TaskManageView` / `TelemetryView` / `CollectJobsView` 原即目标形态。
- 禁止第三种形态（如 `h2 + text-[length:--jc-fs-2xl] + fw-bold`、或给标题挂 scoped 私有类定字号）。

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
