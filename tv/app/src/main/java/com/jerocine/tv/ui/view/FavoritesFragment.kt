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
import com.jerocine.tv.data.FavoriteItem
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.ui.deriveFavoritesTvModel
import com.jerocine.tv.ui.favoriteCards
import kotlinx.coroutines.launch

class FavoritesFragment : Fragment(R.layout.fragment_favorites) {
    private val posterAdapter = PosterAdapter(::handlePosterClick)
    private var items = emptyList<FavoriteItem>()
    private var manageMode = false
    private var contentRevealed = false

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val list = view.findViewById<RecyclerView>(R.id.favorites_list)
        val login = view.findViewById<Button>(R.id.favorites_login)
        val manage = view.findViewById<Button>(R.id.favorites_manage)
        val back = view.findViewById<ImageButton>(R.id.favorites_back)
        contentRevealed = false
        list.layoutManager = GridLayoutManager(requireContext(), COLUMN_COUNT)
        list.adapter = posterAdapter
        list.itemAnimator = null

        listOf<View>(back, login, manage).forEach { control ->
            control.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        }
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }
        login.isVisible = !ServiceLocator.isLoggedIn
        login.setOnClickListener { (requireActivity() as MainActivity).openLogin() }
        manage.setOnClickListener {
            manageMode = !manageMode
            manage.text = if (manageMode) "完成" else "管理"
            manage.isSelected = manageMode
            view.findViewById<TextView>(R.id.favorites_status).apply {
                text = if (manageMode) "已进入管理模式，按确认键移除收藏" else ""
                isVisible = manageMode
            }
        }
        loadFavorites()
    }

    private fun loadFavorites() {
        view?.findViewById<ProgressBar>(R.id.favorites_progress)?.isVisible = true
        viewLifecycleOwner.lifecycleScope.launch {
            items = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.favoriteList(page = 1, size = 60)
            } else {
                ServiceLocator.tokenStore.localFavorites().map { it.toFavoriteItem() }
            }
            bindItems()
            view?.findViewById<ProgressBar>(R.id.favorites_progress)?.isVisible = false
        }
    }

    private fun bindItems() {
        val view = view ?: return
        if (items.isEmpty()) {
            manageMode = false
            view.findViewById<TextView>(R.id.favorites_status).isVisible = false
            view.findViewById<Button>(R.id.favorites_manage).apply {
                text = "管理"
                isSelected = false
            }
        }
        val model = deriveFavoritesTvModel(ServiceLocator.isLoggedIn, items.size)
        view.findViewById<TextView>(R.id.favorites_subtitle).text = model.subtitle
        posterAdapter.submitList(favoriteCards(items))
        view.findViewById<RecyclerView>(R.id.favorites_list).isVisible = items.isNotEmpty()
        view.findViewById<Button>(R.id.favorites_manage).isVisible = items.isNotEmpty()
        view.findViewById<TextView>(R.id.favorites_message).apply {
            text = if (items.isEmpty()) "还没有收藏内容\n在影片详情页点击「收藏」即可加入这里" else ""
            isVisible = items.isEmpty()
        }
        if (!contentRevealed) {
            contentRevealed = true
            val target = if (items.isEmpty()) {
                view.findViewById<View>(R.id.favorites_message)
            } else {
                view.findViewById<View>(R.id.favorites_list)
            }
            target.revealTvContent { ServiceLocator.tokenStore.reduceMotion }
        }
    }

    private fun handlePosterClick(mid: Long) {
        if (!manageMode) {
            (requireActivity() as MainActivity).openDetail(mid)
            return
        }
        viewLifecycleOwner.lifecycleScope.launch {
            val removed = if (ServiceLocator.isLoggedIn) {
                ServiceLocator.userRepository.favoriteRemove(mid)
            } else {
                ServiceLocator.tokenStore.deleteLocalFavorite(mid)
            }
            if (removed) {
                items = items.filterNot { it.mid == mid }
                bindItems()
            }
        }
    }

    companion object {
        private const val COLUMN_COUNT = 6
        fun newInstance() = FavoritesFragment()
    }
}
