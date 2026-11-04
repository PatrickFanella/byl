import Foundation

// Data transfer objects for the Invisible Museum API. Field names follow
// backend/api/openapi.yaml exactly through explicit CodingKeys.

public struct ListPage<T: Decodable>: Decodable {
    public let items: [T]
    public let nextCursor: String

    enum CodingKeys: String, CodingKey {
        case items
        case nextCursor = "next_cursor"
    }
}

public struct APIErrorBody: Decodable, Equatable {
    public struct Detail: Decodable, Equatable {
        public let code: String
        public let message: String
        public let fields: [String: String]?
    }
    public let error: Detail
}

public struct BoundingBox: Decodable, Equatable {
    public let minLat: Double
    public let minLng: Double
    public let maxLat: Double
    public let maxLng: Double

    enum CodingKeys: String, CodingKey {
        case minLat = "min_lat"
        case minLng = "min_lng"
        case maxLat = "max_lat"
        case maxLng = "max_lng"
    }
}

public struct City: Decodable, Identifiable, Equatable {
    public let id: String
    public let slug: String
    public let name: String
    public let region: String
    public let countryCode: String
    public let centerLat: Double
    public let centerLng: Double
    public let bbox: BoundingBox
    public let timezone: String
    public let topics: [Topic]?

    enum CodingKeys: String, CodingKey {
        case id, slug, name, region, bbox, timezone, topics
        case countryCode = "country_code"
        case centerLat = "center_lat"
        case centerLng = "center_lng"
    }
}

public struct Topic: Decodable, Identifiable, Equatable {
    public let id: String
    public let cityID: String
    public let slug: String
    public let name: String
    public let description: String
    public let momentCount: Int

    enum CodingKeys: String, CodingKey {
        case id, slug, name, description
        case cityID = "city_id"
        case momentCount = "moment_count"
    }
}

public struct MediaAsset: Decodable, Identifiable, Equatable {
    public let id: String
    public let kind: String
    public let publicURL: String
    public let mimeType: String
    public let width: Int?
    public let height: Int?
    public let durationSeconds: Double?
    public let caption: String
    public let credit: String
    public let license: String
    public let licenseURL: String
    public let sourceURL: String

    enum CodingKeys: String, CodingKey {
        case id, kind, width, height, caption, credit, license
        case publicURL = "public_url"
        case mimeType = "mime_type"
        case durationSeconds = "duration_seconds"
        case licenseURL = "license_url"
        case sourceURL = "source_url"
    }

    public var url: URL? { URL(string: publicURL) }
}

public struct PlaceSummary: Decodable, Identifiable, Equatable {
    public let id: String
    public let cityID: String
    public let slug: String
    public let name: String
    public let summary: String
    public let kind: String
    public let latitude: Double
    public let longitude: Double
    public let arrivalRadiusM: Double
    public let address: String
    public let status: String
    public let primaryMomentID: String?
    public let momentCount: Int
    public let distanceM: Double?

    enum CodingKeys: String, CodingKey {
        case id, slug, name, summary, kind, latitude, longitude, address, status
        case cityID = "city_id"
        case arrivalRadiusM = "arrival_radius_m"
        case primaryMomentID = "primary_moment_id"
        case momentCount = "moment_count"
        case distanceM = "distance_m"
    }
}

public struct PlaceDetail: Decodable, Identifiable, Equatable {
    public let place: PlaceSummary
    public let primaryMoment: MomentSummary?
    public let moments: [MomentSummary]

    public var id: String { place.id }

    enum CodingKeys: String, CodingKey {
        case primaryMoment = "primary_moment"
        case moments
    }

    public init(from decoder: Decoder) throws {
        place = try PlaceSummary(from: decoder)
        let container = try decoder.container(keyedBy: CodingKeys.self)
        primaryMoment = try container.decodeIfPresent(MomentSummary.self, forKey: .primaryMoment)
        moments = try container.decodeIfPresent([MomentSummary].self, forKey: .moments) ?? []
    }
}

public struct Source: Decodable, Equatable {
    public let title: String
    public let url: String
    public let publisher: String
    public let retrievedAt: String?
    public let license: String
    public let note: String

    enum CodingKeys: String, CodingKey {
        case title, url, publisher, license, note
        case retrievedAt = "retrieved_at"
    }
}

public struct MomentSummary: Decodable, Identifiable, Equatable {
    public let id: String
    public let placeID: String
    public let slug: String
    public let title: String
    public let excerpt: String
    public let dateStart: String?
    public let dateEnd: String?
    public let datePrecision: String
    public let dateDisplay: String
    public let heroMedia: MediaAsset?
    public let hasNarration: Bool
    public let status: String
    public let topicIDs: [String]

    enum CodingKeys: String, CodingKey {
        case id, slug, title, excerpt, status
        case placeID = "place_id"
        case dateStart = "date_start"
        case dateEnd = "date_end"
        case datePrecision = "date_precision"
        case dateDisplay = "date_display"
        case heroMedia = "hero_media"
        case hasNarration = "has_narration"
        case topicIDs = "topic_ids"
    }
}

public struct MomentDetail: Decodable, Identifiable, Equatable {
    public let summary: MomentSummary
    public let body: String
    public let media: [MediaAsset]
    public let sources: [Source]
    public let topics: [Topic]
    public let narration: MediaAsset?

    public var id: String { summary.id }

    enum CodingKeys: String, CodingKey {
        case body, media, sources, topics, narration
    }

