import Foundation

public struct ReportLocation: Codable, Equatable, Sendable {
    public let kind: String
    public let stopID: String?
    public let arrivalID: String?
    public let fromStopID: String?
    public let toStopID: String?
    enum CodingKeys: String, CodingKey {
        case kind
        case stopID = "stop_id", arrivalID = "arrival_id"
        case fromStopID = "from_stop_id", toStopID = "to_stop_id"
    }
    public init(kind: String, stopID: String? = nil, arrivalID: String? = nil, fromStopID: String? = nil, toStopID: String? = nil) {
        self.kind = kind; self.stopID = stopID; self.arrivalID = arrivalID
        self.fromStopID = fromStopID; self.toStopID = toStopID
    }
    public var key: String { kind == "segment" ? "segment:\(fromStopID ?? ""):\(toStopID ?? "")" : "\(kind):\(stopID ?? "")" }
}

public struct ReportPoint: Codable, Equatable, Sendable {
    public let lat: Double
    public let lng: Double
    public init(lat: Double, lng: Double) { self.lat = lat; self.lng = lng }
    public var valid: Bool { lat.isFinite && lng.isFinite && (-90...90).contains(lat) && (-180...180).contains(lng) }
}

public struct ReportAlternative: Codable, Equatable, Sendable {
    public let type: String
    public let coordinates: [[Double]]
    public init(coordinates: [[Double]]) { type = "LineString"; self.coordinates = coordinates }
}

public struct RouteReportInput: Codable, Equatable, Sendable {
    public let tourID: String
    public let revisionID: String
    public let mode: String
    public let affected: ReportLocation
    public let reason: String
    public let details: String
    public let better: String
    public let direction: String
    public let duration: String
    public let observedOn: String?
    public let sourceURL: String
    public let annotation: ReportPoint?
    public let alternative: ReportAlternative?
    enum CodingKeys: String, CodingKey {
        case mode, affected, reason, details, better, direction, duration, annotation, alternative
        case tourID = "tour_id", revisionID = "revision_id", observedOn = "observed_on", sourceURL = "source_url"
    }
}

public struct RouteReport: Codable, Equatable, Identifiable, Sendable {
    public let id: String
    public let ownerAccountID: String
    public let submitted: RouteReportInput
    public let status: String
    public let reason: String?
    public let version: Int
    public let createdAt: String
    public let updatedAt: String
    enum CodingKeys: String, CodingKey {
        case id, submitted, status, reason, version
        case ownerAccountID = "owner_account_id", createdAt = "created_at", updatedAt = "updated_at"
    }
}

public struct RouteCorrectionOutcome: Decodable, Equatable, Sendable {
    public let revisionID: String
    public let previousRevisionID: String
    public let temporary: Bool
    public let reviewOn: String?
    public let reassessmentRequired: Bool
    enum CodingKeys: String, CodingKey {
        case temporary
        case revisionID = "revision_id", previousRevisionID = "previous_revision_id"
        case reviewOn = "review_on", reassessmentRequired = "reassessment_required"
    }
}

public struct ReportTarget: Identifiable, Equatable {
    public var id: String { affected.key }
    public let label: String
    public let affected: ReportLocation
}

