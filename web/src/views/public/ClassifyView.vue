<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'
import * as filmApi from '@/api/film'
import type { Card, ClassifyData } from '@/types/film'
import FilmRow from '@/components/film/FilmRow.vue'
import FilmCard from '@/components/film/FilmCard.vue'
import { useQuerySync } from '@/composables/useQuerySync'
import { useAbortable } from '@/composables/useAbortable'
import { useNavStore } from '@/stores'
import { useViewMode } from '@/composables/useViewMode'

/**
 * /filmClassify?Pid=xxx
 * STORY-012 分类首页：最新上映 / 排行榜 / 最近更新
 */

const { params } = useQuerySync<{ Pid: string }>(
  { Pid: '' },
  {
    path: '/filmClassify',
    onChange: () => {
      void load()
    }
  }
)

const data = ref<ClassifyData>({
  title: undefined,
  news: [],
  top: [],
  recent: []
})
const loading = ref<boolean>(true)
const loaded = ref<boolean>(false)
const errorMsg = ref<string>('')

const abortable = useAbortable()

// 顶级分类切换 (从 nav store 取全部一级分类, 移动端也能切, 不再只有电影)
const navStore = useNavStore()
const { list: navCats } = storeToRefs(navStore)
const currentPid = computed(() => (params.value.Pid || '').trim())

// 非移动端 (desktop/tv) 顶部 Header 已显示分类导航, 页内胶囊冗余 → 仅移动端显示
const { isMobile, isTV } = useViewMode()

async function load(): Promise<void> {
  const pid = (params.value.Pid || '').trim()
  if (!pid) {
    errorMsg.value = '缺少分类参数 Pid'
    loading.value = false
    loaded.value = true
    return
  }
  loading.value = true
  errorMsg.value = ''
  abortable.refresh()
  try {
    const resp = await filmApi.getClassify(pid)
    data.value = resp
    loaded.value = true
  } catch (e) {
    if ((e as { name?: string })?.name === 'CanceledError') return
    errorMsg.value = '加载失败，请稍后重试'
    loaded.value = true
  } finally {
    loading.value = false
  }
}

void load()

function moreLink(sort: string): {
  path: string
  query: Record<string, string>
} {
  return {
    path: '/filmClassifySearch',
    query: { Pid: String(data.value.title?.id || params.value.Pid), Sort: sort }
  }
}

function isReady(items: Card[] | undefined): boolean {
  return Array.isArray(items) && items.length > 0
}

// ─── TV 专用 ──────────────────────────────────────────────
// (顶部胶囊导航已含一级分类, 分类页不再重复 chip bar)

// 标题区目标分类 id (title.id 优先, 否则取 URL Pid)
const tvTitlePid = computed(() =>
  String(data.value.title?.id || params.value.Pid)
)

// TV 三段网格的配置 (复用现有 news/top/recent 数据与 moreLink)
const tvSections = computed(() => [
  {
    key: 'news',
    title: '最新上映',
    sub: '每日更新',
    items: data.value.news,
    sort: 'release_stamp'
  },
  {
    key: 'top',
    title: '排行榜',
    sub: '按热度排序',
    items: data.value.top,
    sort: 'hits'
  },
  {
    key: 'recent',
    title: '最近更新',
    sub: '追更不迷路',
    items: data.value.recent,
    sort: 'update_stamp'
  }
])

const tvAllEmpty = computed(
  () =>
    !isReady(data.value.news) &&
    !isReady(data.value.top) &&
    !isReady(data.value.recent)
)
</script>

