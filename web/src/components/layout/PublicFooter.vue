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
  <footer class="jc-footer">
    <div class="jc-footer__inner container-page">
      <!-- 桌面三列；移动单列居中 -->
      <div class="jc-footer__cols">
        <div class="jc-footer__col jc-footer__col--brand">
          <span class="jc-footer__brand text-brand-gradient">{{ siteName }}</span>
          <p v-if="description" class="jc-footer__desc">
            {{ description }}
          </p>
        </div>
        <div class="jc-footer__col">
          <h4 class="jc-footer__title">站点</h4>
          <ul class="jc-footer__list">
            <li>
              <RouterLink class="jc-footer__link" to="/index">首页</RouterLink>
            </li>
            <li>
              <RouterLink class="jc-footer__link" to="/search">搜索</RouterLink>
            </li>
            <li>
              <RouterLink class="jc-footer__link" to="/history">观看历史</RouterLink>
            </li>
            <li>
              <RouterLink class="jc-footer__link" to="/favorites">我的收藏</RouterLink>
            </li>
          </ul>
        </div>
        <div class="jc-footer__col">
          <h4 class="jc-footer__title">关于</h4>
          <p class="jc-footer__line">
            本站仅提供 web 页面服务，所有视频内容均来自互联网，与本站无关。
          </p>
        </div>
      </div>

      <!-- 底栏 -->
      <div class="jc-footer__bottom">
        <span>{{ siteName }} &copy; {{ year }}</span>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.jc-footer {
  margin-top: auto;
  background-color: #07070a;
  color: var(--jc-text-muted);
  border-top: 1px solid var(--jc-border-subtle);
}

.jc-footer__inner {
  padding-block: var(--jc-space-10) var(--jc-space-6);
}

.jc-footer__cols {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-6);
  text-align: center;
  align-items: center;
}

.jc-footer__col {
  width: 100%;
  max-width: 360px;
}

.jc-footer__col--brand {
  display: flex;
  flex-direction: column;
  gap: var(--jc-space-2);
  align-items: center;
}

.jc-footer__brand {
  font-family: var(--jc-font-display);
  font-size: var(--jc-fs-xl);
  font-weight: var(--jc-fw-black);
  letter-spacing: var(--jc-tracking-tight);
}

.jc-footer__desc {
  font-size: var(--jc-fs-sm);
  line-height: var(--jc-lh-relaxed);
  color: var(--jc-text-muted);
}

.jc-footer__title {
  font-size: var(--jc-fs-sm);
  font-weight: var(--jc-fw-semibold);
  color: var(--jc-text-secondary);
  margin: 0 0 var(--jc-space-3);
  letter-spacing: var(--jc-tracking-wide);
  text-transform: uppercase;
}

.jc-footer__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  /* 移动端: 横排居中, 避免链接竖排上下拉得过长 */
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--jc-space-2) var(--jc-space-4);
}

.jc-footer__link {
  font-size: var(--jc-fs-sm);
  color: var(--jc-text-muted);
  text-decoration: none;
  transition: color var(--jc-dur-fast) var(--jc-ease-standard);
}

.jc-footer__link:hover,
.jc-footer__link:focus-visible {
  color: var(--jc-text-primary);
}

.jc-footer__line {
  font-size: var(--jc-fs-sm);
  line-height: var(--jc-lh-relaxed);
  color: var(--jc-text-muted);
  margin: 0;
}

.jc-footer__bottom {
  margin-top: var(--jc-space-6);
  padding-top: var(--jc-space-4);
  border-top: 1px solid var(--jc-border-subtle);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--jc-space-2);
  font-size: var(--jc-fs-xs);
  color: var(--jc-text-muted);
}

.jc-footer__filing {
  letter-spacing: var(--jc-tracking-wide);
}

@media (min-width: 768px) {
  .jc-footer__cols {
    flex-direction: row;
    justify-content: space-between;
    align-items: flex-start;
    text-align: left;
    gap: var(--jc-space-12);
  }
  .jc-footer__col--brand {
    align-items: flex-start;
    flex: 1.2;
  }
  .jc-footer__col {
    max-width: none;
    flex: 1;
  }
  /* 链接保持横向排列(用户要求非移动端也横排) */
  .jc-footer__list {
    flex-direction: row;
    flex-wrap: wrap;
    justify-content: flex-start;
    gap: var(--jc-space-2) var(--jc-space-4);
  }
}

/* 底部版权: 始终水平居中 */
.jc-footer__bottom {
  flex-direction: row;
  justify-content: center;
  text-align: center;
}
</style>

<style>
[data-mode='tv'] .jc-footer__inner {
  padding-inline: var(--jc-tv-safe);
}
</style>
