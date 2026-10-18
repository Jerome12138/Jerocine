<script setup lang="ts">
/**
 * Web 端统一配置页（/settings, 2026-10-18 重新加入 —— 上一版 /settings 已于 2026-10-10 删除,
 * 当时收进原生抽屉; 现按"web 状态开关需要独立配置页"重新拆出, 数据仍存 web localStorage）。
 *
 * 数据源 = useWebSettings（单一数据源）:
 *   - 播放页等消费方用同一份响应式 ref, 本页改动即刻生效;
 *   - 安卓 web 壳的设置抽屉经 window.__jcConfig 读写同一份 localStorage。
 */
import { useWebSettings } from '@/composables/useWebSettings'

const {
  adFilter,
  filterMode,
  relayCache,
  autoPlayNext,
  setAdFilter,
  setFilterMode,
  setRelayCache,
  setAutoPlayNext
} = useWebSettings()

/** 播放设置项: 统一行模型, 模板循环渲染(键名即 localStorage 契约的语义名) */
const toggles = [
  {
    key: 'adFilter',
    title: '广告过滤',
    desc: '剔除 m3u8 插片/跨域广告 · 默认开启',
    get: () => adFilter.value,
    set: (v: boolean) => setAdFilter(v)
  },
  {
    key: 'relayCache',
    title: '全量中转',
    desc: '直连异常时分片也经服务器转发(流量走服务器, 默认关)',
    get: () => relayCache.value,
    set: (v: boolean) => setRelayCache(v)
  },
  {
    key: 'autoPlayNext',
    title: '自动连播',
    desc: '一集播完自动播放下一集',
    get: () => autoPlayNext.value,
    set: (v: boolean) => setAutoPlayNext(v)
  }
] as const
</script>

<template>
  <section class="jc-settings-page mx-auto w-full max-w-[720px]">
    <h1 class="jc-settings-page__title">设置</h1>
    <p class="jc-settings-page__hint">
      安卓 App 内也可在「设置抽屉」修改同一批开关；数据保存在本设备的网页存储里。
    </p>

    <!-- 播放开关 -->
    <div class="jc-settings-page__group">
      <div class="jc-settings-page__group-title">播放</div>
      <button
        v-for="t in toggles"
        :key="t.key"
        type="button"
        class="jc-settings-page__row"
        role="switch"
        :aria-checked="t.get()"
        data-focusable="true"
        @click="t.set(!t.get())"
      >
        <span class="jc-settings-page__row-main">
          <span class="jc-settings-page__row-title">{{ t.title }}</span>
          <span class="jc-settings-page__row-desc">{{ t.desc }}</span>
        </span>
        <span class="jc-settings-page__switch" :class="{ 'is-on': t.get() }" aria-hidden="true">
          <span class="jc-settings-page__knob" />
        </span>
      </button>

      <!-- 过滤取源方式: 端侧取源(默认) / 云端取源 -->
      <div class="jc-settings-page__row" role="radiogroup" aria-label="过滤取源方式">
        <span class="jc-settings-page__row-main">
          <span class="jc-settings-page__row-title">过滤取源方式</span>
          <span class="jc-settings-page__row-desc">
            端侧取源 = 设备自己拉 m3u8、服务器只做过滤（推荐, 服务器在境外取源慢）；
            云端取源 = 由服务器抓取并过滤，仅设备被源站地域封锁时使用
          </span>
        </span>
      </div>
      <div class="jc-settings-page__segment" role="radiogroup" aria-label="过滤取源方式">
        <button
          type="button"
          class="jc-settings-page__seg-btn"
          :class="{ 'is-active': filterMode === 'client' }"
          role="radio"
          :aria-checked="filterMode === 'client'"
          data-focusable="true"
          @click="setFilterMode('client')"
        >
          <span v-if="filterMode === 'client'" class="jc-settings-page__seg-check" aria-hidden="true">✓</span>
          端侧取源
        </button>
        <button
          type="button"
          class="jc-settings-page__seg-btn"
          :class="{ 'is-active': filterMode === 'server' }"
          role="radio"
          :aria-checked="filterMode === 'server'"
          data-focusable="true"
          @click="setFilterMode('server')"
        >
          <span v-if="filterMode === 'server'" class="jc-settings-page__seg-check" aria-hidden="true">✓</span>
          云端取源
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.jc-settings-page {
  padding: var(--jc-space-6) var(--jc-space-4) 48px;
}
.jc-settings-page__title {
  font-size: var(--jc-fs-xl);
  font-weight: var(--jc-fw-bold);
  color: var(--jc-text-primary);
}
.jc-settings-page__hint {
  margin-top: var(--jc-space-2);
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-secondary);
}
.jc-settings-page__group {
  margin-top: var(--jc-space-5);
  border: 1px solid var(--jc-border-default);
  border-radius: var(--jc-radius-lg);
  background: var(--jc-bg-elevated);
  padding: var(--jc-space-3) var(--jc-space-4) var(--jc-space-4);
}
.jc-settings-page__group-title {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
  letter-spacing: 0.12em;
  margin-bottom: var(--jc-space-2);
}
.jc-settings-page__row {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  width: 100%;
  text-align: left;
  padding: var(--jc-space-3) 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.jc-settings-page__row-main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.jc-settings-page__row-title {
  font-size: var(--jc-fs-base);
  color: var(--jc-text-primary);
}
.jc-settings-page__row-desc {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-secondary);
  line-height: var(--jc-lh-snug);
}
.jc-settings-page__switch {
  flex: 0 0 auto;
  width: 44px;
  height: 24px;
  border-radius: 12px;
  background: var(--jc-bg-surface);
  position: relative;
  transition: background 0.15s ease;
}
.jc-settings-page__switch.is-on {
  background: var(--jc-brand-primary);
}
.jc-settings-page__knob {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #fff;
  transition: transform 0.15s ease;
}
.jc-settings-page__switch.is-on .jc-settings-page__knob {
  transform: translateX(20px);
}
.jc-settings-page__segment {
  display: flex;
  gap: var(--jc-space-2);
  padding: 0 0 var(--jc-space-3);
}
.jc-settings-page__seg-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 6px 14px;
  border-radius: var(--jc-radius-md);
  border: 1px solid var(--jc-border-default);
  background: transparent;
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  cursor: pointer;
}
.jc-settings-page__seg-btn.is-active {
  border-color: var(--jc-brand-primary);
  color: var(--jc-brand-primary);
  background: transparent;
}
.jc-settings-page__seg-check {
  font-size: 0.85em;
}
</style>
