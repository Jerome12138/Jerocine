/**
 * 直播（直播源 + 频道）类型。
 * 服务端解析 m3u 出频道列表（带缓存），web 只消费解析结果。
 */

/** 公开直播源（不含 url —— 频道地址属于内部信息） */
export interface LiveSource {
  id: number
  name: string
}

export interface LiveSourcesResp {
  sources: LiveSource[]
}

/** 公开直播频道 */
export interface LiveChannel {
  name: string
  group: string
  logo: string
  url: string
}

export interface LiveChannelsResp {
  id: number
  name: string
  channels: LiveChannel[]
}

/** 频道分组（web 侧按 group 字段聚合展示） */
export interface LiveChannelGroup {
  name: string
  channels: LiveChannel[]
}

/** 管理端直播源（完整字段） */
export interface LiveSourceManage {
  id: number
  name: string
  url: string
  remark: string
  sort: number
  /** 0 启用 / 1 停用（站内开关同口径） */
  state: number
  createdAt: number
  updatedAt: number
}

/** 管理端直播源写入 */
export interface LiveSourceInput {
  name: string
  url: string
  remark: string
  sort: number
  /** 0 启用 / 1 停用；缺省启用 */
  state: number
}
