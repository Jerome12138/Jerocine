# Jerocine 设计 Token 规范（暗色主题）

> 适用于 `web/` 用户端 + 管理端，Netflix / Disney+ 视觉基调，品紫渐变为品牌强调色。所有 Token 以 CSS 自定义属性输出，前缀统一 `--jc-`（历史沿用，代码中已大量引用，勿改名）。

---

## 0. 设计基调摘要

- 主背景偏黑（`#0b0b0f` 系），层次靠 surface / elevated 提亮
- 主色：**Netflix 红 `#E50914`** 作为 CTA / 高优先操作色
- 辅助强调：**品紫渐变 `#9b49e7 → #4ad1e5`**（品牌强调色，用于 Logo / 进度条 / 后台高亮 / 登录 CTA）
- 卡片不带边框靠阴影区分，hover 时整体 1.08 放大 + 加深阴影
- 文字最低 14px（管理端表格）/ 16px（用户端阅读）

---

## 1. 颜色 Token

### 1.1 背景层级（自下而上）

| Token | 值 | 用途 |
|---|---|---|
| `--jc-bg-base` | `#0b0b0f` | 页面最底层背景（body） |
| `--jc-bg-surface` | `#141518` | 卡片 / 容器默认背景 |
| `--jc-bg-elevated` | `#1c1d22` | 浮起卡片 / hover 后卡片 / 弹窗 |
| `--jc-bg-overlay` | `rgba(0, 0, 0, 0.72)` | 模态遮罩 / Hero 渐变蒙版 |
| `--jc-bg-glass` | `rgba(20, 21, 24, 0.55)` | 玻璃拟态卡片背景（搭配 backdrop-filter: blur(18px)） |
| `--jc-bg-header` | `rgba(11, 11, 15, 0)` | AppHeader 顶部默认透明 |
| `--jc-bg-header-scrolled` | `rgba(11, 11, 15, 0.92)` | AppHeader 滚动后实色 |

### 1.2 文本

| Token | 值 | 用途 |
|---|---|---|
| `--jc-text-primary` | `#FFFFFF` | 主要标题 / 主文本 |
| `--jc-text-secondary` | `rgba(255, 255, 255, 0.78)` | 次要文本 / 描述 |
| `--jc-text-muted` | `rgba(255, 255, 255, 0.55)` | 辅助标签 / 占位 |
| `--jc-text-disabled` | `rgba(255, 255, 255, 0.32)` | 禁用文本 |
| `--jc-text-inverse` | `#0b0b0f` | 浅色背景上的深色文字 |
| `--jc-text-link` | `#4ad1e5` | 链接 / 可点击文本 |
| `--jc-text-link-hover` | `#9b49e7` | 链接 hover |

### 1.3 边框 / 分隔

| Token | 值 | 用途 |
|---|---|---|
| `--jc-border-subtle` | `rgba(255, 255, 255, 0.06)` | 卡片间细微分割 |
| `--jc-border-default` | `rgba(255, 255, 255, 0.12)` | 输入框 / Tab 默认边框 |
| `--jc-border-strong` | `rgba(255, 255, 255, 0.24)` | 焦点 / 强调边框 |
| `--jc-border-brand` | `#E50914` | 品牌色边框 |

### 1.4 状态色

| Token | 值 | 用途 |
|---|---|---|
| `--jc-success` | `#22c55e` | 成功提示 / 在线状态 |
| `--jc-success-soft` | `rgba(34, 197, 94, 0.16)` | 成功背景 |
| `--jc-warning` | `#f59e0b` | 警告 |
| `--jc-warning-soft` | `rgba(245, 158, 11, 0.16)` | 警告背景 |
| `--jc-danger` | `#ef4444` | 错误 / 删除 |
| `--jc-danger-soft` | `rgba(239, 68, 68, 0.16)` | 错误背景 |
| `--jc-info` | `#3b82f6` | 信息提示 |
| `--jc-info-soft` | `rgba(59, 130, 246, 0.16)` | 信息背景 |

### 1.5 品牌主色

| Token | 值 | 用途 |
|---|---|---|
| `--jc-brand-primary` | `#E50914` | Netflix 红，主 CTA / Logo 强调 |
| `--jc-brand-primary-hover` | `#FF1F2C` | 主 CTA hover |
| `--jc-brand-primary-active` | `#B0060F` | 主 CTA active |
| `--jc-brand-purple` | `#9b49e7` | 渐变起点 / 后台主色 |
| `--jc-brand-cyan` | `#4ad1e5` | 渐变终点 / 链接强调 |
| `--jc-brand-gradient` | `linear-gradient(135deg, #9b49e7 0%, #4ad1e5 100%)` | Logo / 后台 Sidebar Active / 登录 CTA |
| `--jc-brand-gradient-hover` | `linear-gradient(135deg, #b366f5 0%, #6ee0f0 100%)` | 渐变 hover |
| `--jc-brand-gradient-text` | `linear-gradient(135deg, #9b49e7 0%, #4ad1e5 100%)` | 文字渐变（搭配 `background-clip: text`） |

