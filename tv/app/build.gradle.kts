plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.serialization")
}

android {
    namespace = "com.jerocine.tv"
    compileSdk = 35

    defaultConfig {
        applicationId = "art.jerocine.tv"
        minSdk = 21          // 覆盖坚果 Nano (Android 6.0 / API 23)
        targetSdk = 30       // 不上 36，避免老 ROM 解析风险
        versionCode = 1
        versionName = "0.1.0"

        // 后端 base url（可用 -PapiBase=... 覆盖，便于模拟器/真机联调）
        val apiBase = (project.findProperty("apiBase") as String?) ?: "http://localhost:9000/"
        buildConfigField("String", "API_BASE_URL", "\"$apiBase\"")
        // Web 端地址（二维码指向 /tv-auth?code=... 供手机扫码授权）
        val webBase = (project.findProperty("webBase") as String?) ?: "http://localhost/"
        buildConfigField("String", "WEB_BASE_URL", "\"$webBase\"")
    }

    buildTypes {
        release {
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    buildFeatures {
        buildConfig = true
    }
    lint {
        disable += "ExpiredTargetSdkVersion"
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.fragment:fragment-ktx:1.8.5")
    implementation("androidx.recyclerview:recyclerview:1.3.2")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")

    // Coil 加载海报图
    implementation("io.coil-kt:coil:2.7.0")

    // 二维码生成（登录扫码）
    implementation("com.google.zxing:core:3.5.3")

    // token 加密存储
    implementation("androidx.security:security-crypto:1.1.0-alpha06")

    // Media3 / ExoPlayer（主播放器）
    implementation("androidx.media3:media3-exoplayer:1.4.1")
    implementation("androidx.media3:media3-exoplayer-hls:1.4.1")
    // 用 OkHttp 做数据源：复用 API 那条能过 TLS 的路径（老设备 CA 问题）
    implementation("androidx.media3:media3-datasource-okhttp:1.4.1")
    implementation("androidx.media3:media3-ui:1.4.1")

    // IJKPlayer（兜底播放器，ffmpeg 内核，啃畸形采集源）
    implementation("tv.danmaku.ijk.media:ijkplayer-java:0.8.8")
    implementation("tv.danmaku.ijk.media:ijkplayer-arm64:0.8.8")
    implementation("tv.danmaku.ijk.media:ijkplayer-armv7a:0.8.8")

    // 网络：Retrofit + kotlinx-serialization（无反射，适合弱机）
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
    implementation("com.squareup.retrofit2:retrofit:2.11.0")
    implementation("com.squareup.retrofit2:converter-kotlinx-serialization:2.11.0")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("com.squareup.okhttp3:logging-interceptor:4.12.0")

    // 单测
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
}
