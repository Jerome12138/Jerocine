# Jerocine 组件视觉规范（暗色主题）

> 所有 Token 引用见 `design-tokens.md`。本文档面向前端开发，给出关键组件的尺寸 / 颜色 / 状态 / 断点行为。

---

## 1. AppHeader 顶部导航

### 结构
左 Logo（渐变文字 Jerocine）｜ 中 主导航（首页 / 分类 / 排行 / 最近更新）｜ 右 搜索 + 用户头像/登录

### 尺寸

| 断点 | 高度 | 内边距（左右） |
|---|---|---|
| `< 768px` | 56px | 16px |
| `768 - 1023px` | 64px | 24px |
| `>= 1024px` | 72px | 40px |

### 样式状态

| 状态 | 背景 | 边框底部 | backdrop-filter |
|---|---|---|---|
| 顶部 / 未滚动（仅 Home） | `--gf-bg-header`（透明） | 无 | 无 |
| 已滚动 / 非 Home | `--gf-bg-header-scrolled` | `1px solid var(--gf-border-subtle)` | `blur(20px)` |

- 滚动阈值：`scrollY > 80px`
- 切换过渡：`background-color, backdrop-filter, border 250ms var(--gf-ease-standard)`
- z-index：`--gf-z-header`，`position: fixed; top: 0`

### Logo
- 文字 "Jerocine"，`var(--gf-fs-xl)` `var(--gf-fw-black)`
- 渐变：`background: var(--gf-brand-gradient); background-clip: text; color: transparent;`
- 移动端高度 28px / 桌面 32px

### 主导航
- 字号 `var(--gf-fs-md)`，`var(--gf-fw-medium)`
- 默认 `--gf-text-secondary`，hover/active `--gf-text-primary`
- Active 当前页：底部 2px 渐变下划线（`--gf-brand-gradient`）
- 移动端（< 1024px）：导航折叠为汉堡按钮 → 抽屉式侧边菜单

### 搜索入口
- 桌面：行内展开搜索框（见 SearchBar）
- 移动：仅显示放大镜图标按钮 44×44px，点击后打开全屏搜索

### 用户区
- 未登录：显示"登录"文字按钮（链接 `/login`）
- 已登录（管理员）：32×32 圆形头像，hover 出现下拉菜单（控制台 / 退出登录）

---

## 2. HeroCarousel 首屏轮播

### 尺寸

| 断点 | 高度 | 文字层定位 |
|---|---|---|
| `< 768px` | `60vh`（最低 420px） | 底部居中对齐 |
| `768 - 1023px` | `65vh` | 左下 24px |
| `>= 1024px` | `70vh`（最低 560px，最高 720px） | 左下 40px / 距底 80px |

### 视觉

- 背景：影片大图 `object-fit: cover`
- 蒙版：底部 `--gf-mask-hero-bottom` + 左侧 `--gf-mask-hero-left`（仅 ≥ 768px）
- 内容容器宽度：`min(640px, 50%)`

### 文字层（左下）

| 元素 | 字号 | 字重 | 颜色 |
|---|---|---|---|
| 影片标题 | `--gf-fs-hero` | `--gf-fw-black` | `--gf-text-primary` |
| 标签行（年份 · 类型 · 地区） | `--gf-fs-md` | `--gf-fw-medium` | `--gf-text-secondary` |
| 剧情简介（最多 3 行） | `--gf-fs-md` | `--gf-fw-regular` | `--gf-text-secondary`，行高 `--gf-lh-relaxed` |

### CTA 按钮组
- 间距 `--gf-space-3`，flex 横排（移动端可堆叠）
- 主按钮"立即播放"：见下方 Button.Primary
- 次按钮"详情"：Button.Ghost

### 指示器
- 底部居中圆点（5 个），单个 8×8px，圆角 full
- Active 宽度变为 24px，背景 `--gf-brand-gradient`
- 切换间隔 6s，hover 暂停

### 断点行为
- 移动：CTA 按钮全宽，文字层底部居中
- 平板：CTA 按钮自适应宽度，文字左对齐
- 桌面：完整左下信息层 + 右侧渐变蒙版露出海报

---

## 3. FilmRow 横向滚动剧集行

### 结构
区块标题 → 横滚卡片列表 → 左右遮罩 + 箭头按钮

### 尺寸

