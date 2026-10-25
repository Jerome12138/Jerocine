/**
 * comment-core-library（UMD 老库, 无官方类型）全局类型声明。
 *
 * 该库 dist 为纯 script（无 module.exports / window 绑定），经 index.html
 * <script defer src="/vendor/comment-core-library/CommentCoreLibrary.js"> 全局加载，
 * 顶层 var 暴露到 window。渲染模型：CommentManager(stage: HTMLElement) + init('css')
 * 创建 DOM 弹幕（div.cmt 绝对定位）挂到 stage；时间单位秒。
 * 事件联动仅用 start/stop/time/seek/clear/load，见 useDanmaku.ts。
 */
interface CommentCoreLibraryLike {
  CommentManager: new (stage: HTMLElement) => CommentManagerLike
}

interface CommentManagerLike {
  stage: HTMLElement
  options: {
    global?: { opacity?: number; scale?: number; className?: string }
    scroll?: { opacity?: number; scale?: number }
    limit?: number
    seekTrigger?: number
  }
  timeline: DanmakuInputComment[]
  init(renderer?: 'css' | 'legacy'): void
  load(comments: DanmakuInputComment[]): void
  clear(): void
  setBounds(): void
  start(): void
  stop(): void
  seek(time: number): void
  /** 推进到指定秒（把 stime<=t 的弹幕送入渲染队列） */
  time(time: number): void
}

/** 库 load() 接受的弹幕对象（与后端 DanmuComment 不同：此处秒 + stime 字段名） */
interface DanmakuInputComment {
  stime: number
  mode?: number
  color?: number
  text: string
  size?: number
}

interface Window {
  CommentCoreLibrary?: CommentCoreLibraryLike
}
