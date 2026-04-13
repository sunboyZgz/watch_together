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

        buildConfigField("String", "APP_ENV", "\"${configValue("APP_ENV", "local")}\"")
        buildConfigField(
            "String",
            "API_BASE_URL",
            "\"${configValue("API_BASE_URL", "http://10.0.2.2:8080")}\""
        )
        buildConfigField(
            "String",
            "WS_BASE_URL",
            "\"${configValue("WS_BASE_URL", "ws://10.0.2.2:8080/ws")}\""
        )
        buildConfigField(
            "String",
            "MEDIA_BASE_URL",
            "\"${configValue("MEDIA_BASE_URL", "http://10.0.2.2:9000/media")}\""
        )
        buildConfigField(
            "String",
            "MEDIA_DEFAULT_ID",
            "\"${configValue("MEDIA_DEFAULT_ID", "")}\""
        )
        buildConfigField("boolean", "DEBUG_SYNC", configValue("DEBUG_SYNC", "true"))
    }

    buildTypes {
        release {
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
    testImplementation(libs.junit)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}
