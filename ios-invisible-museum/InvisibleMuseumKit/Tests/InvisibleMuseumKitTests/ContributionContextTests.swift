import XCTest
@testable import InvisibleMuseumKit

func contributionRevisionFixture(change: ((inout [String: Any]) -> Void)? = nil) throws -> TourRevision {
    var revision = try JSONSerialization.jsonObject(with: fixtureData("tour-revision-1")) as! [String: Any]
    var snapshot = revision["snapshot"] as! [String: Any]
    var tour = snapshot["tour"] as! [String: Any]
    let source: [String: Any] = ["url": "https://example.org/synthetic-signal-reference", "title": "Synthetic reference", "publisher": "Fixture", "version": "fixture-v1", "retrieved_on": "2026-11-07", "license": "unresolved", "license_url": ""]
    let ref: [String: Any] = ["version": 1, "ambiguity_reviewed": true, "competing_paths": [], "stop_areas": [], "source": source]
    let line: [String: Any] = ["type": "LineString", "coordinates": [[-88.0, 41.8], [-88 + 400 / (6_371_000 * cos(41.8 * .pi / 180)) * 180 / .pi, 41.8]]]
    tour["corridor"] = ["intent": ["source": ["note": "Synthetic logical reference"], "segments": [["id": "signal-fixture", "geometry": line, "signal_references": ["walk": ref]]], "routes": [["mode": "walk", "path": [["segment_id": "signal-fixture", "reverse": false]]]]]]
    snapshot["tour"] = tour
    var routes = snapshot["routes"] as! [String: [String: Any]]
    let routeFixture = try JSONSerialization.jsonObject(with: fixtureData("journey-route-return")) as! [String: Any]
    var journey = routeFixture["journey"] as! [String: Any]
    journey["version"] = 1; journey["usable"] = true; journey["state"] = "usable"; journey["quality"] = "pass"
    journey["segments"] = [["id": "signal-fixture", "kind": "trail", "reverse": false, "basis": "reference", "access": "allowed", "source": source]]
    routes["walk"]?["journey"] = journey; snapshot["routes"] = routes; revision["snapshot"] = snapshot
    change?(&revision)
    return try JSONDecoder().decode(TourRevision.self, from: JSONSerialization.data(withJSONObject: revision))
}

final class ContributionContextTests: XCTestCase {
    func testUsesFrozenReviewedLogicalReferenceAndAddsStoryExclusions() throws {
        let revision = try contributionRevisionFixture()
        let contexts = contributionContexts(revision: revision, mode: "walk")
        XCTAssertEqual(contexts.count, 1)
        XCTAssertEqual(contexts.first?.revisionID, revision.id)
        XCTAssertEqual(contexts.first?.segmentID, "signal-fixture")
        XCTAssertEqual(contexts.first?.stopAreas.count, revision.snapshot?.tour.stops.count)
        XCTAssertTrue(DeviationDetector(context: contexts[0]).supported)
        XCTAssertTrue(contributionContexts(revision: revision, mode: "bike").isEmpty)
        XCTAssertTrue(contributionContexts(revision: nil, mode: "walk").isEmpty)
    }

    private func changingReference(_ change: @escaping (inout [String: Any]) -> Void) throws -> TourRevision {
        try contributionRevisionFixture { raw in
            var snapshot = raw["snapshot"] as! [String: Any], tour = snapshot["tour"] as! [String: Any]
            var corridor = tour["corridor"] as! [String: Any], intent = corridor["intent"] as! [String: Any]
            var segments = intent["segments"] as! [[String: Any]], refs = segments[0]["signal_references"] as! [String: Any]
            var ref = refs["walk"] as! [String: Any]; change(&ref)
            refs["walk"] = ref; segments[0]["signal_references"] = refs; intent["segments"] = segments
            corridor["intent"] = intent; tour["corridor"] = corridor; snapshot["tour"] = tour; raw["snapshot"] = snapshot
        }
    }
    func testMissingOrMalformedObservationMetadataDoesNotBreakNavigation() throws {
        for change: (inout [String: Any]) -> Void in [
            { $0["version"] = 2 }, { $0["ambiguity_reviewed"] = false },
            { $0.removeValue(forKey: "competing_paths") }, { $0["stop_areas"] = "malformed" },
            { $0["competing_paths"] = Array(repeating: ["type": "LineString", "coordinates": [[-88, 41.8], [-88, 41.801]]], count: 9) }
        ] {
            let revision = try changingReference(change)
            XCTAssertFalse(revision.snapshot!.tour.stops.isEmpty)
            XCTAssertTrue(contributionContexts(revision: revision, mode: "walk").isEmpty)
        }
    }

    func testTransitPresenceSurvivesCacheRoundTripAndPreventsSampling() throws {
        let revision = try contributionRevisionFixture { raw in
            var snapshot = raw["snapshot"] as! [String: Any], tour = snapshot["tour"] as! [String: Any]
            tour["transit"] = ["version": 1, "future_field": ["scheduled", "rail"]]
            snapshot["tour"] = tour; raw["snapshot"] = snapshot
        }
        XCTAssertTrue(revision.snapshot!.tour.hasTransit)
        XCTAssertTrue(contributionContexts(revision: revision, mode: "walk").isEmpty)
        let saved = try JSONEncoder().encode(revision)
        let restored = try JSONDecoder().decode(TourRevision.self, from: saved)
        XCTAssertEqual(restored, revision)
        XCTAssertTrue(restored.snapshot!.tour.hasTransit)
        XCTAssertTrue(contributionContexts(revision: restored, mode: "walk").isEmpty)
    }

    func testDeniedOrMismatchedTraversalNeverQualifies() throws {
        for field in ["access", "reverse"] {
            let revision = try contributionRevisionFixture { raw in
                var snapshot = raw["snapshot"] as! [String: Any], routes = snapshot["routes"] as! [String: [String: Any]]
                var journey = routes["walk"]!["journey"] as! [String: Any], segments = journey["segments"] as! [[String: Any]]
                segments[0][field] = field == "access" ? "unknown" : true
                journey["segments"] = segments; routes["walk"]?["journey"] = journey; snapshot["routes"] = routes; raw["snapshot"] = snapshot
            }
            XCTAssertTrue(contributionContexts(revision: revision, mode: "walk").isEmpty)
        }
    }
}