<template>
  <!-- ══════════════════ TV (雷鸟卡片式) 分支 ══════════════════ -->
  <div
    v-if="isTV"
    class="gf-classify-tv container-page py-[var(--gf-space-6)] flex flex-col gap-[var(--gf-space-8)]"
  >
    <!-- 顶部一级分类已由全局胶囊导航提供, 分类页不再重复 chip bar -->

    <!-- 标题区: 「分类」当前 + 「分类库」入口 (filmClassifySearch) -->
    <div
      v-if="data.title?.name"
      class="gf-classify-tv__title gf-tv-sec"
    >
      <span class="gf-classify-tv__title-grad t">{{ data.title?.name }}</span>
      <span class="gf-classify-tv__title-sep s" aria-hidden="true">|</span>
      <RouterLink
        v-slot="{ navigate, href }"
        :to="{ path: '/filmClassifySearch', query: { Pid: tvTitlePid } }"
        custom
      >
        <a
          class="gf-tv-chip gf-classify-tv__lib"
          :href="href"
          data-focusable="true"
          tabindex="0"
          @click="navigate"
          @keydown.enter="() => navigate()"
        >
          {{ data.title?.name }}库 ›
        </a>
      </RouterLink>
    </div>

    <!-- 错误态 -->
    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请检查链接中的 Pid 参数"
    />

    <!-- 骨架 -->
    <div
      v-else-if="loading && !loaded"
      class="flex flex-col gap-[var(--gf-space-8)]"
    >
      <div v-for="n in 3" :key="n" class="flex flex-col gap-[var(--gf-space-4)]">
        <BaseSkeleton width="240px" height="36px" />
        <div class="gf-tv-grid">
          <BaseSkeleton
            v-for="i in 6"
            :key="i"
            width="100%"
            height="280px"
            ratio="3/4"
          />
        </div>
      </div>
    </div>

    <!-- 分段网格 (最新上映 / 排行榜 / 最近更新) -->
    <template v-else>
      <section
        v-for="sec in tvSections"
        v-show="isReady(sec.items)"
        :key="sec.key"
        class="gf-classify-tv__section"
      >
        <div class="gf-tv-sec">
          <span class="t">{{ sec.title }}</span>
          <span class="s">{{ sec.sub }}</span>
          <RouterLink
            v-slot="{ navigate, href }"
            :to="moreLink(sec.sort)"
            custom
          >
            <a
              class="gf-tv-more"
              :href="href"
              data-focusable="true"
              tabindex="0"
              @click="navigate"
              @keydown.enter="() => navigate()"
            >
              更多 ›
            </a>
          </RouterLink>
        </div>
        <div class="gf-tv-grid">
          <FilmCard
            v-for="item in sec.items"
            :key="item.mid"
            :item="item"
          />
        </div>
      </section>

      <BaseEmpty
        v-if="tvAllEmpty"
        title="该分类暂无影片"
        description="切换到分类库浏览更多内容"
      >
        <template #action>
          <BaseButton
            v-if="data.title?.id"
            variant="gradient"
            size="md"
            @click="
              $router.push({
                path: '/filmClassifySearch',
                query: { Pid: String(data.title?.id) }
              })
            "
          >
            前往 {{ data.title?.name }}库
          </BaseButton>
        </template>
      </BaseEmpty>
    </template>
  </div>

  <!-- ══════════════════ 桌面 / 移动 原始分支 ══════════════════ -->
  <div v-else class="gf-classify container-page py-[var(--gf-space-6)]">
    <!-- 顶级分类切换 (电影 / 电视剧 / 综艺 / 动漫 …) — 仅移动端显示;
         非移动端顶部 Header 已有分类导航, 此处胶囊冗余隐藏 -->
    <nav
      v-if="navCats.length && isMobile"
      class="gf-classify__cats flex flex-wrap gap-[var(--gf-space-2)] mb-[var(--gf-space-6)]"
      aria-label="影视分类"
    >
      <RouterLink
        v-for="cat in navCats"
        :key="cat.id"
        :to="{ path: '/filmClassify', query: { Pid: String(cat.id) } }"
        class="gf-classify__cat"
        :class="String(cat.id) === currentPid ? 'gf-classify__cat--active' : ''"
        data-focusable="true"
        tabindex="0"
      >
        {{ cat.name }}
      </RouterLink>
    </nav>

    <!-- 顶部 title 切换 -->
    <header
      v-if="data.title?.name"
      class="gf-classify__title flex items-center gap-[var(--gf-space-3)] mb-[var(--gf-space-8)]"
    >
      <RouterLink
        :to="{ path: '/filmClassify', query: { Pid: String(data.title?.id) } }"
        class="gf-classify__title-active"
        data-focusable="true"
        tabindex="0"
      >
        {{ data.title?.name }}
      </RouterLink>
      <span class="gf-classify__title-divider" aria-hidden="true">|</span>
      <RouterLink
        :to="{ path: '/filmClassifySearch', query: { Pid: String(data.title?.id) } }"
        class="gf-classify__title-link"
        data-focusable="true"
        tabindex="0"
      >
        {{ data.title?.name }}库
      </RouterLink>
    </header>

    <!-- 错误态 -->
    <BaseEmpty
      v-if="!loading && errorMsg"
      :title="errorMsg"
      description="请检查链接中的 Pid 参数"
    />

    <!-- 骨架 -->
    <div
      v-else-if="loading && !loaded"
      class="flex flex-col gap-[var(--gf-space-8)]"
    >
      <div v-for="n in 3" :key="n" class="flex flex-col gap-[var(--gf-space-3)]">
        <BaseSkeleton width="220px" height="32px" />
        <div class="flex gap-[var(--gf-space-3)] overflow-hidden">
          <BaseSkeleton
            v-for="i in 6"
            :key="i"
            width="160px"
            height="240px"
            ratio="3/4"
          />
        </div>
      </div>
    </div>

    <!-- 三个 Row -->
    <div
      v-else
      class="gf-classify__rows flex flex-col gap-[var(--gf-space-10)]"
    >
      <FilmRow
        v-if="isReady(data.news)"
        title="最新上映"
        :more-link="moreLink('release_stamp')"
        :items="data.news"
      />
      <FilmRow
        v-if="isReady(data.top)"
        title="排行榜"
        :more-link="moreLink('hits')"
        :items="data.top"
      />
      <FilmRow
        v-if="isReady(data.recent)"
        title="最近更新"
        :more-link="moreLink('update_stamp')"
        :items="data.recent"
      />

      <BaseEmpty
        v-if="
          !isReady(data.news) &&
          !isReady(data.top) &&
          !isReady(data.recent)
        "
        title="该分类暂无影片"
        description="切换到分类库浏览更多内容"
      >
        <template #action>
          <BaseButton
            v-if="data.title?.id"
            variant="gradient"
            size="md"
            @click="
              $router.push({
                path: '/filmClassifySearch',
                query: { Pid: String(data.title?.id) }
              })
            "
          >
            前往 {{ data.title?.name }}库
          </BaseButton>
        </template>
      </BaseEmpty>
    </div>
  </div>
