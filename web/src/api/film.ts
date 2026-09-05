import { http } from './http'
import { swrGet } from '@/utils/swrCache'
import type { Paginated } from '@/types/api'
import type {
  Card,
  ClassifyData,
  FilmDetailResp,
  FilmsQuery,
  Filters,
  HomeData,
  NavCategory,
  PlayInfo
} from '@/types/film'

/** GET /home 首页聚合(SWR: 首屏最重, 切回直接复用 + 后台静默刷新) */
export const getHome = (): Promise<HomeData> =>
  swrGet('home', (silent) => http.get<unknown, HomeData>('/home', { silent }))

/** GET /categories 导航分类 */
export const getCategories = (): Promise<NavCategory[]> =>
  http.get<unknown, NavCategory[]>('/categories')

/** GET /categories/:pid/filters 某一级分类的筛选维度(SWR 按 pid) */
export const getFilters = (pid: number | string): Promise<Filters> =>
  swrGet(`filters:${pid}`, (silent) =>
    http.get<unknown, Filters>(`/categories/${pid}/filters`, { silent })
  )

/** GET /films 列表(keyword 全文检索 / 多维筛选 / 分页) */
export const getFilms = (
  query: FilmsQuery,
  options?: { signal?: AbortSignal }
): Promise<Paginated<Card>> =>
  http.get<unknown, Paginated<Card>>('/films', { params: query, signal: options?.signal })

/** GET /films/classify 分类页三榜(SWR 按 pid) */
export const getClassify = (pid: number | string): Promise<ClassifyData> =>
  swrGet(`classify:${pid}`, (silent) =>
    http.get<unknown, ClassifyData>('/films/classify', { params: { pid }, silent })
  )

/** GET /films/:mid 详情 + 相关推荐(SWR 5min) */
export const getFilmDetail = (mid: number | string): Promise<FilmDetailResp> =>
  swrGet(
    `detail:${mid}`,
    (silent) => http.get<unknown, FilmDetailResp>(`/films/${mid}`, { silent }),
    300_000
  )

/** GET /films/:mid/play 播放信息 */
export const getPlayInfo = (params: {
  mid: number | string
  source?: string
  episode?: number
}): Promise<PlayInfo> =>
  http.get<unknown, PlayInfo>(`/films/${params.mid}/play`, {
    params: { source: params.source, episode: params.episode }
  })

/** GET /films/:mid/related 相关推荐 */
export const getRelated = (mid: number | string): Promise<Card[]> =>
  http.get<unknown, Card[]>(`/films/${mid}/related`)
