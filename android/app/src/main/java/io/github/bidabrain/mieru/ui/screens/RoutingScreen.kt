package io.github.bidabrain.mieru.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Apps
import androidx.compose.material.icons.rounded.Block
import androidx.compose.material.icons.rounded.CloudDownload
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Dns
import androidx.compose.material.icons.rounded.Home
import androidx.compose.material.icons.rounded.KeyboardArrowDown
import androidx.compose.material.icons.rounded.KeyboardArrowUp
import androidx.compose.material.icons.rounded.PhoneIphone
import androidx.compose.material.icons.rounded.Radar
import androidx.compose.material.icons.rounded.Shuffle
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonGroupDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.ToggleButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.core.userMessage
import io.github.bidabrain.mieru.data.PerAppMode
import io.github.bidabrain.mieru.data.Routing
import io.github.bidabrain.mieru.data.Rule
import io.github.bidabrain.mieru.data.RuleActions
import io.github.bidabrain.mieru.data.RuleTypes
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.data.ruleActionLabel
import io.github.bidabrain.mieru.ui.GlassCard
import io.github.bidabrain.mieru.ui.LocalNav
import io.github.bidabrain.mieru.ui.PageScaffold
import io.github.bidabrain.mieru.ui.SectionTitle
import io.github.bidabrain.mieru.ui.SettingRow
import io.github.bidabrain.mieru.ui.Sub
import io.github.bidabrain.mieru.vpn.VpnController
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private val modeHelp = mapOf(
    "bypass_cn" to "国内网站和 IP 直连，其余走代理（推荐）",
    "gfw" to "只有 GFW 列表里的网站走代理，其余直连",
    "global" to "所有流量都走代理",
    "direct" to "全部直连，只保留自定义规则",
)

private val ruleHints = mapOf(
    "DOMAIN-SUFFIX" to "example.com（含所有子域名）",
    "DOMAIN" to "www.example.com（完全匹配）",
    "DOMAIN-KEYWORD" to "google（域名包含该词）",
    "GEOSITE" to "如 netflix、openai、category-ads-all、cn",
    "GEOIP" to "如 CN、telegram、google、netflix、private",
    "IP-CIDR" to "如 10.0.0.0/8 或 2001:db8::/32",
    "DST-PORT" to "如 22 或 6881-6889 或 80/443",
)

