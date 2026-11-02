# 管理后台 UI 组件化规格 (SDD)

> 分支 `manage-ui-rework`，web 根 = `.worktrees/manage-ui/web`（下文以 `web/` 相对路径书写）。
> 本文档 = 第 1 步方案脑暴（§1 决策记录）+ 第 2 步组件规格（§2~§3）。
> 对应 TDD 实施计划见 `docs/specs/manage-ui-plan.md`。

## 0. 背景与硬性需求映射

用户硬性需求 → 本规格条目：

| # | 硬性需求 | 规格落点 |
|---|---------|---------|
| 1 | 移动端/桌面端都要照顾；桌面端部分页面过空 | §2.17 ManageLayout 内容最大宽度 + §2.1 表格紧凑密度（决策 D5） |
| 2 | 表格/表单/按钮/表头筛选组件化，先增强不重造 | §2.1 ManageTable v2、§2.4 ManageSelect、§2.8 StatusTag、§2.9 ManageTabs、§2.10~2.15 表单组件增强（全部是对现存组件的增强，仅 5 个纯新增文件） |
| 3 | 弹窗函数化 + 组件化封装（参考 useConfirm） | §2.5 useManageDialog + ManageDialogHost（决策 D4） |
| 4 | 表格自适应页面高度 + 最低高度；内置分页 | §2.1 fit/minHeight + total/current 内置分页、§2.2 BasePagination 钳制、§2.3 usePagedList（决策 D1/D2） |
| 5 | 表头筛选（列头筛选/排序）组件化 | §2.1 Column.sortable/sorter/filter + sort-change/filter-change（决策 D3） |

复核发现的覆盖情况：§3 全局约定逐条收敛（toast 口径、select、时间格式化、状态映射、弹窗选型），§2 各组件行为规格直接内嵌对应修复。

盘存结论（本仓实测）：

- `ManageTable.vue` 仅被 manage 区 7 个页面使用（UserListView/FilmListView/CronListView/CollectListView/FailureListView/RegisterApplyListView/TaskManageView/SessionListView），修改其默认行为不波及公共区。
- `BasePagination.vue` 被 manage 区 8 个页面 **加** 公共区 3 个页面（ClassifySearchView/HotBoardView/SearchView）使用，其修复只允许「新增 emit」，不允许改既有 emit 语义。
- 现有共享测试基线：`pnpm exec vitest run src/components/base/BasePagination.spec.ts src/components/base/BaseDialog.spec.ts src/components/manage` → 8 文件 / 47 测试全绿（本工作树实跑）。
- 单测风格：vitest + @vue/test-utils + `vi.mock('@/api')`；视口切换用 `useViewMode().setMode('mobile' | 'desktop')`（见 `ManageTable.spec.ts:23-31` 先例）。

---

## 1. 关键决策记录（第 1 步：方案脑暴）

### D1 表格高度自适应方案

| 候选 | 做法 | 取舍 |
|------|------|------|
| A. 纯 CSS flex 链（**采纳**） | 布局壳 main 打通高度链（`min-h-0`），ManageTable 根节点 `fit` 时 `flex flex-col`，滚动区 `flex-1 min-h-0 overflow-auto`，最低高度以 `minHeight` prop 渲染为内联 style | jsdom 可测（断言 class 与 style）；真实浏览器 flex 自动跟随窗口 resize，无需重算；缺点是要求祖先链高度贯通——由布局壳一次性解决（T0，只改 ManageLayout.vue 一处） |
| B. ResizeObserver 实测视口 | mounted 后观测容器/视口，JS 动态算 `max-height = 视口高 - 表格top - 分页高 - 余量` | 精度高但 jsdom 无布局，ResizeObserver 需 stub，可测性差；resize 时需防抖重算，TV 老内核 WebView 上 observer 兼容风险大；弃 |
| C. 视口 calc 锚定 | `max-height: calc(100dvh - offset)`，offset 在 mounted 时用 `getBoundingClientRect().top` 实测 | 依赖 DOM 实测（jsdom 恒为 0，需 stub）；`100dvh` 在 Chrome 61 老 WebView（本仓打包地板，vite.config.ts legacy targets）上不可用；弃 |

**结论**：A。最低高度默认 `320px`，移动端 card 模式忽略 fit（卡片列表自然流）。

### D2 分页内置方式

| 候选 | 做法 | 取舍 |
|------|------|------|
| A. ManageTable 内置（**采纳**） | 新增 `total/current/pageSize/pagerCount` props，`total !== undefined && total > 0` 时在卡片/表格区下方渲染 BasePagination；**只受控**（必须 `v-model:current`），组件不持有分页状态 | 单组件消掉 5+ 页的「ManageTable + BasePagination 拼接」；受控-only 语义单一（页码变化必经页面 → 发请求）；无分页页（CronListView 等）不传 total 即完全不渲染，向后兼容 |
| B. 新包装组件 ManageTableWithPagination | 组合包装，转发全部 slots | generic 组件透传 slots 类型极其繁琐；两套入口并存反而增加选型成本；弃 |
| C. 仅提供 usePagedList composable，分页 UI 仍由页面拼 | 逻辑收敛但模板样板仍在 | C 与 A 不冲突：A 收敛模板，C 收敛状态（末页回夹/序号守卫/loading/error）。两者组合为终态 |

**边界语义**：分页事件全部上抛不改内值（受控）；`total=0` 或未传 `total` 不渲染分页条；末页删空越界由 usePagedList 回夹（治本）+ BasePagination 越界钳制 emit（防御）。

### D3 表头筛选 API 形态

| 候选 | 做法 | 取舍 |
|------|------|------|
| A. 声明式列扩展（**采纳**） | `Column` 增加 `sortable?/sorter?/filter?`，ManageTable 渲染排序箭头与筛选漏斗+下拉面板，事件 `sort-change`/`filter-change` 上抛 | API 收敛在列定义，页面一行开启；与 `#cell` 插槽同一声明风格；代价是 ManageTable 复杂度上升（可控，面板为简单列表非浮层组件） |
| B. 独立 ManageTableFilter 子组件放 th 内手拼 | 灵活但每页样板重现 | 与「组件化收敛」目标相反；弃 |
| C. 纯插槽 `#th-<key>` | 最灵活 | 零收敛，各页继续各写一套；弃 |

