# 管理后台 UI 组件化 TDD 实施计划

> 配套规格：`docs/specs/manage-ui-spec.md`（组件 API/行为/验收标准以规格为准，本文只排任务）。
> 方法论：每任务 = **先写失败单测(red) → 最小实现(green) → 重构(refactor)**；red 阶段先提交测试并跑出失败，再写实现到绿。
> 所有命令在 `web/` 下执行；单测风格沿用 `*.spec.ts`（vitest + @vue/test-utils + `vi.mock('@/api')`）。

## 0. 执行总则

1. **两条工匠线**：
   - **表格线（T 系列）**：布局壳、列表状态、表格、分页、select、状态标签、页签、时间格式化。
   - **弹窗表单线（P 系列）**：弹层栈、弹窗容器、函数化弹窗、动作包装、输入类组件、上传/级联。
2. **任务间零文件冲突**：两条线触碰文件集完全不相交（见 §4 冲突矩阵）；同一线内任务严格按编号串行（存在依赖：P2 依赖 P1 的 useOverlayStack，P3 依赖 P2 的 ManageSheet.width）。两线可并行推进。
3. **迁移组（M 系列）**：共享层（T+P）全部完成后启动；一页一任务，页面文件唯一归属；按 §规格 3 的约定机械迁移；每任务必须同步改写/新增该页 `*.spec.ts` 并保持通过。
4. 每任务完成的硬性检查（该任务自己跑）：新增/改写的 spec 通过 + `pnpm exec vitest run <相关文件>` 全绿 + `pnpm run type-check` 干净。全量门禁在 G1。
5. 本计划不含 commit/push/deploy（由主流程统一提交）。

## 1. 表格线任务

### T0 布局壳：内容最大宽度 + 高度链 + 密度令牌
- 归属：表格线
- 触碰文件：`web/src/components/layout/ManageLayout.vue`、`web/src/assets/styles/theme.css`、`web/src/components/layout/ManageLayout.spec.ts`
- red：ManageLayout.spec.ts 追加失败断言——渲染后存在 `.jc-manage-main` 且类含 `max-w-[var(--jc-content-max-w)]`、`min-h-0`；main 元素含 `min-h-0` 类。
- green：按规格 §2.17 改 main（`flex flex-col min-h-0`）+ 新增 `.jc-manage-main` 包裹层（居中/封顶/高度链）；theme.css 追加 `--jc-table-cell-py: 10px;` 与 `[data-mode='tv']` 覆写 `12px`。
- refactor：无（纯结构改动）；跑既有 ManageLayout.spec.ts 全部用例确认无回归。

### T1 时间格式化：formatShortDateTime
- 归属：表格线
- 触碰文件：`web/src/utils/format.ts`、`web/src/utils/format.spec.ts`（无则新建）
- red：spec 断言 `formatShortDateTime(new Date('2026-01-02T03:04:00')) === '01-02 03:04'`、字符串入参走 formatDateTime、无效输入 `'-'`（规格 §2.16）。
- green：实现 formatShortDateTime（复用 pad）。
- refactor：formatDateTime 与 formatShortDateTime 抽公共 `formatToParts` 消重复。

### T2 BasePagination 越界钳制
- 归属：表格线
- 触碰文件：`web/src/components/base/BasePagination.vue`、`web/src/components/base/BasePagination.spec.ts`
- red：追加 3 条失败用例（规格 §2.2 验收 1-3：越界 emit update:current 且不 emit change；正常范围不 emit）。
- green：`watch(() => totalPages.value, ...)` 内 `props.current > totalPages` → `emit('update:current', totalPages.value)`。
- refactor：确认公共区 3 个 `@change` 使用方语义不变（该步为人工核对 ClassifySearchView/HotBoardView/SearchView 绑定方式，不改文件）。