</template>

<style scoped>
.gf-classify__cat {
  display: inline-flex;
  align-items: center;
  min-height: 36px;
  padding: 0 var(--gf-space-4);
  border-radius: var(--gf-radius-full);
  background-color: var(--gf-bg-elevated);
  color: var(--gf-text-secondary);
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-medium);
  text-decoration: none;
  transition:
    background-color var(--gf-dur-fast) var(--gf-ease-standard),
    color var(--gf-dur-fast) var(--gf-ease-standard);
}
.gf-classify__cat:hover {
  color: var(--gf-text-primary);
  background-color: rgba(255, 255, 255, 0.08);
}
.gf-classify__cat--active {
  background-image: var(--gf-brand-gradient);
  color: #fff;
  font-weight: var(--gf-fw-semibold);
}
.gf-classify__cat:focus,
.gf-classify__cat:focus-visible {
  outline: none;
  box-shadow: var(--gf-shadow-focus-ring);
}

.gf-classify__title {
  flex-wrap: wrap;
}
.gf-classify__title-active,
.gf-classify__title-link {
  text-decoration: none;
  font-size: var(--gf-fs-2xl);
  font-weight: var(--gf-fw-bold);
  outline: none;
  transition: opacity var(--gf-dur-fast) var(--gf-ease-standard);
  border-radius: var(--gf-radius-sm);
}

.gf-classify__title-active {
  background-image: var(--gf-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}

.gf-classify__title-link {
  color: var(--gf-text-secondary);
}
.gf-classify__title-link:hover,
.gf-classify__title-link:focus-visible {
  color: var(--gf-text-primary);
}
.gf-classify__title-active:focus-visible,
.gf-classify__title-link:focus-visible {
  box-shadow: var(--gf-shadow-focus-ring);
}

.gf-classify__title-divider {
  color: var(--gf-text-muted);
  font-size: var(--gf-fs-lg);
}

@media (max-width: 767px) {
  .gf-classify__title-active,
  .gf-classify__title-link {
    font-size: var(--gf-fs-xl);
  }
}

/* ─── TV 分支局部修饰 (chrome 主体在全局 tv-cards.css 的 gf-tv-*) ─── */
.gf-classify-tv__title {
  align-items: center;
}
.gf-classify-tv__title-grad {
  font-size: var(--gf-fs-2xl);
  font-weight: var(--gf-fw-bold);
  background-image: var(--gf-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
}
.gf-classify-tv__title-sep {
  font-size: var(--gf-fs-lg);
  color: var(--gf-text-muted);
  margin: 0 var(--gf-space-3);
}
.gf-classify-tv__lib {
  height: 36px;
  font-size: var(--gf-fs-sm);
}
.gf-classify-tv__section {
  display: flex;
  flex-direction: column;
}

/* 分类页 TV: 影片网格固定每行 6 个 (覆盖全局 gf-tv-grid 的 auto-fill) */
[data-mode='tv'] .gf-tv-grid {
  grid-template-columns: repeat(6, minmax(0, 1fr));
}
</style>
