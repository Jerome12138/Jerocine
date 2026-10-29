package com.jerocine.tv.data

import android.content.Context
import com.jerocine.player.data.ShellPrefs
import com.jerocine.tv.BuildConfig

/**
 * 极简依赖提供者。支持运行时切换服务器地址（同一 APK 连不同后端，参考现有 app 设计）。
 * API base = 服务器地址 + /api/v1/
 */
object ServiceLocator {

    /** 默认服务器（可在设置里改，存 prefs） */
    private const val DEFAULT_SERVER = "https://jerocine.art"

    @Volatile
    var token: String? = null
        private set

    @Volatile
    var serverBase: String = DEFAULT_SERVER
        private set

    @Volatile
    internal lateinit var tokenStore: TokenStore
        private set

    /** 仅用于把"后端地址"镜像进壳层共享 prefs(见 setServer), 不持有 Activity */
    private var appContext: Context? = null

    fun init(context: Context) {
        val app = context.applicationContext
        appContext = app
        tokenStore = TokenStore(app)
        token = tokenStore.token
        tokenStore.server?.takeIf { it.isNotBlank() }?.let { serverBase = it }
        // 老 web 壳用户升级到单包后的一次性迁移: 后端地址此前只存在壳层 prefs
        // (jerocine_prefs/server_url), TokenStore.server 还是空 ⇒ 不迁移的话原生壳会连默认后端。
        if (tokenStore.server.isNullOrBlank()) {
            app.getSharedPreferences(ShellPrefs.NAME, Context.MODE_PRIVATE)
                .getString(ShellPrefs.KEY_SERVER_URL, null)
                ?.trim()
                ?.takeIf { it.isNotEmpty() }
                ?.let {
                    serverBase = it
                    tokenStore.server = it
                }
        }
        rebuild()
    }

    val isLoggedIn: Boolean get() = !token.isNullOrBlank()

    @JvmStatic
    fun reduceMotionMode(): String =
        if (::tokenStore.isInitialized) tokenStore.reduceMotion else "auto"

    private fun apiBase(): String = serverBase.trimEnd('/') + "/api/v1/"

    
    /** Java 侧(web 壳 WebShellActivity)也要能调, 故 @JvmStatic, 与 saveToken/currentToken 一致 */
    @JvmStatic
    fun setServer(url: String) {
        serverBase = url.trim().trimEnd('/')
        if (::tokenStore.isInitialized) tokenStore.server = serverBase
        // 镜像到壳层共享 prefs —— web 壳读的是 jerocine_prefs/server_url(UpdateChecker /
        // promptServerUrl 都是它)。单包内两壳必须收敛到同一个后端地址, 否则切换壳模式后
        // 会连到不同后端。键名只在 ShellPrefs 定义一次。
        appContext?.getSharedPreferences(ShellPrefs.NAME, Context.MODE_PRIVATE)
            ?.edit()
            ?.putString(ShellPrefs.KEY_SERVER_URL, serverBase)
            ?.apply()
        rebuild()
    }

    fun saveLogin(token: String, userName: String) {
        this.token = token
        if (::tokenStore.isInitialized) {
            tokenStore.token = token
            tokenStore.userName = userName
        }
    }

    /**
     * 仅存 token（web 壳登录时只有 jwt, 没有 userName —— userName 由前端 localStorage 管理）。
     * 供 Java 侧 UpdateChecker / JerocineBridge 调用, 统一 token 存储到 TokenStore(加密),
     * 切壳模式时登录态不丢。
     */
    @JvmStatic
    fun saveToken(token: String?) {
        this.token = token
        if (::tokenStore.isInitialized) tokenStore.token = token
    }

    /** 当前 token(Java 侧可读); 未登录返回 null */
    @JvmStatic
    fun currentToken(): String? = token

    fun logout() {
        token = null
        if (::tokenStore.isInitialized) tokenStore.clear()
    }

    @Volatile
    lateinit var repository: FilmRepository
        private set

    @Volatile
    lateinit var userRepository: UserRepository
        private set

    /** API 用 OkHttp（严格 TLS，携带登录 token） */
    @Volatile
    lateinit var httpClient: okhttp3.OkHttpClient
        private set

    /**
     * 媒体用 OkHttp（放宽 TLS + 浏览器 UA）：供 Coil 海报 + ExoPlayer 分片复用。
     * 老设备 CA 库太旧、海外源 CDN 证书链验不过 —— 公开内容走这条链绕开。token 绝不经此。
     */
    @Volatile
    lateinit var mediaClient: okhttp3.OkHttpClient
        private set

    private fun rebuild() {
        httpClient = ApiClient.okhttp { token }
        mediaClient = ApiClient.mediaOkhttp()
        val retrofit = ApiClient.retrofit(apiBase(), httpClient)
        repository = FilmRepository(retrofit.create(JerocineApi::class.java))
        userRepository = UserRepository(retrofit.create(UserApi::class.java))
    }
}
