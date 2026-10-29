/**
 * 直播纯函数（LiveView 的可测内核）：频道分组聚合。
 */
import type { LiveChannel, LiveChannelGroup } from '@/types/live'

/** 未标注分组的频道归入该组（与服务端 DefaultGroup 同值）。 */
export const LIVE_DEFAULT_GROUP = '其他'

/**
 * 频道按分组聚合（保持首次出现顺序；组内保序）。
 * 服务端已按源文件顺序返回频道，这里只做分组归拢，不排序不打乱。
 */
export function groupChannels(channels: readonly LiveChannel[]): LiveChannelGroup[] {
  const groups: LiveChannelGroup[] = []
  const index = new Map<string, number>()
  for (const ch of channels) {
    const name = ch.group?.trim() ? ch.group.trim() : LIVE_DEFAULT_GROUP
    const i = index.get(name)
    if (i !== undefined) {
      groups[i]!.channels.push(ch)
    } else {
      index.set(name, groups.length)
      groups.push({ name, channels: [ch] })
    }
  }
  return groups
}
