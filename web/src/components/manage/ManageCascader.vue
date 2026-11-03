<!--
  分类级联选择器（管理后台用）—— Element Plus 风格「多列逐级展开」面板。
  - 点击带子级的节点 → 右侧展开下一列；叶子节点点击即勾选
  - 多选：每行带复选框；非叶子勾选 = 整棵子树全选，叶子勾选 = 自身
  - 半选(indeterminate) 递归子孙判定
  - 不直接应用筛选，由父组件决定何时生效（same as before）
-->
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FilmClass } from '@/types/manage'

/** 最多展开层数(防止极端长链把列撑爆): 允许 5 列(层级 0..4) */
const MAX_DEPTH = 4

interface Props {
  nodes: FilmClass[]
  selected: Set<number>
}
interface Emits {
  (e: 'change', ids: Set<number>): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

/** 展开路径: 每层被点开(用于决定渲染哪些列)的节点 id */
const activePath = ref<number[]>([])

/** 返回节点的全部子孙 id(含自身) */
function descendantIds(node: FilmClass): number[] {
  const ids = [node.id]
  if (node.children?.length) {
    for (const child of node.children) ids.push(...descendantIds(child))
  }
  return ids
}

function isChecked(node: FilmClass): boolean {
  return props.selected.has(node.id)
}

/** 子孙(递归全部层)中有选中且自身未选中 → indeterminate */
function isPartialChecked(node: FilmClass): boolean {
  if (!node.children?.length) return false
  return (
    descendantIds(node).some((id) => id !== node.id && props.selected.has(id)) &&
    !isChecked(node)
  )
}

/** 点击节点: 有子级 → 展开下一列; 叶子 → 切换自身勾选 */
function onNodeClick(node: FilmClass, level: number): void {
  if (node.children?.length) {
    if (level < MAX_DEPTH) activePath.value = [...activePath.value.slice(0, level), node.id]
  } else {
    toggle(node)
  }
}

/** 勾选: 非叶子切整棵子树, 叶子切自身 */
function onCheck(node: FilmClass): void {
  toggle(node)
}

function toggle(node: FilmClass): void {
  const ids = descendantIds(node)
  const allChecked = ids.every((id) => props.selected.has(id))
  const next = new Set(props.selected)
  if (allChecked) {
    for (const id of ids) next.delete(id)
  } else {
    for (const id of ids) next.add(id)
  }
  emit('change', next)
}

/** 逐级展开得到的列(每列 = 某一层的节点列表) */
const columns = computed<FilmClass[][]>(() => {
  const cols: FilmClass[][] = [props.nodes]
  let list = props.nodes
  for (const id of activePath.value) {
    const node = list.find((n) => n.id === id)
    if (!node?.children?.length) break
    list = node.children
    cols.push(list)
  }
  return cols
})

/** 当前展开路径末位节点 id(用于高亮) */
const activeId = computed(() => activePath.value[activePath.value.length - 1] ?? null)
</script>

<template>
  <div class="jc-cas flex items-stretch">
    <div
      v-for="(col, level) in columns"
      :key="level"
      class="jc-cas__col"
      :class="{ 'jc-cas__col--last': level === columns.length - 1 }"
    >
      <div
        v-for="node in col"
        :key="node.id"
        class="jc-cas__row"
        :class="{ 'jc-cas__row--active': activeId === node.id }"
        role="button"
        tabindex="0"
        @click="onNodeClick(node, level)"
        @keydown.enter="onNodeClick(node, level)"
      >
        <input
          type="checkbox"
          class="jc-cas__check"
          :checked="isChecked(node)"
          :indeterminate="isPartialChecked(node)"
          @click.stop
          @change="onCheck(node)"
        />
        <span
          class="jc-cas__name"
          :class="{ 'jc-cas__name--parent': node.children?.length }"
          :title="node.name"
        >{{ node.name }}</span>
        <span v-if="node.children?.length" class="jc-cas__arrow" aria-hidden="true">›</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.jc-cas {
  max-width: 100%;
  overflow-x: auto;
}

.jc-cas__col {
  min-width: 150px;
  max-height: 320px;
  overflow-y: auto;
  padding: 2px 0;
  border-right: 1px solid var(--jc-border-subtle);
}

.jc-cas__col--last {
  border-right: none;
}

.jc-cas__row {
  display: flex;
  align-items: center;
  gap: var(--jc-space-2);
  padding: var(--jc-space-1) var(--jc-space-2);
  border-radius: var(--jc-radius-sm);
  cursor: pointer;
  font-size: var(--jc-fs-sm);
}

.jc-cas__row:hover {
  background-color: rgba(255, 255, 255, 0.04);
}

.jc-cas__row:focus-visible {
  outline: none;
  box-shadow: var(--jc-shadow-focus-ring);
}

.jc-cas__row--active {
  background-color: rgba(124, 77, 255, 0.16);
}

.jc-cas__check {
  flex-shrink: 0;
  accent-color: var(--jc-brand-purple);
}

.jc-cas__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--jc-text-primary);
}

.jc-cas__name--parent {
  font-weight: var(--jc-fw-medium);
}

.jc-cas__arrow {
  flex-shrink: 0;
  color: var(--jc-text-muted);
}
</style>