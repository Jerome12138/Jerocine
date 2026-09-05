<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { manageApi } from '@/api'
import type { FilmClass } from '@/types/manage'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import BaseButton from '@/components/base/BaseButton.vue'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import BaseEmpty from '@/components/base/BaseEmpty.vue'
import BaseSkeleton from '@/components/base/BaseSkeleton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import { confirm } from '@/composables/useConfirm'

const tree = ref<FilmClass[]>([])
const loading = ref(true)
const dialogOpen = ref(false)
const submitting = ref(false)
const editing = ref<FilmClass | null>(null)
const form = reactive<FilmClass>({
  id: 0,
  pid: 0,
  name: '',
  ename: '',
  show: true,
  sort: 0
})

async function load(): Promise<void> {
  loading.value = true
  try {
    tree.value = await manageApi.film.classTree()
  } finally {
    loading.value = false
  }
}

// 新增时记录目标父级名(空 = 新增顶级)
const addingUnder = ref<string>('')

function openEdit(node: FilmClass): void {
  editing.value = node
  addingUnder.value = ''
  Object.assign(form, { ...node })
  dialogOpen.value = true
}

function openAddTop(): void {
  editing.value = null
  addingUnder.value = ''
  Object.assign(form, { id: 0, pid: 0, name: '', ename: '', show: true, sort: 0 })
  dialogOpen.value = true
}

function openAddChild(parent: FilmClass): void {
  editing.value = null
  addingUnder.value = parent.name
  Object.assign(form, { id: 0, pid: parent.id, name: '', ename: '', show: true, sort: 0 })
  dialogOpen.value = true
}

async function toggleShow(node: FilmClass): Promise<void> {
  const next = !node.show
  const tasks = [manageApi.film.classUpdate({ ...node, show: next })]
  // 关闭父级 → 级联关闭其全部已展示的子级 (子级独立, 不会自动恢复)
  if (!next && node.children?.length) {
    for (const child of node.children) {
      if (child.show) tasks.push(manageApi.film.classUpdate({ ...child, show: false }))
    }
  }
  await Promise.all(tasks)
  await load()
}

async function submit(): Promise<void> {
  submitting.value = true
  try {
    await manageApi.film.classUpdate({ ...form })
    dialogOpen.value = false
    await load()
  } finally {
    submitting.value = false
  }
}

async function remove(node: FilmClass): Promise<void> {
  const ok = await confirm({
    title: '确认删除分类？',
    desc: `「${node.name}」及其全部子分类将被永久删除`,
    okText: '删除',
    danger: true
  })
  if (!ok) return
  await manageApi.film.classDel(node.id)
  await load()
}

onMounted(load)
</script>

<template>
  <section class="bg-surface rounded-card shadow-card p-[var(--gf-space-4)] md:p-[var(--gf-space-5)]">
    <header class="flex items-center justify-between gap-[var(--gf-space-3)] mb-[var(--gf-space-4)]">
      <div class="min-w-0">
        <h2 class="text-lg font-[var(--gf-fw-semibold)]">影视分类管理</h2>
        <p class="text-sm text-muted truncate">维护顶级与子分类的展示状态、名称与排序</p>
      </div>
      <div class="flex items-center gap-[var(--gf-space-2)] shrink-0">
        <BaseButton variant="ghost" size="sm" @click="load">
          <BaseIcon name="refresh" size="16px" />
          <span class="hidden sm:inline">刷新</span>
        </BaseButton>
        <BaseButton variant="gradient" size="sm" @click="openAddTop">
          <BaseIcon name="plus" size="16px" /> 新增分类
        </BaseButton>
      </div>
    </header>

    <div v-if="loading" class="flex flex-col gap-[var(--gf-space-3)]">
      <BaseSkeleton v-for="i in 4" :key="i" shape="rect" height="64px" />
    </div>

    <BaseEmpty v-else-if="!tree.length" description="暂无分类数据" />

    <div v-else class="flex flex-col gap-[var(--gf-space-3)]">
      <article v-for="parent in tree" :key="parent.id" class="gf-cat">
        <!-- 父级 -->
        <div
          class="gf-cat__parent"
          data-focusable="true"
          tabindex="0"
          role="button"
          :aria-label="`编辑分类 ${parent.name}`"
          @click="openEdit(parent)"
          @keydown.enter="openEdit(parent)"
        >
          <span class="gf-cat__bar" aria-hidden="true" />
          <div class="gf-cat__title">
            <span class="gf-cat__name" :class="{ 'gf-cat--off': !parent.show }">{{ parent.name }}</span>
            <span class="gf-cat__meta">#{{ parent.id }} · {{ parent.children?.length ?? 0 }} 子分类</span>
          </div>
          <!-- 展示开关: 带文字标签, 单独可点不触发整行编辑 -->
          <label class="gf-cat__toggle" @click.stop>
            <span class="gf-cat__state" :class="{ 'gf-cat__state--on': parent.show }">
              {{ parent.show ? '展示' : '隐藏' }}
            </span>
            <ManageSwitch :model-value="parent.show" @update:model-value="toggleShow(parent)" />
          </label>
        </div>

        <!-- 子级 (始终渲染, 末尾保留新增子分类入口) -->
        <ul class="gf-cat__children">
          <li
            v-for="child in parent.children"
            :key="child.id"
            class="gf-cat__child"
            data-focusable="true"
            tabindex="0"
            role="button"
            :aria-label="`编辑子分类 ${child.name}`"
            @click="openEdit(child)"
            @keydown.enter="openEdit(child)"
          >
            <span class="gf-cat__dot" :class="{ 'gf-cat__dot--on': child.show }" aria-hidden="true" />
            <span class="gf-cat__cname" :class="{ 'gf-cat--off': !child.show }">{{ child.name }}</span>
            <span class="gf-cat__cid">#{{ child.id }}</span>
            <div class="gf-cat__cctrl" @click.stop>
              <label class="gf-cat__toggle">
                <span class="gf-cat__state gf-cat__state--sm" :class="{ 'gf-cat__state--on': child.show }">
                  {{ child.show ? '展示' : '隐藏' }}
                </span>
                <ManageSwitch :model-value="child.show" @update:model-value="toggleShow(child)" />
              </label>
              <button
                type="button"
                class="gf-cat__del"
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
              class="gf-cat__add"
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

  <ManageSheet
    v-model="dialogOpen"
    :title="editing ? '编辑分类' : addingUnder ? `在「${addingUnder}」下新增子分类` : '新增顶级分类'"
    mobile-mode="sheet"
  >
    <div class="flex flex-col gap-[var(--gf-space-4)]">
      <ManageFormField label="名称" required>
        <ManageInput v-model="form.name" placeholder="例如：动作片" />
      </ManageFormField>
      <ManageFormField label="英文名">
        <ManageInput v-model="form.ename!" placeholder="可选" />
      </ManageFormField>
      <ManageFormField label="排序" hint="数字越小越靠前">
        <ManageInput v-model="form.sort!" type="number" />
      </ManageFormField>
      <ManageFormField label="展示">
        <ManageSwitch v-model="form.show" />
      </ManageFormField>
    </div>
    <template #footer>
      <BaseButton variant="ghost" @click="dialogOpen = false">取消</BaseButton>
      <BaseButton variant="gradient" :loading="submitting" @click="submit">保存</BaseButton>
    </template>
  </ManageSheet>
