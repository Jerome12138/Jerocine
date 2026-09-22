package com.jerocine.tv

import android.app.Application
import coil.Coil
import coil.ImageLoader
import com.jerocine.player.JerocinePlayer
import com.jerocine.tv.data.ServiceLocator

class JerocineTvApp : Application() {
    override fun onCreate() {
        super.onCreate()
        ServiceLocator.init(this)

        // 把壳层能力注入公共播放模块(播放器本身不硬编码域名/TLS 策略):
        //  - mediaClient: 放宽 TLS + 浏览器 UA 的媒体链, 老设备 CA 太旧时也能拉流
        //  - defaultProxyBase: 未显式传 proxyBase 时的兜底代理地址(端侧广告过滤用)
        JerocinePlayer.setMediaClient(ServiceLocator.mediaClient)
        JerocinePlayer.setDefaultProxyBase(ServiceLocator.proxyBase())

        // Coil 用媒体 OkHttp：老设备系统 CA 太旧，第三方图床 CDN（img.lzipic.com 等）现代根证书
        // 验不过（Trust anchor not found），海报全空白；媒体链放宽 TLS + 浏览器 UA（绕豆瓣 418）。
        Coil.setImageLoader(
            ImageLoader.Builder(this)
                .okHttpClient(ServiceLocator.mediaClient)
                .build()
        )
    }
}