**API 细则**（受控/非受控双模式）：`sorter?: (a,b)=>number` 提供时组件内置客户端排序（非受控，箭头自动管理）；`sortable: true` 且无 sorter 时仅 emit `sort-change`（受控，服务端排序，如 FilmListView 的 allowedSort 白名单），箭头状态由可选 prop `sortState?: { key: string; order: 'asc'|'desc' }` 显示。筛选 `filter` 恒受控（value 传入，emit `filter-change` 回传）。移动端 card 模式无表头，排序/筛选控件自动失效——移动端等价筛选由页面在 toolbar 用 ManageSelect 提供（FilmListView 现状即是），写入 §3 约定。同时 `Column.key` 从 `keyof U & string` 放宽为 `string`，新增可选 `field?: keyof U & string` 与 `render?: (row)=>VNodeChild`，消掉 CollectListView:249-254、TaskManageView:579/636、FilmListView:55 的类型 hack。

### D4 弹窗函数化封装形态

| 候选 | 做法 | 取舍 |
|------|------|------|
| A. useConfirm 同款单例 + Host 组件（**采纳**） | 模块级 `manageDialogState` 单例；`ManageDialogHost.vue` 在 App.vue 挂载一次（与 BaseConfirmDialog 同位，App.vue:133 先例）；`openManageDialog(opts): Promise<T|null>` 填 state，Host 渲染 ManageSheet + 动态组件 | 与现有 useConfirm 模式完全同构，主流程零新概念；Host 内复用 ManageSheet → sheet/fullsheet/modal 三形态自适应直接继承；代价：同屏单实例（管理端现状无叠层需求，BaseDialog 叠层 ESC 是结构性风险而非现实问题，由 §2.6 useOverlayStack 兜底） |
| B. 声明式 schema 表单 openFormDialog({schema,...}) | 简单表单零组件 | schema 表达不了上传（ManageImageUpload）、级联（ManageCascader）、动态显隐（CronListView model===1 时勾选资源站），覆盖不了就必须两套并存；弃（其覆盖面由「内容组件契约」承担） |
| C. 动态 mount 实例（h+render 到临时容器） | 支持叠层、无需 Host | 需要手动注入 appContext（pinia/router），生命周期待收（unmount 时机/过渡），复杂度与收益不成比；弃 |

**内容组件契约（关键决策）**：动态组件**完整自治**（含 footer 按钮），通过 `emit('confirm', payload)` / `emit('cancel')` 与 Host 交互。理由：现有页面表单弹窗就是 ManageSheet + 自带 footer（UserListView.vue:317-367、CronListView.vue:342-395），契约对齐现状迁移成本最低；Host 统一 footer 则要求所有内容组件 defineExpose({submit})，反而重写。

**Promise 语义**：`openManageDialog` 返回 pending Promise；`confirm(payload)` → 关闭并 `resolve(payload)`；ESC/遮罩/`cancel`/Host 未挂载/重复 open 挤掉 → 关闭并 `resolve(null)`。调用方 `if (!payload) return`。

**选型规则**（写入 §3）：确认型 → 既有 `confirm()`；跨页复用或含独立提交逻辑的表单 → `openManageDialog`；一次性内联小表单 → 组件式 `ManageSheet` 直接用（不废弃）；纯展示（无表单字段）→ `BaseDialog`。全面禁 `window.confirm/alert`。

### D5 桌面密度解法（页面过空）

| 候选 | 做法 | 取舍 |
|------|------|------|
| A. 内容最大宽度 + 居中（**采纳**） | ManageLayout main 内层 `mx-auto w-full max-w-[var(--jc-content-max-w)]`（令牌已存在，theme.css:135，值 1440px） | 一行布局壳改动覆盖全部 21 页；超宽屏两侧留白、内容密度集中；极宽表格在 1440 内放不下时表格区自带 overflow 滚动兜底 |
| B. 密度令牌 + 表格紧凑（**采纳**） | ManageTable 桌面态单元格纵向 padding 由 `py-[var(--jc-space-3)]` 改 `py-[var(--jc-table-cell-py,10px)]`（新令牌，theme.css 归表格线），桌面一屏多显示约 15~20% 行数 | ManageTable 仅 manage 区使用（已核实），直接改默认无兼容风险；令牌化后 TV/移动端可按 data-mode 覆写 |
| C. 页面级布局重组（双栏/栅格） | Dashboard/Telemetry 卡片栅格化 | 页面级个性化，非共享组件职责，逐页工作量大；弃（列入 exclusions） |

**结论**：A + B 组合，均收在布局壳/共享组件层，迁移组零成本继承。移动端不启用 A/B（卡片自然流 + 现有 44px 触控密度不动）。

---

## 2. 组件规格

命名总览（新增 5 个纯新文件）：

| 文件 | 类型 | 归属线 |
|------|------|--------|
| `web/src/composables/usePagedList.ts` | 新建 | 表格线 |
| `web/src/components/manage/ManageSelect.vue` | 新建 | 表格线 |
| `web/src/components/manage/StatusTag.vue` | 新建 | 表格线 |
| `web/src/components/manage/ManageTabs.vue` | 新建 | 表格线 |
| `web/src/composables/useManageDialog.ts` + `web/src/components/manage/ManageDialogHost.vue` | 新建 ×2 | 弹窗表单线 |
| `web/src/composables/useOverlayStack.ts` | 新建 | 弹窗表单线 |
| `web/src/composables/useAsyncAction.ts` | 新建 | 弹窗表单线 |

其余全部是对现存文件的增强/修复。

### 2.1 ManageTable（改造）

用途：manage 区统一列表组件——桌面表格 / 移动卡片双形态；内置受控分页；桌面高度自适应 + 最低高度；声明式表头排序/筛选；列内联渲染。

**API 契约**（`web/src/components/manage/ManageTable.vue`，generic `T extends Record<string, any>` 保持）：

