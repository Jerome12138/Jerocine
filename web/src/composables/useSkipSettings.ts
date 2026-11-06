/**
 * useSkipSettings - 按片 (filmId) 持久化"跳片头/片尾秒数"，支持用户级全局默认
 *
 * - 存储位置:
 *   - 单影片: localStorage 键 `jc-skip-{filmId}` → JSON `{ intro, outro, enabled }`
 *   - 用户级默认(2026-10-26 新增): localStorage 键 `jc-skip-default` → 同构 JSON,
 *     作为适用于全部影片的默认配置; 单影片配置优先于该默认。
 * - 都没存过 / 解析失败 → 内置默认 (90/60)
 * - 已登录: 单影片 save/reset 异步回写账号; 全局默认仅本地(未登录即本地级)。
 *   安卓播放页由 web 层(PlayView)读取本 composable 后把秒数传给原生播放器, 故安卓同样受配置。
 *
 * 用法:
 *   const { get, effective, save, reset, getDefault, saveDefault, resetDefault } = useSkipSettings()
 *   const cfg = get(filmId)           // 单影片 ?? 用户级默认 ?? 内置默认
 *   const eff = effective(filmId)     // enabled=false → { intro: 0, outro: 0 }(不跳)
 *   save(filmId, { intro: 30, outro: 0, enabled: true })
 *   saveDefault({ intro: 60, outro: 30, enabled: true })   // 全局默认(设置页)
 */
import { getCurrentScope, onScopeDispose } from 'vue'
import { getToken } from '@/utils/token'
import * as skipApi from '@/api/skip'

export const DEFAULT_INTRO_SEC = 90
export const DEFAULT_OUTRO_SEC = 60
/** 用户级全局默认键（设置页"跳过片头片尾"默认配置; 单影片配置优先） */
export const DEFAULT_SKIP_KEY = 'jc-skip-default'

/**
 * 跳过时长归一化：只接受**非负整数**（负数 / 小数 / 非有限值 → 0 / 向下取整）。
 * 抽成纯函数是因为播放器设置浮窗的「[−] 数字 [＋]」控件会传「当前值 − 10」这种**可能为负的中间值**进来；
 * 若只在字符串层用 `replace(/\D/g,'')` 归一化，`-10` 会被剥成 `10`（负数变正数，实测踩过）→ 一律先按数值钳制。
 */
export function normalizeSkipSec(n: number): number {
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : 0
}

/* ---------------- 账号回写（防抖 2s） ----------------
 * 2026-11-06 用户要求：时长设置调接口要防抖。播放器设置浮窗里连点「±10s」/ 连敲数字会高频改动，
 * 每次都 PUT 既浪费又容易被限流；本地 localStorage 仍然**立即**写（播放行为不受影响），
 * 只有「回写账号」这一步延迟到停手 2s 后只发最后一次。 */

/** 回写账号的防抖窗口（ms）。 */
export const SKIP_SAVE_DEBOUNCE_MS = 2000

/** 挂起中的回写任务：filmId → { 定时器, 待发配置 } */
const pendingSkipSaves = new Map<string, { timer: ReturnType<typeof setTimeout>; filmId: number; cfg: SkipConfig }>()

/** 回写账号的唯一出口（未登录 / 失败一律静默，不影响本地）。 */
async function pushSkipSave(filmId: number, cfg: SkipConfig): Promise<void> {
  if (!getToken()) return
  try {
    await skipApi.skipSave(filmId, cfg.intro, cfg.outro, cfg.enabled)
  } catch {
    /* 失败静默：本地已写，账号回写下次再补 */
  }
}

/** 防抖调度：同一片在窗口内的多次改动只保留最后一次。 */
function scheduleSkipSave(filmId: number, cfg: SkipConfig): void {
  if (typeof window === 'undefined') return
  const key = String(filmId)
  const prev = pendingSkipSaves.get(key)
  if (prev) clearTimeout(prev.timer)
  const timer = setTimeout(() => {
    pendingSkipSaves.delete(key)
    void pushSkipSave(filmId, cfg)
  }, SKIP_SAVE_DEBOUNCE_MS)
  pendingSkipSaves.set(key, { timer, filmId, cfg })
}

/** 取消某片的挂起回写（reset 必须调，否则 2s 后又被 PUT 写回来）。 */
function cancelSkipSave(filmId: number): void {
  const key = String(filmId)
  const p = pendingSkipSaves.get(key)
  if (p) {
    clearTimeout(p.timer)
    pendingSkipSaves.delete(key)
  }
}

/** 立即把挂起的回写全部发出（作用域销毁时调用，避免防抖窗口内离开页面丢掉最后一次改动）。 */
export function flushSkipSaves(): void {
  for (const p of [...pendingSkipSaves.values()]) {
    clearTimeout(p.timer)
    void pushSkipSave(p.filmId, p.cfg)
  }
  pendingSkipSaves.clear()
}

export interface SkipConfig {
  intro: number
  outro: number
  /** 跳过总开关: false = 不跳(秒数原样保留, 下次打开不用重填) */
  enabled: boolean
}

function keyOf(filmId: string | number): string {
  return `jc-skip-${filmId}`
}

function parseRaw(raw: string | null): SkipConfig | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as Partial<SkipConfig>
    const intro = Number(parsed.intro)
    const outro = Number(parsed.outro)
    if (!Number.isFinite(intro) || !Number.isFinite(outro)) return null
    return {
      intro: Math.max(0, Math.floor(intro)),
      outro: Math.max(0, Math.floor(outro)),
      // 存量数据无 enabled 字段 → 视为启用
      enabled: parsed.enabled !== false
    }
  } catch {
    return null
  }
}