/// References are derived only from the immutable snapshot being reported.
public struct RouteReportContext {
    public let revision: TourRevision
    public let mode: String
    public init(revision: TourRevision, mode: String) { self.revision = revision; self.mode = mode }
    public var unavailableReason: String? {
        guard ["walk", "bike"].contains(mode), let snapshot = revision.snapshot,
              !revision.id.isEmpty, snapshot.tour.id == revision.tourID,
              snapshot.tour.tour.revisionID == revision.id, snapshot.tour.tour.revisionNumber == revision.number,
              snapshot.tour.tour.modes.contains(mode), let route = snapshot.routes[mode],
              route.status == "ready", route.mode == mode, route.tourID == revision.tourID else {
            return "The selected revision has no retained ready path for this mode."
        }
        let stops = snapshot.tour.stops
        guard (2...30).contains(stops.count), Set(stops.map(\.id)).count == stops.count,
              stops.allSatisfy({ UUID(uuidString: $0.id) != nil }), Set(stops.map(\.position)) == Set(1...stops.count) else {
            return "The retained stop references could not be read."
        }
        return nil
    }
    public var targets: [ReportTarget] {
        guard unavailableReason == nil, let snapshot = revision.snapshot else { return [] }
        let stops = snapshot.tour.stops.sorted { $0.position < $1.position }
        var result = stops.flatMap { stop in
            [ReportTarget(label: "Stop \(stop.position): \(stop.place.name)", affected: ReportLocation(kind: "stop", stopID: stop.id)),
             ReportTarget(label: "Entrance / access at \(stop.place.name)", affected: ReportLocation(kind: "entrance", stopID: stop.id, arrivalID: stop.place.arrivalPoint?.id))]
        }
        for leg in snapshot.routes[mode]?.legs ?? [] {
            guard let from = stops.first(where: { $0.position == leg.fromPosition }),
                  let to = stops.first(where: { $0.position == leg.toPosition }), from.id != to.id,
                  leg.fromStopID == nil || leg.fromStopID == from.id,
                  leg.toStopID == nil || leg.toStopID == to.id else { continue }
            let affected = ReportLocation(kind: "segment", fromStopID: from.id, toStopID: to.id)
            if !result.contains(where: { $0.id == affected.key }) {
                result.append(ReportTarget(label: "Path from \(from.place.name) to \(to.place.name)\(leg.isReturn == true ? " (return)" : "")", affected: affected))
            }
        }
        return result
    }
}

public struct ReportFields: Codable, Equatable, Sendable {
    public var target = ""
    public var reason = "blocked"
    public var details = ""
    public var better = ""
    public var sourceURL = ""
    public var direction = "unknown"
    public var duration = "unknown"
    public var observedOn = ""
    public var latitude = ""
    public var longitude = ""
    public var alternativePoints = ""
    public init() {}

    public func input(context: RouteReportContext) throws -> RouteReportInput {
        guard context.unavailableReason == nil, let target = context.targets.first(where: { $0.id == self.target }) else {
            throw ReportInputError(message: "Choose a retained stop, entrance or path.")
        }
        var annotation: ReportPoint?
        if !latitude.isEmpty || !longitude.isEmpty {
            guard let lat = Double(latitude), let lng = Double(longitude), ReportPoint(lat: lat, lng: lng).valid else {
                throw ReportInputError(message: "Enter both a valid latitude and longitude, or leave both empty.")
            }
            annotation = ReportPoint(lat: lat, lng: lng)
        }
        let lines = alternativePoints.split(whereSeparator: \.isNewline).map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
        var alternative: ReportAlternative?
        if !lines.isEmpty {
            guard (2...64).contains(lines.count) else { throw ReportInputError(message: "An alternative needs 2 to 64 longitude, latitude pairs.") }
            let coordinates = try lines.map { line -> [Double] in
                let values = line.split(separator: ",", omittingEmptySubsequences: false).map { $0.trimmingCharacters(in: .whitespaces) }
                guard values.count == 2, let lng = Double(values[0]), let lat = Double(values[1]), ReportPoint(lat: lat, lng: lng).valid else {
                    throw ReportInputError(message: "Use one longitude, latitude pair per line for the alternative.")
                }
                return [lng, lat]
            }
            alternative = ReportAlternative(coordinates: coordinates)
        }
        let trim: (String) -> String = { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        return RouteReportInput(tourID: context.revision.tourID, revisionID: context.revision.id, mode: context.mode,
            affected: target.affected, reason: reason, details: trim(details), better: trim(better), direction: direction,
            duration: duration, observedOn: observedOn.isEmpty ? nil : observedOn, sourceURL: trim(sourceURL), annotation: annotation, alternative: alternative)
    }
}

public struct ReportInputError: LocalizedError {
    public let message: String
    public var errorDescription: String? { message }
}