| 元素 | 值 |
|---|---|
| 区块标题 | `--gf-fs-xl` / `--gf-fw-bold` / 下边距 `--gf-space-4` |
| Row 与 Row 间距 | `--gf-space-8`（移动）/ `--gf-space-12`（桌面） |
| 卡片间距 | `--gf-space-3`（移动）/ `--gf-space-4`（桌面） |
| 横滚容器内边距左右 | 与页面 gutter 相同 |

### 左右遮罩
- 仅桌面（≥ 1024px）显示
- 宽度 64px，左侧 `--gf-mask-row-left`、右侧 `--gf-mask-row-right`
- 鼠标悬停 Row 时，箭头按钮显现（透明度 `0 → 1`，`var(--gf-dur-fast)`）

### 箭头按钮
- 尺寸 48×64px，垂直贴卡片高度
- 背景 `rgba(0,0,0,0.55)`，hover `rgba(0,0,0,0.8)`
- 圆角 `--gf-radius-md`
- 图标 24px，颜色 `--gf-text-primary`
- 触屏设备隐藏（依靠原生横滚 + 露出下一卡片提示可滚）

### 滚动行为
- CSS：`scroll-snap-type: x mandatory; scroll-snap-align: start`
- 单次箭头滚动：可见区域宽度的 80%
- 移动端隐藏滚动条，触摸惯性滑动
- 箭头到边界时自动禁用（`opacity: 0.3; pointer-events: none`）

---

## 4. FilmCard 影片卡片

### 尺寸（2:3 海报）

| 断点 | 卡片宽 | 高 |
|---|---|---|
| `< 480px` | `(100vw - 32px) / 2.2` ≈ 145px | `× 1.5 ≈ 218px` |
| `480 - 767px` | ≈ 132px | ≈ 198px |
| `768 - 1023px` | ≈ 152px | ≈ 228px |
| `1024 - 1439px` | ≈ 160px | ≈ 240px |
| `>= 1440px` | ≈ 168px | ≈ 252px |

### 默认状态

- 背景：海报图 `object-fit: cover`
- 圆角 `--gf-radius-lg`
- 阴影 `--gf-shadow-md`
- 角标（左上）：`年份`、`分类`、`地区`，背景 `rgba(0,0,0,0.7)`，圆角 `--gf-radius-sm`，字号 `--gf-fs-xs`，padding `2px 6px`
- 角标（右上，可选）：评分（红色 `--gf-brand-primary`）

### Hover 状态（仅 ≥ 1024px）
- `transform: scale(1.08)`
- 阴影 `--gf-shadow-hover`
- 浮现底部渐变 `--gf-mask-card-hover` + 标题 + 一行剧情简介
  - 标题：`--gf-fs-md` / `--gf-fw-semibold` / `--gf-text-primary`
  - 简介：`--gf-fs-sm` / `--gf-text-secondary` / 单行省略
- z-index 升至 5，避免被相邻卡片遮挡
- 过渡 `var(--gf-dur-base) var(--gf-ease-spring)`

### Touch / Active 状态（< 1024px）
- 不放大；按下 `scale(0.97)` 反馈，`var(--gf-dur-instant)`
- 标题始终显示在卡片下方（卡片下方留 `--gf-space-2` + 2 行标题）

### Focus 状态（键盘）
- 外发光 `--gf-shadow-focus-ring`

### 断点行为
- 移动 / 平板：卡片下方常驻显示 2 行标题（`--gf-fs-sm`）
- 桌面：卡片仅图，hover 时蒙版显示标题

---

## 5. FilmDetailHeader 详情页头部

### 结构
模糊海报背景 → 顶部蒙版 → 左 海报（2:3）+ 右 信息（标题 / 标签 / 简介 / CTA / 元数据）

### 尺寸

| 断点 | 整体高度 | 海报宽 | 布局 |
|---|---|---|---|
| `< 768px` | 自适应 | 居中 200×300 | 海报上、信息下 |
| `768 - 1023px` | 自适应 | 220×330 | 左海报右信息（gap 24px） |
| `>= 1024px` | min 480px | 280×420 | 左海报右信息（gap 48px） |
| `>= 1440px` | min 560px | 320×480 | gap 64px |

### 背景
- `background-image: url(海报)`
- `filter: blur(40px) brightness(0.4)`
- 上层叠加 `linear-gradient(180deg, rgba(11,11,15,0.5) 0%, rgba(11,11,15,1) 100%)`

