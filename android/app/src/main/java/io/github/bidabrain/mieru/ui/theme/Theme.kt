package io.github.bidabrain.mieru.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialExpressiveTheme
import androidx.compose.material3.MotionScheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp

private val Light = lightColorScheme(
    primary = Color(0xFF5B3FD0),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFE6DEFF),
    onPrimaryContainer = Color(0xFF1C0A5E),
    secondary = Color(0xFF00838F),
    onSecondary = Color.White,
    secondaryContainer = Color(0xFFC9F1F4),
    onSecondaryContainer = Color(0xFF002024),
    tertiary = Color(0xFFC2457F),
    tertiaryContainer = Color(0xFFFFD9E6),
    onTertiaryContainer = Color(0xFF3E0021),
    background = Color(0xFFF6F3FC),
    surface = Color(0xFFFBF8FF),
    surfaceVariant = Color(0xFFE5E0F0),
    onSurfaceVariant = Color(0xFF48454F),
    outline = Color(0xFF79757F),
    outlineVariant = Color(0xFFCAC4D0),
    error = Color(0xFFBA1A1A),
)

private val Dark = darkColorScheme(
    primary = Color(0xFFCABEFF),
    onPrimary = Color(0xFF2E1A8A),
    primaryContainer = Color(0xFF4430B5),
    onPrimaryContainer = Color(0xFFE6DEFF),
    secondary = Color(0xFF7FD8E0),
    onSecondary = Color(0xFF00363B),
    secondaryContainer = Color(0xFF004F56),
    onSecondaryContainer = Color(0xFFC9F1F4),
    tertiary = Color(0xFFFFB0CC),
    tertiaryContainer = Color(0xFF7D2A55),
    onTertiaryContainer = Color(0xFFFFD9E6),
    background = Color(0xFF0E0B1A),
    surface = Color(0xFF15121F),
    surfaceVariant = Color(0xFF2C2838),
    onSurfaceVariant = Color(0xFFCAC4D0),
    outline = Color(0xFF938F99),
    outlineVariant = Color(0xFF48454F),
    error = Color(0xFFFFB4AB),
)

val LocalDarkTheme = staticCompositionLocalOf { false }

private val AppShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(18.dp),
    large = RoundedCornerShape(26.dp),
    extraLarge = RoundedCornerShape(34.dp),
)

@Composable
fun MieruTheme(themeMode: String, dynamicColor: Boolean, content: @Composable () -> Unit) {
    val dark = when (themeMode) {
        "dark" -> true
        "light" -> false
        else -> isSystemInDarkTheme()
    }
    val ctx = LocalContext.current
    val scheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= 31 -> if (dark) dynamicDarkColorScheme(ctx) else dynamicLightColorScheme(ctx)
        dark -> Dark
        else -> Light
    }
    androidx.compose.runtime.CompositionLocalProvider(LocalDarkTheme provides dark) {
        MaterialExpressiveTheme(
            colorScheme = scheme,
            motionScheme = MotionScheme.expressive(),
            shapes = AppShapes,
            content = content,
        )
    }
}
