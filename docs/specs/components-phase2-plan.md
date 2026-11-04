# 组件化二期 TDD 计划 — web 公共区 + 安卓壳

> 配套规格:docs/specs/components-phase2-spec.md(W1a~W2d / A1~A6 指向该文件)。web 与 android 两域目录不相交,但共享同一 git 工作树,**批次严格串行**避免提交交叉。

## 批次C1:web 小组件五件套(W1a~W1e)

| 步骤 | 内容 |
|---|---|
| C1-1 | BaseSwitch+BaseSwitchRow(TDD):spec 断言两尺寸视觉类/role=switch/aria-checked/change 事件 → 替换 SettingsView ×4 行与 PlayView 跳过弹窗;SettingsView :257-281 死 CSS 顺带删 |
| C1-2 | BasePasswordField(TDD) → 4 页 5 处替换(外壳不动) |
| C1-3 | SkeletonGrid(TDD) → 桌面 ×6 + TV ×3 |
| C1-4 | EpisodeBadge(TDD) → 2 处模板 + tv-cards.css .ep 收敛 |
| C1-5 | SectionHeader(TDD) → 桌面 5 处 |
| C1-6 | 门禁 vitest+type-check;提交 `feat(web): 公共区基础控件组件化(开关/密码框/骨架网格/集数角标/区块标题)` |

## 批次C2:web 页面组合与清理(W2a~W2d)

| 步骤 | 内容 |
|---|---|
| C2-1 | TvResultBoard(TDD,保 DOM 类名与 data-focusable 顺序)→ ClassifySearch/HotBoard TV 分屏收编(含 HotBoard 抄写的 CSS) |
| C2-2 | FilmResultSection(TDD)→ 两页桌面流水线 |
| C2-3 | ListPageHeader(TDD)→ 收藏/历史页头+link-btn+卡角删除钮 |
| C2-4 | 清理:QuickEntryRow/BasePagePlaceholder 删除(全仓 grep 复核) |
| C2-5 | 门禁 vitest+type-check;提交 `refactor(web): 列表页组合组件化(TV 分屏/结果区/页头)+孤儿清理` |

## 批次C3:安卓死代码与弹窗迁移(A1+A2)

| 步骤 | 内容 |
|---|---|
| C3-1 | A1 死代码删除(逐一 grep 复核) |
| C3-2 | A2 JcServerUrlPrompt 下沉 core(JcDialog.custom+ServerUrl.normalize)+两壳改调;UpdateChecker→JcDialog.message |
| C3-3 | 门禁三模块单测;提交 `refactor(android): 死代码出清+服务器地址弹窗下沉 JcServerUrlPrompt+更新弹窗迁 JcDialog` |

## 批次C4:core 工具与逻辑下沉(A3+A4+A5)

| 步骤 | 内容 |
|---|---|
| C4-1 | JcUi(dimenPx/col/glassButtonBg/glassPanelBg)值不变合并 ×7 处改引 |
| C4-2 | DoubleBackExitGate 下沉+WebShell 改用+reset 语义对齐(测试随迁) |
| C4-3 | 网络判定纯函数+方向参数(fail-open/fail-closed 语义保留)+app 单测 |
| C4-4 | 门禁三模块单测;提交 `refactor(android): JcUi/DoubleBackExitGate/网络判定下沉 player-core` |

## 批次C5:播放器令牌补齐与零散(A6)

| 步骤 | 内容 |
|---|---|
| C5-1 | hex→R.color 逐字等值替换+新增 jc_step_btn_press 令牌+GlassToast/dimens 零散 |
| C5-2 | 门禁三模块单测;提交 `refactor(android): 播放器 UI 字面量收敛到令牌(值不变)+下载页 Toast 统一` |

## 三轮审查

- **R1(双评审并行)**:①web 一致性与 TV 保真——逐组件对照规格视觉契约(类名/尺寸/焦点顺序)、grep 验收(裸控件/重复块消减)、chrome61 禁项;②android 铁律与分层——JcDialog 合规、令牌入口唯一、刻意分叉未误伤、nonTransitiveRClass 全限定、值不变证明(逐处 diff 对照旧字面量)。
- **R2**:逻辑回归(web: TV 分支行为/键盘与焦点/替换点 props 传递;android: 弱网降级方向、reset 语义、删代码无悬挂引用)。
- **R3 终验**:规格逐条 pass/fail+真机清单定稿。

## 提交链预期

```
docs: 组件化二期规格与计划落盘(SDD)
feat(web): 公共区基础控件组件化            ← C1
refactor(web): 列表页组合组件化+孤儿清理    ← C2
refactor(android): 死代码+弹窗迁移          ← C3
refactor(android): core 下沉                ← C4
refactor(android): 令牌补齐                 ← C5
fix(web)/fix(android): 第N轮评审修复         ← 逐轮(如需)
```

## 门禁凭据补录（R3 评审 F1）

> 背景：C1(6129a9f)/C5(d0b91ed) 提交信息正文漏记门禁、R1 修复 a903116 改 android 代码亦无凭据行。以下为 2026-11-05 于 HEAD 补跑；佐证口径：`git diff daec1ff..HEAD --stat -- web` 与 `git diff d0b91ed..HEAD --stat -- android` 均为空——web 代码自 C2、android 代码自 C5 起零改动，故补跑结论即当时代码的门禁结论（C2/C3/C4 当次凭据已在各自提交信息正文）。规格 §3 所写 `node <pnpm.cjs>` 为占位符，仓库无 pnpm.cjs，实际用全局 pnpm 等价执行。

| 条目号 | 命令 | 结果 |
|---|---|---|
| C1-6 web 门禁 | `pnpm run test` / `pnpm run type-check`（在 web/ 下） | vitest **138 文件 1396 例全绿**（178s）；vue-tsc 零错误退出；与 C2 daec1ff 提交信息所载当次计数（138/1396+vue-tsc）一致 |
| C5-2 三模块单测 | `bash android/gradlew -p android :player-core:test :tvshell:test :app:testDebugUnitTest` | **BUILD SUCCESSFUL**；:player-core **276** / :tvshell **72** / :app **39**，failures=0 |
| R1 修复 a903116 当次实跑补录 | 同上三模块命令（2026-11-05 06:52 对改动后代码实际执行，非补跑） | BUILD SUCCESSFUL；测试结果 XML（player-core 34 类/tvshell 25 类/app 4 类）合计 276/72/39 例零失败；此后重跑任务 UP-TO-DATE 证 android 代码未再变 |
