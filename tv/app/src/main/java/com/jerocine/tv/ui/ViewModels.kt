package com.jerocine.tv.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.jerocine.tv.data.FilmDetailResp
import com.jerocine.tv.data.HistoryItem
import com.jerocine.tv.data.HomeResp
import com.jerocine.tv.data.ServiceLocator
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

private val repo get() = ServiceLocator.repository
private val userRepo get() = ServiceLocator.userRepository

private suspend fun <T> load(block: suspend () -> T): UiState<T> =
    try {
        UiState.Success(block())
    } catch (e: Exception) {
        UiState.Error(e.message ?: "未知错误")
    }

data class HomeDashboard(
    val home: HomeResp,
    val histories: List<HistoryItem> = emptyList()
)

class HomeViewModel : ViewModel() {
    private val _state = MutableStateFlow<UiState<HomeDashboard>>(UiState.Loading)
    val state: StateFlow<UiState<HomeDashboard>> = _state.asStateFlow()

    init { refresh() }

    fun refresh() {
        _state.value = UiState.Loading
        viewModelScope.launch {
            _state.value = load {
                val home = repo.home()
                val histories = if (ServiceLocator.isLoggedIn) {
                    userRepo.historyList(page = 1, size = 3)
                } else {
                    ServiceLocator.tokenStore.localHistories().take(3).map { it.toHistoryItem() }
                }
                HomeDashboard(home = home, histories = histories)
            }
        }
    }
}

class DetailViewModel : ViewModel() {
    private val _state = MutableStateFlow<UiState<FilmDetailResp>>(UiState.Loading)
    val state: StateFlow<UiState<FilmDetailResp>> = _state.asStateFlow()

    fun load(mid: Long) {
        _state.value = UiState.Loading
        viewModelScope.launch { _state.value = load { repo.filmDetail(mid) } }
    }
}
