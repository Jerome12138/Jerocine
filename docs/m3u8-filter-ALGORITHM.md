# m3u8 去广告过滤算法规范

> Go(`server/internal/service/m3u8_service.go`) / TS(`web/src/utils/m3u8Filter.ts`) / Kotlin
> (`player-core M3u8FilterCore.kt`) 三端共同实现本规范。任一实现与 Go 有歧义时以 Go 为真相，
> 一致性由共享 golden 夹具 `fixtures/m3u8-filter/cases.json` 锁定。改规则必须三端同步 + 重跑夹具。
>
> 三端统一入口语义（本文档称 **client 语义**）：给定清单文本与基准 URL，输出 = **仅绝对化 +
> 过滤（可选）**，子播放列表只绝对化、不代理回环。服务端代理包装（wrapMedia/回环代理）不属于
> 本规范（仅 Go proxy 链路内部使用）。

## 1. 输入输出

```
rewriteM3u8(body: string, base: URL, opts: { filterAds: boolean })
  -> { text: string, filtered: number, total: number }
```

- `text`：重写后的清单（逐行处理，**末尾空行保留**——body 按 `\n` split 后 join 回 `\n`）。
- `filtered`：被判为广告剔除的分片数（连同其悬挂 EXTINF 一起删）。
- `total`：本列表内真实分片（非子播放列表 URI 行）总数。
- `filterAds=false`：只绝对化，不剔任何分片（`filtered=0`，`total` 仍统计）。

## 2. 逐行处理主循环

对每行 `t = TrimSpace(line)`（原行保留用于非 URI 行输出）：

1. **空行** → 原样输出。
2. **`#EXTINF` 行** → 不输出，缓存为 `pendingExtinf`（与分片同生共死）。若行首非 `#EXTINF`
   （如 `#EXT-X-*` 其余标签）→ 不悬挂，走第 3 步。
3. **`#` 开头的标签行** → 输出 `rewriteTagURI(line)`（只改写 `URI="..."` 属性，见 §6）；
   其中 `#EXT-X-DISCONTINUITY` 额外置 `HasDisc` 标记与 `justAfterDisc` 采样位（§7）。
4. **URI 行**（非 `#`）：
   - `abs = absolutize(t, base)`（§4）。
   - **子播放列表**（`isPlaylistURI(abs)`，§5）→ 输出 `abs`（client 语义只绝对化）；
     首个子列表另记为 `firstChild`（供 Go stats 下钻，TS/Kotlin 不需要）。
   - **真实分片** → `total++`；`filterAds` 且 `pendingExtinf != ''` 时按三维判定
     （§3）决定剔除；剔除则丢弃 `pendingExtinf`、`filtered++`、跳过本行输出；
     保留则先输出 `pendingExtinf` 再输出 `abs`。

## 3. 三维广告判定（互斥优先级：cross > num > short）

判定基于**预扫**：按播放顺序收集分片绝对 URI 列表 `segURIs` 与对应时长列表 `segDurs`
（无 EXTINF 的分片时长记 `-1`）。三个判定各返回"下标 → 命中"集合：

| 维度 | 规则 | 不生效（返回空）条件 |
|---|---|---|
| **cross** 跨域/异目录 | 分片归属键 `segKey` ≠ 主导键 `dominant`（全表众数）→ 广告 | `dominant==''` 或空表；异键占比保护：`n ≥ 10` 且异键 `> 30%` → 整表不剔 |
| **num** 编号跳脱 | 尾号远离主内容簇 → 广告 | `n < 10`；可解析尾号比例 `< 80%`；去重后 `< 2` 个值；主簇占比 `< 60%`；候选广告 `> 30%` |
| **short** 极短簇 | 连续 ≥2 个 `<1s` 段且簇累计时长 `≥3s` → 整簇剔除 | 段长未知（-1）；开头前 4 段豁免（下标 `> 4` 才考虑） |

主循环按 `crossDrop || (!crossDrop && numDrop) || (!crossDrop && !numDrop && shortDrop)`
剔除（优先级：cross 先、num 次、short 兜底）。`durationOutliers`（z-score>3）**仅预警不剔除**。

