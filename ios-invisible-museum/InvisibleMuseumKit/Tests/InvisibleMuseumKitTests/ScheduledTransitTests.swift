import XCTest
@testable import InvisibleMuseumKit

@MainActor final class ScheduledTransitTests: XCTestCase {
    private var cache: DiskCache!
    private var api: Endpoints!
    private var now: Date!
    override func setUp() async throws {
        StubURLProtocol.reset()
        cache = try DiskCache(directory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
        api = Endpoints(client: stubbedClient(token: "alice-token"))
        now = transitDate("2026-11-08T20:00:00Z")!
    }
    override func tearDown() async throws { try? FileManager.default.removeItem(at: cache.directory) }
    private func revision(_ mutate: ((inout [String: Any]) -> Void)? = nil) throws -> TourRevision {
        var raw = try JSONSerialization.jsonObject(with: fixtureData("backend-transit-revision")) as! [String: Any]
        // The wire corpus excludes story bodies. Active-trip tests remove featured summaries;
        // this is a synthetic no-story variation, not evidence of reviewed public content.
        var snapshot = raw["snapshot"] as! [String: Any]
        var tour = snapshot["tour"] as! [String: Any]
        var stops = tour["stops"] as! [[String: Any]]
        for n in stops.indices {
            stops[n].removeValue(forKey: "featured_moment"); stops[n].removeValue(forKey: "featured_moment_id")
            var place = stops[n]["place"] as! [String: Any]
            place["arrival_point"] = ["id": "synthetic-arrival-\(n)", "version": 1, "reviewed_at": "2026-11-08T00:00:00Z",
                "point": ["name": "Synthetic public arrival", "kind": "entrance", "latitude": place["latitude"]!, "longitude": place["longitude"]!, "notes": "Test only; not field proof.",
                    "access": [["mode": "walk", "status": "allowed", "surface": "paved", "source_url": "https://example.org/synthetic", "source_title": "Synthetic fixture", "retrieved_at": "2026-11-08", "notes": "Test only"]]]]
            stops[n]["place"] = place
        }
        tour["stops"] = stops; snapshot["tour"] = tour; raw["snapshot"] = snapshot
        mutate?(&raw)
        return try JSONDecoder().decode(TourRevision.self, from: JSONSerialization.data(withJSONObject: raw))
    }
    private func plan(_ mutate: ((inout [String: Any]) -> Void)? = nil) throws -> ScheduledTransitPlan {
        var raw = try JSONSerialization.jsonObject(with: fixtureData("backend-transit-plan")) as! [String: Any]
        mutate?(&raw)
        return try JSONDecoder().decode(ScheduledTransitPlan.self, from: JSONSerialization.data(withJSONObject: raw))
    }
    private func controller(owner: String? = "alice", revision: TourRevision? = nil, current: @escaping () -> Bool = { true }) throws -> TransitTripModel {
        TransitTripModel(api: api, revision: try revision ?? self.revision(), cache: cache, accountID: owner, isCurrent: current, now: { self.now })
    }
    private func stub(_ plan: ScheduledTransitPlan, enabled: Bool = true) {
        StubURLProtocol.handler = { _, path in
            path.hasSuffix("itinerary-capabilities") ? (200, Data("{\"scheduled_transit_v1\":\(enabled)}".utf8)) : (200, try! JSONEncoder().encode(plan))
        }
    }
    private func retained(_ result: ScheduledTransitPlan? = nil) async throws -> TransitTripModel {
        let value = try result ?? plan(); stub(value)
        let c = try controller(); await c.request(departure: transitDate(value.requestedDepartureAt)!); try c.accept(); return c
    }

    func testActualHandlerWireCorpusDecodesWithoutChangingTransitContract() throws {
        let raw = try JSONDecoder().decode(TourRevision.self, from: fixtureData("backend-transit-revision"))
        XCTAssertTrue(try plan().valid(for: raw))
        XCTAssertTrue(raw.snapshot!.tour.transit!.valid(for: raw.snapshot!.tour))
        XCTAssertNil(raw.snapshot?.routes["walk"])
        XCTAssertEqual(try plan().segments.map(\.kind), ["walk_to_stop", "boarding_wait", "transit", "walk_from_stop", "walk", "walk"])
    }
    func testPlanRejectsChangedRevisionLiveRailGeometryAndOrder() throws {
        let revision = try revision()
        let changes: [(inout [String: Any]) -> Void] = [
            { $0["revision_id"] = "other" },
            { $0["connector_version"] = 999 },
            { raw in var service = raw["service"] as! [String: Any]; service["live"] = true; raw["service"] = service },
            { raw in var sections = raw["segments"] as! [[String: Any]]; sections[2]["geometry"] = ["type": "LineString", "coordinates": [[0,0],[1,1]]]; raw["segments"] = sections },
            { raw in var sections = raw["segments"] as! [[String: Any]]; sections.swapAt(0,1); raw["segments"] = sections },
            { raw in var service = raw["service"] as! [String: Any]; var board = service["board"] as! [String: Any]; board["id"] = "999"; service["board"] = board; raw["service"] = service }
        ]
        for change in changes { XCTAssertFalse(try plan(change).valid(for: revision)) }
    }
    func testNoServiceAndStaleAreReadableButCannotGuide() throws {
        for state in ["no_service", "stale", "provider_unavailable", "unavailable", "unsupported", "needs_review"] {
            let value = try plan { raw in
                raw["state"] = state; raw["segments"] = []
                raw["service"] = ["state": state, "scheduled": true, "live": false]
            }
            XCTAssertTrue(value.valid(for: try revision()), state)
            XCTAssertNotNil(value.unavailableReason(now: now), state)
        }
        XCTAssertNotNil(try plan().unavailableReason(now: now.addingTimeInterval(2 * 86400)))
        XCTAssertNotNil(try plan().unavailableReason(now: transitDate("2026-11-08T21:13:00Z")!))
    }
    func testExplicitAcceptAndResumeNeverPostsAnotherPlan() async throws {
        let value = try plan(); stub(value)
        let c = try controller(); await c.request(departure: transitDate(value.requestedDepartureAt)!)
        XCTAssertNil(c.selected); XCTAssertNotNil(c.proposed)
        try c.accept(); XCTAssertTrue(c.canGuide(at: now))
        let count = StubURLProtocol.requests.filter { $0.method == "POST" }.count
        let restored = try controller(); XCTAssertEqual(restored.selected, c.selected); XCTAssertFalse(restored.canGuide(at: now))
        await restored.refreshGate(); XCTAssertTrue(restored.canGuide(at: now))
        XCTAssertEqual(StubURLProtocol.requests.filter { $0.method == "POST" }.count, count)
    }
    func testPlanRequestHasCapabilityAndOnlyDeparturePayload() async throws {
        _ = try await retained()
        let request = try XCTUnwrap(StubURLProtocol.requests.first { $0.method == "POST" })
        XCTAssertEqual(request.headers["X-Byl-Capabilities"], "scheduled-transit-v1")
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(request.body)) as! [String: Any]
        XCTAssertEqual(Set(body.keys), ["version", "departure_at"])
        XCTAssertNil(request.query)
    }
    func testGateDisabledCannotRequestOrGuideOldRetainedPlan() async throws {
        let c = try await retained(); stub(try plan(), enabled: false)
        await c.refreshGate(); XCTAssertFalse(c.canGuide(at: now))
        let count = StubURLProtocol.requests.filter { $0.method == "POST" }.count
        await c.request(departure: transitDate(try plan().requestedDepartureAt)!)
        XCTAssertEqual(StubURLProtocol.requests.filter { $0.method == "POST" }.count, count)
        XCTAssertNotNil(c.selected)
    }
    func testOfflineGateAndExpiredLeaseSuspendDirections() async throws {
        let c = try await retained(); now = now.addingTimeInterval(61)
        XCTAssertFalse(c.canGuide(at: now))
        StubURLProtocol.handler = { _, _ in (503, Data()) }; await c.refreshGate()
        XCTAssertNil(c.enabled); XCTAssertFalse(c.canGuide(at: now)); XCTAssertNotNil(c.selected)
    }
    func testWrongResponseDepartureIsNeverAdopted() async throws {
        stub(try plan()); let c = try controller()
        await c.request(departure: now.addingTimeInterval(1000))
        XCTAssertNil(c.proposed); XCTAssertThrowsError(try c.accept())
    }
    func testStorageFailureDoesNotAdoptProposal() async throws {
        let c = try controller(); let value = try plan(); stub(value)
        await c.request(departure: transitDate(value.requestedDepartureAt)!)
        try FileManager.default.removeItem(at: cache.directory)
        try Data("file".utf8).write(to: cache.directory)
        XCTAssertThrowsError(try c.accept()); XCTAssertNil(c.selected); XCTAssertNotNil(c.proposed)
    }
    func testAccountOriginAndRevisionIsolateRetainedChoices() async throws {
        let original = try await retained()
        XCTAssertNil(try controller(owner: "bob").selected)
        XCTAssertNil(try controller(owner: nil).selected)
        let different = try revision { $0["id"] = "other" }
        XCTAssertNil(try controller(revision: different).selected)
        api.client.baseURL = URL(string: "https://other.test")!
        XCTAssertNil(try controller().selected)
        XCTAssertFalse(original.canGuide(at: now))
    }
    func testNeedsReviewPersistsSuspensionWithoutReplacingChoice() async throws {
        let c = try await retained(); let chosen = c.selected
        let review = try plan { raw in raw["state"] = "needs_review"; raw["segments"] = []; raw["service"] = ["state": "needs_review", "scheduled": true, "live": false] }
        stub(review); await c.request(departure: transitDate(review.requestedDepartureAt)!)
        XCTAssertEqual(c.selected, chosen); XCTAssertTrue(c.guidanceBlocked)
        let reopened = try controller(); await reopened.refreshGate()
        XCTAssertTrue(reopened.guidanceBlocked); XCTAssertFalse(reopened.canGuide(at: now))
        stub(try plan()); await reopened.request(departure: transitDate(try plan().requestedDepartureAt)!); try reopened.accept()
        XCTAssertFalse(reopened.guidanceBlocked)
    }
    func testRemoveAndEndPreventLateAcceptance() async throws {
        let c = try await retained(); try c.remove(); XCTAssertNil(try controller().selected)
        let value = try plan(); stub(value); await c.request(departure: transitDate(value.requestedDepartureAt)!)
        c.invalidate(); XCTAssertThrowsError(try c.accept()); XCTAssertNil(c.proposed)
    }
    func testAccountChangeBeforeResponsePreventsProposal() async throws {
        var current = true
        let value = try plan(); let c = try controller(current: { current })
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("itinerary-capabilities") { return (200, Data("{\"scheduled_transit_v1\":true}".utf8)) }
            current = false
            return (200, try! JSONEncoder().encode(value))
        }
        await c.request(departure: transitDate(value.requestedDepartureAt)!)
        XCTAssertNil(c.proposed); XCTAssertNil(c.selected)
    }
    func testWalkingHandoffHasPublicEndpointsAndRequiresCurrentAdmission() async throws {
        let c = try await retained()
        for boarding in [true,false] {
            let url = try XCTUnwrap(c.walkingURL(boarding: boarding, at: now))
            let query = try XCTUnwrap(URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems)
            XCTAssertEqual(url.host, "maps.apple.com"); XCTAssertEqual(Set(query.map(\.name)), ["saddr","daddr","dirflg"])
            XCTAssertEqual(query.first { $0.name == "dirflg" }?.value, "w")
            XCTAssertFalse(url.absoluteString.contains("alice")); XCTAssertFalse(url.absoluteString.contains("departure"))
        }
        c.invalidate(); XCTAssertNil(c.walkingURL(boarding: true, at: now))
    }
    func testMixedProgressUsesStableStopsAndNoRailWalkingLeg() throws {
        let r = try revision(); XCTAssertNil(r.unavailableReason(mode: "walk"))
        XCTAssertThrowsError(try ActiveTourModel(api: api, revision: r, mode: "walk", cache: cache, isSignedIn: { false }))
        let active = try ActiveTourModel(api: api, revision: r, mode: "walk", cache: cache, isSignedIn: { false }, transitEnabled: true)
        XCTAssertNil(active.route); XCTAssertNotNil(active.transit); XCTAssertEqual(active.futureLegs.count, 2)
        active.markArrivedAtNextStop(); XCTAssertTrue(active.transitInterval); XCTAssertNil(active.mapItemForNextStop()); XCTAssertNil(active.activeLeg)
        let ids = active.visitedStopIDs
        let restored = try XCTUnwrap(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { false }))
        XCTAssertEqual(restored.visitedStopIDs, ids); XCTAssertEqual(restored.revision?.id, r.id)
        restored.markArrivedAtNextStop(); XCTAssertFalse(restored.transitInterval); XCTAssertNotNil(restored.activeLeg)
    }
    func testMalformedSelectionDoesNotBecomeWalkingTour() throws {
        let r = try revision { raw in var s = raw["snapshot"] as! [String: Any]; var t = s["tour"] as! [String: Any]; t["transit"] = ["version": 999]; s["tour"] = t; raw["snapshot"] = s }
        XCTAssertTrue(r.snapshot!.tour.hasTransit); XCTAssertNotNil(r.unavailableReason(mode: "walk"))
    }
    func testCorruptRetainedChoiceRequiresExplicitRemoval() async throws {
        _ = try await retained()
        let file = try XCTUnwrap(FileManager.default.contentsOfDirectory(at: cache.directory, includingPropertiesForKeys: nil).first { $0.lastPathComponent.hasPrefix("transit-plan.v1.") })
        try Data("{invalid".utf8).write(to: file)
        let c = try controller(); stub(try plan())
        await c.request(departure: transitDate(try plan().requestedDepartureAt)!)
        XCTAssertNil(c.proposed); XCTAssertNotNil(c.message)
        try c.remove(); await c.request(departure: transitDate(try plan().requestedDepartureAt)!)
        XCTAssertNotNil(c.proposed)
    }
    func testEndWhilePlanRequestInFlightPreventsLateProposal() async throws {
        let c = try controller(); let value = try plan()
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("itinerary-capabilities") { return (200, Data("{\"scheduled_transit_v1\":true}".utf8)) }
            c.invalidate()
            return (200, try! JSONEncoder().encode(value))
        }
        await c.request(departure: transitDate(value.requestedDepartureAt)!)
        XCTAssertNil(c.proposed); XCTAssertNil(c.selected); XCTAssertFalse(c.busy)
    }
    func testAuthoritativeMixedRevisionRefusalRevokesSavedTrip() async throws {
        let r = try revision()
        let active = try ActiveTourModel(api: api, revision: r, mode: "walk", cache: cache, isSignedIn: { false }, transitEnabled: true)
        StubURLProtocol.handler = { _, _ in (403, Data("{\"error\":{\"code\":\"unsupported_itinerary\",\"message\":\"Disabled\"}}".utf8)) }
        await active.loadRoute()
        XCTAssertTrue(active.revoked); XCTAssertNil(active.route); XCTAssertNil(active.nextStop)
        XCTAssertTrue(active.futureLegs.isEmpty)
        XCTAssertNil(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { false }))
    }
    func testRevisionAdoptionRequiresFreshGateAndLeavesOldPlanSeparate() async throws {
        let first = try revision()
        let next = try revision { raw in
            raw["id"] = "new-revision"; raw["number"] = first.number + 1
            var s = raw["snapshot"] as! [String: Any]; var t = s["tour"] as! [String: Any]
            t["revision_id"] = "new-revision"; t["revision_number"] = first.number + 1; s["tour"] = t; raw["snapshot"] = s
        }
        _ = try await retained()
        let active = try ActiveTourModel(api: api, revision: first, mode: "walk", cache: cache, isSignedIn: { false }, accountID: "alice", transitEnabled: true)
        XCTAssertNotNil(active.transit?.selected)
        active.markArrivedAtNextStop()
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("/revisions") { return (200, try! JSONSerialization.data(withJSONObject: ["items": [["id": next.id,"tour_id": next.tourID,"number":next.number,"published_at":next.publishedAt]], "next_cursor":""])) }
            return (200, try! JSONEncoder().encode(next))
        }
        await active.checkForUpdate(); XCTAssertEqual(active.availableUpdate?.id, next.id)
        XCTAssertThrowsError(try active.adoptingUpdate())
        let replacement = try active.adoptingUpdate(transitEnabled: true)
        XCTAssertEqual(replacement.revision?.id, next.id); XCTAssertEqual(replacement.visitedStopIDs, active.visitedStopIDs)
        XCTAssertNil(replacement.transit?.selected)
        XCTAssertNotNil(try controller(revision: first).selected)
    }
}
