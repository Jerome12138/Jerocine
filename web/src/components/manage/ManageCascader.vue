<!--
  分类级联选择器 (管后台用)。
  - 展示一棵分类树, 支持多选, 支持部分选中(indeterminate)
  - 叶子节点点击只切换自身; 非叶子节点点击切换整棵子树
  - 不直接应用筛选, 由父组件调用 apply() 后自行处理
-->
<script setup lang="ts">
import type { FilmClass } from '@/types/manage'

const MAX_DEPTH = 4

interface Props {
  nodes: FilmClass[]
  selected: Set<number>
  depth?: number
}
interface Emits {
  (e: 'change', ids: Set<number>): void
}

const props = withDefaults(defineProps<Props>(), { depth: 0 })
const emit = defineEmits<Emits>()

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

/** 仅子孙中有选中且自身未选中 → indeterminate */
function isPartialChecked(node: FilmClass): boolean {
  if (!node.children?.length) return false
  return node.children.some((c) => props.selected.has(c.id)) && !isChecked(node)
}

/**
 * 点击节点:
 * - 叶子: 只切换自身 (子 → 父不影响, 父 → 子不影响)
 * - 非叶子: 切换整棵子树 (整棵全选或全取消)
 */
function onToggle(node: FilmClass): void {
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
</script>

<template>
  <ul class="list-none m-0 p-0">
    <li v-for="node in nodes" :key="node.id" class="border-b border-subtle/50 last:border-0">
      <label
        class="flex items-center gap-2 py-1.5 px-2 hover:bg-[rgba(255,255,255,0.04)] rounded cursor-pointer text-sm select-none"
      >
        <input
          type="checkbox"
          class="accent-brand-purple"
          :checked="isChecked(node)"
          :indeterminate="isPartialChecked(node)"
          @change="onToggle(node)"
        />
        <span
          class="flex-1 min-w-0 truncate"
          :class="node.children?.length ? 'font-medium' : 'text-secondary'"
          :title="node.name"
        >{{ node.name }}</span>
      </label>
      <!-- 递归子孙: 最多 MAX_DEPTH 层, 防极端长链爆 DOM -->
      <ManageCascader
        v-if="node.children?.length && depth < MAX_DEPTH"
        :nodes="node.children"
        :selected="selected"
        :depth="depth + 1"
        @change="emit('change', $event)"
      />
    </li>
  </ul>
</template>
