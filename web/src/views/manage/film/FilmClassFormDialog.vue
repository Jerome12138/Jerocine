<script setup lang="ts">
import { reactive } from 'vue'
import { manageApi } from '@/api'
import { toast } from '@/api/http'
import type { FilmClass } from '@/types/manage'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSwitch from '@/components/manage/ManageSwitch.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'

/**
 * 分类新增/编辑表单 —— openManageDialog 内容组件。
 * 契约: emits ('confirm', payload) | ('cancel'), footer 自带。
 *
 * 含独立校验与提交: 空名前端拦截 toast、提交失败弹窗停留(输入不丢);
 * 提交成功后 emit('confirm', 载荷), 由发起页负责重拉分类树。
 */
interface Props {
  /** 编辑目标; null/缺省 = 新增 */
  initial?: FilmClass | null
  /** 新增子分类时的目标父级 id(仅新增态生效; 编辑态以 initial.pid 为准) */
  pid?: number
}
const props = withDefaults(defineProps<Props>(), { initial: null, pid: 0 })

const emit = defineEmits<{
  (e: 'confirm', payload: FilmClass): void
  (e: 'cancel'): void
}>()

const form = reactive<FilmClass>(
  props.initial
    ? { ...props.initial }
    : { id: 0, pid: props.pid, name: '', show: true, sidebarShow: true, filmVisible: true, sort: 0 }
)

const { busy, run } = useAsyncAction()

function cancel(): void {
  emit('cancel')
}

function submit(): void {
  const name = form.name.trim()
  if (!name) {
    toast('error', '请填写分类名称')
    return
  }
  void run({
    action: () =>
      manageApi.film.classUpdate(
        {
          ...form,
          name,
          // ManageInput number 清空后是 undefined, 原样上送服务端 int 绑定会 422 invalid body → 归一 0
          sort: Number(form.sort) || 0
        },
        { silent: true }
      ),
    successMsg: '已保存',
    after: () => emit('confirm', { ...form, name, sort: Number(form.sort) || 0 })
  })
}
</script>

<template>
  <form class="flex flex-col gap-[var(--jc-space-4)]" @submit.prevent="submit">
    <ManageFormField label="名称" required>
      <ManageInput v-model="form.name" placeholder="例如：动作片" />
    </ManageFormField>
    <ManageFormField label="排序" hint="数字越小越靠前">
      <ManageInput v-model="form.sort" type="number" />
    </ManageFormField>
    <ManageFormField
      v-if="form.pid === 0"
      label="顶栏显示"
      hint="控制 web/TV 顶部一级导航是否显示该分类; 隐藏后影片仍可搜索、在列表看到"
    >
      <!-- filmVisible=false 时置灰: 服务端会强制级联置 false, 表单与行内开关口径一致 -->
      <ManageSwitch v-model="form.show" :disabled="!form.filmVisible" />
    </ManageFormField>
    <ManageFormField label="侧边栏显示" hint="控制桌面常驻侧栏 + 移动抽屉中该分类的显隐（一级+二级各自独立）">
      <ManageSwitch v-model="form.sidebarShow" :disabled="!form.filmVisible" />
    </ManageFormField>
    <ManageFormField
      label="影片可见"
      hint="关闭 = 该分类(含子分类)影片从公开列表与搜索全部剔除；详情直链仍可达; 关闭后侧边栏/顶栏开关会被强制禁用"
    >
      <ManageSwitch v-model="form.filmVisible" />
    </ManageFormField>

    <div class="flex justify-end gap-[var(--jc-space-3)] pt-[var(--jc-space-2)]">
      <ManageButton variant="ghost" type="button" @click="cancel">取消</ManageButton>
      <ManageButton variant="gradient" type="submit" :loading="busy">保存</ManageButton>
    </div>
  </form>
</template>
