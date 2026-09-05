<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useSiteStore, useNavStore, useHistoryStore } from '@/stores'
import { useUserStore } from '@/stores/user'
import { useViewMode } from '@/composables/useViewMode'
import { useSearchHistory } from '@/composables/useSearchHistory'
import BaseIcon from '@/components/base/BaseIcon.vue'
import BaseDialog from '@/components/base/BaseDialog.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import LogoMark from '@/components/brand/LogoMark.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'

/**
 * 公开端 Header
 *
 * - 透明 → 滚动后实色（监听 window scroll）
 * - 左：站名（gradient）
 * - 中：搜索框（pill）
 * - 右：导航 + 历史浮层 + 移动端汉堡
 * - 移动端：搜索收成图标 + 抽屉式导航
 * - TV：高度 96px，导航字号放大
 *
 * 数据：useSiteStore / useNavStore / useHistoryStore（不在此处 ensureLoaded，由 App.vue 预热）
 */

const route = useRoute()
const router = useRouter()

const siteStore = useSiteStore()
const navStore = useNavStore()
const historyStore = useHistoryStore()
const userStore = useUserStore()
const { isTV, isMobile } = useViewMode()

const { basic } = storeToRefs(siteStore)
const { list: navList } = storeToRefs(navStore)
const { list: historyList } = storeToRefs(historyStore)
const { isLoggedIn, isAdmin, displayName, info: userInfo } = storeToRefs(userStore)

