<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { manageApi } from '@/api'
import type { FilmClass } from '@/types/manage'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'
import { openManageDialog } from '@/composables/useManageDialog'
import FilmClassFormDialog from './FilmClassFormDialog.vue'

const tree = ref<FilmClass[]>([])
const loading = ref(true)
const loadError = ref('')

const { run } = useAsyncAction()

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    tree.value = await manageApi.film.classTree()
  } catch (e) {
    // 失败走显式错误态+重试, 不冒充"暂无分类数据"(全局拦截器已 toast, 页面主体不能撒谎)
    loadError.value = (e as Error).message ?? '加载失败'
  } finally {
    loading.value = false
  }
}

// ---- 弹窗: openManageDialog 函数化发起, 表单校验/提交自治于 FilmClassFormDialog ----

async function openDialog(options: {
  title: string
  initial: FilmClass | null
  pid?: number
}): Promise<void> {
  const payload = await openManageDialog<FilmClass>({
    title: options.title,
    component: FilmClassFormDialog,
    props: { initial: options.initial, pid: options.pid },
    mobileMode: 'sheet'
  })
  // null = 取消/ESC/遮罩; confirm = 保存成功(弹窗内已落库), 页面重拉树
  if (payload) await load()
}

function openEdit(node: FilmClass): void {
  void openDialog({ title: '编辑分类', initial: node })
}

function openAddTop(): void {
  void openDialog({ title: '新增顶级分类', initial: null })
}

function openAddChild(parent: FilmClass): void {
  void openDialog({ title: `在「${parent.name}」下新增子分类`, initial: null, pid: parent.id })
}

// toggleHelper: 在 tree ref 里按 id 找到节点并就地更新字段(避免全页重载)。
// 返回 true = 找到了(需要走 API 路径), false = 没找到(仅本地预览, 仍发 API 但不清树)。
function updateNodeById(id: number, patch: Partial<FilmClass>): boolean {
  function walk(list: FilmClass[]): boolean {
    for (const n of list) {
      if (n.id === id) { Object.assign(n, patch); return true }
      if (n.children?.length && walk(n.children)) return true
    }
    return false
  }
  return walk(tree.value)
}

/** 开关写操作统一走 useAsyncAction: silent 关拦截器双响, 失败 toast 由 run 单点;
 *  成功不弹 toast(开关视觉翻转即反馈)。乐观更新在 action 内, 失败回滚后 rethrow。 */
function toggleAction(node: FilmClass, patch: Partial<FilmClass>, rollback: Partial<FilmClass>): void {
  void run({
    action: async () => {
      updateNodeById(node.id, patch)
      try {
        await manageApi.film.classUpdate({ ...node, ...patch }, { silent: true })
      } catch (e) {
        // 回滚乐观更新, 不与服务端真实状态漂移
        updateNodeById(node.id, rollback)
        throw e
      }
    }
  })
}

function toggleShow(node: FilmClass): void {
  toggleAction(node, { show: !node.show }, { show: node.show })
}

function toggleSidebarShow(node: FilmClass): void {
  toggleAction(node, { sidebarShow: !node.sidebarShow }, { sidebarShow: node.sidebarShow })
}

/** 影片可见开关: 关闭 = 该分类影片从公开列表/搜索全部剔除。
 *  服务端强制联动(000037 迁移: film_visible=false 时节点+子孙 show/sidebarShow 一并置 false);
 *  前端同步级联更新视觉(本地 tree ref 就地改, 避免重拉整棵树)。 */
function toggleFilmVisible(node: FilmClass): void {
  const next = !node.filmVisible
  // 记录子级原值: 失败时连同级联视觉一起回滚
  const cascaded = !next && node.children?.length
    ? node.children.map((c) => ({ id: c.id, show: c.show, sidebarShow: c.sidebarShow }))
    : []
  void run({
    action: async () => {
      updateNodeById(node.id, { filmVisible: next })
      if (cascaded.length) {
        for (const c of cascaded) {
          updateNodeById(c.id, { show: false, sidebarShow: false })
        }
      }
      // API 只发本节点: 关 → 子孙 show/sidebarShow 的级联落库由服务端 cascadeHideDescendants
      // 兜底(子级 filmVisible 不动, 顶级恢复可见时子级行为不突变); 开 → 只本节点恢复,
      // 子孙两开关保持 false 由用户手动逐个开。保存后服务端 InvalidateCategory, 下次
      // load() 即最新值 —— 前端不重复发子级请求(级联 API 由服务端单点负责, 避免双份写)。
      try {
        await manageApi.film.classUpdate({ ...node, filmVisible: next }, { silent: true })
      } catch (e) {
        updateNodeById(node.id, { filmVisible: !next })
        for (const c of cascaded) {
          updateNodeById(c.id, { show: c.show, sidebarShow: c.sidebarShow })
        }
        throw e
      }
    }
  })
}

