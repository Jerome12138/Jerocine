import { http } from './http'
import { swrGet } from '@/utils/swrCache'
import type { Paginated } from '@/types/api'
import type {
  Card,
  ClassifyData,
  FilmDetailResp,
  FilmsQuery,
  Filters,
  HomeBanner,
  HomeData,
  HotBoardFilters,
  HotBoardQuery,
  NavCategory,
  PlayInfo,
  DanmuSourceResp,
  DanmuBilibiliResp,
  DanmuAggregateResp,
  DanmuTencentEpisodesResp,
  DanmuTencentSegmentsResp,
  DanmuTencentCommentsResp
} from '@/types/film'

/** GET /home 首页聚合(SWR: 首屏最重, 切回直接复用 + 后台静默刷新) */
export const getHome = (): Promise<HomeData> =>
  swrGet('home', (silent) => http.get<unknown, HomeData>('/home', { silent }))

/** GET /categories 导航分类 */
export const getCategories = (): Promise<NavCategory[]> =>
  http.get<unknown, NavCategory[]>('/categories')

/** GET /banners 首页轮播(后台可配; 返回空数组时前端回退按热门派生) */
export const getBanners = (): Promise<HomeBanner[]> =>
  swrGet('banners', (silent) => http.get<unknown, HomeBanner[]>('/banners', { silent }))

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

/** GET /hotboard 热榜(8 榜并集或单榜; 普通请求, 不用 SWR —— 筛选/分页实时反映 URL) */
export const getHotBoard = (
  query: HotBoardQuery,
  options?: { signal?: AbortSignal }
): Promise<Paginated<Card>> =>
  http.get<unknown, Paginated<Card>>('/hotboard', { params: query, signal: options?.signal })

/** GET /hotboard/filters 热榜筛选维度(榜单固定 8 榜; 年份/地区/语言聚合在榜影片) */
export const getHotBoardFilters = (): Promise<HotBoardFilters> =>
  http.get<unknown, HotBoardFilters>('/hotboard/filters')

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

/** GET /films/:mid/danmu/source 弹幕来源候选（平台列表；B站为 seasonId）
 *  options.signal 用于超时/卸载中止（客户端 withTimeout 驱动 AbortController）。 */
export const getDanmuSource = (
  mid: number | string,
  options?: { signal?: AbortSignal }
): Promise<DanmuSourceResp> =>
  http.get<unknown, DanmuSourceResp>(`/films/${mid}/danmu/source`, { signal: options?.signal, silent: true })

/** GET /danmu/bilibili 指定 B站剧集整集弹幕（按集号匹配，越界返回空数组）
 *  options.signal 用于超时/卸载中止。 */
export const getDanmuBilibili = (
  params: { seasonId: string; episode: number },
  options?: { signal?: AbortSignal }
): Promise<DanmuBilibiliResp> =>
  http.get<unknown, DanmuBilibiliResp>('/danmu/bilibili', { params, signal: options?.signal, silent: true })

/** GET /danmu/aggregate 按标题匹配的整集聚合弹幕（服务端按 影片名→作品→集 定位）
 *  options.signal 用于超时/卸载中止。404 = 查无作品/该集（上层标"暂无弹幕"）。 */
export const getDanmuAggregate = (
  params: { title: string; episode: number; year?: number },
  options?: { signal?: AbortSignal }
): Promise<DanmuAggregateResp> =>
  http.get<unknown, DanmuAggregateResp>('/danmu/aggregate', { params, signal: options?.signal, silent: true })

/** GET /danmu/tencent/episodes 腾讯分集匹配（拿 matched.episodeId 复合串，后续须 encodeURIComponent）
 *  options.signal 用于超时/卸载中止。 */
export const getDanmuTencentEpisodes = (
  params: { resourceId: string; episode: number },
  options?: { signal?: AbortSignal }
): Promise<DanmuTencentEpisodesResp> =>
  http.get<unknown, DanmuTencentEpisodesResp>('/danmu/tencent/episodes', { params, signal: options?.signal, silent: true })

/** GET /danmu/tencent/segments 腾讯弹幕分段索引（startTime 秒，按段起始排序）
 *  options.signal 用于超时/卸载中止。 */
export const getDanmuTencentSegments = (
  episodeId: string,
  options?: { signal?: AbortSignal }
): Promise<DanmuTencentSegmentsResp> =>
  http.get<unknown, DanmuTencentSegmentsResp>('/danmu/tencent/segments', {
    params: { episodeId },
    signal: options?.signal,
    silent: true
  })

/** GET /danmu/tencent/comments 腾讯单段弹幕
 *  options.signal 用于超时/卸载中止。 */
export const getDanmuTencentComments = (
  params: { episodeId: string; segmentId: string },
  options?: { signal?: AbortSignal }
): Promise<DanmuTencentCommentsResp> =>
  http.get<unknown, DanmuTencentCommentsResp>('/danmu/tencent/comments', {
    params,
    signal: options?.signal,
    silent: true
  })
