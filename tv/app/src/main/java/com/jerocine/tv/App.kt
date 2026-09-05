package com.jerocine.tv

import android.app.Application
import coil.Coil
import coil.ImageLoader
import com.jerocine.tv.data.ServiceLocator

class JerocineTvApp : Application() {
    override fun onCreate() {
        super.onCreate()
        ServiceLocator.init(this)
        // Coil 用媒体 OkHttp：老设备系统 CA 太旧，第三方图床 CDN（img.lzipic.com 等）现代根证书
        // 验不过（Trust anchor not found），海报全空白；媒体链放宽 TLS + 浏览器 UA（绕豆瓣 418）。
        Coil.setImageLoader(
            ImageLoader.Builder(this)
                .okHttpClient(ServiceLocator.mediaClient)
                .build()
        )
    }
}
