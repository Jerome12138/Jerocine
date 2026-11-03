# 迁移 C 线迷你规格 —— 用户系统与直播域(/manage/user、/manage/register/applies、/manage/system/*、/manage/live)

> 逐页推进:SDD 迷你规格 → spec 先红(red) → 迁移实现转绿(green) → 清理。
> 每页规格在该页动工前追加;关键取舍记录于各节「取舍」。

## 0. 前置勘察(2026-11-03)

共享层(T0-T7/P1-P10)已在库:ManageTable 增强版、usePagedList、useAsyncAction、useManageDialog + ManageDialogHost、ManageSheet.width、ManageInput number 空值、ManageTextarea.maxlength、StatusTag、ManageSelect、formatDateTime 均就绪(A 线补建的 ManageSelect/StatusTag/ManageTabs/usePagedList/constants.ts 直接复用)。基线:本域 6 个 spec 文件 99 条全绿(`npx vitest run src/views/manage/user src/views/manage/system src/views/manage/live`)。

注意:任务页表的 `LiveSourceListView tests:[]` 与实际不符 —— `LiveSourceListView.spec.ts` 已存在 18 条,迁移须全数保留语义。

**API silent 口径**:register.ts(approve/reject/resendActivation)与 system.ts(revokeSession)、live.ts(previewChannels/listLiveChannels)已内置 silent;其余写端点按 A 线 film.ts 先例给本域 API 模块(user/system/mail/danmu/live)加**可选 config 形参**(默认行为不变,仅本域消费方传 {silent:true}),http.ts 不动。

---

## 1. 共享壳层响应式改造(本组专属职责,d)

**审计结论**(逐项对照,均有实证):ManageLayout `.jc-manage-main` 高度链+1440px 封顶已由 T0 落地(ManageLayout.vue:58,spec `main 内存在 .jc-manage-main…` 7 条断言);theme.css 密度令牌 `--jc-table-cell-py:10px`/`[data-mode="tv"] 12px` 已落(theme.css:142/334);侧边栏三形态 drawer/mini/full 已响应式(ManageSidebar.vue:16-22)+ 移动 44px 触摸目标兜底(theme.css:505-513)。**均不需要重做。**

**实缺口**:ManageHeader.vue:80 的 `flex-between` 是**无定义死类**(全仓 grep 无该类定义),header 实际是 display:block,左簇/PWA 徽标/头像竖排堆叠溢出 56px 高度。改为 `flex items-center justify-between`。补 ManageHeader.spec.ts 锁回归(新文件,无归属冲突)。

---

## 2. UserListView(/manage/user)

**改什么**:列表骨架换 usePagedList(删页面自维护 rows/loading/total/pageSize/loadSeq/changePage/末页回夹,fetch 闭包携带 keyword) + ManageTable 内置分页(:total + v-model:current + :page-size),删页面级 BasePagination;新增/编辑抽 `UserFormDialog.vue`、重置密码抽 `UserResetPasswordDialog.vue`(均 openManageDialog 发起,内容组件自治「校验+提交」: 校验拦截/失败 toast 弹窗停留,成功 emit('confirm') → 页面 paged.load());角色 select 换 ManageSelect(数字 value 原类型);角色/状态列换 StatusTag(map 就近页面顶部);私有 fmtTime 删,换 formatDateTime(CreatedAt 为 ISO 字符串);禁用/删除换 useAsyncAction(confirm+silent+successMsg+after=paged.load);页面根 `flex min-h-0 flex-col gap-*`。

**验收**:1) 分页 nav 渲染于 ManageTable 内部,`update:current` 驱动翻页,末页删空回夹/竞态守卫/失败重试不回归;2) findComponent(UserFormDialog/UserResetPasswordDialog) 经 ManageDialogHost 命中,编辑回填 initial;3) 既有 21 条断言语义全保留(弹窗选择器移 Host 根),create/update/setDisabled/remove 载荷带 `{silent:true}`。

