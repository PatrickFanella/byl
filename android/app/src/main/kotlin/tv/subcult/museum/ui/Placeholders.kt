package tv.subcult.museum.ui.tours

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import tv.subcult.museum.ui.AppViewModel

// Replaced in the next task (walk, routes, saved, profile). Kept so the NavHost compiles.
@Composable
fun WalkScreen(model: AppViewModel, onOpenMoment: (String) -> Unit, onEnded: () -> Unit) { Text("Walk") }
