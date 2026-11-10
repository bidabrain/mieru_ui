# gomobile bindings (JNI looks these up by name)
-keep class go.** { *; }
-keep class io.github.bidabrain.mieru.libcore.** { *; }
# app models are (de)serialised by kotlinx.serialization
-keep,includedescriptorclasses class io.github.bidabrain.mieru.data.**$$serializer { *; }
-keepclassmembers class io.github.bidabrain.mieru.data.** { *** Companion; kotlinx.serialization.KSerializer serializer(...); }
-dontwarn org.slf4j.**
