import {
  defineConfig,
  presetAttributify,
  presetIcons,
  presetTypography,
  presetUno,
  transformerDirectives,
  transformerVariantGroup
} from 'unocss'

/**
 * UnoCSS 配置
 * - 断点与 design-tokens.md 第 6 节一致
 * - shortcuts 把 CSS 变量映射成可在 attributify 中使用的原子类
 * - preset-icons 默认走 carbon / mdi 集合
 */
export default defineConfig({
  presets: [
    presetUno(),
    presetAttributify(),
    // preset-icons 0.62.4 在某些 carbon/mdi 图标 build 时输出非法 CSS
    // 暂时禁用，新版组件改用 inline SVG / iconfont 字体
    presetTypography()
  ],
  transformers: [transformerDirectives(), transformerVariantGroup()],
  theme: {
    breakpoints: {
      sm: '360px',
      md: '768px',
      lg: '1024px',
      xl: '1440px',
      '2xl': '1920px'
    },
    colors: {
      // 通过 var() 间接引用，方便后续主题切换
      'gf-bg-base': 'var(--gf-bg-base)',
      'gf-bg-surface': 'var(--gf-bg-surface)',
      'gf-bg-elevated': 'var(--gf-bg-elevated)',
      'gf-text-primary': 'var(--gf-text-primary)',
      'gf-text-secondary': 'var(--gf-text-secondary)',
      'gf-text-muted': 'var(--gf-text-muted)',
      'gf-brand': 'var(--gf-brand-primary)'
    }
  },
  shortcuts: [
    // 背景层
    ['bg-base', 'bg-[var(--gf-bg-base)]'],
    ['bg-surface', 'bg-[var(--gf-bg-surface)]'],
    ['bg-elevated', 'bg-[var(--gf-bg-elevated)]'],
    ['bg-overlay', 'bg-[var(--gf-bg-overlay)]'],
    ['bg-glass', 'bg-[var(--gf-bg-glass)] backdrop-blur-[18px]'],
    ['bg-header', 'bg-[var(--gf-bg-header)]'],
    ['bg-header-scrolled', 'bg-[var(--gf-bg-header-scrolled)]'],

    // 文本
    ['text-primary', 'text-[var(--gf-text-primary)]'],
    ['text-secondary', 'text-[var(--gf-text-secondary)]'],
    ['text-muted', 'text-[var(--gf-text-muted)]'],
    ['text-disabled', 'text-[var(--gf-text-disabled)]'],
    ['text-link', 'text-[var(--gf-text-link)] hover:text-[var(--gf-text-link-hover)]'],

    // 边框
    ['border-subtle', 'border-[var(--gf-border-subtle)]'],
    ['border-default', 'border-[var(--gf-border-default)]'],
    ['border-strong', 'border-[var(--gf-border-strong)]'],
    ['border-brand', 'border-[var(--gf-border-brand)]'],

    // 状态
    ['text-success', 'text-[var(--gf-success)]'],
    ['text-warning', 'text-[var(--gf-warning)]'],
    ['text-danger', 'text-[var(--gf-danger)]'],
    ['text-info', 'text-[var(--gf-info)]'],

    // 品牌
    ['bg-brand', 'bg-[var(--gf-brand-primary)] hover:bg-[var(--gf-brand-primary-hover)]'],
    ['bg-brand-gradient', 'bg-[image:var(--gf-brand-gradient)]'],
    ['text-brand', 'text-[var(--gf-brand-primary)]'],

    // 圆角
    ['rounded-card', 'rounded-[var(--gf-radius-lg)]'],
    ['rounded-pill', 'rounded-[var(--gf-radius-full)]'],

    // 阴影
    ['shadow-card', 'shadow-[var(--gf-shadow-md)]'],
    ['shadow-card-lg', 'shadow-[var(--gf-shadow-lg)]'],
    ['shadow-card-xl', 'shadow-[var(--gf-shadow-xl)]'],
    ['shadow-card-hover', 'shadow-[var(--gf-shadow-hover)]'],
    ['shadow-focus', 'shadow-[var(--gf-shadow-focus-ring)]'],
    ['shadow-brand-glow', 'shadow-[var(--gf-shadow-brand-glow)]'],

    // 容器
    [
      'container-page',
      'mx-auto w-full px-[var(--gf-gutter-mobile)] md:px-[var(--gf-gutter-tablet)] lg:px-[var(--gf-gutter-desktop)] max-w-[var(--gf-container-max)] 2xl:max-w-[var(--gf-container-max-2xl)]'
    ],

    // 实用
    ['flex-center', 'flex items-center justify-center'],
    ['flex-between', 'flex items-center justify-between'],
    ['absolute-center', 'absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2'],

    // 文字渐变
    ['text-brand-gradient', 'bg-clip-text text-transparent bg-[image:var(--gf-brand-gradient)]'],

    // 卡片
    ['card', 'bg-surface rounded-card shadow-card transition duration-[var(--gf-dur-base)]'],
    ['card-hover', 'hover:bg-elevated hover:shadow-card-hover hover:scale-[1.04]']
  ],
  safelist: [
    'iconfont',
    'icon-film',
    'icon-tv',
    'icon-cartoon',
    'icon-variety'
  ]
})