### 海报卡
- 圆角 `--gf-radius-lg`
- 阴影 `--gf-shadow-xl`
- 海报右下角浮"播放"图标按钮 56×56px（仅移动端，桌面用右侧大 CTA）

### 信息区

| 元素 | 字号 | 颜色 |
|---|---|---|
| 主标题 | `--gf-fs-3xl`（桌面 `--gf-fs-hero` 的下限 40px） | `--gf-text-primary` |
| 副标题（英文 / 别名） | `--gf-fs-md` | `--gf-text-muted` |
| 标签行（评分 · 年份 · 时长 · 类型 · 地区 · 语言） | `--gf-fs-sm` | `--gf-text-secondary`，分隔符 ` · ` |
| 简介（最多 5 行 + 展开） | `--gf-fs-md` / `--gf-lh-relaxed` | `--gf-text-secondary` |
| 元数据（导演 / 主演） | `--gf-fs-sm` | 标签 `--gf-text-muted`，值 `--gf-text-primary` |

### CTA 按钮组
- "立即播放"（Button.Primary，红色实心，左侧播放图标）
- "加入收藏"（Button.Outline）
- "分享"（Button.Ghost，仅图标，桌面显示）

---

## 6. EpisodeTabGroup 播放源 + 集数

### Tab（播放源切换）

- 高度 48px
- 字号 `--gf-fs-md` / `--gf-fw-medium`
- 默认 `--gf-text-secondary`，active `--gf-text-primary`
- Active 下方 2px 渐变线 `--gf-brand-gradient`
- 间距 `--gf-space-6`
- 横向可滚动（多源时），左右遮罩
- 触控目标 ≥ 44px

### 集数网格

| 断点 | 列数 | 每格尺寸 |
|---|---|---|
| `< 480px` | 4 列 | (100% / 4) × 48px 高 |
| `480 - 767px` | 5 列 | × 52px |
| `768 - 1023px` | 6 列 | × 52px |
| `1024 - 1439px` | 8 列 | × 56px |
| `>= 1440px` | 10 列 | × 56px |

- 每格圆角 `--gf-radius-md`
- 默认背景 `--gf-bg-elevated`，文字 `--gf-text-secondary`
- Hover：背景 `rgba(255,255,255,0.08)`，文字 `--gf-text-primary`
- Active（当前播放）：背景 `--gf-brand-gradient`，文字 `#fff`，阴影 `--gf-shadow-purple-glow`
- Visited（已观看，cookie）：左上小圆点 6×6 `--gf-success`
- 字号 `--gf-fs-sm` / `--gf-fw-semibold`
- gap `--gf-space-2`（移动）/ `--gf-space-3`（桌面）

---

## 7. SearchBar 搜索框

### 默认（桌面 inline）
- 高度 40px / 44px（`>= 1440px`）
- 背景 `rgba(255,255,255,0.08)`
- 圆角 `--gf-radius-full`
- 边框 `1px solid var(--gf-border-default)`
- 字号 `--gf-fs-md`
- 左侧放大镜图标 18px，左 padding `--gf-space-4`
- 占位文字 "搜影片 / 演员 / 导演..."

### Focus 状态
- 边框 `--gf-border-strong`
- 阴影外发光 `--gf-shadow-focus-ring`
- 背景 `rgba(255,255,255,0.12)`
- 过渡 `var(--gf-dur-fast)`

### 移动全屏搜索
- 触发：点击 Header 放大镜按钮
- 顶部固定输入区高度 64px，背景 `--gf-bg-elevated`
- 下方实时搜索建议列表（每项 56px 高，左图右文）

---

## 8. AppFooter

### 结构
极简：左 站点信息（站名 + 备案号）｜ 右 友情链接 / 关于
高度 ≥ 120px，背景 `--gf-bg-base`，顶部 `1px solid var(--gf-border-subtle)`
字号 `--gf-fs-sm`，颜色 `--gf-text-muted`
内边距上下 `--gf-space-8`，左右与页面 gutter 一致
移动端：堆叠居中，桌面：左右两栏

---

## 9. Button 通用按钮

### 尺寸（高度）

| Size | 高 | 内边距（左右） | 字号 |
|---|---|---|---|
| sm | 32px | 12px | `--gf-fs-sm` |
| md | 40px | 16px | `--gf-fs-md` |
| lg | 48px | 24px | `--gf-fs-md` |
| xl（Hero CTA） | 56px | 32px | `--gf-fs-lg` |

