<script setup lang="ts">
import { computed } from 'vue'
import { useSiteStore } from '@/stores/site'
import BaseIcon from '@/components/base/BaseIcon.vue'

/**
 * ManageSidebar 三 variant:
 *  - drawer    移动端: Teleport 到 body, 全屏 overlay 容器 + 260 左栏 + 右遮罩
 *  - mini      平板:  fixed 72 图标+标签迷你栏(标签直接可见, 触屏无 hover 也可辨识)
 *  - full      桌面:  fixed 220 (折叠能力已移除)
 *
 * Drawer 用 "全屏 fixed 容器 + 内部 h-full" 模式 (而非 inset-y-0/h-screen),
 * iOS Safari 对 inset-y / 100vh 在地址栏隐现时存在已知歧义, fixed inset-0 是
 * 最稳定的覆盖可见视口做法.
 */
const props = withDefaults(defineProps<{
  variant?: 'drawer' | 'mini' | 'full'
  open?: boolean
}>(), {
  variant: 'full',
  open: false
})

const emit = defineEmits<{
  (e: 'close'): void
}>()

const siteStore = useSiteStore()

/**
 * 折叠能力已移除(用户要求): desktop 固定 220px, 平板 mini 响应式固定 72px ——
 * 都是模式驱动的形态, 不再提供用户手动折叠与持久化。
 */
const collapsed = computed(() => props.variant === 'mini')

/**
 * 样式对齐公开端首页抽屉(jc-mnav): 近黑底 rgba(11,11,15,0.98)、分组纯文字标题、
 * 菜单项带图标、hover 白色 6% 淡底、选中淡紫渐变 —— 视觉语言与首页一致。
 */
interface MenuItem { path: string; label: string; icon: string }
interface MenuGroup { title: string; items: MenuItem[] }

const groups: MenuGroup[] = [
  { title: '概览', items: [{ path: '/manage/index', label: '仪表盘', icon: 'home' }] },
  {
    title: '影视',
    items: [
      { path: '/manage/film', label: '影片列表', icon: 'film' },
      { path: '/manage/film/class', label: '分类管理', icon: 'folder' },
      { path: '/manage/film/add', label: '新增影片', icon: 'plus' },
      { path: '/manage/banner', label: '首页轮播', icon: 'image' }
    ]
  },
  {
    title: '采集',
    items: [
      { path: '/manage/collect', label: '采集源', icon: 'magic' },
      { path: '/manage/collect/jobs', label: '任务监控', icon: 'eye' },
      { path: '/manage/collect/failures', label: '补采中心', icon: 'refresh' },
      { path: '/manage/cron', label: '定时任务', icon: 'clock' }
    ]
  },
  {
    title: '文件',
    items: [
      { path: '/manage/file/upload', label: '文件上传', icon: 'upload' },
      { path: '/manage/file/gallery', label: '文件库', icon: 'file' }
    ]
  },
  {
    title: '数据',
    items: [{ path: '/manage/telemetry', label: '埋点监控', icon: 'chart' }]
  },
  {
    title: '系统',
    items: [
      { path: '/manage/system/site', label: '站点配置', icon: 'settings' },
      { path: '/manage/user', label: '用户管理', icon: 'user' }
    ]
  }
]

function onItemClick(): void {
  if (props.variant === 'drawer') emit('close')
}
</script>

