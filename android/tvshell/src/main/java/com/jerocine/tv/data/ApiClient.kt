package com.jerocine.tv.data

import retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.X509TrustManager

/** Retrofit + kotlinx-serialization 客户端工厂 */
object ApiClient {

    /** 宽松 JSON：忽略未知字段、容忍类型、缺失填默认值 */
    val json: Json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        coerceInputValues = true
        explicitNulls = false
    }

    /** 共享 OkHttp：API 与播放器数据源共用同一条（老设备上能过 TLS 的）路径 */
    fun okhttp(tokenProvider: () -> String? = { null }): OkHttpClient {
        val logging = HttpLoggingInterceptor().apply { level = HttpLoggingInterceptor.Level.BASIC }
        return OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(20, TimeUnit.SECONDS)
            .addInterceptor { chain ->
                val b = chain.request().newBuilder()
                tokenProvider()?.let { b.header("Authorization", "Bearer $it") }
                chain.proceed(b.build())
            }
            .addInterceptor(logging)
            .build()
    }

    /**
     * 媒体专用 OkHttp：给 Coil 海报 + ExoPlayer 分片用。
     *
     * 老设备（Android 6，CA 库停留在 2015）不信任第三方图片/视频 CDN 的现代根证书
     * （Let's Encrypt ISRG X1 等），系统栈与普通 OkHttp 都会
     * "Trust anchor for certification path not found" 失败——海报不显示、海外源分片拉不动。
     * 这里的内容都是公开、非敏感的（海报、切片），登录 token 绝不走这条链（见 okhttp()），
     * 因此对媒体链放宽 TLS 校验以覆盖任意 CDN；同时带浏览器 UA 绕开豆瓣等的 418 反爬。
     */
    fun mediaOkhttp(): OkHttpClient {
        val trustAll = object : X509TrustManager {
            override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
            override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
            override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
        }
        val sslContext = SSLContext.getInstance("TLS").apply {
            init(null, arrayOf(trustAll), SecureRandom())
        }
        return OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .sslSocketFactory(sslContext.socketFactory, trustAll)
            .hostnameVerifier { _, _ -> true }
            .addInterceptor { chain ->
                // 浏览器 UA：豆瓣图床对非浏览器 UA 返回 418
                val req = chain.request().newBuilder()
                    .header(
                        "User-Agent",
                        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
                            "(KHTML, like Gecko) Chrome/120.0 Safari/537.36"
                    )
                    .build()
                chain.proceed(req)
            }
            .build()
    }

    fun retrofit(baseUrl: String, client: OkHttpClient): Retrofit =
        Retrofit.Builder()
            .baseUrl(baseUrl)
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
}