/** 滚动 → 切实色背景 */
const scrolled = ref(false)
function onScroll(): void {
  scrolled.value = (window.scrollY ?? 0) > 12
}
onMounted(() => {
  onScroll()
  window.addEventListener('scroll', onScroll, { passive: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onScroll)
})

/** 站名 */
const siteName = computed(() => basic.value?.siteName || 'Jerocine')

/** 顶部 6 项导航 */
const topNav = computed(() => navList.value.slice(0, 6))

/** 搜索 */
const keyword = ref<string>(typeof route.query.search === 'string' ? route.query.search : '')
const { history: searchHistory, add: addSearchHistory, remove: removeSearchHistory, clear: clearSearchHistory } =
  useSearchHistory()

/** 搜索建议下拉: focus 时展开, blur 延迟关 (留 click 处理时间) */
const suggestOpen = ref(false)
const suggestIndex = ref(-1) // 键盘高亮下标 (-1 = 输入框自身)
let suggestBlurTimer: number | null = null

/** 热门影片占位 — 之前用"顶层分类名"做 mock 热词混淆了"分类"和"热搜",
 *  用户明确不要分类. 暂时返空数组, 等接入真正的热门影片 API (如 /index 的 banner)
 *  再填充. 当前下拉只显示搜索历史. */
const hotKeywords = computed<string[]>(() => [])

/** 拍平的"可被键盘选中"建议列表: 热词在前, 历史在后 */
const suggestions = computed<string[]>(() => {
  const set = new Set<string>()
  const out: string[] = []
  for (const h of hotKeywords.value) {
    if (h && !set.has(h.toLowerCase())) {
      set.add(h.toLowerCase())
      out.push(h)
    }
  }
  for (const h of searchHistory.value) {
    if (h && !set.has(h.toLowerCase())) {
      set.add(h.toLowerCase())
      out.push(h)
    }
  }
  return out
})

function openSuggest(): void {
  if (suggestBlurTimer !== null) {
    window.clearTimeout(suggestBlurTimer)
    suggestBlurTimer = null
  }
  suggestOpen.value = true
  suggestIndex.value = -1
}
function deferCloseSuggest(): void {
  if (suggestBlurTimer !== null) {
    window.clearTimeout(suggestBlurTimer)
  }
  // 给 mousedown 在 dropdown 上的事件留 180ms 派发空间
  suggestBlurTimer = window.setTimeout(() => {
    suggestOpen.value = false
    suggestIndex.value = -1
    suggestBlurTimer = null
  }, 180)
}

function submitSearch(): void {
  const k = keyword.value.trim()
  if (!k) return
  addSearchHistory(k)
  router.push({ path: '/search', query: { search: k } })
  // 提交后强制收起下拉与抽屉
  suggestOpen.value = false
  mobileSearchOpen.value = false
  mobileMenuOpen.value = false
}

/** 点选建议: 立即跳搜索 */
function pickSuggest(kw: string): void {
  const trimmed = kw.trim()
  if (!trimmed) return
  keyword.value = trimmed
  addSearchHistory(trimmed)
  router.push({ path: '/search', query: { search: trimmed } })
  suggestOpen.value = false
  mobileSearchOpen.value = false
  mobileMenuOpen.value = false
}

/** 单条删除历史 (不收起下拉) */
function removeHistoryItem(kw: string, e: Event): void {
  e.preventDefault()
  e.stopPropagation()
  removeSearchHistory(kw)
}

/** 键盘上下 + 回车 + Esc */
function onSearchKeydown(e: KeyboardEvent): void {
  if (!suggestOpen.value || suggestions.value.length === 0) {
    // 默认行为: Enter 由 form @submit 接住
    return
  }
  switch (e.key) {
    case 'ArrowDown': {
      e.preventDefault()
      suggestIndex.value = (suggestIndex.value + 1) % suggestions.value.length
      break
    }
    case 'ArrowUp': {
      e.preventDefault()
      const len = suggestions.value.length
      suggestIndex.value = (suggestIndex.value - 1 + len) % len
      break
    }
    case 'Enter': {
      if (suggestIndex.value >= 0) {
        e.preventDefault()
        const kw = suggestions.value[suggestIndex.value]
        if (kw) pickSuggest(kw)
      }
      // 否则交给 form submit
      break
    }
    case 'Escape': {
      suggestOpen.value = false
      suggestIndex.value = -1
      break
    }
  }
}

/** 移动端抽屉 */
const mobileMenuOpen = ref(false)
const mobileSearchOpen = ref(false)
function closeMobile(): void {
  mobileMenuOpen.value = false
  mobileSearchOpen.value = false
}

/** 历史浮层（hover 显示，移动端用点击） */
const historyOpen = ref(false)
let historyCloseTimer: number | null = null
function openHistory(): void {
  if (historyCloseTimer !== null) {
    window.clearTimeout(historyCloseTimer)
    historyCloseTimer = null
  }
  historyOpen.value = true
}
function deferCloseHistory(): void {
  if (historyCloseTimer !== null) {
    window.clearTimeout(historyCloseTimer)
  }
  historyCloseTimer = window.setTimeout(() => {
    historyOpen.value = false
    historyCloseTimer = null
  }, 200)
}
function toggleHistory(): void {
  historyOpen.value = !historyOpen.value
}

/** 历史前 8 条（list 已是 timeStamp desc，最新在前） */
const historyTop = computed(() =>
  historyList.value.slice(0, 8).map((it) => ({
    id: it.id,
    name: it.name,
    picture: it.picture,
    link: it.link,
    source: it.source ?? '',
    episode: it.episode
  }))
)

function goHistoryItem(item: {
  id: string
  link?: string
  source?: string
  episode: string
}): void {
  historyOpen.value = false
  // link 形如 /play?id=...&source=...&episode=...&currentTime=...
  // 直接 router.push 字符串路径，可一次性带上 currentTime 续播
  if (item.link) {
    router.push(item.link)
    return
  }
  router.push({
    path: '/play',
    query: { id: item.id, source: item.source, episode: item.episode }
  })
}

/** 当前路由分类 Pid 高亮 */
const activePid = computed<number | null>(() => {
  const pid = route.query.Pid
  const n = Number(pid)
  return Number.isFinite(n) && n > 0 ? n : null
})

/** route 跳转后自动关闭抽屉 */
function isNavActive(id: number): boolean {
  return activePid.value === id
}

/** 用户菜单（已登录） */
const userMenuOpen = ref(false)
let userMenuTimer: number | null = null
function openUserMenu(): void {
  if (userMenuTimer !== null) {
    window.clearTimeout(userMenuTimer)
    userMenuTimer = null
  }
  userMenuOpen.value = true
}
function deferCloseUserMenu(): void {
  if (userMenuTimer !== null) {
    window.clearTimeout(userMenuTimer)
  }
  userMenuTimer = window.setTimeout(() => {
    userMenuOpen.value = false
    userMenuTimer = null
  }, 200)
}
function toggleUserMenu(): void {
  userMenuOpen.value = !userMenuOpen.value
}
function closeUserMenu(): void {
  userMenuOpen.value = false
  if (userMenuTimer !== null) {
    window.clearTimeout(userMenuTimer)
    userMenuTimer = null
  }
}

const userAvatar = computed(() => {
  const a = userInfo.value?.avatar
  if (a && a !== 'empty') return a
  // 默认走本地静态 SVG (之前 dicebear 远程 API 在 TV 网络不稳/被墙时白图)
  return '/default-avatar.svg'
})

function gotoLogin(): void {
  closeMobile()
  closeUserMenu()
  router.push({
    path: '/login',
    query: { redirect: route.fullPath }
  })
}

async function handleLogout(): Promise<void> {
  closeUserMenu()
  await userStore.logout()
  // 退出后留在当前页面（如果是受保护页则前面 401 拦截器已处理跳转）
}

/** ============ 修改密码 (公开端用户菜单) ============ */
const pwdDialogOpen = ref(false)
const pwdSubmitting = ref(false)
const pwdError = ref('')
const pwdForm = reactive({
  password: '',
  newPassword: '',
  confirmPassword: ''
})

function openChangePwd(): void {
  closeUserMenu()
  pwdForm.password = ''
  pwdForm.newPassword = ''
  pwdForm.confirmPassword = ''
  pwdError.value = ''
  pwdDialogOpen.value = true
}

async function submitChangePwd(): Promise<void> {
  pwdError.value = ''
  if (!pwdForm.password) {
    pwdError.value = '请输入原密码'
    return
  }
  if (pwdForm.newPassword.length < 6) {
    pwdError.value = '新密码至少 6 位'
    return
  }
  if (pwdForm.newPassword !== pwdForm.confirmPassword) {
    pwdError.value = '新密码与确认密码不一致'
    return
  }
  pwdSubmitting.value = true
  try {
    await userStore.changePassword({
      password: pwdForm.password,
      newPassword: pwdForm.newPassword
    })
    pwdDialogOpen.value = false
  } catch (e: unknown) {
    pwdError.value = e instanceof Error ? e.message : '修改失败'
  } finally {
    pwdSubmitting.value = false
  }
}

/* ============ window.__gfModalCloser 注册 ============
 * 必须放在所有 ref (mobileMenuOpen/historyOpen/userMenuOpen/mobileSearchOpen) 声明
 * 之后. 之前放在 ref 声明前 watch source 同步 track deps 时引用 TDZ 中的 let, 报
 * "0 is not a function" 整页 vue-error → web 打不开. */
watch(
  () => mobileMenuOpen.value || historyOpen.value || userMenuOpen.value || mobileSearchOpen.value,
  (open) => {
    if (typeof window === 'undefined') return
    const slot = window as unknown as { __gfModalCloser?: () => boolean }
    if (open) {
      slot.__gfModalCloser = (): boolean => {
        if (mobileMenuOpen.value) { mobileMenuOpen.value = false; return true }
        if (mobileSearchOpen.value) { mobileSearchOpen.value = false; return true }
        if (historyOpen.value) { historyOpen.value = false; return true }
        if (userMenuOpen.value) { userMenuOpen.value = false; return true }
        return false
      }
    } else {
      delete slot.__gfModalCloser
    }
  }
)
</script>

<template>
  <header
    class="gf-header"
    :class="scrolled ? 'gf-header--scrolled' : 'gf-header--top'"
    :data-route="route.name as string | undefined"
  >
    <div class="gf-header__inner container-page flex items-center gap-[var(--gf-space-4)]">
      <!-- 汉堡菜单: 仅 mobile. TV 改用顶部频道 Tab 导航(P1-b), 不再用抽屉(焦点恢复耦合根因);
           desktop md+ 隐. 移动端复用 mobileMenuOpen 抽屉. -->
      <button
        v-show="isMobile"
        class="gf-header__icon-btn"
        type="button"
        aria-label="打开菜单"
        data-focusable="true"
        tabindex="0"
        @click="mobileMenuOpen = !mobileMenuOpen"
      >
        <BaseIcon name="menu" size="22px" />
      </button>

      <!-- 站名 / Logo -->
      <RouterLink
        to="/index"
        class="gf-header__brand flex items-center"
        :aria-label="siteName"
        data-focusable="true"
        tabindex="0"
        @click="closeMobile"
      >
        <LogoMark :size="32" :show-text="true" />
      </RouterLink>

      <!-- 主导航（桌面 / TV） -->
      <nav class="gf-header__nav hidden md:flex items-center gap-[var(--gf-space-5)]" data-focus-zone="tab">
        <RouterLink
          to="/index"
          class="gf-header__nav-link"
          :class="route.name === 'home' ? 'is-active' : ''"
          data-focusable="true"
          tabindex="0"
        >
          首页
        </RouterLink>
        <div
          v-for="nav in topNav"
          :key="nav.id"
          class="gf-header__nav-item"
        >
          <RouterLink
            :to="{ path: '/filmClassify', query: { Pid: nav.id } }"
            class="gf-header__nav-link"
            :class="isNavActive(nav.id) ? 'is-active' : ''"
            data-focusable="true"
          >
            {{ nav.name }}
          </RouterLink>
          <!-- 二级子分类悬停下拉 -->
          <div v-if="nav.children?.length" class="gf-header__subnav">
            <RouterLink
              v-for="sub in nav.children"
              :key="sub.id"
              :to="{ path: '/filmClassifySearch', query: { Pid: nav.id, Category: sub.id } }"
              class="gf-header__subnav-link"
              data-focusable="true"
            >
              {{ sub.name }}
            </RouterLink>
          </div>
        </div>
      </nav>

      <!-- 中部弹性 (左 spacer) -->
      <div class="flex-1 hidden md:block" />

      <!-- 桌面搜索框 (常驻, bilibili 风格居中, 宽 480-520px) -->
      <div class="gf-header__search-wrap hidden md:flex">
        <form
          class="gf-header__search flex items-center w-full"
          role="search"
          @submit.prevent="submitSearch"
        >
          <BaseIcon name="search" size="18px" class="gf-header__search-icon" />
          <input
            v-model="keyword"
            type="search"
            placeholder="搜索影片、剧集、动漫…"
            aria-label="搜索"
            class="gf-header__search-input"
            data-focusable="true"
            tabindex="0"
            autocomplete="off"
            role="combobox"
            :aria-expanded="suggestOpen"
            aria-controls="gf-search-suggest"
            :aria-activedescendant="suggestIndex >= 0 ? `gf-search-opt-${suggestIndex}` : undefined"
            @focus="!isTV && openSuggest()"
            @blur="deferCloseSuggest"
            @click="openSuggest"
            @keydown="onSearchKeydown"
          />
        </form>
        <!-- 建议下拉: 热词 + 历史 -->
        <Transition name="gf-suggest">
          <div
            v-if="suggestOpen && (hotKeywords.length || searchHistory.length)"
            id="gf-search-suggest"
            class="gf-header__suggest"
            role="listbox"
            aria-label="搜索建议"
            @mousedown.prevent
          >
            <div v-if="hotKeywords.length" class="gf-header__suggest-section">
              <div class="gf-header__suggest-title">
                <BaseIcon name="fire" size="14px" />
                <span>热门搜索</span>
              </div>
              <ul class="gf-header__suggest-list gf-header__suggest-list--hot">
                <li
                  v-for="(kw, i) in hotKeywords"
                  :id="`gf-search-opt-${i}`"
                  :key="`hot-${kw}`"
                  role="option"
                  :aria-selected="suggestIndex === i"
                  class="gf-header__suggest-chip"
                  :class="[
                    i < 3 ? 'gf-header__suggest-chip--hot' : '',
                    suggestIndex === i ? 'gf-header__suggest-chip--active' : ''
                  ]"
                  @click="pickSuggest(kw)"
                  @mouseenter="suggestIndex = i"
                >
                  <span v-if="i < 3" class="gf-header__suggest-rank">{{ i + 1 }}</span>
                  {{ kw }}
                </li>
              </ul>
            </div>
            <div v-if="searchHistory.length" class="gf-header__suggest-section">
              <div class="gf-header__suggest-title">
                <BaseIcon name="clock" size="14px" />
                <span>搜索历史</span>
                <button
                  type="button"
                  class="gf-header__suggest-clear"
                  aria-label="清空搜索历史"
                  @click="clearSearchHistory()"
                >
                  清空
                </button>
              </div>
              <ul class="gf-header__suggest-list">
                <li
                  v-for="(kw, i) in searchHistory"
                  :id="`gf-search-opt-${hotKeywords.length + i}`"
                  :key="`his-${kw}`"
                  role="option"
                  :aria-selected="suggestIndex === hotKeywords.length + i"
                  class="gf-header__suggest-row"
                  :class="suggestIndex === hotKeywords.length + i ? 'gf-header__suggest-row--active' : ''"
                  @click="pickSuggest(kw)"
                  @mouseenter="suggestIndex = hotKeywords.length + i"
                >
                  <BaseIcon name="clock" size="14px" class="gf-header__suggest-row-icon" />
                  <span class="gf-header__suggest-row-text">{{ kw }}</span>
                  <button
                    type="button"
                    class="gf-header__suggest-row-x"
                    :aria-label="`删除历史 ${kw}`"
                    @click="removeHistoryItem(kw, $event)"
                  >
                    <BaseIcon name="close" size="14px" />
                  </button>
                </li>
              </ul>
            </div>
          </div>
        </Transition>
      </div>

      <!-- 中部弹性 (右 spacer, 与左 spacer 对称, 让搜索框真正居中) -->
      <div class="flex-1 hidden md:block" />

      <!-- 移动端 spacer: 把右侧 icon + user 推到屏幕右边, 避免左密右空 -->
      <div class="flex-1 md:!hidden" />

      <!-- 移动端搜索图标 -->
      <button
        class="gf-header__icon-btn md:!hidden"
        type="button"
        aria-label="搜索"
        data-focusable="true"
        tabindex="0"
        @click="mobileSearchOpen = !mobileSearchOpen"
      >
        <BaseIcon :name="mobileSearchOpen ? 'close' : 'search'" size="22px" />
      </button>

      <!-- (TV 顶部已精简: 搜索/历史/我的 入口下沉到首页金刚区, 顶部只留居中频道导航) -->

      <!-- 历史按钮 + 浮层. TV 也用同一个下拉浮层 (之前 TV 走 BaseDialog 出现大居中弹窗
           按钮无法 D-pad 聚焦, 用户明确要求复用普通下拉) -->
      <div
        class="gf-header__history relative hidden md:block"
        @mouseenter="!isTV && openHistory()"
        @mouseleave="!isTV && deferCloseHistory()"
      >
        <button
          class="gf-header__icon-btn"
          type="button"
          aria-label="观看历史"
          aria-haspopup="menu"
          data-focusable="true"
          tabindex="0"
          :aria-expanded="historyOpen"
          @click="toggleHistory"
        >
          <BaseIcon name="history" size="22px" />
        </button>
        <Transition name="gf-fade">
          <div
            v-if="historyOpen"
            class="gf-header__history-panel"
            role="menu"
            @mouseenter="!isTV && openHistory()"
            @mouseleave="!isTV && deferCloseHistory()"
          >
            <div class="gf-header__history-title">
              <span>最近观看</span>
              <RouterLink
                to="/history"
                class="text-link text-[var(--gf-fs-sm)]"
                @click="historyOpen = false"
              >
                全部
              </RouterLink>
            </div>
            <ul v-if="historyTop.length" class="gf-header__history-list">
              <li
                v-for="item in historyTop"
                :key="item.id + item.source + item.episode"
              >
                <button
                  type="button"
                  class="gf-header__history-item"
                  data-focusable="true"
                  @click="goHistoryItem(item)"
                >
                  <img
                    v-if="item.picture"
                    :src="item.picture"
                    :alt="item.name"
                    loading="lazy"
                    class="gf-header__history-thumb"
                  />
                  <span class="gf-header__history-meta">
                    <span class="gf-header__history-name">{{ item.name }}</span>
                    <span class="gf-header__history-ep">第 {{ item.episode || '1' }} 集</span>
                  </span>
                </button>
              </li>
            </ul>
            <div v-else class="gf-header__history-empty">
              暂无观看记录
            </div>
          </div>
        </Transition>
      </div>

      <!-- 用户菜单：未登录显示"登录"按钮，已登录显示头像 dropdown -->
      <button
        v-if="!isLoggedIn"
        class="gf-header__login-btn"
        type="button"
        data-focusable="true"
        tabindex="0"
        @click="gotoLogin"
      >
        <BaseIcon name="user" size="18px" />
        <span class="hidden md:inline">登录</span>
      </button>

      <div
        v-else
        class="gf-header__user relative"
        @mouseenter="!isTV && openUserMenu()"
        @mouseleave="!isTV && deferCloseUserMenu()"
      >
        <button
          class="gf-header__user-btn"
          type="button"
          data-focusable="true"
          tabindex="0"
          aria-haspopup="menu"
          :aria-expanded="userMenuOpen"
          @click="toggleUserMenu"
        >
          <img
            :src="userAvatar"
            :alt="displayName"
            class="gf-header__avatar"
          />
          <span class="gf-header__username hidden lg:inline">
            {{ displayName }}
          </span>
          <BaseIcon name="chevron-down" size="14px" class="hidden lg:inline" />
        </button>

        <Transition name="gf-fade">
          <div
            v-if="userMenuOpen"
            class="gf-header__user-panel"
            role="menu"
            @mouseenter="openUserMenu"
            @mouseleave="deferCloseUserMenu"
          >
            <div class="gf-header__user-header">
              <img :src="userAvatar" :alt="displayName" class="gf-header__user-avatar" />
              <div class="gf-header__user-info">
                <div class="gf-header__user-name">{{ displayName }}</div>
                <div class="gf-header__user-role">
                  {{ isAdmin ? '管理员' : '普通用户' }}
                </div>
              </div>
            </div>

            <RouterLink
              to="/history"
              class="gf-header__user-item"
              data-focusable="true"
              tabindex="0"
              @click="closeUserMenu"
            >
              <BaseIcon name="history" size="16px" />
              观看历史
            </RouterLink>

            <RouterLink
              to="/favorites"
              class="gf-header__user-item"
              data-focusable="true"
              tabindex="0"
              @click="closeUserMenu"
            >
              <BaseIcon name="heart" size="16px" />
              我的收藏
            </RouterLink>

            <RouterLink
              v-if="isAdmin"
              to="/manage/index"
              class="gf-header__user-item"
              data-focusable="true"
              tabindex="0"
              @click="closeUserMenu"
            >
              <BaseIcon name="settings" size="16px" />
              后台管理
            </RouterLink>

            <button
              type="button"
              class="gf-header__user-item"
              data-focusable="true"
              tabindex="0"
              @click="openChangePwd"
            >
              <BaseIcon name="lock" size="16px" />
              修改密码
            </button>

            <button
              type="button"
              class="gf-header__user-item gf-header__user-item--danger"
              data-focusable="true"
              tabindex="0"
              @click="handleLogout"
            >
              <BaseIcon name="logout" size="16px" />
              退出登录
            </button>
          </div>
        </Transition>
      </div>
    </div>

    <!-- 移动端搜索条（展开） -->
    <Transition name="gf-slide-down">
      <form
        v-if="mobileSearchOpen"
        class="gf-header__mobile-search md:!hidden"
        role="search"
        @submit.prevent="submitSearch"
      >
        <BaseIcon name="search" size="18px" class="gf-header__search-icon" />
        <input
          v-model="keyword"
          type="search"
          placeholder="搜索影片、剧集、动漫…"
          aria-label="搜索"
          class="gf-header__search-input"
          data-focusable="true"
          tabindex="0"
          autofocus
        />
      </form>
    </Transition>

  </header>

  <!-- ============ 移动/TV 抽屉 (Teleport 到 body, 避免 sticky header 内任何潜在截断) ============
       去掉 md:!hidden — TV 在 md+ 视口, 之前 CSS 把抽屉隐藏了, 用户点击汉堡只 toggle state
       但看不见任何东西. 现在依赖 v-if=mobileMenuOpen 控制可见, 浏览器 desktop 模式没汉堡按钮
       (PublicHeader 汉堡 v-show=isMobile||isTV), 自然不会被打开. -->
  <Teleport to="body">
    <Transition name="gf-mobile-overlay-fade">
      <div
        v-if="mobileMenuOpen"
        class="gf-header__mobile-overlay"
        @click="closeMobile"
      />
    </Transition>
    <Transition name="gf-slide-left">
      <aside
        v-if="mobileMenuOpen"
        class="gf-mnav"
        aria-label="主导航"
        role="dialog"
        aria-modal="true"
      >
        <!-- 顶部 brand + X -->
        <div class="gf-mnav__head">
          <span class="gf-mnav__brand">{{ siteName }}</span>
          <button
            type="button"
            class="gf-mnav__close"
            aria-label="关闭菜单"
            data-focusable="true"
            @click="closeMobile"
          >
            <BaseIcon name="close" size="20px" />
          </button>
        </div>

        <!-- 菜单 (内部滚) -->
        <nav class="gf-mnav__body">
          <!-- 浏览 -->
          <div class="gf-mnav__group">
            <div class="gf-mnav__group-title">浏览</div>
            <RouterLink
              to="/index"
              class="gf-mnav__link" data-focusable="true"
              :class="route.name === 'home' ? 'is-active' : ''"
              @click="closeMobile"
            >
              <BaseIcon name="home" size="16px" class="gf-mnav__link-icon" />
              <span>首页</span>
            </RouterLink>
            <!-- 分类(两级): 父级链到分类首页, 子级链到筛选库 -->
            <template v-for="nav in topNav" :key="nav.id">
              <RouterLink
                :to="{ path: '/filmClassify', query: { Pid: nav.id } }"
                class="gf-mnav__link" data-focusable="true"
                :class="isNavActive(nav.id) ? 'is-active' : ''"
                @click="closeMobile"
              >
                <BaseIcon name="film" size="16px" class="gf-mnav__link-icon" />
                <span>{{ nav.name }}</span>
              </RouterLink>
              <div
                v-if="nav.children?.length"
                class="flex flex-wrap gap-[var(--gf-space-2)] pl-[var(--gf-space-8)] pb-[var(--gf-space-2)]"
              >
                <RouterLink
                  v-for="sub in nav.children"
                  :key="sub.id"
                  :to="{ path: '/filmClassifySearch', query: { Pid: nav.id, Category: sub.id } }"
                  class="text-xs text-secondary bg-elevated px-[10px] py-[4px] rounded-[var(--gf-radius-full)] hover:text-primary no-underline"
                  data-focusable="true"
                  @click="closeMobile"
                >
                  {{ sub.name }}
                </RouterLink>
              </div>
            </template>
          </div>

          <!-- 个人 -->
          <div class="gf-mnav__group">
            <div class="gf-mnav__group-title">个人</div>
            <RouterLink to="/history" class="gf-mnav__link" data-focusable="true" @click="closeMobile">
              <BaseIcon name="history" size="16px" class="gf-mnav__link-icon" />
              <span>观看历史</span>
            </RouterLink>
            <RouterLink to="/favorites" class="gf-mnav__link" data-focusable="true" @click="closeMobile">
              <BaseIcon name="heart" size="16px" class="gf-mnav__link-icon" />
              <span>我的收藏</span>
            </RouterLink>
          </div>

          <!-- 管理 (仅 admin) -->
          <div v-if="isLoggedIn && isAdmin" class="gf-mnav__group">
            <div class="gf-mnav__group-title">管理</div>
            <RouterLink to="/manage/index" class="gf-mnav__link" data-focusable="true" @click="closeMobile">
              <BaseIcon name="settings" size="16px" class="gf-mnav__link-icon" />
              <span>后台管理</span>
            </RouterLink>
          </div>

          <!-- 账户 -->
          <div class="gf-mnav__group">
            <div class="gf-mnav__group-title">账户</div>
            <button
              v-if="isLoggedIn"
              type="button"
              class="gf-mnav__link" data-focusable="true"
              @click="closeMobile(); openChangePwd()"
            >
              <BaseIcon name="lock" size="16px" class="gf-mnav__link-icon" />
              <span>修改密码</span>
            </button>
            <RouterLink
              v-if="!isLoggedIn"
              :to="{ path: '/login', query: { redirect: route.fullPath } }"
              class="gf-mnav__link" data-focusable="true"
              @click="closeMobile"
            >
              <BaseIcon name="user" size="16px" class="gf-mnav__link-icon" />
              <span>登录</span>
            </RouterLink>
            <button
              v-else
              type="button"
              class="gf-mnav__link gf-mnav__link--danger"
              data-focusable="true"
              @click="closeMobile(); handleLogout()"
            >
              <BaseIcon name="logout" size="16px" class="gf-mnav__link-icon" />
              <span>退出登录</span>
            </button>
          </div>
        </nav>
      </aside>
    </Transition>
  </Teleport>

