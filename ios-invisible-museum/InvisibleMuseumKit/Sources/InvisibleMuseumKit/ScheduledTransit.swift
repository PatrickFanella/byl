import Foundation

public struct ItineraryCapabilities: Decodable {
    public let scheduledTransitV1: Bool
    enum CodingKeys: String, CodingKey { case scheduledTransitV1 = "scheduled_transit_v1" }
}

struct TransitPlanRequest: Encodable {
    let version: Int
    let departureAt: String
    enum CodingKeys: String, CodingKey { case version; case departureAt = "departure_at" }
}

public struct TransitPlatform: Codable, Equatable {
    public let id: String
    public let name: String
    public let parentID: String
    public let latitude: Double
    public let longitude: Double
    enum CodingKeys: String, CodingKey { case id, name, latitude, longitude; case parentID = "parent_id" }
    var valid: Bool {
        (1...20).contains(id.count) && id.allSatisfy({ $0.isASCII && $0.isNumber }) && !name.isEmpty && latitude.isFinite && longitude.isFinite && (-90...90).contains(latitude) && (-180...180).contains(longitude)
    }
    func matches(_ other: Self) -> Bool {
        guard valid, other.valid, id == other.id, name == other.name, parentID == other.parentID else { return false }
        let radians = Double.pi / 180
        let lat = (latitude - other.latitude) * radians, lng = (longitude - other.longitude) * radians
        let a = pow(sin(lat / 2), 2) + cos(latitude * radians) * cos(other.latitude * radians) * pow(sin(lng / 2), 2)
        return 6371000 * 2 * asin(sqrt(min(1, max(0, a)))) <= 25
    }
}

public struct TransitSource: Codable, Equatable {
    public let url: String
    public let publisher: String
    public let sha256: String
    public let retrievedAt: String
    public let licenseURL: String
    public let timezone: String
    enum CodingKeys: String, CodingKey {
        case url, publisher, sha256, timezone
        case retrievedAt = "retrieved_at", licenseURL = "license_url"
    }
    var valid: Bool {
        url == "https://www.transitchicago.com/downloads/sch_data/google_transit.zip" && !publisher.isEmpty &&
        licenseURL == "https://www.transitchicago.com/developers/terms/" && timezone == "America/Chicago" &&
        sha256.count == 64 && sha256.allSatisfy { $0.isHexDigit } && transitDate(retrievedAt) != nil
    }
}

public struct ApprovedTransit: Codable, Equatable {
    public struct Intent: Codable, Equatable {
        public let version: Int
        public let provider: String
        public let mode: String
        public let fromStopID: String
        public let toStopID: String
        public let boardStopID: String
        public let alightStopID: String
        public let walkToStopSeconds: Int
        public let walkFromStopSeconds: Int
        public let boardingBufferSeconds: Int
        public let walkingAssumptions: String
        enum CodingKeys: String, CodingKey {
            case version, provider, mode
            case fromStopID = "from_stop_id", toStopID = "to_stop_id", boardStopID = "board_stop_id", alightStopID = "alight_stop_id"
            case walkToStopSeconds = "walk_to_stop_seconds", walkFromStopSeconds = "walk_from_stop_seconds"
            case boardingBufferSeconds = "boarding_buffer_seconds", walkingAssumptions = "walking_assumptions"
        }
    }
    public let version: Int
    public let intent: Intent
    public let board: TransitPlatform
    public let alight: TransitPlatform
    public let source: TransitSource
    public let reviewedAt: String
    public let walkingLegs: [RouteLeg]?
    enum CodingKeys: String, CodingKey { case version, intent, board, alight, source; case reviewedAt = "reviewed_at", walkingLegs = "walking_legs" }

