package com.jerocine.tv

import android.os.Bundle
import android.os.SystemClock
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.lifecycleScope
import com.jerocine.tv.data.HistoryReq
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.data.isPlayerHistoryEvent
import com.jerocine.tv.player.PlayerActivity
import com.jerocine.tv.ui.view.DetailFragment
import com.jerocine.tv.ui.view.HomeFragment
import com.jerocine.tv.ui.view.SearchFragment
import com.jerocine.tv.ui.view.CategoryFragment
import com.jerocine.tv.ui.view.CategoryLibraryFragment
import com.jerocine.tv.ui.view.LoginFragment
import com.jerocine.tv.ui.view.HistoryFragment
import com.jerocine.tv.ui.view.FavoritesFragment
import com.jerocine.tv.ui.view.SettingsFragment
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

class MainActivity : FragmentActivity() {
    private val exitGate = DoubleBackExitGate()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        PlayerActivity.setEventListener(::handlePlayerEvent)
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (supportFragmentManager.backStackEntryCount > 0) {
                    supportFragmentManager.popBackStack()
                    return
                }
                if (exitGate.shouldExit(SystemClock.elapsedRealtime())) {
                    finish()
                } else {
                    Toast.makeText(this@MainActivity, "再按一次退出应用", Toast.LENGTH_SHORT).show()
                }
            }
        })
        supportFragmentManager.addOnBackStackChangedListener {
            if (supportFragmentManager.backStackEntryCount > 0) exitGate.reset()
        }

        if (savedInstanceState == null) {
            supportFragmentManager.beginTransaction()
                .replace(R.id.page_container, HomeFragment.newInstance())
                .commit()
        }
    }

    fun openDetail(mid: Long) {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, DetailFragment.newInstance(mid))
            .addToBackStack(DetailFragment::class.java.simpleName)
            .commit()
    }

    fun openSearch() {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, SearchFragment.newInstance())
            .addToBackStack(SearchFragment::class.java.simpleName)
            .commit()
    }

    fun openCategory(pid: Long) {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, CategoryFragment.newInstance(pid))
            .addToBackStack(CategoryFragment::class.java.simpleName)
            .commit()
    }

    fun openCategoryLibrary(pid: Long, categoryName: String, sort: String = "") {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, CategoryLibraryFragment.newInstance(pid, categoryName, sort))
            .addToBackStack(CategoryLibraryFragment::class.java.simpleName)
            .commit()
    }

    fun openLogin() {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, LoginFragment.newInstance())
            .addToBackStack(LoginFragment::class.java.simpleName)
            .commit()
    }

    fun openHistory() {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, HistoryFragment.newInstance())
            .addToBackStack(HistoryFragment::class.java.simpleName)
            .commit()
    }

    fun openFavorites() {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, FavoritesFragment.newInstance())
            .addToBackStack(FavoritesFragment::class.java.simpleName)
            .commit()
    }

    fun openSettings() {
        supportFragmentManager.beginTransaction()
            .replace(R.id.page_container, SettingsFragment.newInstance())
            .addToBackStack(SettingsFragment::class.java.simpleName)
            .commit()
    }

    override fun onDestroy() {
        PlayerActivity.setEventListener(null)
        super.onDestroy()
    }

    private fun handlePlayerEvent(name: String, payload: org.json.JSONObject) {
        if (name == "skipSettingChanged") {
            handleSkipSettingChanged(payload)
            return
        }
        if (!isPlayerHistoryEvent(name)) return
        val mid = payload.optString("filmId").toLongOrNull() ?: return
        val source = payload.optString("source", "")
        val episode = payload.optInt("episodeIndex", 0)
        val progress = payload.optDouble("position", 0.0).toInt().coerceAtLeast(0)
        val duration = payload.optDouble("duration", 0.0).toInt().coerceAtLeast(0)
        if (!ServiceLocator.isLoggedIn) {
            ServiceLocator.tokenStore.updateLocalHistory(
                mid = mid,
                source = source,
                episodeIndex = episode,
                progress = progress.toDouble(),
                duration = duration.toDouble(),
            )
            return
        }
        lifecycleScope.launch(Dispatchers.IO) {
            ServiceLocator.userRepository.upsertHistory(
                HistoryReq(
                    mid = mid,
                    playFrom = source,
                    episode = episode,
                    progress = progress,
                    duration = duration,
                ),
            )
        }
    }

    private fun handleSkipSettingChanged(payload: org.json.JSONObject) {
        val mid = payload.optString("filmId").toLongOrNull() ?: return
        val intro = payload.optInt("intro", 0).coerceAtLeast(0)
        val outro = payload.optInt("outro", 0).coerceAtLeast(0)
        ServiceLocator.tokenStore.saveLocalSkipSetting(mid, intro, outro)
        if (!ServiceLocator.isLoggedIn) return
        lifecycleScope.launch(Dispatchers.IO) {
            ServiceLocator.userRepository.saveSkipSetting(mid, intro, outro)
        }
    }
}
