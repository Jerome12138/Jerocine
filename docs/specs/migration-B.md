# 迁移 B 线迷你规格 —— 采集与任务域

> 域内页面: CollectListView(/manage/collect)、CollectJobsView(/manage/collect/jobs)、
> FailureListView(/manage/collect/failures)、CronListView(/manage/cron)、TaskManageView(/manage/tasks)。
> 每页一节: 改什么 / 验收标准 / 关键取舍。执行方式 SDD+TDD，逐页推进。

## 0. 前置事实与本线自主决策（关键取舍）

1. **共享层现状**: 提交 95fc5df 已落地 P1-P10 全部 + 表格线 T2/T4（ManageTable v2、BasePagination 钳制），
   但 T0（ManageLayout 密度壳）、T1（formatShortDateTime）、T3（usePagedList）、T5（ManageSelect）、
   T6（StatusTag）、T7（ManageTabs）缺失。工作树无他人未提交改动、无其他迁移文档（无并行迹象）。
2. **决策**: 本线先按规格 §2.3/2.4/2.8/2.9/2.16/2.17 以 TDD 补齐上述 6 项共享件（red→green→refactor），
   再迁移本域页面。理由: 域内 5 页的约定改造（usePagedList/ManageSelect/StatusTag/ManageTabs/短时间格式）
   机械依赖这些组件，缺了它们约定无法遵守；文件均为「新建」或单点小改，与已落地共享件零交集。
   若主流程后续另有 T 线产出，提交时由主流程合并。
3. **API 层小改**: `api/manage/cron.ts`、`api/manage/collect.ts` 的写函数补可选 `config?: AxiosRequestConfig`
   （与同文件 test/startSpider 等既有签名同构，纯增量、不影响既有调用方）。这两文件仅被本域页面消费
   （grep 验证），实为页面私有 API。用途: 写操作 `silent:true` + useAsyncAction 单点提示，消双重 toast。
4. **弹窗选型核对结论**: 本域无跨页复用表单。CollectListView 源站表单/重置库存、CronListView 任务表单
   均为「一次性内联小弹窗」→ 维持组件式 ManageSheet（约定明示「不废弃」），不引入 openManageDialog；
   确认型交互一律 confirm()。TaskManageView/FailureListView/CollectJobsView 的表单类弹窗不存在。
5. **时间格式化口径**: TaskManageView 既有 fmtTime 输出含秒（YYYY-MM-DD HH:mm:ss），utils 单一来源只有
   YYYY-MM-DD HH:mm 与 MM-DD HH:mm 两种 —— 统一降为 formatDateTime（去秒），痕迹: 台账时间列。
6. **CronListView 写操作**维持「拦截器统一提示」口径（现状无双重 toast，M3 机械清单未含 useAsyncAction），
   仅 CollectListView 的 toggleState/remove/startSpider（现状手写 catch toast → 双重 toast）换 useAsyncAction。

## 1. Phase 0 共享件补齐（TDD，先于各页）

- **usePagedList**（§2.3）: 验收 1-9 全绿；current watch 唯一触发器、loadSeq 守卫、末页回夹、CanceledError 静默。
- **ManageSelect**（§2.4）: 验收 1-7；数字 option 原类型 emit、placeholder disabled option、data-focusable 恒定。
- **StatusTag**（§2.8）: String(value) 查 map、null/undefined→fallback、size 透传 BaseTag。
- **ManageTabs**（§2.9）: pill（BaseButton gradient/ghost）与 underline 两形态、role=tablist/tab、count 徽标。
- **formatShortDateTime**（§2.16）: `01-02 03:04`；无效输入 '-'。
- **ManageLayout**（§2.17）: main 加 `flex flex-col min-h-0`，内嵌 `.jc-manage-main`
  （mx-auto w-full min-h-0 flex-1 flex-col max-w-[var(--jc-content-max-w)]）；
  theme.css 追加 `--jc-table-cell-py:10px` + `[data-mode='tv']` 12px。ManageLayout.spec 追加断言。

## 2. M7 CollectListView（/manage/collect）

- 改: 页面根包 `flex min-h-0 flex-col`；4 处手写 select（spiderHours/resultModel/grade/collectType）→
  ManageSelect（value 保数字类型）；`derivedCol + isColKey + as` 断言 hack 删，改派生列
  `{ key:'collected'|'health' }` + `render()`（Column.key 已放宽）；`state` 列改 StatusTag
  （BOOL_STATE_MAP: true→启用/success、false→停用/default）；toggleState/remove/startSpider 换
  useAsyncAction（写请求 silent:true，删手写 catch toast）；表格默认 fit + minHeight。
- 验收: 既有 spec 全绿（新增 ManageSelect/StatusTag 断言）；测速徽标等富渲染列经 render 输出不变；
  删除/启停失败只弹一条错误 toast。
- 取舍: 健康徽标（label 动态拼健康数据 + tooltip）非「状态值→文案」词汇表，StatusTag 不适用，保留计算属性；
  保存表单仍 ManageSheet（见 §0.4）。

## 3. CollectJobsView（/manage/collect/jobs）

- 改: stateVariant/stateText → StatusTag + JOB_STATE_MAP；pause/resume/cancelJob 换 useAsyncAction
  （silent:true + successMsg，busy 全局防重与现状 `:disabled="!!busyId"` 等价）；卡片按钮 data-focusable 补齐。
- 验收: 既有 spec 全绿；状态标签经 StatusTag 渲染文案不变；取消流程 confirm 短路不发请求。
- 取舍: 非表格页（卡片轮询流），不接 usePagedList/ManageTable；轮询结构不动。

