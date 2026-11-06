<script setup lang="ts">
import BaseImage from '@/components/base/BaseImage.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import TvSectionHeader from '@/components/base/TvSectionHeader.vue'
import { buildPlayLink, type HistoryRecord } from '@/stores/history'
import {
  episodeLabel,
  formatProgress,
  formatRelativeTime,
  progressPercent,
  type TimeBucket
} from '@/composables/useTimeBucket'

/**
 * 观看历史 TV(雷鸟卡片式)渲染组件 — 方案 B 拆分(自 HistoryView.vue 下沉)。
 *
 * 职责边界: 本组件只做「TV 渲染」, 数据(favorites/history Store、直播记忆条目合并、
 * 分桶、来源文案)与删除/清空逻辑全在宿主 HistoryView.vue, 经 props + 回调单向传入;
 * 这里不复制任何业务逻辑, 也不另起接口。
 * 宿主以 `v-if="isTV"` 挂载本组件 ⇒ 仅 TV 模式渲染, 组件内无需再判 isTV。
 *
 * 模板/类名/data-focusable/tabindex 与原 HistoryView TV 分支(114-250)逐块等价;
 * 卡片样式走全局 tv-cards.css 词汇类(jc-tv-card/jc-tv-chip/jc-tv-btn/jc-tv-grid),
 * 本组件 scoped 样式只承载原页级 TV 块(jc-tv-history__ 与 jc-tv-hcard__ 两组前缀)。
 */
interface Props {
  /** 按时间分桶后的历史(宿主 computed: groupByTimeBucket(items)) */
  groups: Array<{ bucket: TimeBucket; label: string; items: HistoryRecord[] }>
  /** 历史条数(含「直播」合并条目) —— 操作条计数/按钮显隐用 */
  count: number
  /** 来源文案(云端历史 · 跨设备同步 / 本地历史 · 仅当前浏览器) */
  sourceLabel: string
  /** 是否云端同步模式 */
  remoteMode: boolean
  /** 云端同步中 */
  remoteLoading: boolean
  /** 登录态(TV 操作条的「登录以云端同步」入口) */
  isLoggedIn: boolean
  /** TV「管理」模式(宿主状态: 开启后点整卡=删除该条) */
  manageMode: boolean
  /** 删除单条(宿主 handleRemove: preventDefault/stopPropagation + store.remove) */
  onRemove: (id: string, e: Event) => void
  /** 清空全部(宿主 handleClear: 二次确认 + store.clear + 清直播记忆) */
  onClear: () => void
  /** 切换「管理」模式(宿主状态翻转) */
  onToggleManage: () => void
}
defineProps<Props>()
</script>

