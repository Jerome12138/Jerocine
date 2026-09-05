package com.jerocine.tv.ui.view

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.TextView
import androidx.core.view.isVisible
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import coil.dispose
import com.jerocine.tv.R
import com.jerocine.tv.data.Card
import com.jerocine.tv.data.HistoryItem
import com.jerocine.tv.data.ServiceLocator

data class HomeHeaderContent(
    val hero: Card?,
    val recent: List<HistoryItem>,
    val firstPid: Long,
    val loggedIn: Boolean,
)

class HomeDashboardHeaderAdapter(
    private val onPosterClick: (Long) -> Unit,
    private val onHistory: () -> Unit,
    private val onCategory: (Long) -> Unit,
    private val onSearch: () -> Unit,
    private val onAccount: (Boolean) -> Unit,
    private val onSettings: () -> Unit,
) : RecyclerView.Adapter<HomeDashboardHeaderAdapter.ViewHolder>() {
    private var content: HomeHeaderContent? = null

    init {
        setHasStableIds(true)
    }

    fun submit(value: HomeHeaderContent) {
        val hadContent = content != null
        content = value
        if (hadContent) notifyItemChanged(0) else notifyItemInserted(0)
    }

    override fun getItemCount() = if (content == null) 0 else 1
    override fun getItemId(position: Int) = Long.MIN_VALUE

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): ViewHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_home_dashboard_header, parent, false)
        return ViewHolder(
            view,
            onPosterClick,
            onHistory,
            onCategory,
            onSearch,
            onAccount,
            onSettings,
        )
    }

    override fun onBindViewHolder(holder: ViewHolder, position: Int) {
        content?.let(holder::bind)
    }

    override fun onViewRecycled(holder: ViewHolder) = holder.recycle()

    fun requestInitialFocus(outer: RecyclerView) {
        outer.scrollToPosition(0)
        outer.post {
            (outer.findViewHolderForAdapterPosition(0) as? ViewHolder)?.requestInitialFocus()
        }
    }

    class ViewHolder(
        itemView: View,
        private val onPosterClick: (Long) -> Unit,
        onHistory: () -> Unit,
        private val onCategory: (Long) -> Unit,
        onSearch: () -> Unit,
        private val onAccount: (Boolean) -> Unit,
        onSettings: () -> Unit,
    ) : RecyclerView.ViewHolder(itemView) {
        private val recentPanel = itemView.findViewById<View>(R.id.home_recent_panel)
        private val recentList = itemView.findViewById<RecyclerView>(R.id.home_recent_list)
        private val recentAdapter = HomeRecentAdapter(onPosterClick)
        private val hero = itemView.findViewById<View>(R.id.home_hero)
        private val heroImage = itemView.findViewById<ImageView>(R.id.home_hero_image)
        private val heroTitle = itemView.findViewById<TextView>(R.id.home_hero_title)
        private val heroMeta = itemView.findViewById<TextView>(R.id.home_hero_meta)
        private val history = itemView.findViewById<View>(R.id.home_quick_history)
        private val category = itemView.findViewById<View>(R.id.home_quick_category)
        private val search = itemView.findViewById<View>(R.id.home_quick_search)
        private val account = itemView.findViewById<View>(R.id.home_quick_account)
        private val accountSubtitle = itemView.findViewById<TextView>(R.id.home_quick_account_subtitle)
        private val settings = itemView.findViewById<View>(R.id.home_quick_settings)
        private var heroMid = 0L
        private var firstPid = 0L
        private var loggedIn = false

        init {
            recentList.layoutManager = LinearLayoutManager(itemView.context, RecyclerView.HORIZONTAL, false)
            recentList.adapter = recentAdapter
            recentList.itemAnimator = null
            listOf(hero, history, category, search, account, settings).forEach { control ->
                control.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
            }
            hero.setOnClickListener { if (heroMid > 0) onPosterClick(heroMid) }
            history.setOnClickListener { onHistory() }
            category.setOnClickListener { if (firstPid > 0) onCategory(firstPid) }
            search.setOnClickListener { onSearch() }
            account.setOnClickListener { onAccount(loggedIn) }
            settings.setOnClickListener { onSettings() }
        }

        fun bind(value: HomeHeaderContent) {
            recentPanel.isVisible = value.recent.isNotEmpty()
            recentAdapter.submitList(value.recent.take(3))
            firstPid = value.firstPid
            loggedIn = value.loggedIn
            category.isEnabled = value.firstPid > 0
            accountSubtitle.text = if (value.loggedIn) "账号 · 退出" else "点击登录"

            val card = value.hero
            hero.isVisible = card != null
            heroMid = card?.mid ?: 0L
            hero.tag = heroMid
            heroTitle.text = card?.name.orEmpty()
            heroMeta.text = card?.let {
                listOfNotNull(
                    it.year.takeIf { year -> year > 0 }?.toString(),
                    it.area.ifBlank { null },
                    it.cName.ifBlank { null },
                    it.remarks.ifBlank { null },
                ).joinToString(" · ")
            }.orEmpty()
            if (card != null) heroImage.loadTvBackdrop(card.cover) else heroImage.setImageDrawable(null)
        }

        fun requestInitialFocus() {
            if (hero.isVisible) hero.requestFocus() else history.requestFocus()
        }

        fun recycle() {
            heroImage.dispose()
            heroImage.setImageDrawable(null)
        }
    }
}