圆角统一 `--gf-radius-md`，HeroCTA 用 `--gf-radius-xl`。

### 变体

| 变体 | 默认 | Hover | Active | Disabled |
|---|---|---|---|---|
| Primary | bg `--gf-brand-primary`，text `#fff` | bg `--gf-brand-primary-hover` + `--gf-shadow-brand-glow` | bg `--gf-brand-primary-active` | opacity 0.4 |
| Gradient | bg `--gf-brand-gradient`，text `#fff` | bg `--gf-brand-gradient-hover` + `--gf-shadow-purple-glow` | bg 同 hover，scale(0.98) | opacity 0.4 |
| Outline | bg transparent，border `--gf-border-strong`，text `--gf-text-primary` | bg `rgba(255,255,255,0.08)` | bg `rgba(255,255,255,0.12)` | opacity 0.4 |
| Ghost | bg transparent，text `--gf-text-secondary` | bg `rgba(255,255,255,0.06)`，text `--gf-text-primary` | bg `rgba(255,255,255,0.1)` | opacity 0.4 |
| Danger | bg `--gf-danger`，text `#fff` | brightness(1.1) | brightness(0.9) | opacity 0.4 |

Focus（键盘）：所有变体 `box-shadow: --gf-shadow-focus-ring`。
触控目标最小 44×44px：尺寸不足时给外层 padding（视觉不变，命中区扩大）。

---

## 10. Login 登录页

### 整体
- 全屏背景：影片海报合集 / 视频静音循环（保留 `managebg.png` 作为兜底）
- 上层蒙版 `linear-gradient(135deg, rgba(11,11,15,0.85) 0%, rgba(155,73,231,0.35) 100%)`

### 玻璃卡片
- 居中，宽度：移动 100% - 32px / 平板 420px / 桌面 480px
- 背景 `--gf-bg-glass`
- `backdrop-filter: blur(24px) saturate(140%)`
- 圆角 `--gf-radius-2xl`
- 内边距 `--gf-space-8`
- 阴影 `--gf-shadow-xl` + `--gf-shadow-purple-glow`
- 顶部边框光：`border: 1px solid rgba(255,255,255,0.08); border-top-color: rgba(155,73,231,0.4)`

### 表单
- Logo + 副标题居中，距顶 `--gf-space-6`
- 输入框（账号 / 密码）：见 Input 规范，间距 `--gf-space-4`
- "记住我" + "忘记密码" 同行
- 登录按钮：Button.Gradient，全宽，size lg，圆角 `--gf-radius-md`

### 断点行为
- 移动：卡片占满宽度，背景媒体降低分辨率
- 平板 / 桌面：卡片居中，背景视频自动播放
- 桌面 ≥ 1440px：卡片偏右 1/3 处，左侧露出更多视觉

---

## 11. Input 输入框

| 属性 | 值 |
|---|---|
| 高度 | 44px（移动）/ 48px（桌面） |
| 圆角 | `--gf-radius-md` |
| 背景 | `rgba(255,255,255,0.06)` |
| 边框 | `1px solid var(--gf-border-default)` |
| 文字 | `--gf-text-primary` / `--gf-fs-md` |
| 占位 | `--gf-text-muted` |
| 内边距 | `0 --gf-space-4` |

状态：
- Hover：边框 `--gf-border-strong`
- Focus：边框 `--gf-brand-cyan`，外发光 `--gf-shadow-focus-ring`
- Error：边框 `--gf-danger`，下方提示 `--gf-fs-sm` `--gf-danger`
- Disabled：opacity 0.4，cursor not-allowed

---

## 12. 后台 Sidebar 侧边栏

### 尺寸

| 断点 | 宽度 | 折叠态 |
|---|---|---|
| `< 1024px` | 抽屉式，280px，从左滑入 | 默认隐藏，汉堡按钮触发 |
| `>= 1024px` | 240px | 可折叠到 64px（仅图标） |

### 样式
- 背景 `--gf-bg-surface`
- 右侧边框 `1px solid var(--gf-border-subtle)`
- 顶部 Logo 区高度 64px

