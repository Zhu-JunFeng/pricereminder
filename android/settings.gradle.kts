pluginManagement {
    repositories {
        maven("https://dl.google.com/dl/android/maven2")
        mavenCentral()
        gradlePluginPortal()
    }
    resolutionStrategy {
        eachPlugin {
            if (requested.id.id == "com.android.application") {
                useModule("com.android.tools.build:gradle:${requested.version}")
            }
        }
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        maven("https://dl.google.com/dl/android/maven2")
        mavenCentral()
    }
}

rootProject.name = "PriceReminder"
include(":rule-core", ":app")
