package com.jerocine.tv.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.jerocine.tv.data.Card
import com.jerocine.tv.data.ClassifyResp
import com.jerocine.tv.data.FilmFilters
import com.jerocine.tv.data.FilterTag
import com.jerocine.tv.data.PageMeta
import com.jerocine.tv.data.ServiceLocator
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

private val repo get() = ServiceLocator.repository

class SearchViewModel : ViewModel() {
    private val _state = MutableStateFlow<UiState<List<Card>>>(UiState.Success(emptyList()))
    val state: StateFlow<UiState<List<Card>>> = _state.asStateFlow()
    private var searchJob: Job? = null

    fun search(keyword: String) {
        searchJob?.cancel()
        if (keyword.isBlank()) { _state.value = UiState.Success(emptyList()); return }
        _state.value = UiState.Loading
        searchJob = viewModelScope.launch {
            _state.value = try {
                UiState.Success(repo.films(keyword = keyword).list)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                UiState.Error(e.message ?: "搜索失败")
            }
        }
    }
}

class CategoryViewModel : ViewModel() {
    private val _state = MutableStateFlow<UiState<ClassifyResp>>(UiState.Loading)
    val state: StateFlow<UiState<ClassifyResp>> = _state.asStateFlow()

    fun load(pid: Long) {
        _state.value = UiState.Loading
        viewModelScope.launch {
            _state.value = try {
                UiState.Success(repo.classify(pid))
            } catch (e: Exception) {
                UiState.Error(e.message ?: "加载失败")
            }
        }
    }
}

class CategoryLibraryViewModel : ViewModel() {
    private val _state = MutableStateFlow<UiState<CategoryLibraryContent>>(UiState.Loading)
    val state: StateFlow<UiState<CategoryLibraryContent>> = _state.asStateFlow()
    private var currentPid = 0L
    private var currentPage = 1
    private var filters = FilmFilters()
    private var selections: Map<String, String> = emptyMap()
    private var loadJob: Job? = null

    fun load(pid: Long, sort: String) {
        currentPid = pid
        currentPage = 1
        selections = if (sort.isBlank()) emptyMap() else mapOf("Sort" to sort)
        refresh(loadFilters = true)
    }

    fun selectFilter(key: String, value: String) {
        if (key !in FILTER_KEYS || selections[key].orEmpty() == value) return
        selections = selections + (key to value)
        currentPage = 1
        refresh(loadFilters = false)
    }

    fun changePage(delta: Int) {
        val content = (_state.value as? UiState.Success)?.data ?: return
        val next = currentPage + delta
        if (next !in 1..content.page.pageCount.coerceAtLeast(1).toInt()) return
        currentPage = next
        refresh(loadFilters = false)
    }

    private fun refresh(loadFilters: Boolean) {
        loadJob?.cancel()
        if (_state.value !is UiState.Success) _state.value = UiState.Loading
        val pid = currentPid
        val pageNumber = currentPage
        val selected = selections
        loadJob = viewModelScope.launch {
            _state.value = try {
                if (loadFilters) {
                    filters = try {
                        repo.filters(pid)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (_: Exception) {
                        FilmFilters()
                    }
                }
                val page = repo.films(
                    pid = pid,
                    category = selected["Category"]?.toLongOrNull(),
                    plot = selected["Plot"].nullIfBlank(),
                    area = selected["Area"].nullIfBlank(),
                    language = selected["Language"].nullIfBlank(),
                    year = selected["Year"]?.toIntOrNull(),
                    sort = selected["Sort"].nullIfBlank(),
                    current = pageNumber,
                    size = 48,
                )
                UiState.Success(CategoryLibraryContent(filters, page.list, page.page, selected))
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                UiState.Error(e.message ?: "加载失败")
            }
        }
    }

    private fun String?.nullIfBlank(): String? = this?.takeIf(String::isNotBlank)

    companion object {
        private val FILTER_KEYS = setOf("Category", "Plot", "Area", "Language", "Year", "Sort")
    }
}

data class CategoryLibraryContent(
    val filters: FilmFilters = FilmFilters(),
    val films: List<Card> = emptyList(),
    val page: PageMeta = PageMeta(),
    val selections: Map<String, String> = emptyMap(),
)

data class CategoryFilterGroup(
    val key: String,
    val title: String,
    val options: List<FilterTag>,
    val current: String,
)

fun categoryFilterGroups(content: CategoryLibraryContent): List<CategoryFilterGroup> =
    content.filters.sortList.mapNotNull { key ->
        content.filters.tags[key]?.takeIf { it.isNotEmpty() }?.let { options ->
            CategoryFilterGroup(
                key = key,
                title = content.filters.titles[key].orEmpty().ifBlank { key },
                options = options,
                current = content.selections[key].orEmpty(),
            )
        }
    }
