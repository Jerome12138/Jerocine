# 管理端控件组件化规格（SDD）— manage-controls

> 2026-11-05 ｜ 分支 `manage-controls`（worktree `D:\Git\JerocineHarness\.worktrees\manage-controls`，基线 `feat/baseline-non-ep` @ `3dcb9d4`）
> 方法论：superpowers SDD + TDD。**本文件是唯一契约：实施不得越规格，宁可少做，缺的记入交付报告。**
> 配套任务计划：`docs/specs/manage-controls-plan.md`（批次划分与 TDD 步骤）。
> 关联存档（外层 Harness 仓）：`docs/UI组件库重构交付报告-2026-11-04.md`、`docs/审查报告-管理端EP重构与表格库落地-2026-11-04.md`。

---

## 0. 背景与路线（已拍板）

- ep-refactor（Element Plus，10 笔提交）实验结论：styled 组件库"只做样式主题适配"在本项目不成立——主题桥接税（审查返工 10 条中 7 条是 UI）、TV 空间导航零感知、legacy chrome61 风险、PWA 体积 +150~250KB；且 UI 不可还原（`feat/ep-style-tokens` 9 笔修复失败），已回退基线。
- **路线 B（用户拍板）**：保留自建组件皮；控件内部逻辑**下沉共享 composables（写一次、全端复用）**，不引组件库；EP 分支留 main 存档不再跟进。
- **升级规则（长期）**：未来遇到逻辑真正复杂的控件（日期选择器、异步搜索 combobox、虚拟滚动大表），**按控件**引入 headless 库（reka-ui 类，逻辑来自库、样式 100% 自管），**不引 styled 库**（styled 库只在接受其设计语言时才划算，本项目设计语言自有）。
- 本批目标：①胶囊页签组件化全端统一 ②checkbox 组件化 ③ManageSelect v2 自绘浮层 ④控件高度与影片列表按钮对齐 ⑤弹窗宽度响应式兜底（EP 逻辑移植）⑥管理端组件规范短文档。

## 1. 决策

| # | 决策 | 理由 / 备注 |
|---|---|---|
| D① | ManageTabs 新增 `segment` 变体，视觉**逐像素照抄**仪表盘在线明细两组胶囊；**删除 pill（渐变按钮）变体** | 用户点名"给影片列表等其他页面复用"；4 个 pill 使用页全部迁移后 pill 零引用 |
| D② | 仪表盘两组手写胶囊迁 `ManageTabs variant="segment"`，切换逻辑（switchTime/statusFilter）不动 | 消灭最后一处手写页签 |
| D③ | 新建 `ManageCheckbox`：label 包裹 + 视觉隐藏原生 input + 自绘方框；支持 `indeterminate` | 4 处裸 checkbox 全替换；纯 CSS、零复杂逻辑 |
| D④ | `ManageSelect` 重写为自绘浮层下拉；**API 向后兼容** + 新增 `option`/`trigger` 具名槽 | 原生 `<select>` 弹层由 OS 渲染是"丑"的根源；9 个使用页随组件自动升级 |
| D⑤ | FilmListView 手写排序菜单并入 ManageSelect v2（图标走槽位） | 消灭最后一处手写下拉浮层 |
| D⑥ | ManageInput/ManageSelect 加 `size?: 'sm'|'default'|'lg'`（默认 default）；与 sm 按钮同行的工具栏控件用 `sm` | 用户拍板"以影片列表页按钮高度为准"；ManageTextarea 不加（表单专用，已与 default 按钮同高）；顺带全局隐藏 number input 原生 spinner |
| D⑦ | 弹窗宽度兜底：ManageSheet（modal 态）与 BaseDialog 面板样式**统一加 `max-width: 94vw`**，调用点 width 保持固定 px | EP 用 `min(Xpx,94vw)` 但 CSS `min()` Chrome 79+ 才有，legacy chrome61 不支持；`max-width:94vw` 语义等价且全版本可用、一处改动覆盖全部弹窗 |
| D⑧ | 浮层定位 = absolute 内联（同既有级联/排序面板范式）；不做通用翻转；ManageSheet 弹窗内实测裁剪时该使用点用组件预留的 `teleport` prop（Teleport+fixed） | TV 空间导航按 DOM 几何寻焦，内联浮层最稳 |
| D⑨ | segment 胶囊保持 `min-h-[28px]` 定值 + `text-xs`，**不做分档** | "按原有 UI"优先；分档只适用于表单控件（§5） |
| D⑩ | 级联祖先归一（EP 移植项）：**先核查服务端 `FilterSpec.CatIDs` 语义**再决定实施 | 若直挂父级的影片需父 id 命中，则在 applyCatFilter 加纯函数归一；否则留档跳过 |

