package io.github.bidabrain.mieru.ui.screens

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.CheckCircle
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.ContentPaste
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.EditNote
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.QrCode2
import androidx.compose.material.icons.rounded.QrCodeScanner
import androidx.compose.material.icons.rounded.RadioButtonUnchecked
import androidx.compose.material.icons.rounded.Speed
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.google.zxing.BarcodeFormat
import com.journeyapps.barcodescanner.BarcodeEncoder
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.core.userMessage
import io.github.bidabrain.mieru.data.Profile
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.ui.GlassCard
import io.github.bidabrain.mieru.ui.LocalActivity
import io.github.bidabrain.mieru.ui.LocalNav
import io.github.bidabrain.mieru.ui.PageScaffold
import io.github.bidabrain.mieru.ui.SettingRow
import io.github.bidabrain.mieru.ui.Sub
import io.github.bidabrain.mieru.vpn.VpnController
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Latency results shared by the home and profile pages: id -> ms (>0), -1 failed, 0 testing. */
object Latency {
    val results = mutableStateMapOf<String, Int>()

    suspend fun test(p: Profile) {
        results[p.id] = 0
        val url = Store.data.value.settings.testUrl
        results[p.id] = withContext(Dispatchers.IO) { runCatching { Core.testDelay(p, url) }.getOrDefault(-1) }
    }
}

@Composable
fun LatencyChip(p: Profile) {
    val scope = rememberCoroutineScope()
    val r = Latency.results[p.id]
    val cs = MaterialTheme.colorScheme
    val (text, color) = when {
        r == null -> "测速" to cs.primary
        r == 0 -> "…" to cs.onSurfaceVariant
        r < 0 -> "超时" to cs.error
        r < 300 -> "$r ms" to Color(0xFF2E9E5B)
        r < 800 -> "$r ms" to Color(0xFFD08A00)
        else -> "$r ms" to cs.error
    }
    Text(
        text, style = MaterialTheme.typography.labelMedium, color = color, fontWeight = FontWeight.SemiBold,
        modifier = Modifier
            .clip(RoundedCornerShape(50))
            .background(color.copy(alpha = 0.14f))
            .clickable { scope.launch { Latency.test(p) } }
            .padding(horizontal = 10.dp, vertical = 4.dp),
    )
}

