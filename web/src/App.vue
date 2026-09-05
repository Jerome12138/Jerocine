<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useViewMode } from '@/composables/useViewMode'
import { installSpatialNavigationOnce } from '@/composables/useSpatialNavigation'
import { installDpadBridge } from '@/utils/dpad'
import { useSiteStore, useNavStore, useHistoryStore } from '@/stores'
import { buildPlayLink } from '@/stores/history'
import { jerocine, isNative } from '@/utils/jerocineNative'
import { telemetry } from '@/utils/telemetry'
import { confirm } from '@/composables/useConfirm'

// 布局壳静态导入（首屏必须）
import PublicLayout from '@/components/layout/PublicLayout.vue'
import ManageLayout from '@/components/layout/ManageLayout.vue'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import MinimalLayout from '@/components/layout/MinimalLayout.vue'

const route = useRoute()
const router = useRouter()

// 启动 view-mode 检测，写入 <html data-mode>
useViewMode()

// D-pad keyCode → 标准 KeyboardEvent.key 桥接（始终安装；非 TV 模式无副作用）
const uninstallDpad = installDpadBridge()

// 空间导航：监听 keydown，在 TV 模式下接管方向键 / Enter / Escape
installSpatialNavigationOnce()

// Capacitor Android BACK 键桥. 优先级:
//   1. 有弹层 (modal/sidebar) 打开 → 关弹层
//   2. 路由有上一页 → router.back()
//   3. 在首页 (history.length <= 1) → 双击退出: 第一下 toast, 2s 内第二下 return false 让原生退
//
// 用局部 typed 变量绑 window — 之前用 ;(window as X).fn = ... 的"防 ASI" 头分号
// 在 minifier 里被当 EmptyStatement 删掉, 导致 let backLastTapAt = 0 紧贴下一行
// (window).gfTvBack 被合并成 `let backLastTapAt=0(window).gfTvBack=...` 直接
// 调 0(window) 全页报 "0 is not a function" Vue runtime-0.
const _winBack = window as unknown as {
  gfTvBack?: () => boolean
  __gfModalCloser?: () => boolean
}
let backLastTapAt = 0
const BACK_DOUBLE_MS = 2000
_winBack.gfTvBack = (): boolean => {
  // 0) BaseDialog 弹窗(冷启动续播提示 / 确认框等, body 标 data-gf-modal-open)优先关闭:
  //    APK 的 BACK 走本桥(不派发按键事件), 故需在此主动关弹窗 —— 派发 Escape 让弹窗走自身 closeOnEsc
  //    (BaseDialog onKeydown 监听 Escape→handleClose; 确认框据此 answerConfirm(false))。
  if (typeof document !== 'undefined' && document.body.hasAttribute('data-gf-modal-open')) {
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    return true
  }
  // 1) 任何 modal 注册了 __gfModalCloser 都优先关 (PublicHeader 抽屉等)
  const closer = _winBack.__gfModalCloser
  if (typeof closer === 'function') {
    try {
      if (closer()) return true
    } catch {
      /* ignore */
    }
  }
  // 2) router 有上一页就回
  if (window.history.length > 1) {
    router.back()
    return true
  }
  // 3) 首页双击退出
  const now = Date.now()
  if (now - backLastTapAt < BACK_DOUBLE_MS) {
    backLastTapAt = 0
    return false // 让原生 moveTaskToBack/finish
  }
  backLastTapAt = now
  if (isNative()) jerocine.toast('再按一次返回退出', false)
  return true
}

onBeforeUnmount(() => {
  uninstallDpad()
  delete _winBack.gfTvBack
})

const layoutMap = {
  public: PublicLayout,
  manage: ManageLayout,
  auth: AuthLayout,
  minimal: MinimalLayout
} as const

type LayoutKey = keyof typeof layoutMap

const currentLayout = computed(() => {
  const key = (route.meta.layout as LayoutKey | undefined) ?? 'public'
  return layoutMap[key] ?? PublicLayout
})

// Toast 容器懒加载（拦截器需要）
const ToastContainer = defineAsyncComponent(
  () => import('@/components/base/BaseToastContainer.vue')
)

// 顶部全局 API 进度条 (uiStore.loading 驱动)
const ApiProgressBar = defineAsyncComponent(
  () => import('@/components/base/ApiProgressBar.vue')
)

