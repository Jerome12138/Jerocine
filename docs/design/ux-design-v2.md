# Jerocine UX 设计规范 v2

> **版本** v2.0  
> **日期** 2026-05-16  
> **范围** `web/` 前端全量 UX 规范（用户端 + 管理端 + TV 模式）  
> **目标** WCAG 2.1 **Level AA**  
> **平台** Web Desktop / Web Tablet / Web Mobile / 大屏 TV / Capacitor Android

---

## 0. 摘要

本规范是 `web/` 的 UX source of truth：信息架构、设计 tokens、组件三态（空/错/loading）台词、管理后台风格统一与 a11y（可达性）要求，均以本文为准；实现以代码为准。

**方法论**：信息架构重排、扩展 tokens、补全组件状态机、统一组件库、补齐 a11y——在既有实现基础上迭代收敛到本规范，缺什么补什么。

**实施分批** P0 基础（IA + tokens + 状态系统）→ P1 重点页面（首页 / 详情 / 播放 / 后台）→ P2 精修（动画规范 / i18n 接入点 / 大屏专项）。

---

## 1. 设计原则

| # | 原则 | 落地方式 |
|---|---|---|
| 1 | **影视优先** | 3:4 海报为视觉锚点；所有列表卡、骨架、占位图同比例 |
| 2 | **暗色沉浸** | `bg-base #0b0b0f` 单一底；分区靠 `surface/elevated` 层次，不靠强分隔线 |
| 3 | **触控友好** | 所有 interactive 元素 ≥ 44×44px touch target；移动端 hover 效果禁用 |
| 4 | **键盘可达** | 所有 interactive 必须 `focus-visible` 可见；Tab 顺序与视觉对齐 |
| 5 | **运动克制** | duration 二档（fast 120ms / base 200ms）；遵守 `prefers-reduced-motion` |
| 6 | **API 零改动** | 后端契约不动（PRD §4 硬约束）；前端 query 参数命名严格保留 |

---

## 2. 设计背景（v1 → v2 演进诊断）

> 本节为 v2 立项时对 v1 实现的诊断记录，保留作为设计决策依据；当前实现已按后续章节规范落地。

v1 已交付 11 项 bilibili 风格改造。**当时仍存的 8 类系统性 gap**：

| Gap | 表现 | 影响 |
|---|---|---|
| **G1 IA 混乱** | 首页 hero + chip + 多 row + 推荐瀑布流，4 层堆叠无层级；分类入口与 row 标题重复 | 用户找不到落点 |
| **G2 空状态散** | `BaseEmpty` 各处文案/插画/CTA 各异 | 体验割裂 |
| **G3 错误台词散** | 401/404/网络/超时 各 toast/empty 文案不一 | 用户困惑 |
| **G4 Skeleton 不像页** | ratio/height 散落，骨架不模拟真布局 | 加载体感糟糕 |
| **G5 后台未统一风格** | 表格/筛选/分页与公共端 bilibili 风差远 | 站长体验断层 |
| **G6 动画规范缺** | duration/ease 散落 12 处不同值 | 微交互不一致 |
| **G7 a11y AA 抽样缺** | 触屏 hover 区缺替代、focus ring 颜色对比、aria-live 缺、skip-link 缺 | 残障用户/键盘用户用不顺 |
| **G8 响应式断点遗漏** | 平板 768-1023 几乎无专属布局；TV 仅样式略调，IA 未改 | 多端体验不平衡 |

---

## 3. 信息架构 (IA)

### 3.1 全站路由树

```
/                       重定向 → /index
├── /index              首页 (访客)
├── /filmClassify       分类首页 ?Pid=
├── /filmClassifySearch 分类筛选 ?Pid=&Category=&Plot=&Area=&Language=&Year=&Sort=&current=
├── /search             关键字搜索 ?search=&current=
├── /filmDetail         影片详情 ?link=
├── /play               播放页 ?id=&source=&episode=&currentTime=
├── /history            观看历史 (访客也可, 本地存储)
├── /favorites          收藏 (需登录)
├── /login              登录
└── /manage/*           站长后台 (需登录 + 管理员)
    ├── /index          仪表盘
    ├── /collect        采集
    ├── /cron           定时任务
    ├── /film           影片管理
    ├── /file           文件
    └── /system/webSite 站点配置
```

### 3.2 公共端层级（修正 IA）

```
Layer 0  全局壳        PublicHeader (常驻搜索) + PublicLayout + PublicFooter + MobileTabbar
Layer 1  浏览入口      Home  /  Classify (Pid)  /  Search
Layer 2  内容决策      FilmDetail (backdrop / poster / meta / CTA)
Layer 3  内容消费      Play
Layer 4  个人态        History / Favorites / Settings (隐含 UserMenu)
```

