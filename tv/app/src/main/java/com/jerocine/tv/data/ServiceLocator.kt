package com.jerocine.tv.data

import android.content.Context
import com.jerocine.tv.BuildConfig

/**
 * 极简依赖提供者。支持运行时切换服务器地址（同一 APK 连不同后端，参考现有 app 设计）。
 * API base = 服务器地址 + /api/v1/
 */
object ServiceLocator {

    /** 默认服务器（可在设置里改，存 prefs） */
    private const val DEFAULT_SERVER = "https://jerocine.art"
    private const val DEFAULT_MEDIA_SERVER = "http://jerocine.art"

    @Volatile
    var token: String? = null
        private set

    @Volatile
    var serverBase: String = DEFAULT_SERVER
        private set

    @Volatile
    internal lateinit var tokenStore: TokenStore
        private set

    fun init(context: Context) {
        tokenStore = TokenStore(context.applicationContext)
        token = tokenStore.token
        tokenStore.server?.takeIf { it.isNotBlank() }?.let { serverBase = it }
        rebuild()
    }

    val isLoggedIn: Boolean get() = !token.isNullOrBlank()

    @JvmStatic
    fun reduceMotionMode(): String =
        if (::tokenStore.isInitialized) tokenStore.reduceMotion else "auto"

    private fun apiBase(): String = serverBase.trimEnd('/') + "/api/v1/"

    /** 生产媒体链走 HTTP，规避 Android 6 过期 CA/系统 TLS；登录和业务 API 仍保持 HTTPS。 */
    fun proxyBase(): String {
        val base = serverBase.trimEnd('/')
        return (if (base.equals(DEFAULT_SERVER, ignoreCase = true)) DEFAULT_MEDIA_SERVER else base) + "/api"
    }

    /** 服务端处理 m3u8 清单和广告过滤，媒体分片默认由设备直连。 */
    fun m3u8ProxyUrl(rawLink: String, filterAds: Boolean = true): String {
        val enc = java.net.URLEncoder.encode(rawLink, "UTF-8")
        return proxyBase() + "/v1/m3u8/proxy?src=" + enc + "&filterAds=" +
            (if (filterAds) "1" else "0") + "&proxyMedia=0"
    }

    fun setServer(url: String) {
        serverBase = url.trim().trimEnd('/')
        if (::tokenStore.isInitialized) tokenStore.server = serverBase
        rebuild()
    }

    fun saveLogin(token: String, userName: String) {
        this.token = token
        if (::tokenStore.isInitialized) {
            tokenStore.token = token
            tokenStore.userName = userName
        }
    }

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
