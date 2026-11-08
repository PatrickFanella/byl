import Foundation

public struct DeviationPoint: Codable, Equatable {
    public let lat, lng: Double
    public init(lat: Double, lng: Double) { self.lat = lat; self.lng = lng }
    fileprivate var valid: Bool { lat.isFinite && lng.isFinite && abs(lat) <= 85 && abs(lng) <= 180 }
    fileprivate func distance(to other: Self) -> Double {
        let rad = Double.pi / 180
        let s = pow(sin((other.lat - lat) * rad / 2), 2) + cos(lat * rad) * cos(other.lat * rad) * pow(sin((other.lng - lng) * rad / 2), 2)
        return 2 * 6_371_000 * asin(min(1, sqrt(s)))
    }
}

public struct DeviationFix {
    public let point: DeviationPoint
    public let accuracyM, timestampMs: Double
    public init(point: DeviationPoint, accuracyM: Double, timestampMs: Double) {
        self.point = point; self.accuracyM = accuracyM; self.timestampMs = timestampMs
    }
}

public struct DeviationContext {
    public struct StopArea {
        public let center: DeviationPoint
        public let radiusM: Double
        public init(center: DeviationPoint, radiusM: Double) { self.center = center; self.radiusM = radiusM }
    }
    public let revisionID, segmentID, mode, direction: String
    public let reviewed, ambiguityReviewed: Bool
    public let path: [DeviationPoint]
    public let competingPaths: [[DeviationPoint]]
    public let stopAreas: [StopArea]
    public init(revisionID: String, segmentID: String, mode: String, direction: String, reviewed: Bool, ambiguityReviewed: Bool, path: [DeviationPoint], competingPaths: [[DeviationPoint]], stopAreas: [StopArea]) {
        self.revisionID = revisionID; self.segmentID = segmentID; self.mode = mode; self.direction = direction
        self.reviewed = reviewed; self.ambiguityReviewed = ambiguityReviewed; self.path = path
        self.competingPaths = competingPaths; self.stopAreas = stopAreas
    }
}

/// The only detector value eligible for persistence or transport: no raw fixes.
public struct DeviationSummary: Codable, Equatable {
    public let schemaVersion: Int
    public let detectorVersion: String
    public let calibrated: Bool
    public let revisionID, segmentID, mode, direction, observationPeriod: String
    public let qualifiedPassages, sustainedDepartureRejoins: Int
    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version", detectorVersion = "detector_version", calibrated
        case revisionID = "revision_id", segmentID = "segment_id", mode, direction
        case observationPeriod = "observation_period", qualifiedPassages = "qualified_passages"
        case sustainedDepartureRejoins = "sustained_departure_rejoins"
    }
    init(context: DeviationContext, day: String, departures: Int) {
        schemaVersion = 1; detectorVersion = DeviationDetector.version; calibrated = false
        revisionID = context.revisionID; segmentID = context.segmentID; mode = context.mode; direction = context.direction
        observationPeriod = day; qualifiedPassages = 1; sustainedDepartureRejoins = departures
    }
}

/// Direct port of the uncalibrated v1 reference. No subscriptions, storage or IO.
public final class DeviationDetector {
    public static let version = "uncalibrated-v1"
    public static let calibrated = false
    public enum Input { case fix(DeviationFix), pause, resume, end, permissionLost, manualArrival }
    public private(set) var supported = false
    private struct XY { let x, y: Double }
    private struct Projection { let distance, along: Double }
    private struct Outside { let first: DeviationFix; let departureAlong: Double; var count = 0; var qualified = false }
    private let context: DeviationContext
    private let origin: DeviationPoint
    private var path: [XY] = []
    private var competing: [[XY]] = []
    private var length: Double = 0
    private var paused = false, ended = false, reported = false, armed = false, deviated = false
    private var lastSeen = -Double.infinity
    private var previous: DeviationFix?, start: DeviationFix?
    private var trusted: (fix: DeviationFix, along: Double)?
    private var nearSamples = 0, samples = 0
    private var uncertainSince: Double?
    private var outside: Outside?

    public init(context: DeviationContext) {
        self.context = context
        origin = context.path.first ?? DeviationPoint(lat: 0, lng: 0)
        func validPath(_ points: [DeviationPoint]) -> Bool {
            (2...512).contains(points.count) && points.allSatisfy { $0.valid && abs($0.lat - origin.lat) <= 0.25 && abs($0.lng - origin.lng) <= 0.25 }
        }
        guard context.reviewed, context.ambiguityReviewed, !context.revisionID.isEmpty, !context.segmentID.isEmpty,
              ["walk", "bike"].contains(context.mode), ["forward", "reverse"].contains(context.direction),
              validPath(context.path), context.competingPaths.count <= 8, context.competingPaths.allSatisfy(validPath),
              context.stopAreas.count <= 32, context.stopAreas.allSatisfy({ $0.center.valid && $0.radiusM.isFinite && (0...500).contains($0.radiusM) }) else { return }
        let ordered = context.direction == "reverse" ? Array(context.path.reversed()) : context.path
        path = ordered.map(xy)
        competing = context.competingPaths.map { $0.map(xy) }
        for i in 1..<path.count { length += hypot(path[i].x - path[i-1].x, path[i].y - path[i-1].y) }
        supported = (50...5000).contains(length)
    }

