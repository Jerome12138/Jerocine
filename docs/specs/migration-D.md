# 迁移 D 线迷你规格 —— 综合域(仪表盘 / 轮播 / 文件 / 遥测)

> 域内页面: DashboardView(/manage/index)、BannerListView(/manage/banner)、
> FileGalleryView(/manage/file/gallery)、FileUploadView(/manage/file/upload)、TelemetryView(/manage/telemetry)。
> 每页一节: 改什么 / 验收标准 / 关键取舍。执行方式 SDD+TDD, 逐页推进(先红后绿)。

## 0. 前置事实与本线自主决策(关键取舍)

1. **共享层现状**: 95fc5df(T0-T7/P1-P10 组件化主体) + B 线未提交补件(usePagedList/ManageSelect/StatusTag/ManageTabs/formatShortDateTime/ManageLayout .jc-manage-main)全部就绪, 本线零补建。ManageDialogHost 已挂 App.vue:483。
2. **API 层小改**(与 A/B 线同构的页面私有模块增量): `api/manage/file.ts` remove、`api/manage/telemetry.ts` resolveIssue/unresolveIssue、`api/manage/banner.ts` move/save/remove/disableByMid 补可选尾参 `config?: AxiosRequestConfig` 支撑 `silent: true`。grep 验证消费方均仅本域页面(file.remove→FileGalleryView; telemetry resolve/unresolve→TelemetryView; banner 全部→BannerListView), 不影响他线。
3. **弹窗选型核对**: BannerListView 表单(8 字段+双图上传+3 条校验)与 TelemetryView 标记解决弹窗(commitId 必填校验+提交)均属「含独立校验/提交的表单」→ openManageDialog 函数化, 内容组件自治校验+提交(FilmClassFormDialog 同款契约: emits confirm(payload)|cancel, footer 自带); 确认型(轮播删除/榜单刷新/文件删除/清空记录/撤销解决)一律 confirm()。
4. **非表格页不硬套 ManageTable/usePagedList**: DashboardView 在线明细=客户端切片分页(数据一次拉全, 无逐页 fetch)且行状态三色渲染, FileGalleryView 是媒体网格(视频首帧缩略), TelemetryView 错误列表=可展开行(load-more 追加式)。三者保留原有列表形态; FileGalleryView 仅把分页**状态**收敛进 usePagedList(rows/total/current/回夹/守卫), UI 仍用 BasePagination。
5. **时间格式化口径**: DashboardView fmtTime(实时 HH:mm:ss)/fmtDateTime(当日 HH:mm:ss, 跨日 MM-DD HH:mm:ss)与 TelemetryView fmtTime(toLocaleString 含秒)删除, 统一走 utils: 遥测错误时间→formatDateTime(serverTs); 仪表盘进入/最近活跃→formatShortDateTime(sec*1000)。取舍: utils 单一来源只有分钟精度两种格式, 仪表盘实时列由「纯时分秒」变为「MM-DD HH:mm」(跨日可读性更好, 秒位舍弃), 既有 spec 未断言时间文案, 无回归面。

## 1. FileUploadView(/manage/file/upload, 既有 10 条 spec)

**改什么**: 仅两处 —— ①根节点删页面级 `max-w-[960px]`(约定禁止页面级 max-width hack, 桌面密度由 .jc-manage-main 统一封顶 1440px); ②根节点补 `min-h-0`。多文件上传队列不动(exclusions 明确保留)。

**验收**: 既有 10 条 spec 全绿(不锁根类, 零断言面变化)。

> 勘察修正: 任务清单的 tests 元数据过时 —— file/telemetry/banner 四个 spec 均已存在(提交 6456d08「补齐单元测试」), 本线按「更新既有 spec」执行而非新建。

## 2. FileGalleryView(/manage/file/gallery, 既有 8 条 spec)