<template>
  <section class="jc-tv-history">
    <!-- 操作条: 标题 + 来源标签 + 共N条 + 登录同步 + 清空 -->
    <!-- 横排样式内联写死(不依赖 [data-mode=tv] 全局块, 保证 TV 壳任何环境生效):
         bar=flex 行, head-left(标题+meta)=nowrap 横排, ops 靠右 -->
    <div
      class="jc-tv-history__bar"
      style="display: flex; align-items: center; gap: 14px; flex-wrap: wrap;"
    >
      <div
        class="jc-tv-history__head-left"
        style="display: flex; align-items: center; gap: 12px; flex: 1 1 auto; min-width: 0; flex-wrap: nowrap;"
      >
        <div
          class="jc-tv-history__title"
          style="white-space: nowrap; flex-shrink: 0;"
        >观看历史</div>
        <div
          class="jc-tv-history__meta"
          style="display: flex; align-items: center; gap: 10px; flex-wrap: nowrap; min-width: 0; overflow: hidden; white-space: nowrap;"
        >
          <span class="jc-tv-chip sel jc-tv-history__srcchip" style="flex-shrink: 0;">
            {{ remoteMode ? '云端' : '本地' }}
          </span>
          <span style="min-width: 0; overflow: hidden; text-overflow: ellipsis;">{{ sourceLabel }}</span>
          <span style="flex-shrink: 0;">·</span>
          <span style="flex-shrink: 0;">共 {{ count }} 条</span>
          <span v-if="remoteLoading" class="jc-tv-history__syncing" style="flex-shrink: 0;">同步中…</span>
        </div>
      </div>
      <div
        class="jc-tv-history__ops"
        style="margin-left: auto; display: flex; align-items: center; gap: 10px; flex-shrink: 0;"
      >
        <RouterLink
          v-if="!isLoggedIn"
          to="/login"
          class="jc-tv-chip"
          data-focusable="true"
          tabindex="0"
        >
          登录以云端同步
        </RouterLink>
        <button
          v-if="count"
          type="button"
          class="jc-tv-btn jc-tv-history__manage"
          :class="{ cyan: manageMode }"
          data-focusable="true"
          tabindex="0"
          @click="onToggleManage()"
        >
          {{ manageMode ? '✓ 完成' : '管理删除' }}
        </button>
        <button
          v-if="count"
          type="button"
          class="jc-tv-btn jc-tv-history__clear"
          data-focusable="true"
          tabindex="0"
          @click="onClear"
        >
          <BaseIcon name="trash" size="18px" />
          清空
        </button>
      </div>
    </div>

    <BaseEmpty
      v-if="!count"
      title="还没有观看记录"
      description="去首页找一部喜欢的影片开始观看吧"
    />

    <template v-else>
      <section
        v-for="group in groups"
        :key="group.bucket"
        class="jc-tv-history__group"
      >
        <TvSectionHeader :title="group.label" :sub="`${group.items.length} 条`" />
        <div class="jc-tv-grid">
          <RouterLink
            v-for="record in group.items"
            :key="record.id"
            v-slot="{ navigate }"
            :to="buildPlayLink(record)"
            custom
          >
            <div
              class="jc-tv-card jc-tv-hcard"
              :class="{ 'is-manage': manageMode }"
              data-focusable="true"
              tabindex="0"
              :aria-label="manageMode ? `删除 ${record.name}` : record.name"
              @click="manageMode ? onRemove(record.id, $event) : navigate()"
              @keydown.enter="manageMode ? onRemove(record.id, $event) : navigate()"
            >
            <div class="poster h">
              <BaseImage
                :src="record.picture || ''"
                :alt="record.name"
                ratio="16/9"
                fit="cover"
              />
              <span
                v-if="episodeLabel(record.episode, record.episodeIndex)"
                class="ep"
              >
                {{ episodeLabel(record.episode, record.episodeIndex) }}
              </span>
              <button
                type="button"
                class="del"
                :aria-label="`从历史中移除 ${record.name}`"
                @click="onRemove(record.id, $event)"
              >
                ✕
              </button>
              <span v-if="formatProgress(record.currentTime)" class="ptime">
                {{ formatProgress(record.currentTime) }}
              </span>
              <span
                v-if="progressPercent(record.currentTime, record.duration) > 0"
                class="pbar"
              >
                <i :style="{ width: progressPercent(record.currentTime, record.duration) + '%' }" />
              </span>
              <span v-if="manageMode" class="jc-tv-hcard__delmark" aria-hidden="true">✕ 删除</span>
            </div>
            <div class="name">{{ record.name }}</div>
            <div class="sub">{{ formatRelativeTime(record.timeStamp) }}</div>
            </div>
          </RouterLink>
        </div>
      </section>
    </template>
  </section>
</template>

<!-- ============================================================
 * TV 历史页样式 (方案 B 拆分注记): 原 HistoryView.vue 的页级全局块(:341-532)整块迁入。
 * 样式门控: 本组件**仅 TV 模式挂载**(宿主以 v-if="isTV" 渲染), 故由全局块改为 scoped ——
 * 不会泄漏到桌面/移动; 选择器仍保留 [data-mode='tv'] 前缀, 维持迁移前的特异性口径(双保险)。
 * 全局词汇类(jc-tv-card/jc-tv-chip/jc-tv-btn/jc-tv-grid)来自 tv-cards.css, 本块只承载
 * jc-tv-history__ 与 jc-tv-hcard__ 页面私有词, tv-cards.css 未动。
 * ============================================================ -->