**取舍**:弹窗提交放内容组件内(而非 confirm 后页面提交),避免「失败但弹窗已关、输入丢失」—— 与 A 线 FilmClassFormDialog 同款;「不能修改自己的角色」第二道拦截随提交迁入弹窗组件(第一道=ManageSelect :disabled),selfId 由页面以 prop 下发。

---

## 3. RegisterApplyListView(/manage/register/applies)

**改什么**:usePagedList + 内置分页(同 §2);状态筛选手写 select 换 ManageSelect(aria-label=状态筛选,数字 value,change→paged.search());状态列换 StatusTag(APPLY_STATUS_TAG_MAP:0 待审/warning、1 已通过/success、2 已驳回/danger,驳回理由子行保留);手写 textarea 换 ManageTextarea(maxlength=255);私有 fmtTime 删换 formatDateTime;通过/驳回/补发换 useAsyncAction(warning 是 HTTP 200 业务字段,分支 toast 留 action 内,异常 toast 由 run 单点;approve 带 confirm,reject 抽 `RegisterRejectDialog.vue` 函数化)。

**验收**:1) 翻页/筛选/搜索/回夹/竞态全不回归,筛选切状态回第 1 页;2) ManageTextarea 带 maxlength=255,空理由/超长前端拦截不变;3) 既有 15 条断言语义全保留,approve/reject/resendActivation 保持 silent(已内置),补发不刷新列表的行为锁定不变。

**取舍**:approve 的 warning/success 分支文案留在 action 内(业务结果非异常,run 不传 successMsg);after=paged.load() 统一「成功才重载」—— 通过失败/被取消时 run 已保证不触发。

---

## 4. SiteConfigView + MailConfigCard + DanmuConfigCard(/manage/system/site 内嵌卡片)

**改什么**:三卡全部写操作换 useAsyncAction + `{silent:true}`(updateBasic/updateRuntimeSettings/setTMDBKey/clearTMDBKey/saveMailConfig/testMail/saveDanmuConfig,API 模块加可选 config 形参);MailConfigCard 3 处手写 select(注册模式/服务商预设/加密方式)换 ManageSelect(字符串 value,预设选择为临时 ref 初值 undefined 走 placeholder);数字字段维持「清空=undefined + 提交前 Number() 收口」现状。卡片栅格/加载失败禁存等结构不动。

**验收**:1) 各卡独立保存、响应只更快照不冲草稿、范围校验中文拦截全不回归;2) findComponent(ManageSelect) ≥3 且注册模式/加密方式 value 原类型回传;3) 既有 26+7 条断言语义保留,载荷断言补第二参 `{silent:true}`。

**取舍**:保存成功后「响应回填」(danmu/mail 的 form 回写)留在 action 内随请求完成,successMsg 由 run 弹 —— 单一 toast 来源;TMDB clearKey 的 confirm 继续走 useConfirm(确认型)。

---

## 5. SessionListView(/manage/system/sessions)

**改什么**:非分页列表(服务端 limit 截断,无 page/total),**不套 usePagedList**,保留 load/loadError/keyword;kick 换 useAsyncAction(请求已内置 silent;404=会话已下线是业务分支:action 内吞掉 + info toast,统一由 after 重载;其余异常 rethrow 由 run 单点 error toast);`revoking` 行级 busy 换共享 busy;formatDateTime 已用(既有合规)。

**验收**:1) 踢下线 confirm→revoke→成功 toast+刷新、404→info toast+刷新、失败→error toast 不刷新,三态 toast 语义与既有 12 条断言一致;2) revoke 在途行按钮 disabled,释放后恢复。

**取舍**:404 分支不在 action 内自行重载,而是吞掉异常后交给 after 统一 load() —— 避免「action 内 load + after load」双刷;信息文案「列表已刷新」由 after 兑现。

---

## 6. LiveSourceListView(/manage/live)

