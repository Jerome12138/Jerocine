package com.jerocine.tv.ui

/** 通用 UI 状态：加载中 / 成功 / 错误 */
sealed interface UiState<out T> {
    data object Loading : UiState<Nothing>
    data class Success<T>(val data: T) : UiState<T>
    data class Error(val message: String) : UiState<Nothing>
}
