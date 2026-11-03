# 管理端迁移迷你规格 —— 影片域(/manage/film)

> 逐页推进:SDD 迷你规格 → spec 先红(red) → 迁移实现转绿(green) → 清理。
> 每页规格在该页动工前追加;关键取舍记录于各节「取舍」。

## 0. 前置勘察与共享件补建(2026-11-03)

共享层(表格线 T0-T7 / 弹窗线 P1-P10)落库于 95fc5df:ManageTable 增强版(total/current/fit/minHeight/sortState/filter)、BasePagination 越界钳制、useAsyncAction、useManageDialog + ManageDialogHost(App.vue:483 已挂载)、ManageSheet.width、ManageInput number 空值、ManageImageUpload.reject 均已就绪,基线 48 spec + type-check 全绿。

**缺口**:约定强制引用但尚未存在的文件 —— `usePagedList.ts`、`ManageSelect.vue`、`StatusTag.vue`、`ManageTabs.vue`、`formatShortDateTime`、`views/manage/constants.ts`。六者均为**新增文件**,不落在共享文件归属清单(ManageLayout/theme.css/App.vue/BaseDialog|BaseEmpty|BaseLightbox/http.ts)内,由本域按组件方案 JSON 的 API 原文补建并带 spec,不修改任何他线归属文件。

**ManageLayout `.jc-manage-main`(T0 线)未落地**:页面根按约定写 `<div class="flex min-h-0 flex-col gap-[var(--jc-space-4)]">`,不写页面级 calc/ResizeObserver;fit 高度链待 T0 落地自动贯通,过渡期 ManageTable fit 的 minHeight 320px 兜底,无回归。

**取舍**:不重构 SourceCandidateDialog 的 v-model:visible+@pick 契约为 confirm/cancel —— 它是 95fc5df 共享层产出的 BaseDialog 纯展示容器、详情/新增两页稳定共用,改契约会碰共享文件且违背 (d);影片域内函数化弹窗落在 FilmClassView(见 §2)。

---

## 1. FilmListView(/manage/film)

**改什么**:列表骨架换 usePagedList(删页面自维护 rows/loading/total/pageSize/loadSeq/changePage/末页回夹,fetch 闭包携带 keyword/catIds/status/sort);ManageTable 内置分页(:total + v-model:current + :page-size),删页面级 BasePagination;STATUS_TABS 按钮组换 ManageTabs(pill),提升到 views/manage/constants.ts(FILM_STATUS_TABS);排序手写 select 换 ManageSelect(aria-label=排序方式);状态列换 StatusTag(:map=FILM_STATUS_TAG_MAP,active=在架/success、deleted=回收站/danger);删除/恢复换 useAsyncAction(confirm+silent+successMsg+after=paged.load);deletedAt title 换 formatDateTime;页面根 `flex min-h-0 flex-col gap-*`。

**验收**:
1. 分页 nav 渲染于 ManageTable 内部(页面顶层无独立 BasePagination 组件);点击第 2 页 → searchList 收到 page=2;末页删空回夹、竞态守卫、失败重试行为不回归。
2. 状态列 findComponent(StatusTag) 且 map/value 正确;页签容器 role=tablist + aria-selected;排序 select[aria-label=排序方式] 仍可切换且重置页码。
3. 删除/恢复:confirm → softDelete/restore(silent) → 定制 successMsg → 重载;busy 防重不回归;既有 15 条断言语义全部保留。

**取舍**:ManageTabs/ManageSelect 用 `:model-value` + `@update:model-value` 显式处理(保留「同值不触发」守卫与切页签/切排序必回第 1 页语义),不用裸 v-model + watch,避免双触发歧义。

---

## 2. FilmClassView(/manage/film/class)

**改什么**:①编辑/新增弹窗函数化 —— 域内新建内容组件 `FilmClassFormDialog.vue`(契约 emits confirm(payload)|cancel,footer 自带),页面经 `openManageDialog` 发起(标题逻辑留页面),弹窗内自治「校验+提交」:空名拦截 toast、提交失败弹窗停留,成功才 emit confirm → 页面重拉树;②三个开关与删除换 useAsyncAction(classUpdate/classDel 传 silent:true,失败 toast 由 run 单点);③ManageInput number 清空 emit undefined,提交前 Number() 收口(既有行为保留);④API 层 classUpdate/classDel 增加可选 config 形参(影片域自有 API 模块,向后兼容)。

