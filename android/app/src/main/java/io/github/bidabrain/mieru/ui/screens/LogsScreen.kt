package io.github.bidabrain.mieru.ui.screens

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.DeleteSweep
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.bidabrain.mieru.core.Core
import io.github.bidabrain.mieru.data.LogLine
import io.github.bidabrain.mieru.ui.GlassCard
import io.github.bidabrain.mieru.ui.LocalNav
import io.github.bidabrain.mieru.ui.PageScaffold
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

@Composable
fun LogsScreen() {
    val lines = remember { mutableStateListOf<LogLine>() }
    val state = rememberLazyListState()
    val ctx = LocalContext.current
    val nav = LocalNav.current
    val fmt = remember { SimpleDateFormat("HH:mm:ss", Locale.US) }

    LaunchedEffect(Unit) {
        var after = 0L
        while (true) {
            val batch = withContext(Dispatchers.IO) { runCatching { Core.logs(after) }.getOrNull() }
            if (batch != null && batch.lines.isNotEmpty()) {
                val atBottom = !state.canScrollForward
                lines.addAll(batch.lines)
                if (lines.size > 1000) lines.removeRange(0, lines.size - 1000)
                after = batch.next
                if (atBottom) state.scrollToItem(lines.size)
            }
            delay(1000)
        }
    }

    PageScaffold(
        title = "运行日志",
        back = true,
        state = state,
        actions = {
            IconButton(onClick = {
                copyText(ctx, lines.joinToString("\n") { "${fmt.format(Date(it.time))} [${it.level}] ${it.msg}" })
                nav.toast("日志已复制")
            }) { Icon(Icons.Rounded.ContentCopy, "复制") }
            IconButton(onClick = { Core.clearLogs(); lines.clear() }) { Icon(Icons.Rounded.DeleteSweep, "清空") }
        },
    ) {
        if (lines.isEmpty()) {
            item { GlassCard { Text("暂无日志。连接后这里会显示内核输出。", color = MaterialTheme.colorScheme.onSurfaceVariant) } }
        }
        items(lines, key = { it.seq }) { l ->
            val color = when (l.level) {
                "error" -> MaterialTheme.colorScheme.error
                "warning" -> Color(0xFFD08A00)
                "debug" -> MaterialTheme.colorScheme.outline
                else -> MaterialTheme.colorScheme.onSurface
            }
            Row {
                Text(fmt.format(Date(l.time)), fontSize = 11.sp, fontFamily = FontFamily.Monospace, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.width(8.dp))
                Text(l.msg, fontSize = 12.sp, fontFamily = FontFamily.Monospace, color = color, lineHeight = 16.sp)
            }
        }
    }
}

