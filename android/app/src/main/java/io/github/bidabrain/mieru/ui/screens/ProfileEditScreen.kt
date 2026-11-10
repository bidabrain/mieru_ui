package io.github.bidabrain.mieru.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.RemoveCircleOutline
import androidx.compose.material.icons.rounded.Visibility
import androidx.compose.material.icons.rounded.VisibilityOff
import androidx.compose.material3.ButtonGroupDefaults
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExposedDropdownMenuAnchorType
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenu
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.ToggleButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.core.userMessage
import io.github.bidabrain.mieru.data.Binding
import io.github.bidabrain.mieru.data.Profile
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.ui.GlassCard
import io.github.bidabrain.mieru.ui.LocalNav
import io.github.bidabrain.mieru.ui.PageScaffold
import io.github.bidabrain.mieru.ui.SectionTitle
import io.github.bidabrain.mieru.vpn.VpnController

private val muxOptions = listOf(
    "" to "默认", "MULTIPLEXING_OFF" to "关闭", "MULTIPLEXING_LOW" to "低",
    "MULTIPLEXING_MIDDLE" to "中", "MULTIPLEXING_HIGH" to "高",
)
private val handshakeOptions = listOf("" to "默认", "HANDSHAKE_STANDARD" to "标准", "HANDSHAKE_NO_WAIT" to "不等待（0-RTT）")

private class BindingDraft(port: String, protocol: String) {
    var port by mutableStateOf(port)
    var protocol by mutableStateOf(protocol)
}

