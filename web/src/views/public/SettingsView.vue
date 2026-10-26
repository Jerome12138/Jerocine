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
import { useSkipSettings, type SkipConfig } from '@/composables/useSkipSettings'
import { ref } from 'vue'

const {
  adFilter,
  filterMode,
  relayCache,
  autoPlayNext,
  danmaku,
  setAdFilter,
  setFilterMode,
  setRelayCache,
  setAutoPlayNext,
  setDanmaku
} = useWebSettings()

/** 跳过片头片尾用户级全局默认（单影片配置优先; 未登录即本地级） */
const skip = useSkipSettings()
const skipDefault = ref<SkipConfig>({ ...skip.getDefault() })
/** 草稿输入（秒）, 保存时写回 */
const skipIntroDraft = ref(skipDefault.value.intro)
const skipOutroDraft = ref(skipDefault.value.outro)
function applySkipDefault(): void {
  skipDefault.value = { ...skip.getDefault() }
  skipIntroDraft.value = skipDefault.value.intro
  skipOutroDraft.value = skipDefault.value.outro
}
function saveSkipDefault(): void {
  const cfg: SkipConfig = {
    intro: Number(skipIntroDraft.value) || 0,
    outro: Number(skipOutroDraft.value) || 0,
    enabled: skipDefault.value.enabled
  }
  skip.saveDefault(cfg)
  applySkipDefault()
}
function resetSkipDefault(): void {
  skip.resetDefault()
  applySkipDefault()
}
function toggleSkipDefault(): void {
  skipDefault.value = { ...skipDefault.value, enabled: !skipDefault.value.enabled }
}

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
  },
  {
    key: 'danmaku',
    title: '弹幕',
    desc: '播放时显示平台弹幕 · 默认关闭（需用时打开; 播放器内可对本集单独开）',
    get: () => danmaku.value,
    set: (v: boolean) => setDanmaku(v)
  }
] as const
</script>

<template>
  <section class="jc-settings-page mx-auto w-full max-w-[720px]">
    <h1 class="jc-settings-page__title">设置</h1>
    <p class="jc-settings-page__hint">
      此页设置对全部影片生效，保存在本设备的网页存储里；
      播放页里针对单片影片的设置（如跳过片头/片尾）按影片单独记忆。
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

    <!-- 跳过片头片尾: 用户级全局默认（单影片配置优先） -->
    <div class="jc-settings-page__group">
      <div class="jc-settings-page__group-title">跳过片头片尾（默认）</div>
      <p class="jc-settings-page__hint">
        对全部影片生效；播放页内对单部影片的跳过设置优先于此处默认值。
      </p>
      <button
        type="button"
        class="jc-settings-page__row"
        role="switch"
        :aria-checked="skipDefault.enabled"
        data-focusable="true"
        @click="toggleSkipDefault"
      >
        <span class="jc-settings-page__row-main">
          <span class="jc-settings-page__row-title">启用默认跳过</span>
          <span class="jc-settings-page__row-desc">关闭后默认不跳过（影片单独设置不受影响）</span>
        </span>
        <span class="jc-settings-page__switch" :class="{ 'is-on': skipDefault.enabled }" aria-hidden="true">
          <span class="jc-settings-page__knob" />
        </span>
      </button>
      <div class="jc-settings-page__row" :class="{ 'jc-settings-page__row--dim': !skipDefault.enabled }">
        <span class="jc-settings-page__row-main">
          <span class="jc-settings-page__row-title">片头跳过秒数</span>
        </span>
        <input
          v-model.number="skipIntroDraft"
          type="number"
          min="0"
          max="600"
          step="10"
          class="jc-settings-page__num"
          :disabled="!skipDefault.enabled"
          data-focusable="true"
        />
      </div>
      <div class="jc-settings-page__row" :class="{ 'jc-settings-page__row--dim': !skipDefault.enabled }">
        <span class="jc-settings-page__row-main">
          <span class="jc-settings-page__row-title">片尾跳过秒数</span>
        </span>
        <input
          v-model.number="skipOutroDraft"
          type="number"
          min="0"
          max="600"
          step="10"
          class="jc-settings-page__num"
          :disabled="!skipDefault.enabled"
          data-focusable="true"
        />
      </div>
      <div class="jc-settings-page__actions">
        <button type="button" class="jc-settings-page__ghost" data-focusable="true" @click="resetSkipDefault">
          恢复内置默认
        </button>
        <button
          type="button"
          class="jc-settings-page__ghost jc-settings-page__ghost--primary"
          data-focusable="true"
          @click="saveSkipDefault"
        >
          保存默认
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
.jc-settings-page__row--dim {
  opacity: 0.45;
}
.jc-settings-page__num {
  flex: 0 0 auto;
  width: 96px;
  padding: 6px 10px;
  border-radius: var(--jc-radius-md);
  border: 1px solid var(--jc-border-default);
  background: var(--jc-bg-surface);
  color: var(--jc-text-primary);
  font-size: var(--jc-fs-sm);
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.jc-settings-page__actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--jc-space-2);
  padding-top: var(--jc-space-2);
}
.jc-settings-page__ghost {
  padding: 6px 14px;
  border-radius: var(--jc-radius-md);
  border: 1px solid var(--jc-border-default);
  background: transparent;
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  cursor: pointer;
}
.jc-settings-page__ghost:hover {
  border-color: var(--jc-brand-primary);
  color: var(--jc-brand-primary);
}
.jc-settings-page__ghost--primary {
  color: var(--jc-brand-primary);
  border-color: var(--jc-brand-primary);
}
</style>