// 全局确认弹窗（替换原生 confirm，TV 友好）
const ConfirmDialog = defineAsyncComponent(
  () => import('@/components/base/BaseConfirmDialog.vue')
)

// 路由切换中央 iOS spinner (覆盖所有 layout)
const RouteSpinner = defineAsyncComponent(
  () => import('@/components/layout/RouteSpinner.vue')
)

const siteStore = useSiteStore()
const navStore = useNavStore()
const historyStore = useHistoryStore()

/**
 * #14 原生 APK 冷启动续播提示: 取最近一条观看历史, 弹"继续观看"。
 * 仅原生、每次冷启动一次(sessionStorage 去重); 已在 /play 或无历史则跳过。
 * 登录态 history 在登录 watch 里异步 syncFromRemote 填充 list, 故最多等 ~4s。
 */
async function maybePromptResume(): Promise<void> {
  if (!isNative()) return
  try {
    if (sessionStorage.getItem('gf-resume-prompted') === '1') return
  } catch {
    /* 隐私模式忽略 */
  }
  let rec = historyStore.list[0]
  const deadline = Date.now() + 4000
  while (!rec && Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 250))
    rec = historyStore.list[0]
  }
  if (!rec?.id || !rec.name) return
  try {
    sessionStorage.setItem('gf-resume-prompted', '1')
  } catch {
    /* ignore */
  }
  if (route.path === '/play') return
  const epLabel = rec.episode || `第 ${Number(rec.episodeIndex ?? 0) + 1} 集`
  const ok = await confirm({
    title: '继续观看',
    desc: `上次看到《${rec.name}》${epLabel}, 是否继续?`,
    okText: '继续观看',
    cancelText: '取消'
  })
  if (!ok) return
  // 由当前字段实时拼链接(不用可能过期的 rec.link), 保证接着当前集与进度续播
  void router.push(buildPlayLink(rec))
}

