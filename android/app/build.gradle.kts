plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

import java.util.Properties

val localProperties = Properties().apply {
    val localPropertiesFile = rootProject.file("local.properties")
    if (localPropertiesFile.exists()) {
        localPropertiesFile.inputStream().use(::load)
    }
}

fun configValue(name: String, defaultValue: String): String {
    return localProperties.getProperty(name)
        ?: providers.gradleProperty(name).orNull
        ?: defaultValue
}

fun flavorConfigValue(flavor: String, name: String, defaultValue: String): String {
    val flavorPrefix = flavor.uppercase()
    return localProperties.getProperty("${flavorPrefix}_${name}")
        ?: providers.gradleProperty("${flavorPrefix}_${name}").orNull
        ?: configValue(name, defaultValue)
}

android {
    namespace = "com.example.watch_together"
    compileSdk {
        version = release(36) {
            minorApiLevel = 1
        }
    }

    defaultConfig {
        applicationId = "com.example.watch_together"
        minSdk = 24
        targetSdk = 36
        versionCode = 1
        versionName = "1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        buildConfigField(
            "String",
            "MEDIA_DEFAULT_ID",
            "\"${configValue("MEDIA_DEFAULT_ID", "")}\""
        )
    }

    flavorDimensions += "env"

    productFlavors {
        create("local") {
            dimension = "env"
            buildConfigField("String", "APP_ENV", "\"local\"")
            buildConfigField(
                "String",
                "API_BASE_URL",
                "\"${flavorConfigValue("local", "API_BASE_URL", "http://10.0.2.2:8080")}\""
            )
            buildConfigField(
                "String",
                "WS_BASE_URL",
                "\"${flavorConfigValue("local", "WS_BASE_URL", "ws://10.0.2.2:8080/ws")}\""
            )
            buildConfigField(
                "String",
                "MEDIA_BASE_URL",
                "\"${flavorConfigValue("local", "MEDIA_BASE_URL", "http://10.0.2.2:9000/media/tmp")}\""
            )
            buildConfigField(
                "boolean",
                "REWRITE_LOOPBACK_MEDIA_URLS",
                flavorConfigValue("local", "REWRITE_LOOPBACK_MEDIA_URLS", "true")
            )
        }

        create("prod") {
            dimension = "env"
            buildConfigField("String", "APP_ENV", "\"prod\"")
            buildConfigField(
                "String",
                "API_BASE_URL",
                "\"${flavorConfigValue("prod", "API_BASE_URL", "http://106.12.35.52:8080")}\""
            )
            buildConfigField(
                "String",
                "WS_BASE_URL",
                "\"${flavorConfigValue("prod", "WS_BASE_URL", "ws://106.12.35.52:8080/ws")}\""
            )
            buildConfigField(
                "String",
                "MEDIA_BASE_URL",
                "\"${flavorConfigValue("prod", "MEDIA_BASE_URL", "http://106.12.35.52:9100/watch-together-media")}\""
            )
            buildConfigField(
                "boolean",
                "REWRITE_LOOPBACK_MEDIA_URLS",
                flavorConfigValue("prod", "REWRITE_LOOPBACK_MEDIA_URLS", "false")
            )
        }
    }

    buildTypes {
        debug {
            buildConfigField(
                "boolean",
                "DEBUG_SYNC",
                configValue("DEBUG_SYNC_DEBUG", "true")
            )
        }

        release {
            buildConfigField(
                "boolean",
                "DEBUG_SYNC",
                configValue("DEBUG_SYNC_RELEASE", "false")
            )
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.media3.exoplayer)
    implementation(libs.androidx.media3.exoplayer.hls)
    implementation(libs.androidx.media3.ui)
    implementation(libs.okhttp)
    testImplementation(libs.junit)
    testImplementation(libs.json)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}
