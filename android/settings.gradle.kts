pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositories {
        google()
        mavenCentral()
    }
}
rootProject.name = "invisible-museum"
include(":core")
// :app needs the Android SDK; it is included only when one is present so `gradle :core:test`
// works on any machine with a JDK.
val localProps = file("local.properties")
val sdkDir = System.getenv("ANDROID_HOME") ?: System.getenv("ANDROID_SDK_ROOT")
    ?: if (localProps.exists()) localProps.readLines().firstOrNull { it.startsWith("sdk.dir=") }?.substringAfter("=") else null
if (sdkDir != null && file("app").exists()) include(":app")
