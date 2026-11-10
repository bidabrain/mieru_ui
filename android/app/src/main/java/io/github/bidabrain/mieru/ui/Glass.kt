package io.github.bidabrain.mieru.ui

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import dev.chrisbanes.haze.HazeInput
import dev.chrisbanes.haze.HazeState
import dev.chrisbanes.haze.blur.hazeBlur
import dev.chrisbanes.haze.blur.HazeBlurStyle
import dev.chrisbanes.haze.blur.HazeColorEffect
import java.util.concurrent.ConcurrentHashMap
import io.github.bidabrain.mieru.ui.theme.LocalDarkTheme
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin

/** Background sampled by glass cards. */
val LocalBackdrop = staticCompositionLocalOf<HazeState?> { null }

/** Background + page content, sampled by the floating bars. */
val LocalChrome = staticCompositionLocalOf<HazeState?> { null }

/** Slowly drifting colour blobs: the "aurora" that the frosted glass blurs. */
@Composable
fun AuroraBackground(modifier: Modifier = Modifier, connected: Boolean = false) {
    val cs = MaterialTheme.colorScheme
    val dark = LocalDarkTheme.current
    val t by rememberInfiniteTransition(label = "aurora").animateFloat(
        initialValue = 0f, targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(24000, easing = LinearEasing), RepeatMode.Reverse),
        label = "t",
    )
    val a = if (dark) 0.55f else 0.42f
    val blobs = listOf(
        Blob(cs.primary, 0.15f, 0.12f, 0.75f, 0.0f),
        Blob(if (connected) cs.secondary else cs.tertiary, 0.95f, 0.30f, 0.65f, 0.33f),
        Blob(cs.secondary, 0.25f, 0.78f, 0.70f, 0.66f),
        Blob(cs.primaryContainer, 0.85f, 0.95f, 0.60f, 0.5f),
    )
    Canvas(modifier.fillMaxSize()) {
        drawRect(cs.background)
        val m = minOf(size.width, size.height)
        for (b in blobs) {
            val ph = 2 * PI * (t + b.phase)
            val c = Offset(
                size.width * (b.x + 0.10f * sin(ph).toFloat()),
                size.height * (b.y + 0.06f * cos(ph).toFloat()),
            )
            val r = m * b.r
            drawCircle(Brush.radialGradient(listOf(b.color.copy(alpha = a), Color.Transparent), c, r), r, c)
        }
    }
}

private class Blob(val color: Color, val x: Float, val y: Float, val r: Float, val phase: Float)

/** Frosted surface: real backdrop blur on Android 12+, translucent tint below. */
fun Modifier.frosted(
    state: HazeState?,
    shape: Shape,
    tint: Color,
    dark: Boolean,
    thick: Boolean = false,
): Modifier {
    val clipped = this.clip(shape)
    val glass = if (state != null) {
        clipped.hazeBlur(HazeInput.Sources(state), glassStyle(tint, dark, thick))
    } else {
        clipped.background(tint.copy(alpha = 0.85f))
    }
    return glass.border(
        1.dp,
        Brush.linearGradient(
            listOf(Color.White.copy(alpha = if (dark) 0.22f else 0.85f), Color.White.copy(alpha = if (dark) 0.04f else 0.25f)),
        ),
        shape,
    )
}

@Composable
fun GlassCard(
    modifier: Modifier = Modifier,
    shape: Shape = MaterialTheme.shapes.extraLarge,
    highlighted: Boolean = false,
    contentPadding: PaddingValues = PaddingValues(20.dp),
    onClick: (() -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    val cs = MaterialTheme.colorScheme
    val dark = LocalDarkTheme.current
    val tint = if (highlighted) cs.primaryContainer else cs.surface
    Column(
        modifier
            .frosted(LocalBackdrop.current, shape, tint, dark)
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(contentPadding),
        content = content,
    )
}

@Composable
fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(
        text,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.primary,
        fontWeight = FontWeight.SemiBold,
        modifier = modifier.padding(start = 8.dp, top = 8.dp, bottom = 4.dp),
    )
}

@Composable
fun IconBadge(icon: ImageVector, tint: Color = MaterialTheme.colorScheme.primary, modifier: Modifier = Modifier) {
    androidx.compose.foundation.layout.Box(
        modifier
            .size(40.dp)
            .clip(RoundedCornerShape(14.dp))
            .background(tint.copy(alpha = 0.16f)),
        contentAlignment = Alignment.Center,
    ) { Icon(icon, null, tint = tint, modifier = Modifier.size(22.dp)) }
}

@Composable
fun SettingRow(
    icon: ImageVector?,
    title: String,
    subtitle: String? = null,
    onClick: (() -> Unit)? = null,
    trailing: @Composable (() -> Unit)? = null,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .clip(MaterialTheme.shapes.large)
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(horizontal = 4.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (icon != null) {
            IconBadge(icon)
            Spacer(Modifier.width(14.dp))
        }
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            if (subtitle != null) {
                Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (trailing != null) {
            Spacer(Modifier.width(8.dp))
            trailing()
        }
    }
}

private val styleCache = ConcurrentHashMap<Triple<Color, Boolean, Boolean>, HazeBlurStyle>()

/** Haze styles compare by identity, so cache one per (tint, theme, thickness). */
private fun glassStyle(tint: Color, dark: Boolean, thick: Boolean): HazeBlurStyle =
    styleCache.getOrPut(Triple(tint, dark, thick)) {
        val alpha = when {
            thick && dark -> 0.72f
            thick -> 0.66f
            dark -> 0.50f
            else -> 0.42f
        }
        HazeBlurStyle {
            blurRadius(if (thick) 28.dp else 22.dp)
            noiseFactor(0.06f)
            colorEffects(listOf(HazeColorEffect.tint(tint.copy(alpha = alpha))))
            fallbackColorEffect(HazeColorEffect.tint(tint.copy(alpha = if (dark) 0.86f else 0.82f)))
        }
    }

/** The 見える mark, composed from the adaptive icon layers (painterResource can't load adaptive icons). */
@Composable
fun AppLogo(modifier: Modifier = Modifier) {
    androidx.compose.foundation.layout.Box(modifier.clip(androidx.compose.foundation.shape.CircleShape)) {
        androidx.compose.foundation.Image(
            androidx.compose.ui.res.painterResource(io.github.bidabrain.mieru.R.drawable.ic_launcher_background), null,
            Modifier.fillMaxSize(), contentScale = androidx.compose.ui.layout.ContentScale.Crop,
        )
        androidx.compose.foundation.Image(
            androidx.compose.ui.res.painterResource(io.github.bidabrain.mieru.R.drawable.ic_launcher_foreground), null,
            Modifier.fillMaxSize().graphicsLayerScale(1.35f),
        )
    }
}

private fun Modifier.graphicsLayerScale(s: Float) = this.then(
    Modifier.graphicsLayer(scaleX = s, scaleY = s),
)
