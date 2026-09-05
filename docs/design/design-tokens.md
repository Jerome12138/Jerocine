# Jerocine 设计 Token 规范（暗色主题）

> 适用于 `web/` 用户端 + 管理端，Netflix / Disney+ 视觉基调，品紫渐变为品牌强调色。所有 Token 以 CSS 自定义属性输出，前缀统一 `--gf-`（历史沿用，代码中已大量引用，勿改名）。

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
| `--gf-bg-base` | `#0b0b0f` | 页面最底层背景（body） |
| `--gf-bg-surface` | `#141518` | 卡片 / 容器默认背景 |
| `--gf-bg-elevated` | `#1c1d22` | 浮起卡片 / hover 后卡片 / 弹窗 |
| `--gf-bg-overlay` | `rgba(0, 0, 0, 0.72)` | 模态遮罩 / Hero 渐变蒙版 |
| `--gf-bg-glass` | `rgba(20, 21, 24, 0.55)` | 玻璃拟态卡片背景（搭配 backdrop-filter: blur(18px)） |
| `--gf-bg-header` | `rgba(11, 11, 15, 0)` | AppHeader 顶部默认透明 |
| `--gf-bg-header-scrolled` | `rgba(11, 11, 15, 0.92)` | AppHeader 滚动后实色 |

### 1.2 文本

| Token | 值 | 用途 |
|---|---|---|
| `--gf-text-primary` | `#FFFFFF` | 主要标题 / 主文本 |
| `--gf-text-secondary` | `rgba(255, 255, 255, 0.78)` | 次要文本 / 描述 |
| `--gf-text-muted` | `rgba(255, 255, 255, 0.55)` | 辅助标签 / 占位 |
| `--gf-text-disabled` | `rgba(255, 255, 255, 0.32)` | 禁用文本 |
| `--gf-text-inverse` | `#0b0b0f` | 浅色背景上的深色文字 |
| `--gf-text-link` | `#4ad1e5` | 链接 / 可点击文本 |
| `--gf-text-link-hover` | `#9b49e7` | 链接 hover |

### 1.3 边框 / 分隔

| Token | 值 | 用途 |
|---|---|---|
| `--gf-border-subtle` | `rgba(255, 255, 255, 0.06)` | 卡片间细微分割 |
| `--gf-border-default` | `rgba(255, 255, 255, 0.12)` | 输入框 / Tab 默认边框 |
| `--gf-border-strong` | `rgba(255, 255, 255, 0.24)` | 焦点 / 强调边框 |
| `--gf-border-brand` | `#E50914` | 品牌色边框 |

### 1.4 状态色

| Token | 值 | 用途 |
|---|---|---|
| `--gf-success` | `#22c55e` | 成功提示 / 在线状态 |
| `--gf-success-soft` | `rgba(34, 197, 94, 0.16)` | 成功背景 |
| `--gf-warning` | `#f59e0b` | 警告 |
| `--gf-warning-soft` | `rgba(245, 158, 11, 0.16)` | 警告背景 |
| `--gf-danger` | `#ef4444` | 错误 / 删除 |
| `--gf-danger-soft` | `rgba(239, 68, 68, 0.16)` | 错误背景 |
| `--gf-info` | `#3b82f6` | 信息提示 |
| `--gf-info-soft` | `rgba(59, 130, 246, 0.16)` | 信息背景 |

### 1.5 品牌主色

| Token | 值 | 用途 |
|---|---|---|
| `--gf-brand-primary` | `#E50914` | Netflix 红，主 CTA / Logo 强调 |
| `--gf-brand-primary-hover` | `#FF1F2C` | 主 CTA hover |
| `--gf-brand-primary-active` | `#B0060F` | 主 CTA active |
| `--gf-brand-purple` | `#9b49e7` | 渐变起点 / 后台主色 |
| `--gf-brand-cyan` | `#4ad1e5` | 渐变终点 / 链接强调 |
| `--gf-brand-gradient` | `linear-gradient(135deg, #9b49e7 0%, #4ad1e5 100%)` | Logo / 后台 Sidebar Active / 登录 CTA |
| `--gf-brand-gradient-hover` | `linear-gradient(135deg, #b366f5 0%, #6ee0f0 100%)` | 渐变 hover |
| `--gf-brand-gradient-text` | `linear-gradient(135deg, #9b49e7 0%, #4ad1e5 100%)` | 文字渐变（搭配 `background-clip: text`） |

