package io.github.bidabrain.mieru.ui

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Route
import androidx.compose.material.icons.rounded.Dns
import androidx.compose.material.icons.rounded.Home
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.chrisbanes.haze.hazeSource
import dev.chrisbanes.haze.rememberHazeState
import io.github.bidabrain.mieru.ui.screens.AppsScreen
import io.github.bidabrain.mieru.ui.screens.HomeScreen
import io.github.bidabrain.mieru.ui.screens.LogsScreen
import io.github.bidabrain.mieru.ui.screens.ProfileEditScreen
import io.github.bidabrain.mieru.ui.screens.ProfilesScreen
import io.github.bidabrain.mieru.ui.screens.RoutingScreen
import io.github.bidabrain.mieru.ui.screens.SettingsScreen
import io.github.bidabrain.mieru.ui.theme.LocalDarkTheme
import io.github.bidabrain.mieru.vpn.VpnState
import io.github.bidabrain.mieru.vpn.VpnStatus
import kotlinx.coroutines.launch

sealed interface Sub {
    data class EditProfile(val id: String?) : Sub
    data object Apps : Sub
    data object Logs : Sub
}

class Nav(
    val push: (Sub) -> Unit,
    val pop: () -> Unit,
    val selectTab: (Int) -> Unit,
    val toast: (String) -> Unit,
)

val LocalNav = staticCompositionLocalOf<Nav> { error("no nav") }
val LocalActivity = staticCompositionLocalOf<MainActivity> { error("no activity") }

private data class Tab(val label: String, val icon: ImageVector, val selectedIcon: ImageVector)

private val tabs = listOf(
    Tab("首页", Icons.Outlined.Home, Icons.Rounded.Home),
    Tab("节点", Icons.Outlined.Dns, Icons.Rounded.Dns),
    Tab("分流", Icons.Rounded.Route, Icons.Rounded.Route),
    Tab("设置", Icons.Outlined.Tune, Icons.Rounded.Tune),
)

@Composable
fun AppRoot(activity: MainActivity) {
    val backdrop = rememberHazeState()
    val chrome = rememberHazeState()
    val status by VpnState.status.collectAsState()
    val vpnError by VpnState.error.collectAsState()
    var tab by rememberSaveable { mutableIntStateOf(0) }
    val stack = remember { mutableStateListOf<Sub>() }
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val pending by activity.pendingImport

    val nav = remember {
        Nav(
            push = { stack.add(it) },
            pop = { if (stack.isNotEmpty()) stack.removeAt(stack.lastIndex) },
            selectTab = { stack.clear(); tab = it },
            toast = { msg -> scope.launch { snackbar.currentSnackbarData?.dismiss(); snackbar.showSnackbar(msg) } },
        )
    }

    LaunchedEffect(vpnError) {
        vpnError?.let { nav.toast(it) }
    }
    BackHandler(stack.isNotEmpty()) { nav.pop() }

    CompositionLocalProvider(
        LocalBackdrop provides backdrop,
        LocalChrome provides chrome,
        LocalNav provides nav,
        LocalActivity provides activity,
    ) {
        Box(Modifier.fillMaxSize()) {
            AuroraBackground(
                Modifier
                    .hazeSource(backdrop)
                    .hazeSource(chrome, zIndex = 0f),
                connected = status == VpnStatus.Running,
            )
            Box(
                Modifier
                    .fillMaxSize()
                    .hazeSource(chrome, zIndex = 1f),
            ) {
                val target: Any = stack.lastOrNull() ?: tab
                AnimatedContent(
                    targetState = target,
                    transitionSpec = {
                        if (targetState is Sub && initialState !is Sub || (targetState is Sub && initialState is Sub)) {
                            (slideInHorizontally { it / 3 } + fadeIn()) togetherWith (slideOutHorizontally { -it / 6 } + fadeOut())
                        } else if (initialState is Sub) {
                            (slideInHorizontally { -it / 6 } + fadeIn()) togetherWith (slideOutHorizontally { it / 3 } + fadeOut())
                        } else {
                            (fadeIn() + scaleIn(initialScale = 0.97f)) togetherWith fadeOut()
                        }
                    },
                    label = "page",
                ) { t ->
                    when (t) {
                        0 -> HomeScreen()
                        1 -> ProfilesScreen()
                        2 -> RoutingScreen()
                        3 -> SettingsScreen()
                        is Sub.EditProfile -> ProfileEditScreen(t.id)
                        Sub.Apps -> AppsScreen()
                        Sub.Logs -> LogsScreen()
                    }
                }
            }

            AnimatedVisibility(
                visible = stack.isEmpty(),
                enter = slideInVertically { it } + fadeIn(),
                exit = slideOutVertically { it } + fadeOut(),
                modifier = Modifier.align(Alignment.BottomCenter),
            ) {
                GlassNavBar(selected = tab, onSelect = { tab = it })
            }

            SnackbarHost(
                snackbar,
                Modifier
                    .align(Alignment.BottomCenter)
                    .windowInsetsPadding(WindowInsets.navigationBars)
                    .padding(bottom = if (stack.isEmpty()) 96.dp else 16.dp),
            )
        }

        pending?.let { link ->
            AlertDialog(
                onDismissRequest = { activity.pendingImport.value = null },
                title = { Text("导入节点") },
                text = { Text("是否导入这个 mierus:// 链接？", style = MaterialTheme.typography.bodyMedium) },
                confirmButton = {
                    TextButton(onClick = {
                        activity.pendingImport.value = null
                        activity.importText(link)
                            .onSuccess { nav.toast("已导入 $it 个节点"); nav.selectTab(1) }
                            .onFailure { nav.toast(it.message ?: "导入失败") }
                    }) { Text("导入") }
                },
                dismissButton = { TextButton(onClick = { activity.pendingImport.value = null }) { Text("取消") } },
            )
        }
    }
}

