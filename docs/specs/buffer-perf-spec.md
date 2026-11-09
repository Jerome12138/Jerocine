# 缓冲性能规格（SDD）— buffer-perf

> 2026-11-09 ｜ 分支 `buffer-perf`（worktree `D:\Git\JerocineHarness\.worktrees\buffer-perf`，基线 `0699560`）
> 方法论：superpowers SDD + TDD。**本文件是唯一契约：实施不得越规格，宁可少做，缺的记入交付报告。**
> 证据基底：两轮只读勘察（buffer-perf 工作树，结论均带 file:line；本文所有引用在落笔前已逐条对源码核对）。
> 2026-11-09 第1轮评审修订（10 条全采纳）：D⑤ 改 java.util.TimeZone/Calendar 口径（minSdk 21 无 desugaring，java.time=API 26+ 会崩 Android 5-7 真机且 debug 门禁不跑 lint）；D④ 补失败标志+排队任务入口短路；D⑦ 探测 client TLS 与播放侧同款 + 进程级节流熔断；D⑥ max 180s→150s 补堆内 RAM 维度；D③/§3.1 进度消费点补身份守卫；D⑧ 边际措辞统一「快超过 20%」；x/N 钉「窗口内 1-based 序号」；聚合器改按窗口序号升序重算（消浮点非结合性）；cancel 丢任务竞态注记已知边缘。
> P1 大方向用户已拍板：并行写分片 + 固定线程池（禁 cachedThreadPool）+ 并行度定值 3 + 用户开关默认关（=单线程顺序）+ 进度口径改「已完成窗口时长/窗口总时长」+ cancel 集合化 + 既有契约不动 + 新纯逻辑抽 JVM 可测类。

## 0. 背景与现状（已实锤，勿重查）

1. **P1 单线程顺序写**：`BufferPrefetcher.java:73` `newSingleThreadExecutor` 一根线程顺序写窗口分片；进度按「已写完分片时长累计/窗口总时长」（:189-191 算 target，:213-215 累加后 post 主线程）。契约：**一实例一次 start**（:88-91，正常结束 finally shutdown、cancel 走 shutdownNow，复用即抛/秒回「已取消」，调用方每次缓冲 new 新实例，PlayerActivity.java:1354）；cancel 掐 `activeWriter` 单引用（:74-76, :232-244）并 **必须 shutdownNow**（:241-243 泄漏教训）；每片独立 OkHttpDataSource/CacheDataSource（:197-201，CacheDataSource.close 顺手关 upstream，共享必炸）；UA 必须 `"Jerocine/1.0 (Android TV)"`（:49-50，与播放侧 `MEDIA_UA` PlayerActivity.java:114 同串，不一致=缓存 miss 白写）。
2. **P2a 两份逐字副本**：DefaultLoadControl 参数 `setBufferDurationsMs(30_000, 120_000, 1_500, 2_500)`（位次 = min/max/playback/rebuffer）+ `setPrioritizeTimeOverSizeThresholds(true)` + `setBackBuffer(30_000, true)` 在 PlayerActivity.java:400-405（在线）与 :338-342（本地文件）各一份——改一漏一即本地/在线行为分叉。
3. **P2b native 无测速**：源列表三件套 `session.sourceList / currentSourceIndex / currentRawUrls`（PlayerSession.java:135-136, :151），唯一装载出口 `loadSourceIntoPlayer → loadPlaylistIntoPlayer`（PlayerSession.java:303-361）；手动切源执行序列钉死「先写 currentSourceIndex 再 loadSourceIntoPlayer」（PlayerDialogHelper.java:138-144，顺序错 = refreshLocalDownloads 按旧源查、本地优先失效）；单 URL bypass（PlayerSourceHelper.java:69-72）无多源语义。
4. **OOM 教训（并行化的两条红线）**：DownloadEngine.java:1255-1263 —— media3 SegmentDownloader 把一个清单的所有分片一次性全提交，cachedThreadPool = 无界并发，bf 源 710 线程 + 堆耗尽 OOM 死循环，定论 `newFixedThreadPool(4)`；集级 `MAX_PARALLEL=3`（:77）+ 分片级 4 = 12 连接安全水位。:138-143 构造器注释再钉「**不要用 newCachedThreadPool**」。
5. **测试基建**：纯 JVM JUnit4，无 Robolectric / 无 mockwebserver（player-core/build.gradle:88-90 仅 junit + gson）；BufferPrefetcher 直接依赖 Handler/Looper/Uri（:3-4, :256-258），start() 全链路不可纯 JVM 直测——可测逻辑必须留在纯函数层（既有先例：PlaylistSegmentsTest、PlayerModesTest、JcDialogRulesTest 共 37 个测试文件）。
6. **切源联动取消**：loadPlaylistIntoPlayer → host.onPlaylistReloaded（PlayerSession.java:343）→ cancelBufferPrefetch（PlayerActivity.java:1204-1207）；本地⇄在线切换同取消（:1212-1224）；onDestroy 取消（:895-896）。

## 1. 决策（D①~D⑩，每条含 决策 / 理由 / 不做）

