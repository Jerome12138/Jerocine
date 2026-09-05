package com.jerocine.tv.ui.view

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.ImageButton
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.core.view.isVisible
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import com.jerocine.tv.BuildConfig
import com.jerocine.tv.MainActivity
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.ui.normalizeServerUrl
import com.jerocine.tv.ui.reduceMotionLabel
import kotlinx.coroutines.launch

class SettingsFragment : Fragment(R.layout.fragment_settings) {
    private val groupButtons = linkedMapOf<Int, String>()
    private val panels = linkedMapOf<Int, Int>()

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        groupButtons.putAll(
            linkedMapOf(
                R.id.settings_group_play to "播放",
                R.id.settings_group_ad to "广告过滤",
                R.id.settings_group_account to "账号",
                R.id.settings_group_device to "设备",
                R.id.settings_group_about to "关于",
            )
        )
        panels.putAll(
            linkedMapOf(
                R.id.settings_group_play to R.id.settings_panel_play,
                R.id.settings_group_ad to R.id.settings_panel_ad,
                R.id.settings_group_account to R.id.settings_panel_account,
                R.id.settings_group_device to R.id.settings_panel_device,
                R.id.settings_group_about to R.id.settings_panel_about,
            )
        )
        groupButtons.keys.forEach { id ->
            view.findViewById<Button>(id).apply {
                installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
                setOnClickListener { showGroup(id) }
            }
        }

        val back = view.findViewById<ImageButton>(R.id.settings_back)
        back.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }
        listOf(
            R.id.settings_motion_auto,
            R.id.settings_motion_reduce,
            R.id.settings_motion_full,
            R.id.settings_ad_toggle,
            R.id.settings_account_action,
            R.id.settings_server_save,
            R.id.settings_version_check,
        ).forEach { id ->
            view.findViewById<View>(id).installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        }

        bindReduceMotion(view)
        bindAdFilter(view)
        bindAccount(view)
        bindDevice(view)
        bindAbout(view)
        showGroup(R.id.settings_group_play)
    }

    private fun showGroup(selectedId: Int) {
        val root = view ?: return
        groupButtons.forEach { (id, label) ->
            root.findViewById<Button>(id).apply {
                text = label
                isSelected = id == selectedId
            }
        }
        panels.forEach { (id, panelId) ->
            val panel = root.findViewById<View>(panelId)
            panel.animate().cancel()
            if (id == selectedId) {
                panel.isVisible = true
                if (ServiceLocator.tokenStore.reduceMotion == "on") {
                    panel.alpha = 1f
                } else {
                    panel.alpha = 0f
                    panel.animate().alpha(1f).setDuration(140L).start()
                }
            } else {
                panel.isVisible = false
                panel.alpha = 1f
            }
        }
    }

    private fun bindReduceMotion(root: View) {
        val status = root.findViewById<TextView>(R.id.settings_motion_status)
        fun select(mode: String) {
            ServiceLocator.tokenStore.reduceMotion = mode
            status.text = "当前：${reduceMotionLabel(mode)}"
            root.findViewById<Button>(R.id.settings_motion_auto).isSelected = mode == "auto"
            root.findViewById<Button>(R.id.settings_motion_reduce).isSelected = mode == "on"
            root.findViewById<Button>(R.id.settings_motion_full).isSelected = mode == "off"
        }
        root.findViewById<Button>(R.id.settings_motion_auto).setOnClickListener { select("auto") }
        root.findViewById<Button>(R.id.settings_motion_reduce).setOnClickListener { select("on") }
        root.findViewById<Button>(R.id.settings_motion_full).setOnClickListener { select("off") }
        select(ServiceLocator.tokenStore.reduceMotion)
    }

    private fun bindAdFilter(root: View) {
        val toggle = root.findViewById<Button>(R.id.settings_ad_toggle)
        fun update() {
            toggle.text = if (ServiceLocator.tokenStore.adFilterEnabled) "已开启" else "已关闭"
            toggle.isSelected = ServiceLocator.tokenStore.adFilterEnabled
            root.findViewById<TextView>(R.id.settings_ad_status).text =
                if (ServiceLocator.tokenStore.adFilterEnabled) {
                    "m3u8 跨域 / 插片广告自动剔除"
                } else {
                    "播放时使用原始片源"
                }
        }
        toggle.setOnClickListener {
            ServiceLocator.tokenStore.adFilterEnabled = !ServiceLocator.tokenStore.adFilterEnabled
            update()
        }
        update()
    }

    private fun bindAccount(root: View) {
        val user = root.findViewById<TextView>(R.id.settings_account_user)
        val action = root.findViewById<Button>(R.id.settings_account_action)
        fun update() {
            user.text = ServiceLocator.tokenStore.userName ?: "未登录"
            action.text = if (ServiceLocator.isLoggedIn) "退出登录" else "登录"
            action.setBackgroundResource(
                if (ServiceLocator.isLoggedIn) R.drawable.gf_button_danger else R.drawable.gf_button_primary,
            )
            action.setTextColor(
                ContextCompat.getColorStateList(
                    requireContext(),
                    if (ServiceLocator.isLoggedIn) R.color.gf_danger_btn_text else R.color.gf_primary_btn_text,
                ),
            )
        }
        action.setOnClickListener {
            if (ServiceLocator.isLoggedIn) {
                ServiceLocator.logout()
                update()
            } else {
                (requireActivity() as MainActivity).openLogin()
            }
        }
        update()
    }

    private fun bindDevice(root: View) {
        val server = root.findViewById<EditText>(R.id.settings_server)
        val status = root.findViewById<TextView>(R.id.settings_server_status)
        server.setText(ServiceLocator.serverBase)
        root.findViewById<Button>(R.id.settings_server_save).setOnClickListener {
            val normalized = normalizeServerUrl(server.text.toString())
            if (normalized.isBlank()) {
                status.text = "服务器地址不能为空"
                return@setOnClickListener
            }
            runCatching { ServiceLocator.setServer(normalized) }
                .onSuccess {
                    server.setText(ServiceLocator.serverBase)
                    status.text = "已保存"
                    root.findViewById<TextView>(R.id.settings_about_server).text = ServiceLocator.serverBase
                }
                .onFailure { status.text = it.message ?: "保存失败" }
        }
    }

    private fun bindAbout(root: View) {
        root.findViewById<TextView>(R.id.settings_about_version).text =
            "${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE})"
        root.findViewById<TextView>(R.id.settings_about_build).text =
            if (BuildConfig.DEBUG) "debug" else "release"
        root.findViewById<TextView>(R.id.settings_about_server).text = ServiceLocator.serverBase
        val status = root.findViewById<TextView>(R.id.settings_version_status)
        val check = root.findViewById<Button>(R.id.settings_version_check)
        check.setOnClickListener {
            check.isEnabled = false
            status.text = "检查中..."
            viewLifecycleOwner.lifecycleScope.launch {
                runCatching { ServiceLocator.repository.latestVersion() }
                    .onSuccess {
                        val suffix = if (it.force) " · 强制更新" else ""
                        status.text = "最新版本 ${it.versionName} (${it.versionCode})$suffix"
                    }
                    .onFailure { status.text = it.message ?: "检查失败" }
                check.isEnabled = true
            }
        }
    }

    companion object {
        fun newInstance() = SettingsFragment()
    }
}