onMounted(() => {
  void maybePromptResume()

  // 站点信息 / 顶级导航预热（并行，失败静默，不阻塞页面渲染）
  Promise.all([siteStore.ensureLoaded(), navStore.ensureLoaded()]).catch(() => {
    // 拦截器已统一 toast，这里仅吞错避免冒泡
  })

  // 开机一次性 diag snapshot — 设备/视口/UA/mode 全量上报, 后台一眼看到 TV 的实际渲染态
  telemetry.reportDiag({ source: 'app-mount' })

  // Bridge 双向健康检查 (仅 native): web 发 echoTest → native 200ms 后回 echoReply.
  // 收到 reply 即上报一条 telemetry — 后台同时有 snapshot (web→native) + echoReply
  // (native→web) 就证明两个方向都通.
  if (isNative()) {
    const nonce = 'echo_' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6)
    const t = Date.now()
    const unsub = jerocine.on('echoReply', (payload) => {
      const p = payload as { nonce?: string; t?: number; replyAt?: number } | null
      const rtt = p?.t ? Date.now() - p.t : -1
      telemetry.track('action', {
        category: 'diag',
        action: 'bridge-echo',
        label: p?.nonce === nonce ? 'roundtrip-ok' : 'nonce-mismatch',
        value: rtt,
        extra: { nonce, replyNonce: p?.nonce, rtt }
      })
      unsub()
    })
    const ack = jerocine.call('echoTest', { nonce, t })
    if (!ack.ok) {
      telemetry.track('action', {
        category: 'diag',
        action: 'bridge-echo',
        label: 'invoke-failed',
        extra: { error: ack.error ?? '' }
      })
    }
    // 3s 内没收到 reply 也上报一条, 标记"单向通 / 反向断"
    window.setTimeout(() => {
      unsub() // 幂等
      // 如果已经上报过 roundtrip-ok, 这里再加一条 timeout 也无害 (用 dedupe 合并)
    }, 3000)
  }

  // 全局 JS 错误 → telemetry 上报 + native toast 调试
  if (typeof window !== 'undefined') {
    // 关键: native 的 WebChromeClient.onConsoleMessage 监听了 console.error 弹 Toast,
    // 但单纯 console.error() 调用不会触发 window.error → 不进 telemetry → "toast 看到了
    // 但后台查不到". 这里 hook console.error, 拷一份到 telemetry 后再调原始 console.error
    // 保留 native 显示行为.
    const origConsoleError = console.error.bind(console)
    console.error = (...args: unknown[]): void => {
      try {
        // 找到第一个 Error 参数; 没有就拼字符串
        const errArg = args.find((a) => a instanceof Error)
        if (errArg) {
          telemetry.trackError(errArg as Error, 'console-error', {
            args: args.map((a) => (a instanceof Error ? a.message : String(a))).join(' | ').slice(0, 500)
          })
        } else {
          const msg = args.map((a) => {
            if (typeof a === 'string') return a
            try { return JSON.stringify(a) } catch { return String(a) }
          }).join(' ').slice(0, 1000)
          telemetry.trackError(msg, 'console-error')
        }
      } catch {
        // 上报失败不能让 console.error 本身失效
      }
      origConsoleError(...args)
    }

    window.addEventListener('error', (ev) => {
      // Firefox iOS Reader Mode 注入的 __firefox__ polyfill 在非 Firefox 浏览器上访问
      // window.__firefox__.reader.checkReadability 会 throw. 这是浏览器/AdBlock 注入的
      // 第三方代码, 不在我们控制内, 直接吞掉避免污染列表.
      if (ev.message && /__firefox__|__webkit__messageHandlers|firefox_reader/i.test(ev.message)) {
        return
      }
      // WebView native evaluateJavascript 注入的事件回调若抛错, 浏览器吃掉 source
      // 信息 → message 退化成 "Script error.", filename/lineno 全 0. 不 toast (无诊断
      // 价值), 但仍上报为 'js-error-opaque' 类别让你在后台知道发生过 — 真实 stack
      // 改靠 app.config.errorHandler (Vue 组件错误) + jerocineNative 内 try/catch 上报.
      const isOpaque =
        ev.message === 'Script error.' && !ev.filename && !ev.lineno
      if (isOpaque) {
        telemetry.trackError('Script error (opaque)', 'js-error-opaque', {
          ua: navigator.userAgent,
          path: window.location.pathname
        })
        return
      }
      const msg = `${ev.message} @ ${ev.filename}:${ev.lineno}:${ev.colno}`
      telemetry.trackError(ev.error ?? msg, 'js-error', {
        file: ev.filename,
        line: ev.lineno,
        col: ev.colno
      })
      if (isNative()) jerocine.toast('ERR: ' + msg, true)
      console.error('[gf-onerror]', msg, ev.error)
    })
    window.addEventListener('unhandledrejection', (ev) => {
      const msg = ev.reason instanceof Error ? `${ev.reason.message}\n${ev.reason.stack ?? ''}` : String(ev.reason)
      telemetry.trackError(ev.reason, 'unhandled-rejection')
      if (isNative()) jerocine.toast('REJECT: ' + msg.slice(0, 200), true)
      console.error('[gf-unhandled]', ev.reason)
    })
  }

  // Jerocine TV APK Native 事件订阅
  if (isNative()) {
    // 注: KEYCODE_MENU 现由 native (MainActivity) 直接弹原生设置侧边栏处理,
    // 不再透传 keyMenu 事件给 web. 此处保留 jerocine.on 仅用于播放器相关事件.

    // 原生播放器事件 → 写历史. 3 类:
    //   playerProgress (5s tick): 同步 episodeIndex + position, 不带 duration
    //   playerEpisodeChange (切集瞬间): 立即把 episodeIndex 推过来, position 归 0
    //   playerClosed (退出): 最终 position + duration, 写定结案
    const writeTick = (payload: unknown): void => {
      const p = payload as { filmId?: string; episodeIndex?: number; position?: number; source?: string } | null
      if (!p?.filmId) return
      historyStore.updateProgress(
        String(p.filmId),
        Number(p.episodeIndex ?? 0),
        Number(p.position ?? 0),
        undefined,
        p.source // 原生换源后同步历史片源
      )
    }
    jerocine.on('playerProgress', writeTick)
    jerocine.on('playerEpisodeChange', (payload) => {
      // 切集立即通知 web — 不等下次 5s tick. position=0 因为新集刚开始.
      const p = payload as { filmId?: string; episodeIndex?: number; source?: string } | null
      if (!p?.filmId) return
      historyStore.updateProgress(String(p.filmId), Number(p.episodeIndex ?? 0), 0, undefined, p.source)
    })
    jerocine.on('playerClosed', (payload) => {
      const p = payload as {
        filmId?: string
        episodeIndex?: number
        position?: number
        duration?: number
        source?: string
      } | null
      if (!p?.filmId) return
      historyStore.updateProgress(
        String(p.filmId),
        Number(p.episodeIndex ?? 0),
        Number(p.position ?? 0),
        Number(p.duration ?? 0), // 时长只在退出时落
        p.source
      )
    })

    // 原生播放器内改跳过参数 → 回写账号(跨设备记忆). intro/outro 秒, 0=不跳.
    jerocine.on('skipSettingChanged', (payload) => {
      const p = payload as { filmId?: string; intro?: number; outro?: number } | null
      if (!p?.filmId) return
      void import('@/composables/useSkipSettings').then(({ useSkipSettings }) => {
        useSkipSettings().save(p.filmId as string, {
          intro: Math.max(0, Number(p.intro ?? 0)),
          outro: Math.max(0, Number(p.outro ?? 0))
        })
      })
    })

    // 原生播放器错误 → telemetry
    // Native 现在发的 payload: {code, errorCodeName, message, currentUrl, causeDetail}
    // 之前只取 code+message, currentUrl 和 cause chain 直接丢了, 后台只看到模糊的
    // "PlayerError 3003: Source error" — 没法定位是 m3u8 索引层挂 还是 segment 层挂.
    jerocine.on('playerError', (payload) => {
      const p = payload as {
        code?: number
        errorCodeName?: string
        message?: string
        currentUrl?: string
        causeDetail?: string
      } | null
      const code = p?.code ?? '?'
      const name = p?.errorCodeName ?? ''
      const msg = p?.message ?? ''
      telemetry.trackError(
        new Error(`PlayerError ${code}${name ? ' (' + name + ')' : ''}: ${msg}`),
        'native-error',
        {
          nativeCode: p?.code,
          errorCodeName: p?.errorCodeName,
          currentUrl: p?.currentUrl,
          causeDetail: p?.causeDetail
        }
      )
    })
  }
})
</script>

