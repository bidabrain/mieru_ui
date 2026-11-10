package io.github.bidabrain.mieru.core

import android.content.Context
import android.util.Log
import io.github.bidabrain.mieru.data.AppData
import io.github.bidabrain.mieru.data.AppJson
import io.github.bidabrain.mieru.data.CoreOptions
import io.github.bidabrain.mieru.data.LogBatch
import io.github.bidabrain.mieru.data.Profile
import io.github.bidabrain.mieru.data.Rule
import io.github.bidabrain.mieru.data.Traffic
import io.github.bidabrain.mieru.data.newId
import io.github.bidabrain.mieru.libcore.Libcore
import java.io.File

/** Thin Kotlin facade over the gomobile-generated Libcore. */
object Core {
    lateinit var homeDir: File
        private set

    fun init(ctx: Context) {
        homeDir = File(ctx.filesDir, "core").apply { mkdirs() }
        copyGeoAssets(ctx)
        Libcore.init(homeDir.path)
    }

    /** First launch: seed mihomo's geo databases from the APK so routing works offline. */
    private fun copyGeoAssets(ctx: Context) {
        val names = runCatching { ctx.assets.list("geo")?.toList() }.getOrNull() ?: return
        for (name in names) {
            val out = File(homeDir, name)
            if (out.exists() && out.length() > 0) continue
            runCatching {
                ctx.assets.open("geo/$name").use { input -> out.outputStream().use { input.copyTo(it) } }
            }.onFailure { Log.w("Core", "copy $name failed", it) }
        }
    }

    val version: String get() = Libcore.version()

    fun parseLinks(text: String): List<Profile> =
        AppJson.decodeFromString<List<Profile>>(Libcore.parseLinks(text)).map { it.copy(id = newId()) }

    fun link(p: Profile): String = Libcore.profileLink(AppJson.encodeToString(Profile.serializer(), p))

    /** Validates and normalises; throws with a readable message. */
    fun checkProfile(p: Profile): Profile =
        AppJson.decodeFromString<Profile>(Libcore.checkProfile(AppJson.encodeToString(Profile.serializer(), p)))
            .copy(id = p.id)

    fun checkRule(r: Rule): Rule =
        AppJson.decodeFromString<Rule>(Libcore.checkRule(AppJson.encodeToString(Rule.serializer(), r))).copy(id = r.id)

    fun options(d: AppData, p: Profile): String = AppJson.encodeToString(
        CoreOptions.serializer(),
        CoreOptions(
            profile = p, routing = d.routing, ipv6 = d.settings.ipv6, stack = d.settings.stack,
            logLevel = d.settings.logLevel, geoxSource = d.settings.geoxSource, mixedPort = d.settings.mixedPort,
        ),
    )

    fun preview(d: AppData): String {
        val p = d.selected ?: error("没有节点")
        return Libcore.previewConfig(options(d, p))
    }

    fun start(d: AppData, p: Profile, fd: Int) = Libcore.start(options(d, p), fd)
    fun stop() = Libcore.stop()
    val isRunning: Boolean get() = Libcore.isRunning()

    fun traffic(): Traffic = runCatching { AppJson.decodeFromString<Traffic>(Libcore.traffic()) }.getOrDefault(Traffic())

    fun testDelay(p: Profile, url: String, timeoutMs: Int = 6000): Int =
        Libcore.testDelay(AppJson.encodeToString(Profile.serializer(), p), url, timeoutMs).toInt()

    fun logs(after: Long): LogBatch = AppJson.decodeFromString(Libcore.logs(after))
    fun clearLogs() = Libcore.clearLogs()
    fun updateGeo() = Libcore.updateGeo()
}

/** gomobile wraps Go errors; keep only the message the user should read. */
fun Throwable.userMessage(): String = (message ?: toString()).removePrefix("go.Universe\$proxyerror: ").trim()