    public init(from decoder: Decoder) throws {
        summary = try MomentSummary(from: decoder)
        let container = try decoder.container(keyedBy: CodingKeys.self)
        body = try container.decodeIfPresent(String.self, forKey: .body) ?? ""
        media = try container.decodeIfPresent([MediaAsset].self, forKey: .media) ?? []
        sources = try container.decodeIfPresent([Source].self, forKey: .sources) ?? []
        topics = try container.decodeIfPresent([Topic].self, forKey: .topics) ?? []
        narration = try container.decodeIfPresent(MediaAsset.self, forKey: .narration)
    }
}

public struct RouteSummary: Decodable, Equatable {
    public let mode: String
    public let status: String
    public let distanceM: Double
    public let durationS: Int
    public let estimatedMinutes: Int?
    public let error: String?

    enum CodingKeys: String, CodingKey {
        case mode, status, error
        case distanceM = "distance_m"
        case durationS = "duration_s"
        case estimatedMinutes = "estimated_minutes"
    }
}

public struct RouteLeg: Decodable, Equatable {
    public let fromPosition: Int
    public let toPosition: Int
    public let distanceM: Double
    public let durationS: Int
    public let geometry: [[Double]]

    enum CodingKeys: String, CodingKey {
        case geometry
        case fromPosition = "from_position"
        case toPosition = "to_position"
        case distanceM = "distance_m"
        case durationS = "duration_s"
    }
}

public struct LineString: Decodable, Equatable {
    public let type: String
    public let coordinates: [[Double]]
}

public struct TourRoute: Decodable, Equatable {
    public let id: String
    public let tourID: String
    public let mode: String
    public let status: String
    public let provider: String
    public let distanceM: Double
    public let durationS: Int
    public let geometry: LineString?
    public let legs: [RouteLeg]
    public let error: String?

    enum CodingKeys: String, CodingKey {
        case id, mode, status, provider, geometry, legs, error
        case tourID = "tour_id"
        case distanceM = "distance_m"
        case durationS = "duration_s"
    }
}

public struct TourSummary: Decodable, Identifiable, Equatable {
    public let id: String
    public let cityID: String
    public let slug: String
    public let title: String
    public let summary: String
    public let topicID: String?
    public let topic: Topic?
    public let modes: [String]
    public let heroMedia: MediaAsset?
    public let status: String
    public let visibility: String
    public let ownerAccountID: String?
    public let stopCount: Int
    public let routes: [String: RouteSummary]

    enum CodingKeys: String, CodingKey {
        case id, slug, title, summary, topic, modes, status, visibility, routes
        case cityID = "city_id"
        case topicID = "topic_id"
        case heroMedia = "hero_media"
        case ownerAccountID = "owner_account_id"
        case stopCount = "stop_count"
    }

    public var isUserRoute: Bool { ownerAccountID != nil }
}

public struct TourStop: Decodable, Identifiable, Equatable {
    public let id: String
    public let position: Int
    public let place: PlaceSummary
    public let featuredMomentID: String?
    public let featuredMoment: MomentSummary?
    public let note: String

    enum CodingKeys: String, CodingKey {
        case id, position, place, note
        case featuredMomentID = "featured_moment_id"
        case featuredMoment = "featured_moment"
    }
}

public struct TourDetail: Decodable, Identifiable, Equatable {
    public let tour: TourSummary
    public let stops: [TourStop]

    public var id: String { tour.id }

    enum CodingKeys: String, CodingKey {
        case stops
    }

    public init(from decoder: Decoder) throws {
        tour = try TourSummary(from: decoder)
        let container = try decoder.container(keyedBy: CodingKeys.self)
        stops = try container.decodeIfPresent([TourStop].self, forKey: .stops) ?? []
    }
}

public struct TourProgress: Decodable, Equatable {
    public let accountID: String
    public let tourID: String
    public let mode: String
    public let visitedPositions: [Int]
    public let startedAt: String
    public let completedAt: String?

    enum CodingKeys: String, CodingKey {
        case mode
        case accountID = "account_id"
        case tourID = "tour_id"
        case visitedPositions = "visited_positions"
        case startedAt = "started_at"
        case completedAt = "completed_at"
    }
}

public struct Account: Codable, Identifiable, Equatable {
    public let id: String
    public let provider: String
    public let email: String
    public let displayName: String
    public let role: String

    enum CodingKeys: String, CodingKey {
        case id, provider, email, role
        case displayName = "display_name"
    }

    public init(id: String, provider: String, email: String, displayName: String, role: String) {
        self.id = id
        self.provider = provider
        self.email = email
        self.displayName = displayName
        self.role = role
    }
}

public struct Session: Decodable, Equatable {
    public let token: String
    public let expiresAt: String
    public let account: Account

    enum CodingKeys: String, CodingKey {
        case token, account
        case expiresAt = "expires_at"
    }
}

// Request bodies

public struct StopInput: Encodable, Equatable {
    public var placeID: String
    public var note: String

    enum CodingKeys: String, CodingKey {
        case placeID = "place_id"
        case note
    }

    public init(placeID: String, note: String = "") {
        self.placeID = placeID
        self.note = note
    }
}

public struct CreateUserTourRequest: Encodable {
    public var city: String
    public var title: String
    public var summary: String
    public var modes: [String]
    public var visibility: String
    public var stops: [StopInput]

    public init(city: String, title: String, summary: String = "", modes: [String] = ["walk", "bike"], visibility: String = "unlisted", stops: [StopInput]) {
        self.city = city
        self.title = title
        self.summary = summary
        self.modes = modes
        self.visibility = visibility
        self.stops = stops
    }
}

public struct ProgressRequest: Encodable {
    public var mode: String
    public var visitedPositions: [Int]
    public var completed: Bool

    enum CodingKeys: String, CodingKey {
        case mode, completed
        case visitedPositions = "visited_positions"
    }

    public init(mode: String, visitedPositions: [Int], completed: Bool) {
        self.mode = mode
        self.visitedPositions = visitedPositions
        self.completed = completed
    }
}
