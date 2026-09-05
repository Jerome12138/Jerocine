package com.jerocine.tv.ui.view

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.ImageButton
import android.widget.ProgressBar
import android.widget.TextView
import androidx.core.view.isVisible
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.MainActivity
import com.jerocine.tv.R
import com.jerocine.tv.data.HistoryItem
import com.jerocine.tv.data.LocalHistoryRecord
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.data.SkipDefaults
import com.jerocine.tv.player.buildNativePlayerPayload
import com.jerocine.tv.player.launchNativePlayer
import com.jerocine.tv.ui.groupHistoryForTv
import com.jerocine.tv.ui.flattenHistoryRows
import kotlinx.coroutines.launch

class HistoryFragment : Fragment(R.layout.fragment_history) {
    private val historyAdapter = HistoryAdapter(::handleHistoryClick)
    private var items = emptyList<HistoryItem>()
    private var manageMode = false
    private var contentRevealed = false

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val list = view.findViewById<RecyclerView>(R.id.history_list)
        val manage = view.findViewById<Button>(R.id.history_manage)
        val clear = view.findViewById<Button>(R.id.history_clear)
        val login = view.findViewById<Button>(R.id.history_login)
        val back = view.findViewById<ImageButton>(R.id.history_back)
        contentRevealed = false
        val layoutManager = GridLayoutManager(requireContext(), COLUMN_COUNT)
        layoutManager.spanSizeLookup = object : GridLayoutManager.SpanSizeLookup() {
            override fun getSpanSize(position: Int): Int =
                if (historyAdapter.getItemViewType(position) == HistoryAdapter.TYPE_HEADER) COLUMN_COUNT else 1
        }
        list.layoutManager = layoutManager
        list.adapter = historyAdapter
        list.itemAnimator = null

        listOf<View>(back, login, manage, clear).forEach { control ->
            control.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        }
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }
        login.isVisible = !ServiceLocator.isLoggedIn
        login.setOnClickListener { (requireActivity() as MainActivity).openLogin() }
        manage.setOnClickListener {
            manageMode = !manageMode
            historyAdapter.manageMode = manageMode
            manage.text = if (manageMode) "完成" else "管理"
            manage.isSelected = manageMode
        }
        clear.setOnClickListener { clearHistory() }
        loadHistory()
    }

    private fun loadHistory() {
        showLoading(true)
        viewLifecycleOwner.lifecycleScope.launch {
            items = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.historyList(page = 1, size = 60)
            } else {
                ServiceLocator.tokenStore.localHistories().map { it.toHistoryItem() }
            }
            bindItems()
            showLoading(false)
        }
    }

    private fun bindItems() {
        val view = view ?: return
        val rows = flattenHistoryRows(groupHistoryForTv(items))
        historyAdapter.submitList(rows) {
            if (!contentRevealed) {
                contentRevealed = true
                val target = if (items.isEmpty()) {
                    view.findViewById<View>(R.id.history_message)
                } else {
                    view.findViewById<View>(R.id.history_list)
                }
                target.revealTvContent { ServiceLocator.tokenStore.reduceMotion }
            }
        }
        view.findViewById<TextView>(R.id.history_subtitle).text = if (ServiceLocator.isLoggedIn) {
            "云端进度 · 跨设备同步 · 共 ${items.size} 条"
        } else {
            "本地历史 · 仅当前设备 · 共 ${items.size} 条"
        }
        view.findViewById<TextView>(R.id.history_message).apply {
            text = if (items.isEmpty()) "还没有观看记录" else ""
            isVisible = items.isEmpty()
        }
        view.findViewById<RecyclerView>(R.id.history_list).isVisible = items.isNotEmpty()
        view.findViewById<Button>(R.id.history_manage).isVisible = items.isNotEmpty()
        view.findViewById<Button>(R.id.history_clear).isVisible = items.isNotEmpty()
        if (items.isEmpty()) {
            manageMode = false
            historyAdapter.manageMode = false
            view.findViewById<Button>(R.id.history_manage).apply {
                text = "管理"
                isSelected = false
            }
        }
    }

    private fun handleHistoryClick(history: HistoryItem) {
        if (manageMode) deleteHistory(history) else startPlayback(history)
    }

    private fun deleteHistory(history: HistoryItem) {
        viewLifecycleOwner.lifecycleScope.launch {
            val deleted = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.historyDelete(history.mid)
            } else {
                ServiceLocator.tokenStore.deleteLocalHistory(history.mid)
            }
            if (deleted) {
                items = items.filterNot { it.mid == history.mid }
                bindItems()
            }
        }
    }

    private fun clearHistory() {
        viewLifecycleOwner.lifecycleScope.launch {
            val cleared = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.historyClear()
            } else {
                ServiceLocator.tokenStore.clearLocalHistory()
            }
            if (cleared) {
                items = emptyList()
                manageMode = false
                historyAdapter.manageMode = false
                bindItems()
            }
        }
    }

    private fun startPlayback(history: HistoryItem) {
        val status = view?.findViewById<TextView>(R.id.history_play_status) ?: return
        status.text = "正在准备播放..."
        status.isVisible = true
        viewLifecycleOwner.lifecycleScope.launch {
            runCatching {
                val info = ServiceLocator.repository.playInfo(
                    history.mid,
                    history.source().ifBlank { null },
                    history.episodeNumber(),
                )
                val source = history.source().ifBlank { info.currentSource }
                val selected = info.detail.sources.firstOrNull { it.id == source }
                    ?: info.detail.sources.firstOrNull { it.id == info.currentSource }
                    ?: info.detail.sources.firstOrNull()
                val isSeries = selected != null && selected.episodes.size > 1
                val remoteSkip = if (ServiceLocator.isLoggedIn) {
                    ServiceLocator.userRepository.skipFor(history.mid)
                } else {
                    null
                }
                val skip = remoteSkip ?: ServiceLocator.tokenStore.localSkipFor(history.mid)
                buildNativePlayerPayload(
                    info = info,
                    requestedSource = source,
                    requestedEpisode = history.episodeNumber(),
                    skipIntroSec = skip?.intro ?: if (isSeries) SkipDefaults.INTRO else 0,
                    skipOutroSec = skip?.outro ?: if (isSeries) SkipDefaults.OUTRO else 0,
                    proxyBase = ServiceLocator.proxyBase(),
                    resumeSec = history.resumeSeconds(),
                ) ?: error("播放地址为空")
            }.onSuccess { payload ->
                if (!ServiceLocator.isLoggedIn) {
                    val card = history.previewCard()
                    ServiceLocator.tokenStore.upsertLocalHistory(
                        LocalHistoryRecord(
                            mid = history.mid,
                            source = payload.currentSourceId,
                            episodeIndex = payload.startIndex,
                            progress = history.resumeSeconds(),
                            duration = history.duration,
                            name = card.name,
                            cover = card.cover,
                            cName = card.cName,
                            remarks = card.remarks,
                            updatedAt = System.currentTimeMillis(),
                        ),
                    )
                }
                status.isVisible = false
                launchNativePlayer(requireContext(), payload)
            }.onFailure {
                status.text = it.message ?: "播放失败"
            }
        }
    }

    private fun showLoading(loading: Boolean) {
        view?.findViewById<ProgressBar>(R.id.history_progress)?.isVisible = loading
        if (loading) {
            view?.findViewById<RecyclerView>(R.id.history_list)?.isVisible = false
            view?.findViewById<TextView>(R.id.history_message)?.isVisible = false
        }
    }

    companion object {
        private const val COLUMN_COUNT = 6
        fun newInstance() = HistoryFragment()
    }
}
