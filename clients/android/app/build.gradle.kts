import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "io.resystems.renotify"
    compileSdk = 36
    buildToolsVersion = "36.1.0"

    defaultConfig {
        applicationId = "io.resystems.renotify"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }

    signingConfigs {
        val ksFile = file("../release.keystore")
        val signingProps = file("../release.properties")
        if (ksFile.exists() && signingProps.exists()) {
            val props = Properties().apply { signingProps.inputStream().use(::load) }
            create("release") {
                storeFile = ksFile
                storePassword = props.getProperty("storePassword")
                keyAlias = "renotify"
                keyPassword = props.getProperty("keyPassword")
            }
        } else if (ksFile.exists()) {
            logger.warn("WARNING: release.properties not found — APK will be unsigned. Run 'make generate-keystore' to regenerate.")
        } else {
            logger.warn("WARNING: release.keystore not found — APK will be unsigned. Run 'make generate-keystore' to create one.")
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.findByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    packaging {
        resources {
            excludes += "META-INF/versions/9/OSGI-INF/MANIFEST.MF"
        }
    }
}

dependencies {
    implementation("androidx.activity:activity-ktx:1.10.1")
    implementation("androidx.core:core-ktx:1.16.0")

    // CameraX for camera preview and frame analysis
    implementation("androidx.camera:camera-core:1.4.2")
    implementation("androidx.camera:camera-camera2:1.4.2")
    implementation("androidx.camera:camera-lifecycle:1.4.2")
    implementation("androidx.camera:camera-view:1.4.2")

    // ML Kit Barcode Scanning (bundled, no Play Services)
    implementation("com.google.mlkit:barcode-scanning:17.3.0")

    // Encrypted credential storage
    implementation("androidx.security:security-crypto:1.1.0-alpha06")

    // NATS Java client (WSS, JetStream, token auth)
    implementation("io.nats:jnats:2.21.1")

    // Kotlin coroutines for Android
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.1")

    // Lifecycle-aware coroutine scopes for Activity and Service
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.9.0")
    implementation("androidx.lifecycle:lifecycle-service:2.9.0")

    // RecyclerView for dashboard list (M-09)
    implementation("androidx.recyclerview:recyclerview:1.4.0")

    // JVM unit tests
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.10.1")
    testImplementation("org.mockito:mockito-core:5.11.0")
    testImplementation("org.mockito.kotlin:mockito-kotlin:5.2.1")

    // Instrumented tests
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    androidTestImplementation("androidx.test:runner:1.6.2")
}
