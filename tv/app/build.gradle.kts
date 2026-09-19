import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.serialization")
}

// 签名凭据来源与 web/android 对齐, 优先级: 
//   1. 环境变量 JEROCINE_*(构建脚本 / CI 注入, 本次构建的权威来源)
//   2. tv/keystore.properties(gitignored, 本机开发者自备)
//   3. 都没有 → 不建 signingConfig
// 环境变量优先是刻意的: 工程里若遗留一份 keystore.properties, 不该悄悄盖过构建脚本注入的密钥
// (否则会用一个你没意识到的密钥签名)。空值视为未设置, 继续向下回退。
// 缺凭据时不报错也是刻意的: 让 assembleDebug 仍可用。release 缺签名由 scripts/build-android.sh
// 在构建前拦下(要求只用正式密钥), 不靠 Gradle 兜底。
// 注意 keystore 用同一份即可: web/android 是 art.jerocine.app, 本工程是 art.jerocine.tv,
// 同密钥签不同 applicationId 合法, 两包互不覆盖安装。
val ksPropsFile = rootProject.file("keystore.properties")
val ksProps = Properties().apply {
    if (ksPropsFile.exists()) {
        ksPropsFile.inputStream().use { load(it) }
    }
}
fun envOrProp(name: String, prop: String): String? =
    System.getenv(name)?.takeIf { it.isNotBlank() } ?: ksProps.getProperty(prop)
val ksStorePassword: String? = envOrProp("JEROCINE_STORE_PASSWORD", "storePassword")
val ksKeyAlias: String = envOrProp("JEROCINE_KEY_ALIAS", "keyAlias") ?: "jerocine"
val ksKeyPassword: String? = envOrProp("JEROCINE_KEY_PASSWORD", "keyPassword")
// JEROCINE_KEYSTORE 支持绝对路径, 便于把密钥放在仓库外的任意位置
val ksStoreFile: File =
    System.getenv("JEROCINE_KEYSTORE")?.takeIf { it.isNotBlank() }?.let { file(it) }
        ?: rootProject.file("jerocine.keystore")
val hasSigning = ksStoreFile.exists() && ksStorePassword != null && ksKeyPassword != null

// 版本号唯一来源: 仓库根 scripts/android-versions.properties
// 不要在这里写死数字 —— 改了也只影响这一处, 否则会和 web/android / 构建脚本产物名对不上。
// 临时覆盖: ./gradlew :app:assembleRelease -PnativeVersionCode=1002 -PnativeVersionName=1.0.1
val jcVersionsFile = rootProject.file("../scripts/android-versions.properties")
val jcVersions = Properties().apply {
    if (jcVersionsFile.exists()) {
        jcVersionsFile.inputStream().use { load(it) }
    }
}
fun jcVersion(key: String, prop: String): String =
    (project.findProperty(prop) as String?) ?: jcVersions.getProperty(key)
        ?: throw GradleException("读不到版本号, 请检查 $jcVersionsFile 里的 $key")

android {
    namespace = "com.jerocine.tv"
    compileSdk = 35

    defaultConfig {
        applicationId = "art.jerocine.tv"
        minSdk = 21          // 覆盖坚果 Nano (Android 6.0 / API 23)
        targetSdk = 30       // 不上 36，避免老 ROM 解析风险
        versionCode = jcVersion("native.versionCode", "nativeVersionCode").toInt()
        versionName = jcVersion("native.versionName", "nativeVersionName")

        // 后端 base url（可用 -PapiBase=... 覆盖，便于模拟器/真机联调）
        val apiBase = (project.findProperty("apiBase") as String?) ?: "http://localhost:9000/"
        buildConfigField("String", "API_BASE_URL", "\"$apiBase\"")
        // Web 端地址（二维码指向 /tv-auth?code=... 供手机扫码授权）
        val webBase = (project.findProperty("webBase") as String?) ?: "http://localhost/"
        buildConfigField("String", "WEB_BASE_URL", "\"$webBase\"")
    }

    signingConfigs {
        if (hasSigning) {
            create("release") {
                storeFile = ksStoreFile
                storePassword = ksStorePassword
                keyAlias = ksKeyAlias
                keyPassword = ksKeyPassword
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            if (hasSigning) {
                signingConfig = signingConfigs.getByName("release")
            }
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