**规则**

- Layer 1 卡片点击 → Layer 2 (FilmDetail)，**不再直接到 Play**（避免跳过决策）
- Layer 2 "立即播放" / "继续观看" → Layer 3 (Play)
- Layer 4 内容卡片点击 → Layer 3 (Play, 直达续播位)，**绕过 Layer 2**（已是熟内容）

### 3.3 管理后台层级

```
Layer 0  ManageLayout (Sidebar + Header + 内容区)
Layer 1  Dashboard (仪表盘, 4 张统计卡 + 最近活动流)
Layer 2  资源管理列表 (Collect / Cron / Film / File / User)
Layer 3  详情/编辑/新增 (Drawer 或路由)
Layer 4  系统配置 (Site / Security / About)
```

**Sidebar 分组**

```
概览
  ├ 仪表盘
内容
  ├ 影片管理
  ├ 分类管理
  ├ 新增影片
采集
  ├ 采集源
  ├ 定时任务
  ├ 启动采集 (action)
文件
  ├ 文件库
  ├ 上传
用户 (admin)
  ├ 用户列表
  └ 新增用户
系统
  └ 站点配置
```

---

## 4. User Flows

### 4.1 浏览发现流（访客主路径）

```
[Home /index]
   ├─ Hero 轮播 → 点片 → [FilmDetail]
   ├─ 分类入口 chip → [Classify Pid]
   ├─ 横滚 row 卡片 → [FilmDetail]
   └─ 推荐瀑布流 → [FilmDetail]
                         │
                         ▼
                   [FilmDetail]
                         │
                ┌────────┼──────────┐
                ▼        ▼          ▼
            收藏切换  立即播放    相关推荐
                              │
                              ▼
                          [Play]
                              │
                       ┌──────┼──────┐
                       ▼      ▼      ▼
                    切集    切源   查看完整介绍 → 回 [FilmDetail]
```

### 4.2 搜索流

```
[任何页] 顶栏搜索框
   ├─ 聚焦但未输入 → 下拉显示 历史 + 热搜 (P1 新增)
   ├─ 输入 + 回车 → [Search ?search=]
   │                    │
   │                    ▼
   │              结果首条 = 最佳匹配大卡
   │              其余 = 网格列表
   │              侧栏 (PC) = 相关分类入口 chip
   │                    │
   │                    ▼
   │                 点击 → [FilmDetail]
   │
   └─ 输入但无结果 → "未查询到" + 推荐热搜词
```

### 4.3 观看历史流

```
[History /history]
   ├─ 时间分组 (今天/本周/本月/更早)
   ├─ 卡片底部进度条 (3px 横条 + 渐变填充)
   ├─ 点击卡片 → [Play] 直接续播
   │              └─ source/episode/currentTime 从历史记录回填
   └─ 长按/复选 → 进入批量管理模式 (P1 新增) → 批量删除
```

### 4.4 后台站长流

```
[Login] → 鉴权 → [Dashboard]
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
   采集源列表    定时任务     影片管理
        │            │            │
    + 新建      + 新建       搜索 + 编辑
    测试连通    立即触发    上下架/分类
        │            │            │
        └────────────┴────────────┘
                     │
                     ▼
                配置中心 (Site / Security)
```

### 4.5 错误恢复流（横切所有页面）

```
任何 API 请求
   ├─ 200 → 渲染内容
   ├─ 网络断 / 5xx → ErrorBanner (顶部) + "重试" 按钮
   ├─ 401 → 拦截器清 token + 路由 redirect → /login?redirect=...
   ├─ 403 → toast "权限不足", 留在原页
   ├─ 404 (业务) → 页面级 BaseEmpty + "返回首页"
   └─ 客户端 panic → 全局 Error Boundary 兜底 + "刷新页面"
```

---

## 5. Wireframes (10 核心页面)

> 命名说明：所有断点 ≥1024 视为 **Desktop**，768-1023 为 **Tablet**，<768 为 **Mobile**。TV 模式（[data-mode='tv']）以 Desktop 为基底，安全区 + 字号 +1 档。

### 5.1 Home `/index`

