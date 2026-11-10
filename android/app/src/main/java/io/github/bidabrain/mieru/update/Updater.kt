package io.github.bidabrain.mieru.update

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.core.content.FileProvider
import io.github.bidabrain.mieru.BuildConfig
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.vpn.VpnState
import io.github.bidabrain.mieru.vpn.VpnStatus
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import java.io.File
import java.net.HttpURLConnection
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.URL

data class UpdateInfo(
    val version: String,
    val tag: String,
    val notes: String,
    val apkName: String,
    val apkUrl: String,
    val size: Long,
    val pageUrl: String,
)

sealed interface UpdateUi {
    data object Idle : UpdateUi
    data object Checking : UpdateUi
    data object UpToDate : UpdateUi
    data class Available(val info: UpdateInfo) : UpdateUi
    data class Downloading(val info: UpdateInfo, val progress: Float) : UpdateUi
    data class Ready(val info: UpdateInfo, val file: File) : UpdateUi
    data class Failed(val message: String) : UpdateUi
}

/**
 * Checks GitHub releases tagged `android-v*`. Every release is signed with the same fixed
 * key, so the downloaded APK installs over the current one.
 */
object Updater {
    private const val TAG_PREFIX = "android-v"
    val state = MutableStateFlow<UpdateUi>(UpdateUi.Idle)

    /** The app is excluded from its own VPN; when connected, go through the local proxy port. */
    private fun proxy(): Proxy {
        val port = Store.data.value.settings.mixedPort
        return if (VpnState.status.value == VpnStatus.Running && port > 0)
            Proxy(Proxy.Type.HTTP, InetSocketAddress("127.0.0.1", port))
        else Proxy.NO_PROXY
    }

    private fun open(url: String): HttpURLConnection =
        (URL(url).openConnection(proxy()) as HttpURLConnection).apply {
            connectTimeout = 15000
            readTimeout = 30000
            setRequestProperty("User-Agent", "Mieru-Android/${BuildConfig.VERSION_NAME}")
            setRequestProperty("Accept", "application/vnd.github+json")
            instanceFollowRedirects = true
        }

    suspend fun check(silent: Boolean = false): UpdateInfo? = withContext(Dispatchers.IO) {
        if (!silent) state.value = UpdateUi.Checking
        try {
            val conn = open("https://api.github.com/repos/${BuildConfig.UPDATE_REPO}/releases?per_page=30")
            val body = conn.inputStream.bufferedReader().use { it.readText() }
            if (conn.responseCode != 200) error("GitHub 返回 ${conn.responseCode}")
            Store.updateSettings { it.copy(lastUpdateCheck = System.currentTimeMillis()) }
            val info = pick(Json.parseToJsonElement(body).jsonArray)
            if (info != null && newer(info.version, BuildConfig.VERSION_NAME)) {
                state.value = UpdateUi.Available(info)
                info
            } else {
                if (!silent) state.value = UpdateUi.UpToDate
                null
            }
        } catch (t: Throwable) {
            if (!silent) state.value = UpdateUi.Failed(t.message ?: t.toString())
            null
        }
    }

    private fun pick(releases: JsonArray): UpdateInfo? {
        val abis = Build.SUPPORTED_ABIS.toList()
        for (r in releases) {
            val o = r.jsonObject
            val tag = o.str("tag_name")
            if (!tag.startsWith(TAG_PREFIX) || o.bool("draft") || o.bool("prerelease")) continue
            val assets = o["assets"]?.jsonArray?.map { it.jsonObject }.orEmpty().filter { it.str("name").endsWith(".apk") }
            val asset = abis.firstNotNullOfOrNull { abi -> assets.firstOrNull { it.str("name").contains(abi) } }
                ?: assets.firstOrNull { it.str("name").contains("universal") }
                ?: continue
            return UpdateInfo(
                version = tag.removePrefix(TAG_PREFIX),
                tag = tag,
                notes = o.str("body"),
                apkName = asset.str("name"),
                apkUrl = asset.str("browser_download_url"),
                size = asset["size"]?.jsonPrimitive?.long ?: 0,
                pageUrl = o.str("html_url"),
            )
        }
        return null
    }

    private fun JsonObject.str(k: String) = this[k]?.jsonPrimitive?.content ?: ""
    private fun JsonObject.bool(k: String) = runCatching { this[k]?.jsonPrimitive?.boolean }.getOrNull() ?: false

    /** Semantic-ish comparison: 1.2.10 > 1.2.9. */
    fun newer(remote: String, local: String): Boolean {
        fun parts(v: String) = v.removePrefix("v").split('.', '-').map { it.toIntOrNull() ?: 0 }
        val a = parts(remote)
        val b = parts(local)
        for (i in 0 until maxOf(a.size, b.size)) {
            val x = a.getOrElse(i) { 0 }
            val y = b.getOrElse(i) { 0 }
            if (x != y) return x > y
        }
        return false
    }

    suspend fun download(ctx: Context, info: UpdateInfo): File? = withContext(Dispatchers.IO) {
        try {
            val dir = File(ctx.cacheDir, "updates").apply { mkdirs() }
            dir.listFiles()?.forEach { it.delete() }
            val out = File(dir, info.apkName)
            state.value = UpdateUi.Downloading(info, 0f)
            val conn = open(info.apkUrl)
            conn.setRequestProperty("Accept", "application/octet-stream")
            val total = conn.contentLengthLong.takeIf { it > 0 } ?: info.size
            conn.inputStream.use { input ->
                out.outputStream().use { output ->
                    val buf = ByteArray(64 * 1024)
                    var done = 0L
                    var last = 0L
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        output.write(buf, 0, n)
                        done += n
                        if (total > 0 && done - last > 256 * 1024) {
                            last = done
                            state.value = UpdateUi.Downloading(info, done.toFloat() / total)
                        }
                    }
                }
            }
            state.value = UpdateUi.Ready(info, out)
            out
        } catch (t: Throwable) {
            state.value = UpdateUi.Failed("下载失败：${t.message ?: t}")
            null
        }
    }

    /** Returns false when the user must first allow installing unknown apps. */
    fun install(ctx: Context, file: File): Boolean {
        if (Build.VERSION.SDK_INT >= 26 && !ctx.packageManager.canRequestPackageInstalls()) {
            ctx.startActivity(
                Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${ctx.packageName}"))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            )
            return false
        }
        val uri = FileProvider.getUriForFile(ctx, "${ctx.packageName}.files", file)
        ctx.startActivity(
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK),
        )
        return true
    }

    /** Daily silent check on app start. */
    suspend fun autoCheck() {
        val s = Store.data.value.settings
        if (!s.autoCheckUpdate) return
        if (System.currentTimeMillis() - s.lastUpdateCheck < 12 * 3600_000L) return
        check(silent = true)
    }
}
