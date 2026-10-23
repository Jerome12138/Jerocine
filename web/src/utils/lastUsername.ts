/**
 * 最近一次注册/申请填报的用户名 —— 供激活页默认回填。
 * 场景: 用户在本机填完注册/申请后, 从邮件点激活链接回来, 用户名不用再敲一遍。
 * localStorage 键沿用 jc- 前缀约定; 同设备同浏览器才生效(换设备仍需手填, 非关键数据)。
 */
const KEY = 'jc-last-username'

export function saveLastUserName(name: string): void {
  try {
    localStorage.setItem(KEY, name)
  } catch {
    /* 隐私模式等场景存不进就算了, 不影响主流程 */
  }
}

export function getLastUserName(): string {
  try {
    return localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}