```
Desktop (≥1024)
┌────────────────────────────────────────────────────────────┐
│ Header  Logo | Nav 6项 | [   常驻搜索框 480px   ] | User    │  60px fixed
├────────────────────────────────────────────────────────────┤
│ ┌────────────────────────────────────────────────────┐    │
│ │   Hero 21:9 / 360-520px 高                          │    │  hero 区
│ │   [文案左下]  片名 / tag / 简介 / [立即观看 CTA]    │    │
│ │   [指示条 → 横条带进度填充]  ······                  │    │
│ └────────────────────────────────────────────────────┘    │
│                                                            │
│ [电影] [剧集] [综艺] [动漫] [纪录片]   ← 分类 chip 行 64px │
│                                                            │
│ 最新上架  >                                                │
│ [3:4] [3:4] [3:4] [3:4] [3:4] [3:4] →   ← row, 6 卡可滚 │  ~280px
│                                                            │
│ 本周热门  >                                                │
│ [3:4] [3:4] [3:4] [3:4] [3:4] [3:4] →                    │
│                                                            │
│ ─── 猜你喜欢 ───────────────────────────────────────────  │
│ [3:4][3:4][3:4][3:4][3:4][3:4]   ← 推荐瀑布流 6 列网格    │
│ [3:4][3:4][3:4][3:4][3:4][3:4]                            │
│ [加载更多]                                                 │
└────────────────────────────────────────────────────────────┘
│ Footer                                                     │
└────────────────────────────────────────────────────────────┘

Tablet (768-1023)
[Header 缩 search 320px] → Hero 16:9 → chip 4 项 → row 4-5 卡 → 瀑布流 4 列

Mobile (<768)
[Header 52px + 汉堡 + 搜索图标] → Hero 16:10 → chip 横滚 → row 2-3 卡 → 瀑布流 2-3 列
[底部 MobileTabbar 56px: 首页/分类/历史/我的]
```

**关键差异 vs 当前实现**

- ✅ 已有：Hero / chip 行 / 多 row / 瀑布流 / tabbar
- ⚠️ **改进**：chip 行与 row 标题重复（同样是"电影/剧集"分类），新版要做信息分层 → chip 是"全部分类入口"（含纪录片等没 row 的），row 标题用动作语（"最新上架/本周热门/高分推荐"）而不是分类名
- ⚠️ **改进**：右侧"热播榜"（aside）当前仅 desktop 显示但与下方瀑布流冲突，新版**砍掉 aside**，热播信息融进 row 标题"本周热门"

### 5.2 FilmDetail `/filmDetail?link=`

```
Desktop
┌────────────────────────────────────────────────────────────┐
│ Header                                                     │
├────────────────────────────────────────────────────────────┤
│ ┌────────────────────────────────────────────────────┐    │
│ │  模糊 backdrop (片头大图, blur 40px, 暗化 60%)     │    │
│ │                                                     │    │
│ │  [Poster 3:4]   片名 (display 28-34px)             │    │
│ │   220×293       2024 · 中国 · 动作 · 9.2 分        │    │
│ │                  [tag×6]                            │    │
│ │                  导演 / 主演 / 上映 / 地区          │    │
│ │                  剧情简介 2 行 [展开]               │    │
│ │                  [▶ 继续观看·第3集] [从头播放]      │    │
│ │                                  [♥ 已收藏] [⤴ 分享]│    │
│ └────────────────────────────────────────────────────┘    │
│                                                            │
│ ─── 相关推荐 ─────────────────────────────────────────────  │
│ [3:4][3:4][3:4][3:4][3:4][3:4]                            │
└────────────────────────────────────────────────────────────┘

Mobile: poster 居中 + 信息纵向堆叠 + CTA 全宽
```

**关键变更**

- ✅ 已有：backdrop / poster / 简介折叠 / 继续观看
- 🟡 **遵循方案 A**：详情页不放选集面板（已落地）
- ⚠️ **新增**：分享按钮接入 `navigator.share` API，移动端拉起系统分享面板（PC 用 clipboard）

### 5.3 Play `/play`

```
Desktop (bilibili 风格)
┌────────────────────────────────────────────────────────────┐
│ Header                                                     │
├────────────────────────────────────────────────────────────┤
│ < 返回详情                                                 │
│                                                            │
│ ┌───────────────────────────────┐ ┌─────────────────────┐ │
│ │                               │ │ 选集                 │ │
│ │   Video 16:9                  │ │ [线路1][线路2]      │ │  右栏
│ │                               │ │ [1-30]              │ │  sticky
│ │                               │ │ [1][2][3][4][5][6]  │ │
│ └───────────────────────────────┘ │ [7][8][9]...        │ │
│  片名 · 第3集  查看完整介绍 ›     │ ─────────────       │ │
│  [tag×3]                          │ 相关推荐             │ │
│  [♥ 点赞 1.2w] [⭐ 收藏] [⤴ 分享] │ ┌──┐ 片1            │ │
│                                   │ └──┘ tag           │ │
│  ─── 剧情简介 ──────────         │ ┌──┐ 片2            │ │
│  ......                           │ └──┘ tag           │ │
└───────────────────────────────────┴─────────────────────┘ │
└────────────────────────────────────────────────────────────┘

Mobile: 视频满宽 16:9 → 标题/三连/选集/简介 纵向; 相关推荐放最底
```

