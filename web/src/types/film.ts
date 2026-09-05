/**
 * 影视 DTO —— 对齐新后端 /api/v1 契约(server/internal/dto/film.go + openapi.yaml)。
 * 关键: cover(非 picture)、sources/episodes(非 list/linkList)、related(非 relate)、
 *       dbScore:number、mid 主键、详情字段扁平(无 descriptor)。
 */

/** 导航分类(带子分类) */
export interface NavCategory {
  id: number
  pid: number
  name: string
  children?: NavCategory[]
}

/** 影片卡片(列表/首页/搜索/推荐) */
export interface Card {
  mid: number
  name: string
  cover: string
  cid: number
  pid: number
  cName: string
  subTitle: string
  area: string
  year: number
  state: string
  remarks: string
  dbScore: number
}

/** 单集 */
export interface Episode {
  episode: string
  link: string
}

/** 一个播放源 */
export interface PlaySource {
  id: string
  name: string
  episodes: Episode[]
}

/** 影片详情(字段扁平 + 多源) */
export interface FilmDetail {
  mid: number
  name: string
  cover: string
  cid: number
  pid: number
  cName: string
  subTitle: string
  actor: string
  director: string
  area: string
  language: string
  year: number
  classTag: string
  remarks: string
  state: string
  dbScore: number
  content: string
  playFrom: string[]
  sources: PlaySource[]
}

/** GET /films/{mid} 响应 */
export interface FilmDetailResp {
  detail: FilmDetail
  related: Card[]
}

/** GET /films/{mid}/play 响应 */
export interface PlayInfo {
  detail: FilmDetail
  current: Episode
  currentSource: string
  currentEpisode: number
  related: Card[]
}

/** 首页区块 */
export interface HomeRow {
  nav: NavCategory
  latest: Card[]
  hot: Card[]
}

/** GET /home 响应 */
export interface HomeData {
  categories: NavCategory[]
  rows: HomeRow[]
}

/** GET /films/classify 响应 */
export interface ClassifyData {
  title?: NavCategory
  news: Card[]
  top: Card[]
  recent: Card[]
}

/** 筛选标签(小写 name/value, 对齐后端 TagOption) */
export interface FilterTag {
  name: string
  value: string
}

/** GET /categories/{pid}/filters 响应 */
export interface Filters {
  titles: Record<string, string>
  tags: Record<string, FilterTag[]>
  sortList: string[]
}

/** GET /films 查询参数(camelCase) */
export interface FilmsQuery {
  keyword?: string
  pid?: number
  category?: number
  plot?: string
  area?: string
  language?: string
  year?: number
  sort?: string
  page?: number
  size?: number
}