### 1.6 Hero / 卡片渐变蒙版

| Token | 值 |
|---|---|
| `--jc-mask-hero-bottom` | `linear-gradient(180deg, rgba(11,11,15,0) 0%, rgba(11,11,15,0.55) 60%, rgba(11,11,15,1) 100%)` |
| `--jc-mask-hero-left` | `linear-gradient(90deg, rgba(11,11,15,0.92) 0%, rgba(11,11,15,0.6) 35%, rgba(11,11,15,0) 70%)` |
| `--jc-mask-row-left` | `linear-gradient(90deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%)` |
| `--jc-mask-row-right` | `linear-gradient(270deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%)` |
| `--jc-mask-card-hover` | `linear-gradient(180deg, rgba(0,0,0,0) 50%, rgba(0,0,0,0.85) 100%)` |

---

## 2. 字体 Token

### 2.1 Family

```
--jc-font-sans: "Inter", "Helvetica Neue", "PingFang SC", "Microsoft YaHei",
                "Hiragino Sans GB", "Noto Sans CJK SC", system-ui, sans-serif;
--jc-font-display: "Inter", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
--jc-font-mono:  "JetBrains Mono", "Fira Code", "SFMono-Regular", Consolas, monospace;
```

### 2.2 字号阶梯（rem，root = 16px）

| Token | 值 | px | 用途 |
|---|---|---|---|
| `--jc-fs-xs` | `0.75rem` | 12px | 极小标签（管理端徽标） |
| `--jc-fs-sm` | `0.875rem` | 14px | 后台表格 / 二级辅助 |
| `--jc-fs-base` | `1rem` | 16px | 用户端正文最小值 |
| `--jc-fs-md` | `1.125rem` | 18px | 卡片标题 / 表单输入 |
| `--jc-fs-lg` | `1.25rem` | 20px | 区块标题 / 按钮文字 |
| `--jc-fs-xl` | `1.5rem` | 24px | 页面副标题 |
| `--jc-fs-2xl` | `1.875rem` | 30px | 详情页主标题 |
| `--jc-fs-3xl` | `2.5rem` | 40px | Hero 标题（移动） |
| `--jc-fs-hero` | `clamp(2.5rem, 4vw + 1rem, 4.5rem)` | 40-72px | Hero 标题（响应式） |

### 2.3 字重

| Token | 值 |
|---|---|
| `--jc-fw-regular` | `400` |
| `--jc-fw-medium` | `500` |
| `--jc-fw-semibold` | `600` |
| `--jc-fw-bold` | `700` |
| `--jc-fw-black` | `900`（Hero 标题专用） |

### 2.4 行高

| Token | 值 | 用途 |
|---|---|---|
| `--jc-lh-tight` | `1.15` | Hero / 大标题 |
| `--jc-lh-snug` | `1.3` | 卡片标题 |
| `--jc-lh-normal` | `1.5` | 正文 |
| `--jc-lh-relaxed` | `1.7` | 长描述 / 剧情简介 |

### 2.5 字间距

| Token | 值 |
|---|---|
| `--jc-tracking-tight` | `-0.02em` |
| `--jc-tracking-normal` | `0` |
| `--jc-tracking-wide` | `0.05em` |
| `--jc-tracking-wider` | `0.12em`（小写英文标签） |

---

## 3. 间距系统（4-pt 基准）

| Token | 值 | 典型用途 |
|---|---|---|
| `--jc-space-0` | `0` | 重置 |
| `--jc-space-1` | `4px` | 图标与文字 |
| `--jc-space-2` | `8px` | 紧凑控件内边距 |
| `--jc-space-3` | `12px` | 表单控件 padding |
| `--jc-space-4` | `16px` | 默认 gap / 卡片内边距 |
| `--jc-space-5` | `20px` | 卡片内边距（宽松） |
| `--jc-space-6` | `24px` | 区块内 gap |
| `--jc-space-8` | `32px` | Row 与 Row 之间 |
| `--jc-space-10` | `40px` | 页面段落间距 |
| `--jc-space-12` | `48px` | 大区块分隔 |
| `--jc-space-16` | `64px` | 页面主区段间距 |

### 3.1 容器宽度（页面边距）