**改什么**: ①列表状态收敛 usePagedList(fetch 闭包= manageApi.file.list({current: page}), pageSize 39 对齐服务端) —— 删 items/loading/total/pageSize/loadSeq/params.current/changePage/末页回夹约 40 行样板; BasePagination 改 `v-model:current`(翻页由 current watch 自动触发)。②「全部/图片/视频」筛选按钮组 → ManageTabs(pill, role=tablist)。③删除 → useAsyncAction(confirm + remove(id,{silent:true}) + successMsg「已删除」+ after=load), 消现状双重 toast(拦截器 success + 页面「已删除」; 错误拦截器 + catch 各一条)。④页面根补 min-h-0。

**验收**: 既有 8 条 spec 更新 3 处组件接线断言后全绿 —— ①翻页 emit 由 `change` 改 `update:current`(v-model:current 接法, 翻页由 usePagedList 的 current watch 触发); ②remove 断言补第二参 `{ silent: true }`; ③其余(渲染/筛选/错误态/在途防护/末页回缩)语义不变。新增断言: ManageTabs role=tablist/aria-selected。

## 3. DashboardView(/manage/index, 既有 12 条 spec)

**改什么**: ①fmtTime/fmtDateTime 删除, 进入/最近活跃列统一 formatShortDateTime(sec*1000)(取舍见 §0.5); ②根节点补 min-h-0。在线明细表(客户端分页/三色状态/筛选 pill 组)与卡片栅格保留 —— 页面级布局重组在 exclusions, ManageTable/usePagedList 不适配(见 §0.4)。

**验收**: 既有 12 条 spec 全绿; 新增断言: 明细行时间渲染为 `\d{2}-\d{2} \d{2}:\d{2}` 短格式。

## 4. TelemetryView(/manage/telemetry, 既有 10 条 spec)

**改什么**: ①时间范围 select(v-model.number)与状态筛选 select → ManageSelect×2(数字 option 原类型回传, aria-label=时间范围/状态筛选); ②issueStatus 列 statusVariant/statusText → StatusTag + ISSUE_STATUS_MAP(unresolved→danger/未处理, reopened→warning/已重启, resolved→success/已解决, expired→default/已过期); ③fmtTime → formatDateTime(row.serverTs); ④标记解决弹窗 → openManageDialog + 新建 TelemetryResolveDialog.vue(props: issue 行, 预填 resolution 的 commitId/note; 自治校验 commitId 必填 → toast error 弹窗停留; 自治提交 resolveIssue({silent:true}) 成功「已标记解决」→ emit confirm → 页面 loadAll()); ⑤undoResolve → useAsyncAction(confirm + unresolveIssue({silent:true}) + successMsg「已撤销标记」+ after=loadAll), undoingId 行级 loading 保留。load-more 追加与 reqSeq 守卫保留(§0.4)。

**验收**: 既有 10 条 spec 全绿(12 条含衍生), 更新点 —— ①标记解决/撤销两条用例改 mountAll(页面 + ManageDialogHost 同挂)后经 Host wrapper 查询弹窗内容; ②resolveIssue/unresolveIssue 断言补第二参 `{ silent: true }`。保留: 两个 select 仍按 `findAll('select')[0]/[1]` 定位(ManageSelect 根即原生 select, setValue 字符串值经 onChange 还原原类型), 请求序号守卫/加载更多/骨架/错误态语义不变。

**坑(测试基建)**: 页面 + Host 两个 VTU mount 共享全局 stub 注册表, 后挂的 Host 若不带页面的 stubs(如 RouterLink 替身), 会把页面的 stub 顶掉(Vue warn「Component is missing template or render function: { name: 'RouterLink' }」, RouterLink 行渲染为空)——两个 mount 必须共享同一份 STUBS 常量(实证: __debug.spec 对照挂法 A/B, A 含「好片」true / B false → 补共享 stubs 后 true)。

## 5. BannerListView(/manage/banner, 既有 25 条 spec)