### 1.6 Hero / 卡片渐变蒙版

| Token | 值 |
|---|---|
| `--gf-mask-hero-bottom` | `linear-gradient(180deg, rgba(11,11,15,0) 0%, rgba(11,11,15,0.55) 60%, rgba(11,11,15,1) 100%)` |
| `--gf-mask-hero-left` | `linear-gradient(90deg, rgba(11,11,15,0.92) 0%, rgba(11,11,15,0.6) 35%, rgba(11,11,15,0) 70%)` |
| `--gf-mask-row-left` | `linear-gradient(90deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%)` |
| `--gf-mask-row-right` | `linear-gradient(270deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%)` |
| `--gf-mask-card-hover` | `linear-gradient(180deg, rgba(0,0,0,0) 50%, rgba(0,0,0,0.85) 100%)` |

---

## 2. 字体 Token

### 2.1 Family

```
--gf-font-sans: "Inter", "Helvetica Neue", "PingFang SC", "Microsoft YaHei",
                "Hiragino Sans GB", "Noto Sans CJK SC", system-ui, sans-serif;
--gf-font-display: "Inter", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
--gf-font-mono:  "JetBrains Mono", "Fira Code", "SFMono-Regular", Consolas, monospace;
```

### 2.2 字号阶梯（rem，root = 16px）

| Token | 值 | px | 用途 |
|---|---|---|---|
| `--gf-fs-xs` | `0.75rem` | 12px | 极小标签（管理端徽标） |
| `--gf-fs-sm` | `0.875rem` | 14px | 后台表格 / 二级辅助 |
| `--gf-fs-base` | `1rem` | 16px | 用户端正文最小值 |
| `--gf-fs-md` | `1.125rem` | 18px | 卡片标题 / 表单输入 |
| `--gf-fs-lg` | `1.25rem` | 20px | 区块标题 / 按钮文字 |
| `--gf-fs-xl` | `1.5rem` | 24px | 页面副标题 |
| `--gf-fs-2xl` | `1.875rem` | 30px | 详情页主标题 |
| `--gf-fs-3xl` | `2.5rem` | 40px | Hero 标题（移动） |
| `--gf-fs-hero` | `clamp(2.5rem, 4vw + 1rem, 4.5rem)` | 40-72px | Hero 标题（响应式） |

### 2.3 字重

| Token | 值 |
|---|---|
| `--gf-fw-regular` | `400` |
| `--gf-fw-medium` | `500` |
| `--gf-fw-semibold` | `600` |
| `--gf-fw-bold` | `700` |
| `--gf-fw-black` | `900`（Hero 标题专用） |

### 2.4 行高

| Token | 值 | 用途 |
|---|---|---|
| `--gf-lh-tight` | `1.15` | Hero / 大标题 |
| `--gf-lh-snug` | `1.3` | 卡片标题 |
| `--gf-lh-normal` | `1.5` | 正文 |
| `--gf-lh-relaxed` | `1.7` | 长描述 / 剧情简介 |

### 2.5 字间距

| Token | 值 |
|---|---|
| `--gf-tracking-tight` | `-0.02em` |
| `--gf-tracking-normal` | `0` |
| `--gf-tracking-wide` | `0.05em` |
| `--gf-tracking-wider` | `0.12em`（小写英文标签） |

---

## 3. 间距系统（4-pt 基准）

