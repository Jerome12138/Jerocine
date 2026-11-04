# 安卓播放器修复 TDD 计划 — player-fix

> 配套规格:docs/specs/player-fix-spec.md(Q①~Q⑪ 指向该文件)。批次顺序 = 依赖与风险分层。

## 批次PA:缓存缓冲修复(最小风险起步,验证 gradle 门禁链路)

| 步骤 | 内容 |
|---|---|
| PA1 | PlayerActivity.java: startBufferPrefetch 内 cancel 后补 `bufferPrefetcher = new BufferPrefetcher(sCache);`;:1276 catch 加 Log.w(类名+异常);若 PlayerActivity 有对应逻辑测试则补用例 |
| PA2 | BufferPrefetcher.java:48 UA → `"Jerocine/1.0 (Android TV)"` |
| PA3 | 门禁:android 三模块单测;提交 `fix(android): 缓存缓冲补回实例化(NPE)+UA 对齐+失败日志` |

## 批次PB:直播根因修复(Q①Q②Q⑤,播放器核心,单独提交便于回滚)

| 步骤 | 内容 |
|---|---|
| PB1 | **先核对 media3 实际 API**(gradle 缓存源码):HlsDataSourceFactory 接口与 HlsMediaSource.Factory(HlsDataSourceFactory) 构造存在性;不符即 escalate |
| PB2 | 实现 manifest 免缓存:自定义 HlsDataSourceFactory,`C.DATA_TYPE_MANIFEST` 返回 DefaultDataSource(OkHttp upstream),其余 type 走现 CacheDataSource 链;替换 PlayerActivity.java:361-363 |
| PB3 | onPlayerError 兜底(:454 起):behind-live/reset 类 → seekToDefaultPosition+prepare,限一次/短窗防循环(记重试计数);其余路径不变 |
| PB4 | 直播媒体表判定纯函数(无 ENDLIST 且有 MEDIA-SEQUENCE)+ 单测;接入 M3u8FilterCore 调用点跳过剔段(只 absolutize) |
| PB5 | 门禁:三模块单测;提交 `fix(android): 直播清单免缓存+behind-live 兜底+直播表跳过本地过滤` |

## 批次PC:isLive 贯通与直播体验(Q③Q④Q⑥Q⑦,跨端)

| 步骤 | 内容 |
|---|---|
| PC1 | isLive 通道:LivePlayer.kt buildLivePlayerPayload 加 isLive=true;nativeLive.ts 派发载荷加 isLive;PlayerSourceHelper 解析 EXTRA → PlayerSession.isLive 字段(含类型/契约注释) |
| PC2 | UI:控制面板显隐处若 isLive/isCurrentMediaItemLive → 隐藏进度行、屏蔽 ±10s seek;isLive 时 canPrefetch 恒 false |
| PC3 | TV loadAll 并行(LiveRepository.kt:16-23,coroutine 并行+顺序稳定);openLive 前置 GlassToast「正在进入直播…」 |
| PC4 | 单测:载荷解析 isLive、并行 loadAll 顺序、canPrefetch 短路;web 侧 nativeLive 相关 vitest(如有) |
| PC5 | 门禁:android 三模块单测 + web vitest;提交 `feat(android): 直播 isLive 贯通(隐藏进度条/禁seek/短路预取)+频道并行+进入反馈` |

## 批次PD:服务端与文档(Q⑨Q⑩)

| 步骤 | 内容 |
|---|---|
| PD1 | server/internal/live/client.go 上游超时 20s→8s;确认相关 go 单测 |
| PD2 | CLAUDE.md 广告过滤/预取段落改现状(标注 2026-10-28 变更与日期);PlayView.vue:451 注释修正 |
| PD3 | 门禁:go test ./...;提交 `fix(server): 直播上游拉取超时 20s→8s` + `docs: 同步端侧过滤下线后的播放链路事实` |

## 三轮审查

- **R1(双评审并行)**:①播放器机制评审——PB 三项对 media3 行为的对照(清单免缓存是否波及 VOD 起播耗时/重复请求、兜底重试是否可能死循环、直播表判定误伤面)、CLAUDE.md 铁律符合性;②跨端一致性评审——isLive 双入口齐备、web/TV 行为一致、PC2 的 UI 分支不误伤 VOD。
- **R2**:逻辑回归(并发 loadAll 错误处理、NPE 修复后 BufferPrefetcher 生命周期、extra 缺省向后兼容——旧载荷无 isLive 时默认 false)。
- **R3 终验**:规格 §2 门禁全绿复核 + §4 真机清单定稿输出。

## 提交链预期

```
docs(android): 播放器修复规格与计划落盘(SDD)
fix(android): 缓存缓冲补回实例化(NPE)+UA 对齐+失败日志            ← PA
fix(android): 直播清单免缓存+behind-live 兜底+直播表跳过本地过滤    ← PB
feat(android): 直播 isLive 贯通+频道并行+进入反馈                  ← PC(web 改动并入本笔或独立 fix(web))
fix(server): 直播上游拉取超时 20s→8s + docs 同步                    ← PD
fix(android): 第N轮评审修复                                        ← 逐轮(如需)
```

## 门禁凭据补记(2026-11-05 第3轮评审)

- **PA 批次(75862c9)**:当笔 commit message 未带门禁凭据(PA3 门禁=android 三模块单测)。缓解事实:后续 f8b2907(PB)/bf47bb4(PC) 的 message 记录了同组门禁全绿(含累积代码),回归风险低;本轮终验再以 assembleDebug + 三模块单测重跑覆盖(凭据见第3轮评审修复 commit body)。
- **评审修复批(a954cb2/53080e8)**:当笔 message 同样未带凭据;自第3轮起,每笔修复 commit 的 body 附当轮实跑门禁输出(套数/例数/结果),本笔即首个载体。
- 53080e8 的 server 行为变更(RefreshSourceChannels 先抓后失效)已补两分支单测 `TestRefreshSourceChannelsCacheSafety` 并随本轮 go test 门禁落凭据。
