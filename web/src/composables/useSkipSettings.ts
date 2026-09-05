/**
 * useSkipSettings - 按片 (filmId) 持久化"跳片头/片尾秒数"
 *
 * - 存储位置: localStorage 键 `gf-skip-{filmId}` → JSON `{ intro, outro }`(同步读, 不阻塞播放)
 * - 没存过 / 解析失败 → 用默认值 (90/60), 适用于绝大多数中国大陆影源
 * - 已登录: 跨设备同步 — save/reset 异步回写账号; hydrateSkipFromAccount() 登录/开机时灌入 localStorage。
 *   安卓播放页由 web 层(PlayView)读取本 composable 后把秒数传给原生播放器, 故安卓同样受账号同步。
 *
 * 用法:
 *   const { get, save, DEFAULT_INTRO, DEFAULT_OUTRO } = useSkipSettings()
 *   const cfg = get(filmId)           // { intro: 90, outro: 60 } | 用户自定值
 *   save(filmId, { intro: 30, outro: 0 })
 */
import { getToken } from '@/utils/token'
import * as skipApi from '@/api/skip'

export const DEFAULT_INTRO_SEC = 90
export const DEFAULT_OUTRO_SEC = 60

export interface SkipConfig {
  intro: number
  outro: number
}

function keyOf(filmId: string | number): string {
  return `gf-skip-${filmId}`
}

function getRaw(filmId: string | number): SkipConfig | null {
  if (typeof window === 'undefined') return null
  try {
    const raw = window.localStorage.getItem(keyOf(filmId))
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<SkipConfig>
    const intro = Number(parsed.intro)
    const outro = Number(parsed.outro)
    if (!Number.isFinite(intro) || !Number.isFinite(outro)) return null
    return {
      intro: Math.max(0, Math.floor(intro)),
      outro: Math.max(0, Math.floor(outro))
    }
  } catch {
    return null
  }
}

export interface UseSkipSettings {
  /** 取此 filmId 的跳过配置, 无则返回默认 */
  get: (filmId: string | number) => SkipConfig
  /** 存某剧的配置. intro/outro 都 0 等于关闭跳过 */
  save: (filmId: string | number, cfg: SkipConfig) => void
  /** 清除某剧的配置 (回归默认) */
  reset: (filmId: string | number) => void
  /** 是否走默认 (无自定义) */
  isDefault: (filmId: string | number) => boolean
  DEFAULT_INTRO: number
  DEFAULT_OUTRO: number
}

export function useSkipSettings(): UseSkipSettings {
  return {
    get(filmId) {
      const v = getRaw(filmId)
      return v ?? { intro: DEFAULT_INTRO_SEC, outro: DEFAULT_OUTRO_SEC }
    },
    save(filmId, cfg) {
      if (typeof window === 'undefined') return
      const safe: SkipConfig = {
        intro: Math.max(0, Math.floor(cfg.intro)),
        outro: Math.max(0, Math.floor(cfg.outro))
      }
      try {
        window.localStorage.setItem(keyOf(filmId), JSON.stringify(safe))
      } catch {
        /* 配额满 / 隐私模式 -> 忽略 */
      }
      // 已登录: 异步回写账号(跨设备), 失败静默不影响本地
      if (getToken()) {
        void skipApi.skipSave(Number(filmId), safe.intro, safe.outro).catch(() => {})
      }
    },
    reset(filmId) {
      if (typeof window === 'undefined') return
      try {
        window.localStorage.removeItem(keyOf(filmId))
      } catch {
        /* ignore */
      }
      if (getToken()) {
        void skipApi.skipReset(Number(filmId)).catch(() => {})
      }
    },
    isDefault(filmId) {
      return getRaw(filmId) === null
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
        outro: Math.max(0, Math.floor(r.outro))
      }
      window.localStorage.setItem(keyOf(r.mid), JSON.stringify(safe))
    }
  } catch {
    /* 拉取失败保留本地, 不打扰 */
  }
}