**已落地**：bilibili 布局 / 三连 / 选集（含分段）/ 网络感知 lowBandwidth / 错误重试

**新增**

- ⚠️ 键盘快捷键提示浮层（首次访问显示，3s 自动消失）：空格 / ← → / ↑ ↓ / F (全屏) / M (静音) / Esc
- ⚠️ 倍速 + 画质切换菜单（VHS representations API）—— P1 留口

### 5.4 ClassifySearch `/filmClassifySearch`

```
Desktop
┌────────────────────────────────────────────────────────────┐
│ Header                                                     │
├────────────────────────────────────────────────────────────┤
│ 类型 [全部][动作][喜剧]...  ┐                              │
│ 地区 [全部][华语][日韩]...   │  sticky 吸顶 padding 12px   │
│ 年份 [全部][2025][2024]...   │  bg rgba(11,11,15,.92) blur │
│ 排序 [最新▼][热度][评分]     ┘                             │
├────────────────────────────────────────────────────────────┤
│ 共 326 部                                  [⊞] [☰]         │
│                                                            │
│ [3:4][3:4][3:4][3:4][3:4][3:4]   网格 5-6 列              │
│ [3:4][3:4][3:4][3:4][3:4][3:4]                            │
│ [3:4][3:4][3:4][3:4][3:4][3:4]                            │
│           [< 1 2 3 4 5 ... >]                              │
└────────────────────────────────────────────────────────────┘
```

**已落地**：chip 筛选 / sticky / 5-6 列网格 / 结果计数

**新增**

- ⚠️ 视图切换按钮（⊞ 网格 / ☰ 列表）— 列表模式适合移动端"看更多元信息"场景
- ⚠️ 筛选 chip 顶部加"清空全部"快捷（所有维度回 "全部"）
- ⚠️ 移动端 sticky 筛选区可折叠（点"筛选"按钮展开抽屉，收起后只显示"筛选 3 项"小标签）

### 5.5 Search `/search`

```
Desktop
┌────────────────────────────────────────────────────────────┐
│ Header                                                     │
├────────────────────────────────────────────────────────────┤
│ [────  搜索框 (聚焦居中)  ────]                             │
│                                                            │
│ ──有结果──                                                 │
│ "无间道" 共 18 个结果                                      │
│                                                            │
│ ┌──────────┐                                              │
│ │ Poster   │  最佳匹配                                    │
│ │ 3:4      │  无间道                                      │
│ │ 180×240  │  2002 · 香港 · 警匪  9.3 分                  │
│ │          │  [▶ 立即观看] [查看详情 ›]                   │
│ └──────────┘                                              │
│                                                            │
│ 其他结果                                                   │
│ [3:4][3:4][3:4][3:4][3:4]  网格 5 列                      │
│ [3:4][3:4][3:4][3:4][3:4]                                 │
│                                                            │
│ ──无关键字──                                               │
│ 热门搜索                                                   │
│ [1 流浪地球] [2 庆余年] [3 狂飙] [科幻] [动作]...          │
│                                                            │
│ 历史搜索                                  [清空]            │
│ [无间道 ×] [庆余年 ×] [流浪地球 ×]                         │
└────────────────────────────────────────────────────────────┘
```

**已落地**：最佳匹配大卡 / 热搜词 / 历史 chip

**新增**

- ⚠️ 搜索框获得焦点时下拉显示"热搜 + 历史" suggestion（不必跳到 /search 页才能看）
- ⚠️ 输入时实时联想（暂以本地 fuzzy 实现，后端接口 P2 再说）

### 5.6 History `/history`

已落地：时间分组 + 进度条。**新增**

- ⚠️ 批量管理模式：长按 / 顶部"管理"按钮进入，卡片左上 checkbox，底部固定操作栏 `已选 N 项 [全选] [删除]`
- ⚠️ 卡片 hover 时多一个快捷"继续从 X 分钟 X 秒看"标签

### 5.7 Favorites `/favorites`

```
（结构同 History, 只是不分时间）
按收藏夹分组（默认 / 用户自建）
卡片右上 ❤️ 切换收藏；批量管理同 History
```