## 2. 分组件审查表（全量盘点结论）

### 2.1 manage/*（处置 = 本批动作）

| 组件 | 现状与问题 | 处置 |
|---|---|---|
| ManageButton | 健壮：variant/type 双通道、尺寸走 `--jc-ctrl-*` 三档+TV 令牌 | **不动**（本批高度基准：`size="sm"` = `--jc-ctrl-h-sm`） |
| ManageInput | 缺 size 分档；`type=number` 原生 spinner 暴露（14 处） | +`size` prop（D⑥）；全局 CSS 隐藏 number spinner（§3.6） |
| ManageTextarea | 与 default 按钮同高，无问题 | 不动 |
| ManageSelect | 原生 `<select>` 包装，OS 弹层丑；不支持自定义渲染 | **重写**（D④/§3.5） |
| ManageCascader | 每行裸 checkbox（accent 紫）；面板范式健康 | checkbox 换 ManageCheckbox；其余不动 |
| ManageSwitch | EP 风格自研+令牌+TV ring，已达标 | 不动 |
| ManageTable | 表头筛选面板裸 checkbox（accent 引用悬空 token `--jc-brand`）；筛选触发钮 24px 定值 | checkbox 换 ManageCheckbox；筛选触发钮保留定值（图标钮非表单控件） |
| ManageTabs | pill=渐变按钮组、underline=下划线；与仪表盘手写胶囊两种外观并存 | +`segment`、删 pill（D①） |
| ManageSheet | modal 态宽度固定 px，窄屏溢出风险 | +`max-width: 94vw`（D⑦） |
| ManageFormField | 无问题 | 不动 |
| ManageImageUpload | 隐藏 file input 合规 | 不动 |
| ManageDialogHost | 基线活跃使用（App.vue 挂载） | 不动 |
| StatusTag / SourceCandidateDialog | 无问题；后者宽度 560px 由 BaseDialog 兜底自动覆盖 | 不动 |

### 2.2 base/* 与 layout/*

| 组件 | 处置 |
|---|---|
| BaseDialog | +`max-width: 94vw`（D⑦） |
| BasePagination | 页码 chip 44px 定值**保留**（触控目标刻意为之，不属表单控件） |
| BaseButton / Toast / Confirm / Empty / Icon / Image / Skeleton / Lightbox | 不动 |
| ManageLayout / ManageHeader / ManageSidebar / PublicHeader 等 | 不动（壳层与公共区；Sidebar 裸 button 导航项非表单控件） |

### 2.3 views/manage 逐页

| 页面 | 处置 |
|---|---|
| DashboardView | 两组手写胶囊迁 segment（D②）；顶部"刷新"min-h-44px 文字按钮保留（触控目标） |
| FilmListView | 状态页签 → segment；排序菜单 → ManageSelect v2（D⑤）；工具栏搜索框 → `size="sm"` |
| UserListView / RegisterApplyListView / SessionListView / CronListView / TelemetryView / LiveSourceListView / CollectListView | 与 sm 按钮同行的工具栏输入/下拉 → `size="sm"`（以"同行有 size=sm 按钮"为准逐行核对，没有按钮的行不动） |
| DanmuConfigCard | checkbox 替换（顺带消灭悬空 token `--jc-primary`） |
| FilmCollectDialog | checkbox 替换 |
| FilmClassView | 父/子分类删除 icon 按钮、"添加子分类"按钮——**组合组件候选**交 R1 评审（§6） |
| FileGalleryView | 列表项删除按钮——同上候选 |
| FileUploadView | "重试"link 裸按钮——同上候选 |
| 其余页 | 不动 |

## 3. 组件契约

### 3.1 `web/src/composables/useDismiss.ts`

