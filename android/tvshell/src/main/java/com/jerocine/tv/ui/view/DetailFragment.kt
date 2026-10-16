package com.jerocine.tv.ui.view

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.ImageButton
import android.widget.ImageView
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
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.R
import com.jerocine.tv.data.FilmDetail
import com.jerocine.tv.data.HistoryItem
import com.jerocine.tv.data.LocalFavoriteRecord
import com.jerocine.tv.data.LocalHistoryRecord
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.data.SkipDefaults
import com.jerocine.tv.data.detailResumeHistory
import com.jerocine.tv.player.PlaybackCoordinator
import com.jerocine.tv.player.launchNativePlayer
import com.jerocine.tv.ui.DetailPlaybackSelection
import com.jerocine.tv.ui.DetailViewModel
import com.jerocine.tv.ui.UiState
import com.jerocine.tv.ui.detailEpisodeLabel
import com.jerocine.tv.ui.detailEpisodeSegments
import com.jerocine.tv.ui.deriveDetailTvModel
import com.jerocine.tv.ui.initialDetailPlaybackSelection
import com.jerocine.tv.ui.switchDetailSource
import kotlinx.coroutines.launch
import kotlin.math.ceil

class DetailFragment : Fragment(R.layout.fragment_detail) {
    private val viewModel: DetailViewModel by viewModels()
    private val mid: Long get() = requireArguments().getLong(ARG_MID)
    private val playbackCoordinator by lazy {
        PlaybackCoordinator { filmId, source, episode ->
            ServiceLocator.repository.playInfo(filmId, source, episode)
        }
    }

    private var detail: FilmDetail? = null
    private var resumeSource = ""
    private var resumeEpisode = 0
    private var resumeSec = 0.0
    private var favorite = false
    private var selection = DetailPlaybackSelection("", 0, 0)
    private var selectionTouched = false
    private lateinit var sourceAdapter: DetailChoiceAdapter
    private lateinit var segmentAdapter: DetailChoiceAdapter
    private lateinit var episodeAdapter: DetailChoiceAdapter

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val content = view.findViewById<View>(R.id.detail_content)
        val progress = view.findViewById<ProgressBar>(R.id.detail_progress)
        val error = view.findViewById<TextView>(R.id.detail_error)
        val retry = view.findViewById<Button>(R.id.detail_retry)
        val back = view.findViewById<ImageButton>(R.id.detail_back)
        val play = view.findViewById<Button>(R.id.detail_play)
        val favoriteButton = view.findViewById<Button>(R.id.detail_favorite)
        var contentRevealed = false

