import Foundation

/// Public reference and mode-specific access notes retained in the tour snapshot.
public struct ReviewedCorridor: Codable, Equatable {
    public struct Intent: Codable, Equatable {
        public struct Reference: Codable, Equatable {
            public let note: String?
        }
        public struct Access: Codable, Equatable {
            public let mode: String
            public let notes: String
        }
        public struct Segment: Codable, Equatable {
            public let id: String
            public let access: [Access]?
            public let geometry: LineString?
            public let signalReferences: [String: ReviewedSignalReference]?
            enum CodingKeys: String, CodingKey { case id, access, geometry; case signalReferences = "signal_references" }
            public init(from decoder: Decoder) throws {
                let c = try decoder.container(keyedBy: CodingKeys.self)
                id = try c.decode(String.self, forKey: .id)
                access = try c.decodeIfPresent([Access].self, forKey: .access)
                // Optional observation metadata must never break historical navigation.
                geometry = try? c.decodeIfPresent(LineString.self, forKey: .geometry)
                signalReferences = try? c.decodeIfPresent([String: ReviewedSignalReference].self, forKey: .signalReferences)
            }
        }
        public struct Selection: Codable, Equatable {
            public struct Traversal: Codable, Equatable {
                public let segmentID: String
                public let reverse: Bool?
                enum CodingKeys: String, CodingKey { case segmentID = "segment_id", reverse }
            }
            public let mode: String
            public let path: [Traversal]
        }
        public let source: Reference
        public let segments: [Segment]
        public let routes: [Selection]
    }
    public let intent: Intent?

    public func notices(mode: String) -> [String] {
        guard let intent else { return [] }
        let selected = Set(intent.routes.filter { $0.mode == mode }.flatMap { $0.path.map(\.segmentID) })
        let notes = [intent.source.note ?? ""] + intent.segments.filter { selected.contains($0.id) }
            .flatMap { ($0.access ?? []).filter { $0.mode == mode }.map(\.notes) }
        return notes.filter { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
            .reduce(into: []) { if !$0.contains($1) { $0.append($1) } }
    }
}

public struct ArrivalAccess: Codable, Equatable {
    public let mode: String
    public let status: String
    public let surface: String
    public let sourceURL: String
    public let sourceTitle: String
    public let retrievedAt: String
    public let notes: String
    enum CodingKeys: String, CodingKey {
        case mode, status, surface, notes
        case sourceURL = "source_url", sourceTitle = "source_title", retrievedAt = "retrieved_at"
    }
}

public struct ArrivalPoint: Codable, Equatable {
    public let name: String
    public let kind: String
    public let latitude: Double?
    public let longitude: Double?
    public let access: [ArrivalAccess]
    public let notes: String
}

public struct ApprovedArrival: Codable, Equatable {
    public let id: String
    public let version: Int
    public let point: ArrivalPoint
    public let reviewedAt: String
    enum CodingKeys: String, CodingKey {
        case id, version, point
        case reviewedAt = "reviewed_at"
    }

    public func allows(_ mode: String) -> Bool {
        guard let lat = point.latitude, let lng = point.longitude,
              lat.isFinite, lng.isFinite, (-90...90).contains(lat), (-180...180).contains(lng) else { return false }
        return point.access.filter { $0.mode == mode }.map(\.status) == ["allowed"]
    }
}

public struct TourSnapshot: Codable, Equatable {
    public let tour: TourDetail
    public let moments: [String: MomentDetail]
    public let routes: [String: TourRoute]
}

public struct TourRevision: Codable, Identifiable, Equatable {
    public let id: String
    public let tourID: String
    public let number: Int
    public let publishedAt: String
    public let snapshot: TourSnapshot?
    enum CodingKeys: String, CodingKey {
        case id, number, snapshot
        case tourID = "tour_id", publishedAt = "published_at"
    }

    /// A computed line alone cannot authorize starting a reviewed trip.
    public func unavailableReason(mode: String) -> String? {
        guard ["walk", "bike"].contains(mode), let snapshot, !id.isEmpty, snapshot.tour.id == tourID,
              snapshot.tour.tour.revisionID == id, snapshot.tour.tour.revisionNumber == number,
              snapshot.tour.tour.status == "published", snapshot.tour.tour.modes.contains(mode) else {
            return "The reviewed itinerary is unavailable for this mode."
        }
        let stops = snapshot.tour.stops
        guard (2...30).contains(stops.count), Set(stops.map(\.id)).count == stops.count,
              !stops.contains(where: { $0.id.isEmpty }), Set(stops.map(\.position)) == Set(1...stops.count) else {
            return "The reviewed stop identities could not be read."
        }
        guard let route = snapshot.routes[mode], route.mode == mode, route.tourID == tourID,
              route.status == "ready", let journey = route.journey, journey.version == 1,
              journey.usable, journey.state == "usable", journey.quality == "pass",
              let geometry = route.geometry, geometry.type == "LineString", geometry.coordinates.count >= 2,
              geometry.coordinates.allSatisfy({ $0.count == 2 && $0[0].isFinite && $0[1].isFinite && (-180...180).contains($0[0]) && (-90...90).contains($0[1]) }) else {
            return "The path is not ready for a reviewed trip in this mode."
        }
        guard stops.allSatisfy({ $0.place.arrivalPoint?.allows(mode) == true }) else {
            return "A reviewed arrival point is missing or unavailable for this mode."
        }
        guard stops.allSatisfy({ stop in
            guard let featured = stop.featuredMoment else { return true }
            return snapshot.moments[featured.id]?.summary.id == featured.id
        }) else { return "A retained story is missing from this revision." }
        return nil
    }
}

public struct RevisionProgress: Codable, Equatable {
    public let accountID: String
    public let tourID: String
    public let revisionID: String
    public let mode: String
    public let visitedStopIDs: [String]
    public let completedAt: String?
    enum CodingKeys: String, CodingKey {
        case mode
        case accountID = "account_id", tourID = "tour_id", revisionID = "revision_id"
        case visitedStopIDs = "visited_stop_ids", completedAt = "completed_at"
    }
}

public struct RevisionProgressRequest: Encodable {
    public let mode: String
    public let visitedStopIDs: [String]
    public let completed: Bool
    public var reset = false
    enum CodingKeys: String, CodingKey {
        case mode, completed, reset
        case visitedStopIDs = "visited_stop_ids"
    }

    public init(mode: String, visitedStopIDs: [String], completed: Bool, reset: Bool = false) {
        self.mode = mode
        self.visitedStopIDs = visitedStopIDs
        self.completed = completed
        self.reset = reset
    }
}
