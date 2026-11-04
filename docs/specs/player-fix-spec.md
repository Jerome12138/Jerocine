# 安卓播放器修复规格（SDD）— player-fix

> 2026-11-05 ｜ 分支 `manage-controls`（worktree `D:\Git\JerocineHarness\.worktrees\manage-controls`，紧随管理端控件组件化批次之后）
> 证据基底：`D:\Git\JerocineHarness\docs\排查-安卓播放器-直播与缓存-2026-11-05.md`（两路只读侦察,全部结论带 file:line）
> 方法论：superpowers SDD + TDD。**本文件是唯一契约；实施前必须先读排查报告与 CLAUDE.md 的播放器相关小节。**

## 0. 背景与根因(已实锤,勿重查)

1. **直播 30s 报错**:直播清单第一次响应被 SimpleCache 永久缓存(PlayerActivity.java:624-627 的 CacheDataSource 未对 manifest 免缓存,未设 FLAG_IGNORE_CACHE_FOR_UNSET_LENGTH_REQUESTS)→ 之后每 targetDuration 轮询都从磁盘回放同一静态快照 → Media3 stuck 检测(3.5×targetDuration ≈ 21-35s)抛 PlaylistStuckException → onPlayerError(:454-527)无兜底,弹错 8s 后 finish。
2. **直播进度条**:播放器全链路无 isLive 判定;media3 直播时间线 isSeekable=true → 冻结窗口渲染成 ≈30s 假 VOD 条。
3. **直播进入慢**:TV `LiveRepository.kt:16-23` 串行逐源拉频道(web 侧 nativeLive.ts:83-92 是并行,两套不同步)× 服务端冷缓存替抓上游 m3u 超时 20s(server/internal/live/client.go:16-18)串行相乘;openLive(NativeShellActivity.kt:136-179)零加载反馈。
4. **缓存缓冲不生效**:commit 69d9efb 删掉了 `bufferPrefetcher = new BufferPrefetcher(...)` 实例化(PlayerActivity.java:1246 后无赋值)→ startBufferPrefetch 必现 NPE 被 :1276 catch 吃掉,toast「缓存缓冲启动失败: 未知错误」。BufferPrefetcher.java:48 UA 与播放侧(:616)不一致(次生)。
5. **TTL 审计结论**:全仓三端全部为「写入时刻+TTL」相对语义,**不需要改**(用户怀疑排除)。
6. ⚠️ 事实修正:服务端过滤阶梯(/m3u8/filter、/m3u8/proxy、12s 超时、中转)2026-10-28(69d9efb)已全线下线,现为端侧本地 M3u8FilterCore 过滤;CLAUDE.md 相关段落过时,本批修正。

## 1. 决策