        setupPlaybackChoices(view)
        listOf<View>(back, retry, play, favoriteButton).forEach { control ->
            control.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        }
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }
        retry.setOnClickListener { viewModel.load(mid) }
        viewModel.load(mid)

        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                viewModel.state.collect { state ->
                    when (state) {
                        UiState.Loading -> {
                            progress.isVisible = true
                            content.isVisible = false
                            error.isVisible = false
                            retry.isVisible = false
                        }
                        is UiState.Error -> {
                            progress.isVisible = false
                            content.isVisible = false
                            error.text = state.message
                            error.isVisible = true
                            retry.isVisible = true
                        }
                        is UiState.Success -> {
                            progress.isVisible = false
                            error.isVisible = false
                            retry.isVisible = false
                            content.isVisible = true
                            bindDetail(view, state.data.detail)
                            if (!contentRevealed) {
                                contentRevealed = true
                                content.revealTvContent { ServiceLocator.tokenStore.reduceMotion }
                            }
                        }
                    }
                }
            }
        }
    }

    private fun bindDetail(view: View, value: FilmDetail) {
        detail = value
        if (!selectionTouched) {
            selection = initialDetailPlaybackSelection(playableSources(value), resumeSource, resumeEpisode)
        }
        val model = deriveDetailTvModel(value)
        view.findViewById<ImageView>(R.id.detail_backdrop).loadTvBackdrop(value.cover)
        view.findViewById<ImageView>(R.id.detail_poster).loadTvPoster(value.cover, value.name)
        view.findViewById<TextView>(R.id.detail_title).text = value.name
        view.findViewById<TextView>(R.id.detail_meta).text = model.chips.joinToString(" · ")
        view.findViewById<TextView>(R.id.detail_people).text = listOfNotNull(
            value.director.ifBlank { null }?.let { "导演：$it" },
            value.actor.ifBlank { null }?.let { "主演：$it" },
        ).joinToString("\n")
        view.findViewById<TextView>(R.id.detail_summary).text = model.summary.ifBlank { "暂无简介" }
        renderPlaybackChoices(view)

        val play = view.findViewById<Button>(R.id.detail_play)
        val favoriteButton = view.findViewById<Button>(R.id.detail_favorite)
        play.setOnClickListener { startPlayback(view) }
        favoriteButton.setOnClickListener { toggleFavorite(view, value) }
        play.requestFocus()

        viewLifecycleOwner.lifecycleScope.launch {
            val remoteHistories = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.historyList(page = 1, size = 60)
            } else {
                emptyList()
            }
            applyResume(
                view,
                detailResumeHistory(
                    mid = value.mid,
                    isLoggedIn = ServiceLocator.isLoggedIn,
                    remoteHistories = remoteHistories,
                    localHistories = ServiceLocator.tokenStore.localHistories(),
                ),
                value,
                play,
            )
            favorite = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.isFavorite(value.mid)
            } else {
                ServiceLocator.tokenStore.isLocalFavorite(value.mid)
            }
            updateFavoriteButton(favoriteButton)
        }
    }

    private fun applyResume(view: View, history: HistoryItem?, detail: FilmDetail, play: Button) {
        if (selectionTouched) return
        resumeSource = history?.source()?.takeIf { source ->
            detail.sources.any { it.id == source }
        }.orEmpty()
        resumeEpisode = history?.episodeNumber() ?: 0
        resumeSec = history?.resumeSeconds() ?: 0.0
        selection = initialDetailPlaybackSelection(
            playableSources(detail),
            resumeSource,
            resumeEpisode,
        )
        renderPlaybackChoices(view)
        play.text = if (resumeSec > 0) "继续播放" else "播放"
    }

    private fun startPlayback(view: View) {
        val value = detail ?: return
        val play = view.findViewById<Button>(R.id.detail_play)
        val loading = view.findViewById<ProgressBar>(R.id.detail_play_progress)
        val error = view.findViewById<TextView>(R.id.detail_play_error)
        val source = selection.sourceId.ifBlank { value.sources.firstOrNull()?.id.orEmpty() }
        val episode = selection.episodeIndex
        val sourceInfo = value.sources.firstOrNull { it.id == source } ?: value.sources.firstOrNull()
        val isSeries = sourceInfo != null && sourceInfo.episodes.size > 1

        play.isEnabled = false
        loading.isVisible = true
        error.isVisible = false
        viewLifecycleOwner.lifecycleScope.launch {
            val skip = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.skipFor(value.mid)
            } else {
                ServiceLocator.tokenStore.localSkipFor(value.mid)
            }
            runCatching {
                playbackCoordinator.prepare(
                    mid = value.mid,
                    source = source,
                    episode = episode,
                    resumeSec = resumeSec,
                    skipIntroSec = skip?.intro ?: if (isSeries) SkipDefaults.INTRO else 0,
                    skipOutroSec = skip?.outro ?: if (isSeries) SkipDefaults.OUTRO else 0,
                    proxyBase = ServiceLocator.proxyBase(),
                )
            }.onSuccess { payload ->
                if (!ServiceLocator.isLoggedIn) {
                    ServiceLocator.tokenStore.upsertLocalHistory(
                        LocalHistoryRecord(
                            mid = value.mid,
                            source = payload.currentSourceId,
                            episodeIndex = payload.startIndex,
                            progress = resumeSec,
                            name = value.name,
                            cover = value.cover,
                            cName = value.cName,
                            remarks = value.remarks,
                            updatedAt = System.currentTimeMillis(),
                        ),
                    )
                }
                launchNativePlayer(requireContext(), payload)
            }.onFailure { throwable ->
                error.text = throwable.message ?: "播放失败"
                error.isVisible = true
            }
            loading.isVisible = false
            play.isEnabled = true
        }
    }

    private fun toggleFavorite(view: View, value: FilmDetail) {
        val button = view.findViewById<Button>(R.id.detail_favorite)
        button.isEnabled = false
        viewLifecycleOwner.lifecycleScope.launch {
            favorite = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.toggleFavorite(value.mid)
            } else {
                ServiceLocator.tokenStore.toggleLocalFavorite(
                    LocalFavoriteRecord(
                        mid = value.mid,
                        name = value.name,
                        cover = value.cover,
                        cName = value.cName,
                        remarks = value.remarks,
                        createdAt = System.currentTimeMillis(),
                    ),
                )
            }
            updateFavoriteButton(button)
            button.isEnabled = true
        }
    }

    private fun updateFavoriteButton(button: Button) {
        button.text = if (favorite) "已收藏" else "收藏"
        button.isSelected = favorite
    }

    private fun setupPlaybackChoices(view: View) {
        sourceAdapter = DetailChoiceAdapter(R.layout.item_detail_choice) { sourceId ->
            val value = detail ?: return@DetailChoiceAdapter
            selectionTouched = true
            selection = switchDetailSource(
                playableSources(value),
                sourceId,
                selection.episodeIndex,
            )
            resumeSource = selection.sourceId
            resumeEpisode = selection.episodeIndex
            renderPlaybackChoices(view)
        }
        segmentAdapter = DetailChoiceAdapter(R.layout.item_detail_choice) { segment ->
            selectionTouched = true
            selection = selection.copy(segmentIndex = segment.toIntOrNull() ?: 0)
            renderPlaybackChoices(view)
        }
        episodeAdapter = DetailChoiceAdapter(R.layout.item_detail_episode) { episode ->
            selectionTouched = true
            resumeSec = 0.0
            selection = selection.copy(episodeIndex = episode.toIntOrNull() ?: 0)
            resumeSource = selection.sourceId
            resumeEpisode = selection.episodeIndex
            view.findViewById<Button>(R.id.detail_play).text = "播放"
            renderPlaybackChoices(view)
        }

        view.findViewById<RecyclerView>(R.id.detail_source_list).apply {
            layoutManager = LinearLayoutManager(context, RecyclerView.HORIZONTAL, false)
            adapter = sourceAdapter
            itemAnimator = null
        }
        view.findViewById<RecyclerView>(R.id.detail_segment_list).apply {
            layoutManager = LinearLayoutManager(context, RecyclerView.HORIZONTAL, false)
            adapter = segmentAdapter
            itemAnimator = null
        }
        view.findViewById<RecyclerView>(R.id.detail_episode_list).apply {
            layoutManager = GridLayoutManager(context, EPISODE_COLUMNS)
            adapter = episodeAdapter
            itemAnimator = null
        }
    }

    private fun renderPlaybackChoices(view: View) {
        val value = detail ?: return
        val sources = playableSources(value)
        val panel = view.findViewById<View>(R.id.detail_playback_choices)
        val play = view.findViewById<Button>(R.id.detail_play)
        panel.isVisible = sources.isNotEmpty()
        play.isEnabled = sources.isNotEmpty()
        if (sources.isEmpty()) return

        if (sources.none { it.id == selection.sourceId }) {
            selection = initialDetailPlaybackSelection(sources, resumeSource, resumeEpisode)
        }
        val source = sources.first { it.id == selection.sourceId }
        val safeEpisode = selection.episodeIndex.coerceIn(0, source.episodes.lastIndex)
        val segments = detailEpisodeSegments(source.episodes.size)
        val safeSegment = if (segments.isEmpty()) 0 else {
            selection.segmentIndex.coerceIn(0, segments.lastIndex)
        }
        selection = selection.copy(episodeIndex = safeEpisode, segmentIndex = safeSegment)

        sourceAdapter.submitList(sources.map { item ->
            DetailChoiceItem(
                key = item.id,
                label = "${item.name.ifBlank { item.id }} · ${item.episodes.size}",
                selected = item.id == selection.sourceId,
            )
        })

        val segmentList = view.findViewById<RecyclerView>(R.id.detail_segment_list)
        segmentList.isVisible = segments.isNotEmpty()
        segmentAdapter.submitList(segments.mapIndexed { index, item ->
            DetailChoiceItem(index.toString(), item.label, index == selection.segmentIndex)
        })

        val visibleRange = segments.getOrNull(selection.segmentIndex)?.let { it.start..it.end }
            ?: source.episodes.indices
        episodeAdapter.submitList(visibleRange.map { index ->
            DetailChoiceItem(
                key = index.toString(),
                label = detailEpisodeLabel(value.name, source.episodes[index].episode, index),
                selected = index == selection.episodeIndex,
            )
        })
        view.findViewById<TextView>(R.id.detail_episode_title).text =
            "选集 · ${detailEpisodeLabel(value.name, source.episodes[safeEpisode].episode, safeEpisode)}"

        val rows = ceil(visibleRange.count() / EPISODE_COLUMNS.toDouble()).toInt().coerceAtLeast(1)
        view.findViewById<RecyclerView>(R.id.detail_episode_list).layoutParams =
            view.findViewById<RecyclerView>(R.id.detail_episode_list).layoutParams.apply {
                height = (rows * EPISODE_ROW_HEIGHT_DP * resources.displayMetrics.density).toInt()
            }
    }

    private fun playableSources(value: FilmDetail) = value.sources.filter { it.episodes.isNotEmpty() }

    companion object {
        private const val ARG_MID = "mid"
        private const val EPISODE_COLUMNS = 6
        private const val EPISODE_ROW_HEIGHT_DP = 50

        fun newInstance(mid: Long) = DetailFragment().apply {
            arguments = bundleOf(ARG_MID to mid)
        }
    }
}
