import java.net.URI

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
}

// Version comes from the release tag in CI (android-v1.2.3 -> APP_VERSION=1.2.3).
val appVersion: String = (System.getenv("APP_VERSION") ?: "0.1.0").removePrefix("v")
val appVersionCode: Int = appVersion.split(".", "-").take(3)
    .map { it.toIntOrNull() ?: 0 }
    .let { (it + listOf(0, 0, 0)).take(3) }
    .let { (maj, min, pat) -> maj * 10000 + min * 100 + pat }
    .coerceAtLeast(1)

android {
    namespace = "io.github.bidabrain.mieru"
    compileSdk = 37

    defaultConfig {
        applicationId = "io.github.bidabrain.mieru"
        minSdk = 26
        targetSdk = 37
        versionCode = appVersionCode
        versionName = appVersion
        buildConfigField("String", "UPDATE_REPO", "\"bidabrain/mieru_ui\"")
    }

    // A fixed, public debug key is committed on purpose: every CI release is signed with the
    // same key, so a newer APK always installs over an older one. It is NOT a secret.
    signingConfigs {
        create("fixed") {
            storeFile = file("debug.keystore")
            storePassword = "android"
            keyAlias = "androiddebugkey"
            keyPassword = "android"
        }
    }

    buildTypes {
        debug {
            signingConfig = signingConfigs.getByName("fixed")
        }
        release {
            signingConfig = signingConfigs.getByName("fixed")
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a", "x86_64")
            isUniversalApk = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    packaging {
        jniLibs { useLegacyPackaging = true }
        resources { excludes += "/META-INF/{AL2.0,LGPL2.1}" }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
        optIn.addAll(
            "androidx.compose.material3.ExperimentalMaterial3Api",
            "androidx.compose.material3.ExperimentalMaterial3ExpressiveApi",
            "androidx.compose.foundation.layout.ExperimentalLayoutApi",
        )
    }
}

// Bundle the geo databases so routing works on first launch (mihomo keeps them updated).
val geoDir = layout.projectDirectory.dir("src/main/assets/geo")
val downloadGeo by tasks.registering {
    val base = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/"
    val files = mapOf("GeoSite.dat" to "geosite.dat", "GeoIP.dat" to "geoip-lite.dat")
    outputs.dir(geoDir)
    doLast {
        geoDir.asFile.mkdirs()
        files.forEach { (name, remote) ->
            val out = geoDir.file(name).asFile
            if (!out.exists() || out.length() == 0L) {
                logger.lifecycle("downloading $remote")
                val tmp = File(out.path + ".tmp")
                URI(base + remote).toURL().openStream().use { input -> tmp.outputStream().use { input.copyTo(it) } }
                tmp.renameTo(out)
            }
        }
    }
}
tasks.named("preBuild") { dependsOn(downloadGeo) }

dependencies {
    implementation(files("libs/libcore.aar"))

    val composeBom = platform("androidx.compose:compose-bom:2026.09.00")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.compose.material3:material3:1.5.0-rc01")
    implementation("androidx.compose.material:material-icons-extended:1.7.8")
    implementation("androidx.compose.animation:animation")
    debugImplementation("androidx.compose.ui:ui-tooling")

    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.activity:activity-compose:1.12.2")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.12.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")

    implementation("dev.chrisbanes.haze:haze-blur:2.1.0")

    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
}
