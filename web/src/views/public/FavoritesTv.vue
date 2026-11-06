<script setup lang="ts">
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseTag from '@/components/base/BaseTag.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import type { Card } from '@/types/film'
import type { FavoriteRecord } from '@/stores/favorite'

/**
 * 我的收藏 TV(雷鸟卡片式)渲染组件 — 方案 B 拆分(自 FavoritesView.vue 下沉)。
 *
 * 职责边界: 本组件只做「TV 渲染 + 收藏记录→FilmCard 的展示适配」, 数据(favoriteStore、
 * 来源文案)与取消收藏逻辑全在宿主 FavoritesView.vue, 经 props + 回调单向传入;
 * 这里不复制任何业务逻辑, 也不另起接口。
 * 宿主以 `v-if="isTV"` 挂载本组件 ⇒ 仅 TV 模式渲染, 组件内无需再判 isTV。
 *
 * 模板/类名/data-focusable/tabindex 与原 FavoritesView TV 分支(56-118)逐块等价;
 * TV 侧本就用 FilmCard(桌面侧是手写卡), 两套卡各自留在各自组件。
 * 卡片/网格样式来自全局 tv-cards.css 词汇类(jc-tv-grid/jc-tv-btn/jc-tv-glass-card),
 * 本组件 scoped 样式只承载原页级 TV 块(jc-fav-tv*)。
 */
interface Props {
  /** 收藏记录(宿主 computed: favoriteStore.list) */
  items: FavoriteRecord[]
  /** 来源文案(云端收藏 · 跨设备同步 / 本地收藏 · 仅当前浏览器) */
  sourceLabel: string
  /** 是否云端同步模式 */
  remoteMode: boolean
  /** 云端同步中 */
  remoteLoading: boolean
  /** 登录态(TV 标题区的「登录以云端同步」入口) */
  isLoggedIn: boolean
  /** 取消收藏(宿主 handleRemove: preventDefault/stopPropagation + store.remove) */
  onRemove: (id: string, e: Event) => void
}
defineProps<Props>()

/** 把收藏记录映射成 FilmCard 需要的 Card (FavoriteRecord 字段与 Card 不同) —— TV 侧展示适配, 随组件下沉 */
function toCard(record: FavoriteRecord): Card {
  return {
    mid: Number(record.id) || 0,
    name: record.name,
    cover: record.picture || '',
    cid: record.cid ?? 0,
    pid: record.pid ?? 0,
    cName: '',
    subTitle: '',
    area: '',
    year: 0,
    state: '',
    remarks: record.remarks || '',
    dbScore: 0
  }
}
</script>

<template>
  <section class="jc-fav-tv container-page">
    <!-- 标题区: 我的收藏 + 云端/本地来源标识 + 共 N 部 + 登录入口 -->
    <header class="jc-fav-tv__head">
      <div class="jc-fav-tv__head-left">
        <h1 class="jc-fav-tv__title">
          <BaseIcon name="heart" size="1em" />
          我的收藏
          <BaseTag :variant="remoteMode ? 'purple' : 'default'" size="sm">
            {{ remoteMode ? '云端' : '本地' }}
          </BaseTag>
        </h1>
        <p class="jc-fav-tv__sub">
          {{ sourceLabel }} · 共 {{ items.length }} 部
          <span v-if="remoteLoading" class="text-link">· 同步中…</span>
        </p>
      </div>
      <RouterLink
        v-if="!isLoggedIn"
        to="/login"
        class="jc-tv-btn cyan"
        data-focusable="true"
        tabindex="0"
      >
        登录以云端同步
      </RouterLink>
    </header>

    <!-- 空态: 大字 + 提示 -->
    <div
      v-if="!items.length"
      class="jc-tv-glass-card jc-fav-tv__empty"
    >
      <div class="jc-fav-tv__empty-glyph" aria-hidden="true">
        <BaseIcon name="heart" size="1em" />
      </div>
      <div class="jc-fav-tv__empty-title">还没有收藏内容</div>
      <div class="jc-fav-tv__empty-desc">在影片详情页点击「收藏」即可加入这里</div>
    </div>

    <!-- 收藏网格: 2:3 竖海报 (FilmCard) + 卡角常驻"取消收藏"角标 -->
    <div v-else class="jc-tv-grid">
      <div
        v-for="record in items"
        :key="record.id"
        class="jc-fav-tv__cell"
      >
        <FilmCard :item="toCard(record)" :show-title-below="true" />
        <button
          type="button"
          class="jc-fav-tv__remove"
          :aria-label="`取消收藏 ${record.name}`"
          data-focusable="true"
          tabindex="0"
          @click="onRemove(record.id, $event)"
        >
          <BaseIcon name="close" size="20px" />
        </button>
      </div>
    </div>
  </section>