| # | 决策 | 备注 |
|---|---|---|
| Q① | **P1 清单免缓存**:为 HlsMediaSource 换自定义 HlsDataSourceFactory——`C.DATA_TYPE_MANIFEST` 走裸 DefaultDataSource(OkHttp),分片继续走 CacheDataSource | 根因修复;动播放器装载核心;实施前**先核对本仓 media3 实际版本的 API 形态**(gradle 缓存里看 HlsDataSourceFactory/Factory 构造),不符则 escalate |
| Q② | **P2 报错兜底**:onPlayerError 对 `ERROR_CODE_BEHIND_LIVE_WINDOW` 及 cause 为 PlaylistReset/BehindLive → seekToDefaultPosition()+prepare(),不弹错退出;其余异常路径不变 | media3 标准处方 |
| Q③ | **P3 isLive 贯通**:直播载荷加 `isLive`(TV LivePlayer.kt buildLivePlayerPayload + web utils/nativeLive.ts)→ PlayerActivity EXTRA → PlayerSession;控制面板显隐时若 `session.isLive` 或 `player.isCurrentMediaItemLive()` → 隐藏进度行(exo_position/TimeBar/exo_duration)并屏蔽 ±10s seek;布局文件不动 DOM 结构只加显隐 | 双入口(TV/web 壳)都要带上标志 |
| Q④ | **P7 直播短路预取**:canPrefetch(PlayerPrefetchHelper.java:109-117)对 isLive 恒 false | 依赖 Q③ 的标志 |
| Q⑤ | **P9 直播媒体表跳过本地过滤**:媒体清单无 `#EXT-X-ENDLIST` 且含 `#EXT-X-MEDIA-SEQUENCE` → 不做 M3u8FilterCore 剔段(只 absolutize 直通) | 消除剔段对滑窗的干扰面;master 层本就直通 |
| Q⑥ | **P4 TV 频道并行**:LiveRepository.loadAll for 循环 → coroutine 并行(语义对齐 web Promise.all),保持返回顺序稳定 | — |
| Q⑦ | **P6 进入反馈**:openLive 拉频道前给 GlassToast「正在进入直播…」 | 轻提示,不加新组件 |
| Q⑧ | **P8 缓存缓冲修复**:startBufferPrefetch 在 cancel 后补 `bufferPrefetcher = new BufferPrefetcher(sCache);`(新签名单参!);:1276 catch 加 `Log.w(TAG, "buffer prefetch failed", e)`;BufferPrefetcher.java:48 UA 改 `"Jerocine/1.0 (Android TV)"` 对齐播放侧 | 一行根因修复+可观测 |
| Q⑨ | **P5 服务端上游超时**:live/client.go:16-18 的 20s → 8s | go test 门禁;预热不在本批(记建议) |
| Q⑩ | 文档与注释:CLAUDE.md「广告过滤两条链路/选路阶梯/端侧预取」段落改为 2026-10-28 后现状(端侧本地过滤,服务端阶梯已下线,保留历史结论标注日期);web PlayView.vue:451 过时注释修正 | 纯文档 |
| Q⑪ | 不做:TTL 语义改动(无需)、elapsedRealtime 加固(记建议)、变体选择对齐(观察后再做)、签名防盗链源(源站特性)、服务端预热(建议) | 控制范围 |

## 2. 门禁

- Android 单测:`bash <worktree>/android/gradlew -p <worktree>/android :player-core:test :tvshell:test :app:testDebugUnitTest`
- Android 编译:`bash <worktree>/android/gradlew -p <worktree>/android assembleDebug`(终验必跑)
- Web:`node <pnpm.cjs> -C <web> run test`(P-C 触碰 web 时)
- Server:`go -C <worktree>/server test ./...`(P-D)
- 新逻辑必须带单测(BufferPrefetcher 修复的 catch 分支、isLive 解析、直播清单判定函数用纯函数抽出便于测试)

## 3. 铁律

1. 不 push 不部署;每批次一笔 commit(`fix(android)`/`feat(android)`/`fix(server)`/`docs`);e2e 不跑。
2. CLAUDE.md 播放器铁律全部生效:控制按钮/状态点/中转过滤按钮文案恒定不动;`MENU/INFO/BACK` 键约定不动;两壳共用改动必须留真机验收项;player-core 资源唯一真源;`?attr/` 禁令。
3. 播放器核心改动(Q①/Q②/Q⑤)与 UI 改动(Q③)分层提交,便于回滚定位。
4. media3 版本以本仓实际依赖为准(1.4.1 系),API 核对不过即 escalate,禁止凭记忆写 API。
5. 遇阻如实上报,不造假。

## 4. 真机验收清单(交付给用户,本环境不执行)

1. 直播:进入播放 30s+ 不再报错;进度条/时长文本不显示;±10s seek 无效;频道切换正常
2. 直播进入耗时:冷缓存(可清服务端 Redis live 缓存)/热缓存两档,对比修复前
3. 缓存缓冲:菜单选 5 分钟 → 「缓存缓冲 x%」0→100% → 「缓存完成」;断网重播前 5 分钟离线可播
4. VOD 回归:过滤开/关各一遍(端侧过滤仍生效)、下一集预取、下载页、进度条正常显示
5. 双入口:TV 原生壳直播 + web 壳 /live 页进直播,行为一致
6. 服务端:直播频道冷缓存拉取耗时下降(日志 grep "[live]")
7. 直播唤出控制面板后,遥控器 ←/→ 应在底栏按钮行左右移动焦点(第1轮修复前会卡死在初始聚焦的那颗按钮);进度条方向键 ±10s seek 仍无效
8. VOD 同一片二次起播:清单(master+媒体表)每次走网络回源(Q① 清单免缓存,缓存收益本就小);清单偶发网络失败会直接报装载错误,需人工重试而非静默复用旧表
