import java.util.Properties
import java.io.File

plugins {
    // 由 com.android.application 改为 library：合并后它是 :app(宿主) 的子模块
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.serialization")
}

// ⚠️ 合并后 rootProject 变成 web/android，不能用 rootProject.file("../scripts/...")。
//    与 player-core 相同：向上查找仓库根（5 层内）。
fun findRepoFile(relative: String): File? {
    var dir: File? = rootProject.projectDir
    var depth = 0
    while (dir != null && depth < 5) {
        val candidate = File(dir, relative)
        if (candidate.exists()) return candidate
        dir = dir.parentFile
        depth++
    }
    return null
}

val jcVersionsFile = findRepoFile("scripts/android-versions.properties")
    ?: throw GradleException("找不到 scripts/android-versions.properties, 无法确定版本号")
val jcVersions = Properties().apply { jcVersionsFile.inputStream().use { load(it) } }
fun jcVersion(key: String, prop: String): String =
    (project.findProperty(prop) as String?) ?: jcVersions.getProperty(key)
        ?: throw GradleException("读不到版本号, 请检查 $jcVersionsFile 里的 $key")

android {
    namespace = "com.jerocine.tv"

    // 与宿主 :app 对齐到 36：宿主依赖 androidx.core:core:1.17.0(要求 compileSdk>=36)，
    // 库若仍用 35 会报 "requires ... compile against version 36 or later"。
    compileSdk = 36

    defaultConfig {
        minSdk = 21          // 覆盖坚果 Nano (Android 6.0 / API 23)
        // library 无 applicationId / versionCode / versionName；targetSdk 对库无意义，删掉。

        // 后端 base url（可用 -PapiBase=... 覆盖）
        val apiBase = (project.findProperty("apiBase") as String?) ?: "http://localhost:9000/"
        buildConfigField("String", "API_BASE_URL", "\"$apiBase\"")
        // Web 端地址（二维码指向 /tv-auth?code=...）
        val webBase = (project.findProperty("webBase") as String?) ?: "http://localhost/"
        buildConfigField("String", "WEB_BASE_URL", "\"$webBase\"")

        // ⚠️ library 不自动生成 VERSION_NAME / VERSION_CODE, 必须手动补
        //    (早期 SettingsFragment 直接读它们, 不补就编译失败)。
        //    单包合并后**只有宿主 :app 一个版本号**(web.*) —— 库的版本若还读 native.*
        //    就会和真实 APK 版本脱节(抽屉头部/诊断信息会显示早已作废的旧号)。
        //    这里与宿主同源取值; 运行期展示以 PackageManager 读到的已安装包版本为准。
        buildConfigField("String", "VERSION_NAME", "\"${jcVersion("web.versionName", "webVersionName")}\"")
        buildConfigField("int", "VERSION_CODE", jcVersion("web.versionCode", "webVersionCode"))
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
}

dependencies {
    implementation(project(":player-core"))

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

    // Media3 / ExoPlayer 由 :player-core 以 api 传递，这里不重复声明。
    // ⚠️ IJKPlayer 三件套已删除（死依赖，见执行手册 §1）。

    // 网络：Retrofit + kotlinx-serialization
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