/** 影片数展示: 缺省(计数失败)显示 — */
function filmCountLabel(node: FilmClass): string {
  return typeof node.filmCount === 'number' ? `${node.filmCount} 部` : '—'
}

function remove(node: FilmClass): void {
  void run({
    confirm: {
      title: '确认删除分类？',
      desc: `「${node.name}」及其全部子分类将被永久删除`,
      okText: '删除',
      danger: true
    },
    action: () => manageApi.film.classDel(node.id, { silent: true }),
    successMsg: '已删除',
    after: () => load()
  })
}

onMounted(() => {
  void load()
})
</script>

<template>
  <section class="bg-surface rounded-card shadow-card p-[var(--jc-space-4)] md:p-[var(--jc-space-5)]">
    <header class="flex items-center justify-between gap-[var(--jc-space-3)] mb-[var(--jc-space-4)]">
      <div class="min-w-0">
        <h2 class="text-lg font-[var(--jc-fw-semibold)]">影视分类管理</h2>
        <p class="text-sm text-muted truncate">维护顶级与子分类的菜单显隐、影片可见性、名称与排序</p>
      </div>
      <div class="flex items-center gap-[var(--jc-space-2)] shrink-0">
        <ManageButton variant="ghost" size="sm" @click="load">
          <BaseIcon name="refresh" size="16px" />
          <span class="hidden sm:inline">刷新</span>
        </ManageButton>
        <ManageButton variant="gradient" size="sm" @click="openAddTop">
          <BaseIcon name="plus" size="16px" /> 新增分类
        </ManageButton>
      </div>
    </header>

    <div v-if="loading" class="flex flex-col gap-[var(--jc-space-3)]">
      <BaseSkeleton v-for="i in 4" :key="i" shape="rect" height="64px" />
    </div>

    <!-- 加载失败: 显式错误态+重试, 不冒充"暂无分类数据" -->
    <div v-else-if="loadError" class="flex flex-col items-center gap-[var(--jc-space-3)] py-[var(--jc-space-8)]">
      <BaseEmpty title="加载失败" :description="loadError" hide-icon />
      <ManageButton variant="outline" size="sm" @click="load">
        <BaseIcon name="refresh" size="16px" /> 重试
      </ManageButton>
    </div>

    <BaseEmpty v-else-if="!tree.length" description="暂无分类数据" />

    <div v-else class="flex flex-col gap-[var(--jc-space-3)]">
      <article v-for="parent in tree" :key="parent.id" class="jc-cat">
        <!-- 父级 -->
        <div
          class="jc-cat__parent"
          data-focusable="true"
          tabindex="0"
          role="button"
          :aria-label="`编辑分类 ${parent.name}`"
          @click="openEdit(parent)"
          @keydown.enter="openEdit(parent)"
        >
          <span class="jc-cat__bar" aria-hidden="true" />
          <div class="jc-cat__title">
            <span class="jc-cat__name" :class="{ 'jc-cat--off': !parent.filmVisible }">{{ parent.name }}</span>
            <span class="jc-cat__meta">
              #{{ parent.id }} · 排序 {{ parent.sort ?? 0 }} · {{ parent.children?.length ?? 0 }} 子分类 ·
              <span :class="{ 'jc-cat__films-hidden': !parent.filmVisible }">{{ filmCountLabel(parent) }}</span>
            </span>
          </div>
          <!-- 影片可见开关: 关闭 = 该分类影片从公开列表/搜索全部剔除(pid 过滤天然覆盖子分类) -->
          <label class="jc-cat__toggle" @click.stop title="关闭后该分类影片从公开列表与搜索中剔除；默认可见">
              <span class="jc-cat__state jc-cat__state--films" :class="{ 'jc-cat__state--films-off': !parent.filmVisible }">
              {{ parent.filmVisible ? '可见' : '不可见' }}
            </span>
            <ManageSwitch :model-value="parent.filmVisible" @update:model-value="toggleFilmVisible(parent)" />
          </label>
          <!-- 侧边栏显示开关: 控制桌面常驻侧栏 + 移动抽屉中该子分类的显隐; 二级无顶栏显示(仅一级有意义) -->
          <label class="jc-cat__toggle" :title="`侧边栏显示${parent.sidebarShow ? '' : '（已关）'}`">
            <span class="jc-cat__state" :class="{ 'jc-cat__state--on': parent.sidebarShow }">
              {{ parent.sidebarShow ? '侧栏显示' : '侧栏隐藏' }}
            </span>
            <ManageSwitch
              :model-value="parent.sidebarShow"
              :disabled="!parent.filmVisible"
              @update:model-value="toggleSidebarShow(parent)"
            />
          </label>
          <!-- 顶栏导航显示开关: 仅一级有效, 控制 web/TV 顶部一级导航 -->
          <label class="jc-cat__toggle" title="只控制顶部导航栏是否显示该一级分类; 隐藏后影片仍可搜索、在列表看到">
            <span class="jc-cat__state" :class="{ 'jc-cat__state--on': parent.show }">
              {{ parent.show ? '顶栏显示' : '顶栏隐藏' }}
            </span>
            <ManageSwitch
              :model-value="parent.show"
              :disabled="!parent.filmVisible"
              @update:model-value="toggleShow(parent)"
            />
          </label>
          <!-- 删除入口(此前只有子级行有): 服务端同事务级联删子分类, 文案与之对齐 -->
          <span class="jc-cat__cctrl" @click.stop>
            <button
              type="button"
              class="jc-cat__del"
              :aria-label="`删除 ${parent.name}`"
              data-focusable="true"
              @click="remove(parent)"
            >
              <BaseIcon name="trash" size="16px" />
            </button>
          </span>
        </div>

        <!-- 子级 (始终渲染, 末尾保留新增子分类入口) -->
        <ul class="jc-cat__children">
          <li
            v-for="child in parent.children"
            :key="child.id"
            class="jc-cat__child"
            data-focusable="true"
            tabindex="0"
            role="button"
            :aria-label="`编辑子分类 ${child.name}`"
            @click="openEdit(child)"
            @keydown.enter="openEdit(child)"
          >
            <span class="jc-cat__dot" :class="{ 'jc-cat__dot--on': child.filmVisible }" aria-hidden="true" />
            <span class="jc-cat__cname" :class="{ 'jc-cat--off': !child.filmVisible }">{{ child.name }}</span>
            <span class="jc-cat__cid">#{{ child.id }} · {{ filmCountLabel(child) }}</span>
          <div class="jc-cat__cctrl" @click.stop>
            <label class="jc-cat__toggle" title="关闭后该分类影片从公开列表与搜索中剔除；默认可见">
                <span class="jc-cat__state jc-cat__state--sm jc-cat__state--films" :class="{ 'jc-cat__state--films-off': !child.filmVisible }">
                  {{ child.filmVisible ? '可见' : '不可见' }}
                </span>
              <ManageSwitch :model-value="child.filmVisible" @update:model-value="toggleFilmVisible(child)" />
            </label>
            <label class="jc-cat__toggle" :title="`侧边栏显示${child.sidebarShow ? '' : '（已关）'}`">
              <span class="jc-cat__state jc-cat__state--sm" :class="{ 'jc-cat__state--on': child.sidebarShow }">
                {{ child.sidebarShow ? '侧栏显示' : '侧栏隐藏' }}
              </span>
              <ManageSwitch
                :model-value="child.sidebarShow"
                :disabled="!child.filmVisible"
                @update:model-value="toggleSidebarShow(child)"
              />
            </label>
              <button
                type="button"
                class="jc-cat__del"
                :aria-label="`删除 ${child.name}`"
                data-focusable="true"
                @click="remove(child)"
              >
                <BaseIcon name="trash" size="16px" />
              </button>
            </div>
          </li>
          <li>
            <button
              type="button"
              class="jc-cat__add"
              data-focusable="true"
              @click="openAddChild(parent)"
            >
              <BaseIcon name="plus" size="14px" /> 新增「{{ parent.name }}」子分类
            </button>
          </li>
        </ul>
      </article>
    </div>
  </section>
