package tv.subcult.museum.ui.tours

import android.graphics.Color
import androidx.compose.ui.graphics.toArgb
import tv.subcult.museum.ui.theme.Museum
import tv.subcult.museum.ui.theme.BylPalette
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraUpdateFactory
import org.maplibre.android.geometry.LatLng
import org.maplibre.android.geometry.LatLngBounds
import org.maplibre.android.maps.MapLibreMap
import org.maplibre.android.maps.MapView
import org.maplibre.android.maps.Style
import org.maplibre.android.style.layers.CircleLayer
import org.maplibre.android.style.layers.LineLayer
import org.maplibre.android.style.layers.Property
import org.maplibre.android.style.layers.PropertyFactory
import org.maplibre.android.style.layers.SymbolLayer
import org.maplibre.android.style.sources.GeoJsonSource
import org.maplibre.geojson.Feature
import org.maplibre.geojson.FeatureCollection
import org.maplibre.geojson.LineString
import org.maplibre.geojson.Point
import tv.subcult.museum.core.ActiveTourState
import tv.subcult.museum.core.Fix
import tv.subcult.museum.core.RouteLeg
import tv.subcult.museum.core.TourStop

private const val STYLE_URL = "https://tiles.openfreemap.org/styles/liberty"

/**
 * The route on a map: past legs faint, the active leg in the accent colour, future legs dashed,
 * numbered circles per stop (a check once visited, an action fill for the current stop), the visitor's position.
 * Drawn with style layers only, no annotation plugin. Re-fits only when the stops ahead change.
 */
@Composable
fun TourMap(stops: List<TourStop>, state: ActiveTourState, fix: Fix?, modifier: Modifier = Modifier) {
    val palette = Museum.palette
    val dark = isDarkPalette(palette)
    val context = LocalContext.current
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val holder = remember { MapHolder(context, palette, dark) }

    DisposableEffect(lifecycle) {
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_START -> holder.view.onStart()
                Lifecycle.Event.ON_RESUME -> holder.view.onResume()
                Lifecycle.Event.ON_PAUSE -> holder.view.onPause()
                Lifecycle.Event.ON_STOP -> holder.view.onStop()
                Lifecycle.Event.ON_DESTROY -> holder.destroy()
                else -> Unit
            }
        }
        lifecycle.addObserver(observer)
        onDispose {
            lifecycle.removeObserver(observer)
            holder.destroy()
        }
    }

    AndroidView(factory = { holder.view }, modifier = modifier, update = { holder.render(stops, state, fix, palette, dark) })
}

private fun isDarkPalette(p: BylPalette) = p.background == tv.subcult.museum.ui.theme.DarkBylPalette.background

private class MapHolder(context: android.content.Context, initialPalette: BylPalette, initialDark: Boolean) {
    val view: MapView
    private var map: MapLibreMap? = null
    private var style: Style? = null
    private var pending: (() -> Unit)? = null
    private var fitted = ""
    private var destroyed = false
    private var styleDark = initialDark

    /** Winds the view down once, through pause and stop, whichever path gets here first. */
    fun destroy() {
        if (destroyed) return
        destroyed = true
        runCatching { view.onPause(); view.onStop(); view.onDestroy() }
    }

    init {
        MapLibre.getInstance(context)
        view = MapView(context)
        view.onCreate(null)
        view.getMapAsync { m ->
            map = m
            m.uiSettings.isAttributionEnabled = true
            m.setStyle(Style.Builder().fromUri(if (initialDark) "https://tiles.openfreemap.org/styles/dark" else STYLE_URL)) { s ->
                style = s
                addLayers(s, initialPalette)
                val latest = pending
                pending = null
                latest?.invoke()
            }
        }
    }

