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

## 底色分层规则（2026-11-06 弹窗可辨性修复）

**三层各差一档，控件永远比它的容器亮一档**：

| 层 | 令牌 | 值 |
|---|---|---|
| 页面底 | `--jc-bg-base` | `#0b0b0f` |
| 卡片 / 弹窗 / 浮层底 | `--jc-bg-surface` | `#141518` |
| 控件底 | `--jc-bg-elevated` | `#1c1d22` |

- **卡片 / 弹窗 / 浮层**一律 `bg-surface`：`BaseDialog`、`ManageSheet`（modal · sheet）、`ManageTable` 表头筛选浮层、`ManageSelect` 浮层、管理端设置页各配置卡（`SiteConfigView` / `MailConfigCard` / `DanmuConfigCard`）。
- **输入类控件**一律 `bg-elevated`：`ManageInput`、`ManageTextarea`、`ManageSelect` 触发器、`ManageCheckbox`（未选态）。
- **控件不再靠 `border-ctrl` 描边"显形"**：
  - `ManageInput` / `ManageTextarea` 用 `border border-transparent` **静默占位** —— 保住 1px 盒高（与选择框等高、无跳变），常态无边框、焦点才显形（`focus:border-ctrl-focus` + `focus:shadow-focus`）。
  - `ManageSelect` 触发器 / `ManageCheckbox` 保留 `border-ctrl`（它们需要轮廓定义形状）。
- **反例（违反即修）**：控件底与容器底取同一档 → 只能靠描边区分，控件进了弹窗就糊成一片（本批即由此修复）。

## 滚动条与层叠约定（2026-11-06）

- **滚动条显隐**：滚动容器一律加 `class="jc-scrollauto"` + `@scroll="onScroll"`（`composables/useAutoHideScrollbar`）。
  语义 = **常占位 thin 细条（不产生布局跳动）+ 拇指透明→滚动时显形→停止 700ms 收起**。
  - 不要用 `scrollbar-width: none ↔ thin` 切换（部分内核下切换不生效 → “永远看不到滚动条”）。
  - 已落地：选源横向条、选集网格、管理后台主区（`.jc-manage-main`）。
- **内层滑到头不带动外层**：`.jc-scrollauto` 自带 `overscroll-behavior: contain`（滚动链的标准解法，无需 JS）。
- **层叠（同特异性后者胜）** —— `virtual:uno.css` → … → `theme.css` → 组件 `<style>`：
  - theme.css 中「会被工具类使用的属性」用 `:where()` 包一层（见 CLAUDE.md「Web CSS 层叠约定」）；
  - 组件内不要写否定型规则去盖 theme.css 的通用类（滚动条 / position / outline 三类都栽过）。
- **选源条细节**（2026-11-06）：源胶囊 `min-width` 移动 40 / 平板 46 / 桌面 52；左右翻页箭头取 `--jc-ctrl-h-sm`、与外层间距用 `--jc-space-2`；左右渐隐与箭头**共用同一份可滚状态**（滑到头则隐藏该侧阴影 / 禁用对应箭头）。
- **集格**：集名「全部偏短」（≤5 字）→ 列数翻倍（宽约折半，间隙由 `1fr` 分配自动扣除）；**移动端所有集一律最多两行**（`-webkit-line-clamp: 2`），不区分是否当前集。


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
