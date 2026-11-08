import Foundation

public struct ReviewedSignalReference: Codable, Equatable {
    public struct StopArea: Codable, Equatable {
        public let latitude, longitude, radiusM: Double
        enum CodingKeys: String, CodingKey { case latitude, longitude; case radiusM = "radius_m" }
    }
    public let version: Int
    public let ambiguityReviewed: Bool
    public let competingPaths: [LineString]
    public let stopAreas: [StopArea]
    public let source: JourneySource
    enum CodingKeys: String, CodingKey {
        case version, source
        case ambiguityReviewed = "ambiguity_reviewed", competingPaths = "competing_paths", stopAreas = "stop_areas"
    }
}

/// Only explicitly reviewed, frozen logical references can reach the detector.
public func contributionContexts(revision: TourRevision?, mode: String) -> [DeviationContext] {
    guard let revision, let snapshot = revision.snapshot, !snapshot.tour.hasTransit,
          ["walk", "bike"].contains(mode), snapshot.tour.tour.modes.contains(mode),
          snapshot.tour.id == revision.tourID,
          snapshot.tour.tour.revisionID == nil || snapshot.tour.tour.revisionID == revision.id,
          let journey = snapshot.routes[mode]?.journey, journey.version == 1, journey.usable,
          journey.state == "usable", journey.quality == "pass",
          !journey.segments.contains(where: { $0.kind == "transit" }),
          let intent = snapshot.tour.corridor?.intent,
          let selected = intent.routes.first(where: { $0.mode == mode }), selected.path.count <= 64 else { return [] }
    let storyAreas = snapshot.tour.stops.map { stop in
        DeviationContext.StopArea(center: DeviationPoint(lat: stop.place.arrivalPoint?.point.latitude ?? stop.place.latitude, lng: stop.place.arrivalPoint?.point.longitude ?? stop.place.longitude), radiusM: stop.place.arrivalRadiusM)
    }
    func points(_ line: LineString) -> [DeviationPoint]? {
        guard line.type == "LineString", (2...512).contains(line.coordinates.count), line.coordinates.allSatisfy({ $0.count == 2 }) else { return nil }
        return line.coordinates.map { DeviationPoint(lat: $0[1], lng: $0[0]) }
    }
    var contexts: [DeviationContext] = []
    for traversal in selected.path {
        guard let segment = intent.segments.first(where: { $0.id == traversal.segmentID }),
              let ref = segment.signalReferences?[mode], ref.version == 1, ref.ambiguityReviewed,
              !ref.source.version.isEmpty, let geometry = segment.geometry, let path = points(geometry),
              ref.competingPaths.count <= 8, ref.stopAreas.count + storyAreas.count <= 32,
              journey.segments.contains(where: { $0.id == segment.id && $0.reverse == (traversal.reverse ?? false) && $0.access == "allowed" }) else { continue }
        let competing = ref.competingPaths.compactMap(points)
        guard competing.count == ref.competingPaths.count else { continue }
        let direction = traversal.reverse == true ? "reverse" : "forward"
        let context = DeviationContext(revisionID: revision.id, segmentID: segment.id, mode: mode, direction: direction, reviewed: true, ambiguityReviewed: true, path: path, competingPaths: competing, stopAreas: storyAreas + ref.stopAreas.map { .init(center: .init(lat: $0.latitude, lng: $0.longitude), radiusM: $0.radiusM) })
        if !contexts.contains(where: { $0.segmentID == context.segmentID && $0.direction == direction }), DeviationDetector(context: context).supported { contexts.append(context) }
    }
    return contexts
}