<style scoped>
[data-mode='tv'] .jc-tv-history {
  padding: 6px var(--jc-space-8, 30px) 26px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 操作条 */
[data-mode='tv'] .jc-tv-history__bar {
  display: flex;
  align-items: flex-end;
  gap: 14px;
  flex-wrap: wrap;
}
[data-mode='tv'] .jc-tv-history__head-left {
  flex: 1 1 auto;
  min-width: 0;
  /* 标题 + 来源/同步/计数/标签 强制同一行左右排布(原块级上下堆叠; 禁 wrap 防窄视口换行) */
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  flex-wrap: nowrap;
}
[data-mode='tv'] .jc-tv-history__title {
  font-size: clamp(22px, 2vw, 30px);
  font-weight: 800;
  color: var(--jc-text-primary);
  white-space: nowrap;
}
[data-mode='tv'] .jc-tv-history__meta {
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-muted);
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 10px;
  flex-wrap: nowrap;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
}
[data-mode='tv'] .jc-tv-history__meta > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
[data-mode='tv'] .jc-tv-history__srcchip {
  height: 22px;
  padding: 0 10px;
  font-size: 11px;
  flex-shrink: 0;
}
[data-mode='tv'] .jc-tv-history__syncing {
  color: var(--jc-brand-cyan);
}
[data-mode='tv'] .jc-tv-history__ops {
  margin-left: auto;
  display: flex;
  gap: 10px;
}
[data-mode='tv'] .jc-tv-history__clear,
[data-mode='tv'] .jc-tv-history__manage {
  height: 44px;
  padding: 0 18px;
  font-size: 14px;
}

/* 历史进度卡 (横图 16:9 + 角标 + 进度条 + 下方片名/时间) */
[data-mode='tv'] .jc-tv-hcard {
  display: block;
  text-decoration: none;
  color: var(--jc-text-primary);
}
[data-mode='tv'] .jc-tv-hcard .poster {
  aspect-ratio: 16 / 9;
  border-radius: var(--jc-radius-md, 11px);
  border: 1px solid var(--jc-tv-stroke, rgba(255, 255, 255, 0.13));
  position: relative;
  overflow: hidden;
  background-color: var(--jc-bg-elevated, #1c1d22);
}
[data-mode='tv'] .jc-tv-hcard .ep {
  position: absolute;
  top: 6px;
  left: 6px;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 5px;
  background: var(--jc-brand-gradient);
  color: #fff;
  z-index: 2;
}
[data-mode='tv'] .jc-tv-hcard .del {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.6);
  color: #fff;
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 3;
  cursor: pointer;
  border: none;
}
[data-mode='tv'] .jc-tv-hcard .ptime {
  position: absolute;
  right: 7px;
  bottom: 8px;
  font-size: 10px;
  color: #fff;
  background: rgba(0, 0, 0, 0.7);
  padding: 1px 6px;
  border-radius: 5px;
  z-index: 2;
}
[data-mode='tv'] .jc-tv-hcard .pbar {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  background: rgba(255, 255, 255, 0.25);
  z-index: 2;
}
[data-mode='tv'] .jc-tv-hcard .pbar i {
  display: block;
  height: 100%;
  background: var(--jc-brand-gradient);
}
[data-mode='tv'] .jc-tv-hcard .name {
  margin-top: 7px;
  font-size: 13px;
  font-weight: 600;
  color: var(--jc-text-primary);
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
[data-mode='tv'] .jc-tv-hcard .sub {
  font-size: 11px;
  color: var(--jc-text-muted);
  margin-top: 1px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 焦点: 只放大海报, 下方片名/时间静止 (与 web TV FilmCard 行为一致) */
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus,
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus-visible {
  transform: none;
  box-shadow: none;
  outline: none;
}
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus .poster,
[data-mode='tv'] .jc-tv-hcard[data-focusable='true']:focus-visible .poster {
  transform: scale(var(--jc-tv-focus-scale-card, 1.05));
  box-shadow: var(--jc-tv-focus-ring);
  z-index: 5;
  transform-origin: center;
  transition:
    transform var(--jc-dur-fast) var(--jc-ease-spring),
    box-shadow var(--jc-dur-fast) var(--jc-ease-standard);
}

/* 管理模式: 海报上覆盖红色"✕ 删除"提示, 点整卡(OK 键)即删该条 */
[data-mode='tv'] .jc-tv-hcard__delmark {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  font-size: var(--jc-fs-base);
  font-weight: 800;
  letter-spacing: 1px;
  color: #fff;
  background: rgba(220, 38, 38, 0.42);
}
[data-mode='tv'] .jc-tv-hcard.is-manage .name {
  color: var(--jc-danger, #ff4757);
}
</style>
