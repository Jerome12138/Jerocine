/**
 * 统一的「派发原生(APK)播放器」入口。
 *
 * 背景: 进入原生播放有 3 条路径(详情页直跳 gotoPlay / 路由守卫快捷路径 /play 拦截 /
 * PlayView 兜底), 早期各自构建 playPlaylist 入参 + 建历史 record, 极易漏字段
 * (曾漏 proxyBase 致原生端侧广告过滤"代理地址未传"不触发; 漏 record 致进度不写历史)。
 *
 * 收敛到这一个函数后, 三处只调它, 改/加字段改一处即全覆盖, 不会再漏。
 *
 * 职责: 映射 sources(原始 m3u8) → 建/更新历史 record(否则 native 周期 playerProgress
 * 回传时 updateProgress 因无 record 而 no-op) → 取按片跳过设置 → 拼 proxyBase → playPlaylist。
 *
 * @returns 是否成功派发(无可播放集时 false, 调用方据此回退 web 路由)。
 */
import type { FilmDetail } from '@/types/film'
import { jerocine, absApiBase } from '@/utils/jerocineNative'
import { useSkipSettings } from '@/composables/useSkipSettings'
import { useHistoryStore } from '@/stores/history'

export function dispatchNativePlaylist(
  detail: FilmDetail,
  sourceId: string,
  episodeIndex: number,
  resumeSec = 0
): boolean {
  const sources = detail.sources ?? []
  const src = sources.find((s) => s.id === sourceId) ?? sources[0]
  if (!src) return false
  const mapped = sources.map((s) => ({
    id: s.id,
    name: s.name,
    episodes: (s.episodes ?? []).map((e) => ({
      url: e.link,
      title: `${detail.name} · ${e.episode ?? ''}`.trim()
    }))
  }))
  if (!mapped.some((s) => s.episodes.length > 0)) return false

  const id = String(detail.mid)
  // 历史: 建/更新一条 record。否则从收藏/外链等入口进入, native 回传 playerProgress 时
  // historyStore.updateProgress 因"无已有 record"直接 no-op, 进度/续播都丢。
  try {
    const epLabel = src.episodes?.[episodeIndex]?.episode ?? String(episodeIndex + 1)
    useHistoryStore().record({
      id,
      name: detail.name,
      link:
        `/play?id=${id}&source=${src.id}&episode=${episodeIndex}` +
        (resumeSec > 0 ? `&currentTime=${Math.floor(resumeSec)}` : ''),
      episode: epLabel,
      picture: detail.cover,
      source: src.id,
      episodeIndex,
      currentTime: resumeSec > 0 ? Math.floor(resumeSec) : 0,
      pid: detail.pid,
      cid: detail.cid
    })
  } catch {
    /* 历史建条失败不阻断播放 */
  }

  const skip = useSkipSettings().get(id)
  jerocine.playPlaylist({
    sources: mapped,
    currentSourceId: src.id,
    startIndex: episodeIndex,
    resumeAtSec: resumeSec,
    skipIntroSec: skip.intro,
    skipOutroSec: skip.outro,
    filmId: id,
    filmName: detail.name,
    // 关键: proxyBase 决定原生端侧广告过滤是否触发, 必须传(统一用 absApiBase)
    proxyBase: absApiBase()
  })
  return true
}
