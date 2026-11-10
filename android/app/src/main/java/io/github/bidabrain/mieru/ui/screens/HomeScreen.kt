package io.github.bidabrain.mieru.ui.screens

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.ArrowDownward
import androidx.compose.material.icons.rounded.ArrowUpward
import androidx.compose.material.icons.rounded.Bolt
import androidx.compose.material.icons.rounded.Dns
import androidx.compose.material.icons.rounded.PowerSettingsNew
import androidx.compose.material.icons.rounded.Route
import androidx.compose.material.icons.rounded.Speed
import androidx.compose.material3.ButtonGroupDefaults
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialShapes
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.ToggleButton
import androidx.compose.material3.toShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.bidabrain.mieru.R
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.data.RouteModes
import io.github.bidabrain.mieru.data.Store
import io.github.bidabrain.mieru.data.Traffic
import io.github.bidabrain.mieru.data.formatBytes
import io.github.bidabrain.mieru.data.formatSpeed
import io.github.bidabrain.mieru.ui.GlassCard
import io.github.bidabrain.mieru.ui.IconBadge
import io.github.bidabrain.mieru.ui.LocalActivity
import io.github.bidabrain.mieru.ui.LocalNav
import io.github.bidabrain.mieru.ui.PageScaffold
import io.github.bidabrain.mieru.ui.frosted
import io.github.bidabrain.mieru.ui.theme.LocalDarkTheme
import io.github.bidabrain.mieru.ui.LocalBackdrop
import io.github.bidabrain.mieru.vpn.VpnController
import io.github.bidabrain.mieru.vpn.VpnState
import io.github.bidabrain.mieru.vpn.VpnStatus
import kotlinx.coroutines.delay