| # | 决策 | 理由 | 不做 |
|---|---|---|---|
| D① | **P1 并行引擎**：BufferPrefetcher 分片写并行化，构造器新增 `parallelism` 参数（默认 1 = 现状顺序，兼容既有调用）。开关开 = `Executors.newFixedThreadPool(3)`（写并行度定值 3，**每实例自建**，与调度 worker 分离；执行模型见 §3.1）；开关关 = 现状单线程路径原样保留（仅换 registry/聚合器两处注入，§3.1）。PlayerActivity 按开关传 1/3 | DownloadEngine 两条 OOM 教训红线（§0.4）——固定池线程数恒定、cancel/finally 双路 shutdown 回收；同时至多一个实例在跑（start 前 cancel 旧实例，PlayerActivity.java:1350-1354），稳态 ≤3 写连接 + 1 调度线程，低于下载引擎 12 连接先例；3 路并发对 IO 密集分片约 2-3 倍吞吐，再高开始与播放器自身分片下载抢带宽 | cachedThreadPool；跨实例共享/静态池；并行度可配置 UI；参数热更；自动重试 |
| D② | **P1 用户开关**：默认**关** = 与现状逐值同语义的顺序写（零行为变化保底）。UI 入口 = **更多菜单一行切换项**「并行缓冲： 关」⇄「并行缓冲： 开」（点击翻转 → 落盘 → toast「并行缓冲已开启： 3 路并发写缓存」/「并行缓冲已关闭」），展示条件与「缓存缓冲」同支（在线动态列表且当前集非本地且非直播，PlayerActivity.java:1152 口径）。持久化 = 播放器侧 SharedPreferences 文件 `"jerocine"`（与 ad_filter 同文件先例，PlayerAdFilterHelper.java:34），键 `"buffer_parallel_enabled"`（boolean，默认 false），**唯一声明处 = PlayerActivity 顶部常量**（唯一读写方，注释钉死禁止第三处硬编码） | JcDialog **没有开关行形态**：checkable 只是左侧单选圆圈静态绘制、无 Switch 控件（JcDialogRowAdapter.java:23,70-76），往缓冲时长弹窗塞开关需 custom 插槽整块自绘或扩两壳共用组件（回归面大、踩主题铁律）；更多菜单是既有 list 形态，翻转文案零组件改动、D-pad 焦点即行焦点。prefs 双文件名并存（"jerocine" 播放器侧 / "jerocine_prefs" 壳层 ShellPrefs.java:16），沿用播放器侧文件即零新增口径 | JcDialog 扩展开关行；开关放缓冲时长弹窗内；硬编码第三处 prefs 文件名/键名；进壳层 ShellPrefs |
| D③ | **P1 进度口径**：target = 窗口分片时长和（与现状 :189-191 同口径），cached = **已完成**分片时长和；新纯类 `BufferProgressAggregator` 聚合分片完成事件（按窗口序号**去重**、**单调不减**、线程安全），顺序/并行两模式统一走聚合器；UI toast 文案/频率不变（PlayerActivity.java:1368-1376），但**进度消费点补实例身份守卫**（该 lambda 现无守卫——并行/失败拖尾下，取消后的迟到「缓存缓冲 X%」5s toast 会覆盖「已取消/失败」终态 toast，D⑨② 落到此处，§3.1） | 顺序累加天然单调，并行完成乱序后必须经聚合器保单调+去重（重复上报不虚增）；纯类无 Android 依赖，满足「新纯逻辑抽 JVM 可测类」拍板 | 字节级进度；进度平滑/抑制回退的 UI 处理；进度 UI 改版（仍 centerToast） |
| D④ | **P1 cancel 集合化**：`activeWriter` 单引用 → 新纯类 `BufferWriterRegistry`（register/unregister/cancelAll，经 `Cancellable` 抽象承载 CacheWriter）。cancel() = cancelled 置位 + `registry.cancelAll()`（掐**所有**在途）+ 全部池 shutdownNow；单片 IOException → **置 volatile failed 标志** + cancelAll 掐在途，**每个分片任务入口先查 `cancelled || failed`，命中即刻跳过并落 latch**（§3.1——排队未跑的任务不许继续写完全窗口、失败后不许再有进度 post），再按现状文案 onError「分片 x/N 缓冲失败」（x/N = **窗口内 1-based 序号** `i-from+1`/`end-from`，对齐 BufferPrefetcher.java:218）；完成回调仍只从调度 worker post 主线程、**恰好一次**（§3.1 收尾优先级：cancelled > 首个失败 > onComplete） | 并行下多个在途 CacheWriter，单引用只掐一个；registry 里只有已 register 的在途者，**排队任务必须靠入口查标志短路**，否则首片失败后其余排队任务照常依次写完全窗口（30 分钟档 onError 迟到几十分钟）；「已写分片留在缓存无害」口径不变；CacheWriter.close 顺手关 upstream 的脆约定（:197-201）决定在途者必须显式 cancel 而非放任；registry 经接口抽象可 JVM 单测；单个 cancel 抛异常吞掉继续其余（对齐 :236-239） | 单片自动重试；失败后放任在途/排队任务写完（拖尾不可控）；失败路径 shutdownNow 甩掉排队任务（latch 永不落、onError 反而迟到）；回调改同步；取消双写恢复播放（仍走实例身份守卫统一收尾） |
| D⑤ | **P2a 高峰窗纯函数**：新类 `PeakWindow.isPeak(long epochMillis, TimeZone zone)` —— 每日 **[19:00, 23:30)** ∪ 周六/周日 **[09:00, 23:30)**（**左闭右开**），按注入 `TimeZone` 的本地墙上钟计算（`Calendar.HOUR_OF_DAY/MINUTE/DAY_OF_WEEK`）；生产传 `TimeZone.getDefault()` = 本机时区。**禁用 java.time（ZoneId/LocalTime 等，API 26+）** | **minSdk 21 且全仓无 coreLibraryDesugaring**（player-core/build.gradle:44；grep 全部 build.gradle 仅 :94 注释提及、零依赖行），而 build.gradle:93-96 把 NewApi 提为 fatal 只在 lintVitalRelease 拦（2026-10-15 事故：java.util.function/List.sort 在坚果投影 Android 6 上 NoClassDefFoundError）——`ZoneId.systemDefault()` 会在 initPlayer 装配时崩播放页，且 §8 门禁（单测+assembleDebug）不跑 lint 发现不了；仓内同类禁用先例注释 DownloadQueueOrder.java:31、JcSettingsDrawer.java:674（grep player-core/tvshell main 源码 java.time/ZoneId 零命中）。纯函数可 JVM 单测且边界可枚举（§4.3）；左闭右开消除 23:30:00 端点歧义；时区显式注入让「本机时区」语义可测而非藏进实现 | 节假日/工作日历；用户可配时段；按服务端时钟；跨日合并窗口（23:30 后不顺延）；**引入 coreLibraryDesugaring**（越 D⑩ 零依赖/不动构建水位线红线，如需另行拍板） |
| D⑥ | **P2a 高峰档参数**：新纯类 `PlayerLoadControlPolicy.resolve(boolean localMode, boolean peak)` 输出四值——非高峰在线 = **{30_000, 120_000, 1_500, 2_500}**（逐值 = 现状，golden 锁定）；高峰在线 = **{60_000, 150_000, 3_000, 5_000}**（min 30s→60s，max 120s→150s，起播 1.5s→3s，重缓冲 2.5s→5s）；本地文件模式恒现状值（peak 不影响）；backBuffer 30_000 不动。PlayerActivity **两处装配收敛为调用该策略**（消灭 :338-342 / :400-405 两份逐字副本），装配时 `Log.i("JcLoadControl", ...)` 输出 mode/peak/四值供真机核对；高峰判定在**装配时刻取一次，会话内不热切换** | 数值理由：① 起播 1.5→3s——高峰源站吞吐低，1.5s 首缓冲极易「起播即 rebuffer」（点开就转圈比多等 1.5s 更伤），3s ≈ 多攒 1-2 个分片；② 重缓冲 2.5→5s——高峰卡顿多因带宽跌破码率，2.5s 存量几秒耗尽会连环卡，5s 给吞吐回升留窗口；③ min 30→60s——稳态存量翻倍吸收高峰短时抖动，`prioritizeTimeOverSizeThresholds(true)` 下按时间优先成立，2-6Mbps 码率下 ≈15-45MB，1GB LRU 可容纳；④ max 120→150s——给 60s 稳态留 2.5 倍余量，**刻意不上 180s**：`setPrioritizeTimeOverSizeThresholds(true)`（:340/:403）下字节阈值不构成闸门，缓冲样本全在 **Java 堆**，max 放大即堆顶格同步放大（6Mbps 下 120s≈90MB / 150s≈112MB / 180s≈135MB）——全仓三份 AndroidManifest 无 `largeHeap`（grep 零命中），DownloadEngine.java:1255-1263 有低配机 OOM 事故先例（坚果投影 Android 6 画像），高峰档与 30 分钟缓冲预取同会话叠加（磁盘 LRU + 并发连接争抢）时留保守余量；⑤ backBuffer 与高峰正向卡顿正交不动。非高峰 golden 测试 = 「非高峰与现状完全一致」的机器可查凭据；收敛副本消「改一漏一」分叉；热切换需重建 LoadControl + re-prepare，复杂度与收益不成比——跨峰边界（如 23:30 前后）的会话沿用装配时档位，**记为已知限制** | 本地文件高峰档；运行中热切换；backBuffer 改动；ABR/轨道选择联动；自适应拥塞控制；按设备 RAM 动态分档（不引 ActivityManager 探测，保守定值已覆盖低配画像） |
| D⑦ | **P2b 探测引擎**：新类（暂名 `PlayerSourceSpeedTest`）**起播完成后延迟 5s 后台探测**（完全不阻塞起播路径）：每源取「切过去将播放的那一集」URL（按 PlaybackTarget 钳位口径 `min(curEp, size-1)`，消「被测的集 ≠ 将播的集」）→ GET（**UA 每请求显式** `"Jerocine/1.0 (Android TV)"`）→ master 下钻 ≤2 层（复用 `DownloadEngine.firstVariantUrl` + `PlayerUrls.hasMediaSegments`）→ 计「媒体清单 GET 完成」耗时；并行 ≤4（`newFixedThreadPool(min(4, 源数))`）；单源 `callTimeout(3s)` + `connectTimeout(3s)` + `retryOnConnectionFailure(false)`（计时保真）；**探测 client 新建但 TLS 与播放侧同款**（trust-all X509TrustManager + hostnameVerifier 放宽，镜像 `ApiClient.mediaOkhttp` 的传输策略 ApiClient.kt:44-76；不能 newBuilder 复用——其浏览器 UA 拦截器摘不掉，而播放侧 buildMediaHttp :640-653 经注入 mediaClient（ServiceLocator.kt:171）继承的正是该 TLS）；**进程级节流+熔断**（新纯类 `SourceProbeThrottle`，§3.4/§4.6）：距上次探测 <30 分钟跳过、连续 2 轮探测全败 → 进程内禁探；探测批结束即 shutdown 池与 client（对齐 :241-243 泄漏教训）；结果仅会话内存，不落盘不上报 | 异步 + 5s 延迟错开起播带宽高峰；3s 上限 = 死源快速放弃；≤4 为指令拍板上限、实际 min(4, 源数)（web 侧先例实为**限并发 2**，PlayView.vue:622-23 原文「全并行时各线路争抢用户带宽，计时互相拖慢，排名失真」——防失真的教训一致，数值按指令取 4）；清单口径而非分片口径 = 流量小、3s 内必出结果、已足够反映源站健康度/RTT；**TLS 必须与播放同款**：老设备（Android 6，CA 库停在 2015）不信任现代根证书（LE ISRG X1 等，ApiClient.kt:40-48 注释）——探测若用默认 client 会「播放正常而探测全败」，恰触发 D⑧「当前源失败→切最快成功者」把好端端的会话切走；节流+熔断兜底频率/UA 敏感源（豆瓣 418 反爬先例 ApiClient.kt:63-74），长追剧每集 recreate 也不至于反复轰炸 | 分片级下载测速；多轮采样平均；任何新依赖；结果持久化与服务端回传（PlayLatencyWeb 与排序只认 PlayLatencyMs 语义错位，见勘察风险）；详情页测速 UI；手动触发入口；对敏感源的每集重复轰炸（节流+熔断兜底） |
| D⑧ | **P2b 择优与自动默认**：新纯类 `SourceSpeedRanker.pickBest(results, currentIndex)`：全失败 → -1；成功者中耗时最小者胜；**当前源成功时最优须快超过 20%（严格 `< current × 0.8`，恰 0.8×current 不切）** 才返回最优，否则 -1；当前源失败且存在成功者 → 最快成功者；最优 == 当前 → -1。自动切换编排（PlayerActivity）：探测完成后**守卫全过**才切换——① `sourceList.size() >= 2`；② `!currentBypassFilter`；③ `!isLive`；④ **用户未手动选源**（新标记 `PlayerSession.sourceManuallyPicked`，在 showSourceDialog 与更多菜单「换源播本地」两处置位，PlayerDialogHelper.java:137-145 / PlayerActivity.java:1181）；⑤ `!bufferInProgress`；⑥ 当前集**无任何本地副本**（`isEpisodePlayingLocal` 与 `otherSourceHasEpisode` 均 false）；⑦ ranker 返回 ≠ -1。切换**复用 showSourceDialog 同一执行序列**（`PlaybackTarget.switchSource` → 先写 currentSourceIndex → loadSourceIntoPlayer，PlayerDialogHelper.java:138-144），保留集号与进度续播，toast「已自动切换到更快的「X」源」；探测与决策全程仅 logcat（tag `SourceSpeedTest`），每 Activity 会话至多自动切一次 | 「只用于未手动选源时的自动默认，永不覆盖手动选择」逐字落地；switchSource 同序保证 refreshLocalDownloads 按新源查（§0.3 铁律）；0.8 边际抗单次采样噪声（单会话一次性决策，无振荡面）；本地副本不可被网络测速超越（本地优先不受影响）；bufferInProgress 时放弃避免与缓冲写争抢带宽+计时失真；守卫在**探测完成时重查**（5s 延迟 + 探测期间用户可能已手动操作） | 覆盖/回退用户手动选择；自动切回原源；多次重测；服务端排序联动 |
| D⑨ | **既有契约保全**（任何批次不得触碰）：① **一实例一次 start**（BufferPrefetcher.java:88-91）+ 每次缓冲 new 新实例（PlayerActivity.java:1354）；② **实例身份守卫**（`startedInstance` 回调首行比对，:1360/:1380/:1388）——新增回调消费点（**含进度消费点** :1368，现状无守卫）必须先过守卫；③ **取消链**：onPlaylistReloaded → cancelBufferPrefetch（:1204-1207）/ toggleLocalOnline 先 cancel（:1216）/ onDestroy cancel（:895）——自动切源走 loadSourceIntoPlayer 天然继承同链；④ **UA 三处同串** MEDIA_UA（PlayerActivity.java:114 = BufferPrefetcher.java:50 = 新探测 client）；⑤ **每片独立** OkHttpDataSource/CacheDataSource（:197-201）——并行下更禁止共享；⑥ **cancel 必须 shutdownNow 全部池**（:241-243，每漏一处 = 每次缓冲泄漏永久线程） | 全部是第 2 轮评审/事故钉死的语义：改选缓冲 #2 时 #1 的 onError 时序靠身份守卫兜底（:1356-1359 注释）；泄漏教训；UA 不一致 = 预取/探测结果与播放实际不一致 | 池跨实例复用；契约参数热更；回调改同步 |
| D⑩ | **范围红线**：本批只动 `android/player-core` 一个模块（不含其 build.gradle 依赖清单——零新依赖）；相对窗口语义（minutes/startOffsetSec，BufferPrefetcher.java:83-86，2026-11-08 拍板）与 `startIndexForOffset`/`rangeForMinutes` **原样不动**（顺序/并行共用）；不动 web 播放器（PlayView/ArtPlayer/usePlaySpeedTest/EpisodeTabs）；不动服务器（零 Go 改动）；不做自适应拥塞控制；不改 APK 构建水位线（scripts/android-versions.properties 与打包脚本）；不动 PlayerPrefetchHelper 自动下一集预取 | 控制回归面：相对窗口是用户刚拍板的语义；拥塞控制是独立大坑；构建水位线与本分支无关；预取与缓冲虽同域但口径独立、互不感知 | 即左列全部（逐条 = 不做） |

