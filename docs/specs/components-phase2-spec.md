# 组件化二期规格（SDD）— web 公共区 + 安卓壳

> 2026-11-05 ｜ 分支 `manage-controls`（worktree 同前，紧随播放器修复批次之后）
> 证据基底：`D:\Git\JerocineHarness\docs\排查-公共区与安卓组件化-2026-11-05.md`（两路只读侦察,file:line 已核实）
> 方法论：superpowers SDD + TDD。**本文件是唯一契约；实施不得越规格，宁可少做。**

## 0. 总原则

1. **web 公共区视觉完全不变**：组件化只收敛实现；TV 抽取必须保持 DOM 类名与 data-focusable 顺序逐字不变（或整类迁移组件 scoped 并逐项保留 data-focusable）；legacy chrome61：新组件禁止新增 backdrop-filter/aspect-ratio。
2. **安卓铁律全部生效**（CLAUDE.md）：共享 UI 下沉 :player-core；col()/dimenPx() 令牌唯一入口；JcDialog 三入口禁手写 AlertDialog；`?attr/` 禁令；**刻意分叉清单禁止合并**（jc_tv_dialog_bg/jc_tv_toast_bg/jc_tv_badge_bg、jc_download_btn_primary vs jc_button_primary、jc_splash_brand、tvshell colors.xml 分工、jc_tv_border/border_faint/glass_soft——侦察报告已逐项书面核实）。
3. e2e 不写不跑；不 push 不部署；每批次一笔 commit；新逻辑带单测。

## 1. web 公共区实施清单

| # | 组件 | 替换点（file:line 依据侦察报告） | 契约要点 |
|---|---|---|---|
| W1a | `components/base/BaseSwitch.vue` + `BaseSwitchRow.vue` | SettingsView.vue:88-129（44×24 ×4 行）+ PlayView.vue:2204-2218（36×20 跳过弹窗） | **size 双档 `lg`=44×24 / `sm`=36×20 保两处视觉**；BaseSwitchRow=标题+描述+开关行（role=switch/aria-checked）；数据驱动沿用 SettingsView toggles 数组 |
| W1b | `BasePasswordField.vue` | LoginView:254-282(TV)/:486-513、Register:447-473、Activate:224-250、Reset:96-123 | 图标+眼睛+type 切换；各页外层 jc-tv-input/auth 外壳保留不动 |
| W1c | `SkeletonGrid.vue` | ClassifySearch:360-370、HotBoard:337-347、Classify:358-372、Search:319-321、Home:402-419、FilmDetail:361-377 + TV 变体 :244-251/:248-255/:221-237 | props: count/layout；内部仍用 BaseSkeleton |
| W1d | `EpisodeBadge.vue` | ContinueWatchingRow:147-163、HistoryView:370-387、tv-cards.css `.jc-tv-card .ep`(:64) | 渐变"看到第N集"角标；tv-cards.css 该类收敛后删除 |
| W1e | `SectionHeader.vue`（桌面） | FilmRow:128-142、ContinueWatchingRow:84-51、ClassifyView:385-398、HomeView:684-687、RelatedList:20-25 | h2+「更多›」；slot 差异化；TV 版 jc-tv-sec **不做**（后置） |
| W2a | `TvResultBoard.vue`（TV 分屏：筛选竖栏+网格+翻页器） | ClassifySearchView:211-298 ≈ HotBoardView:205-300（HotBoard:398-460 全套 jc-cs-tv__* CSS 抄写一并收敛） | **DOM 类名与 data-focusable 顺序不变**；两页只留数据与头部 |
| W2b | `FilmResultSection.vue`（桌面流水线：筛选+统计+骨架+Grid+分页+空态） | ClassifySearchView:334-393 ≈ HotBoardView:317-368 | 纯模板收编；BasePagination/FilmFilterBar 原样内嵌 |
| W2c | `ListPageHeader.vue` | FavoritesView:125-145 ≈ HistoryView:256-287 + TV 变体 :61-84/:117-177 + .jc-link-btn CSS ×2 + 卡角删除钮 ×2(:174-181/:327-334) | 标题+来源 Tag+计数+同步中+登录链接 slot；删除钮并入或独立小组件（随实现定，视觉不变） |
| W2d | 清理 | SettingsView :257-281 死 CSS；孤儿组件 QuickEntryRow.vue / BasePagePlaceholder.vue（删前全仓 grep 复核零引用） | 独立 chore 提交 |

**web 不做**（留档）：PlayView Teleport 覆盖层与 TV 工具条 jc-pt-btn；LiveView 频道/源按钮；PublicHeader 内部（B14 chip 重叠留专项）；HeroCarousel/FilmCard/EpisodeTabs/FilmRow（已是组件）；TvSectionHeader（×17 分散后置）；NotFoundView（标杆）；BaseButton 替换 SettingsView ghost 按钮（视觉需逐像素比对，评审若实测等价可并入，否则不动）。