### T3 usePagedList 分页状态收敛
- 归属：表格线
- 触碰文件：`web/src/composables/usePagedList.ts`（新建）、`web/src/composables/usePagedList.spec.ts`（新建）
- red：按规格 §2.3 验收 1-9 写 9 条失败用例（vi.fn fetch + 手控 Promise resolve 顺序）。
- green：实现 immediate load / current watch 触发 / loadSeq 守卫 / 末页回夹 / loadError / CanceledError 静默 / search 语义。
- refactor：回夹与序号守卫抽内部函数；对齐 UserListView.vue:45-73 与 FilmListView.vue:120-154 的现行为作为行为基准（迁移时逐页删除同逻辑）。

### T4 ManageTable v2（内置分页 + fit 高度 + 紧凑密度 + Column 放宽 + 排序/筛选）
- 归属：表格线（ManageTable.vue 全部改造集中于本任务，避免文件内多任务交叉）
- 触碰文件：`web/src/components/manage/ManageTable.vue`、`web/src/components/manage/ManageTable.spec.ts`
- red：按规格 §2.1 验收 1-16 分四组先写失败用例：
  - 4a 分页（验收 4-7）：total/current 渲染 footer、page-change/update:current 双 emit、total=0|undefined 不渲染。
  - 4b fit+密度（验收 1-3 + 16）：jc-table--fit / minHeight 内联 style / mobile 忽略 / 紧凑 padding 类。
  - 4c Column 放宽（验收 13-15）：render 优先、派生列 + #cell、派生列无渲染 → `—`。
  - 4d 排序筛选（验收 8-12）：sorter 客户端排序、sortable 受控 emit、filter 单选/多选/清除/ESC 关面板。
- green：按 4a→4b→4c→4d 顺序分小步实现到绿（每小步 red 都跑出失败再实现）；分页条复用 BasePagination；筛选面板用内联 absolute div + onClickOutside（@vueuse 已自动导入）。
- refactor：默认文本渲染统一走 `row[col.field ?? col.key]`；手机 card 分支与桌面分支共享列取值 helper；跑全文件用例回归。

### T5 ManageSelect
- 归属：表格线
- 触碰文件：`web/src/components/manage/ManageSelect.vue`（新建）、`web/src/components/manage/ManageSelect.spec.ts`（新建）
- red：规格 §2.4 验收 1-7 七条失败用例。
- green：实现（class 对齐 ManageInput、`data-focusable="true"` 恒定、placeholder disabled option、原类型 emit）。
- refactor：无。

### T6 StatusTag
- 归属：表格线
- 触碰文件：`web/src/components/manage/StatusTag.vue`（新建）、`web/src/components/manage/StatusTag.spec.ts`（新建）
- red：命中/未命中 fallback/布尔 'true' 键/自定义 fallback/size 透传五条失败用例。
- green：实现（内部渲染 BaseTag）。
- refactor：无。

### T7 ManageTabs
- 归属：表格线
- 触碰文件：`web/src/components/manage/ManageTabs.vue`（新建）、`web/src/components/manage/ManageTabs.spec.ts`（新建）
- red：渲染数量/点击 emit/aria-selected/underline 类/count 徽标五条失败用例（规格 §2.9）。
- green：实现 pill（BaseButton 组）与 underline 两形态。
- refactor：pill 态复用 BaseButton variant 逻辑，避免复制样式串。

## 2. 弹窗表单线任务

### P1 useOverlayStack + 三弹层接入 + import 修复
- 归属：弹窗表单线
- 触碰文件：`web/src/composables/useOverlayStack.ts`（新建）、`web/src/composables/useOverlayStack.spec.ts`（新建）、`web/src/components/base/BaseDialog.vue`、`web/src/components/base/BaseDialog.spec.ts`、`web/src/components/base/BaseLightbox.vue`、`web/src/components/base/BaseLightbox.spec.ts`（新建）、`web/src/components/base/BaseEmpty.vue`
- red：① 栈本身 4 条用例（规格 §2.6 验收 1-3 + 乱序释放）；② BaseDialog.spec 追加「双弹层按 ESC 只关顶层」失败用例；③ BaseLightbox 首个 spec（渲染/关闭事件）。
- green：实现 acquire/release/isTop；BaseDialog watch 与 onBeforeUnmount 改走栈（替换直接 setModalFlag/overflow）；keydown 改 isTopOverlay 判定并去掉 stopPropagation；BaseLightbox/ManageSheet 同规则（ManageSheet 部分留给 P2，避免本任务碰 ManageSheet.vue）；顺手补 BaseDialog/BaseEmpty 显式 `import BaseIcon`（复核 #16）。
- refactor：BaseConfirmDialog 不动（其经 BaseDialog 间接获得栈能力）；跑 base 目录既有 spec 回归。