    private fun addLayers(s: Style, p: BylPalette) {
        for (id in listOf("legs-past", "legs-future", "legs-active", "stops", "user")) s.addSource(GeoJsonSource(id))
        s.addLayer(LineLayer("legs-past", "legs-past").withProperties(PropertyFactory.lineColor(p.textSecondary.toArgb()), PropertyFactory.lineWidth(4f), PropertyFactory.lineOpacity(0.5f), PropertyFactory.lineCap(Property.LINE_CAP_ROUND)))
        s.addLayer(LineLayer("legs-future", "legs-future").withProperties(PropertyFactory.lineColor(p.textSecondary.toArgb()), PropertyFactory.lineWidth(4f), PropertyFactory.lineDasharray(arrayOf(1.5f, 2f)), PropertyFactory.lineCap(Property.LINE_CAP_ROUND)))
        s.addLayer(LineLayer("legs-active-halo", "legs-active").withProperties(PropertyFactory.lineColor(p.routeHalo.toArgb()), PropertyFactory.lineWidth(8f), PropertyFactory.lineCap(Property.LINE_CAP_ROUND)))
        s.addLayer(LineLayer("legs-active", "legs-active").withProperties(PropertyFactory.lineColor(p.route.toArgb()), PropertyFactory.lineWidth(4f), PropertyFactory.lineCap(Property.LINE_CAP_ROUND)))
        s.addLayer(CircleLayer("user", "user").withProperties(PropertyFactory.circleColor(p.route.toArgb()), PropertyFactory.circleRadius(8f), PropertyFactory.circleStrokeColor(p.routeHalo.toArgb()), PropertyFactory.circleStrokeWidth(3f)))
        s.addLayer(CircleLayer("stops-circle", "stops").withProperties(
            PropertyFactory.circleColor(org.maplibre.android.style.expressions.Expression.get("color")),
            PropertyFactory.circleRadius(14f), PropertyFactory.circleStrokeColor(p.routeHalo.toArgb()), PropertyFactory.circleStrokeWidth(2f)
        ))
        s.addLayer(SymbolLayer("stops-label", "stops").withProperties(
            PropertyFactory.textField(org.maplibre.android.style.expressions.Expression.get("label")),
            PropertyFactory.textSize(12f), PropertyFactory.textColor(org.maplibre.android.style.expressions.Expression.get("textColor")),
            PropertyFactory.textAllowOverlap(true), PropertyFactory.textIgnorePlacement(true)
        ))
    }

    fun render(stops: List<TourStop>, state: ActiveTourState, fix: Fix?, palette: BylPalette, dark: Boolean) {
        val s = style
        if (s == null) {
            pending = { render(stops, state, fix, palette, dark) }
            return
        }
        if (styleDark != dark) {
            styleDark = dark
            style = null
            pending = { render(stops, state, fix, palette, dark) }
            map?.setStyle(Style.Builder().fromUri(if (dark) "https://tiles.openfreemap.org/styles/dark" else STYLE_URL)) { loaded ->
                style = loaded
                addLayers(loaded, palette)
                val latest = pending
                pending = null
                latest?.invoke()
            }
            return
        }
        fun hex(c: androidx.compose.ui.graphics.Color) = String.format("#%06x", c.toArgb() and 0xffffff)
        fun legs(list: List<RouteLeg>) = FeatureCollection.fromFeatures(list.map { l ->
            Feature.fromGeometry(LineString.fromLngLats(l.geometry.filter { it.size >= 2 }.map { Point.fromLngLat(it[0], it[1]) }))
        })
        s.getSourceAs<GeoJsonSource>("legs-past")?.setGeoJson(legs(state.pastLegs))
        s.getSourceAs<GeoJsonSource>("legs-future")?.setGeoJson(legs(state.futureLegs))
        s.getSourceAs<GeoJsonSource>("legs-active")?.setGeoJson(legs(listOfNotNull(state.activeLeg)))
        s.getSourceAs<GeoJsonSource>("stops")?.setGeoJson(FeatureCollection.fromFeatures(stops.map { stop ->
            val visited = stop.position in state.visited
            val next = state.nextStop?.id == stop.id
            Feature.fromGeometry(Point.fromLngLat(stop.place.longitude, stop.place.latitude)).apply {
                addStringProperty("label", if (visited) "✓" else stop.position.toString())
                addStringProperty("color", hex(if (visited) palette.successSurface else if (next) palette.action else palette.background))
                addStringProperty("textColor", hex(if (visited) palette.success else if (next) palette.actionText else palette.text))
            }
        }))
        s.getSourceAs<GeoJsonSource>("user")?.setGeoJson(
            if (fix != null) FeatureCollection.fromFeature(Feature.fromGeometry(Point.fromLngLat(fix.lng, fix.lat))) else FeatureCollection.fromFeatures(emptyList())
        )
        // Frame the stops still ahead (all of them when done); never re-fit on a mere GPS update.
        val ahead = stops.filter { it.position !in state.visited }.ifEmpty { stops }
        val key = ahead.joinToString(",") { it.id }
        if (key != fitted && ahead.isNotEmpty()) {
            fitted = key
            val points = ahead.map { LatLng(it.place.latitude, it.place.longitude) } + listOfNotNull(fix?.let { LatLng(it.lat, it.lng) })
            map?.let { m ->
                if (points.size == 1) m.easeCamera(CameraUpdateFactory.newLatLngZoom(points[0], 15.0))
                else m.easeCamera(CameraUpdateFactory.newLatLngBounds(LatLngBounds.Builder().includes(points).build(), 96), 600)
            }
        }
    }
}