function getRaw(filmId: string | number): SkipConfig | null {
  if (typeof window === 'undefined') return null
  return parseRaw(window.localStorage.getItem(keyOf(filmId)))
}

function getGlobalRaw(): SkipConfig | null {
  if (typeof window === 'undefined') return null
  return parseRaw(window.localStorage.getItem(DEFAULT_SKIP_KEY))
}

export interface UseSkipSettings {
  /** 取此 filmId 的跳过配置: 单影片 ?? 用户级全局默认 ?? 内置默认 */
  get: (filmId: string | number) => SkipConfig
  /** 存某剧的配置. intro/outro 都 0 等于关闭跳过; enabled=false 等于本剧不跳 */
  save: (filmId: string | number, cfg: SkipConfig) => void
  /** 取实际生效的秒数: enabled=false → 全 0(不跳), 秒数原值仍保留在配置里 */
  effective: (filmId: string | number) => { intro: number; outro: number }
  /** 清除某剧的配置 (回归用户级默认/内置默认) */
  reset: (filmId: string | number) => void
  /** 是否走默认 (无自定义) */
  isDefault: (filmId: string | number) => boolean
  /** 用户级全局默认（设置页用）: 无 → 内置默认 */
  getDefault: () => SkipConfig
  /** 保存用户级全局默认 */
  saveDefault: (cfg: SkipConfig) => void
  /** 清除用户级全局默认 (回归内置默认) */
  resetDefault: () => void
  /** 是否未设置过全局默认 */
  isDefaultGlobal: () => boolean
  DEFAULT_INTRO: number
  DEFAULT_OUTRO: number
}

export function useSkipSettings(): UseSkipSettings {
  // 作用域销毁（离开播放页/组件卸载）时把挂起的回写发出去，避免防抖窗口内丢掉最后一次改动。
  // getCurrentScope() 兜底：本 composable 也会在组件作用域外被调用（如 nativePlay 的工具函数），
  // 那时不注册，避免 Vue 的 "onScopeDispose called outside of a scope" 警告。
  if (getCurrentScope()) onScopeDispose(flushSkipSaves)
  const get = (filmId: string | number): SkipConfig =>
    getRaw(filmId) ?? getGlobalRaw() ?? { intro: DEFAULT_INTRO_SEC, outro: DEFAULT_OUTRO_SEC, enabled: true }
  return {
    get,
    save(filmId, cfg) {
      if (typeof window === 'undefined') return
      const safe: SkipConfig = {
        intro: Math.max(0, Math.floor(cfg.intro)),
        outro: Math.max(0, Math.floor(cfg.outro)),
        enabled: cfg.enabled !== false
      }
      try {
        window.localStorage.setItem(keyOf(filmId), JSON.stringify(safe))
      } catch {
        /* 配额满 / 隐私模式 -> 忽略 */
      }
      // 已登录: 防抖 2s 回写账号(跨设备), 失败静默不影响本地
      scheduleSkipSave(Number(filmId), safe)
    },
    effective(filmId) {
      const cfg = get(filmId)
      return cfg.enabled ? { intro: cfg.intro, outro: cfg.outro } : { intro: 0, outro: 0 }
    },
    reset(filmId) {
      if (typeof window === 'undefined') return
      try {
        window.localStorage.removeItem(keyOf(filmId))
      } catch {
        /* ignore */
      }
      // 先取消挂起的防抖回写, 否则 2s 后会把刚删掉的配置又 PUT 回来
      cancelSkipSave(Number(filmId))
      if (getToken()) {
        void skipApi.skipReset(Number(filmId)).catch(() => {})
      }
    },
    isDefault(filmId) {
      return getRaw(filmId) === null
    },
    getDefault() {
      return getGlobalRaw() ?? { intro: DEFAULT_INTRO_SEC, outro: DEFAULT_OUTRO_SEC, enabled: true }
    },
    saveDefault(cfg) {
      if (typeof window === 'undefined') return
      const safe: SkipConfig = {
        intro: Math.max(0, Math.floor(cfg.intro)),
        outro: Math.max(0, Math.floor(cfg.outro)),
        enabled: cfg.enabled !== false
      }
      try {
        window.localStorage.setItem(DEFAULT_SKIP_KEY, JSON.stringify(safe))
      } catch {
        /* ignore */
      }
    },
    resetDefault() {
      if (typeof window === 'undefined') return
      try {
        window.localStorage.removeItem(DEFAULT_SKIP_KEY)
      } catch {
        /* ignore */
      }
    },
    isDefaultGlobal() {
      return getGlobalRaw() === null
    },
    DEFAULT_INTRO: DEFAULT_INTRO_SEC,
    DEFAULT_OUTRO: DEFAULT_OUTRO_SEC
  }
}

/**
 * 从账号拉取全部跳过设置灌入 localStorage(覆盖本地), 让 get() 同步读到账号值。
 * 登录成功后 + 开机已登录时调用。未登录直接跳过。
 */
export async function hydrateSkipFromAccount(): Promise<void> {
  if (typeof window === 'undefined' || !getToken()) return
  try {
    const rows = await skipApi.skipList()
    for (const r of rows) {
      const safe: SkipConfig = {
        intro: Math.max(0, Math.floor(r.intro)),
        outro: Math.max(0, Math.floor(r.outro)),
        enabled: r.enabled !== false
      }
      window.localStorage.setItem(keyOf(r.mid), JSON.stringify(safe))
    }
  } catch {
    /* 拉取失败保留本地, 不打扰 */
  }
}
