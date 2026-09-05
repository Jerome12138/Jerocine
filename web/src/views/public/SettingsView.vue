<script setup lang="ts">
/**
 * SettingsView - 应用设置页 (web + Jerocine APK 共用)
 *
 * 内容:
 *   - 当前服务器地址 (APK 内可见)
 *   - 应用版本号 (web build 时间戳 / APK 走 native getPlatform)
 *   - 检查更新 (仅 APK)
 *   - 更换服务器地址 (仅 APK)
 *   - 广告过滤默认开关 (web 端, 复用 PlayView 的 gf-ad-filter 持久化键)
 *
 * 渲染分支:
 *   - 桌面 / 移动: 原有卡片式布局 (保持不变)
 *   - TV (isTV): 雷鸟卡片式左右分屏 — 左竖排分组, 右当前组设置项面板
 */
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { toast } from '@/api/http'
import { useViewMode } from '@/composables/useViewMode'
import { useUserStore } from '@/stores/user'
import { DEFAULT_INTRO_SEC, DEFAULT_OUTRO_SEC } from '@/composables/useSkipSettings'

interface JerocineNativeBridge {
  getPlatform?: () => string
  checkUpdate?: () => void
  openServerSettings?: () => void
}

const native = computed<JerocineNativeBridge | undefined>(() => {
  if (typeof window === 'undefined') return undefined
  return (window as unknown as { JerocinePlayer?: JerocineNativeBridge }).JerocinePlayer
})
const isNative = computed(() => !!native.value)

const { isTV } = useViewMode()
const userStore = useUserStore()

const serverOrigin = computed(() => {
  if (typeof window === 'undefined') return ''
  return window.location.origin
})

const buildMode = import.meta.env.MODE

const checking = ref(false)
function onCheckUpdate(): void {
  if (!native.value?.checkUpdate) {
    toast('info', '当前为 Web 版本, 升级请下载 APP 安装包')
    return
  }
  checking.value = true
  try {
    native.value.checkUpdate()
  } finally {
    // native check 是异步的, 自己处理结果 toast, 这里给个短反馈
    setTimeout(() => { checking.value = false }, 800)
  }
}

function onOpenServer(): void {
  if (native.value?.openServerSettings) {
    native.value.openServerSettings()
  } else {
    toast('info', 'Web 版本不需要切换服务器地址')
  }
}

/* ============ TV 分支专用状态 ============ */

/** 当前账号 / 角色 (复用 user store, 不新造接口) */
const accountName = computed(() => userStore.displayName)
const isLoggedIn = computed(() => userStore.isLoggedIn)
const roleLabel = computed(() => (userStore.isAdmin ? '管理员' : '普通用户'))

/**
 * 广告过滤默认开关 — 复用 PlayView 的 localStorage 键 gf-ad-filter,
 * 改这里等同于改播放页默认值 (web 端有效; 安卓原生用自身 PREF_AD_FILTER)。
 */
const AD_FILTER_LS_KEY = 'gf-ad-filter'
function readAdFilter(): boolean {
  try {
    const v = localStorage.getItem(AD_FILTER_LS_KEY)
    if (v === null) return true
    return v === '1'
  } catch {
    return true
  }
}
const adFilter = ref<boolean>(readAdFilter())
function toggleAdFilter(): void {
  adFilter.value = !adFilter.value
  try {
    localStorage.setItem(AD_FILTER_LS_KEY, adFilter.value ? '1' : '0')
  } catch {
    /* 隐私模式忽略 */
  }
  toast(adFilter.value ? 'success' : 'info', adFilter.value ? '广告过滤已开启' : '广告过滤已关闭')
}