### P2 ManageSheet 增强
- 归属：弹窗表单线（依赖 P1）
- 触碰文件：`web/src/components/manage/ManageSheet.vue`、`web/src/components/manage/ManageSheet.spec.ts`
- red：规格 §2.10 验收 1-4（挂载即开滚动锁生效 / width 透传 / 无 title 有关闭按钮 / ESC 回归）。
- green：watch 加 `{ immediate: true }`；加 `width` prop（modal 容器 maxWidth）；header 恒渲染；grabber 注释修正；ESC 接入 useOverlayStack。
- refactor：modal/sheet/fullsheet 三分支的滚动锁/标志操作统一走栈 helper。

### P3 useManageDialog + ManageDialogHost + 挂载
- 归属：弹窗表单线（依赖 P2 的 width/header 行为）
- 触碰文件：`web/src/composables/useManageDialog.ts`（新建）、`web/src/composables/useManageDialog.spec.ts`（新建）、`web/src/components/manage/ManageDialogHost.vue`（新建）、`web/src/App.vue`
- red：规格 §2.5 验收 1-9 九条失败用例（mount Host + 页面组件桩）。
- green：实现单例 state / openManageDialog（旧待决 resolve(null) 先例 useConfirm.ts:48-51）/ answerManageDialog / Host 渲染 ManageSheet+动态组件 / hostMounted 标志 / 卸载兜底；App.vue 在 BaseConfirmDialog 同位（约 :130）挂 `<ManageDialogHost />`。
- refactor：Host 的 confirm/cancel 转发抽成显式 handler，模板零内联逻辑。

### P4 useAsyncAction
- 归属：弹窗表单线
- 触碰文件：`web/src/composables/useAsyncAction.ts`（新建）、`web/src/composables/useAsyncAction.spec.ts`（新建）
- red：规格 §2.7 验收 1-6 六条失败用例（vi.mock('@/api/http') 的 toast；useConfirm 真单例 answerConfirm 驱动）。
- green：实现 busy 并发拦截 / confirm 短路 / success+error toast 单点 / CanceledError 静默 / after。
- refactor：错误文案提取 `(e as Error).message ?? '操作失败'` 为内部 helper。

### P5 ManageInput number 空值约定
- 归属：弹窗表单线
- 触碰文件：`web/src/components/manage/ManageInput.vue`、`web/src/components/manage/ManageInput.spec.ts`
- red：规格 §2.11 三条失败用例（number 清空 → undefined；'3' → 3；text 清空仍 ''）。
- green：`onInput` number 分支空串 emit `undefined`；modelValue 类型放宽 `string | number | undefined`，`:value` 绑定 `props.modelValue ?? ''`。
- refactor：无（既有 9 条用例回归）。

### P6 ManageTextarea 补 props
- 归属：弹窗表单线
- 触碰文件：`web/src/components/manage/ManageTextarea.vue`、`web/src/components/manage/ManageTextarea.spec.ts`
- red：maxlength/disabled 两条失败用例（规格 §2.12）。
- green：补两个可选 prop 并透传。
- refactor：无。

### P7 ManageImageUpload 拖入校验 + BaseImage 预览
- 归属：弹窗表单线
- 触碰文件：`web/src/components/manage/ManageImageUpload.vue`、`web/src/components/manage/ManageImageUpload.spec.ts`（新建）
- red：规格 §2.13 验收 1-5 五条失败用例（mock `@/api` 的 file.upload）。
- green：onDrop 加 `image/` 前缀校验 + emit('reject', file)；预览换 BaseImage。
- refactor：upload() 与 onSelect/onDrop 解耦为 `uploadFile(file: File | undefined)`。

