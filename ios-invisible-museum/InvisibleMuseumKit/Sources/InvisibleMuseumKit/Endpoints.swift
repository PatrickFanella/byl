import Foundation

/// Typed calls for every public, auth, and /me route the app uses.
public struct Endpoints {
    public let client: APIClient

    public init(client: APIClient) {
        self.client = client
    }

    // Public
    public func cities() async throws -> [City] {
        let page: ListPage<City> = try await client.get("/api/v1/cities")
        return page.items
    }

    public func city(_ ref: String) async throws -> City {
        try await client.get("/api/v1/cities/\(ref)")
    }

    public func tours(city: String, topic: String? = nil, mode: String? = nil) async throws -> [TourSummary] {
        var q: [String: String] = [:]
        if let topic { q["topic"] = topic }
        if let mode { q["mode"] = mode }
        let page: ListPage<TourSummary> = try await client.get("/api/v1/cities/\(city)/tours", query: q)
        return page.items
    }

    public func places(city: String, near: (lat: Double, lng: Double)? = nil, radiusM: Int = 2000, topic: String? = nil) async throws -> [PlaceSummary] {
        var q: [String: String] = ["limit": "100"]
        if let near {
            q["near"] = "\(near.lat),\(near.lng)"
            q["radius_m"] = String(radiusM)
        }
        if let topic { q["topic"] = topic }
        var result: [PlaceSummary] = []
        var cursor = ""
        repeat {
            q["cursor"] = cursor
            let page: ListPage<PlaceSummary> = try await client.get("/api/v1/cities/\(city)/places", query: q)
            result.append(contentsOf: page.items)
            cursor = page.nextCursor
        } while !cursor.isEmpty && near == nil
        return result
    }

    public func place(_ ref: String, city: String? = nil) async throws -> PlaceDetail {
        var q: [String: String] = [:]
        if let city { q["city"] = city }
        return try await client.get("/api/v1/places/\(ref)", query: q)
    }

    public func moment(_ id: String) async throws -> MomentDetail {
        try await client.get("/api/v1/moments/\(id)")
    }

    public func tour(_ ref: String) async throws -> TourDetail {
        try await client.get("/api/v1/tours/\(ref)")
    }

    public func route(tour: String, mode: String) async throws -> TourRoute {
        try await client.get("/api/v1/tours/\(tour)/route", query: ["mode": mode])
    }

    // Auth
    public struct AppleSignIn: Encodable {
        public let identityToken: String
        public let displayName: String
        enum CodingKeys: String, CodingKey {
            case identityToken = "identity_token"
            case displayName = "display_name"
        }
        public init(identityToken: String, displayName: String) {
            self.identityToken = identityToken
            self.displayName = displayName
        }
    }

    public struct DevSignIn: Encodable {
        public let email: String
        public let displayName: String
        enum CodingKeys: String, CodingKey {
            case email
            case displayName = "display_name"
        }
        public init(email: String, displayName: String) {
            self.email = email
            self.displayName = displayName
        }
    }

    public func signInWithApple(identityToken: String, displayName: String) async throws -> Session {
        try await client.post("/api/v1/auth/apple", body: AppleSignIn(identityToken: identityToken, displayName: displayName))
    }

    public func signInDev(email: String, displayName: String) async throws -> Session {
        try await client.post("/api/v1/auth/dev", body: DevSignIn(email: email, displayName: displayName))
    }

    public func me() async throws -> Account {
        try await client.get("/api/v1/me")
    }

    public func signOut() async throws {
        try await client.delete("/api/v1/auth/session")
    }

    // Me
    public func myTours() async throws -> [TourSummary] {
        let page: ListPage<TourSummary> = try await client.get("/api/v1/me/tours")
        return page.items
    }

    public func createTour(_ body: CreateUserTourRequest) async throws -> TourDetail {
        try await client.post("/api/v1/me/tours", body: body)
    }

    public struct UpdateUserTour: Encodable {
        public var title: String?
        public var summary: String?
        public var modes: [String]?
        public var visibility: String?
        public var stops: [StopInput]?
        public init(title: String? = nil, summary: String? = nil, modes: [String]? = nil, visibility: String? = nil, stops: [StopInput]? = nil) {
            self.title = title
            self.summary = summary
            self.modes = modes
            self.visibility = visibility
            self.stops = stops
        }
    }

    public func updateTour(_ id: String, _ body: UpdateUserTour) async throws -> TourDetail {
        try await client.patch("/api/v1/me/tours/\(id)", body: body)
    }

    public func deleteTour(_ id: String) async throws {
        try await client.delete("/api/v1/me/tours/\(id)")
    }

    public func saved() async throws -> [TourSummary] {
        let page: ListPage<TourSummary> = try await client.get("/api/v1/me/saved")
        return page.items
    }

    public func save(tour: String) async throws {
        try await client.send("PUT", "/api/v1/me/saved/\(tour)", body: Optional<Int>.none)
    }

    public func unsave(tour: String) async throws {
        try await client.delete("/api/v1/me/saved/\(tour)")
    }

    public func progress() async throws -> [TourProgress] {
        let page: ListPage<TourProgress> = try await client.get("/api/v1/me/progress")
        return page.items
    }

    public func putProgress(tour: String, _ body: ProgressRequest) async throws -> TourProgress {
        try await client.put("/api/v1/me/progress/\(tour)", body: body)
    }
}