```ts
/** 浮层关闭逻辑：点击 anchor 外部 / ESC 时触发 onClose。anchor 是"包含触发钮+浮层"的容器。 */
function useDismiss(anchor: Ref<Element | null>, isOpen: Ref<boolean>, onClose: () => void): void
```
- isOpen 为 true 时挂 `window` 的 `pointerdown`（capture）与 `keydown`；false 时卸载；组件卸载时清理。
- 点击目标在 anchor 内 → 忽略（触发钮自己的 click 负责 toggle）。
- FilmListView 现有内联实现（`onClickOutside(catFilterRef/sortRef, …)` + `useEventListener(window,'keydown',…)`，FilmListView.vue:196-210）迁移到 useDismiss，行为不变。

### 3.2 `web/src/composables/useListNavigation.ts`

```ts
interface ListNavOptions {
  itemCount: () => number
  /** 打开时初始 active（通常=当前选中项索引）；默认 0 */
  initial?: () => number
  onConfirm: (index: number) => void
  onClose: () => void
}
function useListNavigation(opts: ListNavOptions): {
  activeIndex: Ref<number>
  /** 挂在触发钮（或浮层）上的 keydown 处理：↑↓ 循环移动、Home/End、Enter/Space 确认、Esc 关闭；已处理的方向键 preventDefault */
  onKeydown: (e: KeyboardEvent) => void
  /** 打开浮层时调用：重置 activeIndex = opts.initial() */
  open: () => void
}
```
- spec 用例：循环移动（末→首）、Enter 确认回调带索引、Esc 关闭回调、itemCount=0 时不动作、open() 重置。

### 3.3 ManageTabs `segment` 变体（D①/D⑨）

- 容器：`<div role="tablist" class="flex rounded-full bg-elevated p-0.5 text-xs">`
- 项：`<button type="button" role="tab" :aria-selected data-focusable="true" class="px-3 py-1 rounded-full min-h-[28px] transition-colors">`，激活 `bg-surface shadow text-primary`，未激活 `text-secondary`（**无 hover 类，照抄现状**）。
- count 徽标：与 pill 同款文本形式 `（{{ count }}）` 内联在 label 后。
- underline 变体与 count 逻辑不动；**pill 分支删除**，`variant` 类型收窄为 `'segment' | 'underline'`（默认 `segment`）。
- 迁移点：DashboardView.vue:358-410（两组）→ `<ManageTabs variant="segment" :model-value="timeFilter" :tabs="..." @update:model-value="switchTime" />` 与 statusFilter 同理；FilmListView.vue:248、FailureListView.vue:176、FileGalleryView.vue:102、BannerListView.vue:388 加 `variant="segment"`。
- ManageTabs.spec.ts：pill 用例改写为 segment（激活类断言 `bg-surface`/`shadow`），其余用例保留。

### 3.4 `web/src/components/manage/ManageCheckbox.vue`（D③）

```ts
interface Props {
  modelValue: boolean
  indeterminate?: boolean
  disabled?: boolean
  label?: string            // 兜底文案；复杂内容用默认 slot
}
emit: 'update:modelValue': [value: boolean]
```
- DOM：`<label class="inline-flex items-center gap-[var(--jc-space-2)] cursor-pointer">` + `<input type="checkbox" class="absolute h-0 w-0 opacity-0" (几何位置保留，供 TV 空间导航)>` + 自绘方框 `<span aria-hidden="true">` + 文案。
- 方框视觉：16×16（`w-4 h-4`）、`rounded-[var(--jc-radius-sm)]`、`border border-strong bg-elevated`；checked/indeterminate：`bg-[var(--jc-brand-primary)] border-[var(--jc-brand-primary)]`，checked 画白勾（内联 SVG polyline）、indeterminate 画白横杠；hover 未选中时 border 提亮；`:focus-visible` 时方框 `box-shadow: var(--jc-shadow-focus-ring)`；disabled 整体 `opacity-50 cursor-not-allowed`。
- `input` 挂 `data-focusable="true"` + `v-model` 不用（`:checked` + `@change` 手控，indeterminate 不被 Vue 重置）。
- 替换点（§2.3）+ ManageTable 筛选面板选项、ManageCascader 行内（`:indeterminate="isPartialChecked(node)"`）。

### 3.5 ManageSelect v2（D④/D⑤/D⑥）