```ts
import type { VNodeChild } from 'vue'

export interface ColumnFilterOption {
  label: string
  value: string | number
}

export interface ColumnFilter {
  options: ColumnFilterOption[]
  /** 受控值；undefined = 未筛选；multiple 时为数组 */
  value?: string | number | Array<string | number>
  /** 多选，默认 false（单选） */
  multiple?: boolean
}

export interface Column<U> {
  /** 列标识（v-for key 与事件 key），允许派生列名，不再限定 keyof U */
  key: string
  /** 取值字段；省略时回落 key（旧用法 `key: 'name' as const` 继续成立） */
  field?: keyof U & string
  label: string
  width?: string
  align?: 'left' | 'center' | 'right'
  /** 受控排序开关（无 sorter 时仅 emit） */
  sortable?: boolean
  /** 客户端排序回调；提供后点击表头自动排序（非受控） */
  sorter?: (a: U, b: U) => number
  filter?: ColumnFilter
  /** 列内联渲染；优先级 render > #cell 插槽 > 默认文本 */
  render?: (row: U) => VNodeChild
}

export interface ManageTableProps<U> {
  columns: Column<U>[]
  rows: U[]
  rowKey: keyof U & string
  loading?: boolean
  empty?: string
  mobileVariant?: 'card' | 'scroll'          // 默认 'card'（不变）
  actionsWidth?: string                      // 默认 '180px'（不变）
  /** 服务端总数；传了才渲染内置分页条 */
  total?: number
  /** 受控页码（配 v-model:current 使用） */
  current?: number
  pageSize?: number                          // 默认 20
  pagerCount?: number                        // 透传 BasePagination，默认 7
  /** 桌面高度自适应，默认 true；mobile card 模式忽略 */
  fit?: boolean
  /** fit 模式下滚动区最低高度，默认 '320px' */
  minHeight?: string
  /** 受控排序箭头显示（无 sorter 的 sortable 列用） */
  sortState?: { key: string; order: 'asc' | 'desc' }
}

// emits
'update:current' (page: number)
'page-change'   (page: number)             // 与 update:current 同时 emit，语义同 BasePagination.change
'sort-change'   (s: { key: string; order: 'asc' | 'desc' | null })
'filter-change' (f: { key: string; value: string | number | Array<string | number> | undefined })

// slots：toolbar / cell / actions / mobile-card 全部保留，签名不变
```

**行为规格**：

1. 内置分页：`total !== undefined && total > 0` 时在内容区之后渲染 `<footer class="jc-table__pagination">` 内嵌 `BasePagination`（透传 current/pageSize/total/pagerCount）。组件不持有页码状态：点击分页 → 同时 emit `update:current` 与 `page-change`，不改内值。`total === undefined || total === 0` → 不渲染分页条。
2. fit 高度：`fit !== false` 且非 mobile card 模式时根 section 挂类 `jc-table--fit`（`display:flex; flex-direction:column`），内容滚动容器挂 `jc-table__scroll`（`flex:1; min-height:0; overflow:auto`，横向滚动保留），并以 `:style="{ minHeight: props.minHeight }"` 渲染最低高度。分页 footer 在滚动区之外（不随内容滚动）。mobile card 模式（`useCardMode === true`）不挂 `jc-table--fit`、不渲染 minHeight。
3. 桌面紧凑密度：桌面/TV 表格态的 th/td 纵向 padding 使用 `var(--jc-table-cell-py, 10px)`（新令牌，theme.css 增加；移动卡片模式不变）。不再提供 density prop（ManageTable 仅 manage 区使用，直接改默认）。
4. 客户端排序：列提供 `sorter` 时，点击该列表头循环 `asc → desc → 清除`；组件内部以 computed 输出排序后的 rows（不修改 props.rows），箭头方向组件自管。
5. 受控排序：`sortable === true` 且无 `sorter` 时点击表头仅 emit `sort-change({ key, order })`（首次 `asc`）；箭头方向由 `sortState` 显示，未传不显示箭头。
6. 表头筛选：列提供 `filter` 时表头 label 旁渲染漏斗按钮（类名 `jc-th-filter`，`data-focusable="true"`）；点击展开内联下拉面板（`jc-th-filter__panel`，absolute 定位，不做 Teleport）；单选点击 option → emit `filter-change({ key, value })`，再次点击当前值 → `value: undefined`（清除）；`multiple: true` 时面板为 checkbox 组，每次勾选 emit 完整数组，全部取消 → `value: []`？——否，与单选对齐：全取消 emit `undefined`。面板关闭：点面板外 / ESC / 再点漏斗按钮。`filter.value` 命中的 option 高亮。
7. 单元格渲染优先级：`col.render?.(row)` → `#cell` 插槽 → 默认文本 `row[col.field ?? col.key] ?? '—'`。`key` 为派生列名且无 field/render/#cell 时显示 `—`（不再需要 `as unknown as` 断言）。
8. `#cell` 插槽的 `value` 参数 = `row[col.field ?? col.key]`（派生列为 undefined，页面从 row 自取）。
9. 现有 loading 骨架 / BaseEmpty / mobile card 默认渲染行为全部不变。

**验收标准**（每条 = 一条单测，落 `ManageTable.spec.ts` 扩展）：

1. desktop + 默认 fit → 根节点含 `jc-table--fit`，滚动容器内联 style 含 `min-height: 320px`。
2. `min-height="480px"` → style 含 `min-height: 480px`。
3. `setMode('mobile')` + card 模式 → 不含 `jc-table--fit`。
4. `total=100, current=2, pageSize=20` → 存在 `.jc-table__pagination` 且其中页码含「2」。
5. 点击分页第 3 页 → `emitted('page-change')` 与 `emitted('update:current')` 均为 `[[3]]`。
6. 不传 total → 不存在 `.jc-table__pagination`；`total=0` 同。
7. `total=5, current=9` → 正常渲染不抛错（钳制归 BasePagination/usePagedList）。
8. `sortable` 列点表头 → `emitted('sort-change')` 为 `[[{ key, order: 'asc' }]]`，再次点击 `desc`，第三次 `null`。
9. `sorter` 列点击 → rows 按 sorter 升序渲染（DOM 行序断言），再点反向；`sort-change` 仍 emit。
10. `filter` 单选：点漏斗 → 面板出现；点 option `value=1` → `emitted('filter-change')` 为 `[[{ key, value: 1 }]]`；再点同值 → `[[.., { key, value: undefined }]]`。
11. `filter.multiple`：勾两个 option → emit `value` 为长度 2 的数组；取消全部 → `undefined`。
12. ESC 关闭筛选面板：面板开着时 document 触发 Escape → 面板消失。
13. `render` 回调返回 `h('b', 'x')` → 对应 td 内出现 `<b>x</b>`。
14. 派生列（`key: 'status'` 无 field）+ `#cell` 插槽 → 渲染插槽内容且组件不抛错。
15. 派生列无任何渲染途径 → td 文本为 `—`。
16. 回归：本文件既有 7 条用例（desktop table / mobile card / scroll / loading / empty / slot 覆盖）全部继续通过。

### 2.2 BasePagination（修复）

用途不变（页码条，manage + 公共区共用）。修复复核 #2：total 收缩后越界 current 不回传。