<!-- 修改密码弹窗 (公开端用户菜单触发) -->
  <BaseDialog v-model:visible="pwdDialogOpen" title="修改密码">
    <div class="flex flex-col gap-[var(--gf-space-4)]">
      <ManageFormField label="原密码" required>
        <ManageInput v-model="pwdForm.password" type="password" placeholder="原密码" />
      </ManageFormField>
      <ManageFormField label="新密码" required hint="至少 6 位">
        <ManageInput v-model="pwdForm.newPassword" type="password" placeholder="新密码" />
      </ManageFormField>
      <ManageFormField label="确认密码" required>
        <ManageInput
          v-model="pwdForm.confirmPassword"
          type="password"
          placeholder="再次输入新密码"
        />
      </ManageFormField>
      <p v-if="pwdError" class="text-xs text-[var(--gf-danger)]">
        {{ pwdError }}
      </p>
    </div>
    <template #footer>
      <BaseButton variant="ghost" @click="pwdDialogOpen = false">取消</BaseButton>
      <BaseButton variant="gradient" :loading="pwdSubmitting" @click="submitChangePwd">
        确认
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.gf-header {
  position: sticky;
  top: 0;
  z-index: var(--gf-z-header);
  width: 100%;
  transition:
    background-color var(--gf-dur-base) var(--gf-ease-standard),
    backdrop-filter var(--gf-dur-base) var(--gf-ease-standard),
    border-color var(--gf-dur-base) var(--gf-ease-standard);
  border-bottom: 1px solid transparent;
}

