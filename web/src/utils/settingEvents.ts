/**
 * Web 设置账号记忆的事件名 (发布方/订阅方共享, 独立模块避免循环依赖):
 *  - SETTING_CHANGED: 设置持久化到 localStorage 时派发(useWebSettings.persist /
 *    useDanmaku.writeDanmakuSettings), useCloudSettings 订阅并防抖上报云端。
 *  - SETTINGS_APPLIED: 云端快照合并到本地后派发(useCloudSettings.apply),
 *    useDanmaku 订阅以重载弹幕 settings(响应式 ref 无法感知第三方写入 localStorage)。
 */
export const SETTING_CHANGED_EVENT = 'jc-setting-changed'
export const SETTINGS_APPLIED_EVENT = 'jc-web-settings-applied'
