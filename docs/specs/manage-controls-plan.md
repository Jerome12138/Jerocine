# 管理端控件组件化 TDD 计划 — manage-controls

> 配套规格：`docs/specs/manage-controls-spec.md`（契约与决策编号 D①~D⑩ 均指向该文件）。
> 执行方式：superpowers TDD——每个组件/能力**先写或改 spec 测试跑红 → 实现跑绿 → 小步重构**；批次全绿后提交一笔。
> 门禁命令见规格 §7.7。本机 pnpm 必须经 node 运行。

## 批次A：公共逻辑层 + ManageCheckbox（W0+W2）

| 步骤 | 内容 | 红/绿凭据 |
|---|---|---|
| A1 | `composables/useDismiss.spec.ts`（红）→ 实现 `useDismiss.ts`（绿）：外点关闭、ESC 关闭、isOpen=false 不监听、卸载清理 | 新 spec 全绿 |
| A2 | FilmListView.vue:196-210 迁 useDismiss（行为不变，删除内联 listener） | 既有 FilmListView 相关 spec 仍绿 |
| A3 | `composables/useListNavigation.spec.ts`（红）→ 实现（绿）：循环/Enter/Esc/Home/End/空列表/open 重置（§3.2 用例清单） | 新 spec 全绿 |
| A4 | `components/manage/ManageCheckbox.spec.ts`（红）→ 实现（绿）：checked/indeterminate/disabled 渲染、change emit、data-focusable | 新 spec 全绿 |
| A5 | 替换 4 处：DanmuConfigCard.vue:139-144（并删 `--jc-primary` 悬空引用）、FilmCollectDialog.vue:264-270、ManageTable.vue:381-387（并删 `--jc-brand` 悬空引用）、ManageCascader.vue:112-119（indeterminate） | 4 页既有 spec 仍绿；全量 test 绿 |
| A6 | 提交：`refactor(manage-ui): 公共逻辑层 composables + ManageCheckbox 组件落地` | — |

## 批次B：胶囊页签 + 弹窗宽度兜底（W1+W5）

| 步骤 | 内容 | 红/绿凭据 |
|---|---|---|
| B1 | ManageTabs.spec.ts 新增 segment 用例（激活类 `bg-surface`/`shadow`、min-h-28、无 hover）跑红 → 实现 segment 变体（绿），**同提交删 pill 分支**并改写 pill 用例 | ManageTabs.spec 全绿 |
| B2 | 迁移：DashboardView 两组（switchTime/statusFilter 逻辑不动）+ FilmListView/FailureListView/FileGalleryView/BannerListView `variant="segment"` | 5 页 spec 仍绿 |
| B3 | ManageSheet.spec / BaseDialog 相关：新增窄口 max-width 断言（D⑦）跑红 → 面板样式加 `max-width: 94vw`（绿） | 两组件 spec 全绿 |
| B4 | 提交：`feat(manage-ui): 胶囊页签 segment 变体全端统一 + 弹窗窄口兜底` | — |

## 批次C：ManageSelect v2 + 尺寸统一（W3+W4）

| 步骤 | 内容 | 红/绿凭据 |
|---|---|---|
| C1 | ManageSelect.spec 重写（红）：API 兼容用例（options/placeholder/disabled/ariaLabel/原类型回传）+ 新能力（浮层开关、外点/ESC 关闭、键盘 ↑↓/Enter/Esc、active 类、data-focusable、option/trigger 槽、size 档类）→ 实现 v2（绿） | 组件 spec 全绿 |
| C2 | 逐页跑受影响 spec（9 个使用页）修断言（行为断言口径）；`grep '<select'` manage 区清零 | 全量 test 绿 |
| C3 | FilmListView 排序菜单（:294-316）删手写浮层 → ManageSelect（trigger 槽 icon+label、option 槽 icon）；内联 dismiss 迁 useDismiss（A2 已就位） | FilmListView spec 绿 |
| C4 | ManageInput.spec 加 size 用例（红）→ 实现 size prop（绿）；ManageSelect 同步 size（C1 已含） | 两组件 spec 绿 |
| C5 | 工具栏对齐：§2.3 所列 8 页"与 sm 按钮同行的输入/下拉"加 `size="sm"`（逐行核对，无按钮同行的不动）；theme.css 加 number spinner 隐藏规则 | 全量 test + type-check 绿 |
| C6 | e2e 选择器核查：`grep -rn "selectOption\|<select" web/tests/e2e/`，命中则改选择器文件（不执行 e2e） | 只改文件 |
| C7 | 提交：`feat(manage-ui): ManageSelect v2 自绘浮层 + 控件尺寸档位统一` | — |

## 批次D：级联语义 + 规范文档（W6+W7）

| 步骤 | 内容 | 红/绿凭据 |
|---|---|---|
| D1 | 按"级联语义核查员"结论：若服务端 CatIDs 需要父级 id → FilmListView.applyCatFilter 加纯函数祖先归一（勾选 ∪ 子孙 ∪ 部分勾选祖先）+ spec 用例；否则在规格 §1 D⑩ 标注"核查结论：无需实施 + 证据"，不写代码 | 结论留档 |
| D2 | 新建 `web/docs/manage-ui-conventions.md`（~40 行，规格 §0/§3/§5/§6/§7 精华：尺寸三档令牌、data-focusable 必带、逻辑下沉 composables、组合组件门槛、headless 升级规则） | — |
| D3 | 提交：`docs(manage-ui): 级联语义核查结论 + 管理端组件规范` | — |

## 三轮审查（实现全部完成后）

- **R1 一致性与风格**（两名评审并行）：①页面一致性：全 manage 页走查——同类 UI 同组件、令牌引用、四档规则（§5）、§8.1-8.4 验收 grep；②组件质量：新组件代码/TDD 质量/TV focusable/a11y/类型严格。**按 §6 规则提组合组件提案（至多实施 2 个）**。→ 修复员逐条处置 → 全量 test + type-check → `fix(manage-ui): 第一轮评审修复`。
- **R2 逻辑与回归**（一名独立评审）：逻辑正确性、noUncheckedIndexedAccess 边界、e2e 选择器遗留、悬空引用/死代码、对公共区/无关文件的误伤。→ 修复员 → 门禁 → `fix(manage-ui): 第二轮评审修复`。
- **R3 终验**（一名独立评审）：对照规格 §8 验收标准逐条核验 + 分辨率四档令牌核对 + 关键页抽查（Dashboard/FilmList/LiveSourceList 弹窗）。→ 修复员（只修 high/medium）→ **最终门禁：`pnpm run build` + 全量 test** → `fix(manage-ui): 终验修复`（如有）。

## 提交链预期

```
docs(manage-ui): 控件组件化规格与计划落盘（SDD）        ← 工作流启动前
refactor(manage-ui): 公共逻辑层 composables + ManageCheckbox 组件落地   ← 批次A
feat(manage-ui): 胶囊页签 segment 变体全端统一 + 弹窗窄口兜底            ← 批次B
feat(manage-ui): ManageSelect v2 自绘浮层 + 控件尺寸档位统一             ← 批次C
docs(manage-ui): 级联语义核查结论 + 管理端组件规范                       ← 批次D
fix(manage-ui): 第N轮评审修复                                           ← 每轮（如需）
```

铁律：全程不 push、不部署；e2e 不跑（用户自行执行）。
