package com.jerocine.tv

import android.content.Context
import android.os.Bundle
import android.os.SystemClock
import android.text.InputType
import android.view.ViewGroup
import android.view.inputmethod.InputMethodManager
import android.widget.EditText
import android.widget.LinearLayout
import androidx.activity.OnBackPressedCallback
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.lifecycleScope
import com.jerocine.player.ui.GlassToast
import com.jerocine.player.ui.JcDeviceDiagnostics
import com.jerocine.player.ui.JcDialog
import com.jerocine.tv.data.HistoryReq
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.player.data.ServerUrl
import com.jerocine.player.data.ShellPrefs
import com.jerocine.tv.data.isPlayerHistoryEvent
import com.jerocine.player.JerocinePlayer
import com.jerocine.tv.ui.reduceMotionLabel
import com.jerocine.tv.ui.view.DetailFragment
import com.jerocine.tv.ui.view.HomeFragment
import com.jerocine.tv.ui.view.SearchFragment
import com.jerocine.tv.ui.view.CategoryFragment
import com.jerocine.tv.ui.view.CategoryLibraryFragment
import com.jerocine.tv.ui.view.LoginFragment
import com.jerocine.tv.ui.view.HistoryFragment
import com.jerocine.tv.ui.view.FavoritesFragment
import com.jerocine.player.ui.SettingsDrawerController
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import com.jerocine.player.R as PlayerR

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
                    GlassToast.show(this@NativeShellActivity, "再按一次退出应用")
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
                getSharedPreferences(ShellPrefs.NAME, MODE_PRIVATE),
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

    /**
     * 检查更新 —— 原生壳没有自升级通道(下载/安装走 web 壳的 UpdateChecker),
     * 这里只查服务端最新版本并如实告知, 与抽屉化之前的全页设置页行为一致。
     */
    override fun onCheckUpdate() {
        lifecycleScope.launch {
            val text = runCatching { ServiceLocator.repository.latestVersion() }
                .fold(
                    onSuccess = { v ->
                        "最新版本 ${v.versionName} (${v.versionCode})" + if (v.force) " · 强制更新" else ""
                    },
                    onFailure = { "检查失败：${it.message ?: "网络异常"}" }
                )
            JcDialog.message(this@NativeShellActivity)
                .title("检查更新")
                .message(text)
                .confirmButton("知道了")
                .show()
        }
    }

    /** 设备诊断 —— 共用 core 的一份实现(单包双壳同一套字段与判定, 细则 §5.1) */
    override fun onDeviceDiagnostics() {
        JcDeviceDiagnostics.show(this)
    }

    /**
     * 服务器地址编辑 —— 原生壳的**唯一**后端切换入口(低配机跑不了 web 壳,
     * 之前的实现只是提示"请去 web 壳改", 等于把入口关了)。
     * 保存走 ServiceLocator.setServer, 它会同时写 TokenStore 与壳层共享 prefs,
     * 保证切壳模式后两壳连的是同一个后端。
     */
    override fun onServerUrlEdit() {
        val density = resources.displayMetrics.density
        fun dp(v: Int) = (v * density).toInt()
        val input = EditText(this).apply {
            setText(ServiceLocator.serverBase)
            setSelection(text.length)
            hint = "https://example.com"
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI
            isSingleLine = true
            setTextColor(androidx.core.content.ContextCompat.getColor(
                this@NativeShellActivity, PlayerR.color.jc_text))
            setHintTextColor(androidx.core.content.ContextCompat.getColor(
                this@NativeShellActivity, PlayerR.color.jc_text_tertiary))
            setBackgroundResource(R.drawable.jc_input)
            setPadding(dp(16), 0, dp(16), 0)
        }
        val wrap = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            // 与 JcDialog 标题左对齐(内边距来自主题 dialogPreferredPadding, 不散写 24dp)
            val padX = JcDialog.contentPaddingX(this@NativeShellActivity)
            setPadding(padX, dp(8), padX, 0)
            addView(input, LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, dp(58)))
        }
        JcDialog.custom(this)
            .title("服务器地址")
            .content(wrap)
            .confirmButton("保存", Runnable {
                val url = ServerUrl.normalize(input.text.toString())
                if (url.isBlank()) {
                    toast("地址不能为空")
                    return@Runnable
                }
                runCatching { ServiceLocator.setServer(withScheme(url)) }
                    .onSuccess { toast("已保存：${ServiceLocator.serverBase}") }
                    .onFailure { toast(it.message ?: "保存失败") }
            })
            .cancelButton("取消")
            .show()
        // 原生壳没有 web 壳那样的地址栏, 弹窗出来直接拉起软键盘(遥控器/触控都要能输入)
        input.requestFocus()
        input.postDelayed({
            (getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager)
                ?.showSoftInput(input, 0)
        }, 200)
    }

    /** 缺协议头时补 http:// —— 与 web 壳 promptServerUrl 的归一化保持一致 */
    private fun withScheme(url: String): String =
        if (url.startsWith("http://") || url.startsWith("https://")) url else "http://$url"

    override fun getServerUrl(): String = ServiceLocator.serverBase

    override fun onAccountAction() {
        if (ServiceLocator.isLoggedIn) {
            ServiceLocator.saveToken(null)
            GlassToast.show(this, "已退出登录")
        } else {
            openLogin()
        }
    }

    override fun getAccountInfo(): SettingsDrawerController.AccountInfo {
        // 文案拼装一律走 core 的工厂(AccountInfo.loggedIn/loggedOut) —— 两壳账号行必须同源,
        // 否则同一个抽屉在两种模式下显示不同的字。这里只喂"登录态 + 昵称"。
        // 原生壳自登录有昵称; web 壳登录只存了 token(昵称在前端 localStorage) ⇒ 传空退回"已登录"。
        return if (ServiceLocator.isLoggedIn) {
            SettingsDrawerController.AccountInfo.loggedIn(ServiceLocator.tokenStore.userName ?: "")
        } else {
            SettingsDrawerController.AccountInfo.loggedOut()
        }
    }

    override fun hasDisplayMode(): Boolean = false
    override fun onCycleDisplayMode() {}
    override fun getDisplayModeLabel(): String = ""

    // ---- 减少动效(原生壳特有; 抽屉里的唯一入口) ----
    override fun hasMotionRow(): Boolean = true
    override fun getMotionLabel(): String =
        reduceMotionLabel(ServiceLocator.tokenStore.reduceMotion)

    /** 三档循环: 自动 → 减少 → 完整 → 自动; 与全页设置页的三选一写的是同一个键 */
    override fun onCycleMotion() {
        val next = when (ServiceLocator.tokenStore.reduceMotion) {
            "on" -> "off"
            "off" -> "auto"
            else -> "on"
        }
        ServiceLocator.tokenStore.reduceMotion = next
        toast("动画已设为${reduceMotionLabel(next)}")
    }

    override fun toast(msg: String) {
        // 与 web 壳同一份玻璃 Toast(下沉自 web 壳, 细则 §5.1): 两壳轻提示观感一致
        GlassToast.show(this, msg)
    }

    override fun onShellModeChanged(mode: String) {
        toast("壳模式已设为 ${SettingsDrawerController.shellModeLabel(mode)}，重启后生效")
    }

    override fun getCurrentShellMode(): String =
        getSharedPreferences(ShellPrefs.NAME, MODE_PRIVATE)
            .getString(ShellPrefs.KEY_SHELL_MODE, "auto") ?: "auto"

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
