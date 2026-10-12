package com.jerocine.tv.data

/**
 * 单包合并后的「壳层共享 SharedPreferences 契约」唯一声明处。
 *
 * 合并前两壳是两个 APK, 各写各的 prefs, 名字/键名重复不算问题;
 * 合并成一个包后 **两个壳读写的是同一份 prefs**(同一 applicationId), 键名一旦漂移
 * 就会出现"web 壳存了、原生壳读不到"这类静默错配(例如切壳模式后登录态/后端地址不一致)。
 *
 * 因此: prefs 文件名与键名只在此定义, web 壳(com.jerocine.app) 与原生壳(com.jerocine.tv)
 * 一律引用本对象, **禁止再硬编码字面量**。
 *
 * 键值语义:
 *  - [KEY_SHELL_MODE]: "auto"(默认, 走设备探针) / "web" / "native"
 *      写方: 设置抽屉"壳模式"行、原生壳设置页、运行时降级兜底(WebView 不可恢复)
 *      读方: [com.jerocine.tv.route.DeviceProbe] (EntryRouter 路由)
 *  - [KEY_SERVER_URL]: 后端地址(不带尾斜杠)。两壳共用 ⇒ 切换壳模式后连的是同一个后端。
 */
object ShellPrefs {
    const val NAME = "jerocine_prefs"
    const val KEY_SHELL_MODE = "jc-shell-mode"
    const val KEY_SERVER_URL = "server_url"
}