```ts
export interface SelectOption { label: string; value: string | number }
interface Props {
  modelValue: string | number | undefined
  options: SelectOption[]
  disabled?: boolean
  placeholder?: string
  ariaLabel?: string
  size?: 'sm' | 'default' | 'lg'      // 默认 'default'（§3.6）
  teleport?: boolean                   // 默认 false；弹窗内裁剪时启用（D⑧）
}
emit: 'update:modelValue': [value: string | number]
slots: option(scoped { option, active, selected }), trigger(scoped { selected: SelectOption | undefined })
```
- **触发钮**：`<button type="button" class="与 ManageInput 同族（bg-elevated border border-default rounded-[var(--jc-ctrl-radius)] text-… focus:border-strong focus:shadow-focus disabled:opacity-50）+ size 档高/字/内边距">`，内容 = trigger 槽或当前选中 label（无选中显示 placeholder，`text-muted`），右侧 chevron-down 图标（BaseIcon）；`aria-haspopup="listbox"` + `:aria-expanded` + `data-focusable="true"`。
- **浮层**：外层 `relative` 包裹；浮层 `absolute top-full left-0 mt-1 z-50 w-full max-h-[320px] overflow-auto rounded-lg border border-subtle bg-surface shadow-lg py-1`（对齐 FilmListView 级联面板范式）；选项 `role="option"` + `:aria-selected` + `data-focusable="true"` + `w-full text-left px-3 py-1.5 text-sm hover:bg-[rgba(255,255,255,0.06)]`，选中项 `text-[var(--jc-brand-purple)]`，active（键盘）项加 `bg-[rgba(255,255,255,0.06)]`。
- **行为**：开关=触发钮 click；`useDismiss` 管外点+ESC；`useListNavigation` 管键盘（open() 时 active=选中项索引）；关闭后焦点归还触发钮；打开时浮层内选项按 DOM 序可被空间导航直接聚焦。
- **原类型回传**：emit 时按 `String(o.value)` 匹配 options 取原始 value（保留现实现语义，数字值不被字符串化）。
- **teleport**：true 时浮层 Teleport 到 body + fixed 定位（getBoundingClientRect），其余不变；仅弹窗内实测裁剪的使用点启用。
- 迁移点：全部 9 个使用页**零改动**（API 兼容）；FilmListView.vue:294-316 排序菜单删除，改用 `<ManageSelect>`（trigger 槽放 icon+label+chevron，option 槽放 icon）。

### 3.6 size 档映射（D⑥，与 ManageButton.vue:102-109 同构）

| size | 高 | 水平内边距 | 字号 |
|---|---|---|---|
| sm | `--jc-ctrl-h-sm` | `--jc-ctrl-px-sm` | `--jc-ctrl-fs-sm` |
| default | `--jc-ctrl-h` | `--jc-ctrl-px` | `--jc-ctrl-fs` |
| lg | `--jc-ctrl-h-lg` | `--jc-ctrl-px-lg` | `--jc-ctrl-fs-lg` |

- 令牌（theme.css，2026-11-05 重构后）：**标准档按端决定** —— 移动 <768 = 24／平板（默认）= 28／桌面 ≥1280 = 32／TV `[data-mode='tv']` = 48；
  `sm`/`lg` **由标准档派生**（`sm = 标准 −4`、`lg = 标准 +8`，写公式不逐端写值）；**移动端 `sm` 钉死 = 标准**；
  字号 `--jc-ctrl-fs` 12/13/14（TV 18）、水平内距 `--jc-ctrl-px` 8/10/12（TV 20）、纵向内距 `--jc-ctrl-py` 2/3/4（TV 8）。
- 组件**默认不标 size**（跟随端标准）；只有需要"紧凑/突出"的少数场景才显式标 `sm`/`lg`。
- ManageInput：class 拼接改为由 size 计算的 computed；`min-h` 保持（textarea 语义不变，这里只改高度档）。
- number spinner 隐藏：`assets/styles/theme.css` 加一条全局规则 `input[type='number']::-webkit-outer-spin-button, input[type='number']::-webkit-inner-spin-button { -webkit-appearance: none; margin: 0; }`（chrome61 可用）。

## 4. 弹窗宽度兜底（D⑦）

