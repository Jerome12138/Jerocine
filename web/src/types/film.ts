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
  /** 横版大图(16:9, 轮播/横幅背景用)。来自 TMDB 回填(movie_search.backdrop), 后端按
   *  "有则返回、无则字段缺省"输出; 一律按"有则用、无则回退 cover"处理。 */
  poster?: string
  cid: number
  pid: number
  cName: string
  subTitle: string
  area: string
  year: number
  state: string
  remarks: string
  dbScore: number
  /** 上映日期(ISO 前缀串: 2026-09-11 / 2026-07 / 2007)。源站未提供时字段缺省 */
  pubDate?: string
  /** 当前豆瓣榜位(1 起)。不在榜时字段缺省 —— 卡片角标 "Hot No.N" 用 */
  hotRank?: number
  /** 榜位来源榜单中文名(如 "热门电影")。不在榜时缺省 —— 无轮播位时这条卡片要兜底当首屏大图,
   *  描述行靠它把 "No.N" 说清是哪个榜 */
  hotBoard?: string
  /** 类型标签(如 "动作,冒险")。同上, 兜底首屏大图时拆成标签行; 无标签影片缺省 */
  classTag?: string
}

/** 首页轮播项(GET /banners) —— 后端生效位: 手动配置位 + 热榜自动补位, 与后台管理页同源。 */
export interface HomeBanner {
  /** banner=手动配置位 / fallback=热榜自动补位 */
  source: 'banner' | 'fallback'
  mid?: number
  name: string
  subtitle?: string
  /** 横图(宽幅主视觉; 自动位为 TMDB 回填图, 可能缺省) */
  image?: string
  /** 竖图/封面 */
  poster?: string
  /** 自定义跳转(站内路径或外链); 优先于 mid */
  link?: string
  /** 影片元信息(带 mid 的生效位由后端按 mid 补齐; 纯自定义位缺省) —— 首屏大图描述行用 */
  dbScore?: number
  /** 类型标签(如 "动作,冒险"), 前端按逗号/顿号/斜杠拆分展示 */
  classTag?: string
  /** 当前豆瓣榜位(1 起), 缺省/0 = 不在榜 */
  hotRank?: number
  /** 榜位来源榜单中文名(如 "热门电影"), 展示成「豆瓣·热门电影 No.1」 */
  hotBoard?: string
}

/** HeroCarousel 单项 —— 兼容"影片卡片(Card)"与"后台 Banner"两种来源。 */
export interface HeroItem {
  mid?: number
  name: string
  /** 竖图/封面 */
  cover?: string
  /** 横图(宽幅主视觉) */
  poster?: string
  cName?: string
  area?: string
  year?: number
  remarks?: string
  /** 自定义跳转(站内路径或外链); 有则优先于 mid */
  link?: string
  /** 评分(0/缺省 = 无评分) —— 与详情页 dbScore 同源 */
  dbScore?: number
  /** 类型标签(classTag 原始串, 前端拆分) */
  classTag?: string
  /** 豆瓣榜位(0/缺省 = 不在榜) */
  hotRank?: number
  /** 榜位来源榜单中文名 */
  hotBoard?: string
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
  /** 服务端 m3u8 可达性(广告过滤代理链路): false=直接走直链跳过过滤代理; undefined=未测 */
  adFilterOk?: boolean
}

/** 影片详情(字段扁平 + 多源) */
export interface FilmDetail {
  mid: number
  name: string
  cover: string
  /** 横图(16:9, 详情页 hero 背景用)。TMDB 回填后才有, 缺省为 undefined, 使用处须兜底 cover */
  backdrop?: string
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
  /** 上映日期(ISO 前缀串, 源站未提供时缺省) */
  pubDate?: string
  /** 当前豆瓣榜位(1 起, 不在榜缺省) —— 详情页「豆瓣·热门电影 No.N」用 */
  hotRank?: number
  /** 榜位来源榜单中文名(如 "热门电影"); 同一片多集合各有位次, 展示具体是哪个榜的 No.X */
  hotBoard?: string
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
  /** 全站跨类别热榜(「🔥 热门榜单」行) —— 与 rows[].hot(该分类热榜)口径不同, 后端已混排好 */
  hot: Card[]
  rows: HomeRow[]
}

/** GET /films/classify 响应 */
export interface ClassifyData {
  title?: NavCategory
  news: Card[]
  top: Card[]
  recent: Card[]
  /** 高分榜(db_score 降序, 排除解说) —— scoredCount 为 0 时必为空数组, 不渲染分区 */
  score: Card[]
  /** 该分类有评分的影片总数(0 = 该分类无评分数据, 隐藏高分榜入口) */
  scoredCount: number
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