    public func valid(for tour: TourDetail) -> Bool {
        let i = intent
        let stops = tour.stops.sorted { $0.position < $1.position }
        guard version > 0, i.version == 1, i.provider == "cta_gtfs_blue_v1", i.mode == "walk", tour.tour.modes.contains(i.mode),
              let from = stops.firstIndex(where: { $0.id == i.fromStopID }), from + 1 < stops.count,
              stops[from + 1].id == i.toStopID, stops[from + 1].position == stops[from].position + 1,
              board.valid, alight.valid, board.id == i.boardStopID, alight.id == i.alightStopID, board.id != alight.id,
              (60...3600).contains(i.walkToStopSeconds), (60...3600).contains(i.walkFromStopSeconds),
              (60...1800).contains(i.boardingBufferSeconds), !i.walkingAssumptions.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              i.walkingAssumptions.utf8.count <= 2000, source.valid, transitDate(reviewedAt) != nil else { return false }
        let legs = walkingLegs ?? []
        guard legs.count == stops.count - 2 else { return false }
        for n in 0..<(stops.count - 1) where n != from {
            let matches = legs.filter { $0.fromPosition == stops[n].position && $0.toPosition == stops[n + 1].position }
            guard matches.count == 1, let leg = matches.first, leg.isReturn != true,
                  leg.fromStopID == nil || leg.fromStopID == stops[n].id, leg.toStopID == nil || leg.toStopID == stops[n + 1].id,
                  leg.durationS >= 0, leg.distanceM.isFinite, leg.distanceM >= 0, validTransitLine(leg.geometry) else { return false }
        }
        return true
    }
}

public struct ScheduledTransitPlan: Codable, Equatable {
    public struct Service: Codable, Equatable {
        public let state: String
        public let scheduled: Bool
        public let live: Bool
        public let source: TransitSource?
        public let board: TransitPlatform?
        public let alight: TransitPlatform?
        public let tripID: String?
        public let routeID: String?
        public let headsign: String?
        public let departureAt: String?
        public let arrivalAt: String?
        public let issues: [String]?
        enum CodingKeys: String, CodingKey {
            case state, scheduled, live, source, board, alight, headsign, issues
            case tripID = "trip_id", routeID = "route_id", departureAt = "departure_at", arrivalAt = "arrival_at"
        }
    }
    public struct Segment: Codable, Equatable {
        public let kind: String
        public let fromStopID: String?
        public let toStopID: String?
        public let fromPlatformID: String?
        public let toPlatformID: String?
        public let geometry: LineString?
        public let durationS: Int
        public let departureAt: String?
        public let arrivalAt: String?
        public let basis: String
        public let navigation: String
        enum CodingKeys: String, CodingKey {
            case kind, geometry, basis, navigation
            case fromStopID = "from_stop_id", toStopID = "to_stop_id", fromPlatformID = "from_platform_id", toPlatformID = "to_platform_id"
            case durationS = "duration_s", departureAt = "departure_at", arrivalAt = "arrival_at"
        }
        public var label: String {
            switch kind {
            case "walk_to_stop": return "Walk to boarding platform"
            case "boarding_wait": return "Boarding buffer and scheduled wait"
            case "transit": return "Scheduled Blue Line ride"
            case "walk_from_stop": return "Walk from alighting platform"
            default: return "Reviewed walking section"
            }
        }
    }
    public let version: Int
    public let tourID: String
    public let revisionID: String?
    public let connectorVersion: Int
    public let state: String
    public let requestedDepartureAt: String
    public let departureContextStopID: String
    public let service: Service
    public let segments: [Segment]
    public let assumptions: [String]
    enum CodingKeys: String, CodingKey {
        case version, state, service, segments, assumptions
        case tourID = "tour_id", revisionID = "revision_id", connectorVersion = "connector_version"
        case requestedDepartureAt = "requested_departure_at", departureContextStopID = "departure_context_stop_id"
    }