- `ManageSheet.vue` modal 态面板与 `BaseDialog.vue` 面板：加 `max-width: 94vw`（scoped style 或原子类 `max-w-[94vw]`，跟随文件现状风格）。
- 调用点不动：FilmCollectDialog 880、FilmManualAddDialog 720、LiveSourceListView 1120/480/420、SourceCandidateDialog 560 等保持固定 px。
- 验证：窄窗（<720px）打开以上弹窗不溢出、可横向完整呈现。

## 5. 分辨率四档规则（评审与实现共同遵守）

1. 表单控件（input/select/checkbox/switch/button）尺寸一律走 `--jc-ctrl-*`/`--jc-switch-*` 令牌——四档自动生效，**禁止新写固定 px 高度**。
2. 例外（刻意定值，保留）：segment 胶囊 28px（D⑨）、BasePagination chip 44px、ManageTable 表头筛选钮 24px、Dashboard 刷新按钮 44px。
3. 浮层 `max-h-[320px]` 内滚动，四档一致。
4. 评审员核对方式：读代码核对令牌引用（禁止以"截图"为凭）。

## 6. 组合组件评估规则（R1 评审员执行）

- 提案门槛：**≥3 处重复**，或 2 处且存在漂移风险；且实现成本 ≤1 个新文件 + ≤3 个页面触点；否则只记 `deferred` 建议，不实施。
- 已知候选（按此规则裁决）：
  - `ManageIconButton`：FilmClassView 删除 icon 按钮 ×2 + FileGalleryView 删除按钮（3 处，均"裸 button + hover 变 danger"）；
  - `ManageErrorState`：FilmListView.vue:221-231 与 LiveSourceListView.vue:496-502 的"错误文案+重试按钮"块（2 处）；
  - FileUploadView "重试"link 裸按钮 → ManageButton `text`（若视觉一致）。
- 本批**至多实施 2 个**提案（防止范围膨胀），其余记入交付报告"建议后续"。

## 7. 铁律（全部批次与评审适用）

1. **不 push、不部署**；每批次/每轮修复各一笔 commit（前缀 `feat(manage-ui)`/`refactor(manage-ui)`/`fix(manage-ui)`/`docs(manage-ui)`，中文主题）。
2. **e2e 不写不跑**（playwright 只读；允许修改 `web/tests/e2e/**` 的选择器文件但不执行）。
3. 公共区不动：`views/public`、`views/auth`、`PublicHeader/PublicFooter/MobileTabbar/AuthLayout`。
4. 不引第三方组件库/新依赖；BaseIcon 内联 SVG 体系保留。
5. 新代码：中文注释、说明设计意图、密度沿用所在文件；`data-focusable="true"` 必带；样式走 `--jc-*` 令牌与 UnoCSS 语义 shortcuts。
6. 测试：组件新能力必须带 vitest spec（TDD 先红后绿）；页面行为断言优先，不断言内部类名（ManageTabs segment 激活类断言除外——那是视觉契约本身）。
7. 门禁命令（本机 pnpm 需经 node 运行）：
   - 单文件：`node C:/Users/kilim/AppData/Roaming/npm/node_modules/pnpm/bin/pnpm.cjs -C <web根> exec vitest run <文件>`
   - 全量：`node …/pnpm.cjs -C <web根> run test`
   - 类型：`node …/pnpm.cjs -C <web根> run type-check`
   - 构建：`node …/pnpm.cjs -C <web根> run build`（= vue-tsc + vite build，最终门禁必须跑这条）
8. 遇门禁不可通过或指令矛盾：如实上报（escalate），不得造假绕过。

## 8. 验收标准

1. `grep '<select'` 与 `grep 'type="checkbox"'` 在 `views/manage/**` + `components/manage/**` 中仅命中新组件内部实现（ManageSelect v2 无 `<select>`；ManageCheckbox 内一个隐藏 input）。
2. 手写胶囊/手写下拉浮层为零（DashboardView 两组、FilmListView 排序菜单均已组件化）。
3. 仪表盘胶囊 DOM 类名与改前逐字一致；影片列表工具栏输入框与按钮同高（同档令牌）。
4. 全部弹窗窄窗（<720px）不溢出（max-width:94vw）。
5. 悬空 token 引用（`--jc-primary`/`--jc-brand`）清零。
6. `vitest run` 全绿、`pnpm run build` 全绿；每批次/每轮有对应提交。
7. 新组件均有 spec；四档分辨率以令牌核对通过。
