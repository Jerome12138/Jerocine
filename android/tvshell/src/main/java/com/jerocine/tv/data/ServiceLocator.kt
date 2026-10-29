package com.jerocine.tv.data

import android.content.Context
import com.jerocine.player.data.ShellPrefs
import com.jerocine.tv.BuildConfig

/**
 * 极简依赖提供者。支持运行时切换服务器地址（同一 APK 连不同后端，参考现有 app 设计）。
 * API base = 服务器地址 + /api/v1/
 *
 * 2026-10-29 解耦: "安卓自身调用的接口域"与"页面站点地址"分离 ——
 *   - 接口根: 构建期注入(JEROCINE_API_BASE, 含 /api 不含 /v1, 形状同前端 VITE_API_BASE),
 *     运行时不可变, 未注入(空)时回退 serverBase 旧行为(公开仓自构建仍可用);
 *   - 站点地址: serverBase, 设置抽屉可改, 只影响 web 壳加载页 / device 登录页。
 */
object ServiceLocator {

    /** 出厂站点地址兜底 —— 仅注入值(JEROCINE_WEB_BASE)为空时才会用到; 占位域名, 公开仓不存真实域名 */
    private const val DEFAULT_SERVER = "https://jerocine.invalid"

    /** 构建期注入的接口根(含 /api, 不含 /v1); 空 = 未注入 */
    @Volatile
    private var injectedApiBase: String = ""

    @Volatile
    var token: String? = null
        private set

    @Volatile
    var serverBase: String = DEFAULT_SERVER
        private set
        /* 语义(2026-10-29 解耦后): 站点地址 —— web 壳加载页 / device 登录页 / 出厂默认。
           安卓自身接口不再跟随它(走 injectedApiBase), 除非构建未注入(空→回退旧行为)。 */

    @Volatile
    internal lateinit var tokenStore: TokenStore
        private set

    /** 仅用于把"后端地址"镜像进壳层共享 prefs(见 setServer), 不持有 Activity */
    private var appContext: Context? = null

    fun init(context: Context, injectedApiBase: String = "", injectedWebBase: String = "") {
        applyInjectedApiBase(injectedApiBase)
        // 出厂站点地址: 构建注入值优先, 注入为空才落占位 DEFAULT_SERVER。
        // serverBase 之后仍会被持久化 prefs 覆盖(见下)。
        serverBase = injectedWebBase.trim().ifBlank { DEFAULT_SERVER }
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

    /**
     * 构建期注入接口根(JEROCINE_API_BASE, 形状同前端 VITE_API_BASE: 含 /api、不含版本号)。
     * 由 Application 启动时调用一次; 与 setServer 无关 —— 接口域不随"设置抽屉改站点地址"漂移。
     */
    fun applyInjectedApiBase(url: String?) {
        injectedApiBase = url?.trim()?.trimEnd('/') ?: ""
    }

    /**
     * 当前生效的接口根(到 /api/v1, 无尾斜杠; Retrofit baseUrl 再补 "/")。
     * 注入优先; 未注入回退 serverBase 旧行为 —— 公开仓自构建(不注入)连接自己部署的后端。
     * 诊断/测试可读。
     */
    val effectiveApiBase: String
        get() {
            val injected = injectedApiBase.trim().trimEnd('/')
            return if (injected.isNotEmpty()) "$injected/v1"
            else serverBase.trimEnd('/') + "/api/v1"
        }

    private fun apiBase(): String = effectiveApiBase + "/"

    
    /**
     * 运行时切换站点地址(web 壳加载页等)。Java 侧(web 壳 WebShellActivity)也要能调,
     * 故 @JvmStatic, 与 saveToken/currentToken 一致。
     * 注意: 只影响站点地址; 接口域由构建注入(applyInjectedApiBase), 不随这里变。
     */
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