@Composable
fun ProfileEditScreen(id: String?) {
    val nav = LocalNav.current
    val ctx = LocalContext.current
    val original = remember(id) { Store.data.value.profiles.firstOrNull { it.id == id } ?: Profile(bindings = listOf(Binding(port = 0))) }

    var name by remember { mutableStateOf(original.name) }
    var server by remember { mutableStateOf(original.server) }
    var user by remember { mutableStateOf(original.username) }
    var pass by remember { mutableStateOf(original.password) }
    var showPass by remember { mutableStateOf(false) }
    var mtu by remember { mutableStateOf(if (original.mtu > 0) original.mtu.toString() else "") }
    var mux by remember { mutableStateOf(original.multiplexing) }
    var handshake by remember { mutableStateOf(original.handshakeMode) }
    val bindings = remember {
        mutableStateListOf<BindingDraft>().apply {
            original.bindings.forEach { add(BindingDraft(if (it.port == 0 && it.portRange.isEmpty()) "" else it.portText, it.protocol)) }
        }
    }

    fun save() {
        val draft = original.copy(
            name = name.trim(), server = server.trim(), username = user.trim(), password = pass,
            mtu = mtu.trim().toIntOrNull() ?: 0, multiplexing = mux, handshakeMode = handshake,
            bindings = bindings.map { b ->
                val t = b.port.trim()
                if (t.contains('-')) Binding(portRange = t, protocol = b.protocol) else Binding(port = t.toIntOrNull() ?: 0, protocol = b.protocol)
            },
        )
        runCatching { Core.checkProfile(draft) }
            .onSuccess { p ->
                Store.upsertProfile(p)
                if (Store.data.value.selected?.id == p.id) VpnController.reloadIfRunning(ctx)
                nav.toast("已保存")
                nav.pop()
            }
            .onFailure { nav.toast(it.userMessage()) }
    }

    PageScaffold(
        title = if (id == null) "新建节点" else "编辑节点",
        back = true,
        actions = {
            TextButton(onClick = ::save) {
                Icon(Icons.Rounded.Check, null)
                Spacer(Modifier.width(4.dp))
                Text("保存")
            }
        },
    ) {
        item { SectionTitle("服务器") }
        item {
            GlassCard {
                Field("名称", name, { name = it }, placeholder = "可选，默认用服务器地址")
                Field("服务器地址", server, { server = it }, placeholder = "域名或 IP", keyboard = KeyboardType.Uri)
                Field("用户名", user, { user = it })
                OutlinedTextField(
                    value = pass, onValueChange = { pass = it }, label = { Text("密码") }, singleLine = true,
                    visualTransformation = if (showPass) VisualTransformation.None else PasswordVisualTransformation(),
                    trailingIcon = {
                        IconButton(onClick = { showPass = !showPass }) {
                            Icon(if (showPass) Icons.Rounded.VisibilityOff else Icons.Rounded.Visibility, "显示密码")
                        }
                    },
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        }
        item { SectionTitle("端口") }
        item {
            GlassCard {
                bindings.forEachIndexed { i, b ->
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding4()) {
                        OutlinedTextField(
                            value = b.port, onValueChange = { b.port = it.filter { c -> c.isDigit() || c == '-' } },
                            label = { Text("端口或范围") }, placeholder = { Text("2012 或 3000-3010") }, singleLine = true,
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                            modifier = Modifier.weight(1f),
                        )
                        Spacer(Modifier.width(8.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(ButtonGroupDefaults.ConnectedSpaceBetween)) {
                            listOf("TCP", "UDP").forEachIndexed { j, proto ->
                                ToggleButton(
                                    checked = b.protocol == proto,
                                    onCheckedChange = { b.protocol = proto },
                                    shapes = if (j == 0) ButtonGroupDefaults.connectedLeadingButtonShapes() else ButtonGroupDefaults.connectedTrailingButtonShapes(),
                                ) { Text(proto) }
                            }
                        }
                        IconButton(onClick = { if (bindings.size > 1) bindings.removeAt(i) }, enabled = bindings.size > 1) {
                            Icon(Icons.Rounded.RemoveCircleOutline, "删除端口")
                        }
                    }
                }
                TextButton(onClick = { bindings.add(BindingDraft("", "TCP")) }) {
                    Icon(Icons.Rounded.Add, null)
                    Spacer(Modifier.width(4.dp))
                    Text("添加端口")
                }
            }
        }
        item { SectionTitle("高级") }
        item {
            GlassCard {
                Field("MTU", mtu, { mtu = it.filter(Char::isDigit) }, placeholder = "留空使用默认（1400）", keyboard = KeyboardType.Number)
                Choice("多路复用", mux, muxOptions) { mux = it }
                Spacer(Modifier.height(8.dp))
                Choice("握手模式", handshake, handshakeOptions) { handshake = it }
                Spacer(Modifier.height(6.dp))
                Text(
                    "多路复用和握手模式需与服务器端设置兼容；不确定时保持默认。",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

private fun Modifier.padding4() = this.then(Modifier.fillMaxWidth())

@Composable
fun Field(
    label: String,
    value: String,
    onChange: (String) -> Unit,
    placeholder: String? = null,
    keyboard: KeyboardType = KeyboardType.Text,
    singleLine: Boolean = true,
) {
    OutlinedTextField(
        value = value, onValueChange = onChange, label = { Text(label) }, singleLine = singleLine,
        placeholder = placeholder?.let { { Text(it) } },
        keyboardOptions = KeyboardOptions(keyboardType = keyboard),
        modifier = Modifier.fillMaxWidth(),
    )
    Spacer(Modifier.height(8.dp))
}

@Composable
fun Choice(label: String, value: String, options: List<Pair<String, String>>, onChange: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(expanded = open, onExpandedChange = { open = it }) {
        OutlinedTextField(
            value = options.firstOrNull { it.first == value }?.second ?: value,
            onValueChange = {}, readOnly = true, label = { Text(label) },
            trailingIcon = { androidx.compose.material3.ExposedDropdownMenuDefaults.TrailingIcon(open) },
            modifier = Modifier
                .fillMaxWidth()
                .menuAnchor(ExposedDropdownMenuAnchorType.PrimaryNotEditable),
        )
        ExposedDropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            options.forEach { (k, v) ->
                DropdownMenuItem(text = { Text(v) }, onClick = { onChange(k); open = false })
            }
        }
    }
}
