package tv.subcult.museum.ui.routes

import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import tv.subcult.museum.ui.AppViewModel

@Composable
fun RoutesScreen(model: AppViewModel, onNew: () -> Unit, onEdit: (String) -> Unit, onOpenTour: (String) -> Unit) { Text("Routes") }

@Composable
fun RouteBuilderScreen(model: AppViewModel, editingId: String?, onDone: () -> Unit) { Text("Builder") }