**改什么**:①新增/编辑直播源抽 `LiveSourceFormDialog.vue` 函数化(自治校验+提交,校验文案/排序 0 兜底警告全保留,state 复选框换 ManageSwitch,成功 emit confirm → 页面 load()+loadCounts());②频道面板/自定义频道/改名三个内联弹窗 BaseDialog→ManageSheet(width 透传,移动端得 sheet 形态+ESC 弹层栈),页内 payload 逻辑不动;③toggleState/remove/toggleChannel/saveRename/saveCustom/removeChannel/toggleLiveNav 换 useAsyncAction —— 源行级共用一实例(替代 rowBusyId 互斥)、频道操作共用一实例(替代 chSaving 互斥,保住「任一在途全行禁用」语义);live.ts 写端点加可选 config 形参传 silent。

**验收**:1) 既有 18 条断言语义全保留(F2-F10 全数):频道分页/过滤、隐藏 payload 字段矩阵、改名带回 hidden/groupName、upsert 在途禁用、确认弹窗;2) findComponent(LiveSourceFormDialog) 命中,createLiveSource 排序清空按 0 兜底;3) updateLiveSource/deleteLiveSource/upsertLiveChannel/deleteLiveChannel/updateLiveNav 载荷带 `{silent:true}`;4) findComponent(ManageSwitch) 在弹窗表单内命中。

**取舍**:源表单函数化 vs 频道面板保持组件式 —— 面板是复杂管理面(搜索+分页+批量操作),非「含独立校验/提交的表单」,与 A 线保留 SourceCandidateDialog 同理;自定义频道/改名是一次性内联小弹窗,按选型保留组件式但迁 ManageSheet(移动适配收益,零逻辑迁移)。

---

## 7. 完成记录(2026-11-03)

- 七项(壳层修复 + 6 页/卡)全部完成;本域 + 触及共享件 spec **161 条全绿**(`npx vitest run src/views/manage/user src/views/manage/system src/views/manage/live src/components/layout/ManageHeader.spec.ts ManageLayout ManageSidebar + usePagedList/useAsyncAction/useManageDialog`,12 文件);全仓 `pnpm test` **121 文件 1243 条全绿**;`pnpm run type-check` 0 error。
- 过程注记:
  1. 壳层唯一实缺口是 ManageHeader.vue:80 的 `flex-between` 无定义死类(header 竖排溢出),改为 `flex items-center justify-between`,ManageHeader.spec.ts 追加回归锁;ManageLayout/侧边栏/密度令牌 T0 已落地,本组审计未重做(证据见 §1)。
  2. API 口径:user/system/mail/danmu/live 五个本域模块写端点加**可选 config 形参**(默认行为不变,仅本域传 {silent:true});register.ts/revokeSession 原已内置;http.ts 未动。
  3. 函数化弹窗 5 个:UserFormDialog/UserResetPasswordDialog/RegisterRejectDialog/LiveSourceFormDialog(自治校验+提交,失败弹窗停留);LiveSource 频道面板与两个一次性小弹窗保留组件式并迁 ManageSheet(mobile fullsheet/width 透传),与 A 线保留 SourceCandidateDialog 同理。
  4. StatusTag 落地:用户角色/禁用态、注册申请状态(映射常量就近页面顶部,未提升 constants —— 均为单页消费)。
  5. SessionListView 不套 usePagedList(服务端 limit 截断无分页);kick 404 业务分支吞掉后由 after 统一重载,避免双刷;共享 busy 使「任一 revoke 在途全部按钮禁用」(新锁)。
  6. LiveSource 频道操作沿用跨行互斥语义:chBusy 单实例替代 chSaving,F9「upsert 在途全行禁用」原样保留;loadChannels 失败仍走拦截器提示(非 silent 的 GET 不在 run 内)。
  7. 全仓 `pnpm test` 曾出现一次 10 失败瞬时态(并行会话改动文件被同时收集),复跑即全绿,与本域文件无关。
- 验证命令:`npx vitest run …`(161 passed)、`npx vitest run`(1243 passed)、`pnpm run type-check`(0 error)。e2e(playwright manage)未跑 —— 与 A 线口径一致,约定的验收面为该页 spec + type-check。
