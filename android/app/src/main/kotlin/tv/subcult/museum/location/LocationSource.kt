package tv.subcult.museum.location

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Build
import android.os.SystemClock
import androidx.core.content.ContextCompat
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import tv.subcult.museum.core.Fix

/** LocationManager wrapped as a StateFlow of fixes; started while a screen that needs it is shown. */
class LocationSource(private val context: Context) {
    private val manager = context.getSystemService(Context.LOCATION_SERVICE) as LocationManager
    private val _fixes = MutableStateFlow<Fix?>(null)
    val fixes: StateFlow<Fix?> = _fixes
    private var listening = false

    // All four callbacks: on API 26–29 the status/provider methods are abstract and a SAM lambda
    // would throw AbstractMethodError when location is toggled.
    private val listener = object : LocationListener {
        override fun onLocationChanged(location: Location) {
            val ageMillis = (SystemClock.elapsedRealtimeNanos() - location.elapsedRealtimeNanos) / 1_000_000
            if (ageMillis !in 0..30_000) return
            _fixes.value = Fix(location.latitude, location.longitude, if (location.hasAccuracy()) location.accuracy.toDouble() else -1.0, System.currentTimeMillis() - ageMillis)
        }
        @Deprecated("Deprecated in Java")
        override fun onStatusChanged(provider: String?, status: Int, extras: android.os.Bundle?) {}
        override fun onProviderEnabled(provider: String) {}
        override fun onProviderDisabled(provider: String) {}
    }

    fun hasPermission(): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED

    @Suppress("MissingPermission")
    fun start() {
        if (listening || !hasPermission()) return
        val precise = ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        val provider = when {
            !precise -> LocationManager.NETWORK_PROVIDER
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.S && manager.isProviderEnabled(LocationManager.FUSED_PROVIDER) -> LocationManager.FUSED_PROVIDER
            manager.isProviderEnabled(LocationManager.GPS_PROVIDER) -> LocationManager.GPS_PROVIDER
            else -> LocationManager.NETWORK_PROVIDER
        }
        runCatching {
            manager.getLastKnownLocation(provider)?.let { listener.onLocationChanged(it) }
            manager.requestLocationUpdates(provider, 2000L, 3f, listener, context.mainLooper)
            listening = true
        }
    }

    fun stop() {
        if (!listening) return
        manager.removeUpdates(listener)
        listening = false
        _fixes.value = null
    }
}