.gf-header--top {
  background-color: rgba(11, 11, 15, 0);
  background-image: linear-gradient(
    180deg,
    rgba(0, 0, 0, 0.55) 0%,
    rgba(0, 0, 0, 0) 100%
  );
}

.gf-header--scrolled {
  background-color: var(--gf-bg-header-scrolled);
  backdrop-filter: blur(18px) saturate(140%);
  -webkit-backdrop-filter: blur(18px) saturate(140%);
  border-bottom-color: var(--gf-border-subtle);
}

.gf-header__inner {
  height: 56px;
}

@media (min-width: 768px) {
  .gf-header__inner {
    height: 64px;
  }
}

.gf-header__brand {
  font-family: var(--gf-font-display);
  font-size: var(--gf-fs-xl);
  font-weight: var(--gf-fw-black);
  letter-spacing: var(--gf-tracking-tight);
  text-decoration: none;
  white-space: nowrap;
  outline: none;
  border-radius: var(--gf-radius-md);
  padding: 4px 6px;
  margin-left: -6px;
}

.gf-header__nav {
  margin-left: var(--gf-space-4);
}

.gf-header__nav-item {
  position: relative;
}
/* 二级子分类悬停下拉 (桌面) */
.gf-header__subnav {
  position: absolute;
  top: 100%;
  left: 0;
  min-width: 220px;
  margin-top: 4px;
  padding: var(--gf-space-2);
  background-color: var(--gf-bg-elevated);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-md);
  box-shadow: var(--gf-shadow-card);
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 2px;
  opacity: 0;
  visibility: hidden;
  transform: translateY(4px);
  transition:
    opacity var(--gf-dur-fast) var(--gf-ease-standard),
    transform var(--gf-dur-fast) var(--gf-ease-standard),
    visibility var(--gf-dur-fast) var(--gf-ease-standard);
  z-index: 50;
}
.gf-header__nav-item:hover .gf-header__subnav,
.gf-header__nav-item:focus-within .gf-header__subnav {
  opacity: 1;
  visibility: visible;
  transform: translateY(0);
}
.gf-header__subnav-link {
  white-space: nowrap;
  padding: 6px 10px;
  border-radius: var(--gf-radius-sm);
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  text-decoration: none;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-header__subnav-link:hover,
.gf-header__subnav-link:focus-visible {
  background-color: rgba(255, 255, 255, 0.08);
  color: var(--gf-text-primary);
  outline: none;
}

.gf-header__nav-link {
  position: relative;
  display: inline-flex;
  align-items: center;
  height: 40px;
  padding: 0 4px;
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  text-decoration: none;
  white-space: nowrap;
  transition: color var(--gf-dur-fast) var(--gf-ease-standard);
  border-radius: var(--gf-radius-sm);
  outline: none;
}

.gf-header__nav-link:hover,
.gf-header__nav-link:focus-visible {
  color: var(--gf-text-primary);
}

.gf-header__nav-link.is-active {
  color: var(--gf-text-primary);
  font-weight: var(--gf-fw-semibold);
}

.gf-header__nav-link.is-active::after {
  content: '';
  position: absolute;
  left: 4px;
  right: 4px;
  bottom: 4px;
  height: 2px;
  background-image: var(--gf-brand-gradient);
  border-radius: 2px;
}

/* ===== TV: 顶部频道导航 = 居中胶囊药丸 (对齐设计稿 .tv-nav) ===== */
[data-mode='tv'] .gf-header__inner {
  position: relative;
}
[data-mode='tv'] .gf-header__nav {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  margin: 0;
  gap: 8px;
  padding: 5px;
  background: rgba(0, 0, 0, 0.35);
  border: 1px solid var(--gf-border-subtle);
  border-radius: 999px;
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}
[data-mode='tv'] .gf-header__nav-link {
  height: auto;
  padding: 8px 22px;
  border-radius: 999px;
  font-size: var(--gf-fs-base);
  font-weight: var(--gf-fw-medium);
  color: var(--gf-text-secondary);
}
/* TV 去桌面下划线高亮, 改"白底黑字"药丸 */
[data-mode='tv'] .gf-header__nav-link.is-active {
  background: rgba(255, 255, 255, 0.92);
  color: #0a0a0f;
  font-weight: var(--gf-fw-bold);
}
[data-mode='tv'] .gf-header__nav-link.is-active::after {
  display: none;
}
/* 遥控器聚焦: 药丸浅底 (青色焦点环由全局 [data-focusable]:focus 叠加) */
[data-mode='tv'] .gf-header__nav-link:focus-visible {
  background: rgba(255, 255, 255, 0.18);
  color: var(--gf-text-primary);
}

/* TV 顶部精简: 去掉 logo/历史/用户/登录, 只留居中频道导航(搜索/我的等入口下沉首页金刚区) */
[data-mode='tv'] .gf-header__brand,
[data-mode='tv'] .gf-header__history,
[data-mode='tv'] .gf-header__user,
[data-mode='tv'] .gf-header__login-btn {
  display: none !important;
}

/* 搜索 - bilibili 风格常驻框, PC 480 / 大屏 520 */
.gf-header__search {
  position: relative;
  height: 40px;
  width: 480px;
  max-width: 100%;
  background-color: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: var(--gf-radius-full);
  padding: 0 14px 0 38px;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    border-color var(--gf-dur-fast) var(--gf-ease-standard),
    box-shadow var(--gf-dur-fast) var(--gf-ease-standard);
}

.gf-header__search:focus-within {
  background-color: rgba(255, 255, 255, 0.12);
  border-color: rgba(155, 73, 231, 0.55);
  box-shadow: none;
}

.gf-header__search-icon {
  position: absolute;
  left: 12px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--gf-text-muted);
  pointer-events: none;
}