## 2. 契约保全清单（引用 D⑨，评审按此逐条核对）

见 §1 D⑨——六条逐字保持，实施 PR 描述需逐条自检勾选。

## 3. 关键设计细则

### 3.1 BufferPrefetcher 并行执行模型（D①/D③/D④）

```
构造: BufferPrefetcher(cache, parallelism)   // 旧单参构造保留, 委托 (cache, 1)
     worker    = newSingleThreadExecutor()                       // 调度线程(现状角色)
     writePool = parallelism >= 2 ? newFixedThreadPool(parallelism) : null

顺序档(parallelism=1, 默认): 现状 run() 结构原样 —— worker 内联 pre-work + 逐片写。
  仅两处替换: activeWriter 单引用 → registry 登记/注销; cachedSec 累加 → 聚合器。
  观测行为与改前逐值一致(单测 + 顺序语义双保险)。

并行档(parallelism=3): worker 做 pre-work(fetch/过滤/inspect/窗口计算, :131-191 原样)
  → 每片一个任务提交 writePool → CountDownLatch await 全部落幕
    (单片失败: 置 volatile failed 标志 + registry.cancelAll() 掐在途 → 在途 CacheWriter 快速退出;
     **每个分片任务入口先查 cancelled||failed, 命中即刻跳过并落 latch 返回** —— registry 只有
     已 register 的在途者, 排队未跑的任务全靠入口查标志短路: 既不会"首片失败后其余排队任务
     照常依次写完全窗口"(30 分钟档 onError 迟到几十分钟), 也没有失败后仍持续上涨的进度 toast;
     **不得用 shutdownNow 甩掉排队任务**——被甩的任务不落 latch, await 反而挂死)
  → 收尾判定基于 volatile failed/cancelled, 与任务结束顺序解耦
  → worker 收尾, 回调仍只从 worker post 主线程、恰好一次, 优先级:
      cancelled      → onError("已取消")          // 对齐 :224-227
      首个失败       → onError("分片 x/N 缓冲失败: …")  // x/N = 窗口内 1-based 序号 i-from+1/end-from, 对齐 :218
      全部成功       → onComplete()               // 对齐 :228
  cancel(): cancelled + registry.cancelAll() + worker.shutdownNow() + writePool.shutdownNow()

生命周期: writePool 随实例生灭(构造建 / finally shutdown / cancel shutdownNow),
  与 worker 同轨; 无跨实例状态。单分片写体(pre-work 后的部分)两档完全共用。
```