**验收**:
1. 弹窗经 ManageDialogHost 渲染(findComponent(FilmClassFormDialog) 可命中,initial 回填);classUpdate 载荷带 (payload, { silent: true });保存成功关弹窗+重拉,失败弹窗停留。
2. 开关乐观更新+失败回滚不回归;classDel(id, { silent: true });既有 15 条断言语义全保留(选择器从页面根改为 Host 根)。

**取舍**:开关成功不弹 toast(视觉翻转即反馈,免「操作成功」噪音);弹窗提交放内容组件内(而非 confirm 后页面提交),避免「失败但弹窗已关、输入丢失」的体验回退 —— 与「含独立校验/提交的表单→openManageDialog」选型一致。

---

## 3. FilmDetailView(/manage/film/detail?query id)

**改什么**:单源采集 collectOne 与候选版本采集 onPickCandidate 换 useAsyncAction(请求保持 silent,业务结果四态 toast 留在 action 内 —— 采集失败是 HTTP 200 里的 error 字段,非异常;异常 toast 由 run 单点,删手写 catch);按钮加 :disabled="busy" 全局防并发采集;其余(合并源列表/软删禁跳/源列表错误重试)不动;SourceCandidateDialog 保持组件式(见 §0 取舍)。

**验收**:
1. 采集四态 toast(失败/完成入库/弹候选/未搜到)与 silent 断言不回归;采集成功重载详情。
2. 新增:采集进行中(busy)再点其它源 → 不发第二个请求(并发拦截),释放后可再发起。

**取舍**:所有采集共享一个 useAsyncAction 实例 —— 采集是重操作,全局串行防抖,按钮 loading 仍按 siteId 粒度显示。

---

## 4. FilmAddView(/manage/film/add)

**改什么**:①「顶级分类」手写 <select> 换 ManageSelect(数字 value 原类型回传,「请选择」= 0);②采集(批量/单条命中/候选 pick)与建片提交换 useAsyncAction:add 载荷 silent:true,成功 after 跳回列表,失败 run 单点 toast、表单停留;per-hit loading 保留 hitCollecting 映射;③ManageInput number 清空 → undefined,提交 Number() 收口(既有);SourceCandidateDialog 保持组件式。整页表单仍为路由页,不弹窗化。

**验收**:
1. findComponent(ManageSelect) 命中,选择分类后提交 payload.pid 为数字;建片提交 class add(payload, { silent: true }),成功跳 /manage/film。
2. 采集进行中再点采集按钮不重复发请求(并发拦截);搜索/采集四态 toast 与既有断言不回归。

**取舍**:采集按钮用 :loading 不加 :disabled(JSDOM 下 disabled 元素 trigger 语义不稳),并发安全由 run 的 busy 拦截承担; FilmDetailView 的 :disabled 为视觉反馈补充,非安全依赖。

---

## 5. 完成记录(2026-11-03 08:02)

- 四页迁移全部完成, 影片域 spec 58 条 + 触及的共享组件 spec 全绿; `pnpm run type-check` 全仓通过。
- 过程注记:
  1. usePagedList 实际签名为 `usePagedList({ fetch })`(约定文本示例把 fetch 写成首参, 以 T 组实现与其 spec 为准), FilmListView 落地时已对齐。
  2. ManageTabs 泛型 emit 在 vue-tsc 2.1.10 下 V 推断回落默认值 string, 三个域消费方同款报错; T 组最终以「defineProps 内联对象类型引用泛型」方案根治, 全仓 type-check 转绿, 本域 handler 保持强类型 `ManageFilmStatus`。
  3. `views/manage/constants.ts` 新增 FILM_STATUS_TABS / FILM_STATUS_TAG_MAP(FilmListView 消费; FailureListView 的数值态页签留采集域自命名提升)。
  4. api/manage/film.ts 的 classUpdate/classDel/add 增加可选 config 形参(仅本域消费方), 支撑 silent 口径; http.ts 未动。
  5. 共享壳层 ManageLayout/theme.css(T0 线)由并行代理完成, 本域页面根只写 `flex min-h-0 flex-col gap-*`, 未做页面级高度 hack。
- 验证命令: `pnpm vitest run src/views/manage/film src/components/manage/ManageTabs.spec.ts ...`(65 passed)、`pnpm run type-check`(0 error)。e2e(playwright manage)未跑 —— 约定的验收面为该页 spec + type-check。
