<script setup lang="ts">
/**
 * TvSectionHeader — TV 模式区块标题原子（§C8#5）。
 * 收编 8 文件 14 处手拼 `div.jc-tv-sec`（HomeView×4/FilmDetailView×2/SearchView×2/
 * HistoryView/ClassifyView×2/ClassifySearchView/HotBoardView/DeviceConfirmView）。
 *
 * 结构 = 原 jc-tv-sec 三段式: .t 标题 + .s 副标题 + 右侧「更多」入口。
 * - props title/sub 走标准三段式（sub 可省）;
 * - #more 槽放右侧入口, RouterLink/button 由宿主原样传入 —— data-focusable="true" /
 *   tabindex="0" 逐项保留在宿主（TV 空间导航依赖, 不可收进组件内）;
 * - 默认槽放异形头整段自绘（ClassifySearch/HotBoard 的 jc-cs-tv__head 计数头、
 *   ClassifyView 渐变标题行）, 宿主 span 可继续用 .t/.s 类拿迁入样式。
 *
 * 样式: jc-tv-sec/.t/.s 三条规则自 tv-cards.css「区块标题」节整类迁入本件（见 <style>）。
 * 注意是**非 scoped**: scoped 会给末位选择器追加 [data-v] 把容器规则抬到 (0,3,0),
 * 压过宿主页同特异性覆盖（ClassifyView .jc-classify-tv__title{align-items:center}）,
 * 视觉回归; 非 scoped 保持原全局规则的特异性与级联语义不变。
 */
interface Props {
  /** 标题（.t span）; 默认槽自绘时可省 */
  title?: string
  /** 副标题（.s span, 可省） */
  sub?: string
}

withDefaults(defineProps<Props>(), { title: '', sub: '' })
</script>

<template>
  <div class="jc-tv-sec">
    <slot>
      <span v-if="title" class="t">{{ title }}</span>
      <span v-if="sub" class="s">{{ sub }}</span>
    </slot>
    <slot name="more" />
  </div>
</template>

<!-- 非 scoped（理由见头注）: data-mode 门控保留, 与 tv-cards.css 原规则逐字同值 -->
<style>
[data-mode='tv'] .jc-tv-sec {
  display: flex;
  align-items: baseline;
  margin-bottom: 12px;
}
[data-mode='tv'] .jc-tv-sec .t {
  font-size: var(--jc-fs-base);
  font-weight: var(--jc-fw-bold);
}
[data-mode='tv'] .jc-tv-sec .s {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
  margin-left: 8px;
}
</style>