@Composable
fun RoutingScreen() {
    val data by Store.data.collectAsState()
    val r = data.routing
    val ctx = LocalContext.current
    val nav = LocalNav.current
    val scope = rememberCoroutineScope()
    var editing by remember { mutableStateOf<Rule?>(null) }
    var dnsEdit by remember { mutableStateOf(false) }
    var updatingGeo by remember { mutableStateOf(false) }

    fun change(fn: (Routing) -> Routing) {
        Store.updateRouting(fn)
        VpnController.reloadIfRunning(ctx)
    }

    PageScaffold(title = "分流", subtitle = "规则自上而下匹配，命中即停止") {
        item { SectionTitle("模式") }
        item {
            GlassCard {
                ModeSelector(r.mode) { m -> change { it.copy(mode = m) } }
                Spacer(Modifier.height(10.dp))
                Text(modeHelp[r.mode].orEmpty(), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }

        item { SectionTitle("常用") }
        item {
            GlassCard(contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 8.dp)) {
                SwitchRow(Icons.Rounded.Home, "绕过局域网", "局域网和保留地址直连", r.bypassLan) { v -> change { it.copy(bypassLan = v) } }
                SwitchRow(Icons.Rounded.Block, "拦截广告", "GEOSITE category-ads-all", r.blockAds) { v -> change { it.copy(blockAds = v) } }
                SwitchRow(Icons.Rounded.PhoneIphone, "Apple / 微软国内服务直连", "仅绕过大陆模式生效", r.appleCnDirect) { v -> change { it.copy(appleCnDirect = v) } }
                SwitchRow(Icons.Rounded.Radar, "流量嗅探", "从 TLS / HTTP 中识别域名，规则更准", r.sniff) { v -> change { it.copy(sniff = v) } }
                SettingRow(
                    Icons.Rounded.Apps, "分应用代理",
                    when (data.settings.perAppMode) {
                        PerAppMode.PROXY -> "仅 ${data.settings.perAppList.size} 个应用走代理"
                        PerAppMode.BYPASS -> "${data.settings.perAppList.size} 个应用不走代理"
                        else -> "关闭：所有应用都经过分流"
                    },
                    onClick = { nav.push(Sub.Apps) },
                    trailing = { Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null) },
                )
            }
        }

        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                SectionTitle("自定义规则", Modifier.weight(1f))
                TextButton(onClick = { editing = Rule() }) {
                    Icon(Icons.Rounded.Add, null, Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("添加")
                }
            }
        }
        if (r.rules.isEmpty()) {
            item {
                GlassCard {
                    Text("还没有自定义规则", style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
                    Text(
                        "自定义规则优先于上面的模式。例：让 netflix 走代理、公司域名直连、某个 IP 段拦截。",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.height(10.dp))
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        listOf(
                            Rule("GEOSITE", "openai", "PROXY"),
                            Rule("GEOSITE", "netflix", "PROXY"),
                            Rule("GEOSITE", "steam@cn", "DIRECT"),
                        ).forEach { preset ->
                            FilterChip(
                                selected = false,
                                onClick = { change { it.copy(rules = it.rules + preset) } },
                                label = { Text("${preset.value} → ${ruleActionLabel(preset.action)}") },
                                leadingIcon = { Icon(Icons.Rounded.Add, null, Modifier.size(16.dp)) },
                            )
                        }
                    }
                }
            }
        } else {
            item {
                GlassCard(contentPadding = androidx.compose.foundation.layout.PaddingValues(vertical = 6.dp, horizontal = 8.dp)) {
                    r.rules.forEachIndexed { i, rule ->
                        RuleRow(
                            rule,
                            onToggle = { v -> change { rt -> rt.copy(rules = rt.rules.map { if (it.id == rule.id) it.copy(enabled = v) else it }) } },
                            onEdit = { editing = rule },
                            onUp = if (i > 0) ({ change { it.copy(rules = it.rules.swap(i, i - 1)) } }) else null,
                            onDown = if (i < r.rules.lastIndex) ({ change { it.copy(rules = it.rules.swap(i, i + 1)) } }) else null,
                        )
                    }
                }
            }
        }

        item { SectionTitle("DNS 与规则数据") }
        item {
            GlassCard(contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 8.dp)) {
                SettingRow(
                    Icons.Rounded.Dns, "DNS 服务器",
                    "直连 ${r.directDns.firstOrNull().orEmpty()} · 远程 ${r.remoteDns.firstOrNull().orEmpty()}",
                    onClick = { dnsEdit = true },
                    trailing = { Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null) },
                )
                SwitchRow(Icons.Rounded.Shuffle, "Fake-IP", "更快、防 DNS 污染；关闭后使用真实 IP", r.fakeIp) { v -> change { it.copy(fakeIp = v) } }
                SettingRow(
                    Icons.Rounded.CloudDownload, "更新规则数据",
                    if (updatingGeo) "正在下载 GeoSite / GeoIP…" else "每 24 小时自动更新，也可以手动更新",
                    onClick = {
                        if (updatingGeo) return@SettingRow
                        updatingGeo = true
                        scope.launch {
                            val res = withContext(Dispatchers.IO) { runCatching { Core.updateGeo() } }
                            updatingGeo = false
                            res.onSuccess { nav.toast("规则数据已更新"); VpnController.reloadIfRunning(ctx) }
                                .onFailure { nav.toast("更新失败：" + it.userMessage()) }
                        }
                    },
                )
            }
        }
    }

    editing?.let { rule ->
        RuleDialog(
            rule,
            isNew = r.rules.none { it.id == rule.id },
            onDismiss = { editing = null },
            onSave = { saved ->
                change { rt ->
                    rt.copy(rules = if (rt.rules.any { it.id == saved.id }) rt.rules.map { if (it.id == saved.id) saved else it } else rt.rules + saved)
                }
                editing = null
            },
            onDelete = {
                change { rt -> rt.copy(rules = rt.rules.filterNot { it.id == rule.id }) }
                editing = null
            },
        )
    }

    if (dnsEdit) DnsDialog(r, onDismiss = { dnsEdit = false }) { d, rem ->
        change { it.copy(directDns = d, remoteDns = rem) }
        dnsEdit = false
    }
}

private fun <T> List<T>.swap(a: Int, b: Int): List<T> = toMutableList().also { val t = it[a]; it[a] = it[b]; it[b] = t }

@Composable
fun SwitchRow(icon: androidx.compose.ui.graphics.vector.ImageVector, title: String, sub: String?, checked: Boolean, onChange: (Boolean) -> Unit) {
    SettingRow(icon, title, sub, onClick = { onChange(!checked) }, trailing = { Switch(checked, onChange) })
}