### 3.1 cross：归属键与主导键

- `segKey(abs)` = `host + "/" + 内容目录`：
  1. 取 path，**先剥文件名**（去掉最后一段）；
  2. 剩余目录段取**前两段**；只有一段取一段；空则只 `host`。
- `dominant` = `segKey` 众数（出现最多者；并列时取任一——但占比保护使并列 50% 形态恒不剔，
  结果确定）。
- 保护：`n ≥ 10` 且异键数 `> n × 0.30` → 主导键疑似选错，整表不剔（宁可漏过滤）。

### 3.2 num：编号跳脱

- `segNumber(abs)`：取 path 文件名 → 去扩展名 → 尾部连续数字；**无则 -1**；超 15 位截末 15 位。
  （query 不影响：取 `URL.path` 不含 query。）
- 可解析比例 = 有编号数 / n，`< 0.80` → 跳过（哈希命名源）。
- 去重排序后按 `gap > thr` 切簇，`thr = max(1000, 1000 × medianGap(distinct))`；
  `medianGap` = 相邻差值排序后的中位值。
- 取**分片数最多**的簇为主内容簇；占比 `< 0.60` → 不可信，跳过。
- 簇外有编号分片 = 候选广告；候选 `> n × 0.30` → 误判，整表不剔。

### 3.3 short：极短簇

- 簇定义：连续满足 `dur ∈ [0, 1.0)` 且下标 `> 4` 的段；
- 簇累计时长 `≥ 3.0` 才剔整簇；否则保留（转场/镜头切换）。

## 4. absolutize（RFC3986 相对解析）

`http(s)` 绝对 → 原样；其余用 `URL(base).resolve(ref)`（Node/浏览器 URL 的 resolve 语义 =
Go `ResolveReference`）；解析失败原样返回。

## 5. isPlaylistURI

忽略 query/fragment，`path` 以 `.m3u8` 结尾（大小写不敏感）。

## 6. rewriteTagURI

正则 `URI="([^"]+)"` 首个命中：组内值 `absolutize` 后回填，标签其余部分不变。
（client 语义：只绝对化；不做代理/签名包装。）

## 7. 插片标记与采样（仅统计，不参与剔除）

- `HasDisc`：表内含 `#EXT-X-DISCONTINUITY`。
- `justAfterDisc`：DISCONTINUITY 后首个分片 URI 收进 `Sample`（≤5 条）；`#EXT-X-DISCONTINUITY`
  后**不一定**是广告，判定仍以三维为准。
- `DurOutlier`/`DurSample`：时长 z-score>3 的分片数与样本（预警日志用）。

## 8. 阈值常量（三端一致）

| 常量 | 值 | 含义 |
|---|---|---|
| adNumMinSegs | 10 | 编号/跨域检测最少分片数 |
| adNumMinCoverage | 0.80 | 编号可解析最低比例 |
| adNumMinContent | 0.60 | 主内容簇最低占比 |
| adNumMaxDropFrac | 0.30 | 编号候选广告剔除占比上限 |
| adCrossMinSegs | 10 | 跨域占比保护最少分片数 |
| adCrossMaxDropFrac | 0.30 | 跨域异键剔除占比上限 |
| adShortMaxDur | 1.0 | 极短判定阈值（秒） |
| adShortSkipHead | 4 | 开头豁免段数 |
| adShortBurstMinDur | 3.0 | 短簇剔除最小累计时长（秒） |
| m3u8SampleMax | 5 | 采样条数上限 |

## 9. 双实现防漂移纪律

1. **夹具为唯一裁决**：三端实现必须通过 `fixtures/m3u8-filter/cases.json` 全部用例；
2. 改规则 → 先用 Go 生成器重算期望值入库（生成器见 Go `m3u8_service.go` 的
   `rewriteM3u8Core`，client 语义），再同步 TS/Kotlin，三端跑绿才算完成；
3. 任何一端对同形态输出不一致 → 以 Go 为准修两端，并在夹具补该用例。