## 4. M5 FailureListView（/manage/collect/failures）

- 改: rows/loading/total/page/loadSeq/changePage/末页回夹删 → usePagedList（status 闭包捕获）；
  STATUS_TABS 按钮组 → ManageTabs（pill，count 徽标带 pendingCount）；BasePagination 拼接删 →
  ManageTable 内置分页（total/current/page-size）；fmtTime → formatShortDateTime？否——本页 fmtTime
  输出 YYYY-MM-DD HH:mm → formatDateTime；status 列 → StatusTag（FAIL_STATUS_MAP）；
  recover/clearHandled 换 useAsyncAction；页面根补 min-h-0。
- 验收: 既有 7 条 spec 全绿（tab 切换 search() 回第 1 页、快速切 tab 序号守卫语义由 usePagedList 承接）；
  新增断言: ManageTabs aria-selected、内置分页 footer 渲染。
- 取舍: tab 切换 = paged.search()（回第 1 页），与现状 page=1+load 等价；pendingCount 探测请求独立保留。

## 5. M3 CronListView（/manage/cron）

- 改: 任务类型 select → ManageSelect；state 列 → StatusTag（BOOL_STATE_MAP 复用 §2 常量本页自定义）、
  model 列 → StatusTag（MODEL_MAP）、lastStatus → StatusTag（RUN_STATUS_MAP）；
  fmtRunTime → formatShortDateTime；表格挂默认 fit；页面根 `flex min-h-0 flex-col`；
  form.time 适配 ManageInput number 空值（提交处已有 `Number(form.time)` 收口，校验文案不变）。
- 验收: 既有 spec 全绿（含「错误交全局拦截器」口径不变、表单校验文案不变、time 清空/0/负数/小数拒绝）。
- 取舍: 写操作维持拦截器提示（§0.6）；客户端关键字过滤保留 computed（服务端无分页）。

## 6. M8 TaskManageView（/manage/tasks）

- 改: 下划线页签 → ManageTabs variant="underline"（label+count 拆开传）；:590/:600 两处 select →
  ManageSelect（修 data-focusable 缺口）；jobState/runStatus → StatusTag（JOB_STATE_MAP/RUN_STATUS_MAP）；
  历史台账与失败台账两表 → usePagedList + ManageTable 内置分页（删 2 处 BasePagination 拼接）；
  fmtTime → formatDateTime（去秒，§0.5）；`(row as unknown as Record<string,unknown>)` 兜底删
  （列全覆写后 v-else 兜底分支为死代码）；运行中卡片操作 pause/resume/cancel/rerun/recover 换 useAsyncAction。
- 验收: 既有 7 条 spec 全绿（筛选变更重置页码 = search()、翻页保留筛选 = v-model:current、
  重跑行级防连点文案「提交中…」保留）；轮询节奏 spec 不回归。
- 取舍: 失败台账 fetch 闭包固定 status=0（本页只看待补采）；elapsedMs 派生在 fetch 映射里完成。

## 7. 收尾门禁（本线自跑）

- `pnpm exec vitest run src/views/manage/collect src/views/manage/cron src/views/manage/tasks
  src/composables/usePagedList.spec.ts src/components/manage/ManageSelect.spec.ts
  src/components/manage/StatusTag.spec.ts src/components/manage/ManageTabs.spec.ts
  src/utils/format.spec.ts src/components/layout/ManageLayout.spec.ts`
- `pnpm run type-check`

## 8. 执行记录（2026-11-03 收尾时点）

| 项 | 结果 |
|----|------|
| Phase 0 共享件 | usePagedList 11/11、ManageSelect+StatusTag+ManageTabs 23/23、format 33/33、ManageLayout 14/14 全绿 |
| M7 CollectListView | 17/17 绿（新增 fit/render/ManageSelect/silent 断言; change/remove 走 useAsyncAction 单一错误 toast） |
| CollectJobsView | 10/10 绿（StatusTag+useAsyncAction; 新增 StatusTag variant 断言） |
| M5 FailureListView | 8/8 绿（usePagedList+内置分页+ManageTabs; 探测先行用 immediate:false 保持 mock 消费序） |
| M3 CronListView | 17/17 绿（ManageSelect+StatusTag×3+formatShortDateTime; 写操作维持拦截器口径, 见 §0.6） |
| M8 TaskManageView | 9/9 绿（ManageTabs underline+ManageSelect×2+StatusTag×2+usePagedList×2+内置分页; as-hack 兜底与 .jc-select 死样式删除） |
| 全仓回归 | `pnpm test` 121 文件 / 1235 用例全绿（Phase 0 动了共享 format/ManageLayout, 加跑全量防跨域回归） |
| type-check | `pnpm run type-check` 干净（ManageTabs 泛型经 defineProps 内联对象类型引用——独立 interface 引用 SFC 泛型参数 vue-tsc 解析不到, 已留注释） |
| e2e | 未跑: playwright 配置要求本地后端 127.0.0.1:3601(mock 适配器已移除, 探活 curl 连接拒绝), 属 G1/主流程门禁环境 |

补充取舍: 动态成功文案（补采「待处理 N 条」）无法走 useAsyncAction 静态 successMsg,
由 action 内自管 success toast、run 不传 successMsg —— toast 总数仍为 1, 不违背单一提示来源。
历史台账「重跑」按钮 disabled 由行级 rerunBusyId 与页级 busy 双条件驱动（保留行级文案「提交中…」）。
