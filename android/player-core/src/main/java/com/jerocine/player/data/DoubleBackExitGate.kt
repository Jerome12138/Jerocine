package com.jerocine.player.data

/**
 * 双击返回退出的确认窗口(纯逻辑, 单包双壳共用 —— 组件化二期 A4 自 tvshell 下沉)。
 *
 * 用法: 第一次按返回 → [shouldExit] 返回 false(调用方提示"再按一次退出");
 * 窗口期内再按 → 返回 true(调用方退出)。退出后窗口自动清零。
 * 页面栈变化时调用 [reset] 取消未决窗口(原生壳 fragment backstack 变化 reset,
 * web 壳在前端路由消费返回时 reset), 避免跨页面误触退出。
 */
class DoubleBackExitGate(
    private val intervalMs: Long = 2_000L,
) {
    private var firstPressAt: Long? = null

    fun shouldExit(nowMs: Long): Boolean {
        val previous = firstPressAt
        if (previous != null && nowMs - previous in 0..intervalMs) {
            firstPressAt = null
            return true
        }
        firstPressAt = nowMs
        return false
    }

    fun reset() {
        firstPressAt = null
    }
}
