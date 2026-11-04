<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { manageApi } from '@/api'
import type { FilmClass } from '@/types/manage'
import ManageSheet from '@/components/manage/ManageSheet.vue'
import ManageFormField from '@/components/manage/ManageFormField.vue'
import ManageInput from '@/components/manage/ManageInput.vue'
import ManageSelect from '@/components/manage/ManageSelect.vue'
import ManageTextarea from '@/components/manage/ManageTextarea.vue'
import ManageImageUpload from '@/components/manage/ManageImageUpload.vue'
import ManageButton from '@/components/manage/ManageButton.vue'
import { useAsyncAction } from '@/composables/useAsyncAction'
import type { SelectOption } from '@/components/manage/ManageSelect.vue'

/**
 * 手动新增影片弹窗（原独立页 views/manage/film/FilmAddView.vue 的「手动新增影片」段收编而来）。
 *
 * 与「采集新片」拆成两个入口：本弹窗只做手工录入（补录用），采集走 FilmCollectDialog。
 * 校验沿用基线写法（字段级 error + ManageFormField error prop）；
 * 成功后 emit('saved') 由影片列表刷新并关闭。
 */
const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const { busy, run } = useAsyncAction()

// ============ 分类树（顶级分类下拉） ============
const tree = ref<FilmClass[]>([])
const treeError = ref('')

async function loadTree(): Promise<void> {
  treeError.value = ''
  try {
    tree.value = await manageApi.film.classTree()
  } catch (e) {
    treeError.value = (e as Error).message ?? '加载失败'
  }
}

// ============ 手动新增表单 ============
const form = reactive({
  name: '',
  enName: '',
  cid: 0,
  pid: 0,
  picture: '',
  year: '',
  area: '',
  director: '',
  actor: '',
  classTag: '',
  content: ''
})
/** 字段级错误(ManageFormField error prop 展示) */
const nameError = ref('')
const yearError = ref('')

/** 顶级分类下拉: 「请选择」= 0(顶级), 其余为分类树一级节点(数字 value 原类型回传) */
const pidOptions = computed<SelectOption[]>(() => [
  { label: '请选择', value: 0 },
  ...tree.value.map((t) => ({ label: t.name, value: t.id }))
])

/** 每次打开都重置表单与校验态（原独立页每次进入都是全新表单） */
function resetForm(): void {
  form.name = ''
  form.enName = ''
  form.cid = 0
  form.pid = 0
  form.picture = ''
  form.year = ''
  form.area = ''
  form.director = ''
  form.actor = ''
  form.classTag = ''
  form.content = ''
  nameError.value = ''
  yearError.value = ''
}

async function submit(): Promise<void> {
  nameError.value = ''
  yearError.value = ''
  const name = form.name.trim()
  if (!name) {
    nameError.value = '请填写影片名'
    return
  }
  const yearStr = String(form.year).trim()
  if (yearStr && !/^\d{4}$/.test(yearStr)) {
    yearError.value = '年份需为 4 位数字（如 2024），可留空'
    return
  }
  await run({
    action: () =>
      manageApi.film.add(
        {
          name,
          enName: form.enName,
          // ManageInput number 清空后是 undefined, 原样上送服务端 int64 绑定会 422 invalid body → 归一 0(与 year 同口径)
          cid: Number(form.cid) || 0,
          pid: form.pid,
          cover: form.picture,
          area: form.area,
          year: Number(form.year) || 0,
          director: form.director,
          actor: form.actor,
          classTag: form.classTag,
          content: form.content
        },
        { silent: true }
      ),
    after: () => {
      visible.value = false
      emit('saved')
    }
  })
}

watch(
  visible,
  (v) => {
    if (!v) return
    resetForm()
    void loadTree()
  },
  { immediate: true }
)
</script>

<template>
  <ManageSheet v-model="visible" title="手动新增影片" width="720px">
    <form
      class="grid grid-cols-1 md:grid-cols-2 gap-[var(--jc-space-5)]"
      @submit.prevent="submit"
    >
      <ManageFormField label="影片名" required :error="nameError">
        <ManageInput v-model="form.name" />
      </ManageFormField>
      <ManageFormField label="英文名">
        <ManageInput v-model="form.enName" />
      </ManageFormField>

      <ManageFormField label="顶级分类 (Pid)">
        <div class="flex flex-col gap-[var(--jc-space-2)]">
          <ManageSelect v-model="form.pid" :options="pidOptions" aria-label="顶级分类" />
          <div
            v-if="treeError"
            class="flex items-center gap-[var(--jc-space-2)] text-xs text-[var(--jc-danger)] flex-wrap"
          >
            <span>分类树加载失败：{{ treeError }}</span>
            <ManageButton variant="ghost" size="sm" @click="loadTree">重试</ManageButton>
          </div>
        </div>
      </ManageFormField>
      <ManageFormField label="子分类 (Cid)">
        <ManageInput v-model="form.cid" type="number" />
      </ManageFormField>

      <ManageFormField label="海报" class="md:col-span-2">
        <ManageImageUpload v-model="form.picture" ratio="2/3" placeholder="或粘贴图片 URL" />
      </ManageFormField>

      <ManageFormField label="年份" :error="yearError">
        <ManageInput v-model="form.year" placeholder="2024" />
      </ManageFormField>
      <ManageFormField label="地区">
        <ManageInput v-model="form.area" placeholder="中国大陆" />
      </ManageFormField>
      <ManageFormField label="导演">
        <ManageInput v-model="form.director" />
      </ManageFormField>
      <ManageFormField label="主演">
        <ManageInput v-model="form.actor" placeholder="逗号分隔" />
      </ManageFormField>
      <ManageFormField label="标签">
        <ManageInput v-model="form.classTag" placeholder="逗号分隔" />
      </ManageFormField>

      <ManageFormField label="剧情简介" class="md:col-span-2">
        <ManageTextarea v-model="form.content" :rows="6" />
      </ManageFormField>
    </form>

    <template #footer>
      <ManageButton variant="ghost" type="button" @click="visible = false">取消</ManageButton>
      <ManageButton variant="gradient" type="button" :loading="busy" @click="submit">保存</ManageButton>
    </template>
  </ManageSheet>
</template>
