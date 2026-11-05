package tv.subcult.museum.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/** The dark gallery look shared with iOS and the web. */
object Museum {
    val background = Color(0xFF0A0A0B)
    val surface = Color(0xFF121215)
    val surfaceElevated = Color(0xFF1B1B1F)
    val border = Color(0x17FFFFFF)
    val text = Color.White
    val muted = Color(0x8AFFFFFF)
    val faint = Color(0x57FFFFFF)
    val blue = Color(0xFF3B82F6)
    val purple = Color(0xFFA855F7)
    val green = Color(0xFF4ADE80)
    val danger = Color(0xFFDC3857)
}

private val scheme = darkColorScheme(
    primary = Museum.blue,
    onPrimary = Color.Black,
    secondary = Museum.purple,
    background = Museum.background,
    onBackground = Museum.text,
    surface = Museum.surface,
    onSurface = Museum.text,
    surfaceVariant = Museum.surfaceElevated,
    onSurfaceVariant = Museum.muted,
    outline = Museum.border,
    error = Museum.danger
)

private val typography = Typography(
    headlineLarge = TextStyle(fontSize = 30.sp, fontWeight = FontWeight.Bold, lineHeight = 36.sp),
    headlineMedium = TextStyle(fontSize = 24.sp, fontWeight = FontWeight.Bold, lineHeight = 30.sp),
    titleLarge = TextStyle(fontSize = 20.sp, fontWeight = FontWeight.SemiBold),
    titleMedium = TextStyle(fontSize = 16.sp, fontWeight = FontWeight.SemiBold),
    bodyLarge = TextStyle(fontSize = 16.sp, lineHeight = 24.sp),
    bodyMedium = TextStyle(fontSize = 14.sp, lineHeight = 20.sp),
    bodySmall = TextStyle(fontSize = 12.sp, lineHeight = 16.sp),
    labelSmall = TextStyle(fontSize = 11.sp, fontWeight = FontWeight.Bold, letterSpacing = 1.5.sp)
)

@Composable
fun InvisibleMuseumTheme(content: @Composable () -> Unit) {
    // The gallery is dark regardless of the system setting.
    isSystemInDarkTheme()
    MaterialTheme(colorScheme = scheme, typography = typography, content = content)
}