**变更**：新增 `watch([totalPages])`：当 `props.current > totalPages.value` 时 `emit('update:current', totalPages.value)`（**不** emit `change`，避免双触发）。仅此一处行为变化；公共区 3 个只听 `@change` 的页面不受影响（不监听 update:current 时无行为变化，显示层 pages computed 本就 clamp）。

**验收标准**（`BasePagination.spec.ts` 追加）：

1. `current=9, total=5, pageSize=5`（totalPages=1）→ `emitted('update:current')` 为 `[[1]]`，且未 emit `change`。
2. `current=12, total=40, pageSize=10`（totalPages=4）→ `emitted('update:current')` 为 `[[4]]`。
3. `current=2, total=40, pageSize=10` → 不 emit（正常范围内）。
4. 回归：既有 5 条用例通过。

### 2.3 usePagedList（新建）

用途：收敛 5 个分页列表页重复的 load/search/changePage/末页回夹/请求序号守卫/loading/loadError 样板（复核 #2 的治本层）。

**API 契约**（`web/src/composables/usePagedList.ts`）：

```ts
interface PagedFetchResult<T> {
  list: T[]
  total: number
}

interface PagedListOptions<T> {
  /** 拉取函数：页码与页大小由 usePagedList 传入；筛选参数由调用方闭包捕获 */
  fetch: (page: number, pageSize: number) => Promise<PagedFetchResult<T>>
  /** 初始页大小，默认 20 */
  pageSize?: number
  /** 是否立即 load()，默认 true */
  immediate?: boolean
}

interface PagedListState<T> {
  rows: Ref<T[]>
  total: Ref<number>
  current: Ref<number>
  pageSize: Ref<number>
  loading: Ref<boolean>
  /** 失败信息文案（= Error.message），成功时清空 */
  loadError: Ref<string>
  /** 关键词等条件变化后调用：回第 1 页并加载 */
  search: () => void
  /** 按当前页码加载（重试 = 直接调它） */
  load: () => Promise<void>
}

function usePagedList<T>(options: PagedListOptions<T>): PagedListState<T>
```

**行为规格**：

1. `immediate !== false` 时 setup 同步发起首次 `load()`（fetch(1, pageSize)）。
2. `current` 的 watch 是加载触发器：`current` 变化 → 自动 `load()`；`search()` 行为 = 若 `current === 1` 直接 `load()`，否则置 `current = 1`（由 watch 触发，避免双请求）。
3. 请求序号守卫：内部 `loadSeq`，仅采纳最后一次请求结果写 rows/total/loading/loadError；旧响应后到直接丢弃。
4. 末页回夹：若本响应 `list.length === 0 && current > 1 && total > 0 && ceil(total/pageSize) < current` → 置 `current = ceil(total/pageSize)`（watch 触发自动重拉），不再手写递归。
5. 失败：`loadError = e.message ?? '加载失败'`，rows/total 保留上次值；取消类错误（`e.name === 'CanceledError'` 或 axios isCancel）按「被新请求取代」静默丢弃，不写 loadError。
6. 成功：`rows = list`，`total = resp.total`，`loadError = ''`。
7. `pageSize` 为组件持有的 ref：服务端若回填不同 size，页面可 `paged.pageSize.value = resp.page.size` 后再写 total（不做隐式回填，避免双源）。

**验收标准**（`usePagedList.spec.ts` 新建，fetch 用 vi.fn + 可控 Promise）：

1. 默认 immediate → fetch 被调 `(1, 20)` 一次。
2. fetch resolve `{ list: [3 项], total: 64 }` → rows 长度 3、total=64、loading=false、loadError=''。
3. fetch reject `Error('boom')` → loadError='boom'、rows 保留上次值。
4. `current=3` 时 fetch 返回 `{ list: [], total: 40 }`（pageCount=2 < 3）→ 自动二次 fetch(2)。
5. 快速两次 load：第 1 次慢、第 2 次快 → 慢响应不覆盖 rows（序号守卫）。
6. `search()` 当 current=3 → current 变 1 且 fetch 仅以 (1,…) 追加一次。
7. `search()` 当 current=1 → 直接 load，无多余翻页。
8. `pageSize: 50` → fetch 首参 `(1, 50)`。
9. `immediate: false` → setup 后不自动 fetch。

### 2.4 ManageSelect（新建）

用途：统一 13 处手写原生 `<select>`（复核 #3），统一 class 串、44px 触控区与 TV 聚焦。

**API 契约**（`web/src/components/manage/ManageSelect.vue`）：

```ts
interface SelectOption {
  label: string
  value: string | number
}
interface Props {
  modelValue: string | number | undefined
  options: SelectOption[]
  disabled?: boolean
  /** modelValue 为 undefined 时显示的占位项 */
  placeholder?: string
  /** 无 label 包裹时必填（无障碍） */
  ariaLabel?: string
}
emit 'update:modelValue' (value: string | number)
```

**行为规格**：

1. 根元素 `<select>`，class 与 ManageInput 同族（bg-elevated / border-default / rounded-md / min-h-[44px] md:min-h-[36px] / text-sm / focus:border-strong），固定 `data-focusable="true"`。
2. `modelValue` 严格相等匹配 option.value（类型一致性由页面保证；数字 option emit 数字）。
3. `placeholder` 存在且 `modelValue === undefined` 时插入 `<option value="" disabled hidden>{{ placeholder }}</option>` 并选中它。
4. change 事件按 option.value 原类型 emit（不经过字符串化）。

**验收标准**（`ManageSelect.spec.ts` 新建）：

1. options 3 项 → 渲染 3 个 option。
2. modelValue=2 → value=2 的 option selected。
3. 选择 value 为数字的 option → `emitted('update:modelValue')` 为 `[[2]]`（number 而非 '2'）。
4. disabled → select.disabled=true 且无 data-focusable。
5. 恒有 `data-focusable="true"`（enabled 时）。
6. `modelValue=undefined, placeholder='请选择'` → DOM 中存在文本「请选择」的 disabled option 且被选中。
7. ariaLabel → select 的 aria-label 属性透传。

### 2.5 useManageDialog + ManageDialogHost（新建）

用途：函数发起 + 组件渲染的管理弹窗（复核 #6 的机制统一），形态继承 ManageSheet（modal/sheet/fullsheet 自适应）。

**API 契约**：