@Composable
fun HomeScreen() {
    val data by Store.data.collectAsState()
    val status by VpnState.status.collectAsState()
    val nav = LocalNav.current
    val ctx = LocalContext.current
    val profile = data.selected

    PageScaffold(
        title = "Mieru",
        subtitle = "見える · mieru 客户端",
        actions = {
            io.github.bidabrain.mieru.ui.AppLogo(Modifier.padding(end = 12.dp).size(38.dp))
        },
    ) {
        item { ConnectCard(status) }
        item {
            GlassCard(onClick = { nav.selectTab(1) }) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    IconBadge(Icons.Rounded.Dns)
                    Spacer(Modifier.width(14.dp))
                    Column(Modifier.weight(1f)) {
                        Text("当前节点", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(
                            profile?.name ?: "还没有节点，点这里添加",
                            style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold,
                            maxLines = 1, overflow = TextOverflow.Ellipsis,
                        )
                        if (profile != null) {
                            Text(
                                profile.summary, style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis,
                            )
                        }
                    }
                    if (profile != null) LatencyChip(profile)
                    Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        item { TrafficCard(status) }
        item {
            GlassCard {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    IconBadge(Icons.Rounded.Route, tint = MaterialTheme.colorScheme.secondary)
                    Spacer(Modifier.width(14.dp))
                    Column(Modifier.weight(1f)) {
                        Text("分流模式", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                        Text(
                            "规则 ${data.routing.rules.count { it.enabled }} 条 · 详细设置在「分流」页",
                            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                Spacer(Modifier.height(14.dp))
                ModeSelector(data.routing.mode) { m ->
                    Store.updateRouting { it.copy(mode = m) }
                    VpnController.reloadIfRunning(ctx)
                }
            }
        }
    }
}

@Composable
fun ModeSelector(mode: String, onChange: (String) -> Unit) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(ButtonGroupDefaults.ConnectedSpaceBetween)) {
        RouteModes.forEachIndexed { i, (key, label) ->
            ToggleButton(
                checked = mode == key,
                onCheckedChange = { if (it) onChange(key) },
                modifier = Modifier.weight(1f),
                shapes = when (i) {
                    0 -> ButtonGroupDefaults.connectedLeadingButtonShapes()
                    RouteModes.lastIndex -> ButtonGroupDefaults.connectedTrailingButtonShapes()
                    else -> ButtonGroupDefaults.connectedMiddleButtonShapes()
                },
                contentPadding = PaddingValues(horizontal = 4.dp),
            ) { Text(label, maxLines = 1, style = MaterialTheme.typography.labelLarge) }
        }
    }
}

@Composable
private fun ConnectCard(status: VpnStatus) {
    val activity = LocalActivity.current
    val cs = MaterialTheme.colorScheme
    val running = status == VpnStatus.Running
    val busy = status == VpnStatus.Starting || status == VpnStatus.Stopping

    val spin by rememberInfiniteTransition(label = "spin").animateFloat(
        0f, 360f, infiniteRepeatable(tween(20000, easing = LinearEasing)), label = "deg",
    )
    val scale by animateFloatAsState(
        if (running) 1f else 0.92f, spring(dampingRatio = Spring.DampingRatioMediumBouncy, stiffness = Spring.StiffnessLow),
        label = "scale",
    )
    val fill by animateColorAsState(if (running) cs.primary else cs.surface, label = "fill")
    val iconTint by animateColorAsState(if (running) cs.onPrimary else cs.primary, label = "tint")
    val cookie = MaterialShapes.Cookie9Sided.toShape()
    val circle = MaterialShapes.Circle.toShape()

    var elapsed by remember { mutableLongStateOf(0L) }
    LaunchedEffect(running) {
        while (running) {
            elapsed = (System.currentTimeMillis() - VpnState.startedAt) / 1000
            delay(1000)
        }
    }

    GlassCard(contentPadding = PaddingValues(vertical = 28.dp, horizontal = 20.dp)) {
        Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
            Box(Modifier.size(196.dp), contentAlignment = Alignment.Center) {
                val shape = if (running) cookie else circle
                Box(
                    Modifier
                        .size(184.dp)
                        .graphicsLayer {
                            scaleX = scale; scaleY = scale
                            rotationZ = if (running) spin else 0f
                        }
                        .then(
                            if (running) Modifier.clip(shape).background(fill)
                            else Modifier.frosted(LocalBackdrop.current, shape, cs.primaryContainer, LocalDarkTheme.current, thick = true),
                        )
                        .clickable(
                            interactionSource = remember { MutableInteractionSource() },
                            indication = androidx.compose.material3.ripple(),
                            enabled = !busy,
                        ) { if (running) activity.disconnect() else activity.connect() },
                )
                AnimatedContent(busy, transitionSpec = { fadeIn() togetherWith fadeOut() }, label = "icon") { b ->
                    if (b) {
                        LoadingIndicator(Modifier.size(72.dp), color = if (running) cs.onPrimary else cs.primary)
                    } else {
                        Icon(Icons.Rounded.PowerSettingsNew, "连接", tint = iconTint, modifier = Modifier.size(64.dp))
                    }
                }
            }
            Spacer(Modifier.height(18.dp))
            Text(
                when (status) {
                    VpnStatus.Running -> "已连接"
                    VpnStatus.Starting -> "正在连接…"
                    VpnStatus.Stopping -> "正在断开…"
                    VpnStatus.Stopped -> "未连接"
                },
                style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold,
                color = if (running) cs.primary else cs.onSurface,
            )
            Text(
                if (running) formatDuration(elapsed) else "轻点按钮开始连接",
                style = MaterialTheme.typography.bodyMedium,
                color = cs.onSurfaceVariant,
                fontFamily = if (running) FontFamily.Monospace else FontFamily.Default,
            )
        }
    }
}

private fun formatDuration(s: Long): String = "%02d:%02d:%02d".format(s / 3600, s / 60 % 60, s % 60)

@Composable
private fun TrafficCard(status: VpnStatus) {
    var t by remember { mutableStateOf(Traffic()) }
    LaunchedEffect(status) {
        while (status == VpnStatus.Running) {
            t = Core.traffic()
            delay(1000)
        }
        if (status == VpnStatus.Stopped) t = Traffic()
    }
    val cs = MaterialTheme.colorScheme
    GlassCard {
        Row(Modifier.fillMaxWidth()) {
            TrafficCell(Icons.Rounded.ArrowUpward, "上传", formatSpeed(t.up), "共 ${formatBytes(t.upTotal)}", cs.tertiary, Modifier.weight(1f))
            Spacer(Modifier.width(12.dp))
            TrafficCell(Icons.Rounded.ArrowDownward, "下载", formatSpeed(t.down), "共 ${formatBytes(t.downTotal)}", cs.secondary, Modifier.weight(1f))
        }
        Spacer(Modifier.height(10.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Rounded.Bolt, null, Modifier.size(16.dp), tint = cs.onSurfaceVariant)
            Spacer(Modifier.width(4.dp))
            Text("活动连接 ${t.conns}", style = MaterialTheme.typography.labelMedium, color = cs.onSurfaceVariant)
        }
    }
}

@Composable
private fun TrafficCell(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    value: String,
    total: String,
    color: androidx.compose.ui.graphics.Color,
    modifier: Modifier,
) {
    Column(modifier) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier
                    .size(24.dp)
                    .clip(CircleShape)
                    .background(color.copy(alpha = 0.18f)),
                contentAlignment = Alignment.Center,
            ) { Icon(icon, null, Modifier.size(16.dp), tint = color) }
            Spacer(Modifier.width(8.dp))
            Text(label, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Spacer(Modifier.height(6.dp))
        Text(value, fontSize = 22.sp, fontWeight = FontWeight.Bold, maxLines = 1)
        Text(total, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
fun TestAllButton(onClick: () -> Unit) {
    FilledTonalButton(onClick = onClick) {
        Icon(Icons.Rounded.Speed, null, Modifier.size(18.dp))
        Spacer(Modifier.width(6.dp))
        Text("测速")
    }
}
