<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useUIStore } from '@/stores/ui'
import { useSiteStore } from '@/stores/site'
import BaseIcon from '@/components/base/BaseIcon.vue'

/**
 * ManageSidebar 三 variant:
 *  - drawer    移动端: Teleport 到 body, 全屏 overlay 容器 + 260 左栏 + 右遮罩
 *  - icon-rail 平板:  fixed 64 强制图标栏
 *  - full      桌面:  fixed 64/220 可折叠 (持久化)
 *
 * Drawer 用 "全屏 fixed 容器 + 内部 h-full" 模式 (而非 inset-y-0/h-screen),
 * iOS Safari 对 inset-y / 100vh 在地址栏隐现时存在已知歧义, fixed inset-0 是
 * 最稳定的覆盖可见视口做法.
 */
const props = withDefaults(defineProps<{
  variant?: 'drawer' | 'icon-rail' | 'full'
  open?: boolean
}>(), {
  variant: 'full',
  open: false
})

const emit = defineEmits<{
  (e: 'toggle-collapsed'): void
  (e: 'close'): void
}>()

const uiStore = useUIStore()
const siteStore = useSiteStore()
const { sidebarCollapsed } = storeToRefs(uiStore)

const collapsed = computed(() => {
  if (props.variant === 'icon-rail') return true
  return sidebarCollapsed.value
})

interface MenuItem { path: string; label: string }
interface MenuGroup { title: string; icon: string; items: MenuItem[] }

const groups: MenuGroup[] = [
  { title: '概览', icon: 'home', items: [{ path: '/manage/index', label: '仪表盘' }] },
  {
    title: '影视',
    icon: 'film',
    items: [
      { path: '/manage/film', label: '影片列表' },
      { path: '/manage/film/class', label: '分类管理' },
      { path: '/manage/film/add', label: '新增影片' }
    ]
  },
  {
    title: '采集',
    icon: 'magic',
    items: [
      { path: '/manage/collect/index', label: '采集源' },
      { path: '/manage/collect/jobs', label: '任务监控' },
      { path: '/manage/cron/index', label: '定时任务' }
    ]
  },
  {
    title: '文件',
    icon: 'folder',
    items: [
      { path: '/manage/file/upload', label: '文件上传' },
      { path: '/manage/file/gallery', label: '文件库' }
    ]
  },
  {
    title: '数据',
    icon: 'chart',
    items: [{ path: '/manage/telemetry', label: '埋点监控' }]
  },
  {
    title: '系统',
    icon: 'settings',
    items: [{ path: '/manage/system/webSite', label: '站点配置' }]
  }
]

function onItemClick(): void {
  if (props.variant === 'drawer') emit('close')
}

function onToggleCollapse(): void {
  uiStore.toggleSidebar()
  emit('toggle-collapsed')
}
</script>