不变量：目标进度 target 计算不动（:189-191）；UA 不动（:50）；每片独立 DataSource 不动（:202-206）；「已写分片留在缓存无害」不动；「cancelled 分支挡不住恢复播放、靠身份守卫」的分工不动（:104-108）。**进度回调消费点（PlayerActivity.java:1368-1376 的 onProgress lambda，现状无守卫）补实例身份守卫**：首行 `bufferPrefetcher != startedInstance` 直接 return（D⑨②）——取消/失败后的迟到进度 toast 不再覆盖终态 toast。

已知边缘（既有行为，非本批引入，本批不修）：cancel() 的 worker.shutdownNow() 若发生在 start() 提交的任务被调度前，任务被池丢弃且永无回调 → bufferInProgress 卡 true、播放停在暂停态（用户手动按播放可解）。单线程现状同样存在（BufferPrefetcher.java:95 提交与 :243 shutdownNow 的竞态），并行模型不改变该面；修复需跨实例回调对账/启动超时兜底，越本批范围。

### 3.2 新纯类 API（无任何 Android import，落 `com.jerocine.player`）

```java
/** 窗口进度聚合: 乱序完成仍单调; 重复窗口序号只计首次; 线程安全。JVM 可测。 */
public final class BufferProgressAggregator {
    public BufferProgressAggregator(double targetSeconds)          // target = 窗口分片时长和
    public synchronized void onSegmentDone(int windowIndex, double durationSeconds)
    /** cached = 已完成分片时长和, **按窗口序号升序重算**(非到达序累加) —— 消除乱序下
     *  double 求和的非结合性: 任意完成顺序结果逐位一致, 全完成时与 target 精确相等
     *  (exact assertEquals 合法, PlayerActivity.java:1369 的 pct 取整对 ulp 级尾差本就无感)。 */
    public synchronized double cachedSeconds()
    public double targetSeconds()
}

/** 在途写登记处: cancel 要掐所有在途, 而非单个。经 Cancellable 抽象可 JVM 单测。 */
public final class BufferWriterRegistry {
    public interface Cancellable { void cancel(); }                // CacheWriter::cancel 适配
    public void register(Cancellable w)
    public void unregister(Cancellable w)                          // finally 里的对称注销
    public void cancelAll()   // 掐所有; 单个抛异常吞掉继续其余(对齐 :236-239); 幂等; 空集安全
}
```

