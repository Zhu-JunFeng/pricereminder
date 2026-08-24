-keepattributes *Annotation*
-keep class world.zcn.pricereminder.** { *; }

# Tink references these compile-time-only annotations from method metadata.
-dontwarn com.google.errorprone.annotations.CanIgnoreReturnValue
-dontwarn com.google.errorprone.annotations.CheckReturnValue
-dontwarn com.google.errorprone.annotations.Immutable
-dontwarn com.google.errorprone.annotations.RestrictedApi