<template>
  <Teleport to="body">
    <!-- ============== DRAWER (移动端) ============== -->
    <Transition name="drawer">
      <div
        v-if="props.variant === 'drawer' && props.open"
        class="jc-drawer-root fixed inset-0 z-[100] flex"
        role="dialog"
        aria-modal="true"
        aria-label="管理菜单"
      >
        <!-- 左侧 260 sidebar 主体 -->
        <aside
          class="jc-drawer-panel w-[260px] h-full bg-[rgba(11,11,15,0.98)] border-r border-subtle flex flex-col shadow-2xl"
          @click.stop
        >
          <!-- 顶部 Brand + 关闭 X -->
          <div class="px-[var(--jc-space-4)] py-[var(--jc-space-3)] border-b border-subtle flex items-center gap-[var(--jc-space-3)] shrink-0 min-h-[56px]">
            <span class="jc-ms__brand truncate flex-1">
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
          <nav class="flex-1 overflow-y-auto py-[var(--jc-space-3)] min-h-0">
            <!-- 返回影视首页 (to="/" 会被前缀匹配, 故不套 active 淡渐变) -->
            <RouterLink
              to="/"
              class="jc-ms__link mb-[var(--jc-space-2)]"
              active-class=""
              data-focusable="true"
              @click="onItemClick"
            >
              <BaseIcon name="home" size="16px" class="jc-ms__link-icon" />
              <span>返回影视首页</span>
            </RouterLink>
            <div
              v-for="g in groups"
              :key="g.title"
              class="mb-[var(--jc-space-3)]"
            >
              <div class="jc-ms__group-title">{{ g.title }}</div>
              <RouterLink
                v-for="it in g.items"
                :key="it.path"
                :to="it.path"
                class="jc-ms__link"
                active-class="jc-ms__link--active"
                data-focusable="true"
                @click="onItemClick"
              >
                <BaseIcon :name="it.icon" size="16px" class="jc-ms__link-icon" />
                <span>{{ it.label }}</span>
              </RouterLink>
            </div>
          </nav>

          <!-- 底部关闭按钮 -->
          <button
            type="button"
            class="border-t border-subtle px-[var(--jc-space-4)] py-[var(--jc-space-3)] flex items-center justify-center gap-[var(--jc-space-2)] text-secondary hover:bg-elevated hover:text-primary transition-colors min-h-[48px] shrink-0 bg-transparent border-l-0 border-r-0 border-b-0 w-full cursor-pointer"
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
          class="jc-drawer-mask flex-1 bg-black/60"
          @click="emit('close')"
        />
      </div>
    </Transition>

    <!-- ============== 非 Drawer (桌面 full / 平板 mini): 顶部直达页顶 ============== -->
    <aside
      v-if="props.variant !== 'drawer'"
      class="fixed top-0 bottom-0 left-0 z-[80] bg-[rgba(11,11,15,0.98)] border-r border-subtle flex flex-col"
      :style="{ width: collapsed ? '72px' : '220px' }"
    >
      <!-- Brand: 首页同款品牌字(渐变色); mini 档宽度所限显示站点名首字 -->
      <div
        class="px-[var(--jc-space-4)] py-[var(--jc-space-4)] border-b border-subtle flex items-center justify-center gap-1 min-h-[56px] shrink-0 min-w-0"
      >
        <template v-if="!collapsed">
          <span class="jc-ms__brand truncate">{{ siteStore.basic?.siteName || 'Jerocine' }}后台</span>
        </template>
        <span
          v-else
          class="jc-ms__brand"
          :title="siteStore.basic?.siteName || 'Jerocine'"
        >{{ (siteStore.basic?.siteName || 'Jerocine').slice(0, 1) }}</span>
      </div>

      <!-- 菜单 -->
      <nav class="flex-1 overflow-y-auto py-[var(--jc-space-3)] min-h-0">
        <!-- 返回影视首页 (to="/" 会被前缀匹配, 故不套 active 淡渐变) -->
        <RouterLink
          to="/"
          class="jc-ms__link mb-[var(--jc-space-2)]"
          :class="{ 'jc-ms__link--mini': collapsed }"
          active-class=""
          :title="collapsed ? '返回影视首页' : undefined"
          data-focusable="true"
        >
          <BaseIcon name="home" size="20px" class="jc-ms__link-icon" />
          <span
            v-if="!collapsed"
            class="jc-ms__link-text"
          >返回影视首页</span>
          <span
            v-else
            class="jc-ms__mini-label"
          >返回影视首页</span>
        </RouterLink>
        <div
          v-for="g in groups"
          :key="g.title"
          class="mb-[var(--jc-space-3)]"
        >
          <div v-if="!collapsed" class="jc-ms__group-title">{{ g.title }}</div>
          <RouterLink
            v-for="it in g.items"
            :key="it.path"
            :to="it.path"
            class="jc-ms__link"
            :class="{ 'jc-ms__link--mini': collapsed }"
            active-class="jc-ms__link--active"
            :title="collapsed ? it.label : undefined"
            data-focusable="true"
          >
            <BaseIcon :name="it.icon" size="20px" class="jc-ms__link-icon" />
            <span
              v-if="!collapsed"
              class="jc-ms__link-text"
            >{{ it.label }}</span>
            <span
              v-else
              class="jc-ms__mini-label"
            >{{ it.label }}</span>
          </RouterLink>
        </div>
      </nav>

    </aside>
  </Teleport>
