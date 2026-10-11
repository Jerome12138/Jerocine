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
import com.jerocine.player.JerocinePlayer
import com.jerocine.tv.ui.view.DetailFragment
import com.jerocine.tv.ui.view.HomeFragment
import com.jerocine.tv.ui.view.SearchFragment
import com.jerocine.tv.ui.view.CategoryFragment
import com.jerocine.tv.ui.view.CategoryLibraryFragment
import com.jerocine.tv.ui.view.LoginFragment
import com.jerocine.tv.ui.view.HistoryFragment
import com.jerocine.tv.ui.view.FavoritesFragment
import com.jerocine.tv.ui.SettingsDrawerController
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

class NativeShellActivity : FragmentActivity(), SettingsDrawerController.Host {
    private val exitGate = DoubleBackExitGate()
    private var settingsDrawer: SettingsDrawerController? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // 底部系统导航栏(手势条/三键条)与页面背景同色 — 装平板/手机时避免黑白条突兀
        window.navigationBarColor =
            androidx.core.content.ContextCompat.getColor(this, com.jerocine.player.R.color.jc_bg)
        setContentView(R.layout.activity_native_shell)
        JerocinePlayer.config().setCallback(::handlePlayerEvent)
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                // 抽屉打开时, BACK 仅关抽屉
                if (settingsDrawer?.isShowing == true) {
                    settingsDrawer?.hide()
                    return
                }
                if (supportFragmentManager.backStackEntryCount > 0) {
                    supportFragmentManager.popBackStack()
                    return
                }
                if (exitGate.shouldExit(SystemClock.elapsedRealtime())) {
                    finish()
                } else {
                    Toast.makeText(this@NativeShellActivity, "再按一次退出应用", Toast.LENGTH_SHORT).show()
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
        if (settingsDrawer == null) {
            settingsDrawer = SettingsDrawerController(
                this,
                getSharedPreferences("jerocine_prefs", MODE_PRIVATE),
                this
            )
        }
        settingsDrawer?.show()
    }

    /**
     * MENU 键 → 切换设置抽屉(与 web 壳同语义)
     */
    override fun dispatchKeyEvent(event: android.view.KeyEvent): Boolean {
        if (event.keyCode == android.view.KeyEvent.KEYCODE_MENU
            && event.action == android.view.KeyEvent.ACTION_DOWN
        ) {
            if (settingsDrawer?.isShowing == true) settingsDrawer?.hide()
            else openSettings()
            return true
        }
        return super.dispatchKeyEvent(event)
    }

    // ==================== SettingsDrawerController.Host 实现 ====================

    override fun onRefreshPage() {
        // 原生壳: 重建当前 fragment 以刷新数据
        val top = supportFragmentManager.fragments.lastOrNull()
        if (top != null) {
            supportFragmentManager.beginTransaction()
                .detach(top)
                .attach(top)
                .commit()
        }
    }

    override fun onExitApp() { finishAffinity() }

    override fun onCheckUpdate() {
        Toast.makeText(this, "原生壳暂未集成检查更新", Toast.LENGTH_SHORT).show()
    }

    override fun onDeviceDiagnostics() {
        val info = com.jerocine.tv.ui.DeviceDiagnostics.collect(this)
        androidx.appcompat.app.AlertDialog.Builder(this)
            .setTitle("设备诊断")
            .setMessage(info.toText())
            .setPositiveButton("关闭", null)
            .show()
    }

    override fun onServerUrlEdit() {
        Toast.makeText(this, "服务器地址修改请在 web 壳设置中操作", Toast.LENGTH_SHORT).show()
    }

    override fun getServerUrl(): String = ServiceLocator.serverBase

    override fun onAccountAction() {
        if (ServiceLocator.isLoggedIn) {
            ServiceLocator.saveToken(null)
            Toast.makeText(this, "已退出登录", Toast.LENGTH_SHORT).show()
        } else {
            openLogin()
        }
    }

    override fun getAccountInfo(): SettingsDrawerController.AccountInfo {
        return if (ServiceLocator.isLoggedIn) {
            SettingsDrawerController.AccountInfo(
                "已登录",
                "收藏 / 历史 / 跳过设置多端同步",
                "退出",
                true
            )
        } else {
            SettingsDrawerController.AccountInfo.loggedOut()
        }
    }

    override fun hasDisplayMode(): Boolean = false
    override fun onCycleDisplayMode() {}
    override fun getDisplayModeLabel(): String = ""

    override fun onShellModeChanged(mode: String) {
        Toast.makeText(
            this,
            "壳模式已设为 ${SettingsDrawerController.shellModeLabel(mode)}，重启后生效",
            Toast.LENGTH_SHORT
        ).show()
    }

    override fun getCurrentShellMode(): String =
        getSharedPreferences("jerocine_prefs", MODE_PRIVATE)
            .getString(SettingsDrawerController.KEY_SHELL_MODE, "auto") ?: "auto"

    override fun onDestroy() {
        JerocinePlayer.config().setCallback(null)
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
