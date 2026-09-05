<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { filmApi } from '@/api'
import type { Card } from '@/types/film'
import BaseButton from '@/components/base/BaseButton.vue'
import BaseIcon from '@/components/base/BaseIcon.vue'
import FilmCard from '@/components/film/FilmCard.vue'

const router = useRouter()

/** 是否可以返回上一页（直接访问 404 时 history.length 为 1） */
const canGoBack = computed(() => {
  if (typeof window === 'undefined') return false
  return window.history.length > 1
})

/** "或许你想看?" 推荐影片列表（最多 4 张） */
const recommendList = ref<Card[]>([])
const recommendLoading = ref(false)

function goHome(): void {
  router.replace('/index')
}

function goBack(): void {
  if (canGoBack.value) {
    router.back()
  } else {
    router.replace('/index')
  }
}

function goSearch(): void {
  router.push('/search')
}

/**
 * 从首页接口取一批热门作为「或许你想看」推荐。
 * 优先用 banner（首屏精选），不足 4 张再补 content[].hot/movies，去重后截取前 4 张。
 * 失败静默：仅不展示推荐区块，不阻塞主操作。
 */
async function loadRecommend(): Promise<void> {
  recommendLoading.value = true
  try {
    const data = await filmApi.getHome()
    const pool: Card[] = []
    const seen = new Set<number>()
    const push = (items?: Card[]): void => {
      if (!items) return
      for (const it of items) {
        if (seen.has(it.mid) || !it.cover) continue
        seen.add(it.mid)
        pool.push(it)
        if (pool.length >= 4) break
      }
    }
    for (const row of data.rows ?? []) {
      push(row.hot)
      if (pool.length >= 4) break
      push(row.latest)
      if (pool.length >= 4) break
    }
    recommendList.value = pool.slice(0, 4)
  } catch {
    recommendList.value = []
  } finally {
    recommendLoading.value = false
  }
}

onMounted(() => {
  loadRecommend()
})
</script>

<template>
  <div class="nf-page">
    <!-- 装饰：两团径向渐变光斑 (紫 + 青) -->
    <div class="nf-bg-orbs" aria-hidden="true">
      <span class="nf-orb nf-orb--purple" />
      <span class="nf-orb nf-orb--cyan" />
    </div>

    <main class="nf-content">
      <!-- 大号 404 渐变数字 -->
      <p class="nf-code" aria-label="HTTP 404">
        404
      </p>

      <h1 class="nf-title">
        页面失踪了
      </h1>
      <p class="nf-desc">
        这里没有你想找的内容，它可能被吃掉了，或者从未存在过。<br class="nf-desc__br">不如换个入口继续探索？
      </p>

      <!-- 三按钮：返回首页 / 返回上一页（可选） / 去搜索 -->
      <div class="nf-actions">
        <BaseButton
          variant="gradient"
          size="lg"
          class="nf-btn"
          @click="goHome"
        >
          <template #icon>
            <BaseIcon name="home" size="20px" />
          </template>
          返回首页
        </BaseButton>

        <BaseButton
          v-if="canGoBack"
          variant="outline"
          size="lg"
          class="nf-btn"
          @click="goBack"
        >
          <template #icon>
            <BaseIcon name="arrow-left" size="20px" />
          </template>
          返回上一页
        </BaseButton>

        <BaseButton
          variant="outline"
          size="lg"
          class="nf-btn"
          @click="goSearch"
        >
          <template #icon>
            <BaseIcon name="search" size="20px" />
          </template>
          去搜索
        </BaseButton>
      </div>

      <!-- 或许你想看? -->
      <section
        v-if="recommendList.length > 0"
        class="nf-recommend"
        aria-label="或许你想看"
      >
        <header class="nf-recommend__header">
          <span class="nf-recommend__bar" aria-hidden="true" />
          <h2 class="nf-recommend__title">
            或许你想看?
          </h2>
        </header>
        <ul class="nf-recommend__grid">
          <li
            v-for="item in recommendList"
            :key="item.mid"
            class="nf-recommend__cell"
          >
            <FilmCard :item="item" />
          </li>
        </ul>
      </section>
    </main>
  </div>
</template>

<style scoped>
.nf-page {
  position: relative;
  min-height: 100vh;
  min-height: 100dvh;
  width: 100%;
  background-color: var(--gf-bg-base);
  color: var(--gf-text-primary);
  overflow: hidden;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: clamp(40px, 8vw, 96px) var(--gf-space-4)
    clamp(48px, 8vw, 96px);
}

