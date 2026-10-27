<script setup lang="ts">
/**
 * 弹幕设置面板（B站/腾讯式）——由播放页 BaseDialog 承载。
 * 所有项实时生效（localStorage 由 useDanmaku 持久化）：
 *  - 字号: 0=自适应(随全屏/移动端), 其余为具体 px
 *  - 透明度/显示区域/描边: CSS 实时全量生效
 *  - 速度/同屏上限/顶部/底部/彩色: 对新弹幕生效（速度变更触发重载）
 *
 * 开关层级(2026-10-27): 仅保留用户级总开关(默认关); 单影片开关已移除,
 * 弹幕配置数据全局生效。实际生效 = 全局开关 && 非移动端(移动端不开放弹幕)。
 */
import type { DanmuProvider } from '@/types/film'
import {
  DANMAKU_FONT_STEPS,
  DANMAKU_AREA_STEPS,
  DANMAKU_SPEED_STEPS,
  DANMAKU_LIMIT_STEPS,
  type DanmakuSettings
} from '@/composables/useDanmaku'

defineOptions({ name: 'DanmakuSettingsPanel' })

const props = defineProps<{
  settings: DanmakuSettings
  /** 弹幕主开关（用户级, /settings 同源; 默认关, 随账号/浏览器记忆） */
  enabled: boolean
  providers: DanmuProvider[]
  provider: string
}>()

const emit = defineEmits<{
  (e: 'update', patch: Partial<DanmakuSettings>): void
  (e: 'toggle-enabled'): void
  (e: 'set-provider', provider: string): void
}>()

const PROVIDER_LABELS: Record<string, string> = {
  bilibili: 'B站',
  tencent: '腾讯',
  iqiyi: '爱奇艺',
  youku: '优酷'
}
const providerLabel = (p: string) => PROVIDER_LABELS[p] ?? p

const FONT_LABELS = ['自适应', '极小', '小', '中', '大', '特大'] as const
const AREA_LABELS = ['1/4屏', '半屏', '3/4屏', '全屏'] as const
/** 速度档标签: 数值(库 dur 倍率)越大滚动越慢 */
const SPEED_LABELS = ['极快', '快', '正常', '较慢', '慢'] as const
const LIMIT_LABELS = ['少', '中', '多', '不限'] as const

function patch(p: Partial<DanmakuSettings>): void {
  emit('update', p)
}
</script>