.gf-header__search-input {
  flex: 1;
  height: 100%;
  background: transparent;
  border: none;
  outline: none;
  color: var(--gf-text-primary);
  font-size: var(--gf-fs-sm);
}

/* iOS Safari: input font-size < 16px focus 时会自动 zoom (不还原).
   mobile 端强制 16px (PC scoped 优先级覆盖了 reset.css 的全局规则, 需就地补) */
@media (max-width: 767px) {
  .gf-header__search-input {
    font-size: 16px;
  }
}

.gf-header__search-input::placeholder {
  color: var(--gf-text-muted);
}

/* 聚焦由外层 .gf-header__search:focus-within 处理(紫边); input 本身不叠加全局 2px outline + 3px 环 */
.gf-header__search-input:focus,
.gf-header__search-input:focus-visible {
  outline: none;
  box-shadow: none;
}

@media (min-width: 1440px) {
  .gf-header__search {
    width: 520px;
  }
}

/* 中等屏幕收窄, 防止挤压 nav / 用户菜单 */
@media (min-width: 768px) and (max-width: 1023px) {
  .gf-header__search {
    width: 360px;
  }
}

/* 搜索 + 下拉建议容器 (relative, 让 dropdown 绝对定位锚到这里) */
.gf-header__search-wrap {
  position: relative;
}