<template>
  <component :is="currentLayout">
    <RouterView v-slot="{ Component, route: r }">
      <!--
        manage 后台: 不走 page transition. 500ms out-in 让菜单切换体感"加载不出来"
        (旧内容 500ms 淡出 → 中间空窗 → 新内容 500ms 淡入, 共 1s 以上无反馈).
        7aeb667 只去了 KeepAlive 不够, 这里再 bypass Transition 给工具型页面瞬时反馈.
        loading 状态交由各 view 自己的 Skeleton 兜底.
      -->
      <component
        v-if="r.meta.layout === 'manage'"
        :is="Component"
        :key="r.fullPath"
      />
      <!--
        public 端: 去掉 mode="out-in" 与 leave 过渡. out-in 会让旧页先跑满 500ms
        淡出再 mount 新页, 中间一帧空窗, 体感"加载不出来" (旧页淡出→空窗→新页淡入 共 ~1s).
        现在旧页瞬时移除, 新页 150ms 淡入: 保留轻微动效但无空窗.
        (manage 后台见上方 v-if 分支, 完全无过渡.)
      -->
      <Transition v-else name="page">
        <!--
          key 用 path + 影片 id(不含 source/episode 等其它 query):
          切集/切源只改 source/episode, path 与 id 不变 → key 不变 → 播放页
          不 remount, 纯响应式动态更新播放信息(切集不再整页重建播放器)。
          仅换影片(id 变)或跨路由(path 变)才重建播放器, 避免跨影片状态污染。
        -->
        <component :is="Component" :key="r.path + (r.query.id ?? '')" />
      </Transition>
    </RouterView>
  </component>
  <ToastContainer />
  <ConfirmDialog />
  <ApiProgressBar />
  <RouteSpinner />
</template>

<style>
/*
 * public 页切换: 仅新页淡入 (150ms = --gf-dur-fast), 不定义 leave-* → 旧页瞬时移除, 无空窗.
 * 配合模板里去掉 mode="out-in", 消除原先 500ms 淡出 + 空窗 + 500ms 淡入的 ~1s 惩罚.
 * (实测主因之一: 用户感到"切页加载不出来" 多来自这段固定动画, 与网络无关.)
 */
.page-enter-active {
  transition: opacity var(--gf-dur-fast) var(--gf-ease-out);
}
.page-enter-from {
  opacity: 0;
}
</style>