| 断点 | 容器最大宽 | 左右 gutter |
|---|---|---|
| `< 768px` | 100% | 16px |
| `768 - 1023px` | 100% | 24px |
| `1024 - 1439px` | 100% | 40px |
| `1440 - 1919px` | `1280px`（中央） | auto |
| `>= 1920px` | `1600px`（中央） | auto |

CSS 变量：
```
--jc-container-max: 1280px;
--jc-container-max-2xl: 1600px;
--jc-gutter-mobile: 16px;
--jc-gutter-tablet: 24px;
--jc-gutter-desktop: 40px;
```

---

## 4. 圆角

| Token | 值 | 用途 |
|---|---|---|
| `--jc-radius-none` | `0` | 表格 / Hero |
| `--jc-radius-sm` | `4px` | 徽标 / 小标签 |
| `--jc-radius-md` | `8px` | 输入框 / 按钮 |
| `--jc-radius-lg` | `12px` | 卡片 / 弹窗 |
| `--jc-radius-xl` | `20px` | 玻璃卡 / Hero CTA |
| `--jc-radius-2xl` | `28px` | 大卡片 / 登录卡 |
| `--jc-radius-full` | `9999px` | Pagination chip / 头像 / 搜索框 |

---

## 5. 阴影（暗色优化）

> 暗色主题阴影偏向于"光晕 + 黑边"组合，hover 提升明显。

| Token | 值 | 用途 |
|---|---|---|
| `--jc-shadow-sm` | `0 1px 2px rgba(0,0,0,0.4)` | 输入框 / 小卡片 |
| `--jc-shadow-md` | `0 4px 12px rgba(0,0,0,0.5)` | 默认卡片 |
| `--jc-shadow-lg` | `0 12px 32px rgba(0,0,0,0.6)` | 浮起卡片 / 弹窗 |
| `--jc-shadow-xl` | `0 24px 60px rgba(0,0,0,0.75)` | 全屏弹层 / Hero CTA |
| `--jc-shadow-hover` | `0 18px 40px rgba(0,0,0,0.7), 0 0 0 1px rgba(255,255,255,0.06)` | 卡片 hover 提升 |
| `--jc-shadow-brand-glow` | `0 0 0 4px rgba(229, 9, 20, 0.25)` | CTA focus / active 光晕 |
| `--jc-shadow-purple-glow` | `0 0 24px rgba(155, 73, 231, 0.45)` | 后台 / 登录强调光晕 |
| `--jc-shadow-focus-ring` | `0 0 0 3px rgba(74, 209, 229, 0.6)` | 表单 focus（无障碍） |

---

## 6. 断点

```
--jc-bp-sm:  360px;   /* 小屏手机 */
--jc-bp-md:  768px;   /* 平板竖 / 大屏手机横 */
--jc-bp-lg:  1024px;  /* 平板横 / 小屏笔记本 */
--jc-bp-xl:  1440px;  /* 桌面 */
--jc-bp-2xl: 1920px;  /* 大屏 / 4K 缩放后 */
```

媒体查询示例（mobile-first）：
```
@media (min-width: 768px)  { /* md+ */ }
@media (min-width: 1024px) { /* lg+ */ }
@media (min-width: 1440px) { /* xl+ */ }
@media (min-width: 1920px) { /* 2xl+ */ }
```

### 6.1 各断点 Row 卡片列数（FilmCard 2:3）

| 断点 | FilmRow 可见列数 | 卡片宽度策略 |
|---|---|---|
| `< 480px` | 2.2 列（露出下一张） | `(100vw - 32px) / 2.2` |
| `480 - 767px` | 3.2 列 | `(100vw - 32px) / 3.2` |
| `768 - 1023px` | 4.5 列 | `(100vw - 48px) / 4.5` |
| `1024 - 1439px` | 6 列 | `(100vw - 80px) / 6` |
| `1440 - 1919px` | 7 列 | 容器 1280 / 7 |
| `>= 1920px` | 8 列 | 容器 1600 / 8 |

---

## 7. 动画

### 7.1 缓动函数

| Token | 值 | 用途 |
|---|---|---|
| `--jc-ease-standard` | `cubic-bezier(0.4, 0, 0.2, 1)` | 大多数过渡 |
| `--jc-ease-out` | `cubic-bezier(0.16, 1, 0.3, 1)` | 进场（淡入 + 上移） |
| `--jc-ease-in` | `cubic-bezier(0.7, 0, 0.84, 0)` | 退场 |
| `--jc-ease-spring` | `cubic-bezier(0.34, 1.56, 0.64, 1)` | 卡片 hover 弹性放大 |
| `--jc-ease-linear` | `linear` | 进度条 / loading |