</template>

<style scoped>
/* ===== 品牌字: 与首页 jc-mnav__brand 同款(字体/字重/渐变色) ===== */
.jc-ms__brand {
  font-family: var(--jc-font-display);
  font-size: var(--jc-fs-lg);
  font-weight: var(--jc-fw-bold);
  background-image: var(--jc-brand-gradient);
  background-clip: text;
  -webkit-background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
  white-space: nowrap;
}

/* ===== 菜单样式: 对齐公开端首页抽屉(jc-mnav)的视觉语言 ===== */
.jc-ms__group-title {
  padding: var(--jc-space-1) var(--jc-space-4) var(--jc-space-2);
  font-size: var(--jc-fs-xs);
  font-weight: var(--jc-fw-semibold);
  letter-spacing: var(--jc-tracking-wide);
  text-transform: uppercase;
  color: var(--jc-text-muted);
}

.jc-ms__link {
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  width: 100%;
  min-height: 44px;
  padding: 0 var(--jc-space-4);
  color: var(--jc-text-secondary);
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-medium);
  text-decoration: none;
  background: transparent;
  transition:
    background-color var(--jc-dur-fast) var(--jc-ease-standard),
    color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-ms__link:hover,
.jc-ms__link:focus-visible {
  background-color: rgba(255, 255, 255, 0.06);
  color: var(--jc-text-primary);
  outline: none;
}
/* 选中: 淡紫渐变底 + 提亮文字(与首页抽屉 is-active 同款), 不再用实心渐变白字 */
.jc-ms__link--active {
  background-image: linear-gradient(90deg, rgba(155, 73, 231, 0.18), rgba(74, 209, 229, 0.08));
  color: var(--jc-text-primary);
}
.jc-ms__link--active:hover {
  background-image: linear-gradient(90deg, rgba(155, 73, 231, 0.18), rgba(74, 209, 229, 0.08));
}
.jc-ms__link-icon {
  color: var(--jc-text-muted);
  flex-shrink: 0;
}
.jc-ms__link:hover .jc-ms__link-icon,
.jc-ms__link--active .jc-ms__link-icon {
  color: var(--jc-text-primary);
}

/* ===== 平板 mini 档: 图标上、中文标签下, 触屏无 hover 也直接可读 ===== */
.jc-ms__link--mini {
  flex-direction: column;
  justify-content: center;
  gap: var(--jc-space-1);
  padding: var(--jc-space-2) 2px;
  min-height: 56px;
}
.jc-ms__mini-label {
  font-size: 10px;
  line-height: 1.2;
  letter-spacing: 0.02em;
  white-space: nowrap;
  color: inherit;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}
.jc-ms__link--mini .jc-ms__link-icon {
  color: var(--jc-text-secondary);
}
.jc-ms__link--mini.jc-ms__link--active .jc-ms__mini-label {
  color: var(--jc-text-primary);
  font-weight: var(--jc-fw-semibold);
}

/* Drawer 进出动画: panel 左滑 + 遮罩淡入 */
.drawer-enter-active .jc-drawer-panel,
.drawer-leave-active .jc-drawer-panel {
  transition: transform var(--jc-dur-base) var(--jc-ease-standard);
}
.drawer-enter-from .jc-drawer-panel,
.drawer-leave-to .jc-drawer-panel {
  transform: translateX(-100%);
}
.drawer-enter-active .jc-drawer-mask,
.drawer-leave-active .jc-drawer-mask {
  transition: opacity var(--jc-dur-base) var(--jc-ease-standard);
}
.drawer-enter-from .jc-drawer-mask,
.drawer-leave-to .jc-drawer-mask {
  opacity: 0;
}
</style>
