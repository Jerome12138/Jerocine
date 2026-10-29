package com.jerocine.app

import android.app.Application
import coil.Coil
import coil.ImageLoader
import com.jerocine.player.JerocinePlayer
import com.jerocine.player.data.CrashLog
import com.jerocine.tv.data.ServiceLocator

/**
 * 单包唯一 Application —— 进程级初始化必须住在宿主 :app。
 *
 * 2026-10-12 从 `:tvshell`（原 `com.jerocine.tv`）迁来：Application 是**进程级**的，
 * 与它并列的 `EntryRouterActivity` / packageName / 版本号都在宿主，它却住在"原生壳"库里，
 * 语义错位（详见 `docs/单包合并架构复盘与共性件组织方案-2026-10-12.md` §3）。
 *
 * 内容仍是"两壳都需要的初始化"（合并前 tv 壳一份，见细则 §7）：
 * 依赖方向 :app → :tvshell，因此这里引用 tvshell 的 ServiceLocator 是允许的。
 */
class JerocineApp : Application() {
    override fun onCreate() {
        super.onCreate()

        // 全局未捕获异常落盘(环形保留 5 份, 诊断页可见摘要) —— 最先装, 让后续初始化
        // 期间的崩溃也有现场。报告写 filesDir/crashes/, 写完交回系统原 handler, 崩溃流程不变。
        CrashLog.install(this)

        // 构建期域名注入(Harness 私有层持有真实值; 公开仓自构建落 .invalid 占位):
        //   API_BASE = 原生接口根(含 /api, 形状同前端 VITE_API_BASE) —— 接口域运行时不可变;
        //   WEB_BASE = 出厂站点地址(web 壳加载页默认, 设置抽屉仍可改)。
        ServiceLocator.init(
            this,
            injectedApiBase = BuildConfig.JEROCINE_API_BASE,
            injectedWebBase = BuildConfig.JEROCINE_WEB_BASE,
        )

        // 把壳层能力注入公共播放模块(播放器本身不硬编码域名/TLS 策略):
        //  - mediaClient: 放宽 TLS + 浏览器 UA 的媒体链, 老设备 CA 太旧时也能拉流
        JerocinePlayer.config().setMediaClient(ServiceLocator.mediaClient)

        // Coil 用媒体 OkHttp：老设备系统 CA 太旧，第三方图床 CDN（img.lzipic.com 等）现代根证书
        // 验不过（Trust anchor not found），海报全空白；媒体链放宽 TLS + 浏览器 UA（绕豆瓣 418）。
        Coil.setImageLoader(
            ImageLoader.Builder(this)
                .okHttpClient(ServiceLocator.mediaClient)
                .build()
        )
    }
}
