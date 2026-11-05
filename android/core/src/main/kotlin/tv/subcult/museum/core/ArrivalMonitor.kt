package tv.subcult.museum.core

import kotlin.math.asin
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

data class Point(val lat: Double, val lng: Double)

/** A location fix; `accuracyM` below zero means unknown. */
data class Fix(val lat: Double, val lng: Double, val accuracyM: Double, val timestampMillis: Long = System.currentTimeMillis()) {
    val point: Point get() = Point(lat, lng)
}

/** Fixes less accurate than this are ignored; a 200 m fix in a car park must never count. */
const val MAX_ACCURACY_M = 65.0

/** Haversine distance in metres. */
fun distanceM(a: Point, b: Point): Double {
    val r = 6_371_000.0
    val dLat = Math.toRadians(b.lat - a.lat)
    val dLng = Math.toRadians(b.lng - a.lng)
    val s = sin(dLat / 2).pow(2) + cos(Math.toRadians(a.lat)) * cos(Math.toRadians(b.lat)) * sin(dLng / 2).pow(2)
    return 2 * r * asin(min(1.0, sqrt(s)))
}

/** True when the fix is usable and within the stop's radius plus the fix's own uncertainty. */
fun hasArrived(fix: Fix, stop: Point, radiusM: Double): Boolean {
    if (kotlin.math.abs(System.currentTimeMillis() - fix.timestampMillis) > 30_000) return false
    if (fix.accuracyM < 0 || fix.accuracyM > MAX_ACCURACY_M) return false
    return distanceM(fix.point, stop) <= radiusM + fix.accuracyM
}

/** Decides when the visitor has reached a stop; fires once per stop until retargeted. */
class ArrivalMonitor(stop: Point, radiusM: Double) {
    var stop: Point = stop
        private set
    var radiusM: Double = radiusM
        private set
    var hasArrived: Boolean = false
        private set

    fun evaluate(fix: Fix): Boolean = hasArrived(fix, stop, radiusM)

    fun distance(fix: Fix): Double = distanceM(fix.point, stop)

    /** Returns true the first time the stop is reached. */
    fun update(fix: Fix): Boolean {
        if (hasArrived || !evaluate(fix)) return false
        hasArrived = true
        return true
    }

    fun retarget(stop: Point, radiusM: Double) {
        this.stop = stop
        this.radiusM = radiusM
        hasArrived = false
    }
}