/* 建议下拉面板 */
.gf-header__suggest {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  right: 0;
  z-index: 30;
  background-color: rgba(20, 20, 24, 0.96);
  backdrop-filter: blur(12px);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-lg);
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.5);
  padding: var(--gf-space-3);
  max-height: 480px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-3);
}

.gf-header__suggest-section {
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-2);
}

.gf-header__suggest-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--gf-fs-xs);
  font-weight: var(--gf-fw-semibold);
  color: var(--gf-text-secondary);
  letter-spacing: var(--gf-tracking-wide);
}

.gf-header__suggest-clear {
  margin-left: auto;
  border: none;
  background: transparent;
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-xs);
  cursor: pointer;
  padding: 2px 6px;
  border-radius: var(--gf-radius-sm);
}
.gf-header__suggest-clear:hover,
.gf-header__suggest-clear:focus-visible {
  color: var(--gf-text-primary);
  background-color: rgba(255, 255, 255, 0.08);
  outline: none;
}

/* 热词: chip 网格 (bilibili 风格), 前 3 个紫渐变高亮 */
.gf-header__suggest-list--hot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--gf-space-2);
  list-style: none;
  margin: 0;
  padding: 0;
}

.gf-header__suggest-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 6px 12px;
  border-radius: var(--gf-radius-full);
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-secondary);
  background-color: rgba(255, 255, 255, 0.06);
  border: 1px solid transparent;
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard),
    border-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-header__suggest-chip:hover,
.gf-header__suggest-chip--active {
  background-color: rgba(155, 73, 231, 0.18);
  color: var(--gf-text-primary);
  border-color: rgba(155, 73, 231, 0.45);
}

.gf-header__suggest-chip--hot .gf-header__suggest-rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: var(--gf-radius-sm);
  background-image: var(--gf-brand-gradient);
  color: #fff;
  font-size: 10px;
  font-weight: var(--gf-fw-bold);
}

/* 历史: 行式列表 */
.gf-header__suggest-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}

.gf-header__suggest-row {
  display: flex;
  align-items: center;
  gap: var(--gf-space-2);
  padding: 8px 8px;
  border-radius: var(--gf-radius-sm);
  cursor: pointer;
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-secondary);
}
.gf-header__suggest-row:hover,
.gf-header__suggest-row--active {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--gf-text-primary);
}

.gf-header__suggest-row-icon {
  color: var(--gf-text-muted);
  flex-shrink: 0;
}

.gf-header__suggest-row-text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gf-header__suggest-row-x {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: var(--gf-text-muted);
  cursor: pointer;
  border-radius: 9999px;
  opacity: 0;
  transition:
    opacity var(--gf-dur-fast) var(--gf-ease-standard),
    background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-header__suggest-row:hover .gf-header__suggest-row-x,
.gf-header__suggest-row--active .gf-header__suggest-row-x,
.gf-header__suggest-row-x:focus-visible {
  opacity: 1;
}
.gf-header__suggest-row-x:hover,
.gf-header__suggest-row-x:focus-visible {
  background-color: rgba(255, 255, 255, 0.12);
  color: var(--gf-text-primary);
  outline: none;
}

/* 下拉渐显 */
.gf-suggest-enter-from,
.gf-suggest-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
.gf-suggest-enter-active,
.gf-suggest-leave-active {
  transition:
    opacity var(--gf-dur-fast) var(--gf-ease-standard),
    transform var(--gf-dur-fast) var(--gf-ease-standard);
}

/* 图标按钮 */
.gf-header__icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  min-width: 44px;
  background: transparent;
  border: none;
  border-radius: var(--gf-radius-md);
  color: var(--gf-text-secondary);
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard);
}

.gf-header__icon-btn:hover,
.gf-header__icon-btn:focus-visible {
  background-color: rgba(255, 255, 255, 0.08);
  color: var(--gf-text-primary);
}

/* 历史浮层 */
.gf-header__history-panel {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  width: 320px;
  background-color: var(--gf-bg-surface);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-lg);
  box-shadow: var(--gf-shadow-lg);
  padding: var(--gf-space-3);
  z-index: var(--gf-z-dropdown);
}

.gf-header__history-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--gf-space-2) var(--gf-space-2);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-semibold);
  color: var(--gf-text-primary);
  border-bottom: 1px solid var(--gf-border-subtle);
}

.gf-header__history-list {
  list-style: none;
  margin: 0;
  padding: var(--gf-space-2) 0 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 360px;
  overflow-y: auto;
}

.gf-header__history-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: var(--gf-space-3);
  padding: var(--gf-space-2);
  background: transparent;
  border: none;
  border-radius: var(--gf-radius-md);
  color: var(--gf-text-primary);
  cursor: pointer;
  text-align: left;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}

.gf-header__history-item:hover,
.gf-header__history-item:focus-visible {
  background-color: rgba(255, 255, 255, 0.06);
  outline: none;
}