### 5.8 Manage Dashboard `/manage/index`

```
Desktop
┌──────┬─────────────────────────────────────────────────────┐
│      │ 面包屑: 仪表盘                       [搜索][用户▾]  │  56px
│ Logo ├─────────────────────────────────────────────────────┤
│ ▾    │                                                     │
│ 概览 │ ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐                   │
│ 内容 │ │影片  │ │采集  │ │定时  │ │用户  │   统计卡 4 张  │
│ 采集 │ │ 12.5w│ │  8   │ │  16  │ │ 320  │                │
│ 文件 │ └─────┘ └─────┘ └─────┘ └─────┘                   │
│ 用户 │                                                     │
│ 系统 │ ┌─────────────────────┐ ┌─────────────────────┐    │
│      │ │ 最近采集 (折线图)    │ │ 分类分布 (饼图)      │    │
│ 220  │ │                     │ │                     │    │
│ /64  │ └─────────────────────┘ └─────────────────────┘    │
│      │                                                     │
│      │ 最近活动                                            │
│      │ [事件流: spider 完成 / cron 触发 / 文件上传 …]       │
└──────┴─────────────────────────────────────────────────────┘
```

**新增/改进**

- ⚠️ 当前 Dashboard 只 4 张统计卡，缺图表/活动流 — 补两张 chart 卡 + 活动流
- ⚠️ Sidebar 可折叠（已落地），加二级菜单展开/收起

### 5.9 Manage 资源列表（采集/cron/影片通用模板）

```
Desktop
┌──────┬─────────────────────────────────────────────────────┐
│ Side │ 面包屑: 采集源管理                        [+ 新增]  │
│      ├─────────────────────────────────────────────────────┤
│      │ [搜索] [类型 ▾] [状态 ▾]    刷新 ↻  导出 ⤓          │
│      │ ┌─────────────────────────────────────────────────┐ │
│      │ │ 名称  │ 类型 │ URI  │ 状态 │ 权重 │ 操作         │ │
│      │ ├──────┼──────┼──────┼──────┼──────┼──────────────┤ │
│      │ │ ...  │ ...  │ ...  │ ●启用│ ...  │ [测试][编辑] │ │
│      │ │ ...  │ ...  │ ...  │ ○停用│ ...  │ [...]        │ │
│      │ └─────────────────────────────────────────────────┘ │
│      │                                         < 1 2 3 >  │
└──────┴─────────────────────────────────────────────────────┘

Mobile: 表格降级为卡片列表, 每行一卡显示主字段 + 折叠次要字段
```

**风格一致性改造**

- ⚠️ 表格行高 48px (touch friendly), 偶数行 zebra 极浅（rgba 0.02）
- ⚠️ 状态用 BaseTag (success/warning/danger) 替代原生 dot
- ⚠️ 操作按钮统一 BaseButton size=sm ghost 风格

### 5.10 Auth `/login`

```
全屏背景 backdrop (站点 hero 大图 blur)
   ┌───────────────────────────┐
   │  Jerocine                   │
   │  登录                      │
   │                           │
   │  用户名 [______________]   │  44px input
   │  密码   [_____________⨀]   │
   │                           │
   │  [────── 登录 ──────]      │  primary CTA 全宽
   │                           │
   │  忘记密码? · 联系站长       │
   └───────────────────────────┘
```

**改进**

- ⚠️ 错误信息以 inline message 出现在输入框下方（不再 toast）
- ⚠️ 密码可见性按钮（眼睛 icon）

---

## 6. Components Library v2

### 6.1 基础（保留 + 强化）

| 组件 | 现状 | v2 改动 |
|---|---|---|
| BaseButton | 多 variant 全, size 全 | 加 `loading` prop（spinner）+ icon 位 |
| BaseIcon | path map | 补充 8 个图标: filter / sort / settings / refresh / download / more-horizontal / trash / check |
| BaseImage | ratio + lazy | 加 `placeholder` prop（blur-up） |
| BaseTag | variant 全 | 加 `dot` prop (左侧色点, 用于状态) |
| BaseSkeleton | rect / text | 加 `aspect` prop 替代 ratio 字符串, 与 token `--gf-card-aspect` 联动 |
| BaseDialog | 居中 modal | 加 `fullscreen` 移动端断点 + drawer 模式 |
| BasePagination | 数字翻页 | 加 `simple` 模式（仅上一页/下一页） |
| BaseEmpty | 标题/描述/action | **强化**：4 种预设 (no-data/no-result/error/network)，含统一插画 SVG |

