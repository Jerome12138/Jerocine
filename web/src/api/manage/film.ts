import { http, toast } from '../http'
import type { AxiosRequestConfig } from 'axios'
import type {
  CollectFilmPayload,
  FilmAddPayload,
  FilmClass,
  ManageFilmDetailResp,
  ManageFilmSearchParams,
  ManageFilmSearchResp,
  SourceFilmResult
} from '@/types/manage'

/** GET /manage/films 后台影片搜索（分页 {list,page}） */
export const searchList = (
  params: ManageFilmSearchParams
): Promise<ManageFilmSearchResp> =>
  http.get<unknown, ManageFilmSearchResp>('/manage/films', { params })

/** GET /manage/categories 分类树（顶级带 children, 与公开 /categories 同构） */
export const classTree = (): Promise<FilmClass[]> =>
  http.get<unknown, FilmClass[]>('/manage/categories')

/** 分类详情（无单查询端点, 取树后递归匹配 id, 含子级） */
export const classFind = async (id: number): Promise<FilmClass | undefined> => {
  const walk = (nodes: FilmClass[]): FilmClass | undefined => {
    for (const c of nodes) {
      if (c.id === id) return c
      const hit = c.children ? walk(c.children) : undefined
      if (hit) return hit
    }
    return undefined
  }
  return walk(await classTree())
}

/** DELETE /manage/categories/:id 删除分类。config.silent=true 关闭拦截器的「操作成功」 toast，由调用方自管提示 */
export const classDel = (id: number, config?: AxiosRequestConfig): Promise<void> =>
  http.delete<unknown, void>(`/manage/categories/${id}`, config)

/** POST /manage/categories 新增 / 修改分类（upsert）。config.silent=true 关闭拦截器 toast，由调用方自管提示 */
export const classUpdate = (data: FilmClass, config?: AxiosRequestConfig): Promise<void> =>
  http.post<unknown, void>('/manage/categories', data, config)

/** POST /manage/films 新增影片。config.silent=true 关闭拦截器的「操作成功」 toast，由调用方自管提示 */
export const add = (data: FilmAddPayload, config?: AxiosRequestConfig): Promise<void> =>
  http.post<unknown, void>('/manage/films', data, config)

/** DELETE /manage/films/:mid 软删影片（可在回收站恢复）。config.silent=true 关闭拦截器的「操作成功」 toast，由调用方自管提示 */
export const softDelete = (
  mid: number | string,
  config?: AxiosRequestConfig
): Promise<void> => http.delete<unknown, void>(`/manage/films/${mid}`, config)

/** POST /manage/films/:mid/restore 从回收站恢复影片。config.silent=true 关闭拦截器的「操作成功」 toast，由调用方自管提示 */
export const restore = (
  mid: number | string,
  config?: AxiosRequestConfig
): Promise<void> => http.post<unknown, void>(`/manage/films/${mid}/restore`, undefined, config)

/** GET /manage/films/:mid/detail 后台影片详情(实时全源全集) */
export const detail = (mid: number | string): Promise<ManageFilmDetailResp> =>
  http.get<unknown, ManageFilmDetailResp>(`/manage/films/${mid}/detail`)

/** POST /manage/spider/search 按片名搜各源(预览不落库; sources 空=全部启用源) */
export const searchSources = (
  keyword: string,
  sources?: string[]
): Promise<SourceFilmResult[]> =>
  http.post<unknown, SourceFilmResult[]>('/manage/spider/search', { keyword, sources })

/**
 * POST /manage/spider/collect-film 采集影片。返回形态取决于入参:
 * - 单源精确({sourceId, mid|keyword|vodId}) → 返回**单个** SourceFilmResult(可能含 candidates);
 * - 多源({keyword, sources?}) → 返回 SourceFilmResult[]。
 * 调用方用 normalizeCollectResult / Array.isArray 归一化处理。
 * config.silent=true 关闭拦截器的「操作成功」 toast —— 采集结果成败交由调用方按
 * 结果内 error/collected 字段自管提示(单源采集业务失败时 HTTP 仍是 200, 拦截器的
 * 成功 toast 会与页面的「采集失败」toast 同时出现, 自相矛盾)。
 */
export const collectFilm = (
  payload: CollectFilmPayload,
  config?: AxiosRequestConfig
): Promise<SourceFilmResult | SourceFilmResult[]> =>
  http.post<unknown, SourceFilmResult | SourceFilmResult[]>(
    '/manage/spider/collect-film',
    payload,
    config
  )

/** 把 collectFilm 返回(单个或数组)归一为数组, 便于统一遍历 */
export const normalizeCollectResult = (
  res: SourceFilmResult | SourceFilmResult[]
): SourceFilmResult[] => (Array.isArray(res) ? res : [res])

/**
 * 汇总各源采集/补采结果并 toast —— 采集新片弹窗与影片列表行内「补源」共用。
 * 业务成败在 HTTP 200 的 error/collected 字段里, 由这里单点汇报(配合 silent 请求, 避免
 * 拦截器的「操作成功」与业务失败 toast 同屏自相矛盾)。
 */
export const reportCollectResults = (res: SourceFilmResult[], ctx = ''): void => {
  const oks = res.filter((r) => !r.error)
  const errs = res.filter((r) => r.error)
  const total = oks.reduce((sum, r) => sum + (r.collected ?? 0), 0)
  const parts = oks.map((r) => `${r.sourceName} ${r.collected}`).join('、')
  if (oks.length) {
    toast('success', `${ctx}采集完成, 共入库 ${total} 集` + (parts ? `（${parts}）` : ''))
  }
  if (errs.length) {
    toast('error', `${errs.length} 个源采集失败: ${errs.map((r) => r.sourceName).join('、')}`)
  }
  if (!oks.length && !errs.length) {
    toast('error', `${ctx}未采集到任何内容`)
  }
}