### P8 ManageCascader 半选递归
- 归属：弹窗表单线
- 触碰文件：`web/src/components/manage/ManageCascader.vue`、`web/src/components/manage/ManageCascader.spec.ts`（新建）
- red：三层树仅叶子选中 → 顶层 indeterminate（当前实现失败）；全选/无选/子树 toggle 回归用例（规格 §2.14）。
- green：`isPartialChecked` 复用 `descendantIds` 递归判断。
- refactor：无。

### P9 ManageSwitch 命中区
- 归属：弹窗表单线
- 触碰文件：`web/src/components/manage/ManageSwitch.vue`
- red：无新增可断言用例（纯 CSS）；任务 red 阶段 = 跑既有 6 条 spec 确认基线绿，人工核对实现与注释一致（规格 §2.15）。
- green：删 `.jc-switch__hit`，改 `::after { position:absolute; inset:-8px; content:'' }`。
- refactor：注释更新为真实命中区尺寸（60×44）。

### P10 SourceCandidateDialog 测试补齐
- 归属：弹窗表单线
- 触碰文件：`web/src/components/manage/SourceCandidateDialog.spec.ts`（新建）
- red→green 顺序反转说明：本任务为**补测试**（行为已存在），先写用例跑绿即为验收；发现缺陷则修组件（同任务内允许）。
- 覆盖：打开渲染候选列表 / 选择 emit / loading 态 / 空列表。（组件本体改造归 M12，届时 spec 随 M12 扩展。）

## 3. 迁移组任务（共享层完成后，一页一任务）

> 每任务固定三段：① 按规格 §3 约定替换样板；② 改写/新增该页 spec 并跑绿；③ `pnpm run type-check`。涉及既有 spec 的文件一并在该任务内修改（页面文件与其 spec 同任务唯一归属）。

| 任务 | 触碰文件 | 机械动作清单 |
|------|---------|-------------|
| M1 | `views/manage/user/UserListView.vue`、`views/manage/user/UserListView.spec.ts`、`views/manage/user/dialogs/UserFormDialog.vue`（新建） | usePagedList 重写 load；ManageTable 内置分页删 BasePagination；表单/重置密码两弹窗抽 UserFormDialog + openManageDialog（§3.4）；toggleDisabled/removeUser/submit* 换 useAsyncAction（§3.3）；角色 select 换 ManageSelect；fmtTime 删 → formatDateTime；角色/状态列换 StatusTag |
| M2 | `views/manage/film/FilmListView.vue`、其 spec | usePagedList；内置分页；删除/恢复换 useAsyncAction；STATUS_TABS 换 ManageTabs；deletedAt 伪 key 列改派生列（`key:'status'` + render，§2.1）；fmtTime 删；高度接 §3.2（页面根 min-h-0） |
| M3 | `views/manage/cron/CronListView.vue`、其 spec | 任务类型 select 换 ManageSelect；state/lastStatus 换 StatusTag（含 lastRunVariant/Text 删除）；fmtRunTime → formatShortDateTime；`form.time` 适配 number 空值（提交处 `Number()` 收口已有）；表格挂 fit |
| M4 | `views/manage/user/RegisterApplyListView.vue`、其 spec | usePagedList；内置分页；手写 textarea 换 ManageTextarea（maxlength=255）；statusVariant/Name 换 StatusTag；fmtTime 删 |
| M5 | `views/manage/collect/FailureListView.vue`、其 spec | usePagedList；内置分页；STATUS_TABS 换 ManageTabs；fmtTime → formatShortDateTime |
| M6 | `views/manage/file/FileGalleryView.vue`、其 spec | usePagedList；内置分页（§3.1 骨架） |
| M7 | `views/manage/collect/CollectListView.vue`、其 spec | 4 处 select 换 ManageSelect（:480/:650/:660/:670）；derivedCol+as 断言 hack 改派生列 + render（规格 §2.1）；badges 换 StatusTag |
| M8 | `views/manage/tasks/TaskManageView.vue`、其 spec | jobState/runStatus 换 StatusTag；下划线页签换 ManageTabs variant="underline"；:526/:536 两处 select 换 ManageSelect（顺带修 data-focusable）；fmtRunTime → formatShortDateTime；`(row as unknown as Record<string,unknown>)` 兜底删 |
| M9 | `views/manage/live/LiveSourceListView.vue`、其 spec | window.confirm(:148/:289) → confirm()；BaseDialog 表单弹窗(:13) → ManageSheet；BasePagination 拼接 → 内置分页 |
| M10 | `views/manage/banner/BannerListView.vue`、其 spec | 7 处 window.alert → useAsyncAction 的 successMsg / toast；写操作补 silent:true（§3.3） |
| M11 | `views/manage/telemetry/TelemetryView.vue`、其 spec | :302 表单弹窗 BaseDialog → ManageSheet；:200/:239 select 换 ManageSelect |
| M12 | `views/manage/film/FilmAddView.vue`、`views/manage/film/FilmDetailView.vue`、`components/manage/SourceCandidateDialog.vue`、`components/manage/SourceCandidateDialog.spec.ts`、`composables/useCollectFilm.ts`（新建） | 海报字段改 ManageImageUpload（删手写 pickFile/handleUpload :224-246 与模板 430-456）；collectFilm 流程（FilmDetailView:183-209 / FilmAddView:140-194）下沉 useCollectFilm，SourceCandidateDialog 加 `collect` prop 内部处理 loading/toast/关闭；P10 的 spec 随契约扩展 |
| M13 | `views/manage/system/MailConfigCard.vue`、其 spec | :158/:183/:206 三处 select 换 ManageSelect |
| M14 | `views/manage/system/SiteConfigView.vue`、`views/manage/system/DanmuConfigCard.vue`、各自 spec | runtime.* 数字字段适配 ManageInput number 空值（提交前 Number 收口）；select 换 ManageSelect |
| M15 | `views/manage/DashboardView.vue`、其 spec | :133-145 手写分页钳制 watch 删除（usePagedList 接管）；fmtTime → formatShortDateTime |
| M16 | `views/manage/system/SessionListView.vue` | formatDateTime 已在用，核对无重复实现即可；表格挂 fit；如无改动则任务记「核对通过」 |