```ts
// web/src/composables/useManageDialog.ts
import type { Component } from 'vue'

interface ManageDialogOptions {
  title: string
  /** 内容组件（组件对象即可，内部 shallowRef 存放，不需要页面 markRaw） */
  component: Component
  /** 透传给内容组件的 props（不含 open 状态） */
  props?: Record<string, unknown>
  /** 移动端形态，默认 'sheet'（透传 ManageSheet.mobile-mode） */
  mobileMode?: 'sheet' | 'fullsheet'
  /** modal 宽度（透传 ManageSheet.width），默认沿用 ManageSheet 默认 */
  width?: string
}

export interface ManageDialogState {
  visible: boolean
  title: string
  component: Component | null
  props: Record<string, unknown>
  mobileMode: 'sheet' | 'fullsheet'
  width?: string
  /** 内容组件是否已声明 onHostReady（用于 Host 挂载检测与单测） */
  resolve: ((v: unknown) => void) | null
}

export const manageDialogState: ManageDialogState        // 模块单例（同 confirmState 先例）
export const manageDialogHostMounted: Ref<boolean>       // Host onMounted 置 true / onBeforeUnmount 置 false

export function openManageDialog<T = unknown>(options: ManageDialogOptions): Promise<T | null>
export function answerManageDialog(result: { type: 'confirm'; payload: unknown } | { type: 'close' }): void
```

```vue
<!-- web/src/components/manage/ManageDialogHost.vue -->
<!-- 模板骨架（行为规格）：
<ManageSheet
  :model-value="manageDialogState.visible"
  :title="manageDialogState.title"
  :mobile-mode="manageDialogState.mobileMode"
  :width="manageDialogState.width"
  @update:model-value="(v) => !v && answerManageDialog({ type: 'close' })"
>
  <component
    :is="manageDialogState.component"
    v-bind="manageDialogState.props"
    @confirm="(payload: unknown) => answerManageDialog({ type: 'confirm', payload })"
    @cancel="() => answerManageDialog({ type: 'close' })"
  />
</ManageSheet>
-->
```

**内容组件契约**：`emits: (e: 'confirm', payload: T) | (e: 'cancel')`；内容组件自渲染 footer（取消按钮 emit `cancel`，提交按钮 emit `confirm(表单值)`）。Host 不注入任何 footer。

**行为规格（Promise 语义与生命周期）**：

1. `openManageDialog` → 若 `manageDialogState.resolve` 非空（有待决弹窗）→ 先以 `null` resolve 旧 Promise（同 useConfirm.ts:48-51 先例）→ 填充 state、`visible = true`、保存新 resolve → 返回新 Promise。
2. `Host 未挂载`（`manageDialogHostMounted === false`，含单测未挂 Host、公共区误调用）→ 立即 `resolve(null)` + `console.warn('[useManageDialog] Host 未挂载')`，不置 visible。
3. `answerManageDialog({type:'confirm', payload})` → `visible=false`、`resolve(payload)`、清空 resolve。
4. `answerManageDialog({type:'close'})` → 同上但 `resolve(null)`。ESC / 遮罩点击（经 ManageSheet `update:modelValue=false`）/ 内容组件 `cancel` 都汇入此路径。
5. Host `onBeforeUnmount`（路由切走）时若仍有待决 → `resolve(null)`，防悬挂。
6. Host 挂载点：`App.vue` 中 BaseConfirmDialog 同位（App.vue:130 附近 lazy 分支），公共区调用同样可渲染（组件通用），manage 区为唯一约定使用方。

**验收标准**（`useManageDialog.spec.ts` 新建，须 mount Host）：

1. mount Host → `openManageDialog({ title, component: Comp })` → `manageDialogState.visible === true`，Host 内渲染了 Comp（findComponent 命中）。
2. Comp 实例 `vm.$emit('confirm', { a: 1 })` → promise resolves `{ a: 1 }` 且 visible=false。
3. Comp `$emit('cancel')` → resolves `null`。
4. Host 根上触发 Escape（ManageSheet document keydown 路径）→ resolves `null`。
5. pending 未决时第二次 open → 第一个 promise resolves `null`，state.component 为第二个组件。
6. `props: { initial: row }` → Comp 收到 `initial` prop。
7. 未 mount Host 直接 open → resolves `null`（且 console.warn 被调用，vi.spyOn 断言）。
8. unmount Host（wrapper.unmount）时 pending → resolves `null`。
9. `width: '640px'` → modal 容器 style 含 maxWidth: 640px；`mobileMode:'fullsheet'` + setMode('mobile') → 容器类含 `jc-sheet--fullsheet`。

### 2.6 useOverlayStack（新建 + 三弹层接入）

用途：修复复核 #7 —— 叠层弹窗 ESC 全关、`data-jc-modal-open`/scroll lock 单槽互踩；ESC 归属最顶层，计数式滚动锁。

**API 契约**（`web/src/composables/useOverlayStack.ts`）：

```ts
/** 申请一层弹层：计数 +1；返回本层序号（从 1 开始） */
export function acquireOverlay(): number
/** 释放本层：计数 -1；归零时清理 body 标志与滚动锁 */
export function releaseOverlay(seq: number): void
export function overlayCount(): number
/** seq 是否为当前最顶层（ESC 归属判断） */
export function isTopOverlay(seq: number): boolean
```

**行为规格**：

1. `acquireOverlay`：计数 +1 返回序号；计数 1→N 变更时（即首次）`document.body.setAttribute('data-jc-modal-open','1')` + `document.body.style.overflow='hidden'`；已锁时重复 acquire 不重复设置。
2. `releaseOverlay`：计数 -1；归零时移除属性并 `overflow=''`；乱序释放（先放顶层再放底层）计数仍正确。
3. `isTopOverlay(seq)`：`seq === 当前最大未释放序号`。
4. 接入约定（三个组件同改）：打开时 `mySeq = acquireOverlay()`，关闭/卸载时 `releaseOverlay(mySeq)`；keydown 处理改为 `e.key === 'Escape' && props.visible && closeOnEsc && isTopOverlay(mySeq)` 才执行关闭。落点：`BaseDialog.vue`（watch visible 与 onBeforeUnmount）、`BaseLightbox.vue`、`ManageSheet.vue`。`stopPropagation` 移除（归属判断已由 isTopOverlay 承担）。
5. 顺手修复（并入同任务避免文件冲突）：`BaseDialog.vue:136`、`BaseEmpty.vue:23` 补显式 `import BaseIcon`（复核 #16）。

**验收标准**：

