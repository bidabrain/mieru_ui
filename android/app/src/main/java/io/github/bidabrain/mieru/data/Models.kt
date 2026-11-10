package io.github.bidabrain.mieru.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.util.UUID

val AppJson = Json {
    ignoreUnknownKeys = true
    encodeDefaults = true
    explicitNulls = false
}

fun newId(): String = UUID.randomUUID().toString()

@Serializable
data class Binding(
    val port: Int = 0,
    val portRange: String = "",
    val protocol: String = "TCP",
) {
    val portText: String get() = portRange.ifEmpty { port.toString() }
}

@Serializable
data class Profile(
    val id: String = newId(),
    val name: String = "",
    val server: String = "",
    val username: String = "",
    val password: String = "",
    val bindings: List<Binding> = listOf(Binding(port = 0)),
    val mtu: Int = 0,
    val multiplexing: String = "",
    val handshakeMode: String = "",
) {
    val summary: String
        get() = server + "  ·  " + bindings.joinToString("  ") { "${it.protocol} ${it.portText}" }
}

@Serializable
data class Rule(
    val type: String = "DOMAIN-SUFFIX",
    val value: String = "",
    val action: String = "PROXY",
    val enabled: Boolean = true,
    val id: String = newId(),
)

@Serializable
data class Routing(
    val mode: String = "bypass_cn",
    val bypassLan: Boolean = true,
    val blockAds: Boolean = false,
    val appleCnDirect: Boolean = true,
    val rules: List<Rule> = emptyList(),
    val directDns: List<String> = listOf("https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"),
    val remoteDns: List<String> = listOf("https://1.1.1.1/dns-query", "https://dns.google/dns-query"),
    val fakeIp: Boolean = true,
    val sniff: Boolean = true,
)

object PerAppMode {
    const val OFF = "off"
    const val PROXY = "proxy"   // only selected apps use the VPN
    const val BYPASS = "bypass" // selected apps skip the VPN
}

@Serializable
data class Settings(
    val ipv6: Boolean = false,
    val stack: String = "gvisor",
    val logLevel: String = "info",
    val geoxSource: String = "jsdelivr",
    val mixedPort: Int = 17890,
    val perAppMode: String = PerAppMode.OFF,
    val perAppList: Set<String> = emptySet(),
    val theme: String = "system", // system | light | dark
    val dynamicColor: Boolean = false,
    val autoCheckUpdate: Boolean = true,
    val lastUpdateCheck: Long = 0,
    val testUrl: String = "https://www.gstatic.com/generate_204",
)

@Serializable
data class AppData(
    val profiles: List<Profile> = emptyList(),
    val selectedId: String? = null,
    val routing: Routing = Routing(),
    val settings: Settings = Settings(),
) {
    val selected: Profile? get() = profiles.firstOrNull { it.id == selectedId } ?: profiles.firstOrNull()
}

/** Shape sent to the Go core (mirrors libcore.Options). */
@Serializable
data class CoreOptions(
    val profile: Profile,
    val routing: Routing,
    val ipv6: Boolean,
    val stack: String,
    val logLevel: String,
    val geoxSource: String,
    val mixedPort: Int,
)

@Serializable
data class Traffic(
    val up: Long = 0,
    val down: Long = 0,
    val upTotal: Long = 0,
    val downTotal: Long = 0,
    val conns: Int = 0,
)

@Serializable
data class LogLine(val seq: Long, val time: Long, val level: String, val msg: String)

@Serializable
data class LogBatch(val next: Long, val lines: List<LogLine>)

val RouteModes = listOf(
    "bypass_cn" to "绕过大陆",
    "gfw" to "GFW 列表",
    "global" to "全局",
    "direct" to "直连",
)

fun routeModeLabel(mode: String) = RouteModes.firstOrNull { it.first == mode }?.second ?: mode

val RuleTypes = listOf("DOMAIN-SUFFIX", "DOMAIN", "DOMAIN-KEYWORD", "GEOSITE", "GEOIP", "IP-CIDR", "DST-PORT")
val RuleActions = listOf("PROXY" to "代理", "DIRECT" to "直连", "REJECT" to "拦截")
fun ruleActionLabel(a: String) = RuleActions.firstOrNull { it.first == a }?.second ?: a

fun formatBytes(b: Long): String {
    if (b < 1024) return "$b B"
    val units = listOf("KB", "MB", "GB", "TB")
    var v = b / 1024.0
    var i = 0
    while (v >= 1024 && i < units.lastIndex) { v /= 1024; i++ }
    return if (v >= 100) "%.0f %s".format(v, units[i]) else "%.1f %s".format(v, units[i])
}

fun formatSpeed(b: Long) = formatBytes(b) + "/s"