</template>

<style scoped>
.jc-cat {
  border: 1px solid var(--jc-border-default);
  border-radius: var(--jc-radius-lg);
  background-color: var(--jc-bg-elevated);
  overflow: hidden;
}

/* ---- 父级 ---- */
.jc-cat__parent {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--jc-space-3);
  min-height: 56px;
  padding: var(--jc-space-3) var(--jc-space-4) var(--jc-space-3) var(--jc-space-5);
  cursor: pointer;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-cat__parent:hover {
  background-color: rgba(255, 255, 255, 0.04);
}
.jc-cat__bar {
  position: absolute;
  left: 0;
  top: var(--jc-space-2);
  bottom: var(--jc-space-2);
  width: 3px;
  border-radius: 0 3px 3px 0;
  background-image: var(--jc-brand-gradient);
}
.jc-cat__title {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}
.jc-cat__name {
  font-weight: var(--jc-fw-semibold);
  color: var(--jc-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.jc-cat__meta {
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
  font-family: var(--jc-font-mono);
}

/* ---- 子级 ---- */
.jc-cat__children {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--jc-border-subtle);
  background-color: var(--jc-bg-surface);
}
.jc-cat__child {
  display: flex;
  align-items: center;
  gap: var(--jc-space-2);
  min-height: 48px;
  padding: var(--jc-space-2) var(--jc-space-3) var(--jc-space-2) var(--jc-space-5);
  border-bottom: 1px solid var(--jc-border-subtle);
  cursor: pointer;
  transition: background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-cat__child:hover {
  background-color: rgba(255, 255, 255, 0.04);
}
.jc-cat__dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: var(--jc-radius-full);
  background-color: var(--jc-text-muted);
}
.jc-cat__dot--on {
  background-color: var(--jc-success);
}
.jc-cat__cname {
  /* 不撑满行: 名称后紧跟 #id·N部 meta(用户要求), 只在名称过长时收缩截断 */
  flex: 0 1 auto;
  min-width: 0;
  color: var(--jc-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.jc-cat__cid {
  flex-shrink: 0;
  margin-left: var(--jc-space-2);
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
  font-family: var(--jc-font-mono);
}
.jc-cat--off {
  opacity: 0.5;
  text-decoration: line-through;
}

/* ---- 控制区 ---- */
.jc-cat__cctrl {
  display: flex;
  align-items: center;
  gap: var(--jc-space-2);
  /* cname 不再撑满行, 控制区自己推到行尾 */
  margin-left: auto;
  flex-shrink: 0;
}
.jc-cat__toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--jc-space-2);
  cursor: pointer;
}
.jc-cat__state {
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-muted);
  min-width: 2em;
  text-align: right;
  user-select: none;
}
.jc-cat__state--sm {
  font-size: var(--jc-fs-xs);
}
.jc-cat__state--on {
  color: var(--jc-brand-purple);
  font-weight: var(--jc-fw-medium);
}
/* 影片可见开关的文字色(区别于菜单开关的紫色): 可见 = 绿, 隐藏 = 警示红 */
.jc-cat__state--films {
  color: var(--jc-success);
}
.jc-cat__state--films-off {
  color: var(--jc-danger);
  font-weight: var(--jc-fw-medium);
}
/* 分类名旁的影片数在"影片可见关闭"时的提示色 */
.jc-cat__films-hidden {
  color: var(--jc-danger);
}
.jc-cat__del {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: var(--jc-radius-md);
  color: var(--jc-text-muted);
  transition:
    color var(--jc-dur-fast) var(--jc-ease-standard),
    background-color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-cat__del:hover {
  color: var(--jc-danger);
  background-color: rgba(255, 255, 255, 0.06);
}
.jc-cat__del:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring);
}

