package com.jerocine.tv.ui.view

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.ImageButton
import android.widget.ProgressBar
import android.widget.TextView
import androidx.core.os.bundleOf
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
import com.jerocine.tv.ui.CategoryLibraryViewModel
import com.jerocine.tv.ui.UiState
import com.jerocine.tv.ui.categoryFilterGroups
import kotlinx.coroutines.launch

class CategoryLibraryFragment : Fragment(R.layout.fragment_category_library) {
    private val viewModel: CategoryLibraryViewModel by viewModels()
    private val pid by lazy { requireArguments().getLong(ARG_PID) }
    private val categoryName by lazy { requireArguments().getString(ARG_NAME).orEmpty().ifBlank { "分类" } }
    private val sort by lazy { requireArguments().getString(ARG_SORT).orEmpty() }
    private val posterAdapter = PosterAdapter(::openDetail, R.layout.item_category_poster)
    private val filterAdapter = CategoryFilterAdapter(::selectFilter)

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val back = view.findViewById<ImageButton>(R.id.category_library_back)
        val title = view.findViewById<TextView>(R.id.category_library_title)
        val subtitle = view.findViewById<TextView>(R.id.category_library_subtitle)
        val count = view.findViewById<TextView>(R.id.category_library_count)
        val filters = view.findViewById<RecyclerView>(R.id.category_library_filters)
        val list = view.findViewById<RecyclerView>(R.id.category_library_list)
        val pager = view.findViewById<View>(R.id.category_library_pager)
        val previous = view.findViewById<Button>(R.id.category_library_previous)
        val next = view.findViewById<Button>(R.id.category_library_next)
        val pageLabel = view.findViewById<TextView>(R.id.category_library_page)
        val progress = view.findViewById<ProgressBar>(R.id.category_library_progress)
        val message = view.findViewById<TextView>(R.id.category_library_message)
        val retry = view.findViewById<Button>(R.id.category_library_retry)
        var contentRevealed = false

        title.text = categoryName
        subtitle.text = "| ${categoryName}库"
        filters.layoutManager = androidx.recyclerview.widget.LinearLayoutManager(requireContext())
        filters.adapter = filterAdapter
        filters.itemAnimator = null
        list.layoutManager = GridLayoutManager(requireContext(), RESULT_COLUMN_COUNT)
        list.adapter = posterAdapter
        list.itemAnimator = null
        back.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        retry.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        previous.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        next.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }
        retry.setOnClickListener { viewModel.load(pid, sort) }
        previous.setOnClickListener { viewModel.changePage(-1) }
        next.setOnClickListener { viewModel.changePage(1) }

        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                viewModel.state.collect { state ->
                    when (state) {
                        UiState.Loading -> {
                            progress.isVisible = true
                            list.isVisible = false
                            filters.isVisible = false
                            pager.isVisible = false
                            message.isVisible = false
                            retry.isVisible = false
                        }
                        is UiState.Error -> {
                            progress.isVisible = false
                            list.isVisible = false
                            filters.isVisible = false
                            pager.isVisible = false
                            message.text = "加载失败：${state.message}"
                            message.isVisible = true
                            retry.isVisible = true
                        }
                        is UiState.Success -> {
                            val cards = state.data.films
                            val page = state.data.page
                            progress.isVisible = false
                            count.text = "共 ${page.total} 部影片 · 第 ${page.current} / ${page.pageCount.coerceAtLeast(1)} 页"
                            val groups = categoryFilterGroups(state.data)
                            filterAdapter.submitList(groups)
                            filters.isVisible = groups.isNotEmpty()
                            posterAdapter.submitList(cards) {
                                if (!contentRevealed && cards.isNotEmpty()) {
                                    contentRevealed = true
                                    list.revealTvContent { ServiceLocator.tokenStore.reduceMotion }
                                }
                            }
                            list.isVisible = cards.isNotEmpty()
                            val pageCount = page.pageCount.coerceAtLeast(1)
                            pager.isVisible = pageCount > 1
                            pageLabel.text = "${page.current} / $pageCount"
                            previous.isEnabled = page.current > 1
                            previous.alpha = if (previous.isEnabled) 1f else 0.4f
                            next.isEnabled = page.current.toLong() < pageCount
                            next.alpha = if (next.isEnabled) 1f else 0.4f
                            message.text = if (cards.isEmpty()) "该分类暂无影片" else ""
                            message.isVisible = cards.isEmpty()
                            retry.isVisible = cards.isEmpty()
                        }
                    }
                }
            }
        }
        viewModel.load(pid, sort)
    }

    private fun openDetail(mid: Long) = (requireActivity() as MainActivity).openDetail(mid)

    private fun selectFilter(key: String, value: String) = viewModel.selectFilter(key, value)

    companion object {
        private const val ARG_PID = "pid"
        private const val ARG_NAME = "name"
        private const val ARG_SORT = "sort"
        private const val RESULT_COLUMN_COUNT = 4

        fun newInstance(pid: Long, categoryName: String, sort: String) =
            CategoryLibraryFragment().apply {
                arguments = bundleOf(ARG_PID to pid, ARG_NAME to categoryName, ARG_SORT to sort)
            }
    }
}
