package com.jerocine.tv.ui.view

import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.TextView
import androidx.core.view.isVisible
import androidx.fragment.app.Fragment
import androidx.fragment.app.viewModels
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jerocine.tv.R
import com.jerocine.tv.data.ServiceLocator
import com.jerocine.tv.ui.LoginUi
import com.jerocine.tv.ui.LoginViewModel
import com.jerocine.tv.ui.applyLoginKeyboardKey
import com.jerocine.tv.ui.generateQr
import com.jerocine.tv.ui.loginKeyboardKeys
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class LoginFragment : Fragment(R.layout.fragment_login) {
    private val viewModel: LoginViewModel by viewModels()
    private var mode = MODE_QR
    private var target = TARGET_ACCOUNT
    private var account = ""
    private var password = ""

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val qrMode = view.findViewById<Button>(R.id.login_mode_qr)
        val passwordMode = view.findViewById<Button>(R.id.login_mode_password)
        val qrPanel = view.findViewById<View>(R.id.login_qr_panel)
        val passwordPanel = view.findViewById<View>(R.id.login_password_panel)
        val accountField = view.findViewById<Button>(R.id.login_account)
        val passwordField = view.findViewById<Button>(R.id.login_password)
        val keyboard = view.findViewById<RecyclerView>(R.id.login_keyboard)
        val status = view.findViewById<TextView>(R.id.login_status)
        val retry = view.findViewById<Button>(R.id.login_retry)
        val back = view.findViewById<ImageButton>(R.id.login_back)
        var panelRevealed = false

        listOf<View>(back, qrMode, passwordMode, accountField, passwordField, retry).forEach { control ->
            control.installTvFocusAnimation { ServiceLocator.tokenStore.reduceMotion }
        }
        back.setOnClickListener { requireActivity().onBackPressedDispatcher.onBackPressed() }

        fun updateMode(next: String) {
            mode = next
            qrPanel.isVisible = next == MODE_QR
            passwordPanel.isVisible = next == MODE_PASSWORD
            qrMode.isSelected = next == MODE_QR
            passwordMode.isSelected = next == MODE_PASSWORD
            if (!panelRevealed) {
                panelRevealed = true
                (if (next == MODE_QR) qrPanel else passwordPanel)
                    .revealTvContent { ServiceLocator.tokenStore.reduceMotion }
            }
        }

        fun selectTarget(next: String) {
            target = next
            accountField.isSelected = next == TARGET_ACCOUNT
            passwordField.isSelected = next == TARGET_PASSWORD
        }

        fun updateFields() {
            accountField.text = account.ifBlank { "用户名 / 邮箱" }
            passwordField.text = if (password.isBlank()) "密码" else "*".repeat(password.length)
        }

        fun submitPassword() {
            val cleanAccount = account.trim()
            when {
                cleanAccount.isBlank() -> status.text = "请输入用户名 / 邮箱"
                password.isBlank() -> status.text = "请输入密码"
                else -> viewLifecycleOwner.lifecycleScope.launch {
                    status.text = "登录中..."
                    runCatching { ServiceLocator.repository.login(cleanAccount, password) }
                        .onSuccess {
                            ServiceLocator.saveLogin(it.token, it.userName)
                            parentFragmentManager.popBackStack()
                        }
                        .onFailure { status.text = it.message ?: "登录失败" }
                }
            }
        }

        qrMode.setOnClickListener { updateMode(MODE_QR) }
        passwordMode.setOnClickListener {
            updateMode(MODE_PASSWORD)
            accountField.requestFocus()
        }
        accountField.setOnClickListener { selectTarget(TARGET_ACCOUNT) }
        passwordField.setOnClickListener { selectTarget(TARGET_PASSWORD) }
        retry.setOnClickListener { viewModel.start() }

        keyboard.layoutManager = GridLayoutManager(requireContext(), 8)
        keyboard.adapter = SearchKeyboardAdapter(loginKeyboardKeys()) { key ->
            if (key == "登录") {
                submitPassword()
            } else {
                val command = when (key) {
                    "退格" -> "BACKSPACE"
                    "清空" -> "CLEAR"
                    "空格" -> "SPACE"
                    else -> key
                }
                if (target == TARGET_PASSWORD) {
                    password = applyLoginKeyboardKey(password, command)
                } else {
                    account = applyLoginKeyboardKey(account, command)
                }
                updateFields()
            }
        }
        keyboard.itemAnimator = null
        selectTarget(TARGET_ACCOUNT)
        updateFields()
        updateMode(MODE_QR)

        viewLifecycleOwner.lifecycleScope.launch {
            viewLifecycleOwner.repeatOnLifecycle(Lifecycle.State.STARTED) {
                viewModel.state.collect { state ->
                    when (state) {
                        LoginUi.Loading -> {
                            status.text = "正在获取配对码..."
                            retry.isVisible = false
                        }
                        is LoginUi.Pairing -> {
                            status.text = "用手机扫码确认后，此设备将自动登录"
                            retry.isVisible = false
                            bindQr(view, state.userCode)
                        }
                        LoginUi.Approved -> parentFragmentManager.popBackStack()
                        is LoginUi.Error -> {
                            status.text = state.message
                            retry.isVisible = true
                        }
                    }
                }
            }
        }
    }

    private fun bindQr(view: View, userCode: String) {
        view.findViewById<TextView>(R.id.login_code).text = userCode
        val content = ServiceLocator.serverBase.trimEnd('/') + "/device?code=" + userCode
        viewLifecycleOwner.lifecycleScope.launch {
            val bitmap = withContext(Dispatchers.Default) { generateQr(content, 360) }
            view.findViewById<ImageView>(R.id.login_qr).setImageBitmap(bitmap)
        }
    }

    companion object {
        private const val MODE_QR = "qr"
        private const val MODE_PASSWORD = "password"
        private const val TARGET_ACCOUNT = "account"
        private const val TARGET_PASSWORD = "password"

        fun newInstance() = LoginFragment()
    }
}