<template>
  <Teleport to="body">
    <!-- ============== DRAWER (移动端) ============== -->
    <Transition name="drawer">
      <div
        v-if="props.variant === 'drawer' && props.open"
        class="gf-drawer-root fixed inset-0 z-[100] flex"
        role="dialog"
        aria-modal="true"
        aria-label="管理菜单"
      >
        <!-- 左侧 260 sidebar 主体 -->
        <aside
          class="gf-drawer-panel w-[260px] h-full bg-[#191a23] border-r border-subtle flex flex-col shadow-2xl"
          @click.stop
        >
          <!-- 顶部 Brand + 关闭 X -->
          <div class="px-[var(--gf-space-4)] py-[var(--gf-space-3)] border-b border-subtle flex items-center gap-[var(--gf-space-3)] shrink-0 min-h-[56px]">
            <span class="font-[var(--gf-fw-bold)] italic text-brand-gradient text-[var(--gf-fs-lg)] truncate flex-1">
              {{ siteStore.basic?.siteName || 'Jerocine' }}
            </span>
            <button
              type="button"
              class="w-9 h-9 flex items-center justify-center text-muted hover:text-primary rounded bg-transparent border-0 cursor-pointer"
              aria-label="关闭菜单"
              data-focusable="true"
              @click="emit('close')"
            >
              <BaseIcon name="close" size="20px" />
            </button>
          </div>

          <!-- 菜单 (内部滚) -->
          <nav class="flex-1 overflow-y-auto py-[var(--gf-space-3)] min-h-0">
            <!-- 返回影视首页 (to="/" 会被前缀匹配, 故不套 active gradient) -->
            <RouterLink
              to="/"
              class="flex items-center gap-[var(--gf-space-3)] px-[var(--gf-space-4)] py-[var(--gf-space-3)] mb-[var(--gf-space-2)] text-secondary hover:bg-elevated hover:text-primary transition-colors min-h-[44px]"
              active-class=""
              data-focusable="true"
              @click="onItemClick"
            >
              <BaseIcon name="home" size="18px" />
              <span>返回影视首页</span>
            </RouterLink>
            <div
              v-for="g in groups"
              :key="g.title"
              class="mb-[var(--gf-space-3)]"
            >
              <div class="px-[var(--gf-space-4)] py-[var(--gf-space-2)] text-xs text-muted uppercase tracking-wider flex items-center gap-[var(--gf-space-2)]">
                <BaseIcon :name="g.icon" size="14px" />
                {{ g.title }}
              </div>
              <RouterLink
                v-for="it in g.items"
                :key="it.path"
                :to="it.path"
                class="flex items-center gap-[var(--gf-space-3)] px-[var(--gf-space-4)] py-[var(--gf-space-3)] text-secondary hover:bg-elevated hover:text-primary transition-colors min-h-[44px]"
                active-class="bg-[image:var(--gf-brand-gradient)] text-white shadow-purple-glow"
                data-focusable="true"
                @click="onItemClick"
              >
                <span>{{ it.label }}</span>
              </RouterLink>
            </div>
          </nav>

          <!-- 底部关闭按钮 -->
          <button
            type="button"
            class="border-t border-subtle px-[var(--gf-space-4)] py-[var(--gf-space-3)] flex items-center justify-center gap-[var(--gf-space-2)] text-secondary hover:bg-elevated hover:text-primary transition-colors min-h-[48px] shrink-0 bg-transparent border-l-0 border-r-0 border-b-0 w-full cursor-pointer"
            aria-label="关闭菜单"
            data-focusable="true"
            @click="emit('close')"
          >
            <BaseIcon name="close" size="18px" />
            <span class="text-sm">关闭菜单</span>
          </button>
        </aside>

        <!-- 右侧遮罩 (占满剩余, 点击关闭) -->
        <div
          class="gf-drawer-mask flex-1 bg-black/60"
          @click="emit('close')"
        />
      </div>
    </Transition>

    <!-- ============== 非 Drawer (桌面 full / 平板 icon-rail) ============== -->
    <aside
      v-if="props.variant !== 'drawer'"
      class="fixed top-[56px] bottom-0 left-0 z-[80] bg-[#191a23] border-r border-subtle flex flex-col transition-[width] duration-[var(--gf-dur-base)]"
      :style="{ width: collapsed ? '64px' : '220px' }"
    >
      <!-- Brand (full 模式可点切换折叠) -->
      <button
        type="button"
        class="px-[var(--gf-space-4)] py-[var(--gf-space-4)] border-b border-subtle flex items-center gap-[var(--gf-space-3)] w-full bg-transparent border-l-0 border-r-0 border-t-0 cursor-pointer text-left min-h-[56px] shrink-0"
        :class="{ 'cursor-default': props.variant === 'icon-rail' }"
        :disabled="props.variant === 'icon-rail'"
        :title="collapsed ? '展开侧栏' : '折叠侧栏'"
        :aria-label="collapsed ? '展开侧栏' : '折叠侧栏'"
        :data-focusable="props.variant === 'full' ? 'true' : undefined"
        @click="props.variant === 'full' && onToggleCollapse()"
      >
        <span class="font-[var(--gf-fw-bold)] italic text-brand-gradient text-[var(--gf-fs-lg)] truncate flex-1">
          {{ collapsed ? 'GF' : (siteStore.basic?.siteName || 'Jerocine') }}
        </span>
        <BaseIcon
          v-if="!collapsed && props.variant === 'full'"
          name="chevron-left"
          size="16px"
          class="text-muted shrink-0"
        />
      </button>

      <!-- 菜单 -->
      <nav class="flex-1 overflow-y-auto py-[var(--gf-space-3)] min-h-0">
        <!-- 返回影视首页 (to="/" 会被前缀匹配, 故不套 active gradient) -->
        <RouterLink
          to="/"
          class="flex items-center gap-[var(--gf-space-3)] px-[var(--gf-space-4)] py-[var(--gf-space-3)] mb-[var(--gf-space-2)] text-secondary hover:bg-elevated hover:text-primary transition-colors min-h-[44px]"
          :class="{ 'justify-center': collapsed }"
          active-class=""
          :title="collapsed ? '返回影视首页' : undefined"
          data-focusable="true"
        >
          <BaseIcon name="home" size="20px" />
          <span v-if="!collapsed">返回影视首页</span>
        </RouterLink>
        <div
          v-for="g in groups"
          :key="g.title"
          class="mb-[var(--gf-space-3)]"
        >
          <div
            v-if="!collapsed"
            class="px-[var(--gf-space-4)] py-[var(--gf-space-2)] text-xs text-muted uppercase tracking-wider flex items-center gap-[var(--gf-space-2)]"
          >
            <BaseIcon :name="g.icon" size="14px" />
            {{ g.title }}
          </div>
          <RouterLink
            v-for="it in g.items"
            :key="it.path"
            :to="it.path"
            class="flex items-center gap-[var(--gf-space-3)] px-[var(--gf-space-4)] py-[var(--gf-space-3)] text-secondary hover:bg-elevated hover:text-primary transition-colors min-h-[44px]"
            :class="{ 'justify-center': collapsed }"
            active-class="bg-[image:var(--gf-brand-gradient)] text-white shadow-purple-glow"
            :title="collapsed ? it.label : undefined"
            data-focusable="true"
          >
            <BaseIcon
              v-if="collapsed"
              :name="g.icon"
              size="20px"
            />
            <span v-if="!collapsed">{{ it.label }}</span>
          </RouterLink>
        </div>
      </nav>

    </aside>
  </Teleport>
</template>

<style scoped>
/* Drawer 进出动画: panel 左滑 + 遮罩淡入 */
.drawer-enter-active .gf-drawer-panel,
.drawer-leave-active .gf-drawer-panel {
  transition: transform var(--gf-dur-base) var(--gf-ease-standard);
}
.drawer-enter-from .gf-drawer-panel,
.drawer-leave-to .gf-drawer-panel {
  transform: translateX(-100%);
}
.drawer-enter-active .gf-drawer-mask,
.drawer-leave-active .gf-drawer-mask {
  transition: opacity var(--gf-dur-base) var(--gf-ease-standard);
}
.drawer-enter-from .gf-drawer-mask,
.drawer-leave-to .gf-drawer-mask {
  opacity: 0;
}
</style>
