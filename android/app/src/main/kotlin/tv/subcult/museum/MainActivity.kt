@file:OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)

package tv.subcult.museum

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.DirectionsWalk
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.Museum
import androidx.compose.material.icons.filled.Route
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavHostController
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import tv.subcult.museum.ui.AppViewModel
import tv.subcult.museum.ui.home.HomeScreen
import tv.subcult.museum.ui.moments.MomentScreen
import tv.subcult.museum.ui.places.PlaceScreen
import tv.subcult.museum.ui.profile.ProfileScreen
import tv.subcult.museum.ui.routes.RouteBuilderScreen
import tv.subcult.museum.ui.routes.RoutesScreen
import tv.subcult.museum.ui.routes.RouteSuggestionsScreen
import tv.subcult.museum.ui.routes.RouteReportsScreen
import tv.subcult.museum.ui.saved.SavedScreen
import tv.subcult.museum.ui.signin.SignInScreen
import tv.subcult.museum.ui.theme.InvisibleMuseumTheme
import tv.subcult.museum.ui.theme.Museum
import tv.subcult.museum.ui.tours.TourDetailScreen
import tv.subcult.museum.ui.tours.TourListScreen
import tv.subcult.museum.ui.tours.WalkScreen

class MainActivity : ComponentActivity() {
    private val model: AppViewModel by viewModels { AppViewModel.Factory((application as MuseumApp).container) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        // Only on a fresh start: after a rotation the same intent would push the tour again.
        if (savedInstanceState == null) handleLink(intent)
        setContent { InvisibleMuseumTheme { MuseumNavHost(model) } }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleLink(intent)
    }

    override fun onResume() {
        super.onResume()
        model.onResume()
    }

    override fun onStop() {
        super.onStop()
        model.onStop()
    }

    /** invisiblemuseum://tour/{slug} and https://host/t/{slug}. */
    private fun handleLink(intent: Intent?) {
        val uri = intent?.data ?: return
        val segments = uri.pathSegments
        val slug = when {
            uri.scheme == "invisiblemuseum" && uri.host == "tour" -> segments.firstOrNull()
            segments.size >= 2 && segments[0] == "t" -> segments[1]
            else -> null
        }
        // Anything a web page can put in a BROWSABLE link reaches here; accept only slug characters.
        if (slug != null && SLUG.matches(slug)) model.openTourLink(slug)
    }
}

private val SLUG = Regex("[A-Za-z0-9_-]{1,64}")

private data class Tab(val route: String, val label: String, val icon: ImageVector)

private val tabs = listOf(
    Tab("home", "Home", Icons.Default.Museum),
    Tab("tours", "Tours", Icons.Default.DirectionsWalk),
    Tab("routes", "Routes", Icons.Default.Route),
    Tab("saved", "Saved", Icons.Default.Favorite),
    Tab("profile", "Profile", Icons.Default.AccountCircle)
)