    public func valid(for revision: TourRevision) -> Bool {
        guard let tour = revision.snapshot?.tour, let choice = tour.transit, choice.valid(for: tour),
              version == 1, tourID == revision.tourID, revisionID == revision.id, connectorVersion == choice.version,
              departureContextStopID == choice.intent.fromStopID, let requested = transitDate(requestedDepartureAt),
              !service.live, assumptions.count <= 20, assumptions.allSatisfy({ $0.utf8.count <= 4000 }),
              ["scheduled", "no_service", "stale", "provider_unavailable", "unavailable", "unsupported", "needs_review"].contains(state) else { return false }
        guard state == "scheduled" else { return segments.isEmpty && service.state == state }
        guard service.state == state, service.scheduled, service.source?.valid == true,
              service.board?.matches(choice.board) == true, service.alight?.matches(choice.alight) == true,
              service.routeID == "Blue", !(service.tripID ?? "").isEmpty,
              let departure = service.departureAt.flatMap(transitDate), let arrival = service.arrivalAt.flatMap(transitDate),
              departure.timeIntervalSince(requested) >= Double(choice.intent.walkToStopSeconds + choice.intent.boardingBufferSeconds),
              departure.timeIntervalSince(requested) <= Double(choice.intent.walkToStopSeconds + choice.intent.boardingBufferSeconds + 21600),
              arrival > departure, arrival.timeIntervalSince(departure) <= 10800 else { return false }
        let stops = tour.stops.sorted { $0.position < $1.position }
        var index = 0
        for n in 0..<(stops.count - 1) {
            if stops[n].id == choice.intent.fromStopID {
                let kinds = ["walk_to_stop", "boarding_wait", "transit", "walk_from_stop"]
                let bases = ["curator_walking_estimate", "schedule_and_curator_buffer", "cta_static_schedule", "curator_walking_estimate"]
                let navigation = ["external_directions_required", "check_station_signs_and_alerts", "scheduled_rail", "external_directions_required"]
                let times = [requested, requested.addingTimeInterval(Double(choice.intent.walkToStopSeconds)), departure, arrival, arrival.addingTimeInterval(Double(choice.intent.walkFromStopSeconds))]
                for k in 0..<4 {
                    guard index < segments.count else { return false }
                    let s = segments[index]; index += 1
                    guard s.kind == kinds[k], s.basis == bases[k], s.navigation == navigation[k], s.geometry == nil,
                          s.fromStopID == (k == 0 ? choice.intent.fromStopID : nil), s.toStopID == (k == 3 ? choice.intent.toStopID : nil),
                          s.fromPlatformID == (k == 0 ? nil : (k == 3 ? choice.alight.id : choice.board.id)),
                          s.toPlatformID == (k == 3 ? nil : (k == 0 || k == 1 ? choice.board.id : choice.alight.id)),
                          let a = s.departureAt.flatMap(transitDate), let b = s.arrivalAt.flatMap(transitDate),
                          abs(a.timeIntervalSince(times[k])) < 0.01, abs(b.timeIntervalSince(times[k + 1])) < 0.01,
                          abs(Double(s.durationS) - b.timeIntervalSince(a)) < 1 else { return false }
                }
            } else {
                guard index < segments.count, let leg = choice.walkingLegs?.first(where: { $0.fromPosition == stops[n].position && $0.toPosition == stops[n + 1].position }) else { return false }
                let s = segments[index]; index += 1
                guard s.kind == "walk", s.fromStopID == stops[n].id, s.toStopID == stops[n + 1].id,
                      s.fromPlatformID == nil, s.toPlatformID == nil, s.departureAt == nil, s.arrivalAt == nil,
                      s.geometry?.type == "LineString", s.geometry?.coordinates == leg.geometry, s.durationS == leg.durationS,
                      s.basis == "retained_qualified_walking_route", s.navigation == "in_app_walking" else { return false }
            }
        }
        return index == segments.count
    }

    public func unavailableReason(now: Date) -> String? {
        guard state == "scheduled" else { return "Timetable result: \(state.replacingOccurrences(of: "_", with: " ")). Choose another time explicitly or wait for curator review." }
        guard let retrieved = service.source.flatMap({ transitDate($0.retrievedAt) }), now.timeIntervalSince(retrieved) <= 86400,
              retrieved.timeIntervalSince(now) <= 300 else { return "The retained timetable is stale. Review a fresh plan before using directions." }
        guard let departure = service.departureAt.flatMap(transitDate), departure > now else { return "The scheduled boarding time has passed. Byl does not know whether the train ran or you boarded." }
        return nil
    }
}

func validTransitLine(_ coordinates: [[Double]]) -> Bool {
    coordinates.count >= 2 && coordinates.allSatisfy { $0.count == 2 && $0[0].isFinite && $0[1].isFinite && (-180...180).contains($0[0]) && (-90...90).contains($0[1]) }
}

public func transitDate(_ text: String) -> Date? {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter.date(from: text) ?? ISO8601DateFormatter().date(from: text)
}