<template>
  <div class="jc-danmu-panel">
    <!-- 弹幕总开关(用户级): 全站唯一开关, 默认关; 已移除单影片开关(2026-10-27) -->
    <div class="jc-danmu-panel__row jc-danmu-panel__row--main">
      <div class="jc-danmu-panel__label">
        <span>弹幕</span>
      </div>
      <button
        type="button"
        class="jc-danmu-switch"
        :class="{ 'is-on': enabled }"
        role="switch"
        :aria-checked="enabled"
        @click="emit('toggle-enabled')"
      >
        <span class="jc-danmu-switch__track" aria-hidden="true"><span class="jc-danmu-switch__knob" /></span>
      </button>
    </div>

    <!-- 字号: 档位式, 0=自适应(随全屏/移动端) -->
    <div class="jc-danmu-panel__row">
      <div class="jc-danmu-panel__label">
        <span>字号</span>
      </div>
      <div class="jc-danmu-panel__segs">
        <button
          v-for="(px, i) in DANMAKU_FONT_STEPS"
          :key="px"
          type="button"
          class="jc-danmu-seg"
          :class="{ 'is-active': settings.fontSize === px }"
          @click="patch({ fontSize: px })"
        >
          {{ FONT_LABELS[i] }}
        </button>
      </div>
    </div>

    <!-- 透明度: 滑块 10-100% -->
    <div class="jc-danmu-panel__row">
      <div class="jc-danmu-panel__label">
        <span>透明度</span>
        <span class="jc-danmu-panel__value">{{ Math.round(settings.opacity * 100) }}%</span>
      </div>
      <input
        type="range"
        min="10"
        max="100"
        step="5"
        class="jc-danmu-range"
        :value="Math.round(settings.opacity * 100)"
        @input="patch({ opacity: Number(($event.target as HTMLInputElement).value) / 100 })"
      />
    </div>

    <!-- 显示区域 -->
    <div class="jc-danmu-panel__row">
      <div class="jc-danmu-panel__label">
        <span>显示区域</span>
      </div>
      <div class="jc-danmu-panel__segs">
        <button
          v-for="(a, i) in DANMAKU_AREA_STEPS"
          :key="a"
          type="button"
          class="jc-danmu-seg"
          :class="{ 'is-active': settings.area === a }"
          @click="patch({ area: a })"
        >
          {{ AREA_LABELS[i] }}
        </button>
      </div>
    </div>

    <!-- 速度 -->
    <div class="jc-danmu-panel__row">
      <div class="jc-danmu-panel__label">
        <span>速度</span>
      </div>
      <div class="jc-danmu-panel__segs">
        <button
          v-for="(s, i) in DANMAKU_SPEED_STEPS"
          :key="s"
          type="button"
          class="jc-danmu-seg"
          :class="{ 'is-active': settings.speed === s }"
          @click="patch({ speed: s })"
        >
          {{ SPEED_LABELS[i] }}
        </button>
      </div>
    </div>

    <!-- 同屏数量 -->
    <div class="jc-danmu-panel__row">
      <div class="jc-danmu-panel__label">
        <span>同屏数量</span>
      </div>
      <div class="jc-danmu-panel__segs">
        <button
          v-for="(n, i) in DANMAKU_LIMIT_STEPS"
          :key="n"
          type="button"
          class="jc-danmu-seg"
          :class="{ 'is-active': settings.limit === n }"
          @click="patch({ limit: n })"
        >
          {{ LIMIT_LABELS[i] }}
        </button>
      </div>
    </div>

    <!-- 模式/外观 2x2 -->
    <div class="jc-danmu-panel__grid">
      <button type="button" class="jc-danmu-toggle" :class="{ 'is-on': settings.topMode }" @click="patch({ topMode: !settings.topMode })">
        <span class="jc-danmu-toggle__box" aria-hidden="true">✓</span>
        <span>顶部弹幕</span>
      </button>
      <button type="button" class="jc-danmu-toggle" :class="{ 'is-on': settings.bottomMode }" @click="patch({ bottomMode: !settings.bottomMode })">
        <span class="jc-danmu-toggle__box" aria-hidden="true">✓</span>
        <span>底部弹幕</span>
      </button>
      <button type="button" class="jc-danmu-toggle" :class="{ 'is-on': settings.colored }" @click="patch({ colored: !settings.colored })">
        <span class="jc-danmu-toggle__box" aria-hidden="true">✓</span>
        <span>彩色弹幕</span>
      </button>
      <button type="button" class="jc-danmu-toggle" :class="{ 'is-on': settings.outline }" @click="patch({ outline: !settings.outline })">
        <span class="jc-danmu-toggle__box" aria-hidden="true">✓</span>
        <span>描边</span>
      </button>
    </div>

    <!-- 弹幕源（多源时切换） -->
    <div v-if="providers.length > 1" class="jc-danmu-panel__row">
      <div class="jc-danmu-panel__label">
        <span>弹幕源</span>
        <span class="jc-danmu-panel__hint">当前：{{ providerLabel(provider) }}</span>
      </div>
      <div class="jc-danmu-panel__segs">
        <button
          v-for="pr in providers"
          :key="pr.provider"
          type="button"
          class="jc-danmu-seg"
          :class="{ 'is-active': provider === pr.provider }"
          @click="emit('set-provider', pr.provider)"
        >
          {{ providerLabel(pr.provider) }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.jc-danmu-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 2px 0;
}
.jc-danmu-panel__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.jc-danmu-panel__row--main {
  padding-bottom: 4px;
}
.jc-danmu-panel__label {
  display: flex;
  flex-direction: column;
  gap: 0;
  min-width: 0;
}
.jc-danmu-panel__label > span:first-child {
  font-size: 14px;
  font-weight: 600;
  color: var(--jc-text-primary, #e6e8ee);
}
.jc-danmu-panel__hint {
  font-size: 12px;
  color: var(--jc-text-tertiary, #8a8f9c);
}
.jc-danmu-panel__value {
  font-size: 13px;
  color: var(--jc-accent, #3b82f6);
  font-variant-numeric: tabular-nums;
}
.jc-danmu-panel__segs {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
  flex-shrink: 0;
}
.jc-danmu-seg {
  min-width: 44px;
  padding: 5px 10px;
  border-radius: 8px;
  border: 1px solid var(--jc-border, rgba(255, 255, 255, 0.12));
  background: rgba(255, 255, 255, 0.05);
  color: var(--jc-text-secondary, #b7bcc8);
  font-size: 13px;
  line-height: 1.2;
  cursor: pointer;
  transition: all 0.15s ease;
}
.jc-danmu-seg:hover {
  background: rgba(255, 255, 255, 0.09);
  color: var(--jc-text-primary, #e6e8ee);
}
.jc-danmu-seg.is-active {
  border-color: var(--jc-accent, #3b82f6);
  background: color-mix(in srgb, var(--jc-accent, #3b82f6) 22%, transparent);
  color: var(--jc-text-primary, #e6e8ee);
}
.jc-danmu-range {
  flex: 1;
  max-width: 200px;
  accent-color: var(--jc-accent, #3b82f6);
  height: 4px;
  cursor: pointer;
}
.jc-danmu-panel__grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.jc-danmu-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid var(--jc-border, rgba(255, 255, 255, 0.12));
  background: rgba(255, 255, 255, 0.04);
  color: var(--jc-text-secondary, #b7bcc8);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.jc-danmu-toggle:hover {
  background: rgba(255, 255, 255, 0.08);
}
.jc-danmu-toggle.is-on {
  border-color: color-mix(in srgb, var(--jc-accent, #3b82f6) 60%, transparent);
  color: var(--jc-text-primary, #e6e8ee);
}
.jc-danmu-toggle__box {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 4px;
  border: 1px solid var(--jc-border, rgba(255, 255, 255, 0.25));
  font-size: 11px;
  color: transparent;
  flex-shrink: 0;
}
.jc-danmu-toggle.is-on .jc-danmu-toggle__box {
  background: var(--jc-accent, #3b82f6);
  border-color: var(--jc-accent, #3b82f6);
  color: #fff;
}
.jc-danmu-switch {
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  flex-shrink: 0;
}
.jc-danmu-switch__track {
  display: block;
  width: 42px;
  height: 24px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.15);
  position: relative;
  transition: background 0.2s ease;
}
.jc-danmu-switch__knob {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #fff;
  transition: left 0.2s ease;
}
.jc-danmu-switch.is-on .jc-danmu-switch__track {
  background: var(--jc-accent, #3b82f6);
}
.jc-danmu-switch.is-on .jc-danmu-switch__knob {
  left: 21px;
}
</style>