### 菜单项
- 高度 48px
- 内边距左 `--gf-space-5`，gap 图标-文字 `--gf-space-3`
- 图标 20px，字号 `--gf-fs-md`
- 默认 `--gf-text-secondary`
- Hover：背景 `rgba(155,73,231,0.08)`，文字 `--gf-text-primary`
- Active：背景 `rgba(155,73,231,0.18)`，文字 `--gf-text-primary`，左侧 3px 渐变条 `--gf-brand-gradient`
- 子菜单：缩进 `--gf-space-6`，字号 `--gf-fs-sm`，高度 40px

---

## 13. 后台数据表格

### 容器
- 背景 `--gf-bg-surface`
- 圆角 `--gf-radius-lg`
- 内边距 `--gf-space-5`

### 表头
- 高度 48px
- 背景 `rgba(255,255,255,0.04)`
- 文字 `--gf-fs-sm` / `--gf-fw-semibold` / `--gf-text-secondary` / 大写字间距 `--gf-tracking-wider`
- 底部 `1px solid var(--gf-border-default)`

### 数据行
- 高度 56px
- 文字 `--gf-fs-sm` / `--gf-text-primary`
- 行间分隔 `1px solid var(--gf-border-subtle)`
- Hover 行：背景 `rgba(155,73,231,0.06)`
- 选中行（checkbox）：背景 `rgba(155,73,231,0.12)`

### 操作列按钮
- 链接式 `--gf-text-link`，hover `--gf-text-link-hover`
- 危险操作"删除"用 `--gf-danger`
- 间距 `--gf-space-3`

### 断点行为
- `< 768px`：表格转换为卡片列表（每行 → 一张卡片，关键字段竖排）
- `768 - 1023px`：表格横向滚动，固定首列
- `>= 1024px`：完整表格

---

## 14. Empty / Loading 状态

### Empty 空状态
- 居中布局，内边距上下 `--gf-space-12`
- 插画 / 图标 120×120（保留 `404.png` 风格）
- 主文案 `--gf-fs-lg` / `--gf-fw-semibold` / `--gf-text-primary`
- 副文案 `--gf-fs-md` / `--gf-text-muted` / 上间距 `--gf-space-2`
- CTA 按钮（可选） Button.Outline，上间距 `--gf-space-6`

### Loading 加载状态
- **骨架屏**（首屏 / 列表加载）：
  - 占位块背景 `linear-gradient(90deg, rgba(255,255,255,0.04) 0%, rgba(255,255,255,0.08) 50%, rgba(255,255,255,0.04) 100%)`
  - 闪烁动画 1.4s 无限循环
  - 圆角与目标元素一致
- **Spinner**（按钮 / 弹窗内）：
  - 24px 圆环，渐变 `--gf-brand-gradient`，旋转 1s 线性
- **页面级**：居中 spinner 64px + 下方文字 `--gf-text-muted`

---

## 15. Pagination 分页

### 样式
- 居中横排，间距 `--gf-space-2`
- 单个 chip：高 36px / 最小宽 36px / 圆角 `--gf-radius-full`
- 默认背景 `--gf-bg-elevated`，文字 `--gf-text-secondary`
- Hover：背景 `rgba(255,255,255,0.1)`，文字 `--gf-text-primary`
- Active（当前页）：背景 `--gf-brand-gradient`，文字 `#fff`，阴影 `--gf-shadow-purple-glow`
- Disabled（首尾）：opacity 0.3，不可点

### 断点
- 移动：仅显示"上一页 / 当前页/总页 / 下一页"3 个元素
- 平板：显示 5 个页码 + 首尾
- 桌面：显示 7 个页码 + 首尾 + 跳转输入框（可选）

---

## 16. 通用：断点行为速查

| 组件 | < 768 | 768-1023 | 1024-1439 | >= 1440 |
|---|---|---|---|---|
| AppHeader | 56h，导航折叠 | 64h | 72h | 72h，居中 1280 容器 |
| HeroCarousel | 60vh，文字底居中 | 65vh，左下 | 70vh，左下完整 | 同左 |
| FilmRow 列数 | 2.2 / 3.2 | 4.5 | 6 | 7 / 8 |
| FilmCard | 标题常驻显示 | 同 | hover 显示标题 | 同 |
| EpisodeTab 集数 | 4 列 | 6 列 | 8 列 | 10 列 |
| Sidebar | 抽屉式 | 抽屉式 | 240px 固定 | 240px，可折叠 64px |
| Table | 转卡片列表 | 横滚 | 完整表格 | 完整表格 |
| Pagination | 上下页 + 当前 | 5 页码 | 7 页码 | 7 页码 + 跳转 |
