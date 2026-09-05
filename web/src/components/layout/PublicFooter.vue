<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useSiteStore } from '@/stores'

const siteStore = useSiteStore()
const { basic } = storeToRefs(siteStore)

const siteName = computed(() => basic.value?.siteName || 'Jerocine')
const description = computed(() => basic.value?.description || '')
const year = new Date().getFullYear()
</script>

<template>
  <footer class="gf-footer">
    <div class="gf-footer__inner container-page">
      <!-- 桌面三列；移动单列居中 -->
      <div class="gf-footer__cols">
        <div class="gf-footer__col gf-footer__col--brand">
          <span class="gf-footer__brand text-brand-gradient">{{ siteName }}</span>
          <p v-if="description" class="gf-footer__desc">
            {{ description }}
          </p>
        </div>
        <div class="gf-footer__col">
          <h4 class="gf-footer__title">站点</h4>
          <ul class="gf-footer__list">
            <li>
              <RouterLink class="gf-footer__link" to="/index">首页</RouterLink>
            </li>
            <li>
              <RouterLink class="gf-footer__link" to="/search">搜索</RouterLink>
            </li>
            <li>
              <RouterLink class="gf-footer__link" to="/history">观看历史</RouterLink>
            </li>
            <li>
              <RouterLink class="gf-footer__link" to="/favorites">我的收藏</RouterLink>
            </li>
          </ul>
        </div>
        <div class="gf-footer__col">
          <h4 class="gf-footer__title">关于</h4>
          <p class="gf-footer__line">
            本站仅提供 web 页面服务，所有视频内容均来自互联网，与本站无关。
          </p>
        </div>
      </div>

      <!-- 底栏 -->
      <div class="gf-footer__bottom">
        <span>{{ siteName }} &copy; {{ year }}</span>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.gf-footer {
  margin-top: auto;
  background-color: #07070a;
  color: var(--gf-text-muted);
  border-top: 1px solid var(--gf-border-subtle);
}

.gf-footer__inner {
  padding-block: var(--gf-space-10) var(--gf-space-6);
}

.gf-footer__cols {
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-6);
  text-align: center;
  align-items: center;
}

.gf-footer__col {
  width: 100%;
  max-width: 360px;
}

.gf-footer__col--brand {
  display: flex;
  flex-direction: column;
  gap: var(--gf-space-2);
  align-items: center;
}

.gf-footer__brand {
  font-family: var(--gf-font-display);
  font-size: var(--gf-fs-xl);
  font-weight: var(--gf-fw-black);
  letter-spacing: var(--gf-tracking-tight);
}

.gf-footer__desc {
  font-size: var(--gf-fs-sm);
  line-height: var(--gf-lh-relaxed);
  color: var(--gf-text-muted);
}

.gf-footer__title {
  font-size: var(--gf-fs-sm);
  font-weight: var(--gf-fw-semibold);
  color: var(--gf-text-secondary);
  margin: 0 0 var(--gf-space-3);
  letter-spacing: var(--gf-tracking-wide);
  text-transform: uppercase;
}

.gf-footer__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  /* 移动端: 横排居中, 避免链接竖排上下拉得过长 */
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--gf-space-2) var(--gf-space-4);
}

.gf-footer__link {
  font-size: var(--gf-fs-sm);
  color: var(--gf-text-muted);
  text-decoration: none;
  transition: color var(--gf-dur-fast) var(--gf-ease-standard);
}

.gf-footer__link:hover,
.gf-footer__link:focus-visible {
  color: var(--gf-text-primary);
}

.gf-footer__line {
  font-size: var(--gf-fs-sm);
  line-height: var(--gf-lh-relaxed);
  color: var(--gf-text-muted);
  margin: 0;
}

.gf-footer__bottom {
  margin-top: var(--gf-space-6);
  padding-top: var(--gf-space-4);
  border-top: 1px solid var(--gf-border-subtle);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--gf-space-2);
  font-size: var(--gf-fs-xs);
  color: var(--gf-text-muted);
}

.gf-footer__filing {
  letter-spacing: var(--gf-tracking-wide);
}

@media (min-width: 768px) {
  .gf-footer__cols {
    flex-direction: row;
    justify-content: space-between;
    align-items: flex-start;
    text-align: left;
    gap: var(--gf-space-12);
  }
  .gf-footer__col--brand {
    align-items: flex-start;
    flex: 1.2;
  }
  .gf-footer__col {
    max-width: none;
    flex: 1;
  }
  /* 链接保持横向排列(用户要求非移动端也横排) */
  .gf-footer__list {
    flex-direction: row;
    flex-wrap: wrap;
    justify-content: flex-start;
    gap: var(--gf-space-2) var(--gf-space-4);
  }
}

/* 底部版权: 始终水平居中 */
.gf-footer__bottom {
  flex-direction: row;
  justify-content: center;
  text-align: center;
}
</style>

<style>
[data-mode='tv'] .gf-footer__inner {
  padding-inline: var(--gf-tv-safe);
}
</style>