@Composable
private fun RuleRow(rule: Rule, onToggle: (Boolean) -> Unit, onEdit: () -> Unit, onUp: (() -> Unit)?, onDown: (() -> Unit)?) {
    val cs = MaterialTheme.colorScheme
    val actionColor = when (rule.action) {
        "PROXY" -> cs.primary
        "DIRECT" -> cs.secondary
        else -> cs.error
    }
    Row(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(18.dp))
            .clickable(onClick = onEdit)
            .padding(horizontal = 8.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                rule.value, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis,
                color = if (rule.enabled) cs.onSurface else cs.outline,
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(rule.type, style = MaterialTheme.typography.labelSmall, fontFamily = FontFamily.Monospace, color = cs.onSurfaceVariant)
                Spacer(Modifier.width(8.dp))
                Text(
                    ruleActionLabel(rule.action), style = MaterialTheme.typography.labelSmall, color = actionColor,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier
                        .clip(RoundedCornerShape(6.dp))
                        .background(actionColor.copy(alpha = 0.14f))
                        .padding(horizontal = 6.dp, vertical = 1.dp),
                )
            }
        }
        IconButton(onClick = { onUp?.invoke() }, enabled = onUp != null) { Icon(Icons.Rounded.KeyboardArrowUp, "上移") }
        IconButton(onClick = { onDown?.invoke() }, enabled = onDown != null) { Icon(Icons.Rounded.KeyboardArrowDown, "下移") }
        Switch(rule.enabled, onToggle)
    }
}

@Composable
private fun RuleDialog(rule: Rule, isNew: Boolean, onDismiss: () -> Unit, onSave: (Rule) -> Unit, onDelete: () -> Unit) {
    var type by remember { mutableStateOf(rule.type) }
    var value by remember { mutableStateOf(rule.value) }
    var action by remember { mutableStateOf(rule.action) }
    var error by remember { mutableStateOf<String?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (isNew) "添加规则" else "编辑规则") },
        text = {
            Column {
                Choice("类型", type, RuleTypes.map { it to it }) { type = it; error = null }
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = value, onValueChange = { value = it; error = null }, singleLine = true,
                    label = { Text("内容") }, placeholder = { Text(ruleHints[type].orEmpty()) },
                    isError = error != null,
                    supportingText = { Text(error ?: ruleHints[type].orEmpty()) },
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(4.dp))
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(ButtonGroupDefaults.ConnectedSpaceBetween)) {
                    RuleActions.forEachIndexed { i, (k, label) ->
                        ToggleButton(
                            checked = action == k, onCheckedChange = { action = k }, modifier = Modifier.weight(1f),
                            shapes = when (i) {
                                0 -> ButtonGroupDefaults.connectedLeadingButtonShapes()
                                RuleActions.lastIndex -> ButtonGroupDefaults.connectedTrailingButtonShapes()
                                else -> ButtonGroupDefaults.connectedMiddleButtonShapes()
                            },
                        ) { Text(label) }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                runCatching { Core.checkRule(rule.copy(type = type, value = value, action = action)) }
                    .onSuccess(onSave)
                    .onFailure { error = it.userMessage() }
            }) { Text("保存") }
        },
        dismissButton = {
            Row {
                if (!isNew) TextButton(onClick = onDelete) {
                    Icon(Icons.Rounded.Delete, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.error)
                    Spacer(Modifier.width(4.dp))
                    Text("删除", color = MaterialTheme.colorScheme.error)
                }
                TextButton(onClick = onDismiss) { Text("取消") }
            }
        },
    )
}

@Composable
private fun DnsDialog(r: Routing, onDismiss: () -> Unit, onSave: (List<String>, List<String>) -> Unit) {
    var direct by remember { mutableStateOf(r.directDns.joinToString("\n")) }
    var remote by remember { mutableStateOf(r.remoteDns.joinToString("\n")) }
    fun lines(s: String) = s.lines().map { it.trim() }.filter { it.isNotEmpty() }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("DNS 服务器") },
        text = {
            Column {
                OutlinedTextField(
                    direct, { direct = it }, label = { Text("直连 DNS（国内）") }, minLines = 2,
                    supportingText = { Text("每行一个，支持 https:// tls:// 或 IP") }, modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    remote, { remote = it }, label = { Text("远程 DNS（经代理）") }, minLines = 2,
                    supportingText = { Text("用于解析被污染的域名") }, modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { if (lines(direct).isNotEmpty() && lines(remote).isNotEmpty()) onSave(lines(direct), lines(remote)) }) { Text("保存") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