### 6.2 新增组件（v2）

| 组件 | 用途 |
|---|---|
| **BaseFilterChipRow** | 单维筛选 chip 行（含"全部"+ N 个选项 + 多选模式） |
| **BaseSegmented** | 分段控件（如选集 1-30/31-60, 网格/列表视图切换） |
| **BaseAvatar** | 头像（含尺寸 + dicebear fallback + 上传按钮态） |
| **BaseDropdown** | 通用下拉（搜索 suggestion / 用户菜单 / 设置） |
| **BaseDataTable** | 后台数据表（含 zebra / hover / 排序 / 空态 / mobile 卡片降级） |
| **BaseStatusDot** | 资源状态 dot（success/warning/danger/info），用于列表 |
| **BasePageHeader** | 后台页头（面包屑 + 标题 + actions 右对齐） |
| **BaseErrorBoundary** | 全局错误兜底（Vue ErrorCaptured + fallback UI） |
| **BaseSkipLink** | 跳转链接（顶部隐藏，Tab focus 时显示，跳到 main） |
| **BaseShortcutHint** | 键盘快捷键提示浮层（首次访问 / "?" 触发） |

### 6.3 业务组件升级

| 组件 | 改动 |
|---|---|
| **FilmCard** | 加 `loading` 骨架内嵌（不依赖父级 Skeleton） |
| **FilmRow** | 加 `variant="grid"` 模式（瀑布流复用同组件） |
| **HeroCarousel** | 加 `interval=0` 静态模式（A11y reduced-motion 自动启用） |
| **EpisodeTabs** | 加 `mode="compact"` 移动端紧凑模式（每行 5 个 chip 紧贴） |
| **PublicHeader** | 搜索框聚焦下拉 suggestion + 顶部 SkipLink |
| **ManageSidebar** | 二级菜单展开/收起 + 折叠态 tooltip |
| **ManageHeader** | 加面包屑（自动 router watcher） |

---

## 7. Design Tokens v2 (增量)

> 当前 `theme.css` 已有 134 个 token + P0 加的 12 个。下面是 **v2 再增量** 部分，与现有共存。

### 7.1 状态色（语义化扩展）

```css
/* 当前已有 success/warning/danger/info, 补 soft / strong 变体 */
--gf-success-strong: #16a34a;
--gf-warning-strong: #d97706;
--gf-danger-strong:  #dc2626;
--gf-info-strong:    #2563eb;

/* 加 selected / pressed 状态层 */
--gf-state-hover:    rgba(255, 255, 255, 0.06);
--gf-state-active:   rgba(255, 255, 255, 0.10);
--gf-state-selected: rgba(155, 73, 231, 0.12);
```

### 7.2 运动 token（统一)

```css
/* 已有 dur-fast/base/slow, 补 ease 套件 */
--gf-ease-standard:    cubic-bezier(0.4, 0.0, 0.2, 1);     /* 已有 */
--gf-ease-decelerate:  cubic-bezier(0.0, 0.0, 0.2, 1);     /* 入场 */
--gf-ease-accelerate:  cubic-bezier(0.4, 0.0, 1.0, 1);     /* 离场 */
--gf-ease-spring:      cubic-bezier(0.34, 1.56, 0.64, 1);  /* 已有 */
--gf-ease-emphasized:  cubic-bezier(0.2, 0, 0, 1);         /* 强调 */

/* prefers-reduced-motion 自动减弱 */
@media (prefers-reduced-motion: reduce) {
  :root {
    --gf-dur-fast: 0.01ms;
    --gf-dur-base: 0.01ms;
    --gf-dur-slow: 0.01ms;
  }
  .gf-hero__bar-progress { animation: none; transform: scaleX(1); }
}
```

### 7.3 后台专用

```css
--gf-manage-row-height:        48px;     /* 表格行高 */
--gf-manage-cell-padding-x:    12px;
--gf-manage-zebra-bg:          rgba(255, 255, 255, 0.02);
--gf-manage-table-header-bg:   rgba(255, 255, 255, 0.04);
```

### 7.4 焦点环（a11y AA）

```css
/* 现有 shadow-focus-ring 偏暗, AA 要求 2px / 与背景对比 3:1 */
--gf-focus-ring-color: var(--gf-brand-cyan);  /* #4ad1e5, 对 bg-base 对比 ≥ 7:1 */
--gf-focus-ring:       0 0 0 2px var(--gf-bg-base), 0 0 0 4px var(--gf-focus-ring-color);
/* 双层 ring: 内圈 bg-base 制造 gap, 外圈 cyan, 任何背景都清晰可见 */
```