| Token | 值 | 典型用途 |
|---|---|---|
| `--gf-space-0` | `0` | 重置 |
| `--gf-space-1` | `4px` | 图标与文字 |
| `--gf-space-2` | `8px` | 紧凑控件内边距 |
| `--gf-space-3` | `12px` | 表单控件 padding |
| `--gf-space-4` | `16px` | 默认 gap / 卡片内边距 |
| `--gf-space-5` | `20px` | 卡片内边距（宽松） |
| `--gf-space-6` | `24px` | 区块内 gap |
| `--gf-space-8` | `32px` | Row 与 Row 之间 |
| `--gf-space-10` | `40px` | 页面段落间距 |
| `--gf-space-12` | `48px` | 大区块分隔 |
| `--gf-space-16` | `64px` | 页面主区段间距 |

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
--gf-container-max: 1280px;
--gf-container-max-2xl: 1600px;
--gf-gutter-mobile: 16px;
--gf-gutter-tablet: 24px;
--gf-gutter-desktop: 40px;
```

---

## 4. 圆角

| Token | 值 | 用途 |
|---|---|---|
| `--gf-radius-none` | `0` | 表格 / Hero |
| `--gf-radius-sm` | `4px` | 徽标 / 小标签 |
| `--gf-radius-md` | `8px` | 输入框 / 按钮 |
| `--gf-radius-lg` | `12px` | 卡片 / 弹窗 |
| `--gf-radius-xl` | `20px` | 玻璃卡 / Hero CTA |
| `--gf-radius-2xl` | `28px` | 大卡片 / 登录卡 |
| `--gf-radius-full` | `9999px` | Pagination chip / 头像 / 搜索框 |

---

## 5. 阴影（暗色优化）

> 暗色主题阴影偏向于"光晕 + 黑边"组合，hover 提升明显。

| Token | 值 | 用途 |
|---|---|---|
| `--gf-shadow-sm` | `0 1px 2px rgba(0,0,0,0.4)` | 输入框 / 小卡片 |
| `--gf-shadow-md` | `0 4px 12px rgba(0,0,0,0.5)` | 默认卡片 |
| `--gf-shadow-lg` | `0 12px 32px rgba(0,0,0,0.6)` | 浮起卡片 / 弹窗 |
| `--gf-shadow-xl` | `0 24px 60px rgba(0,0,0,0.75)` | 全屏弹层 / Hero CTA |
| `--gf-shadow-hover` | `0 18px 40px rgba(0,0,0,0.7), 0 0 0 1px rgba(255,255,255,0.06)` | 卡片 hover 提升 |
| `--gf-shadow-brand-glow` | `0 0 0 4px rgba(229, 9, 20, 0.25)` | CTA focus / active 光晕 |
| `--gf-shadow-purple-glow` | `0 0 24px rgba(155, 73, 231, 0.45)` | 后台 / 登录强调光晕 |
| `--gf-shadow-focus-ring` | `0 0 0 3px rgba(74, 209, 229, 0.6)` | 表单 focus（无障碍） |

---

## 6. 断点

```
--gf-bp-sm:  360px;   /* 小屏手机 */
--gf-bp-md:  768px;   /* 平板竖 / 大屏手机横 */
--gf-bp-lg:  1024px;  /* 平板横 / 小屏笔记本 */
--gf-bp-xl:  1440px;  /* 桌面 */
--gf-bp-2xl: 1920px;  /* 大屏 / 4K 缩放后 */
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
| `--gf-ease-standard` | `cubic-bezier(0.4, 0, 0.2, 1)` | 大多数过渡 |
| `--gf-ease-out` | `cubic-bezier(0.16, 1, 0.3, 1)` | 进场（淡入 + 上移） |
| `--gf-ease-in` | `cubic-bezier(0.7, 0, 0.84, 0)` | 退场 |
| `--gf-ease-spring` | `cubic-bezier(0.34, 1.56, 0.64, 1)` | 卡片 hover 弹性放大 |
| `--gf-ease-linear` | `linear` | 进度条 / loading |

### 7.2 时长

| Token | 值 | 用途 |
|---|---|---|
| `--gf-dur-instant` | `80ms` | 按钮按下 |
| `--gf-dur-fast` | `150ms` | 颜色 / 透明度切换 |
| `--gf-dur-base` | `250ms` | 卡片放大 / hover |
| `--gf-dur-slow` | `400ms` | 弹窗 / 抽屉 |
| `--gf-dur-page` | `500ms` | 页面切换淡入 |

### 7.3 关键动画规范

