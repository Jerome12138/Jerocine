package com.jerocine.tv.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.jerocine.tv.data.ServiceLocator
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

sealed interface LoginUi {
    data object Loading : LoginUi
    data class Pairing(val userCode: String) : LoginUi
    data object Approved : LoginUi
    data class Error(val message: String) : LoginUi
}

fun applyLoginKeyboardKey(value: String, key: String): String = when (key) {
    "BACKSPACE" -> value.dropLast(1)
    "CLEAR" -> ""
    "SPACE" -> value + " "
    else -> value + key
}

fun loginKeyboardKeys(): List<String> =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@._-".map(Char::toString) +
        listOf("退格", "清空", "空格", "登录")

class LoginViewModel : ViewModel() {
    private val repo get() = ServiceLocator.repository
    private val _state = MutableStateFlow<LoginUi>(LoginUi.Loading)
    val state: StateFlow<LoginUi> = _state.asStateFlow()

    init {
        start()
    }

    fun start() {
        _state.value = LoginUi.Loading
        viewModelScope.launch {
            try {
                val code = repo.deviceCode()
                _state.value = LoginUi.Pairing(code.userCode)
                poll(code.deviceCode, code.interval.coerceAtLeast(2))
            } catch (e: Exception) {
                _state.value = LoginUi.Error(e.message ?: "获取配对码失败")
            }
        }
    }

    private suspend fun poll(deviceCode: String, intervalSec: Int) {
        repeat(120) {
            delay(intervalSec * 1000L)
            try {
                val result = repo.devicePoll(deviceCode)
                if (result.status == "ok" && result.token.isNotBlank()) {
                    ServiceLocator.saveLogin(result.token, result.userName)
                    _state.value = LoginUi.Approved
                    return
                }
            } catch (e: Exception) {
                _state.value = LoginUi.Error(e.message ?: "配对失败")
                return
            }
        }
        _state.value = LoginUi.Error("配对超时，请重试")
    }
}
