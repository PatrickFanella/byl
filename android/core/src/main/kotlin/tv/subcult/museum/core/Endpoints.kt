package tv.subcult.museum.core

/** Typed calls for every public, auth, and /me route the app uses. */
class Endpoints(val client: ApiClient) {
    // Public
    suspend fun cities(): List<City> = client.get<ListPage<City>>("/api/v1/cities").items

    suspend fun city(ref: String): City = client.get("/api/v1/cities/$ref")

    suspend fun tours(city: String, topic: String? = null, mode: String? = null): List<TourSummary> {
        val q = buildMap {
            if (topic != null) put("topic", topic)
            if (mode != null) put("mode", mode)
        }
        return client.get<ListPage<TourSummary>>("/api/v1/cities/$city/tours", q).items
    }

    suspend fun places(city: String, near: Pair<Double, Double>? = null, radiusM: Int = 2000, topic: String? = null): List<PlaceSummary> {
        val q = buildMap {
            put("limit", "100")
            if (near != null) {
                put("near", "${near.first},${near.second}")
                put("radius_m", radiusM.toString())
            }
            if (topic != null) put("topic", topic)
        }
        return client.get<ListPage<PlaceSummary>>("/api/v1/cities/$city/places", q).items
    }

    suspend fun place(ref: String, city: String? = null): PlaceDetail =
        client.get("/api/v1/places/$ref", if (city != null) mapOf("city" to city) else emptyMap())

    suspend fun moment(id: String): MomentDetail = client.get("/api/v1/moments/$id")

    suspend fun tour(ref: String): TourDetail = client.get("/api/v1/tours/$ref")

    suspend fun route(tour: String, mode: String): TourRoute =
        client.get("/api/v1/tours/$tour/route", mapOf("mode" to mode))

    // Auth
    suspend fun signInGoogle(idToken: String, displayName: String = ""): Session =
        client.post("/api/v1/auth/google", GoogleSignInRequest(idToken, displayName))

    suspend fun signInDev(email: String, displayName: String): Session =
        client.post("/api/v1/auth/dev", DevSignInRequest(email, displayName))

    suspend fun me(): Account = client.get("/api/v1/me")

    suspend fun signOut() = client.delete("/api/v1/auth/session")

    // Me
    suspend fun myTours(): List<TourSummary> = client.get<ListPage<TourSummary>>("/api/v1/me/tours").items

    suspend fun createTour(body: CreateUserTourRequest): TourDetail = client.post("/api/v1/me/tours", body)

    suspend fun updateTour(id: String, body: UpdateUserTourRequest): TourDetail = client.patch("/api/v1/me/tours/$id", body)

    suspend fun deleteTour(id: String) = client.delete("/api/v1/me/tours/$id")

    suspend fun saved(): List<TourSummary> = client.get<ListPage<TourSummary>>("/api/v1/me/saved").items

    suspend fun save(tour: String) = client.send("PUT", "/api/v1/me/saved/$tour")

    suspend fun unsave(tour: String) = client.delete("/api/v1/me/saved/$tour")

    suspend fun progress(): List<TourProgress> = client.get<ListPage<TourProgress>>("/api/v1/me/progress").items

    suspend fun putProgress(tour: String, body: ProgressRequest): TourProgress = client.put("/api/v1/me/progress/$tour", body)
}