/** 左侧分组 (依平台动态裁剪: 设备组仅 APK 可见) */
type GroupId = 'play' | 'adfilter' | 'account' | 'device' | 'about'
interface SettingGroup {
  id: GroupId
  icon: string
  label: string
}
const groups = computed<SettingGroup[]>(() => {
  const base: SettingGroup[] = [
    { id: 'play', icon: '▶', label: '播放' },
    { id: 'adfilter', icon: '🛡', label: '广告过滤' },
    { id: 'account', icon: '👤', label: '账号' }
  ]
  if (isNative.value) {
    base.push({ id: 'device', icon: '📺', label: '设备' })
  }
  base.push({ id: 'about', icon: 'ℹ', label: '关于' })
  return base
})
// 支持 /settings?group=account 深链(首页"我的"卡直达账号分组)
const route = useRoute()
const VALID_GROUPS: readonly GroupId[] = ['play', 'adfilter', 'account', 'device', 'about']
const activeGroup = ref<GroupId>(
  typeof route.query.group === 'string' &&
    (VALID_GROUPS as readonly string[]).includes(route.query.group)
    ? (route.query.group as GroupId)
    : 'play'
)
function selectGroup(id: GroupId): void {
  activeGroup.value = id
}

const platformLabel = computed(() => (isNative.value ? 'Jerocine TV APP' : 'Web 浏览器'))

async function onLogout(): Promise<void> {
  await userStore.logout()
  toast('success', '已退出登录')
}
</script>