1. `useOverlayStack.spec.ts`：acquire×2 → count=2 且 body 有 data-jc-modal-open、overflow hidden；release 首个 → 属性仍在；release 第二个 → 属性移除 + overflow=''。
2. `isTopOverlay`：后申请者 true、先申请者 false。
3. 乱序 release → 归零清理正确。
4. `BaseDialog.spec.ts` 追加：mount 两个 visible=true 的 BaseDialog → 派发一次 Escape → 仅顶层一个 emit update:visible=false；再派发一次 → 第二个关闭。
5. `ManageSheet.spec.ts` / BaseLightbox spec：单个弹层 ESC 行为回归不变。

### 2.7 useAsyncAction（新建）

用途：收敛各页写操作的 `busy + confirm + toast + reload` 样板；统一 toast 口径（复核 #1）——写请求一律 `{ silent: true }`，成功/失败提示由 run 单点负责。

**API 契约**（`web/src/composables/useAsyncAction.ts`）：

```ts
import type { ConfirmOptions } from '@/composables/useConfirm'

interface RunActionOptions {
  /** 实际请求；内部应传 { silent: true }（约定，TS 无法强制，spec/评审把关） */
  action: () => Promise<unknown>
  /** 确认弹窗选项；用户取消 → run 返回 false 且不执行 action */
  confirm?: ConfirmOptions
  /** 成功文案；省略则不弹成功 toast */
  successMsg?: string
  /** 成功后的回调（通常 reload） */
  after?: () => void | Promise<void>
}

function useAsyncAction(): {
  busy: Ref<boolean>
  /** 返回是否实际执行成功（确认取消/并发拦截/失败均 false） */
  run: (options: RunActionOptions) => Promise<boolean>
}
```

**行为规格**：

1. 并发拦截：`busy === true` 时 run 立即返回 false（防连点）。
2. 流程：busy=true → `confirm(options.confirm)`（若给）取消 → busy=false、return false → `await action()` → 成功：`successMsg` 有则 `toast('success', successMsg)`，`await options.after?.()`，return true → 失败：`toast('error', (e as Error).message ?? '操作失败')`，return true→false；finally busy=false。
3. 失败不 rethrow（unhandled rejection 兜住）；`CanceledError` 亦按失败 toast? —— 否：取消类错误静默（return false，不 toast），与 §2.3 口径一致。
4. 错误提示口径：action 内 silent:true → http 拦截器不弹（http.ts:152-176 handleHttpError silent 分支），此处 toast 是唯一错误提示；页面不再手写 catch toast。

**验收标准**（`useAsyncAction.spec.ts`，vi.mock('@/api/http') 的 toast + 真实 useConfirm 单例）：

1. run 成功 → action 调用、`toast` 收到 `('success', successMsg)`、after 调用、返回 true、busy 复位 false。
2. action reject `Error('x')` → toast 收到 `('error','x')`、after 未调用、返回 false。
3. 给 confirm 且 answerConfirm(false) → action 未调用、返回 false。
4. busy 中第二次 run → 立即 false，action 未二次调用。
5. successMsg 省略 → 无 success toast（仍 return true）。
6. action reject `name='CanceledError'` 的错误 → 无 error toast、返回 false。

### 2.8 StatusTag（新建）

用途：统一「状态值 → BaseTag variant/文案」映射（复核 #13）。

**API 契约**（`web/src/components/manage/StatusTag.vue`）：

```ts
type BaseTagVariant = 'default' | 'brand' | 'purple' | 'success' | 'warning' | 'danger' | 'info'
export interface StatusMapEntry { label: string; variant: BaseTagVariant }
export type StatusMap = Record<string, StatusMapEntry>

interface Props {
  /** 任意原始值；比较时 String(value) 后查 map */
  value: string | number | boolean | null | undefined
  map: StatusMap
  /** 未命中回退，默认 { label: '—', variant: 'default' } */
  fallback?: StatusMapEntry
  size?: 'xs' | 'sm' | 'md'   // 透传 BaseTag，默认 'sm'
}
```

**行为规格**：`String(props.value)` 查 map；命中渲染 `BaseTag :variant="entry.variant" :size="size"` 内容为 `entry.label`；未命中渲染 fallback；value 为 null/undefined 直接 fallback。

**验收标准**：命中（value='success' → BaseTag 文本「成功」variant=success）；布尔 true → 查 key 'true'；未命中 → fallback 文本 `—` variant default；自定义 fallback 生效。

### 2.9 ManageTabs（新建）

用途：统一三种页签形态（复核 #13：FilmListView/FailureListView 按钮组、TaskManageView 下划线式）。

**API 契约**（`web/src/components/manage/ManageTabs.vue`）：

```ts
export interface TabItem<V extends string | number = string> {
  value: V
  label: string
  /** 可选计数徽标 */
  count?: number
}
interface Props {
  modelValue: V
  tabs: TabItem<V>[]
  /** 'pill' = 按钮组（默认，FilmListView 现样式）；'underline' = 下划线 tab */
  variant?: 'pill' | 'underline'
}
emit 'update:modelValue' (value: V)
```

**行为规格**：容器 `role="tablist"`，每项 `role="tab"` + `aria-selected` + `data-focusable="true"`；active 项 pill 态渲染 BaseButton variant=gradient、非 active variant=ghost（现状同构）；underline 态 active 项类 `jc-tab--active`（底边框高亮）；count > 0 时渲染数字徽标 span。点击 emit `update:modelValue`（组件不改内值）。

**验收标准**：渲染 tabs 数量；点击第 2 项 → emit `[[tab2.value]]`；active 项 aria-selected=true；variant='underline' → 容器类含 `jc-tabs--underline`；count 渲染数字。

### 2.10 ManageSheet（增强）

用途不变（manage 区统一弹窗容器，ManageDialogHost 的底座）。

**变更**（复核 #8）：

1. `watch(modelValue)` 增加 `{ immediate: true }`（对齐 BaseDialog.vue:80）——挂载即开的弹层 ESC/滚动锁/标志位全部生效。
2. 新增 `width?: string` prop（默认 '480px'）：modal 形态容器 `:style="{ width: '90%', maxWidth: props.width }"`（语义对齐 BaseDialog.width）。
3. modal 形态 header 恒渲染：title 为空时只显示关闭按钮（修复 :97 `v-if="title"` 导致无关闭按钮）。
4. grabber（:123）注释修正：删除「下拉关」声明，关闭方式 = 遮罩点击 / ESC / footer 按钮（touch 下滑手势不做，见 exclusions）。
5. ESC 监听接入 useOverlayStack（isTopOverlay 判定，同 §2.6）。

**验收标准**（`ManageSheet.spec.ts` 追加）：

