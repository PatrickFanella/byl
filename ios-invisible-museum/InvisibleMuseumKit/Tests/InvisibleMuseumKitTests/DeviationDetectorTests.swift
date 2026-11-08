import XCTest
@testable import InvisibleMuseumKit

final class DeviationDetectorTests: XCTestCase {
    private let epoch = ISO8601DateFormatter().date(from: "2026-11-07T12:00:00Z")!.timeIntervalSince1970 * 1000
    private func point(_ x: Double, _ y: Double = 0) -> DeviationPoint {
        DeviationPoint(lat: 41.8 + y / 6_371_000 * 180 / .pi, lng: -88 + x / (6_371_000 * cos(41.8 * .pi / 180)) * 180 / .pi)
    }
    private func context(path: [DeviationPoint]? = nil) -> DeviationContext {
        DeviationContext(revisionID: "reviewed-revision", segmentID: "reviewed-segment", mode: "walk", direction: "forward", reviewed: true, ambiguityReviewed: true, path: path ?? [point(0), point(400)], competingPaths: [], stopAreas: [])
    }
    private func fix(_ seconds: Double, _ x: Double, _ y: Double = 0, accuracy: Double = 3, epoch: Double? = nil) -> DeviationDetector.Input {
        .fix(DeviationFix(point: point(x, y), accuracyM: accuracy, timestampMs: (epoch ?? self.epoch) + seconds * 1000))
    }

    func testEverySharedReferenceFixture() throws {
        let fixture = try XCTUnwrap(JSONSerialization.jsonObject(with: fixtureData("deviation-v1")) as? [String: Any])
        let cases = try XCTUnwrap(fixture["cases"] as? [[String: Any]])
        let epoch = try XCTUnwrap(ISO8601DateFormatter().date(from: fixture["epoch"] as! String)).timeIntervalSince1970 * 1000
        XCTAssertEqual(cases.count, 26)
        for row in cases {
            let name = row["name"] as! String
            let path = (row["path_m"] as! [[Double]]).map { point($0[0], $0[1]) }
            let competing = (row["competing_paths_m"] as! [[[Double]]]).map { $0.map { point($0[0], $0[1]) } }
            let areas = (row["stop_areas_m"] as! [[Double]]).map { DeviationContext.StopArea(center: point($0[0], $0[1]), radiusM: $0[2]) }
            let context = DeviationContext(revisionID: "reviewed-revision", segmentID: "reviewed-segment", mode: row["mode"] as! String, direction: row["direction"] as! String, reviewed: row["eligible"] as! Bool, ambiguityReviewed: row["ambiguity_reviewed"] as! Bool, path: path, competingPaths: competing, stopAreas: areas)
            let detector = DeviationDetector(context: context)
            var reports: [DeviationSummary] = []
            for sample in row["samples"] as! [Any] {
                if let values = sample as? [Double] {
                    reports += detector.push(fix(values[0], values[1], values[2], accuracy: values[3], epoch: epoch))
                } else {
                    let action = (sample as! [String: String])["action"]!
                    let input: DeviationDetector.Input
                    switch action {
                    case "pause": input = .pause
                    case "resume": input = .resume
                    case "end": input = .end
                    case "permission_lost": input = .permissionLost
                    case "manual_arrival": input = .manualArrival
                    default: XCTFail("Unknown fixture control \(action)"); continue
                    }
                    reports += detector.push(input)
                }
            }
            let expected = row["expected_deviation"] as? Bool
            XCTAssertEqual(reports.count, expected == nil ? 0 : 1, name)
            if let expected, let report = reports.first {
                XCTAssertEqual(report, DeviationSummary(context: context, day: "2026-11-07", departures: expected ? 1 : 0), name)
            }
        }
    }

    func testMalformedTimestampsAndGeometryCannotQualify() {
        let detector = DeviationDetector(context: context())
        for time in [Double.nan, .infinity, -1, epoch + 0.5, 253402300800000] {
            XCTAssertTrue(detector.push(.fix(DeviationFix(point: point(0), accuracyM: 3, timestampMs: time))).isEmpty)
        }
        for path in [[], [point(0)], [point(0), point(1)], [point(0), DeviationPoint(lat: .nan, lng: 0)]] {
            XCTAssertFalse(DeviationDetector(context: context(path: path)).supported)
        }
    }

    func testUTCDateBoundaryAndExpiredEpisodeDoNotBridgeEvidence() {
        let detector = DeviationDetector(context: context())
        let midnight = ISO8601DateFormatter().date(from: "2026-11-07T23:59:00Z")!.timeIntervalSince1970 * 1000
        var reports: [DeviationSummary] = []
        for i in 0...20 { reports += detector.push(fix(Double(i) * 10, Double(i) * 20, epoch: midnight)) }
        XCTAssertTrue(reports.isEmpty)
        let expired = DeviationDetector(context: context())
        for i in 0...95 { _ = expired.push(fix(Double(i) * 10, min(Double(i) * 20, 80))) }
        for i in 5...20 { reports += expired.push(fix(960 + Double(i - 5) * 10, Double(i) * 20)) }
        XCTAssertTrue(reports.isEmpty)
    }

    func testSummaryContainsOnlyAllowedFieldsAndNoCoordinatesOrExactTime() throws {
        let detector = DeviationDetector(context: context())
        var reports: [DeviationSummary] = []
        for i in 0...20 { reports += detector.push(fix(Double(i) * 10, Double(i) * 20)) }
        let summary = try XCTUnwrap(reports.first)
        let raw = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(summary)) as? [String: Any])
        XCTAssertEqual(Set(raw.keys), Set(["schema_version", "detector_version", "calibrated", "revision_id", "segment_id", "mode", "direction", "observation_period", "qualified_passages", "sustained_departure_rejoins"]))
        XCTAssertFalse(summary.calibrated)
        for i in 0...20 { XCTAssertTrue(detector.push(fix(300 + Double(i) * 10, Double(i) * 20)).isEmpty) }
    }

    func testResumeStartsFreshAndEndStaysTerminal() {
        let detector = DeviationDetector(context: context())
        _ = detector.push(fix(0, 0)); _ = detector.push(.pause); _ = detector.push(.resume)
        var reports: [DeviationSummary] = []
        for i in 0...20 { reports += detector.push(fix(20 + Double(i) * 10, Double(i) * 20)) }
        XCTAssertEqual(reports.count, 1)
        _ = detector.push(.end); _ = detector.push(.resume)
        XCTAssertTrue(detector.push(fix(500, 0)).isEmpty)
    }
}
