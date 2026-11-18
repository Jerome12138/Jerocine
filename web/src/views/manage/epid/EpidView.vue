<script setup lang="ts">
/**
 * 集身份（epid）单页 —— 2026-11-18 从「系统设置」里抽出来独立成页。
 *
 * 内容：
 *  - `EpidDeriveCard`：派生状态 / 指标 / 操作（含校正台账，台账已改接 ManageTable + 分页）
 *
 * 路由：`/manage/epid`（侧栏「系统」组 → 集身份）
 */
import EpidDeriveCard from './EpidDeriveCard.vue'
</script>

<template>
  <div class="flex flex-col gap-[var(--jc-space-5)]">
    <EpidDeriveCard />
  </div>
</template>