</template>

<style scoped>
.gf-cat {
  border: 1px solid var(--gf-border-default);
  border-radius: var(--gf-radius-lg);
  background-color: var(--gf-bg-elevated);
  overflow: hidden;
}

/* ---- 父级 ---- */
.gf-cat__parent {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--gf-space-3);
  min-height: 56px;
  padding: var(--gf-space-3) var(--gf-space-4) var(--gf-space-3) var(--gf-space-5);
  cursor: pointer;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-cat__parent:hover {
  background-color: rgba(255, 255, 255, 0.04);
}
.gf-cat__bar {
  position: absolute;
  left: 0;
  top: var(--gf-space-2);
  bottom: var(--gf-space-2);
  width: 3px;
  border-radius: 0 3px 3px 0;
  background-image: var(--gf-brand-gradient);
}
.gf-cat__title {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}
.gf-cat__name {
  font-weight: var(--gf-fw-semibold);
  color: var(--gf-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gf-cat__meta {
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  font-family: var(--gf-font-mono);
}

/* ---- 子级 ---- */
.gf-cat__children {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--gf-border-subtle);
  background-color: var(--gf-bg-surface);
}
.gf-cat__child {
  display: flex;
  align-items: center;
  gap: var(--gf-space-2);
  min-height: 48px;
  padding: var(--gf-space-2) var(--gf-space-3) var(--gf-space-2) var(--gf-space-5);
  border-bottom: 1px solid var(--gf-border-subtle);
  cursor: pointer;
  transition: background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-cat__child:hover {
  background-color: rgba(255, 255, 255, 0.04);
}
.gf-cat__dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: var(--gf-radius-full);
  background-color: var(--gf-text-muted);
}
.gf-cat__dot--on {
  background-color: var(--gf-success);
}
.gf-cat__cname {
  min-width: 0;
  flex: 1;
  color: var(--gf-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.gf-cat__cid {
  flex-shrink: 0;
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
  font-family: var(--gf-font-mono);
}
.gf-cat--off {
  opacity: 0.5;
  text-decoration: line-through;
}

/* ---- 控制区 ---- */
.gf-cat__cctrl {
  display: flex;
  align-items: center;
  gap: var(--gf-space-2);
  flex-shrink: 0;
}
.gf-cat__toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--gf-space-2);
  cursor: pointer;
}
.gf-cat__state {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
  min-width: 2em;
  text-align: right;
  user-select: none;
}
.gf-cat__state--sm {
  font-size: var(--gf-fs-xs);
}
.gf-cat__state--on {
  color: var(--gf-brand-purple);
  font-weight: var(--gf-fw-medium);
}
.gf-cat__del {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: var(--gf-radius-md);
  color: var(--gf-text-muted);
  transition:
    color var(--gf-dur-fast) var(--gf-ease-standard),
    background-color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-cat__del:hover {
  color: var(--gf-danger);
  background-color: rgba(255, 255, 255, 0.06);
}
.gf-cat__del:focus-visible {
  outline: none;
  box-shadow: var(--gf-shadow-focus-ring);
}

/* ---- 新增子分类 ---- */
.gf-cat__add {
  display: inline-flex;
  align-items: center;
  gap: var(--gf-space-1);
  width: 100%;
  min-height: 44px;
  padding: 0 var(--gf-space-3) 0 var(--gf-space-5);
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
  text-align: left;
  transition: color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-cat__add:hover {
  color: var(--gf-brand-purple);
}
.gf-cat__add:focus-visible {
  outline: none;
  box-shadow: var(--gf-shadow-focus-ring);
}

/* ---- 移动端: 隐藏次要信息 + 放大触控 ---- */
@media (max-width: 640px) {
  .gf-cat__meta,
  .gf-cat__cid {
    display: none;
  }
  .gf-cat__state {
    display: none;
  }
  .gf-cat__parent,
  .gf-cat__child {
    min-height: 52px;
  }
}
</style>
