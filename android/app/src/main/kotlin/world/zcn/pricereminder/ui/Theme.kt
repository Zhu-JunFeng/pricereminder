package world.zcn.pricereminder.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val LightColors = lightColorScheme(
    primary = Color(0xFF007F88), onPrimary = Color.White,
    primaryContainer = Color(0xFFD7F1F2), onPrimaryContainer = Color(0xFF102E31),
    background = Color.White, onBackground = Color(0xFF172A2E),
    surface = Color(0xFFF4F8F8), onSurface = Color(0xFF172A2E),
    outline = Color(0xFFD5DEDF), error = Color(0xFFC63E3E),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF65CED2), onPrimary = Color(0xFF00373B),
    primaryContainer = Color(0xFF174E52), onPrimaryContainer = Color(0xFFD7F1F2),
    background = Color(0xFF10191B), onBackground = Color(0xFFE7F0F0),
    surface = Color(0xFF172326), onSurface = Color(0xFFE7F0F0),
    outline = Color(0xFF3B4B4E), error = Color(0xFFFF8A85),
)

val RiseColor = Color(0xFF178A55)
val FallColor = Color(0xFFC63E3E)

@Composable
fun PriceReminderTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = if (isSystemInDarkTheme()) DarkColors else LightColors, content = content)
}
