package io.github.bidabrain.mieru.ui.screens

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.Article
import androidx.compose.material.icons.rounded.Code
import androidx.compose.material.icons.rounded.ColorLens
import androidx.compose.material.icons.rounded.Info
import androidx.compose.material.icons.rounded.Lan
import androidx.compose.material.icons.rounded.Language
import androidx.compose.material.icons.rounded.Layers
import androidx.compose.material.icons.rounded.Link
import androidx.compose.material.icons.rounded.NewReleases
import androidx.compose.material.icons.rounded.SystemUpdate
import androidx.compose.material.icons.rounded.Timer
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearWavyProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import io.github.bidabrain.mieru.BuildConfig
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.data.Settings
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.data.formatBytes
import io.github.bidabrain.mieru.ui.AppLogo
import io.github.bidabrain.mieru.ui.GlassCard
import io.github.bidabrain.mieru.ui.LocalNav
import io.github.bidabrain.mieru.ui.PageScaffold
import io.github.bidabrain.mieru.ui.SectionTitle
import io.github.bidabrain.mieru.ui.SettingRow
import io.github.bidabrain.mieru.ui.Sub
import io.github.bidabrain.mieru.update.UpdateUi
import io.github.bidabrain.mieru.update.Updater
import io.github.bidabrain.mieru.vpn.VpnController
import kotlinx.coroutines.launch