### 7.2 时长

| Token | 值 | 用途 |
|---|---|---|
| `--jc-dur-instant` | `80ms` | 按钮按下 |
| `--jc-dur-fast` | `150ms` | 颜色 / 透明度切换 |
| `--jc-dur-base` | `250ms` | 卡片放大 / hover |
| `--jc-dur-slow` | `400ms` | 弹窗 / 抽屉 |
| `--jc-dur-page` | `500ms` | 页面切换淡入 |

### 7.3 关键动画规范

- **页面切换**：`opacity 0 → 1` + `translateY(8px → 0)`，`var(--jc-dur-page) var(--jc-ease-out)`
- **卡片 hover 放大**：`scale(1) → scale(1.08)` + 阴影提升，`var(--jc-dur-base) var(--jc-ease-spring)`
- **AppHeader 背景切换**：`background-color` + `backdrop-filter`，`var(--jc-dur-base) var(--jc-ease-standard)`
- **FilmRow 横滚**：`transform: translateX()`，`var(--jc-dur-slow) var(--jc-ease-out)`
- **骨架屏闪烁**：`opacity 0.4 ↔ 0.8`，`1.4s var(--jc-ease-linear) infinite`
- **prefers-reduced-motion**：所有 > 200ms 的动画自动降级为 80ms，禁用 scale 放大

---

## 8. 层级（z-index）

| Token | 值 | 用途 |
|---|---|---|
| `--jc-z-base` | `0` | 默认 |
| `--jc-z-row` | `10` | 横滚 Row 的箭头 |
| `--jc-z-header` | `100` | AppHeader 固定 |
| `--jc-z-dropdown` | `200` | 下拉菜单 |
| `--jc-z-overlay` | `900` | 模态遮罩 |
| `--jc-z-modal` | `1000` | 弹窗 |
| `--jc-z-toast` | `1100` | Toast / Notification |
| `--jc-z-tooltip` | `1200` | Tooltip |

---

## 9. 完整 CSS 变量清单（速查）

