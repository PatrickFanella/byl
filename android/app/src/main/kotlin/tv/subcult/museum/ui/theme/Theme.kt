package tv.subcult.museum.ui.theme

import android.content.Context
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.*
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import tv.subcult.museum.R

private val LocalPalette = staticCompositionLocalOf { LightBylPalette }
enum class Appearance { System, Light, Dark }
class AppearancePreference(context: Context) {
    private val prefs = context.getSharedPreferences("byl.appearance", Context.MODE_PRIVATE)
    var value by mutableStateOf(Appearance.entries.firstOrNull { it.name == prefs.getString("mode", null) } ?: Appearance.System)
        private set
    fun set(value: Appearance) {
        this.value = value
        prefs.edit().putString("mode", value.name).apply()
    }
}
val LocalAppearance = staticCompositionLocalOf<AppearancePreference> { error("No appearance provider") }

object Museum {
    val palette: BylPalette @Composable get() = LocalPalette.current
    val background: Color @Composable get() = palette.background
    val surface: Color @Composable get() = palette.surface
    val surfaceElevated: Color @Composable get() = palette.surface
    val border: Color @Composable get() = palette.borderSubtle
    val borderControl: Color @Composable get() = palette.borderControl
    val text: Color @Composable get() = palette.text
    val muted: Color @Composable get() = palette.textSecondary
    val faint: Color @Composable get() = palette.textSecondary
    val action: Color @Composable get() = palette.action
    val actionText: Color @Composable get() = palette.actionText
    val success: Color @Composable get() = palette.success
    val successSurface: Color @Composable get() = palette.successSurface
    val info: Color @Composable get() = palette.info
    val danger: Color @Composable get() = palette.danger
}

private val body = FontFamily(Font(R.font.public_sans_regular), Font(R.font.public_sans_bold, FontWeight.Bold))
private val display = FontFamily(Font(R.font.libre_baskerville))
val campaignFont = FontFamily(Font(R.font.barlow_condensed_bold, FontWeight.Bold))
private val typography = Typography(
    headlineLarge = TextStyle(fontFamily = display, fontSize = 32.sp, lineHeight = 40.sp),
    headlineMedium = TextStyle(fontFamily = display, fontSize = 28.sp, lineHeight = 36.sp),
    titleLarge = TextStyle(fontFamily = display, fontSize = 22.sp, lineHeight = 30.sp),
    titleMedium = TextStyle(fontFamily = body, fontSize = 16.sp, fontWeight = FontWeight.Bold, lineHeight = 26.sp),
    bodyLarge = TextStyle(fontFamily = body, fontSize = 18.sp, lineHeight = 30.sp),
    bodyMedium = TextStyle(fontFamily = body, fontSize = 16.sp, lineHeight = 26.sp),
    bodySmall = TextStyle(fontFamily = body, fontSize = 14.sp, lineHeight = 20.sp),
    labelLarge = TextStyle(fontFamily = body, fontSize = 16.sp, fontWeight = FontWeight.Bold, lineHeight = 26.sp),
    labelMedium = TextStyle(fontFamily = body, fontSize = 14.sp, fontWeight = FontWeight.Bold, lineHeight = 20.sp),
    labelSmall = TextStyle(fontFamily = body, fontSize = 12.sp, fontWeight = FontWeight.Bold, lineHeight = 16.sp)
)

@Composable
fun InvisibleMuseumTheme(content: @Composable () -> Unit) {
    val context = LocalContext.current
    val preference = remember(context) { AppearancePreference(context) }
    val dark = when (preference.value) {
        Appearance.System -> isSystemInDarkTheme()
        Appearance.Light -> false
        Appearance.Dark -> true
    }
    SideEffect {
        (context as? android.app.Activity)?.window?.let { window ->
            val controller = androidx.core.view.WindowCompat.getInsetsController(window, window.decorView)
            controller.isAppearanceLightStatusBars = !dark
            controller.isAppearanceLightNavigationBars = !dark
        }
    }
    val p = if (dark) DarkBylPalette else LightBylPalette
    val base = if (dark) darkColorScheme() else lightColorScheme()
    val scheme = base.copy(
        primary = p.action, onPrimary = p.actionText,
        primaryContainer = p.surface, onPrimaryContainer = p.text,
        secondary = p.action, onSecondary = p.actionText,
        secondaryContainer = p.surface, onSecondaryContainer = p.text,
        tertiary = p.info, onTertiary = p.infoSurface,
        background = p.background, onBackground = p.text,
        surface = p.background, onSurface = p.text,
        surfaceVariant = p.surface, onSurfaceVariant = p.textSecondary,
        surfaceContainer = p.surface, surfaceContainerHigh = p.surface, surfaceContainerHighest = p.surface,
        outline = p.borderControl, outlineVariant = p.borderSubtle,
        error = p.danger, onError = p.dangerSurface,
        errorContainer = p.dangerSurface, onErrorContainer = p.danger
    )
    CompositionLocalProvider(LocalPalette provides p, LocalAppearance provides preference) {
        MaterialTheme(
            colorScheme = scheme, typography = typography,
            shapes = Shapes(extraSmall = RoundedCornerShape(4.dp), small = RoundedCornerShape(4.dp),
                medium = RoundedCornerShape(8.dp), large = RoundedCornerShape(8.dp), extraLarge = RoundedCornerShape(8.dp)),
            content = content
        )
    }
}