### 3.3 P2a 判定点与参数表（D⑤/D⑥）

| 档位 | minBuffer | maxBuffer | bufferForPlayback | bufferForPlaybackAfterRebuffer | backBuffer |
|---|---|---|---|---|---|
| 非高峰在线（= 现状 = 本地恒定） | 30_000 | 120_000 | 1_500 | 2_500 | 30_000 |
| 高峰在线 | 60_000 | 150_000 | 3_000 | 5_000 | 30_000 |

- 判定点：`initPlayer` 装配时 `PeakWindow.isPeak(System.currentTimeMillis(), TimeZone.getDefault())` 取一次（D⑤：java.util.TimeZone/Calendar 口径，禁 java.time）；`initLocalPlayer` 走 `resolve(true, …)` 恒现状。两处 `setBufferDurationsMs` 位次不变 (min, max, playback, rebuffer)。
- 装配 `Log.i("JcLoadControl", "mode=" + … + " peak=" + … + " buffer=" + …)` 一行供真机核对。

### 3.4 P2b 探测与自动默认（D⑦/D⑧）

- 触发：PlayerActivity 首次装载完成后（onCreate :308 与 onNewIntent :942 两个入口均可为「首次」，`hasProbed` 标记去重）初筛守卫（≥2 源 / !bypass / !isLive / !localPlayback）通过 → `postDelayed(probe, 5_000)`；onDestroy `removeCallbacksAndMessages(null)`（:899 已有同款清理）。
- 节流/熔断（**进程级**，新纯类 `SourceProbeThrottle`）：`hasProbed` 是实例标记，onNewIntent 的 recreate() 路径会重置实例字段——节流以进程级静态「最近探测时间」为准（距上次 <30 分钟跳过，换集/重建不至每集对全部源各轰 1-3 个无 Referer 清单 GET）；连续 2 轮探测全败 → 进程内禁探至进程重启（频率/UA 敏感源兜底，豆瓣 418 反爬先例 ApiClient.kt:63-74）。判定逻辑纯函数可单测（§4.6）。
- 探测：全部源（**含当前源**，margin 比较需要）并行 ≤4、单源 ≤3s；每源结果 `{sourceIndex, ok, elapsedMs}`；client 用完即 shutdown。
- 择优与切换：`SourceSpeedRanker.pickBest` → 守卫**在探测完成时重查**（含 sourceManuallyPicked / bufferInProgress / 本地副本）→ 复用 switchSource 序列切换 → toast；决策全程 logcat `SourceSpeedTest`（每源一行结果 + 决策一行含原因）。
- `PlayerSession.sourceManuallyPicked`（volatile boolean，默认 false）：仅在两处用户显式动作置位——showSourceDialog 点击他源（PlayerDialogHelper.java:137-145）、更多菜单「换源播本地」（PlayerActivity.java:1181-1182）。自动切换本身**不置位**（不抢夺用户后续手动权）。