<template>
  <!-- ============ TV: 雷鸟卡片式左右分屏 ============ -->
  <main v-if="isTV" class="gf-set-tv">
    <div class="gf-tv-sec">
      <span class="t">⚙ 设置</span>
      <span class="s">播放 · 广告过滤 · 账号 · 关于</span>
    </div>

    <div class="gf-set-tv__split">
      <!-- 左: 设置分组列表 -->
      <nav class="gf-set-tv__groups">
        <a
          v-for="g in groups"
          :key="g.id"
          class="gf-tv-listrow gf-set-tv__group"
          :class="{ 'is-active': activeGroup === g.id }"
          data-focusable="true"
          tabindex="0"
          @click="selectGroup(g.id)"
          @keydown.enter.prevent="selectGroup(g.id)"
        >
          <span class="gf-set-tv__group-ic">{{ g.icon }}</span>
          <span class="lab">{{ g.label }}</span>
          <span class="right">›</span>
        </a>
      </nav>

      <!-- 右: 当前组设置项面板 -->
      <div class="gf-tv-panel" :class="activeGroup === 'about' ? 'z3' : activeGroup === 'account' ? 'z2' : 'z1'">
        <!-- 播放 -->
        <template v-if="activeGroup === 'play'">
          <div class="gf-tv-sec">
            <span class="t">▶ 播放</span>
            <span class="s">跳过与续集设置 · 账号级跨设备同步</span>
          </div>
          <div class="gf-set-tv__rows">
            <div class="gf-tv-listrow">
              <div>
                <div class="lab">跳过片头(默认)</div>
                <div class="sub">每集开头自动前跳 · 单片可在播放页单独调整</div>
              </div>
              <span class="right">{{ DEFAULT_INTRO_SEC }} 秒</span>
            </div>
            <div class="gf-tv-listrow">
              <div>
                <div class="lab">跳过片尾(默认)</div>
                <div class="sub">距结尾自动跳过并续播下一集 · 单片可在播放页单独调整</div>
              </div>
              <span class="right">{{ DEFAULT_OUTRO_SEC }} 秒</span>
            </div>
            <div class="gf-tv-listrow gf-set-tv__note">
              <span class="gf-set-tv__note-ic">☁</span>
              <div class="sub">
                片头/片尾秒数随账号保存(user_skip_setting), 登录后多台设备自动同步, 安卓原生播放器同样生效。
              </div>
            </div>
          </div>
        </template>

        <!-- 广告过滤 -->
        <template v-else-if="activeGroup === 'adfilter'">
          <div class="gf-tv-sec">
            <span class="t">🛡 广告过滤</span>
            <span class="s">端侧抓流 → 服务端仅过滤文本 → 直连分片播</span>
          </div>
          <div class="gf-set-tv__rows">
            <div
              class="gf-tv-listrow gf-set-tv__toggle-row"
              data-focusable="true"
              tabindex="0"
              @click="toggleAdFilter"
              @keydown.enter.prevent="toggleAdFilter"
            >
              <div>
                <div class="lab">广告过滤</div>
                <div class="sub">m3u8 跨域 / 插片广告自动剔除 · 默认开启</div>
              </div>
              <span class="right">
                <span class="gf-tv-toggle" :class="{ on: adFilter }"><i></i></span>
              </span>
            </div>
            <div class="gf-tv-listrow gf-set-tv__note">
              <span class="gf-set-tv__note-ic">🛡</span>
              <div class="sub">
                安卓原生播放器使用设备本地开关(默认开启), 起播后会提示本集已剔除的分片数。
              </div>
            </div>
          </div>
        </template>

        <!-- 账号 -->
        <template v-else-if="activeGroup === 'account'">
          <div class="gf-tv-sec">
            <span class="t">👤 账号</span>
            <span class="s">登录与跨设备</span>
          </div>
          <div class="gf-set-tv__rows">
            <div class="gf-tv-listrow">
              <div>
                <div class="lab">当前账号</div>
                <div class="sub">{{ roleLabel }} · 同名最多 10 台设备在线</div>
              </div>
              <span class="right">{{ isLoggedIn ? accountName : '未登录' }}</span>
            </div>
            <div
              v-if="isLoggedIn"
              class="gf-tv-listrow"
              data-focusable="true"
              tabindex="0"
              @click="onLogout"
              @keydown.enter.prevent="onLogout"
            >
              <div>
                <div class="lab">退出登录</div>
                <div class="sub">清除本机凭证</div>
              </div>
              <span class="right"><span class="gf-tv-btn">退出</span></span>
            </div>
          </div>
        </template>

        <!-- 设备 (仅 APK) -->
        <template v-else-if="activeGroup === 'device'">
          <div class="gf-tv-sec">
            <span class="t">📺 设备</span>
            <span class="s">仅 APK 可见</span>
          </div>
          <div class="gf-set-tv__rows">
            <div class="gf-tv-listrow">
              <div><div class="lab">运行平台</div></div>
              <span class="right">{{ platformLabel }}</span>
            </div>
            <div class="gf-tv-listrow">
              <div><div class="lab">服务器地址</div></div>
              <span class="right gf-set-tv__mono">{{ serverOrigin }}</span>
            </div>
            <div
              class="gf-tv-listrow"
              data-focusable="true"
              tabindex="0"
              @click="onOpenServer"
              @keydown.enter.prevent="onOpenServer"
            >
              <div>
                <div class="lab">更换服务器地址</div>
                <div class="sub">连按 4 次返回也可呼出</div>
              </div>
              <span class="right"><span class="gf-tv-btn cyan">更换</span></span>
            </div>
          </div>
        </template>

        <!-- 关于 -->
        <template v-else>
          <div class="gf-tv-sec">
            <span class="t">ℹ 关于</span>
            <span class="s">版本与更新</span>
          </div>
          <div class="gf-set-tv__rows">
            <div class="gf-tv-listrow">
              <div><div class="lab">运行平台</div></div>
              <span class="right">{{ platformLabel }}</span>
            </div>
            <div class="gf-tv-listrow">
              <div><div class="lab">构建模式</div></div>
              <span class="right gf-set-tv__mono">{{ buildMode }}</span>
            </div>
            <div
              class="gf-tv-listrow"
              data-focusable="true"
              tabindex="0"
              @click="onCheckUpdate"
              @keydown.enter.prevent="onCheckUpdate"
            >
              <div>
                <div class="lab">检查新版本</div>
                <div class="sub">{{ isNative ? 'GET /app/version/latest' : 'Web 版升级请下载安装包' }}</div>
              </div>
              <span class="right">
                <span class="gf-tv-btn primary">{{ checking ? '检查中…' : '立即检查' }}</span>
              </span>
            </div>
          </div>
        </template>
      </div>
    </div>
  </main>

  <!-- ============ 桌面 / 移动: 原有布局 ============ -->
  <main v-else class="container-page py-[var(--gf-space-6)]">
    <h1 class="text-2xl font-[var(--gf-fw-semibold)] mb-[var(--gf-space-5)]">设置</h1>

    <section class="gf-settings-card">
      <h2 class="gf-settings-card__title">应用信息</h2>
      <div class="gf-settings-row">
        <span class="gf-settings-row__label">运行平台</span>
        <span class="gf-settings-row__value">{{ isNative ? 'Jerocine TV APP' : 'Web 浏览器' }}</span>
      </div>
      <div class="gf-settings-row">
        <span class="gf-settings-row__label">服务器地址</span>
        <span class="gf-settings-row__value font-mono break-all">{{ serverOrigin }}</span>
      </div>
      <div class="gf-settings-row">
        <span class="gf-settings-row__label">构建模式</span>
        <span class="gf-settings-row__value font-mono">{{ buildMode }}</span>
      </div>
    </section>

    <section v-if="isNative" class="gf-settings-card">
      <h2 class="gf-settings-card__title">应用更新</h2>
      <div class="gf-settings-row">
        <span class="gf-settings-row__label">检查新版本</span>
        <BaseButton variant="gradient" :loading="checking" @click="onCheckUpdate">
          <BaseIcon name="refresh" size="16px" /> 立即检查
        </BaseButton>
      </div>
      <div class="gf-settings-row">
        <span class="gf-settings-row__label">服务器地址</span>
        <BaseButton variant="ghost" @click="onOpenServer">
          <BaseIcon name="settings" size="16px" /> 更换
        </BaseButton>
      </div>
    </section>
  </main>
