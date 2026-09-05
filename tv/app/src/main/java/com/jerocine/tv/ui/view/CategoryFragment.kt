package com.jerocine.tv.ui.view

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.ImageButton
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
import com.jerocine.tv.ui.CategoryViewModel
import com.jerocine.tv.ui.UiState
import com.jerocine.tv.ui.deriveCategoryTvSections
import com.jerocine.tv.ui.flattenCategoryRows
import kotlinx.coroutines.launch

class CategoryFragment : Fragment(R.layout.fragment_category) {
    private val viewModel: CategoryViewModel by viewModels()
    private val categoryAdapter = CategoryAdapter(::openDetail, ::openLibrary)
    private val pid: Long by lazy { requireArguments().getLong(ARG_PID) }
    private var categoryName = "分类"

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val title = view.findViewById<TextView>(R.id.category_title)
        val library = view.findViewById<Button>(R.id.category_library)
        val back = view.findViewById<ImageButton>(R.id.category_back)
        val list = view.findViewById<RecyclerView>(R.id.category_list)
        val progress = view.findViewById<ProgressBar>(R.id.category_progress)
        val message = view.findViewById<TextView>(R.id.category_message)
        val retry = view.findViewById<Button>(R.id.category_retry)
        var contentRevealed = false
        val layoutManager = GridLayoutManager(requireContext(), COLUMN_COUNT)
        layoutManager.spanSizeLookup = object : GridLayoutManager.SpanSizeLookup() {
            override fun getSpanSize(position: Int): Int =
                if (categoryAdapter.getItemViewType(position) == CategoryAdapter.TYPE_HEADER) COLUMN_COUNT else 1
        }
        list.layoutManager = layoutManager
        list.adapter = categoryAdapter
        list.itemAnimator = null
        back.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        retry.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        library.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        library.isEnabled = false
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }
        retry.setOnClickListener { viewModel.load(pid) }

        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                viewModel.state.collect { state ->
                    when (state) {
                        UiState.Loading -> {
                            progress.isVisible = true
                            list.isVisible = false
                            message.isVisible = false
                            retry.isVisible = false
                        }
                        is UiState.Error -> {
                            progress.isVisible = false
                            list.isVisible = false
                            message.text = "加载失败：${state.message}"
                            message.isVisible = true
                            retry.isVisible = true
                        }
                        is UiState.Success -> {
                            val rows = flattenCategoryRows(deriveCategoryTvSections(state.data))
                            categoryName = state.data.title?.name?.ifBlank { "分类" } ?: "分类"
                            title.text = categoryName
                            library.text = "${title.text}库 ›"
                            library.isEnabled = true
                            library.setOnClickListener { openLibrary("") }
                            progress.isVisible = false
                            categoryAdapter.submitList(rows) {
                                if (!contentRevealed && rows.isNotEmpty()) {
                                    contentRevealed = true
                                    list.revealTvContent { ServiceLocator.tokenStore.reduceMotion }
                                }
                            }
                            list.isVisible = rows.isNotEmpty()
                            message.text = if (rows.isEmpty()) "暂无分类内容" else ""
                            message.isVisible = rows.isEmpty()
                            retry.isVisible = rows.isEmpty()
                        }
                    }
                }
            }
        }
        viewModel.load(pid)
    }

    private fun openDetail(mid: Long) = (requireActivity() as MainActivity).openDetail(mid)

    private fun openLibrary(sort: String) =
        (requireActivity() as MainActivity).openCategoryLibrary(pid, categoryName, sort)

    companion object {
        private const val ARG_PID = "pid"
        private const val COLUMN_COUNT = 6

        fun newInstance(pid: Long) = CategoryFragment().apply {
            arguments = Bundle().apply { putLong(ARG_PID, pid) }
        }
    }
}