/* ---------- 背景光斑 ---------- */
.nf-bg-orbs {
  position: absolute;
  inset: 0;
  pointer-events: none;
  z-index: 0;
  overflow: hidden;
}
.nf-orb {
  position: absolute;
  border-radius: 9999px;
  filter: blur(120px);
  opacity: 0.55;
  will-change: transform;
}
.nf-orb--purple {
  width: clamp(360px, 50vw, 720px);
  aspect-ratio: 1;
  background: radial-gradient(
    circle at 30% 30%,
    rgba(155, 73, 231, 0.55),
    rgba(155, 73, 231, 0) 70%
  );
  top: -10%;
  left: -10%;
  animation: nf-orb-float-a 14s ease-in-out infinite alternate;
}
.nf-orb--cyan {
  width: clamp(320px, 45vw, 640px);
  aspect-ratio: 1;
  background: radial-gradient(
    circle at 70% 70%,
    rgba(74, 209, 229, 0.5),
    rgba(74, 209, 229, 0) 70%
  );
  bottom: -15%;
  right: -10%;
  animation: nf-orb-float-b 18s ease-in-out infinite alternate;
}

@keyframes nf-orb-float-a {
  0% {
    transform: translate3d(0, 0, 0) scale(1);
  }
  100% {
    transform: translate3d(4%, 6%, 0) scale(1.08);
  }
}
@keyframes nf-orb-float-b {
  0% {
    transform: translate3d(0, 0, 0) scale(1);
  }
  100% {
    transform: translate3d(-5%, -4%, 0) scale(1.1);
  }
}

/* ---------- 主体 ---------- */
.nf-content {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 960px;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.nf-code {
  font-family: var(--gf-font-display);
  font-weight: var(--gf-fw-black);
  font-size: clamp(120px, 25vw, 320px);
  line-height: 0.92;
  letter-spacing: var(--gf-tracking-tight);
  margin: 0;
  background-image: var(--gf-brand-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  -webkit-text-fill-color: transparent;
  filter: drop-shadow(0 12px 40px rgba(155, 73, 231, 0.35));
  user-select: none;
}

.nf-title {
  margin: var(--gf-space-2) 0 0;
  font-size: clamp(1.5rem, 3vw, 2.25rem);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
  letter-spacing: var(--gf-tracking-tight);
  line-height: var(--gf-lh-tight);
}

.nf-desc {
  margin: var(--gf-space-4) 0 0;
  font-size: var(--gf-fs-md);
  color: var(--gf-text-secondary);
  line-height: var(--gf-lh-relaxed);
  max-width: 520px;
}

/* ---------- 按钮组 ---------- */
.nf-actions {
  margin-top: var(--gf-space-8);
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--gf-space-3);
}
.nf-btn {
  min-width: 160px;
}

/* ---------- 推荐区 ---------- */
.nf-recommend {
  margin-top: clamp(40px, 6vw, 80px);
  width: 100%;
  text-align: left;
}
.nf-recommend__header {
  display: flex;
  align-items: center;
  gap: var(--gf-space-3);
  margin-bottom: var(--gf-space-5);
}
.nf-recommend__bar {
  display: inline-block;
  width: 4px;
  height: 22px;
  border-radius: var(--gf-radius-sm);
  background-image: var(--gf-brand-gradient);
}
.nf-recommend__title {
  margin: 0;
  font-size: var(--gf-fs-xl);
  font-weight: var(--gf-fw-bold);
  color: var(--gf-text-primary);
  letter-spacing: var(--gf-tracking-tight);
}
.nf-recommend__grid {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--gf-card-gap);
}
.nf-recommend__cell {
  min-width: 0;
}

/* ---------- 响应式 ---------- */
@media (max-width: 480px) {
  .nf-desc__br {
    display: none;
  }
  .nf-actions {
    flex-direction: column;
    align-items: stretch;
    width: 100%;
    max-width: 320px;
  }
  .nf-btn {
    width: 100%;
  }
}

@media (min-width: 768px) {
  .nf-recommend__grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: clamp(16px, 2vw, 24px);
  }
}

/* 偏好减少动画：停止光斑漂浮 */
@media (prefers-reduced-motion: reduce) {
  .nf-orb {
    animation: none !important;
  }
}
</style>
