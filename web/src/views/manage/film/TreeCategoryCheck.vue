<!--
  分类树形复选框(递归组件)。
  独立文件才能自我递归引用; 同文件内 script+template 不能自引用。
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
  (e: 'toggle', node: FilmClass): void
}

const props = withDefaults(defineProps<Props>(), { depth: 0 })
const emit = defineEmits<Emits>()

function isChecked(node: FilmClass): boolean {
  return props.selected.has(node.id)
}

function isPartialChecked(node: FilmClass): boolean {
  if (!node.children?.length) return false
  return node.children.some((c) => props.selected.has(c.id)) && !isChecked(node)
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
          @change="emit('toggle', node)"
        />
        <span
          class="flex-1 min-w-0 truncate"
          :class="node.children?.length ? 'font-medium' : 'text-secondary'"
          :title="node.name"
        >{{ node.name }}</span>
      </label>
      <TreeCategoryCheck
        v-if="node.children?.length && depth < MAX_DEPTH"
        :nodes="node.children"
        :selected="selected"
        :depth="depth + 1"
        @toggle="emit('toggle', node)"
      />
    </li>
  </ul>
</template>
