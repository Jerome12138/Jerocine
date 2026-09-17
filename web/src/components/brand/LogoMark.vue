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
  <span class="jc-logo" :style="{ height: size + 'px' }">
    <span class="jc-logo__text" :style="{ fontSize: size * 0.7 + 'px' }">
      Jerocine影视
    </span>
  </span>
</template>

<style scoped>
.jc-logo {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  line-height: 1;
}
.jc-logo__mark {
  filter: drop-shadow(0 2px 8px rgba(155, 73, 231, 0.35));
}
.jc-logo__text {
  display: inline-block;
  font-weight: 800;
  font-style: italic;
  letter-spacing: 0.5px;
  /* 斜体字形右倾, background-clip:text 的绘制区域以文字盒为界,
     最右字符的右缘会被裁掉一角 —— 加右内边距把渐变背景铺到斜体溢出处 */
  padding-inline-end: 6px;
  background-image: linear-gradient(135deg, #9b49e7 0%, #ff5cc4 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}
</style>