- **页面切换**：`opacity 0 → 1` + `translateY(8px → 0)`，`var(--gf-dur-page) var(--gf-ease-out)`
- **卡片 hover 放大**：`scale(1) → scale(1.08)` + 阴影提升，`var(--gf-dur-base) var(--gf-ease-spring)`
- **AppHeader 背景切换**：`background-color` + `backdrop-filter`，`var(--gf-dur-base) var(--gf-ease-standard)`
- **FilmRow 横滚**：`transform: translateX()`，`var(--gf-dur-slow) var(--gf-ease-out)`
- **骨架屏闪烁**：`opacity 0.4 ↔ 0.8`，`1.4s var(--gf-ease-linear) infinite`
- **prefers-reduced-motion**：所有 > 200ms 的动画自动降级为 80ms，禁用 scale 放大

---

## 8. 层级（z-index）

| Token | 值 | 用途 |
|---|---|---|
| `--gf-z-base` | `0` | 默认 |
| `--gf-z-row` | `10` | 横滚 Row 的箭头 |
| `--gf-z-header` | `100` | AppHeader 固定 |
| `--gf-z-dropdown` | `200` | 下拉菜单 |
| `--gf-z-overlay` | `900` | 模态遮罩 |
| `--gf-z-modal` | `1000` | 弹窗 |
| `--gf-z-toast` | `1100` | Toast / Notification |
| `--gf-z-tooltip` | `1200` | Tooltip |

---

## 9. 完整 CSS 变量清单（速查）