</template>

<!-- ============================================================
 * TV 收藏页样式 (方案 B 拆分注记): 原 FavoritesView.vue 的页级全局块(:209-298)整块迁入。
 * 样式门控: 本组件**仅 TV 模式挂载**(宿主以 v-if="isTV" 渲染), 故由全局块改为 scoped ——
 * 不会泄漏到桌面/移动; 选择器仍保留 [data-mode='tv'] 前缀, 维持迁移前的特异性口径(双保险)。
 * 全局词汇类(jc-tv-grid/jc-tv-btn/jc-tv-glass-card)来自 tv-cards.css, 本块只承载 jc-fav-tv__ 页面私有词。
 * ============================================================ -->
<style scoped>
[data-mode='tv'] .jc-fav-tv {
  padding-block: var(--jc-tv-safe-y) var(--jc-space-16);
}
[data-mode='tv'] .jc-fav-tv__head {
  display: flex;
  align-items: flex-end;
  gap: var(--jc-space-4);
  flex-wrap: wrap;
  margin-bottom: var(--jc-space-8);
}
[data-mode='tv'] .jc-fav-tv__head-left {
  min-width: 0;
}
[data-mode='tv'] .jc-fav-tv__title {
  display: flex;
  align-items: center;
  gap: 10px;
  /* 2xl 太大 → xl(对齐历史页观感) */
  font-size: var(--jc-fs-xl);
  font-weight: var(--jc-fw-bold);
  color: var(--jc-text-primary);
}

/* 收藏卡片偏大 → 缩小 card-min, 网格更密(参考历史页) */
[data-mode='tv'] .jc-fav-tv .jc-tv-grid {
  --jc-tv-card-min: clamp(150px, 13vw, 200px);
}
[data-mode='tv'] .jc-fav-tv__sub {
  margin-top: 8px;
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-muted);
}
[data-mode='tv'] .jc-fav-tv__head .jc-tv-btn.cyan {
  margin-left: auto;
  text-decoration: none;
}

/* 网格单元: FilmCard + 右上角取消收藏角标 */
[data-mode='tv'] .jc-fav-tv__cell {
  position: relative;
}
[data-mode='tv'] .jc-fav-tv__remove {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 4;
  width: 40px;
  height: 40px;
  min-height: 40px;
  border-radius: 999px;
  border: none;
  background: rgba(0, 0, 0, 0.62);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background var(--jc-dur-fast) var(--jc-ease-standard);
}
[data-mode='tv'] .jc-fav-tv__remove:hover,
[data-mode='tv'] .jc-fav-tv__remove:focus,
[data-mode='tv'] .jc-fav-tv__remove:focus-visible {
  background: rgba(0, 0, 0, 0.88);
  outline: none;
}

/* 空态 */
[data-mode='tv'] .jc-fav-tv__empty {
  text-align: center;
  padding: 56px 40px;
}
[data-mode='tv'] .jc-fav-tv__empty-glyph {
  font-size: 56px;
  line-height: 1;
  color: var(--jc-text-muted);
  opacity: 0.6;
}
[data-mode='tv'] .jc-fav-tv__empty-title {
  margin-top: 16px;
  font-size: var(--jc-fs-lg);
  font-weight: var(--jc-fw-bold);
  color: var(--jc-text-primary);
}
[data-mode='tv'] .jc-fav-tv__empty-desc {
  margin-top: 8px;
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-muted);
}
</style>
