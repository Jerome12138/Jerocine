# danmu-service（弹幕数据服务）

Jerocine 弹幕数据服务壳：直接复用上游 **[@rexnow/scraper-kit](https://github.com/baranwang/rex-widget)**（MIT），
本目录**不含任何弹幕抓取逻辑**，只有路由/缓存/限流/单位转换的薄胶水。

架构：

```
Jerocine Go 后端 ──HTTP──▶ danmu-service (本目录, Node)
                              ├─ GET /danmu/:provider/episodes?resourceId=&episode=&episodeName=
                              ├─ GET /danmu/:provider/segments?episodeId=
                              └─ GET /danmu/:provider/comments?episodeId=&segmentId=
```

## 上游（UPSTREAM）

- 上游仓库：`https://github.com/baranwang/rex-widget`
- **pin：`0dc573e831462ba74764b23c80b8470c3fd7ab22`**（2026-10-24 "chore: version packages (#62)"）
- 快照位置：`upstream/`（仅 `packages/{utils,storage,fetch,scraper-kit}` 源码+dist + workspace 配置，
  不含 node_modules；完整仓库见 fork `qiguo093/Platform-Danmu`）
- **升级流程**：`git clone` 上游到临时目录 → 按依赖序 `pnpm --filter @rexnow/<pkg> build`
  （utils→storage→fetch→scraper-kit）→ 用新 dist 覆盖 `upstream/packages/*/dist/` → 更新本文件 pin → 重新验证 smoke
- **⚠️ 本地补丁（升级后必须重新应用）**：`scraper-kit/src/scrapers/tencent/tencent.ts` 的
  `getEpisodesPage` 在上游是 TS `private`（dist 运行时仍可调），本壳依赖它做腾讯分页
  （绕开 `selectEpisodeCandidates` 的 whole 过滤）。src 快照已去掉 `private` 并留
  `[本地补丁·Jerocine]` 标记；升级 clone 后需重新应用该行改动再 build。若漏应用，
  壳内 `listEpisodes` 有 fail-fast 守卫（缺方法 → 500 + 提示），不会静默无弹幕。

## 本地运行与验证

```powershell
# 1) 装运行时依赖（壳自身零框架依赖；上游外部依赖 crypto-js/protobufjs 等由部署构建注入）
node server.js                      # 监听 127.0.0.1:3611（HOST 环境变量可改；容器内由 compose/Dockerfile 设 0.0.0.0）

# 2) 单测（node:test 原生）
node --test test/

# 3) 冒烟（真实接口，需外网）
node smoke/smoke_bilibili.mjs       # B站：流浪地球2 seasonId=44847
node smoke/smoke_scraperkit.mjs     # 腾讯：心动的信号第九季 cid=mzc002003kpyd2m
```

## 接口契约（详见 docs/弹幕服务集成方案-2026-10-25.md §8）

| 接口 | 参数 | 返回 |
|---|---|---|
| `/danmu/:provider/episodes` | `resourceId`（裸值，壳内加前缀 `cid=`/`seasonId=`）、`episode`（集号）、`episodeName`（可选，综艺期号识别） | `{ ok, data: { provider, episodes: [...], matched: {episodeId,...} \| null } }` |
| `/danmu/:provider/segments` | `episodeId`（复合串，必须 encodeURIComponent） | `{ ok, data: [{ provider, segmentId, startTime(秒) }] }` |
| `/danmu/:provider/comments` | `episodeId` + `segmentId`（缺省 = 整集聚合） | `{ ok, data: [{ text, time(ms), color, mode }] }` |

关键约定（§8.1/8.5）：

- **timestamp 秒→毫秒是壳内唯一转换点**（上游 B站浮点秒/腾讯整数秒，`Math.round(t*1000)`）
- **fetch adapter 必须自处理 `base64Data`**（B站 seg.so protobuf 通道；`options.base64Data===true` 时返回 base64 字符串）
- 段间节流 ≥200ms、单 provider 并发 1（与 Go 侧纪律一致）
- provider 支持：`bilibili` / `tencent`（其余 iqiyi/youku/mgtv/renren 由上游支持，未在本壳启用，启用时在 `lib/scraper.js` 白名单加一行）

## 部署（D2 起）

- 镜像：`node:22-alpine`，**本机构建产物进包**（编译不上生产机，与 server 同哲学）
- 端口 `:3611` 内网 only，经 Go 网关访问；不可用/超时 → Go 静默降级"暂无弹幕"