    public func push(_ input: Input) -> [DeviationSummary] {
        guard case .fix(let fix) = input else {
            clear(); previous = nil
            if case .end = input { ended = true }
            if case .resume = input { paused = false } else { paused = true }
            return []
        }
        guard supported, !paused, !ended, !reported else { return [] }
        let time = fix.timestampMs
        guard time.isFinite, time.rounded(.towardZero) == time, time >= 0, time <= 253402300799999, time > lastSeen else { return [] }
        lastSeen = time
        guard fix.point.valid, fix.accuracyM.isFinite, (0...20).contains(fix.accuracyM) else { clear(); previous = nil; return [] }
        let accuracy = max(3, fix.accuracyM)
        let prior = previous; previous = fix
        if let prior {
            let elapsed = time - prior.timestampMs
            let movement = max(0, prior.point.distance(to: fix.point) - max(3, prior.accuracyM) - accuracy)
            if elapsed > 20_000 || movement / (elapsed / 1000) > (context.mode == "walk" ? 4 : 15) { clear(); return [] }
        }
        if let start, time - start.timestampMs > 900_000 || Self.day(fix) != Self.day(start) { clear() }
        let projections = project(fix.point, line: path).sorted { $0.distance < $1.distance }
        guard let closest = projections.first else { clear(); return [] }
        let ambiguous = projections.contains { $0.distance <= accuracy + 10 && abs($0.along - closest.along) > max(20, accuracy * 2) }
        let competitor = competing.contains { project(fix.point, line: $0).contains { $0.distance <= accuracy + 10 } }
        let stopAccess = context.stopAreas.contains { $0.center.distance(to: fix.point) <= $0.radiusM + accuracy }
        if ambiguous || competitor || stopAccess { clear(); return [] }
        if let trusted, closest.along < trusted.along - 30 { clear(); return [] }
        let near = closest.distance + accuracy <= 20
        let far = closest.distance - accuracy >= 35
        if !near && !far {
            nearSamples = 0; uncertainSince = uncertainSince ?? time
            if outside?.qualified == false { outside = nil }
            if trusted == nil || time - uncertainSince! > 20_000 { clear() }
            return []
        }
        uncertainSince = nil; samples += 1
        if near {
            nearSamples += 1
            if nearSamples >= 2 { armed = true }
            if start == nil && closest.along <= length * 0.1 { start = fix }
            if let candidate = outside, nearSamples >= 2 {
                if candidate.qualified && closest.along - candidate.departureAlong >= 30 { deviated = true }
                outside = nil
            }
            trusted = (fix, closest.along)
            if let start, outside == nil, nearSamples >= 2, closest.along >= length * 0.9, samples >= 5,
               time - start.timestampMs >= 30_000, start.point.distance(to: fix.point) - accuracy - max(3, start.accuracyM) >= 30 {
                reported = true
                let result = DeviationSummary(context: context, day: Self.day(fix), departures: deviated ? 1 : 0)
                clear(); previous = nil; lastSeen = -Double.infinity
                return [result]
            }
        } else {
            guard start != nil, let trusted else { clear(); return [] }
            if outside == nil && !armed { clear(); return [] }
            if outside == nil { outside = Outside(first: fix, departureAlong: trusted.along) }
            nearSamples = 0; outside!.count += 1
            if outside!.count >= 3, time - outside!.first.timestampMs >= 20_000,
               outside!.first.point.distance(to: fix.point) - accuracy - max(3, outside!.first.accuracyM) >= 30 { outside!.qualified = true }
        }
        return []
    }

    private func clear() {
        start = nil; trusted = nil; nearSamples = 0; armed = false; uncertainSince = nil; samples = 0; deviated = false; outside = nil
    }
    private static func day(_ fix: DeviationFix) -> String {
        let f = DateFormatter(); f.calendar = Calendar(identifier: .gregorian); f.locale = Locale(identifier: "en_US_POSIX"); f.timeZone = TimeZone(secondsFromGMT: 0); f.dateFormat = "yyyy-MM-dd"
        return f.string(from: Date(timeIntervalSince1970: fix.timestampMs / 1000))
    }
    private func xy(_ point: DeviationPoint) -> XY {
        XY(x: (point.lng - origin.lng) * .pi / 180 * 6_371_000 * cos(origin.lat * .pi / 180), y: (point.lat - origin.lat) * .pi / 180 * 6_371_000)
    }
    private func project(_ point: DeviationPoint, line: [XY]) -> [Projection] {
        let p = xy(point)
        var along = 0.0
        var result: [Projection] = []
        for i in 1..<line.count {
            let a = line[i-1], b = line[i], dx = b.x - a.x, dy = b.y - a.y, length = hypot(dx, dy)
            if length == 0 { continue }
            let t = max(0, min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / (length * length)))
            result.append(Projection(distance: hypot(p.x - a.x - t * dx, p.y - a.y - t * dy), along: along + t * length))
            along += length
        }
        return result
    }
}