### 3.5 UI 与文案（D②，中文）

- 更多菜单（在线动态列表支）：「下载管理」→「缓存缓冲」→（缓冲中追加）「取消缓冲」→ **「并行缓冲： 关/开」**（仅与「缓存缓冲」同支展示）→（别源已下载）「换源播本地」→「诊断信息」。
- 文案定稿风格对齐既有菜单项（中文、无句号、冒号用全角）；toast 文案见 D②。
- 探测/择优零 UI（完全后台），不新增任何弹窗（新弹窗一律走 JcDialog 的铁律因此天然满足）。

### 3.6 持久化（D②）

- 文件 `"jerocine"`（`MODE_PRIVATE`，与 PlayerAdFilterHelper.java:34 同文件不同键）；键 `"buffer_parallel_enabled"`；唯一声明处 = PlayerActivity 常量（唯一读写方），注释对齐 ShellPrefs.java:3-13「键名只在此定义一次」口吻。

## 4. 测试清单（TDD：每条先 red → green → refactor；落点 = 新测试类 + 被测生产类）

| # | 测试类（player-core `src/test/java/com/jerocine/player/`） | 关键用例（red 先行） | 被测类（`src/main/java/com/jerocine/player/`） |
|---|---|---|---|
| 1 | `BufferProgressAggregatorTest` | 乱序完成序列输出单调不减；乱序完成与升序重算**逐位一致**（double 非结合性消解，口径见 §3.2）；重复窗口序号只计一次；target 恒等于窗口时长和；全完成后 cached==target（**exact assertEquals 合法**——升序重算使终值与 target 同序求和）；duration=0 的分片合法；并发线程争调（多线程 hammer 后终值正确） | `BufferProgressAggregator`（新，D③） |
| 2 | `BufferWriterRegistryTest` | register→cancelAll 掐所有；unregister 后不再被掐；单个 cancel 抛异常不影响其余；空集/重复 cancelAll 幂等安全 | `BufferWriterRegistry`（新，D④） |
| 3 | `PeakWindowTest` | 平日 18:59:59.999=false / 19:00:00=true / 23:29:59.999=true / 23:30:00=false；周六日 08:59:59.999=false / 09:00:00=true / 周日 23:30:00=false；**周一 09:00=false**（每日窗 19:00 起）；同一 epoch 换 TimeZone（Asia/Shanghai vs UTC）结论翻转（时区可测）；周末窗不越午夜 | `PeakWindow`（新，D⑤；**java.util.TimeZone/Calendar 口径，禁 java.time**） |
| 4 | `PlayerLoadControlPolicyTest` | **golden**：(!local, !peak) == {30000,120000,1500,2500}（锁现状 = 「非高峰与现状完全一致」凭据）；(!local, peak) == {60000,150000,3000,5000}；(local, peak 真/假) 均恒现状值；backBufferMs()==30_000 | `PlayerLoadControlPolicy`（新，D⑥） |
| 5 | `SourceSpeedRankerTest` | 全失败→-1；成功者最快者胜；当前成功时**严格 `<0.8×current` 才返回，恰 0.8×current 不切**；当前失败且有成功者→最快成功者；最优==当前→-1；空表→-1 | `SourceSpeedRanker`（新，D⑧） |
| 6 | `SourceProbeThrottleTest` | 冷启动（无记录）允许；距上次探测 <30 分钟跳过；≥30 分钟允许；连续 1 轮全败后（≥30 分钟）仍允许；**连续 2 轮全败 → 任何时间禁探**（进程级熔断）；若出现成功则全败计数清零 | `SourceProbeThrottle`（新，D⑦） |
| 7 | 既有回归 | PlaylistSegmentsTest / M3u8FilterCoreTest / PlayerModesTest 等保持绿——窗口计算（:174-187）与相对窗口语义 D⑩ 钉死不动，顺序/并行共用 | 既有生产类（零改动） |
| 8 | **不可 JVM 直测面（如实标注，按 player-fix-spec §2 注记先例，以码审 + §6 真机清单覆盖）** | BufferPrefetcher 并行执行本体（Handler/Looper/线程池/latch/失败标志短路时序）、PlayerActivity 开关读写与菜单装配、**进度/完成回调消费与身份守卫落地**、自动切换编排、探测引擎网络行为、sourceManuallyPicked 置位链 | —（依赖 android 运行时，仓内无 Robolectric） |