---

## 8. 状态系统（Loading / Empty / Error）

### 8.1 Loading 三档

```
Skeleton  → 内容已知尺寸 (列表/卡片/详情) — 用 BaseSkeleton 模拟真布局
Spinner   → 短任务 < 1s (按钮提交 / 内联) — BaseButton.loading
Progress  → 已知进度 (上传 / 网络感知码率) — 线性条
```

**统一规则**: 任何 API 请求 > 200ms 显示 loading；< 200ms 不显示（避免闪烁）。

### 8.2 Empty 四种预设（BaseEmpty preset）

| Preset | 场景 | 文案 | CTA |
|---|---|---|---|
| `no-data` | 首次访问没数据 | "暂无内容" + 子文案 | 通常无 |
| `no-result` | 筛选/搜索结果空 | "没有符合条件的内容" | "重置筛选" / "清空搜索" |
| `error` | 业务错误 | "{{err}}" | "重试" |
| `network` | 网络断连 | "网络似乎断了" | "重试" |

### 8.3 Error 横切

| 类型 | 表现 | 恢复 |
|---|---|---|
| 网络/5xx | 顶部 ErrorBanner (red soft 背景) + 重试 | 重试或自动 30s 后重试 |
| 401 | 拦截器清 token + 路由 redirect /login | 重新登录 |
| 403 | toast "权限不足" | 留在原页 |
| 404 业务 | 页面级 BaseEmpty type=no-data | 返回首页 |
| 客户端 panic | BaseErrorBoundary 兜底 | 刷新页面 |
| 视频源错 | 已有重试机制（2 次退避 1.5s/4s） | 全失败换源 |

---

## 9. Accessibility (WCAG 2.1 AA)

### 9.1 Perceivable

| 项 | 要求 | 实施 |
|---|---|---|
| 文字对比 | ≥ 4.5:1（正文）/ 3:1（大字） | text-primary #fff 对 bg-base #0b0b0f = 19:1 ✅ / text-secondary rgba(.78) ≈ 12:1 ✅ / text-muted rgba(.55) ≈ 7:1 ✅ |
| 非文字对比 | ≥ 3:1（icon / border） | focus-ring cyan #4ad1e5 对 bg-base ≈ 7:1 ✅ |
| 不靠颜色传达 | 状态 dot 配 icon/text | BaseStatusDot 接 label prop, dot 仅辅助 |
| 缩放 200% | 不破坏布局 | rem-based 字体 + clamp() 缩放安全 |
| 视觉简单 | 单底色 + 不主动用红色报警（保留品牌色） | 暗色一致 |

### 9.2 Operable

| 项 | 要求 | 实施 |
|---|---|---|
| 全键盘可达 | Tab 顺序与视觉对齐 | 所有交互元素 `tabindex="0"` 或原生可聚焦 |
| Focus 可见 | 对比 ≥ 3:1 | `--gf-focus-ring` 双层 ring（任何背景可见） |
| 跳过导航 | "Skip to main content" | BaseSkipLink 组件，顶部 Tab 第一个 focus |
| 触屏 target | ≥ 44×44px | 所有按钮 / chip / tab 强约束 |
| 自动播放 | 静音 + 用户控制 | Hero 静态 mode (reduced-motion) / 视频 default autoplay=false |
| 快捷键 | Esc 退出 modal, ← → 切集 | BaseShortcutHint 首次提示 |

### 9.3 Understandable

| 项 | 要求 | 实施 |
|---|---|---|
| 语言 | `lang="zh-CN"` | index.html |
| Form 标签 | label for + aria-describedby | 所有 input |
| 错误说明 | aria-invalid + 具体 message | BaseInput + 业务 |
| 一致导航 | Header / Tabbar 路径稳定 | IA §3 |
| 一致命名 | "立即播放" / "继续观看" 用语固定 | 文案规范 §9.5 |

### 9.4 Robust

| 项 | 要求 | 实施 |
|---|---|---|
| 语义 HTML | `<header><nav><main><footer><article>` | 所有 layout |
| ARIA 标签 | icon-only 必须 aria-label | 已部分实施, 补 3 处 (Hero / 关闭按钮 / 三连) |
| 动态区域 | aria-live=polite | toast / progress / shareLabel |
| Modal | role=dialog + aria-modal | BaseDialog 已有 |

### 9.5 文案规范

