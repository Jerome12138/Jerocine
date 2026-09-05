package com.jerocine.tv.ui.view

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.ImageButton
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import androidx.core.view.isVisible
import androidx.fragment.app.Fragment
import androidx.fragment.app.viewModels
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.MainActivity
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.data.applySearchKeyboardKey
import com.jerocine.tv.ui.SearchViewModel
import com.jerocine.tv.ui.UiState
import com.jerocine.tv.ui.deriveSearchTvModel
import com.jerocine.tv.ui.searchKeyboardKeys
import kotlinx.coroutines.launch

class SearchFragment : Fragment(R.layout.fragment_search) {
    private val viewModel: SearchViewModel by viewModels()
    private val resultAdapter = PosterAdapter(::openDetail)
    private var keyword = ""
    private var histories = emptyList<String>()

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val query = view.findViewById<TextView>(R.id.search_query)
        val back = view.findViewById<ImageButton>(R.id.search_back)
        val keyboard = view.findViewById<RecyclerView>(R.id.search_keyboard)
        val historyRow = view.findViewById<LinearLayout>(R.id.search_history_row)
        val clearHistory = view.findViewById<Button>(R.id.search_clear_history)
        val results = view.findViewById<RecyclerView>(R.id.search_results)
        val progress = view.findViewById<ProgressBar>(R.id.search_progress)
        val message = view.findViewById<TextView>(R.id.search_message)
        val resultCount = view.findViewById<TextView>(R.id.search_result_count)
        var resultsRevealed = false

        histories = ServiceLocator.tokenStore.searchHistory()
        keyboard.layoutManager = GridLayoutManager(requireContext(), 8)
        keyboard.adapter = SearchKeyboardAdapter(searchKeyboardKeys()) { key ->
            if (key == "搜索") {
                submitSearch()
            } else {
                val command = when (key) {
                    "退格" -> "BACKSPACE"
                    "空格" -> "SPACE"
                    "清空" -> "CLEAR"
                    else -> key
                }
                keyword = applySearchKeyboardKey(keyword, command)
                query.text = keyword.ifBlank { "输入关键字 / 拼音首字母" }
                viewModel.search(keyword.trim())
            }
        }
        keyboard.itemAnimator = null

        results.layoutManager = GridLayoutManager(requireContext(), 4)
        results.adapter = resultAdapter
        results.itemAnimator = null

        back.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        clearHistory.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }
        clearHistory.setOnClickListener {
            ServiceLocator.tokenStore.clearSearchHistory()
            histories = emptyList()
            bindKeywords(historyRow, histories)
            clearHistory.isVisible = false
        }
        bindKeywords(historyRow, histories)
        clearHistory.isVisible = histories.isNotEmpty()

        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                viewModel.state.collect { state ->
                    when (state) {
                        UiState.Loading -> {
                            progress.isVisible = true
                            results.isVisible = resultAdapter.itemCount > 0
                            message.isVisible = false
                        }
                        is UiState.Error -> {
                            progress.isVisible = false
                            results.isVisible = false
                            message.text = "搜索失败：${state.message}"
                            message.isVisible = true
                        }
                        is UiState.Success -> {
                            val model = deriveSearchTvModel(keyword, emptyList(), histories, state.data)
                            progress.isVisible = false
                            resultCount.text = "共 ${model.results.size} 部"
                            resultAdapter.submitList(model.results) {
                                if (!resultsRevealed && model.results.isNotEmpty()) {
                                    resultsRevealed = true
                                    results.revealTvContent { ServiceLocator.tokenStore.reduceMotion }
                                }
                            }
                            results.isVisible = model.results.isNotEmpty()
                            message.text = if (model.results.isEmpty()) {
                                "${model.emptyTitle}\n${model.emptyDesc}"
                            } else {
                                ""
                            }
                            message.isVisible = model.results.isEmpty()
                        }
                    }
                }
            }
        }
        keyboard.post { keyboard.requestFocus() }
    }

    private fun bindKeywords(container: LinearLayout, values: List<String>) {
        container.removeAllViews()
        values.take(4).forEach { value ->
            val button = Button(requireContext(), null, 0, R.style.GfChip).apply {
                text = value
                isAllCaps = false
                isFocusable = true
                textSize = 14f
                installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
                setOnClickListener {
                    keyword = value
                    view?.findViewById<TextView>(R.id.search_query)?.text = value
                    submitSearch()
                }
            }
            container.addView(button, LinearLayout.LayoutParams(0, 44.dp, 1f).apply {
                marginEnd = 6.dp
            })
        }
    }

    private fun submitSearch() {
        keyword = keyword.trim()
        if (keyword.isNotBlank()) {
            ServiceLocator.tokenStore.addSearchHistory(keyword)
            histories = ServiceLocator.tokenStore.searchHistory()
            view?.findViewById<LinearLayout>(R.id.search_history_row)?.let { bindKeywords(it, histories) }
            view?.findViewById<Button>(R.id.search_clear_history)?.isVisible = histories.isNotEmpty()
        }
        viewModel.search(keyword)
    }

    private fun openDetail(mid: Long) = (requireActivity() as MainActivity).openDetail(mid)

    private val Int.dp: Int get() = (this * resources.displayMetrics.density).toInt()

    companion object {
        fun newInstance() = SearchFragment()
    }
}