.gf-header__history-thumb {
  width: 56px;
  height: 36px;
  object-fit: cover;
  border-radius: var(--gf-radius-sm);
  background-color: var(--gf-bg-elevated);
  flex-shrink: 0;
}

.gf-header__history-meta {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.gf-header__history-name {
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  color: var(--gf-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gf-header__history-ep {
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  margin-top: 2px;
}

.gf-header__history-empty {
  padding: var(--gf-space-6) var(--gf-space-2);
  text-align: center;
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-sm);
}

/* 用户菜单 / 登录按钮 */
.gf-header__login-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--gf-space-2);
  height: 36px;
  padding: 0 var(--gf-space-3);
  border-radius: var(--gf-radius-full);
  background-color: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.14);
  color: var(--gf-text-primary);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    border-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-header__login-btn:hover,
.gf-header__login-btn:focus-visible {
  background-color: rgba(255, 255, 255, 0.14);
  border-color: rgba(255, 255, 255, 0.24);
  outline: none;
}

.gf-header__user-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--gf-space-2);
  height: 40px;
  padding: 2px var(--gf-space-2);
  border-radius: var(--gf-radius-full);
  /* 宽度跟随内容, 不被父级 flex 拉伸; 防止整体撑宽 */
  flex: 0 0 auto;
  max-width: 200px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--gf-text-secondary);
  cursor: pointer;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    border-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-header__user-btn:hover,
.gf-header__user-btn:focus-visible {
  background-color: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.14);
  color: var(--gf-text-primary);
  outline: none;
}

.gf-header__avatar {
  width: 32px;
  height: 32px;
  border-radius: 9999px;
  object-fit: cover;
  background-color: var(--gf-bg-elevated);
  border: 1px solid rgba(255, 255, 255, 0.14);
}

.gf-header__username {
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  /* 宽度自适应文字, 设上限避免长名撑宽顶栏(放宽上限, 让常见长账号名完整显示) */
  max-width: clamp(96px, 14vw, 220px);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gf-header__user-panel {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  width: 240px;
  background-color: var(--gf-bg-surface);
  border: 1px solid var(--gf-border-subtle);
  border-radius: var(--gf-radius-lg);
  box-shadow: var(--gf-shadow-lg);
  padding: var(--gf-space-3);
  z-index: var(--gf-z-dropdown);
}

.gf-header__user-header {
  display: flex;
  align-items: center;
  gap: var(--gf-space-3);
  padding: var(--gf-space-2);
  border-bottom: 1px solid var(--gf-border-subtle);
  margin-bottom: var(--gf-space-2);
}
.gf-header__user-avatar {
  width: 44px;
  height: 44px;
  border-radius: 9999px;
  object-fit: cover;
  background-color: var(--gf-bg-elevated);
}
.gf-header__user-info {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.gf-header__user-name {
  font-size: var(--gf-fs-md);
  font-weight: var(--gf-fw-semibold);
  color: var(--gf-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gf-header__user-role {
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  margin-top: 2px;
}

.gf-header__user-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: var(--gf-space-3);
  padding: var(--gf-space-3) var(--gf-space-2);
  background: transparent;
  border: none;
  border-radius: var(--gf-radius-md);
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  cursor: pointer;
  text-align: left;
  text-decoration: none;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-header__user-item:hover,
.gf-header__user-item:focus-visible {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--gf-text-primary);
  outline: none;
}
.gf-header__user-item--danger {
  color: var(--gf-danger);
}
.gf-header__user-item--danger:hover {
  background-color: rgba(255, 71, 87, 0.12);
}

/* 移动端搜索条 */
.gf-header__mobile-search {
  position: relative;
  display: flex;
  align-items: center;
  margin: 0 var(--gf-gutter-mobile) var(--gf-space-3);
  height: 40px;
  background-color: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: var(--gf-radius-full);
  padding: 0 14px 0 38px;
}

/* ============ 移动端抽屉 (Teleport 到 body, 整组样式) ============ */
.gf-header__mobile-overlay {
  position: fixed;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.5);
  /* 高于 --gf-z-header (100), 低于 mnav 自身 */
  z-index: 899;
}

.gf-mnav {
  position: fixed;
  top: 0;
  left: 0;
  bottom: 0;
  width: min(72vw, 260px);
  display: flex;
  flex-direction: column;
  background-color: rgba(11, 11, 15, 0.98);
  border-right: 1px solid var(--gf-border-subtle);
  box-shadow: 12px 0 32px rgba(0, 0, 0, 0.5);
  /* 必须 > --gf-z-header (100), 用 overlay 层级 (900) 直接盖住 header 含汉堡按钮 */
  z-index: 900;
}

.gf-mnav__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--gf-space-3);
  padding: var(--gf-space-3) var(--gf-space-4);
  border-bottom: 1px solid var(--gf-border-subtle);
  flex-shrink: 0;
  min-height: 56px;
}
.gf-mnav__brand {
  font-family: var(--gf-font-display);
  font-size: var(--gf-fs-lg);
  font-weight: var(--gf-fw-bold);
  background-image: var(--gf-brand-gradient);
  background-clip: text;
  -webkit-background-clip: text;
  color: transparent;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gf-mnav__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  background: transparent;
  border: none;
  color: var(--gf-text-muted);
  cursor: pointer;
  border-radius: var(--gf-radius-md);
  flex-shrink: 0;
}
.gf-mnav__close:hover,
.gf-mnav__close:focus-visible {
  color: var(--gf-text-primary);
  background-color: rgba(255, 255, 255, 0.08);
  outline: none;
}

.gf-mnav__body {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding: var(--gf-space-3) 0;
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-4);
}

.gf-mnav__group {
  display: flex;
  flex-direction: column;
}
.gf-mnav__group-title {
  padding: var(--gf-space-1) var(--gf-space-4);
  font-size: var(--gf-fs-xs);
  font-weight: var(--gf-fw-semibold);
  letter-spacing: var(--gf-tracking-wide);
  text-transform: uppercase;
  color: var(--gf-text-muted);
}

.gf-mnav__link {
  display: flex;
  align-items: center;
  gap: var(--gf-space-3);
  width: 100%;
  min-height: 44px;
  padding: 0 var(--gf-space-4);
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  text-decoration: none;
  background: transparent;
  border: none;
  cursor: pointer;
  text-align: left;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-mnav__link:hover,
.gf-mnav__link:focus-visible,
.gf-mnav__link.is-active {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--gf-text-primary);
  outline: none;
}
.gf-mnav__link.is-active {
  background-image: linear-gradient(
    90deg,
    rgba(155, 73, 231, 0.18),
    rgba(74, 209, 229, 0.08)
  );
}
.gf-mnav__link-icon {
  color: var(--gf-text-muted);
  flex-shrink: 0;
}
.gf-mnav__link.is-active .gf-mnav__link-icon,
.gf-mnav__link:hover .gf-mnav__link-icon {
  color: var(--gf-text-primary);
}
.gf-mnav__link--danger {
  color: var(--gf-danger);
}
.gf-mnav__link--danger:hover {
  background-color: rgba(255, 71, 87, 0.12);
  color: var(--gf-danger);
}
.gf-mnav__link--danger .gf-mnav__link-icon {
  color: var(--gf-danger);
}

