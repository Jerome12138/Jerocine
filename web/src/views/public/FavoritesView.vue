<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { storeToRefs } from 'pinia'
import { useFavoriteStore, useUserStore } from '@/stores'
import { useViewMode } from '@/composables/useViewMode'
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'

/**
 * 我的收藏 — 壳(数据/状态/路由) + 双分支渲染
 * - 数据: favoriteStore(本地/云端)
 * - 桌面 / 移动: 本文件内模板(手写卡)
 * - TV(雷鸟卡片式): 渲染组件 FavoritesTv.vue(方案 B, defineAsyncComponent 异步分片),
 *   本壳只做数据获取与取消收藏逻辑, 经 props + 回调单向传入; 不另起接口。
 */

/** TV 渲染组件(方案 B 拆分): TV 模板/页级 TV CSS 全部在 FavoritesTv.vue, 仅 isTV 时挂载 */
const FavoritesTv = defineAsyncComponent(() => import('./FavoritesTv.vue'))

const favoriteStore = useFavoriteStore()
const userStore = useUserStore()
const { isTV } = useViewMode()
const { list, remoteMode, remoteLoading } = storeToRefs(favoriteStore)
const { isLoggedIn } = storeToRefs(userStore)

const items = computed(() => list.value)
const sourceLabel = computed(() =>
  remoteMode.value ? '云端收藏 · 跨设备同步' : '本地收藏 · 仅当前浏览器'
)

function detailLink(id: string): { path: string; query: { link: string } } {
  return { path: '/filmDetail', query: { link: id } }
}

function handleRemove(id: string, e: Event): void {
  e.preventDefault()
  e.stopPropagation()
  void favoriteStore.remove(id)
}
</script>

<template>
  <!-- ===================== TV(雷鸟卡片式)分支: 渲染组件(方案 B) ===================== -->
  <!-- TV 模板/页级 TV CSS 全部在 FavoritesTv.vue; 此处只做数据供给与取消收藏回调。 -->
  <FavoritesTv
    v-if="isTV"
    :items="items"
    :source-label="sourceLabel"
    :remote-mode="remoteMode"
    :remote-loading="remoteLoading"
    :is-logged-in="isLoggedIn"
    :on-remove="handleRemove"
  />

  <!-- ===================== 桌面 / 移动分支 (页头/删除钮已随 W2c 收编) ===================== -->
  <section
    v-else
    class="container-page py-[var(--jc-space-6)]"
  >
    <ListPageHeader
      title="我的收藏"
      :source-label="sourceLabel"
      :remote-mode="remoteMode"
      :count="items.length"
      unit="部"
      :remote-loading="remoteLoading"
      :logged-in="isLoggedIn"
    />

    <BaseEmpty
      v-if="!items.length"
      title="还没有收藏内容"
      description="在影片详情页点击「收藏」即可加入这里"
    />

    <div
      v-else
      class="jc-card-grid"
    >
      <RouterLink
        v-for="record in items"
        :key="record.id"
        :to="detailLink(record.id)"
        class="jc-fav-card group block"
        data-focusable="true"
        tabindex="0"
        :aria-label="record.name"
      >
        <div class="relative overflow-hidden rounded-[var(--jc-radius-lg)] shadow-card aspect-[3/4] bg-elevated">
          <BaseImage
            :src="record.picture || ''"
            :alt="record.name"
            ratio="3/4"
            fit="cover"
          />

          <CardCornerRemove
            z-class="z-2"
            :label="`取消收藏 ${record.name}`"
            @click="handleRemove(record.id, $event)"
          />

          <div class="absolute inset-0 bg-[linear-gradient(180deg,transparent_50%,rgba(0,0,0,0.85)_100%)] opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity" />
        </div>

        <div class="mt-[var(--jc-space-2)]">
          <h3 class="text-[length:var(--jc-fs-sm)] font-[var(--jc-fw-medium)] text-primary line-clamp-1">
            {{ record.name }}
          </h3>
          <p v-if="record.remarks" class="text-[length:var(--jc-fs-xs)] text-muted mt-[2px] line-clamp-1">
            {{ record.remarks }}
          </p>
        </div>
      </RouterLink>
    </div>
  </section>
</template>

<style scoped>
/* .jc-link-btn 视觉已随 W2c 收敛进 ListPageHeader */
.jc-fav-card {
  text-decoration: none;
  outline: none;
  transition: transform var(--jc-dur-base) var(--jc-ease-spring);
}
.jc-fav-card:focus-visible > div:first-child {
  box-shadow: var(--jc-shadow-focus-ring), var(--jc-shadow-hover);
}
@media (hover: hover) and (pointer: fine) {
  .jc-fav-card:hover > div:first-child {
    transform: scale(1.04);
    box-shadow: var(--jc-shadow-hover);
  }
}
.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>

<!-- 页级 TV 样式(jc-fav-tv*)已随渲染组件迁入 FavoritesTv.vue(scoped) -->