</template>

<style scoped>
.gf-settings-card {
  background: var(--gf-bg-elevated);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-lg);
  padding: var(--gf-space-5);
  margin-bottom: var(--gf-space-4);
}
.gf-settings-card__title {
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-semibold);
  margin-bottom: var(--gf-space-4);
  color: var(--gf-text-primary);
}
.gf-settings-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--gf-space-4);
  padding: var(--gf-space-3) 0;
  border-bottom: 1px solid var(--gf-border-subtle);
}
.gf-settings-row:last-child {
  border-bottom: none;
}
.gf-settings-row__label {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-secondary);
  flex-shrink: 0;
}
.gf-settings-row__value {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-primary);
  text-align: right;
}

/* ============ TV 雷鸟分屏 ============ */
.gf-set-tv {
  padding: var(--gf-space-4) var(--gf-space-6) var(--gf-space-8);
}
.gf-set-tv__split {
  display: grid;
  grid-template-columns: 260px 1fr;
  gap: var(--gf-space-4);
  align-items: start;
}
.gf-set-tv__groups {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.gf-set-tv__group {
  min-height: 56px;
  cursor: pointer;
  text-decoration: none;
}
.gf-set-tv__group.is-active {
  background: var(--gf-brand-gradient);
  border-color: transparent;
}
.gf-set-tv__group.is-active .right {
  color: rgba(255, 255, 255, 0.9);
}
.gf-set-tv__group-ic {
  font-size: 20px;
  line-height: 1;
}
.gf-set-tv__rows {
  display: flex;
  flex-direction: column;
  gap: 11px;
}
.gf-set-tv__toggle-row {
  cursor: pointer;
}
.gf-set-tv__note {
  min-height: 52px;
  background: rgba(74, 209, 229, 0.08);
  border-color: rgba(74, 209, 229, 0.3);
}
.gf-set-tv__note-ic {
  font-size: 18px;
  line-height: 1;
}
.gf-set-tv__mono {
  font-family: monospace;
}
</style>
