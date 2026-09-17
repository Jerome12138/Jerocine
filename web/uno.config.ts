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
      'jc-bg-base': 'var(--jc-bg-base)',
      'jc-bg-surface': 'var(--jc-bg-surface)',
      'jc-bg-elevated': 'var(--jc-bg-elevated)',
      'jc-text-primary': 'var(--jc-text-primary)',
      'jc-text-secondary': 'var(--jc-text-secondary)',
      'jc-text-muted': 'var(--jc-text-muted)',
      'jc-brand': 'var(--jc-brand-primary)'
    }
  },
  shortcuts: [
    // 背景层
    ['bg-base', 'bg-[var(--jc-bg-base)]'],
    ['bg-surface', 'bg-[var(--jc-bg-surface)]'],
    ['bg-elevated', 'bg-[var(--jc-bg-elevated)]'],
    ['bg-overlay', 'bg-[var(--jc-bg-overlay)]'],
    ['bg-glass', 'bg-[var(--jc-bg-glass)] backdrop-blur-[18px]'],
    ['bg-header', 'bg-[var(--jc-bg-header)]'],
    ['bg-header-scrolled', 'bg-[var(--jc-bg-header-scrolled)]'],

    // 文本
    ['text-primary', 'text-[var(--jc-text-primary)]'],
    ['text-secondary', 'text-[var(--jc-text-secondary)]'],
    ['text-muted', 'text-[var(--jc-text-muted)]'],
    ['text-disabled', 'text-[var(--jc-text-disabled)]'],
    ['text-link', 'text-[var(--jc-text-link)] hover:text-[var(--jc-text-link-hover)]'],

    // 边框
    ['border-subtle', 'border-[var(--jc-border-subtle)]'],
    ['border-default', 'border-[var(--jc-border-default)]'],
    ['border-strong', 'border-[var(--jc-border-strong)]'],
    ['border-brand', 'border-[var(--jc-border-brand)]'],

    // 状态
    ['text-success', 'text-[var(--jc-success)]'],
    ['text-warning', 'text-[var(--jc-warning)]'],
    ['text-danger', 'text-[var(--jc-danger)]'],
    ['text-info', 'text-[var(--jc-info)]'],

    // 品牌
    ['bg-brand', 'bg-[var(--jc-brand-primary)] hover:bg-[var(--jc-brand-primary-hover)]'],
    ['bg-brand-gradient', 'bg-[image:var(--jc-brand-gradient)]'],
    ['text-brand', 'text-[var(--jc-brand-primary)]'],

    // 圆角
    ['rounded-card', 'rounded-[var(--jc-radius-lg)]'],
    ['rounded-pill', 'rounded-[var(--jc-radius-full)]'],

    // 阴影
    ['shadow-card', 'shadow-[var(--jc-shadow-md)]'],
    ['shadow-card-lg', 'shadow-[var(--jc-shadow-lg)]'],
    ['shadow-card-xl', 'shadow-[var(--jc-shadow-xl)]'],
    ['shadow-card-hover', 'shadow-[var(--jc-shadow-hover)]'],
    ['shadow-focus', 'shadow-[var(--jc-shadow-focus-ring)]'],
    ['shadow-brand-glow', 'shadow-[var(--jc-shadow-brand-glow)]'],

    // 容器
    [
      'container-page',
      'mx-auto w-full px-[var(--jc-gutter-mobile)] md:px-[var(--jc-gutter-tablet)] lg:px-[var(--jc-gutter-desktop)] max-w-[var(--jc-container-max)] 2xl:max-w-[var(--jc-container-max-2xl)]'
    ],

    // 实用
    ['flex-center', 'flex items-center justify-center'],
    ['flex-between', 'flex items-center justify-between'],
    ['absolute-center', 'absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2'],

    // 文字渐变
    ['text-brand-gradient', 'bg-clip-text text-transparent bg-[image:var(--jc-brand-gradient)]'],

    // 卡片
    ['card', 'bg-surface rounded-card shadow-card transition duration-[var(--jc-dur-base)]'],
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