## 2. 安卓侧实施清单（动序=批次序）

| # | 动作 | 位置 | 要点 |
|---|---|---|---|
| A1 | 死代码删除 | tvshell `PlayerChoiceDialog.kt`（92 行）+ 专用资源 4 件（dialog_player_choice.xml/item_player_choice.xml/drawable/jc_player_choice_item.xml/res/color/jc_player_choice_text.xml）；app `WebShellActivity.java:702-734 showDiagToast()`；`WebShellActivity.java:586-597 onAccountActionClick()`；tvshell `fragment_placeholder.xml` | **删除前逐一 grep 复核零引用**（源码+XML onclick） |
| A2 | JcDialog 迁移 ×1 + 服务器地址弹窗下沉 | 抽 `com.jerocine.player.ui.JcServerUrlPrompt.show(activity, current, onSave)`（内部 JcDialog.custom + `ServerUrl.normalize()`），两壳改调；`UpdateChecker.java:185` 更新弹窗 → `JcDialog.message()`（force 时省 cancelButton） | web 壳保存回调保留 `ServiceLocator.setServer(url)`；对照 tvshell NativeShellActivity.kt:279-293 既有写法 |
| A3 | 工具下沉 core | `com.jerocine.player.ui.JcUi`：dimenPx/col/glassButtonBg(=buildDrawerButtonBg 两份逐行合并)/glassPanelBg；WebShellActivity:657-674、JcSettingsDrawer:757-777、各 dp()/col() 副本（WebShell:474/:484、JcSettingsDrawer:545/:554、BrandSplashView:52、DownloadActivity:109、NativeShellActivity.kt:257）改引 core | **值不变**；nonTransitiveRClass 下 :app 引 core 资源写全限定 `com.jerocine.player.R.*` |
| A4 | DoubleBackExitGate 下沉 | tvshell `DoubleBackExitGate.kt`（含测试）→ `com.jerocine.player.data`（或 ui）；WebShellActivity 内联 `lastExitBackAt`(:80,:803-811) 改用并**对齐 reset 语义**（backstack 变化 reset）；tvshell 原引用改 import | 测试随迁或双跑 |
| A5 | 网络判定抽纯函数 | `WebShellActivity.isOnline()`:336-346（fail-open）与 `ServerNavigationWebViewClient.isOffline()`:133-143（fail-closed）→ core 纯函数+方向参数 | **两方向 fail-safe 语义必须保留**；跑 app 单测；真机弱网列为验收项 |
| A6 | 播放器令牌补齐（最后单独提交） | PlayerDialogHelper.java:235,240,241 setTextColor(0xFFFFFFFF)→jc_text；BrandSplashView.java:41-43 JC_BG/JC_ACCENT→R.color.jc_bg/jc_accent（0x99FFFFFF 无对应令牌→注释保留）；jc_checkbox_on.xml #FF4AD1E5→jc_accent；jc_badge_bg.xml #99000000→jc_scrim；jc_step_btn_bg.xml:17 #FF2BB8CC→**core 新增令牌 jc_step_btn_press**；jc_checkbox_off.xml #66FFFFFF 注释保留 | 值逐字不变；download/DownloadActivity:1745、DownloadEngine:479 裸 Toast→GlassToast；GlassToast:68 96dp→dimens；改共享层→**真机回归项**（原生壳主题非 AppCompat） |
| — | 不做 | tvshell jc_tv_quick_*.xml ×5；JcSettingsDrawer 内部散写字号（未强制）；jc_tv_stroke vs jc_stroke 差 1 alpha（列真机截图对比项）；R3 评审若发现新候选按「≤3 处重复或漂移风险+低风险」门槛裁决 | — |

## 3. 门禁

- web 批次：`node <pnpm.cjs> -C <web> run test` + `run type-check`（终验加 `run build`）
- android 批次：`bash <android>/gradlew -p <android> :player-core:test :tvshell:test :app:testDebugUnitTest`（终验加 `assembleDebug`）
- 新组件/新逻辑带 spec；web 组件 spec 断言渲染/交互行为与关键类名（视觉契约类名可断言）；android 纯函数单测

## 4. 真机验收清单（交付用户）

1. 安卓：双壳弹窗（服务器地址/更新弹窗/诊断）在**原生壳**点一遍（非 AppCompat 主题）；离线兜底页两按钮；弱网降级方向（fail-closed 不把有网用户踢进原生壳）；双击退出；下载页 Toast；播放器跳过面板/弹窗视觉与改前一致（令牌替换零漂移）
2. web：登录/注册/激活/重置 4 页密码框；SettingsView 与 PlayView 跳过弹窗两尺寸开关；ClassifySearch/HotBoard 桌面+TV 两形态；收藏/历史页头与卡角删除；首页/详情/行组件的角标与区块标题；chrome61 低配真机抽查
3. 现有 e2e（用户自跑）：manage spec 不受本期影响（本期未动 manage 区）