### G1 全量门禁（收尾，主流程执行）
- `pnpm test`（vitest 全量）
- `pnpm run type-check`
- `pnpm exec playwright test manage`（VITE_USE_MOCK=1）
- 手工抽查：2560px/1440px 桌面列表页居中封顶与行密度；窗口缩放表格滚动且 ≥320px；375px 移动卡片流；`?mode=tv` ESC 与焦点环。
- 通过后主流程统一 commit（本计划不含提交动作）。

## 4. 冲突矩阵（两条线触碰文件集不相交的证明）

| 线 | 文件集 |
|----|--------|
| 表格线 T0-T7 | ManageLayout.vue、theme.css、format.ts(+spec)、BasePagination.vue(+spec)、usePagedList.ts(+spec)、ManageTable.vue(+spec)、ManageSelect.vue(+spec)、StatusTag.vue(+spec)、ManageTabs.vue(+spec) |
| 弹窗线 P1-P10 | useOverlayStack.ts(+spec)、BaseDialog.vue(+spec)、BaseLightbox.vue(+spec)、BaseEmpty.vue、ManageSheet.vue(+spec)、useManageDialog.ts(+spec)、ManageDialogHost.vue、App.vue、useAsyncAction.ts(+spec)、ManageInput.vue(+spec)、ManageTextarea.vue(+spec)、ManageImageUpload.vue(+spec)、ManageCascader.vue(+spec)、ManageSwitch.vue、SourceCandidateDialog.spec.ts |
| 迁移组 M1-M16 | 每任务一个页面文件 + 该页 spec + 该任务新建的页内组件/composable（M12 的 SourceCandidateDialog.vue 在 P10 仅补 spec 不改本体，时序 P→M 无并行） |

交集检查：theme.css 仅 T0；App.vue 仅 P3；ManageSheet.vue 仅 P2；BaseDialog.vue 仅 P1；ManageTable.vue 仅 T4；ManageLayout.vue 仅 T0——**两条组件线零交集** ✓。