**改什么**: ①表单函数化 —— 新建 BannerFormDialog.vue(props: initial 完整表单初值 + adoptSlot?, 自治 3 条校验 toast + 自治提交 save(payload, adoptSlot, {silent:true}), 成功「已保存」→ emit confirm), 页面 openAdd/openEdit/adoptSlide 改为构造表单初值 + openManageDialog(sheet 态标题逻辑留在页面, mobileMode='fullsheet'); ②行内写操作换 useAsyncAction(single 实例): move/toggleState/disableByMid(silent:true, 成功不弹 toast —— 看板即时回读即反馈, 消现状每次操作一条「操作成功」噪音, A 线 §2 同款取舍)、remove(confirm + silent + 「已删除」)、refreshHot(confirm + silent + 动态报告文案 action 内自管)、fillBackdrop(保留 fillingMids 行级 loading 与按 status 定制 toast, silent 已在 API 内); inFlight 删, 行内按钮 :disabled 收敛到 busy; ③既有 ManageSheet/BaseLightbox 部分随表单移入内容组件(lightbox 留页面)。

**验收**: 既有 25 条断言语义全保留(表单类选择器从页面根改到 Host 根, mountAll 模式); 新增断言: findComponent(BannerFormDialog) + save 第三参 {silent:true}; 删除/启停/移动/回填 toast 单一来源。

**取舍**: 采纳自动位的钉入 slot 由页面在 open 时作为 props 传入内容组件, 提交时随 save 透传 —— Promise 语义只回传「成功与否」, 业务载荷不需要回传页面。

## 6. 执行记录(2026-11-03)

| 项 | 结果 |
|----|------|
| FileUploadView | 10/10 绿(仅根类清理, 零断言面变化) |
| FileGalleryView | 8/8 绿(红→绿: usePagedList 接管分页状态 + ManageTabs + 删除走 useAsyncAction; spec 更新 update:current/silent 断言) |
| DashboardView | 16/16 绿(红→绿: 新增短时间格式断言后删 fmtTime/fmtDateTime 换 formatShortDateTime(sec*1000)) |
| TelemetryView | 12/12 绿(ManageSelect×2 + StatusTag/ISSUE_STATUS_MAP + formatDateTime + 函数化解决弹窗 TelemetryResolveDialog + undoResolve 走 useAsyncAction) |
| BannerListView | 23/23 绿(红→绿: 表单函数化 BannerFormDialog + 六类写操作 useAsyncAction + inFlight 删除收敛到 busy) |
| 本域合跑 | `pnpm vitest run src/views/manage/DashboardView.spec.ts src/views/manage/banner src/views/manage/file src/views/manage/telemetry` → 5 文件 69/69 通过(API 签名变更后复跑确认) |
| type-check | `pnpm run type-check` 全仓 0 error(banner API config 参补齐后) |
| e2e | 未跑: playwright 配置需本地后端(mock 适配器已移除), 属 G1/主流程门禁环境, 同 A/B 线口径 |

过程注记:
1. 任务清单 tests 元数据过时: file/telemetry 的 spec 早已存在(6456d08), 本线按「更新既有 spec」执行。
2. **VTU 多 app stub 注册表坑**: 页面 + ManageDialogHost 两个 mount 共享全局 stub 注册表, 后挂的 Host 必须带上页面的全部 stubs(RouterLink 替身等), 否则页面 RouterLink 渲染为空(Vue warn「missing template or render function」)——TelemetryView/BannerListView 两 spec 均以共享 STUBS 常量解决, 已写入 §4。
3. api/manage/{file,telemetry,banner}.ts 写函数补可选尾参 config(A/B 线同构增量, grep 验证消费方仅本域页面); http.ts 未动。
4. BannerListView 轮询守卫由 sheetOpen 换 manageDialogState.visible(函数化弹窗打开时暂停 30s 轮询)。
5. 写操作 toast 口径最终态: move/toggleState/disableByMid 成功静默(看板回读即反馈, 消原「操作成功」噪音); remove「已删除」/保存「已保存」/回填与榜单刷新动态文案 action 内自管; 全部 silent + run 单点错误提示, 无双重 toast。
6. 工作区存在 C 线(user 域)并行改动, 本线仅触及上列文件, 未碰共享壳层与他域文件。