@Composable
fun ProfilesScreen() {
    val data by Store.data.collectAsState()
    val nav = LocalNav.current
    val activity = LocalActivity.current
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var showAdd by remember { mutableStateOf(false) }
    var share by remember { mutableStateOf<Profile?>(null) }
    var confirmDelete by remember { mutableStateOf<Profile?>(null) }

    val scanner = androidx.activity.compose.rememberLauncherForActivityResult(ScanContract()) { res ->
        val text = res.contents ?: return@rememberLauncherForActivityResult
        activity.importText(text)
            .onSuccess { nav.toast("已导入 $it 个节点") }
            .onFailure { nav.toast(it.message ?: "二维码不是有效的 mieru 链接") }
    }

    PageScaffold(
        title = "节点",
        subtitle = "${data.profiles.size} 个节点",
        actions = {
            if (data.profiles.isNotEmpty()) {
                IconButton(onClick = { data.profiles.forEach { p -> scope.launch { Latency.test(p) } } }) {
                    Icon(Icons.Rounded.Speed, "全部测速")
                }
            }
        },
        overlay = {
            ExtendedFloatingActionButton(
                onClick = { showAdd = true },
                icon = { Icon(Icons.Rounded.Add, null) },
                text = { Text("添加节点") },
                modifier = Modifier
                    .align(Alignment.BottomEnd)
                    .navigationBarsPadding()
                    .padding(end = 20.dp, bottom = 108.dp),
            )
        },
    ) {
        if (data.profiles.isEmpty()) {
            item {
                GlassCard {
                    Text("还没有节点", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(6.dp))
                    Text(
                        "在 mieru-ui 面板的「用户管理 → 分享」里复制 mierus:// 链接或扫描二维码，然后点右下角「添加节点」导入。",
                        style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        items(data.profiles, key = { it.id }) { p ->
            val selected = p.id == data.selected?.id
            ProfileCard(
                p, selected,
                onSelect = {
                    if (!selected) {
                        Store.select(p.id)
                        VpnController.reloadIfRunning(ctx)
                    }
                },
                onEdit = { nav.push(Sub.EditProfile(p.id)) },
                onShare = { share = p },
                onDelete = { confirmDelete = p },
            )
        }
    }

    if (showAdd) {
        ModalBottomSheet(onDismissRequest = { showAdd = false }) {
            Column(Modifier.padding(horizontal = 20.dp).padding(bottom = 28.dp)) {
                Text("添加节点", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.height(12.dp))
                SettingRow(Icons.Rounded.ContentPaste, "从剪贴板导入", "mierus:// 链接（可多行）或 mieru 客户端 JSON", onClick = {
                    showAdd = false
                    val text = clipboardText(ctx)
                    if (text.isNullOrBlank()) nav.toast("剪贴板是空的")
                    else activity.importText(text)
                        .onSuccess { nav.toast("已导入 $it 个节点") }
                        .onFailure { nav.toast(it.message ?: "导入失败") }
                })
                SettingRow(Icons.Rounded.QrCodeScanner, "扫描二维码", "扫描面板分享页的二维码", onClick = {
                    showAdd = false
                    scanner.launch(
                        ScanOptions().setDesiredBarcodeFormats(ScanOptions.QR_CODE).setPrompt("对准 mieru 分享二维码")
                            .setBeepEnabled(false).setOrientationLocked(false),
                    )
                })
                SettingRow(Icons.Rounded.EditNote, "手动填写", "自己输入服务器、端口和账号", onClick = {
                    showAdd = false
                    nav.push(Sub.EditProfile(null))
                })
            }
        }
    }

    share?.let { p -> ShareDialog(p) { share = null } }

    confirmDelete?.let { p ->
        AlertDialog(
            onDismissRequest = { confirmDelete = null },
            title = { Text("删除节点") },
            text = { Text("确定删除「${p.name}」？") },
            confirmButton = {
                TextButton(onClick = { Store.deleteProfile(p.id); confirmDelete = null }) {
                    Text("删除", color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { confirmDelete = null }) { Text("取消") } },
        )
    }
}

@Composable
private fun ProfileCard(
    p: Profile,
    selected: Boolean,
    onSelect: () -> Unit,
    onEdit: () -> Unit,
    onShare: () -> Unit,
    onDelete: () -> Unit,
) {
    var menu by remember { mutableStateOf(false) }
    val cs = MaterialTheme.colorScheme
    GlassCard(highlighted = selected, onClick = onSelect) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(
                if (selected) Icons.Rounded.CheckCircle else Icons.Rounded.RadioButtonUnchecked, null,
                tint = if (selected) cs.primary else cs.outline,
            )
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f)) {
                Text(p.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(p.server, style = MaterialTheme.typography.bodySmall, color = cs.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Spacer(Modifier.height(6.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    p.bindings.take(3).forEach { b ->
                        Text(
                            "${b.protocol} ${b.portText}",
                            style = MaterialTheme.typography.labelSmall,
                            color = cs.onSecondaryContainer,
                            modifier = Modifier
                                .clip(RoundedCornerShape(8.dp))
                                .background(cs.secondaryContainer.copy(alpha = 0.7f))
                                .padding(horizontal = 8.dp, vertical = 2.dp),
                        )
                    }
                }
            }
            LatencyChip(p)
            Box {
                IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "更多") }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    DropdownMenuItem(text = { Text("编辑") }, leadingIcon = { Icon(Icons.Rounded.Edit, null) }, onClick = { menu = false; onEdit() })
                    DropdownMenuItem(text = { Text("分享") }, leadingIcon = { Icon(Icons.Rounded.QrCode2, null) }, onClick = { menu = false; onShare() })
                    DropdownMenuItem(
                        text = { Text("删除", color = cs.error) },
                        leadingIcon = { Icon(Icons.Rounded.Delete, null, tint = cs.error) },
                        onClick = { menu = false; onDelete() },
                    )
                }
            }
        }
    }
}

@Composable
private fun ShareDialog(p: Profile, onDismiss: () -> Unit) {
    val ctx = LocalContext.current
    val nav = LocalNav.current
    val link = remember(p) { runCatching { Core.link(p) }.getOrElse { it.userMessage() } }
    val qr: Bitmap? = remember(link) {
        runCatching { BarcodeEncoder().encodeBitmap(link, BarcodeFormat.QR_CODE, 720, 720) }.getOrNull()
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(p.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        text = {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                if (qr != null) {
                    Image(
                        qr.asImageBitmap(), "二维码",
                        Modifier
                            .size(240.dp)
                            .clip(RoundedCornerShape(16.dp))
                            .background(Color.White)
                            .padding(8.dp),
                    )
                }
                Spacer(Modifier.height(10.dp))
                Text("链接包含密码，请只分享给自己的设备。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        },
        confirmButton = {
            TextButton(onClick = {
                copyText(ctx, link)
                nav.toast("链接已复制")
                onDismiss()
            }) {
                Icon(Icons.Rounded.ContentCopy, null, Modifier.size(18.dp))
                Spacer(Modifier.width(6.dp))
                Text("复制链接")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("关闭") } },
    )
}

fun clipboardText(ctx: Context): String? {
    val cm = ctx.getSystemService(ClipboardManager::class.java)
    return cm.primaryClip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.coerceToText(ctx)?.toString()
}

fun copyText(ctx: Context, text: String) {
    ctx.getSystemService(ClipboardManager::class.java).setPrimaryClip(ClipData.newPlainText("mieru", text))
}
