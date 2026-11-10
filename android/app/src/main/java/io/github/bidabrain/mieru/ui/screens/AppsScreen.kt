package io.github.bidabrain.mieru.ui.screens

import android.Manifest
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.graphics.drawable.Drawable
import androidx.compose.foundation.Image
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material3.ButtonGroupDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.ToggleButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.graphics.drawable.toBitmap
import io.github.bidabrain.mieru.data.PerAppMode
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.ui.GlassCard
import io.github.bidabrain.mieru.ui.PageScaffold
import io.github.bidabrain.mieru.vpn.VpnController
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

private class AppItem(val pkg: String, val label: String, val system: Boolean, val icon: Drawable?)

@Composable
fun AppsScreen() {
    val ctx = LocalContext.current
    val data by Store.data.collectAsState()
    val s = data.settings
    var apps by remember { mutableStateOf<List<AppItem>?>(null) }
    var query by remember { mutableStateOf("") }
    var showSystem by remember { mutableStateOf(false) }
    var menu by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        apps = withContext(Dispatchers.IO) {
            val pm = ctx.packageManager
            pm.getInstalledPackages(PackageManager.GET_PERMISSIONS)
                .filter { it.packageName != ctx.packageName && it.requestedPermissions?.contains(Manifest.permission.INTERNET) == true }
                .mapNotNull { pi ->
                    val ai = pi.applicationInfo ?: return@mapNotNull null
                    AppItem(
                        pi.packageName,
                        ai.loadLabel(pm).toString(),
                        ai.flags and ApplicationInfo.FLAG_SYSTEM != 0,
                        runCatching { ai.loadIcon(pm) }.getOrNull(),
                    )
                }
                .sortedBy { it.label.lowercase() }
        }
    }

    fun update(mode: String = s.perAppMode, list: Set<String> = s.perAppList) {
        Store.updateSettings { it.copy(perAppMode = mode, perAppList = list) }
        VpnController.reloadIfRunning(ctx)
    }

    val visible = apps.orEmpty().filter {
        (showSystem || !it.system || it.pkg in s.perAppList) &&
            (query.isBlank() || it.label.contains(query, true) || it.pkg.contains(query, true))
    }.sortedByDescending { it.pkg in s.perAppList }

    PageScaffold(
        title = "分应用代理",
        subtitle = "已选 ${s.perAppList.size} 个",
        back = true,
        actions = {
            Box {
                IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "更多") }
                DropdownMenu(menu, { menu = false }) {
                    DropdownMenuItem(text = { Text(if (showSystem) "隐藏系统应用" else "显示系统应用") }, onClick = { showSystem = !showSystem; menu = false })
                    DropdownMenuItem(text = { Text("全选当前列表") }, onClick = { update(list = s.perAppList + visible.map { it.pkg }); menu = false })
                    DropdownMenuItem(text = { Text("反选当前列表") }, onClick = {
                        val vis = visible.map { it.pkg }.toSet()
                        update(list = (s.perAppList - vis) + (vis - s.perAppList)); menu = false
                    })
                    DropdownMenuItem(text = { Text("清空") }, onClick = { update(list = emptySet()); menu = false })
                }
            }
        },
    ) {
        item {
            GlassCard {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(ButtonGroupDefaults.ConnectedSpaceBetween)) {
                    val modes = listOf(PerAppMode.OFF to "关闭", PerAppMode.PROXY to "仅所选走代理", PerAppMode.BYPASS to "所选不走代理")
                    modes.forEachIndexed { i, (k, label) ->
                        ToggleButton(
                            checked = s.perAppMode == k, onCheckedChange = { update(mode = k) }, modifier = Modifier.weight(1f),
                            shapes = when (i) {
                                0 -> ButtonGroupDefaults.connectedLeadingButtonShapes()
                                modes.lastIndex -> ButtonGroupDefaults.connectedTrailingButtonShapes()
                                else -> ButtonGroupDefaults.connectedMiddleButtonShapes()
                            },
                            contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 4.dp),
                        ) { Text(label, maxLines = 1, style = MaterialTheme.typography.labelMedium) }
                    }
                }
                Spacer(Modifier.size(10.dp))
                OutlinedTextField(
                    query, { query = it }, singleLine = true, placeholder = { Text("搜索应用或包名") },
                    leadingIcon = { Icon(Icons.Rounded.Search, null) }, modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(50),
                )
            }
        }
        if (apps == null) {
            item { Box(Modifier.fillMaxWidth().padding(32.dp), contentAlignment = Alignment.Center) { LoadingIndicator() } }
        }
        items(visible, key = { it.pkg }) { app ->
            val checked = app.pkg in s.perAppList
            GlassCard(
                highlighted = checked,
                contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 14.dp, vertical = 10.dp),
                shape = MaterialTheme.shapes.large,
                onClick = { update(list = if (checked) s.perAppList - app.pkg else s.perAppList + app.pkg) },
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    val bmp = remember(app.pkg) { runCatching { app.icon?.toBitmap(96, 96)?.asImageBitmap() }.getOrNull() }
                    if (bmp != null) Image(bmp, null, Modifier.size(40.dp).clip(RoundedCornerShape(10.dp)))
                    else Spacer(Modifier.size(40.dp))
                    Spacer(Modifier.width(12.dp))
                    androidx.compose.foundation.layout.Column(Modifier.weight(1f)) {
                        Text(app.label, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(app.pkg, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    Checkbox(checked, null)
                }
            }
        }
    }
}