@Composable
fun MuseumNavHost(model: AppViewModel) {
    val nav = rememberNavController()
    val state by model.state.collectAsStateWithLifecycle()
    val backStack by nav.currentBackStackEntryAsState()
    val route = backStack?.destination?.route ?: "home"
    val isWalk = route.startsWith("walk/")

    LaunchedEffect(state.pendingTourSlug) {
        val slug = state.pendingTourSlug ?: return@LaunchedEffect
        nav.navigate("tours/$slug")
        model.consumeTourLink()
    }
    // The active tour takes the whole screen until it ends.
    LaunchedEffect(state.active?.let { System.identityHashCode(it) }) {
        val active = state.active ?: return@LaunchedEffect
        if (!isWalk) nav.navigate("walk/${active.tour.slug}?mode=${active.mode}") { launchSingleTop = true }
    }

    Scaffold(
        containerColor = Museum.background,
        bottomBar = {
            if (!isWalk) {
                NavigationBar(containerColor = Museum.surface) {
                    tabs.forEach { tab ->
                        NavigationBarItem(
                            selected = route == tab.route || route.startsWith(tab.route + "/"),
                            onClick = { nav.navigateTab(tab.route) },
                            icon = { Icon(tab.icon, contentDescription = tab.label) },
                            label = { Text(tab.label, style = MaterialTheme.typography.labelSmall, maxLines = 1, softWrap = false, overflow = TextOverflow.Ellipsis) },
                            colors = NavigationBarItemDefaults.colors(selectedIconColor = Museum.actionText, indicatorColor = Museum.action, selectedTextColor = Museum.text, unselectedIconColor = Museum.muted, unselectedTextColor = Museum.muted)
                        )
                    }
                }
            }
        }
    ) { padding ->
        NavHost(nav, startDestination = "home", modifier = Modifier.padding(if (isWalk) PaddingValues(0.dp) else padding)) {
            composable("home") { HomeScreen(model, onOpenTour = { nav.navigate("tours/$it") }, onOpenPlace = { nav.navigate("places/$it") }) }
            composable("tours") { TourListScreen(model, onOpenTour = { nav.navigate("tours/$it") }) }
            composable("tours/{ref}", arguments = listOf(navArgument("ref") { type = NavType.StringType })) { entry ->
                TourDetailScreen(
                    model, ref = entry.arguments?.getString("ref") ?: "",
                    onOpenPlace = { id, featured -> nav.navigate("places/$id" + (featured?.let { "?featured=$it" } ?: "")) },
                    onBack = { nav.popBackStack() },
                    onReport = { tour, revision, mode -> nav.navigate("route-report/$tour/$revision?mode=$mode") }
                )
            }
            composable(
                "walk/{ref}?mode={mode}",
                arguments = listOf(navArgument("ref") { type = NavType.StringType }, navArgument("mode") { type = NavType.StringType; defaultValue = "walk" })
            ) {
                WalkScreen(model, onOpenMoment = { nav.navigate("trip-story/${state.active?.revision?.id ?: "legacy"}/$it") }, onEnded = { nav.navigateTab("tours") })
            }
            composable("trip-story/{revision}/{id}", arguments = listOf(navArgument("revision") { type = NavType.StringType }, navArgument("id") { type = NavType.StringType })) { entry ->
                val active = state.active
                val tripState = active?.state?.collectAsStateWithLifecycle()?.value
                val revision = entry.arguments?.getString("revision") ?: ""
                val id = entry.arguments?.getString("id") ?: ""
                val retained = if (active?.canUse == true && tripState?.revoked == false && active.revision?.id == revision) active.retainedMoment(id) else null
                MomentScreen(model, id, retained = retained, revisionOnly = true, revisionKey = revision)
            }
            composable(
                "places/{ref}?featured={featured}",
                arguments = listOf(navArgument("ref") { type = NavType.StringType }, navArgument("featured") { type = NavType.StringType; nullable = true })
            ) { entry ->
                PlaceScreen(model, ref = entry.arguments?.getString("ref") ?: "", featuredId = entry.arguments?.getString("featured"), onOpenMoment = { nav.navigate("moments/$it") })
            }
            composable("moments/{id}", arguments = listOf(navArgument("id") { type = NavType.StringType })) { entry ->
                MomentScreen(model, id = entry.arguments?.getString("id") ?: "")
            }
            composable("routes") { RoutesScreen(model, onNew = { nav.navigate("routes/new") }, onEdit = { nav.navigate("routes/$it/edit") }, onOpenTour = { nav.navigate("tours/$it") }, onSuggestions = { nav.navigate("routes/suggestions") }, onReports = { nav.navigate("routes/reports") }) }
            composable("routes/suggestions") { RouteSuggestionsScreen(model, onBack = { nav.popBackStack() }) }
            composable("routes/reports") { RouteReportsScreen(model, onBack = { nav.popBackStack() }, onOpenTour = { nav.navigate("tours/$it") }) }
            composable("route-report/{tour}/{revision}?mode={mode}", arguments = listOf(navArgument("tour") { type = NavType.StringType }, navArgument("revision") { type = NavType.StringType }, navArgument("mode") { type = NavType.StringType; defaultValue = "walk" })) { entry ->
                RouteReportsScreen(model, onBack = { nav.popBackStack() }, onOpenTour = { nav.navigate("tours/$it") }, tourId = entry.arguments?.getString("tour"), revisionId = entry.arguments?.getString("revision"), mode = entry.arguments?.getString("mode") ?: "walk")
            }
            composable("routes/new") { RouteBuilderScreen(model, editingId = null, onDone = { nav.popBackStack() }) }
            composable("routes/{id}/edit", arguments = listOf(navArgument("id") { type = NavType.StringType })) { entry ->
                RouteBuilderScreen(model, editingId = entry.arguments?.getString("id"), onDone = { nav.popBackStack() })
            }
            composable("saved") { SavedScreen(model, onOpenTour = { nav.navigate("tours/$it") }) }
            composable("profile") { ProfileScreen(model) }
        }
    }

    if (state.showSignIn) {
        ModalBottomSheet(onDismissRequest = { model.showSignIn(false) }, containerColor = Museum.background) {
            SignInScreen(model)
        }
    }
}

private fun NavHostController.navigateTab(route: String) {
    navigate(route) {
        popUpTo(graph.findStartDestination().id) { saveState = true }
        launchSingleTop = true
        restoreState = true
    }
}