| 场景 | 文案（统一） |
|---|---|
| 加载 | "加载中…" |
| 空数据 | "暂无内容" |
| 空搜索 | "没有找到符合条件的内容" |
| 网络错 | "网络似乎断了，请检查后重试" |
| 5xx | "服务暂时不可用，请稍后再试" |
| 401 | "登录已过期，请重新登录" |
| 403 | "权限不足" |
| 主播按钮 | "立即观看" / "继续观看·第N集" |
| 收藏 | "收藏" / "已收藏" |
| 分享 | "分享" / "已复制" / "复制失败" |

---

## 10. Developer Handoff

### 10.1 实施优先级

**P0 基础（必做，1-2 周）**

1. 扩展 `theme.css` 加入 §7 增量 tokens
2. 实现 `BaseSkipLink` + 接入 PublicLayout / ManageLayout
3. 全局 prefers-reduced-motion 兼容（§7.2）
4. 统一 BaseEmpty 4 预设
5. 全局 ErrorBoundary
6. 文案规范集中到 `src/locales/zh-CN.ts`，为 i18n 留口
7. 信息架构修正：首页 chip 行用法、row 标题用动作语
8. WCAG 抽样补全（focus-ring 双层、3 处 aria-label 补全）

**P1 重点页面（2-3 周）**

9. Search 搜索框聚焦下拉 suggestion（含热搜+历史）
10. ClassifySearch 视图切换 (网格/列表) + 移动端筛选抽屉
11. History/Favorites 批量管理模式
12. ManageDashboard 补图表 + 活动流
13. Manage 资源列表用 BaseDataTable 统一样式
14. Manage 二级菜单 / 面包屑
15. PlayView 快捷键提示浮层

**P2 精修（1-2 周）**

16. 倍速 + 画质切换菜单（VHS representations）
17. i18n 集成（vue-i18n@9, 中文为基线, 留英文 scaffold）
18. TV 模式 IA 专项（不仅是字号放大）
19. PWA / Capacitor 优化（splash / offline 提示）

### 10.2 验证清单

```
□ 所有交互元素 :focus-visible 可见
□ 所有 icon-only 按钮有 aria-label
□ 所有图片有 alt
□ Lighthouse Accessibility ≥ 95
□ axe-core 0 critical, 0 serious
□ 键盘走完一遍 Home → Detail → Play → History 不卡顿
□ 200% 缩放不出现横向滚动条 (320px 视口下)
□ prefers-reduced-motion 下所有动画停
□ Skip-link 在 Tab 顺序第 1 位
□ 文案 9.5 表格全站统一
□ vitest 测试通过 (现 36 + 增量 ≥ 50)
```

### 10.3 给后续 Dev Agent 的提示

- **不抛弃当前实现**: P0/P1/P2 已落地的 11 项都保留，本设计是叠加 + 系统化
- **按 §10.1 P0 → P1 → P2 分批 commit**，每批跑 type-check + vitest + build
- **a11y 检查随改动同步**: 改一个组件就在该 PR 里验该组件无 axe 报错
- **文案改动一次性提交**: 不要散落到各 commit
- **设计 tokens 改动**: 全部进 theme.css 一处，不在组件 scoped style 写死

---

## 11. 设计验证 (映射 PRD)

| 用户故事 | 页面 | v2 状态 |
|---|---|---|
| US 首页 hero + row + 热播 | Home | ✅ 已落地; v2 热播融进 row 标题 |
| US 详情 backdrop + CTA + 选集 | FilmDetail | ✅ 已落地; v2 选集移交播放页（方案 A） |
| US 播放 video + 选集换源 | Play | ✅ 已落地 |
| US 搜索 keyword | Search | ✅ 已落地; v2 加 suggestion |
| US 分类首页 news/top/recent | Classify | ✅ 已落地 |
| US 筛选 chip | ClassifySearch | ✅ 已落地; v2 加视图切换 + 移动抽屉 |
| US 后台登录 + 路由守卫 | Login | ✅ 已落地 |
| US 后台仪表盘 | Dashboard | 🟡 v2 补图表 + 活动流 |
| US 后台 CRUD (collect/cron/film/file) | Manage 列表 | 🟡 v2 统一 BaseDataTable |
| US 后台站点配置 | System/webSite | ✅ 已落地 |

---

## 12. 签收

- [ ] 产品经理 review
- [ ] 系统架构师 review (a11y / i18n 实施可行性)
- [ ] Sprint Planning 拆解为开发故事
- [ ] 开始 P0 实施

---

><!-- 原始产出: BMAD create-ux-design workflow, 2026-05-16 -->  
*生成时间: 2026-05-16*  
<!-- 基线: v2 立项时的前端实现快照 -->
