package tv.subcult.museum.core

import java.net.URLEncoder

/** Turn-by-turn in Google Maps (walking or cycling); resolve before launching. */
fun navigationUri(lat: Double, lng: Double, mode: String): String =
    "google.navigation:q=$lat,$lng&mode=${if (mode == "bike") "b" else "w"}"

/** Any maps app: a labelled pin at the stop. */
fun geoUri(lat: Double, lng: Double, name: String): String =
    "geo:$lat,$lng?q=$lat,$lng(${URLEncoder.encode(name, "UTF-8").replace("+", "%20")})"