@Composable
fun SettingsScreen() {
    val data by Store.data.collectAsState()
    val s = data.settings
    val ctx = LocalContext.current
    val nav = LocalNav.current
    val scope = rememberCoroutineScope()
    val update by Updater.state.collectAsState()
    var portEdit by remember { mutableStateOf(false) }
    var preview by remember { mutableStateOf<String?>(null) }

    fun change(reload: Boolean = true, fn: (Settings) -> Settings) {
        Store.updateSettings(fn)
        if (reload) VpnController.reloadIfRunning(ctx)
    }

    PageScaffold(title = "设置") {
        item { SectionTitle("外观") }
        item {
            GlassCard(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp)) {
                Choice("主题", s.theme, listOf("system" to "跟随系统", "light" to "浅色", "dark" to "深色")) { v -> change(false) { it.copy(theme = v) } }
                Spacer(Modifier.height(4.dp))
                SwitchRow(Icons.Rounded.ColorLens, "动态取色", "跟随壁纸颜色（Android 12+）", s.dynamicColor) { v -> change(false) { it.copy(dynamicColor = v) } }
            }
        }

        item { SectionTitle("网络") }
        item {
            GlassCard(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp)) {
                SwitchRow(Icons.Rounded.Language, "IPv6", "为 VPN 分配 IPv6 地址并代理 IPv6 流量", s.ipv6) { v -> change { it.copy(ipv6 = v) } }
                Choice("TUN 网络栈", s.stack, listOf("gvisor" to "gVisor（兼容性最好）", "mixed" to "Mixed", "system" to "System（性能更好）")) { v ->
                    change { it.copy(stack = v) }
                }
                Spacer(Modifier.height(4.dp))
                SettingRow(
                    Icons.Rounded.Lan, "本地代理端口",
                    if (s.mixedPort > 0) "127.0.0.1:${s.mixedPort}（HTTP / SOCKS5）" else "已关闭",
                    onClick = { portEdit = true },
                )
                Choice(
                    "规则数据下载源", s.geoxSource,
                    listOf("jsdelivr" to "jsDelivr CDN（国内较快）", "github" to "GitHub Releases"),
                ) { v -> change { it.copy(geoxSource = v) } }
                Spacer(Modifier.height(8.dp))
                Choice("日志级别", s.logLevel, listOf("debug" to "调试", "info" to "信息", "warning" to "警告", "error" to "错误", "silent" to "关闭")) { v ->
                    change { it.copy(logLevel = v) }
                }
            }
        }

        item { SectionTitle("工具") }
        item {
            GlassCard(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp)) {
                SettingRow(Icons.Rounded.Article, "运行日志", "查看内核输出", onClick = { nav.push(Sub.Logs) },
                    trailing = { Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null) })
                SettingRow(Icons.Rounded.Code, "查看生成的配置", "当前节点 + 分流设置生成的 mihomo 配置", onClick = {
                    preview = runCatching { Core.preview(Store.data.value) }.getOrElse { "生成失败：" + it.message }
                })
                SettingRow(Icons.Rounded.Timer, "测速地址", s.testUrl)
            }
        }

        item { SectionTitle("更新") }
        item {
            GlassCard(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp)) {
                SwitchRow(Icons.Rounded.NewReleases, "自动检查更新", "启动时每 12 小时检查一次 GitHub Release", s.autoCheckUpdate) { v ->
                    change(false) { it.copy(autoCheckUpdate = v) }
                }
                SettingRow(
                    Icons.Rounded.SystemUpdate, "检查更新",
                    when (val u = update) {
                        UpdateUi.Checking -> "正在检查…"
                        UpdateUi.UpToDate -> "已是最新版本 ${BuildConfig.VERSION_NAME}"
                        is UpdateUi.Available -> "发现新版本 ${u.info.version}"
                        is UpdateUi.Downloading -> "正在下载 ${(u.progress * 100).toInt()}%"
                        is UpdateUi.Ready -> "下载完成，点击安装"
                        is UpdateUi.Failed -> u.message
                        UpdateUi.Idle -> "当前版本 ${BuildConfig.VERSION_NAME}"
                    },
                    onClick = {
                        when (val u = update) {
                            is UpdateUi.Ready -> Updater.install(ctx, u.file)
                            is UpdateUi.Downloading, UpdateUi.Checking -> Unit
                            else -> scope.launch { Updater.check() }
                        }
                    },
                )
                (update as? UpdateUi.Downloading)?.let { u ->
                    LinearWavyProgressIndicator(progress = { u.progress }, modifier = Modifier.fillMaxWidth())
                    Spacer(Modifier.height(8.dp))
                }
            }
        }

        item { SectionTitle("关于") }
        item {
            GlassCard(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp)) {
                SettingRow(null, "Mieru ${BuildConfig.VERSION_NAME}", Core.version, trailing = { AppLogo(Modifier.heightIn(max = 40.dp).height(40.dp).fillMaxWidth(0.12f)) })
                SettingRow(Icons.Rounded.Link, "项目主页", "github.com/${BuildConfig.UPDATE_REPO}", onClick = {
                    ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("https://github.com/${BuildConfig.UPDATE_REPO}")))
                })
                SettingRow(Icons.Rounded.Info, "开源协议", "GPL-3.0 · 内核 mihomo + mieru")
            }
        }
    }

    // Update available dialog
    (update as? UpdateUi.Available)?.let { u ->
        AlertDialog(
            onDismissRequest = { Updater.state.value = UpdateUi.Idle },
            icon = { Icon(Icons.Rounded.SystemUpdate, null) },
            title = { Text("发现新版本 ${u.info.version}") },
            text = {
                Column(Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState())) {
                    Text("当前 ${BuildConfig.VERSION_NAME} · 安装包 ${formatBytes(u.info.size)}", style = MaterialTheme.typography.labelMedium)
                    Spacer(Modifier.height(8.dp))
                    Text(u.info.notes.ifBlank { "（无更新说明）" }, style = MaterialTheme.typography.bodyMedium)
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch { Updater.download(ctx, u.info)?.let { Updater.install(ctx, it) } }
                }) { Text("下载并安装") }
            },
            dismissButton = { TextButton(onClick = { Updater.state.value = UpdateUi.Idle }) { Text("以后再说") } },
        )
    }

    if (portEdit) {
        var text by remember { mutableStateOf(if (s.mixedPort > 0) s.mixedPort.toString() else "") }
        AlertDialog(
            onDismissRequest = { portEdit = false },
            title = { Text("本地代理端口") },
            text = {
                OutlinedTextField(
                    text, { text = it.filter(Char::isDigit).take(5) }, singleLine = true,
                    label = { Text("端口") }, supportingText = { Text("仅本机可访问；留空关闭。应用内检查更新也会经由此端口。") },
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    val p = text.toIntOrNull() ?: 0
                    if (p in 0..65535 && (p == 0 || p >= 1024)) {
                        change { it.copy(mixedPort = p) }
                        portEdit = false
                    } else nav.toast("端口需在 1024-65535 之间")
                }) { Text("保存") }
            },
            dismissButton = { TextButton(onClick = { portEdit = false }) { Text("取消") } },
        )
    }

    preview?.let { text ->
        AlertDialog(
            onDismissRequest = { preview = null },
            icon = { Icon(Icons.Rounded.Layers, null) },
            title = { Text("mihomo 配置") },
            text = {
                Column(Modifier.heightIn(max = 420.dp).verticalScroll(rememberScrollState())) {
                    Text(
                        text.replace(Regex("\"password\": \"[^\"]*\""), "\"password\": \"******\""),
                        style = MaterialTheme.typography.bodySmall,
                        fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace,
                    )
                }
            },
            confirmButton = { TextButton(onClick = { preview = null }) { Text("关闭") } },
            dismissButton = {
                TextButton(onClick = { copyText(ctx, text); nav.toast("已复制（含密码）") }) { Text("复制") }
            },
        )
    }
}

