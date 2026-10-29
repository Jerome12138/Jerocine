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

/** 热榜榜单维度选项(value=豆瓣集合名, label=中文榜名) */
export interface HotBoardOption {
  value: string
  label: string
}

/** GET /hotboard/filters 响应: 榜单固定 8 榜, 年份/地区/语言由在榜影片聚合 */
export interface HotBoardFilters {
  boards: HotBoardOption[]
  years: number[]
  areas: string[]
  languages: string[]
}

/** GET /hotboard 查询参数(小写, 对齐后端 query) */
export interface HotBoardQuery {
  /** 榜单集合名; 空 = 8 榜并集(全站热榜) */
  board?: string
  pid?: number
  year?: number
  area?: string
  language?: string
  /** rank=按位次(默认) / hot=按热度分(全榜并集时可用) */
  sort?: string
  page?: number
  size?: number
}

/* ============ 弹幕（P0：豆瓣 vendors 候选 + B站弹幕代理） ============ */

/** 一个可拉弹幕的平台（与播放源互不关联，见方案 §四之二） */
export interface DanmuProvider {
  /** tencent / iqiyi / youku / bilibili */
  provider: string
  /** 平台侧整剧/整片资源 id（B站为 seasonId） */
  resourceId: string
}

/** GET /films/:mid/danmu/source 响应（不暴露 db_id，评审 7.7） */
export interface DanmuSourceResp {
  providers: DanmuProvider[]
}

/** 单条弹幕（后端代理返回，时间轴毫秒） */
export interface DanmuComment {
  text: string
  time: number
  color: number
  mode: number
}

/** GET /danmu/bilibili 响应 */
export interface DanmuBilibiliResp {
  comments: DanmuComment[]
}

/** GET /danmu/tencent/episodes 响应（matched=null 表示无匹配集 → 静默无弹幕） */
export interface DanmuTencentEpisodesResp {
  ok: boolean
  data: {
    matched: { episodeId: string } | null
  }
}

/** 腾讯弹幕分段（startTime 秒） */
export interface DanmuTencentSegment {
  provider: string
  startTime: number
  segmentId: string
}

/** GET /danmu/tencent/segments 响应 */
export interface DanmuTencentSegmentsResp {
  ok: boolean
  data: DanmuTencentSegment[]
}

/** GET /danmu/tencent/comments 响应 */
export interface DanmuTencentCommentsResp {
  ok: boolean
  data: DanmuComment[]
}