@Composable
private fun GlassNavBar(selected: Int, onSelect: (Int) -> Unit) {
    val cs = MaterialTheme.colorScheme
    Box(
        Modifier
            .fillMaxWidth()
            .windowInsetsPadding(WindowInsets.navigationBars)
            .padding(horizontal = 20.dp, vertical = 12.dp),
    ) {
        NavigationBar(
            modifier = Modifier
                .frosted(LocalChrome.current, RoundedCornerShape(32.dp), cs.surface, LocalDarkTheme.current, thick = true)
                .height(72.dp),
            containerColor = Color.Transparent,
            tonalElevation = 0.dp,
            windowInsets = WindowInsets(0),
        ) {
            tabs.forEachIndexed { i, t ->
                NavigationBarItem(
                    selected = selected == i,
                    onClick = { onSelect(i) },
                    icon = { Icon(if (selected == i) t.selectedIcon else t.icon, contentDescription = t.label) },
                    label = { Text(t.label, fontWeight = if (selected == i) FontWeight.SemiBold else FontWeight.Normal) },
                    colors = NavigationBarItemDefaults.colors(indicatorColor = cs.primaryContainer.copy(alpha = 0.85f)),
                )
            }
        }
    }
}

/**
 * Standard page: content scrolls under a frosted top bar that firms up once content is
 * underneath it. [bottomInset] leaves room for the floating nav bar on top-level pages.
 */
@Composable
fun PageScaffold(
    title: String,
    subtitle: String? = null,
    back: Boolean = false,
    bottomInset: Boolean = !back,
    state: LazyListState = rememberLazyListState(),
    actions: @Composable () -> Unit = {},
    overlay: @Composable BoxScope.() -> Unit = {},
    content: LazyListScope.() -> Unit,
) {
    val nav = LocalNav.current
    val top = WindowInsets.statusBars.asPaddingValues().calculateTopPadding()
    val bottom = WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
    val barAlpha by animateFloatAsState(if (state.canScrollBackward) 1f else 0f, label = "bar")
    Box(Modifier.fillMaxSize()) {
        LazyColumn(
            state = state,
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(
                start = 16.dp, end = 16.dp,
                top = top + 76.dp,
                bottom = bottom + if (bottomInset) 112.dp else 28.dp,
            ),
            verticalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(12.dp),
            content = content,
        )
        Box(Modifier.fillMaxWidth()) {
            Box(
                Modifier
                    .matchParentSize()
                    .graphicsLayer { alpha = barAlpha }
                    .frosted(LocalChrome.current, RoundedCornerShape(0.dp), MaterialTheme.colorScheme.surface, LocalDarkTheme.current, thick = true),
            )
            Row(
                Modifier
                    .fillMaxWidth()
                    .padding(top = top)
                    .height(68.dp)
                    .padding(horizontal = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (back) {
                    IconButton(onClick = nav.pop) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "返回") }
                } else {
                    Spacer(Modifier.width(12.dp))
                }
                androidx.compose.foundation.layout.Column(Modifier.weight(1f)) {
                    Text(
                        title,
                        style = if (back) MaterialTheme.typography.titleLarge else MaterialTheme.typography.headlineSmall,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    if (subtitle != null) {
                        Text(subtitle, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                actions()
            }
        }
        overlay()
    }
}