/* ---- 新增子分类 ---- */
.jc-cat__add {
  display: inline-flex;
  align-items: center;
  gap: var(--jc-space-1);
  width: 100%;
  min-height: 44px;
  padding: 0 var(--jc-space-3) 0 var(--jc-space-5);
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-muted);
  text-align: left;
  transition: color var(--jc-dur-fast) var(--jc-ease-standard);
}
.jc-cat__add:hover {
  color: var(--jc-brand-purple);
}
.jc-cat__add:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring);
}

/* ---- 移动端: 隐藏次要信息 + 保留开关文案(换行排布) + 放大触控 ---- */
@media (max-width: 640px) {
  .jc-cat__meta,
  .jc-cat__cid {
    display: none;
  }
  .jc-cat__parent,
  .jc-cat__child {
    min-height: 52px;
    flex-wrap: wrap;
    row-gap: var(--jc-space-2);
  }
  /* 开关文案在移动端保留: 此前 display:none 导致只剩裸开关, 用户不知道开的是「影片可见/侧栏/顶栏」哪个 */
  .jc-cat__state {
    font-size: var(--jc-fs-xs);
    min-width: 0;
    text-align: left;
  }
  /* 父级: 名称独占首行, 三开关 + 删除落到次行 */
  .jc-cat__parent .jc-cat__title {
    flex: 1 1 100%;
  }
  .jc-cat__parent .jc-cat__cctrl {
    margin-left: 0;
  }
  /* 子级: 圆点 + 名称留首行, 控制簇独占次行 */
  .jc-cat__child .jc-cat__cctrl {
    margin-left: 0;
    flex: 1 1 100%;
    flex-wrap: wrap;
    row-gap: var(--jc-space-2);
  }
}
</style>