/* 过渡 */
.gf-fade-enter-active,
.gf-fade-leave-active {
  transition: opacity var(--gf-dur-fast) var(--gf-ease-standard),
    transform var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-fade-enter-from,
.gf-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.gf-slide-down-enter-active,
.gf-slide-down-leave-active {
  transition: opacity var(--gf-dur-base) var(--gf-ease-standard),
    transform var(--gf-dur-base) var(--gf-ease-standard);
  overflow: hidden;
}
.gf-slide-down-enter-from,
.gf-slide-down-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

/* 移动端抽屉: 从左侧滑入 + 遮罩淡入 */
.gf-slide-left-enter-active,
.gf-slide-left-leave-active {
  transition: transform var(--gf-dur-base) var(--gf-ease-standard);
}
.gf-slide-left-enter-from,
.gf-slide-left-leave-to {
  transform: translateX(-100%);
}
.gf-mobile-overlay-fade-enter-active,
.gf-mobile-overlay-fade-leave-active {
  transition: opacity var(--gf-dur-base) var(--gf-ease-standard);
}
.gf-mobile-overlay-fade-enter-from,
.gf-mobile-overlay-fade-leave-to {
  opacity: 0;
}
</style>

<style>
/* TV history Dialog 内容 */
.gf-tv-history__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--gf-space-3);
  max-height: 60vh;
  overflow-y: auto;
}
.gf-tv-history__item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: var(--gf-space-3);
  padding: var(--gf-space-3);
  background: var(--gf-bg-elevated);
  border: none;
  border-radius: var(--gf-radius-md);
  color: var(--gf-text-primary);
  cursor: pointer;
  text-align: left;
  outline: none;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-tv-history__item:focus,
.gf-tv-history__item:focus-visible {
  outline: none;
  background-color: rgba(255, 255, 255, 0.08);
}
.gf-tv-history__thumb {
  width: 96px;
  height: 64px;
  object-fit: cover;
  border-radius: var(--gf-radius-sm);
  background-color: var(--gf-bg-base);
  flex-shrink: 0;
}
.gf-tv-history__meta {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.gf-tv-history__name {
  font-size: var(--gf-fs-md);
  font-weight: var(--gf-fw-semibold);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.gf-tv-history__ep {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
  margin-top: 4px;
}
.gf-tv-history__empty {
  padding: var(--gf-space-8) var(--gf-space-2);
  text-align: center;
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-md);
}

/* TV 模式覆盖：高度放大、字号放大、强制实色背景（避免透明导航被忽略）
 * 关键: TV CSS 视口实际只有 960px (因 dpr=2 + width=device-width). header 内
 * 不能用固定 px 撑 — 这里全部改用 flex 自适应 + 缩小硬编码宽度. */
[data-mode='tv'] .gf-header__inner {
  height: 80px; /* 96 → 80, 节省高度 */
  padding-inline: var(--gf-tv-safe);
  gap: var(--gf-space-3); /* 让 nav / search 之间间距收紧 */
}
[data-mode='tv'] .gf-header__brand {
  font-size: var(--gf-fs-xl); /* 2xl → xl, 缩小 logo 文字宽度 */
  flex: 0 0 auto;
}
[data-mode='tv'] .gf-header__nav {
  flex: 1 1 auto;
  min-width: 0; /* 允许 nav 收缩, 不撑爆 */
  /* overflow-x clip 裁横向溢出, 但纵向 visible — 否则 nav-link 焦点框上下被截断 */
  overflow-x: clip;
  overflow-y: visible;
}
[data-mode='tv'] .gf-header__nav-link {
  height: 48px;
  font-size: var(--gf-fs-sm); /* md → sm, nav 文字小一档 */
  padding: 0 var(--gf-space-2);
}
[data-mode='tv'] .gf-header__icon-btn {
  width: 48px;
  height: 48px;
}
[data-mode='tv'] .gf-header__search {
  height: 48px;
  /* 420 → 自适应: 最大 280, 在窄视口下进一步收缩 */
  width: clamp(160px, 20vw, 280px);
  flex: 0 1 auto;
  font-size: var(--gf-fs-sm);
}
[data-mode='tv'] .gf-header__search-input {
  font-size: var(--gf-fs-sm);
}
/* TV 顶栏悬浮玻璃: 顶部(覆盖 hero 上方)用半透明渐变, 滚动后转实色(复用 scrolled 态).
 * 之前 TV 无 hero 故强制实色; 现首页有沉浸 Banner, 顶栏悬浮其上更沉浸. */
[data-mode='tv'] .gf-header--top {
  background-color: transparent;
  background-image: var(--gf-tv-header-float);
  border-bottom-color: transparent;
}
[data-mode='tv'] .gf-header--scrolled {
  background-color: var(--gf-tv-header-solid);
  backdrop-filter: none; /* 弱 WebView 防掉帧 */
  -webkit-backdrop-filter: none;
  border-bottom-color: var(--gf-border-subtle);
}
/* TV: 内联搜索框改为右端搜索图标(goSearch); 二级悬停下拉删除(focus-within 会在 D-pad 聚焦时误弹) */
[data-mode='tv'] .gf-header__search-wrap {
  display: none;
}
[data-mode='tv'] .gf-header__subnav {
  display: none !important;
}
/* TV: 当前频道 Tab 选中态(青色胶囊) */
[data-mode='tv'] .gf-header__nav-link.is-active {
  color: var(--gf-brand-cyan);
  background-color: var(--gf-tv-selected-bg);
  border-radius: var(--gf-radius-full);
}

/* TV 焦点环：导航 / 图标 / 搜索.
 * 顶栏高度有限, box-shadow 环会被 header/nav 上下边界裁切 → 一律改 outline(随圆角,
 * 不被祖先 overflow/边界裁切), 细环 2px + offset, 配合柔光. */
[data-mode='tv'] .gf-header__nav-link:focus,
[data-mode='tv'] .gf-header__nav-link:focus-visible,
[data-mode='tv'] .gf-header__brand:focus,
[data-mode='tv'] .gf-header__brand:focus-visible {
  outline: 2px solid var(--gf-brand-cyan);
  outline-offset: 1px;
  box-shadow: 0 0 12px rgba(74, 209, 229, 0.4);
  /* 跟随药丸圆角(原 radius-sm 会把聚焦/选中的频道 tab 描边压成方形) */
  border-radius: var(--gf-radius-full);
  color: var(--gf-text-primary);
  /* 不放大: TV 全局 focus 的 scale(1.08) 会让药丸 tab 与相邻 tab 交叉重叠 */
  transform: none;
}
[data-mode='tv'] .gf-header__icon-btn:focus,
[data-mode='tv'] .gf-header__icon-btn:focus-visible {
  outline: 2px solid var(--gf-brand-cyan);
  outline-offset: 1px;
  box-shadow: 0 0 12px rgba(74, 209, 229, 0.4);
  background-color: rgba(255, 255, 255, 0.12);
  color: var(--gf-text-primary);
}
[data-mode='tv'] .gf-header__search:focus-within {
  outline: 2px solid var(--gf-brand-cyan);
  outline-offset: 1px;
  box-shadow: 0 0 12px rgba(74, 209, 229, 0.4);
  background-color: rgba(255, 255, 255, 0.12);
}
</style>