```css
:root {
  /* 背景 */
  --gf-bg-base: #0b0b0f;
  --gf-bg-surface: #141518;
  --gf-bg-elevated: #1c1d22;
  --gf-bg-overlay: rgba(0, 0, 0, 0.72);
  --gf-bg-glass: rgba(20, 21, 24, 0.55);
  --gf-bg-header: rgba(11, 11, 15, 0);
  --gf-bg-header-scrolled: rgba(11, 11, 15, 0.92);

  /* 文本 */
  --gf-text-primary: #FFFFFF;
  --gf-text-secondary: rgba(255, 255, 255, 0.78);
  --gf-text-muted: rgba(255, 255, 255, 0.55);
  --gf-text-disabled: rgba(255, 255, 255, 0.32);
  --gf-text-inverse: #0b0b0f;
  --gf-text-link: #4ad1e5;
  --gf-text-link-hover: #9b49e7;

  /* 边框 */
  --gf-border-subtle: rgba(255, 255, 255, 0.06);
  --gf-border-default: rgba(255, 255, 255, 0.12);
  --gf-border-strong: rgba(255, 255, 255, 0.24);
  --gf-border-brand: #E50914;

  /* 状态 */
  --gf-success: #22c55e;
  --gf-success-soft: rgba(34, 197, 94, 0.16);
  --gf-warning: #f59e0b;
  --gf-warning-soft: rgba(245, 158, 11, 0.16);
  --gf-danger: #ef4444;
  --gf-danger-soft: rgba(239, 68, 68, 0.16);
  --gf-info: #3b82f6;
  --gf-info-soft: rgba(59, 130, 246, 0.16);

  /* 品牌 */
  --gf-brand-primary: #E50914;
  --gf-brand-primary-hover: #FF1F2C;
  --gf-brand-primary-active: #B0060F;
  --gf-brand-purple: #9b49e7;
  --gf-brand-cyan: #4ad1e5;
  --gf-brand-gradient: linear-gradient(135deg, #9b49e7 0%, #4ad1e5 100%);
  --gf-brand-gradient-hover: linear-gradient(135deg, #b366f5 0%, #6ee0f0 100%);

  /* 蒙版 */
  --gf-mask-hero-bottom: linear-gradient(180deg, rgba(11,11,15,0) 0%, rgba(11,11,15,0.55) 60%, rgba(11,11,15,1) 100%);
  --gf-mask-hero-left: linear-gradient(90deg, rgba(11,11,15,0.92) 0%, rgba(11,11,15,0.6) 35%, rgba(11,11,15,0) 70%);
  --gf-mask-row-left: linear-gradient(90deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%);
  --gf-mask-row-right: linear-gradient(270deg, rgba(11,11,15,1) 0%, rgba(11,11,15,0) 100%);
  --gf-mask-card-hover: linear-gradient(180deg, rgba(0,0,0,0) 50%, rgba(0,0,0,0.85) 100%);

  /* 字体 */
  --gf-font-sans: "Inter", "Helvetica Neue", "PingFang SC", "Microsoft YaHei", "Hiragino Sans GB", "Noto Sans CJK SC", system-ui, sans-serif;
  --gf-font-display: "Inter", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  --gf-font-mono: "JetBrains Mono", "Fira Code", "SFMono-Regular", Consolas, monospace;

  --gf-fs-xs: 0.75rem;
  --gf-fs-sm: 0.875rem;
  --gf-fs-base: 1rem;
  --gf-fs-md: 1.125rem;
  --gf-fs-lg: 1.25rem;
  --gf-fs-xl: 1.5rem;
  --gf-fs-2xl: 1.875rem;
  --gf-fs-3xl: 2.5rem;
  --gf-fs-hero: clamp(2.5rem, 4vw + 1rem, 4.5rem);

  --gf-fw-regular: 400;
  --gf-fw-medium: 500;
  --gf-fw-semibold: 600;
  --gf-fw-bold: 700;
  --gf-fw-black: 900;

  --gf-lh-tight: 1.15;
  --gf-lh-snug: 1.3;
  --gf-lh-normal: 1.5;
  --gf-lh-relaxed: 1.7;

  --gf-tracking-tight: -0.02em;
  --gf-tracking-normal: 0;
  --gf-tracking-wide: 0.05em;
  --gf-tracking-wider: 0.12em;

  /* 间距 */
  --gf-space-0: 0;
  --gf-space-1: 4px;
  --gf-space-2: 8px;
  --gf-space-3: 12px;
  --gf-space-4: 16px;
  --gf-space-5: 20px;
  --gf-space-6: 24px;
  --gf-space-8: 32px;
  --gf-space-10: 40px;
  --gf-space-12: 48px;
  --gf-space-16: 64px;

  /* 容器 */
  --gf-container-max: 1280px;
  --gf-container-max-2xl: 1600px;
  --gf-gutter-mobile: 16px;
  --gf-gutter-tablet: 24px;
  --gf-gutter-desktop: 40px;

  /* 圆角 */
  --gf-radius-none: 0;
  --gf-radius-sm: 4px;
  --gf-radius-md: 8px;
  --gf-radius-lg: 12px;
  --gf-radius-xl: 20px;
  --gf-radius-2xl: 28px;
  --gf-radius-full: 9999px;

  /* 阴影 */
  --gf-shadow-sm: 0 1px 2px rgba(0,0,0,0.4);
  --gf-shadow-md: 0 4px 12px rgba(0,0,0,0.5);
  --gf-shadow-lg: 0 12px 32px rgba(0,0,0,0.6);
  --gf-shadow-xl: 0 24px 60px rgba(0,0,0,0.75);
  --gf-shadow-hover: 0 18px 40px rgba(0,0,0,0.7), 0 0 0 1px rgba(255,255,255,0.06);
  --gf-shadow-brand-glow: 0 0 0 4px rgba(229, 9, 20, 0.25);
  --gf-shadow-purple-glow: 0 0 24px rgba(155, 73, 231, 0.45);
  --gf-shadow-focus-ring: 0 0 0 3px rgba(74, 209, 229, 0.6);

  /* 断点（仅供 JS 读取） */
  --gf-bp-sm: 360px;
  --gf-bp-md: 768px;
  --gf-bp-lg: 1024px;
  --gf-bp-xl: 1440px;
  --gf-bp-2xl: 1920px;

  /* 动画 */
  --gf-ease-standard: cubic-bezier(0.4, 0, 0.2, 1);
  --gf-ease-out: cubic-bezier(0.16, 1, 0.3, 1);
  --gf-ease-in: cubic-bezier(0.7, 0, 0.84, 0);
  --gf-ease-spring: cubic-bezier(0.34, 1.56, 0.64, 1);
  --gf-ease-linear: linear;

  --gf-dur-instant: 80ms;
  --gf-dur-fast: 150ms;
  --gf-dur-base: 250ms;
  --gf-dur-slow: 400ms;
  --gf-dur-page: 500ms;

  /* 层级 */
  --gf-z-base: 0;
  --gf-z-row: 10;
  --gf-z-header: 100;
  --gf-z-dropdown: 200;
  --gf-z-overlay: 900;
  --gf-z-modal: 1000;
  --gf-z-toast: 1100;
  --gf-z-tooltip: 1200;
}
```