```css
:root {
  /* 背景 */
  --jc-bg-base: #0b0b0f;
  --jc-bg-surface: #141518;
  --jc-bg-elevated: #1c1d22;
  --jc-bg-overlay: rgba(0, 0, 0, 0.72);
  --jc-bg-glass: rgba(20, 21, 24, 0.55);
  --jc-bg-header: rgba(11, 11, 15, 0);
  --jc-bg-header-scrolled: rgba(11, 11, 15, 0.92);

  /* 文本 */
  --jc-text-primary: #FFFFFF;
  --jc-text-secondary: rgba(255, 255, 255, 0.78);
  --jc-text-muted: rgba(255, 255, 255, 0.55);
  --jc-text-disabled: rgba(255, 255, 255, 0.32);
  --jc-text-inverse: #0b0b0f;
  --jc-text-link: #4ad1e5;
  --jc-text-link-hover: #9b49e7;

  /* 边框 */
  --jc-border-subtle: rgba(255, 255, 255, 0.06);
  --jc-border-default: rgba(255, 255, 255, 0.12);
  --jc-border-strong: rgba(255, 255, 255, 0.24);
  --jc-border-brand: #E50914;

  /* 状态 */
  --jc-success: #22c55e;
  --jc-success-soft: rgba(34, 197, 94, 0.16);
  --jc-warning: #f59e0b;
  --jc-warning-soft: rgba(245, 158, 11, 0.16);
  --jc-danger: #ef4444;
  --jc-danger-soft: rgba(239, 68, 68, 0.16);
  --jc-info: #3b82f6;
  --jc-info-soft: rgba(59, 130, 246, 0.16);

  /* 品牌 */
  --jc-brand-primary: #E50914;
  --jc-brand-primary-hover: #FF1F2C;
  --jc-brand-primary-active: #B0060F;
  --jc-brand-purple: #9b49e7;
  --jc-brand-cyan: #4ad1e5;
  --jc-brand-gradient: linear-gradient(135deg, #9b49e7 0%, #4ad1e5 100%);
  --jc-brand-gradient-hover: linear-gradient(135deg, #b366f5 0%, #6ee0f0 100%);

  /* 蒙版 */
  --jc-mask-hero-bottom: linear-gradient(180deg, rgba(11,11,15,0) 0%, rgba(11,11,15,0.55) 60%, rgba(11,11,15,1) 100%);
  --jc-mask-hero-left: linear-gradient(90deg, rgba(11,11,15,0.92) 0%, rgba(11,11,15,0.6) 35%, rgba(11,11,15,0) 70%);
  --jc-mask-row-left: linear-gradient(90deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%);
  --jc-mask-row-right: linear-gradient(270deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%);
  --jc-mask-card-hover: linear-gradient(180deg, rgba(0,0,0,0) 50%, rgba(0,0,0,0.85) 100%);

  /* 字体 */
  --jc-font-sans: "Inter", "Helvetica Neue", "PingFang SC", "Microsoft YaHei", "Hiragino Sans GB", "Noto Sans CJK SC", system-ui, sans-serif;
  --jc-font-display: "Inter", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  --jc-font-mono: "JetBrains Mono", "Fira Code", "SFMono-Regular", Consolas, monospace;

  --jc-fs-xs: 0.75rem;
  --jc-fs-sm: 0.875rem;
  --jc-fs-base: 1rem;
  --jc-fs-md: 1.125rem;
  --jc-fs-lg: 1.25rem;
  --jc-fs-xl: 1.5rem;
  --jc-fs-2xl: 1.875rem;
  --jc-fs-3xl: 2.5rem;
  --jc-fs-hero: clamp(2.5rem, 4vw + 1rem, 4.5rem);

  --jc-fw-regular: 400;
  --jc-fw-medium: 500;
  --jc-fw-semibold: 600;
  --jc-fw-bold: 700;
  --jc-fw-black: 900;

  --jc-lh-tight: 1.15;
  --jc-lh-snug: 1.3;
  --jc-lh-normal: 1.5;
  --jc-lh-relaxed: 1.7;

  --jc-tracking-tight: -0.02em;
  --jc-tracking-normal: 0;
  --jc-tracking-wide: 0.05em;
  --jc-tracking-wider: 0.12em;

  /* 间距 */
  --jc-space-0: 0;
  --jc-space-1: 4px;
  --jc-space-2: 8px;
  --jc-space-3: 12px;
  --jc-space-4: 16px;
  --jc-space-5: 20px;
  --jc-space-6: 24px;
  --jc-space-8: 32px;
  --jc-space-10: 40px;
  --jc-space-12: 48px;
  --jc-space-16: 64px;

  /* 容器 */
  --jc-container-max: 1280px;
  --jc-container-max-2xl: 1600px;
  --jc-gutter-mobile: 16px;
  --jc-gutter-tablet: 24px;
  --jc-gutter-desktop: 40px;

  /* 圆角 */
  --jc-radius-none: 0;
  --jc-radius-sm: 4px;
  --jc-radius-md: 8px;
  --jc-radius-lg: 12px;
  --jc-radius-xl: 20px;
  --jc-radius-2xl: 28px;
  --jc-radius-full: 9999px;

  /* 阴影 */
  --jc-shadow-sm: 0 1px 2px rgba(0,0,0,0.4);
  --jc-shadow-md: 0 4px 12px rgba(0,0,0,0.5);
  --jc-shadow-lg: 0 12px 32px rgba(0,0,0,0.6);
  --jc-shadow-xl: 0 24px 60px rgba(0,0,0,0.75);
  --jc-shadow-hover: 0 18px 40px rgba(0,0,0,0.7), 0 0 0 1px rgba(255,255,255,0.06);
  --jc-shadow-brand-glow: 0 0 0 4px rgba(229, 9, 20, 0.25);
  --jc-shadow-purple-glow: 0 0 24px rgba(155, 73, 231, 0.45);
  --jc-shadow-focus-ring: 0 0 0 3px rgba(74, 209, 229, 0.6);

  /* 断点（仅供 JS 读取） */
  --jc-bp-sm: 360px;
  --jc-bp-md: 768px;
  --jc-bp-lg: 1024px;
  --jc-bp-xl: 1440px;
  --jc-bp-2xl: 1920px;

  /* 动画 */
  --jc-ease-standard: cubic-bezier(0.4, 0, 0.2, 1);
  --jc-ease-out: cubic-bezier(0.16, 1, 0.3, 1);
  --jc-ease-in: cubic-bezier(0.7, 0, 0.84, 0);
  --jc-ease-spring: cubic-bezier(0.34, 1.56, 0.64, 1);
  --jc-ease-linear: linear;

  --jc-dur-instant: 80ms;
  --jc-dur-fast: 150ms;
  --jc-dur-base: 250ms;
  --jc-dur-slow: 400ms;
  --jc-dur-page: 500ms;

  /* 层级 */
  --jc-z-base: 0;
  --jc-z-row: 10;
  --jc-z-header: 100;
  --jc-z-dropdown: 200;
  --jc-z-overlay: 900;
  --jc-z-modal: 1000;
  --jc-z-toast: 1100;
  --jc-z-tooltip: 1200;
}
```