1. `mount({ props: { modelValue: true } })`（挂载即开）→ `document.body.style.overflow === 'hidden'`。
2. width='640px' → modal 容器 style 含 maxWidth: 640px。
3. 无 title → modal 内关闭按钮存在。
4. ESC 单实例行为回归（既有 6 条用例通过）。

### 2.11 ManageInput（增强）

**变更**（复核 #10）：`type="number"` 且输入框被清空时 emit `undefined`（当前 emit '' 造成隐性类型漂移）；`modelValue` 类型放宽为 `string | number | undefined`。数字字段提交前由页面 `Number()` 收口（CronListView.vue:177 现状已是此写法，天然兼容）。

**验收标准**（`ManageInput.spec.ts` 追加）：type=number 清空 → `emitted('update:modelValue')` 含 `undefined`；输入 '3' → `[3]`；type=text 清空 → `['']`（行为不变）；回归既有 9 条。

### 2.12 ManageTextarea（增强）

**变更**（复核 #11）：补 `maxlength?: number | string`、`disabled?: boolean` props，透传原生属性；使 RegisterApplyListView.vue:227-234 的手写 textarea 可替换。

**验收标准**：maxlength=255 → textarea 属性 maxlength="255"；disabled → 属性与置灰类；既有 5 条回归。

### 2.13 ManageImageUpload（增强）

**变更**（复核 #5/#11）：

1. `onDrop` 校验 `file.type.startsWith('image/')`：非图片不进入 upload，emit `reject` 事件（payload 为该 File），页面监听后自行 toast（组件不直接 toast）。
2. 预览 `<img>`（:81-86）换 `BaseImage`（获得裂图兜底）；不加 lightbox（见 exclusions）。
3. emits 增加 `(e: 'reject', file: File): void`。

**验收标准**（`ManageImageUpload.spec.ts` 新建，mock `@/api`）：

1. drop `File({ type: 'text/plain' })` → 无上传请求、`emitted('reject')` 长度 1。
2. drop `File({ type: 'image/png' })` → manageApi.file.upload 被调、成功后 emit update:modelValue(URL)。
3. upload reject → 组件内错误文案渲染（现 error ref 行为）。
4. modelValue 有值 → BaseImage 渲染；「清除」按钮 emit update:modelValue('')。
5. ratio prop → 预览容器 style aspectRatio。

### 2.14 ManageCascader（修复）

**变更**（复核 #14）：`isPartialChecked`（ManageCascader.vue:38-42）从「仅检查直接子级」改为递归检查全部子孙（复用 :25 descendantIds）：`node.children?.length && descendantIds(node).some(id => id !== node.id && selected.has(id)) && !isChecked(node)`。

**验收标准**（`ManageCascader.spec.ts` 新建）：三层树仅第 3 层叶子选中 → 顶层 checkbox `indeterminate=true`；子级全选 → 顶层 checked；无选中 → 均否；toggle 子树全选/全取消（既有语义回归）；MAX_DEPTH 渲染深度不变。

### 2.15 ManageSwitch（修复）

**变更**（复核 #9）：`.jc-switch__hit` 删除（pointer-events:none 的伪触控区），改按钮 `::after { content:''; position:absolute; inset:-8px }` 扩展命中区至 60x44，实际可点区不再小于声明的 44px 标准。纯 CSS，无 DOM/props 变化。

**验收标准**：既有 6 条 spec 回归通过（jsdom 不测命中区像素，人工验收 + e2e 兜底）；DOM 结构不变。

### 2.16 utils/format（增强）

**变更**（复核 #12）：`web/src/utils/format.ts` 追加：

```ts
/** 短格式：MM-DD HH:mm（CronListView.fmtRunTime / Dashboard 场景） */
export function formatShortDateTime(ts: number | string | Date): string
```

规则与 formatDateTime 一致（无效输入返回 '-'）。秒级时间戳由调用处 `* 1000`（约定写入 §3.7，不在 utils 里做自动嗅探——1000 以下 ms 值与秒值歧义不可靠）。

**验收标准**（`format.spec.ts` 追加）：`formatShortDateTime(new Date('2026-01-02T03:04:00'))` → `'01-02 03:04'`；字符串入参 `'2026-01-02T03:04:05'` 走 formatDateTime → `'2026-01-02 03:04'`；`formatDateTime('not-a-date')` → `'-'`。

### 2.17 ManageLayout（布局壳增强）

**变更**（决策 D1/D5，`web/src/components/layout/ManageLayout.vue`）：

1. main（:54-58）增加高度链类：`flex flex-col min-h-0`（main 本身 flex-1 不变）。
2. main 内新增内容包裹层：`<div class="jc-manage-main mx-auto flex w-full min-h-0 flex-1 flex-col max-w-[var(--jc-content-max-w)]">`（居中 + 封顶 1440px + 高度链贯通给页面根）。
3. 主题令牌（theme.css，归同任务）：追加 `--jc-table-cell-py: 10px;`；`[data-mode='tv']` 下覆写 `12px`（TV 行高略增保遥控可读）。

**验收标准**（`ManageLayout.spec.ts` 追加）：渲染后 `.jc-manage-main` 存在且类含 `max-w-[var(--jc-content-max-w)]` 与 `min-h-0`；main 含 `min-h-0`；回归既有用例。

---

## 3. 全局约定（迁移组机械执行）

### 3.1 列表页骨架（usePagedList + 内置分页）

```ts
const params = reactive({ keyword: '', status: 'active' })   // 筛选参数留在页面
const paged = usePagedList<ManageFilmRow>((page, size) =>
  manageApi.film.searchList({ keyword: params.keyword, status: params.status, page, size })
    .then((r) => ({ list: r.list ?? [], total: r.page?.total ?? 0 }))
)
// 搜索：paged.search()；翻页：v-model:current 双向；重试：paged.load()
```

```html
<ManageTable
  :columns="columns" :rows="paged.rows.value" row-key="mid"
  :loading="paged.loading.value" :total="paged.total.value"
  v-model:current="paged.current.value" :page-size="paged.pageSize.value"
  empty="未找到影片"
>
```

（`<script setup>` 下可直接 `v-model:current="paged.current"`。）删除页面自维护的 rows/loading/total/pageSize/loadSeq/changePage/末页回夹代码。BasePagination 手动拼接一律移除。

### 3.2 表格高度接法

页面根元素必须是 flex 列且允许收缩：`<div class="flex min-h-0 flex-col gap-[var(--jc-space-4)]">`（ManageLayout 的 `.jc-manage-main` 已提供 flex 链，页面只需 `min-h-0`）。ManageTable 默认 `fit` + `min-height 320px`，普通页面**零配置**；确实不适合限高的页面（如 CollectListView 多面板页）传 `:fit="false"` 退出。

