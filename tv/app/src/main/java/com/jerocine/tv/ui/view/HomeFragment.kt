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
import androidx.recyclerview.widget.ConcatAdapter
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.MainActivity
import com.jerocine.tv.R
import com.jerocine.tv.data.HistoryItem
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.ui.HomeDashboard
import com.jerocine.tv.ui.HomeViewModel
import com.jerocine.tv.ui.UiState
import com.jerocine.tv.ui.deriveHomeTvDashboard
import kotlinx.coroutines.launch

class HomeFragment : Fragment(R.layout.fragment_home) {
    private val viewModel: HomeViewModel by viewModels()
    private val sectionAdapter = HomeSectionAdapter(::openDetail, ::openCategory)
    private val navAdapter = HomeNavAdapter(::openCategory)
    private val headerAdapter = HomeDashboardHeaderAdapter(
        onPosterClick = ::openDetail,
        onHistory = ::openHistory,
        onCategory = ::openCategory,
        onSearch = ::openSearch,
        onAccount = ::openAccount,
        onSettings = ::openSettings,
    )
    private var focusedMid: Long? = null
    private var dashboard: HomeDashboard? = null
    private var recent: List<HistoryItem> = emptyList()
    private var recentLoaded = false

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val sections = view.findViewById<RecyclerView>(R.id.home_sections)
        val progress = view.findViewById<ProgressBar>(R.id.home_progress)
        val errorText = view.findViewById<TextView>(R.id.home_error_text)
        val retry = view.findViewById<Button>(R.id.home_retry)
        val nav = view.findViewById<RecyclerView>(R.id.home_navigation)
        val search = view.findViewById<ImageButton>(R.id.home_search)
        val login = view.findViewById<Button>(R.id.home_login)
        val history = view.findViewById<ImageButton>(R.id.home_history)
        val favorites = view.findViewById<ImageButton>(R.id.home_favorites)
        val settings = view.findViewById<ImageButton>(R.id.home_settings)
        var contentRevealed = false

        sections.layoutManager = LinearLayoutManager(requireContext())
        sections.adapter = ConcatAdapter(headerAdapter, sectionAdapter)
        sections.itemAnimator = null
        nav.layoutManager = LinearLayoutManager(requireContext(), RecyclerView.HORIZONTAL, false)
        nav.adapter = navAdapter
        nav.itemAnimator = null
        listOf<View>(search, login, history, favorites, settings, retry).forEach { control ->
            control.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        }
        retry.setOnClickListener { viewModel.refresh() }
        search.setOnClickListener { (requireActivity() as MainActivity).openSearch() }
        login.text = "登录"
        login.setOnClickListener { (requireActivity() as MainActivity).openLogin() }
        history.setOnClickListener { (requireActivity() as MainActivity).openHistory() }
        favorites.setOnClickListener { (requireActivity() as MainActivity).openFavorites() }
        settings.setOnClickListener { (requireActivity() as MainActivity).openSettings() }
        loadRecentOnce()

        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                viewModel.state.collect { state ->
                    when (state) {
                        UiState.Loading -> {
                            progress.isVisible = true
                            sections.isVisible = false
                            errorText.isVisible = false
                            retry.isVisible = false
                        }
                        is UiState.Error -> {
                            progress.isVisible = false
                            sections.isVisible = false
                            errorText.text = state.message
                            errorText.isVisible = true
                            retry.isVisible = true
                        }
                        is UiState.Success -> {
                            dashboard = state.data
                            val items = mapHomeSections(state.data)
                            navAdapter.submitList(mapHomeNavigation(state.data))
                            submitHeader()
                            progress.isVisible = false
                            errorText.isVisible = items.isEmpty()
                            errorText.text = if (items.isEmpty()) "暂无内容" else ""
                            retry.isVisible = items.isEmpty()
                            sections.isVisible = items.isNotEmpty()
                            sectionAdapter.submitList(items) {
                                sections.post {
                                    if (!contentRevealed) {
                                        contentRevealed = true
                                        sections.revealTvContent { ServiceLocator.tokenStore.reduceMotion }
                                    }
                                    val mid = focusedMid
                                    if (mid == null || !sectionAdapter.requestPosterFocus(
                                            sections,
                                            mid,
                                            headerAdapter.itemCount,
                                        )
                                    ) {
                                        headerAdapter.requestInitialFocus(sections)
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    override fun onDestroyView() {
        focusedMid = view?.findFocus()?.tag as? Long ?: focusedMid
        super.onDestroyView()
    }

    private fun openDetail(mid: Long) {
        focusedMid = mid
        (requireActivity() as MainActivity).openDetail(mid)
    }

    private fun openCategory(pid: Long) {
        (requireActivity() as MainActivity).openCategory(pid)
    }

    private fun openHistory() {
        (requireActivity() as MainActivity).openHistory()
    }

    private fun openSearch() {
        (requireActivity() as MainActivity).openSearch()
    }

    private fun openAccount(loggedIn: Boolean) {
        if (loggedIn) (requireActivity() as MainActivity).openSettings()
        else (requireActivity() as MainActivity).openLogin()
    }

    private fun openSettings() {
        (requireActivity() as MainActivity).openSettings()
    }

    private fun submitHeader() {
        val value = dashboard ?: return
        val model = deriveHomeTvDashboard(value)
        headerAdapter.submit(
            HomeHeaderContent(
                hero = model.hero,
                recent = recent,
                firstPid = model.firstPid,
                loggedIn = ServiceLocator.isLoggedIn,
            ),
        )
    }

    private fun loadRecentOnce() {
        if (recentLoaded) return
        recentLoaded = true
        viewLifecycleOwner.lifecycleScope.launch {
            recent = runCatching {
                if (ServiceLocator.isLoggedIn) {
                    ServiceLocator.userRepository.historyList(page = 1, size = 3)
                } else {
                    ServiceLocator.tokenStore.localHistories().take(3).map { it.toHistoryItem() }
                }
            }.getOrDefault(emptyList())
            submitHeader()
        }
    }

    companion object {
        fun newInstance() = HomeFragment()
    }
}
