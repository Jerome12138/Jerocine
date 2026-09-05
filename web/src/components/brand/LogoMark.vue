<script setup lang="ts">
/**
 * LogoMark - Jerocine 品牌 logo (纯文字, 移除了原圆形 SVG 图标 — 用户反馈
 * 圆形 logo 占位太大不美观, 字标更适合 TV 远观视场).
 *
 * - "Jerocine" 文字 (渐变色, 斜体)
 * - showText=false 时仍渲染 (本组件已无图标可隐), 保留 prop 防破坏调用方
 * - 字号 / 整体大小用 size prop 控制 (字号 = size * 0.7)
 */
interface Props {
  size?: number
  showText?: boolean
}
withDefaults(defineProps<Props>(), {
  size: 36,
  showText: true
})
</script>

<template>
  <span class="gf-logo" :style="{ height: size + 'px' }">
    <span class="gf-logo__text" :style="{ fontSize: size * 0.7 + 'px' }">
      Jerocine
    </span>
  </span>
</template>

<style scoped>
.gf-logo {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  line-height: 1;
}
.gf-logo__mark {
  filter: drop-shadow(0 2px 8px rgba(155, 73, 231, 0.35));
}
.gf-logo__text {
  font-weight: 800;
  font-style: italic;
  letter-spacing: 0.5px;
  background-image: linear-gradient(135deg, #9b49e7 0%, #ff5cc4 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}
</style>