### 3.3 写操作口径（useAsyncAction + silent）

```ts
const { busy, run } = useAsyncAction()
async function removeFilm(row: ManageFilmRow): Promise<void> {
  await run({
    confirm: { title: `删除《${row.name}》？`, desc: '…', okText: '删除', danger: true },
    action: () => manageApi.film.softDelete(row.mid, { silent: true }),   // 必须 silent:true
    successMsg: '已移入回收站',
    after: () => paged.load(),
  })
}
```

规则：① 写请求一律 `{ silent: true }`；② 页面删除手写 try/finally busy 与 catch toast；③ 成功文案 = successMsg；④ 失败文案 = run 的 error toast（拦截器被 silent 关闭，不双弹）；⑤ 列表加载请求**不**传 silent（错误仍由拦截器统一提示，页面只落 loadError 文案）。

### 3.4 弹窗选型与函数化调用

- 确认型 → `confirm({...})`（useConfirm，现状保留）。
- 跨页复用或含独立校验/提交逻辑的表单 → `openManageDialog`：

```ts
// 内容组件（页内放 views/manage/<域>/dialogs/XxxFormDialog.vue，跨页共享放 components/manage/）
const emits = defineEmits<{ (e: 'confirm', payload: UserFormPayload): void; (e: 'cancel'): void }>()
// footer：取消 → emits('cancel')；提交校验通过 → emits('confirm', form)

// 调用方
const payload = await openManageDialog<UserFormPayload>({
  title: editing.value ? `编辑用户 — ${editing.value.userName}` : '新增用户',
  component: UserFormDialog,
  props: { initial: editing.value },
  mobileMode: 'sheet',        // ≤5 字段；>5 字段或富文本/多步 → 'fullsheet'
})
if (!payload) return          // null = 取消/关闭
await run({ action: () => manageApi.user.update(payload.id, ...), successMsg: '已保存', after: paged.load })
```

- 一次性内联小弹窗 → 组件式 `ManageSheet`（不废弃）。
- 纯展示（无表单字段）→ `BaseDialog`。
- **禁** `window.confirm` / `window.alert`（LiveSourceListView.vue:148/289、BannerListView.vue 7 处 alert 迁移时清除）。

### 3.5 ManageSelect 替换

13 处手写 `<select>` 一律换：

```html
<ManageSelect v-model="form.role" :options="[{ label: '普通用户', value: 0 }, { label: '管理员', value: 1 }]" aria-label="角色" />
```

value 保持原 option 的类型（`:value="0"` 数字 → options value 数字）。原 select 的 class 串、data-focusable 全部删除（组件内置）。TaskManageView.vue:526/536 两处缺 data-focusable 的 select 一并替换（自动修复 TV 聚焦）。

### 3.6 状态映射与页签

```html
<StatusTag :value="row.lastStatus" :map="RUN_STATUS_MAP" />   <!-- map 定义在页面顶部常量 -->
<ManageTabs v-model="status" :tabs="STATUS_TABS" />            <!-- 桌面/移动同构，替换手写按钮组 -->
```

map 常量就近放各页（status→文案映射属业务词汇表，不强制全局集中）；两页以上完全相同的 map（如 FilmListView/FailureListView 的 STATUS_TABS）提升到 `web/src/views/manage/constants.ts`。

### 3.7 时间格式化

删除各页私有 fmtTime/fmtRunTime（复核 #12 列出的 8+ 处），统一：

- `YYYY-MM-DD HH:mm` → `formatDateTime(ts)`（utils/format.ts 已有，接受 number(ms)/string/Date）。
- `MM-DD HH:mm` → `formatShortDateTime(ts)`（本规格新增）。
- 秒级时间戳调用处 `formatDateTime(sec * 1000)`。

### 3.8 移动端降级规则

- ManageTable fit/表头排序/筛选在 mobile card 模式自动失效（组件内处理，页面无感）；移动端等价筛选放 toolbar（ManageSelect），排序下拉现状保留。
- 弹窗：ManageSheet/openManageDialog 自动 modal→sheet/fullsheet；`mobileMode` 选择规则 = ≤5 字段 sheet、>5 字段/富文本/多步 fullsheet（ManageSheet.vue:8-10 既有建议升格为约定）。
- 触控密度令牌（44px 命中区、min-h-[44px] 输入控件）不得因桌面密度调整而缩水。

### 3.9 TV/遥控器

一切自定义可点元素必须带 `data-focusable="true"` + 键盘可达（Enter 触发）；ManageSelect/ManageTabs/ManageTable 表头按钮/筛选面板 option 内置。

### 3.10 共享文件归属（防冲突）

| 文件 | 唯一归属 |
|------|---------|
| `web/src/components/layout/ManageLayout.vue` | 表格线（T0 布局壳） |
| `web/src/assets/styles/theme.css`（新增密度令牌） | 表格线（T0） |
| `web/src/App.vue`（挂 ManageDialogHost） | 弹窗表单线（P3） |
| `web/src/components/base/BaseDialog.vue` / `BaseEmpty.vue` / `BaseLightbox.vue` | 弹窗表单线（P2 overlay + import 修复） |
| `web/src/api/http.ts` | **任何线都不改**（口径靠 silent 约定，见 exclusions） |
| 页面文件 | 迁移组按页唯一归属（M 系列，一页一任务） |

---

## 4. 验收与门禁

1. 组件级：§2 各「验收标准」逐条落 spec；`pnpm exec vitest run src/components/manage src/components/base src/composables/usePagedList.spec.ts src/composables/useManageDialog.spec.ts src/composables/useOverlayStack.spec.ts src/composables/useAsyncAction.spec.ts src/utils` 全绿。
2. 页面级：迁移组每页完成后跑该页 spec（CronListView.spec.ts 等既有页面 spec 必须继续通过；被替换的样板逻辑相关断言同步改写）。
3. 全量门禁（收尾）：`pnpm test`（vitest 全量）+ `pnpm run type-check`（vue-tsc）+ `pnpm exec playwright test manage`（e2e，VITE_USE_MOCK=1）。
4. 手工抽查（非单测可覆盖项）：桌面 1440px 与 2560px 下列表页内容居中封顶、一屏行数提升；窗口缩放表格区滚动且不低于 320px；移动端 375px 卡片流正常；TV 模式（?mode=tv）焦点环与 ESC 行为。