## 5. 分阶段实施计划（P1 → P2；批次内 TDD，每批次一笔 commit）

| 批次 | 内容 | 提交 |
|---|---|---|
| **P1-A 纯逻辑**（零风险起步，验门禁链路） | A1 BufferProgressAggregator red→green；A2 BufferWriterRegistry red→green（§4.1/4.2） | `feat(android): 缓冲进度聚合器与在途写注册表(纯逻辑, JVM 单测)` |
| **P1-B 引擎+UI** | B1 BufferPrefetcher：构造器 parallelism + registry/聚合器接入 + 并行路径（§3.1，顺序档观测行为逐值不变）；B2 PlayerActivity：prefs 常量与读写 + 更多菜单「并行缓冲」行 + 按开关传 1/3 + **进度回调消费点补实例身份守卫**（§3.1/D③）；门禁三模块单测 | `feat(android): 缓存缓冲并行写分片(固定池3, 默认关)+并行缓冲开关` |
| **P2-a 高峰水位** | C1 PeakWindow + PlayerLoadControlPolicy red→green（§4.3/4.4）；C2 PlayerActivity 两处装配收敛走策略 + peak 判定 + Log.i；门禁 | `feat(android): 高峰水位放大缓冲参数(非高峰 golden 锁现状, 两处副本收敛)` |
| **P2-b 源测速择优** | D1 SourceSpeedRanker red→green（§4.5）；D2 探测引擎 + PlayerSession.sourceManuallyPicked 置位（两处）+ 自动切换编排（复用 switchSource 序列）；门禁 | `feat(android): 后台源测速择优(不阻塞起播, 永不覆盖手动选择)` |
| 评审修复 | 每轮独立 commit | `fix(android): buffer-perf 第N轮评审修复` |

