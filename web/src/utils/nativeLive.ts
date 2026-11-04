/**
 * 直播 → 原生(APK)播放器派发（与影片 nativePlay.ts 同思路）。
 *
 * 映射：**直播源 = 播放器的「源」，频道 = 播放器的「集」** —— 于是 WebShell(TV 网页壳)
 * 内点频道即可拉起原生 ExoPlayer，播放器自带的「换源 / 选集」面板就是「换直播源 / 切频道」。
 *
 * 与影片派发的差异：直播**不建观看历史 record**、无续播、无跳片头片尾（符合直播语义）。
 */
import type { LiveChannel, LiveSource } from '@/types/live'
import { liveApi } from '@/api'
import { isNative, jerocine } from '@/utils/jerocineNative'
import { readLiveLast, writeLiveLast } from '@/utils/live'

export interface LivePlaylistCfg {
  sources: Array<{ id: string; name: string; episodes: Array<{ url: string; title: string }> }>
  currentSourceId: string
  startIndex: number
  filmId: string
  filmName: string
  /** Q③ 直播标志: JerocineBridge 读它写 EXTRA_IS_LIVE → PlayerSession.isLive
   *  （原生播放器据此隐藏进度行/屏蔽 ±10s seek/短路预取）。旧载荷无此键 = false 向后兼容。 */
  isLive: boolean
}

/**
 * 纯映射：源 + 各源频道 → playPlaylist 入参。
 * 无任何可播频道时返回 null（调用方据此回退网页内播放）。
 */
export function buildLivePlaylistCfg(
  sources: readonly LiveSource[],
  channelsBySource: ReadonlyMap<number, readonly LiveChannel[]>,
  currentSourceId: number,
  startChannelIndex: number,
  filmName = '直播'
): LivePlaylistCfg | null {
  const mapped = sources
    .map((s) => ({
      id: String(s.id),
      name: s.name,
      episodes: (channelsBySource.get(s.id) ?? [])
        .filter((c) => !!c.url)
        .map((c) => ({ url: c.url, title: c.name || '频道' }))
    }))
    .filter((s) => s.episodes.length > 0)
  if (mapped.length === 0) return null

  const selected = mapped.find((s) => s.id === String(currentSourceId)) ?? mapped[0]!
  return {
    sources: mapped,
    currentSourceId: selected.id,
    startIndex: Math.min(Math.max(startChannelIndex, 0), selected.episodes.length - 1),
    // 非数字 filmId：避免原生回传 playerProgress 时被 web 当作影片进度处理
    filmId: '',
    filmName,
    // Q③: 直播载荷恒带 isLive=true（TV 壳 LivePlayer.buildLivePlayerPayload 同款标志）
    isLive: true
  }
}

/**
 * 有原生桥时派发原生播放。返回是否已派发（false → 调用方回退网页内播放）。
 */
export function dispatchNativeLive(
  sources: readonly LiveSource[],
  channelsBySource: ReadonlyMap<number, readonly LiveChannel[]>,
  currentSourceId: number,
  startChannelIndex: number,
  filmName = '直播'
): boolean {
  if (!isNative()) return false
  const cfg = buildLivePlaylistCfg(sources, channelsBySource, currentSourceId, startChannelIndex, filmName)
  if (!cfg) return false
  jerocine.playPlaylist(cfg)
  return true
}

/**
 * 原生壳 **路由守卫** 用：拉「全部源 + 频道」→ 派发原生播放（跳过 web /live 列表页）。
 * 返回是否已派发（false → 调用方放行 web /live 兜底）。
 * 起点优先按上次观看恢复（源 + 频道地址），否则第一个源的第一个频道；起播后回写记忆。
 */
export async function dispatchNativeLiveForRoute(): Promise<boolean> {
  if (!isNative()) return false
  const resp = await liveApi.getLiveSources()
  const sources = resp?.sources ?? []
  if (sources.length === 0) return false

  const channelsBySource = new Map<number, LiveChannel[]>()
  await Promise.all(
    sources.map(async (s) => {
      try {
        channelsBySource.set(s.id, (await liveApi.getLiveChannels(s.id)).channels)
      } catch {
        channelsBySource.set(s.id, [])
      }
    })
  )

  const last = readLiveLast()
  const startSourceId =
    last?.sourceId && sources.some((s) => s.id === last.sourceId) ? last.sourceId : sources[0]!.id
  const list = channelsBySource.get(startSourceId) ?? []
  const index = last?.url ? Math.max(list.findIndex((c) => c.url === last.url), 0) : 0

  const dispatched = dispatchNativeLive(sources, channelsBySource, startSourceId, index)
  if (dispatched) {
    writeLiveLast({
      sourceId: startSourceId,
      url: list[index]?.url ?? '',
      sourceName: sources.find((s) => s.id === startSourceId)?.name ?? '',
      channelName: list[index]?.name ?? '',
      ts: Date.now()
    })
  }
  return dispatched
}
