buildscript {
    repositories {
        google()
        mavenCentral()
    }
    dependencies {
        classpath("com.android.tools.build:gradle:8.11.0")
        classpath("org.jetbrains.kotlin:kotlin-gradle-plugin:1.9.25")
    }
}

allprojects {
    // Use Nix's patched aapt2 instead of the generic Linux binary from Maven.
    // Outside the Android Nix shell, Gradle keeps its standard behavior.
    System.getenv("TAURI_ANDROID_AAPT2")?.let {
        extensions.extraProperties["android.aapt2FromMavenOverride"] = it
    }
    repositories {
        google()
        mavenCentral()
    }
}

tasks.register("clean").configure {
    delete("build")
}