提交链预期：

```
docs(specs): 缓冲性能规格落盘(P1 并行缓冲/P2a 高峰水位/P2b 源测速择优)   ← 本笔
feat(android): 缓冲进度聚合器与在途写注册表(纯逻辑, JVM 单测)            ← P1-A
feat(android): 缓存缓冲并行写分片(固定池3, 默认关)+并行缓冲开关           ← P1-B
feat(android): 高峰水位放大缓冲参数(非高峰 golden 锁现状, 两处副本收敛)    ← P2-a
feat(android): 后台源测速择优(不阻塞起播, 永不覆盖手动选择)               ← P2-b
fix(android): buffer-perf 第N轮评审修复                                  ← 逐轮(如需)
```

## 6. 不做清单（全局红线，汇总自 D①~D⑩「不做」列）

1. **不动 web 播放器**（PlayView/ArtPlayer/usePlaySpeedTest/EpisodeTabs）——本批改动面仅 `android/player-core`。
2. **不动服务器**（零 Go 改动；测速结果不回传、不接 PlayLatency 通道、不做服务端排序联动）。
3. **不改相对窗口语义**（BufferPrefetcher.java:83-86 的 minutes/startOffsetSec 口径与 :174-187 窗口计算原样）。
4. **不做自适应拥塞控制**（不引带宽估计/动态并发/ABR 参数调整；高峰档 = 静态二档）。
5. **不改 APK 构建水位线**（scripts/android-versions.properties 与打包脚本零改动）。
6. **不扩 JcDialog 组件**（无开关行）；**不新增第三处 prefs 文件名/键名硬编码**。
7. **不动 PlayerPrefetchHelper** 自动下一集预取及其短路链。
8. 并行度不做可配置 UI；单片失败不自动重试；LoadControl 不做运行中热切换（跨峰会话沿用装配时档位，已知限制）。

## 7. 铁律（全部批次与评审适用）

1. **不 push、不部署**；每批次一笔 commit（`feat(android)`/`refactor(android)`/`fix(android)`/`docs(specs)`，中文主题）；e2e 不写不跑。
2. 分层提交：纯逻辑类（P1-A/C1/D1）与引擎改造（B1/D2）与 UI（B2）不混笔，便于回滚定位。
3. 播放器弹窗/菜单一律走 `JcDialog`（CLAUDE.md 播放器弹窗铁律）；本批新增仅更多菜单列表行，不碰主题/布局 token，不写 `?attr/`。
4. 门禁不过或规格与实情矛盾：如实上报（escalate），不得造假绕过。

## 8. 门禁（与脚本一致）

```bash
cd <worktree>/android && ./gradlew :player-core:test :tvshell:test :app:testDebugUnitTest
```

- 与 scripts/build-android.sh:324-328 `run_gradlew()`（`cd "$dir" && ./gradlew "$@"`）同形态；每批次提交前必跑全绿，commit body 附实跑凭据（对齐 player-fix-plan 门禁凭据补记惯例）。
- 终验（触碰播放器装配的批次）：`cd <worktree>/android && ./gradlew assembleDebug`。
- 新逻辑必须带单测（§4）；不可 JVM 直测面如实标注（§4.8），不作假测试。
- 本笔 docs 提交为纯文档，不跑门禁。

## 9. 真机验收清单（交付给用户，本环境不执行）

1. **开关**：更多菜单出现「并行缓冲： 关」；点击翻转 + toast；重进播放器记忆；默认关时缓冲行为与改前一致（5 分钟 → 0→100% → 缓存完成 → 断网重播可离线）。
2. **并行收益**：同源同时段对比，并行开「缓冲 30 分钟」耗时可感知短于关；进度 toast 单调不回退、不闪跳。
3. **取消**：并行开进行中点「取消缓冲」立即停（logcat 无残留分片写、无新线程增长），已写部分保留；**取消/失败后无迟到的进度百分比 toast 覆盖终态提示**（进度消费点守卫，§3.1）；切源/切本地/退出播放页在途缓冲自动取消（既有链回归）。
4. **改选**：缓冲 #1 途中改选 #2：无误 toast、#2 暂停语义保持（实例身份守卫回归）。
5. **高峰水位**：19:00-23:30 起播 logcat `JcLoadControl` 显示 60000/150000/3000/5000；其余时段显示 30000/120000/1500/2500；本地文件播放恒后者；周六日 09:00-23:30 同高峰档。
6. **源测速**：双源影片起播后 ~10s 内 logcat `SourceSpeedTest` 每源一条结果 + 一条决策；用户手动切源后无自动切换；单 URL/单源/直播/本地模式/别源已下载 均不自动切；自动切换发生时从原进度续播且 toast 提示。
7. **回归**：本地文件播放、下载管理、进度条、倍速、下一集预取均不受影响；`:tvshell` / `:app` 单测全绿（两壳共用层无行为漂移）。
