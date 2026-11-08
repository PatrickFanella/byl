import XCTest
import CoreLocation
@testable import InvisibleMuseumKit

@MainActor
final class PinnedTourTests: XCTestCase {
    private var cache: DiskCache!
    private var api: Endpoints!

    override func setUp() async throws {
        try await super.setUp()
        StubURLProtocol.reset()
        cache = try DiskCache(directory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
        api = Endpoints(client: stubbedClient(token: "alice-token"))
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: cache.directory)
        try await super.tearDown()
    }

    /// These injected review/arrival values are synthetic client-test assertions, not
    /// evidence that the straight-line compatibility fixture is a qualified public trip.
    private func revision(_ number: Int = 1, change: ((inout [String: Any]) -> Void)? = nil) throws -> TourRevision {
        var raw = try XCTUnwrap(JSONSerialization.jsonObject(with: fixtureData("tour-revision-\(number)")) as? [String: Any])
        var snapshot = raw["snapshot"] as! [String: Any]
        var tour = snapshot["tour"] as! [String: Any]
        var stops = tour["stops"] as! [[String: Any]]
        for index in stops.indices {
            var place = stops[index]["place"] as! [String: Any]
            place["arrival_point"] = ["id": "arrival-\(place["id"]!)", "version": 1, "reviewed_at": "2026-11-07T00:00:00Z", "point": ["name": "Synthetic entrance", "kind": "entrance", "latitude": (place["latitude"] as! Double) + 0.0002, "longitude": (place["longitude"] as! Double) + 0.0002, "notes": "Synthetic test access only.", "access": ["walk", "bike"].map { ["mode": $0, "status": "allowed", "surface": "paved", "source_url": "https://example.org/test", "source_title": "Synthetic test reference", "retrieved_at": "2026-11-07", "notes": "Test only"] }]]
            stops[index]["place"] = place
        }
        tour["stops"] = stops
        tour["revision_id"] = raw["id"]
        tour["revision_number"] = number
        let routeFixture = try XCTUnwrap(JSONSerialization.jsonObject(with: fixtureData("journey-route-return")) as? [String: Any])
        let journey = routeFixture["journey"] as! [String: Any]
        var routes = snapshot["routes"] as! [String: [String: Any]]
        for mode in routes.keys { routes[mode]?["journey"] = journey }
        tour["journeys"] = Dictionary(uniqueKeysWithValues: routes.keys.map { ($0, journey) })
        snapshot["tour"] = tour
        snapshot["routes"] = routes
        raw["snapshot"] = snapshot
        change?(&raw)
        return try JSONDecoder().decode(TourRevision.self, from: JSONSerialization.data(withJSONObject: raw))
    }

    private func start(_ revision: TourRevision, signedIn: Bool = false, mode: String = "walk", owner: String? = "alice") throws -> ActiveTourModel {
        try ActiveTourModel(api: api, revision: revision, mode: mode, cache: cache, isSignedIn: { signedIn }, accountID: owner)
    }

    private func progress(_ revision: TourRevision, ids: [String], owner: String = "alice") throws -> Data {
        try JSONSerialization.data(withJSONObject: ["account_id": owner, "tour_id": revision.tourID, "revision_id": revision.id, "mode": "walk", "visited_stop_ids": ids, "completed_at": NSNull()])
    }

    private func metadata(_ revision: TourRevision) throws -> Data {
        try JSONSerialization.data(withJSONObject: ["items": [["id": revision.id, "tour_id": revision.tourID, "number": revision.number, "published_at": revision.publishedAt]], "next_cursor": ""])
    }

    func testSharedCaptureRetainsStableIDsAcrossReorderWithoutClaimingReadiness() throws {
        let first = try JSONDecoder().decode(TourRevision.self, from: fixtureData("tour-revision-1"))
        let second = try JSONDecoder().decode(TourRevision.self, from: fixtureData("tour-revision-2"))
        let visited = try JSONDecoder().decode(RevisionProgress.self, from: fixtureData("tour-progress-1"))
        XCTAssertEqual(Set(first.snapshot!.tour.stops.map(\.id)), Set(second.snapshot!.tour.stops.map(\.id)))
        let id = try XCTUnwrap(visited.visitedStopIDs.first)
        XCTAssertEqual(first.snapshot?.tour.stops.first { $0.id == id }?.position, 1)
        XCTAssertEqual(second.snapshot?.tour.stops.first { $0.id == id }?.position, 2)
        XCTAssertNotNil(first.unavailableReason(mode: "walk"))
    }

    func testLegacyResponseDecodesButDoesNotOfferANewReviewedStart() async throws {
        let legacy = try JSONDecoder().decode(TourDetail.self, from: fixtureData("tour"))
        XCTAssertNil(legacy.tour.revisionID)
        XCTAssertNil(legacy.stops.first?.place.arrivalPoint)
        StubURLProtocol.handler = { _, _ in (200, fixtureData("tour")) }
        let detail = TourDetailModel(api: api, ref: legacy.id)
        await detail.load()
        XCTAssertFalse(detail.canStart(mode: "walk"))
    }

    func testLegacyRestoreDoesNotJoinNewRoutesOrSyncPositionProgress() async throws {
        let legacy = try JSONDecoder().decode(TourDetail.self, from: fixtureData("tour"))
        let old = ActiveTourModel(api: api, tour: legacy, mode: "walk", cache: cache, isSignedIn: { false }, accountID: "alice")
        old.markArrivedAtNextStop()
        let restored = try XCTUnwrap(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { true }, accountID: "alice"))
        await restored.loadRoute()
        await restored.loadRemoteProgress()
        restored.markArrivedAtNextStop()
        await restored.syncIfNeeded()
        XCTAssertTrue(restored.legacyResume)
        XCTAssertTrue(StubURLProtocol.requests.isEmpty)
    }

    func testPinnedRouteAndStoryNeverJoinTheCurrentCatalogue() async throws {
        let original = try revision()
        let changed = try revision(2)
        StubURLProtocol.handler = { _, _ in (200, try! JSONEncoder().encode(changed)) }
        let model = try start(original)
        await model.loadRoute()
        XCTAssertEqual(model.route, original.snapshot?.routes["walk"])
        XCTAssertEqual(model.tour, original.snapshot?.tour)
        let featured = try XCTUnwrap(model.tour.stops.first?.featuredMoment?.id)
        XCTAssertEqual(model.retainedMoment(featured), original.snapshot?.moments[featured])
        XCTAssertTrue(StubURLProtocol.requests.allSatisfy { $0.path.hasSuffix("/revisions/\(original.id)") })
    }

    func testOfflineRestoreUsesExactRevisionDespiteLaterTourCacheOverwrite() async throws {
        let first = try revision()
        let model = try start(first)
        model.markArrivedAtNextStop()
        cache.save(try revision(2).snapshot!.tour, key: ActiveTourModel.tourKey(tourID: first.tourID))
        let restored = try XCTUnwrap(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { false }, accountID: "alice"))
        StubURLProtocol.handler = { _, _ in (503, Data()) }
        await restored.loadRoute()
        XCTAssertEqual(restored.revision?.id, first.id)
        XCTAssertEqual(restored.tour.stops, first.snapshot?.tour.stops)
        XCTAssertEqual(restored.visitedStopIDs, model.visitedStopIDs)
        XCTAssertEqual(restored.route, first.snapshot?.routes["walk"])
        XCTAssertTrue(restored.routeFromCache)
    }

    func testAccountAndOriginCannotRestoreAnotherTripsSnapshot() throws {
        _ = try start(revision())
        XCTAssertNil(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { true }, accountID: "bob"))
        let other = stubbedClient(token: "alice-token")
        other.baseURL = URL(string: "https://other.test")!
        XCTAssertNil(ActiveTourModel.restore(api: Endpoints(client: other), cache: cache, isSignedIn: { true }, accountID: "alice"))
    }

    func testMissingPinnedCacheCannotFallBackToTheLatestTour() throws {
        let first = try revision()
        _ = try start(first)
        cache.remove(ActiveTourModel.revisionKey(api: api, accountID: "alice", revisionID: first.id))
        cache.save(try revision(2).snapshot!.tour, key: ActiveTourModel.tourKey(tourID: first.tourID))
        XCTAssertNil(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { false }, accountID: "alice"))
    }

    func testStartRequiresDurableSnapshotStorage() throws {
        let first = try revision()
        try FileManager.default.removeItem(at: cache.directory)
        XCTAssertThrowsError(try start(first))
        XCTAssertFalse(cache.has(ActiveTourRecord.key))
    }

    func testRevisionProgressPostsStableIDsToTheExactRevisionEndpoint() async throws {
        let first = try revision()
        let stopID = first.snapshot!.tour.stops[0].id
        let response = try progress(first, ids: [stopID])
        StubURLProtocol.handler = { _, _ in (200, response) }
        let model = try start(first, signedIn: true)
        model.markArrivedAtNextStop()
        await model.flush()
        let request = try XCTUnwrap(StubURLProtocol.requests.first)
        XCTAssertEqual(request.path, "/api/v1/me/progress/\(first.tourID)/revisions/\(first.id)")
        let body = try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(request.body)) as? [String: Any])
        XCTAssertEqual(body["visited_stop_ids"] as? [String], [stopID])
        XCTAssertNil(body["visited_positions"])
    }

    func testRemoteProgressRejectsAnotherOwnerOrRevisionAndFiltersUnknownIDs() async throws {
        let first = try revision()
        let valid = first.snapshot!.tour.stops[0].id
        let model = try start(first, signedIn: true)
        let wrong = try progress(first, ids: [valid], owner: "bob")
        StubURLProtocol.handler = { _, _ in (200, wrong) }
        await model.loadRemoteProgress()
        XCTAssertTrue(model.visited.isEmpty)
        let otherRevision = try progress(revision(2), ids: [valid])
        StubURLProtocol.handler = { _, _ in (200, otherRevision) }
        await model.loadRemoteProgress()
        XCTAssertTrue(model.visited.isEmpty)
        let response = try progress(first, ids: [valid, "unknown-stop"])
        StubURLProtocol.handler = { _, _ in (200, response) }
        await model.loadRemoteProgress()
        XCTAssertEqual(model.visitedStopIDs, [valid])
    }

    func testOfflineRevisionResetResumesBeforeAnyRemoteMerge() async throws {
        let first = try revision()
        let original = try start(first, signedIn: true)
        StubURLProtocol.handler = { _, _ in (503, Data()) }
        original.resetProgress()
        await original.flush()
        XCTAssertTrue(original.needsSync)
        StubURLProtocol.reset()
        let response = try progress(first, ids: [])
        StubURLProtocol.handler = { _, _ in (200, response) }
        let restored = try XCTUnwrap(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { true }, accountID: "alice"))
        await restored.loadRemoteProgress()
        XCTAssertFalse(restored.needsSync)
        XCTAssertTrue(restored.visited.isEmpty)
        let request = try XCTUnwrap(StubURLProtocol.requests.first)
        XCTAssertEqual(request.method, "PUT")
        let body = try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(request.body)) as? [String: Any])
        XCTAssertEqual(body["reset"] as? Bool, true)
        XCTAssertEqual(body["visited_stop_ids"] as? [String], [])
    }

    func testAbsentRemoteProgressIsNotASyncFailureOrAnEmptyPut() async throws {
        let first = try revision()
        StubURLProtocol.handler = { _, _ in (404, fixtureData("error_not_found")) }
        let model = try start(first, signedIn: true)

        await model.loadRemoteProgress()

        XCTAssertNil(model.syncError)
        XCTAssertFalse(model.needsSync)
        XCTAssertFalse(StubURLProtocol.requests.contains { $0.method == "PUT" })
    }

    func testUpdateRequiresExplicitTransitionAndReconcilesReorderedIDs() async throws {
        let first = try revision()
        let second = try revision(2)
        let model = try start(first)
        model.markArrivedAtNextStop()
        let oldIDs = model.visitedStopIDs
        let full = try JSONEncoder().encode(second)
        let metadata = try metadata(second)
        let legacyCurrent = try JSONEncoder().encode(first.snapshot!.tour)
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("/revisions") { return (200, metadata) }
            if path.contains("/revisions/") { return (200, full) }
            return (200, legacyCurrent)
        }
        await model.checkForUpdate()
        XCTAssertEqual(model.revision?.id, first.id)
        XCTAssertEqual(model.visited, [1])
        XCTAssertNil(model.updateError)
        XCTAssertEqual(model.availableUpdate?.id, second.id)
        let adopted = try model.adoptingUpdate()
        XCTAssertEqual(adopted.revision?.id, second.id)
        XCTAssertEqual(adopted.visitedStopIDs, oldIDs)
        XCTAssertEqual(adopted.visited, [2])
        XCTAssertEqual(adopted.nextStop?.position, 1)
        let oldAgain = try start(first)
        XCTAssertEqual(oldAgain.visited, [1])
    }

    func testNestedSnapshotRevisionIdentityIsRequiredForStartRestoreAndAdoption() async throws {
        let malformed = try revision(2) { raw in
            var snapshot = raw["snapshot"] as! [String: Any]
            var tour = snapshot["tour"] as! [String: Any]
            tour["revision_id"] = "other-revision"
            tour["revision_number"] = 99
            snapshot["tour"] = tour
            raw["snapshot"] = snapshot
        }
        XCTAssertThrowsError(try start(malformed))

        try cache.savePrivate(malformed, key: ActiveTourModel.revisionKey(api: api, accountID: "alice", revisionID: malformed.id))
        try cache.savePrivate(ActiveTourRecord(tourID: malformed.tourID, mode: "walk", accountID: "alice", revisionID: malformed.id, origin: api.client.baseURL.absoluteString), key: ActiveTourRecord.key)
        XCTAssertNil(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { true }, accountID: "alice"))

        let current = try revision()
        let model = try start(current)
        let revisionList = try metadata(malformed)
        let full = try JSONEncoder().encode(malformed)
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("/revisions") { return (200, revisionList) }
            if path.contains("/revisions/") { return (200, full) }
            return (500, Data())
        }
        await model.checkForUpdate()
        XCTAssertNotNil(model.updateError)
        XCTAssertThrowsError(try model.adoptingUpdate())
    }

    func testChangedEntranceDoesNotCarryTheOldVisit() async throws {
        let model = try start(revision())
        model.markArrivedAtNextStop()
        let changed = try revision(2) { raw in
            var snapshot = raw["snapshot"] as! [String: Any]
            var tour = snapshot["tour"] as! [String: Any]
            var stops = tour["stops"] as! [[String: Any]]
            var place = stops[1]["place"] as! [String: Any]
            var arrival = place["arrival_point"] as! [String: Any]
            arrival["version"] = 2
            place["arrival_point"] = arrival; stops[1]["place"] = place
            tour["stops"] = stops; snapshot["tour"] = tour; raw["snapshot"] = snapshot
        }
        let full = try JSONEncoder().encode(changed)
        let revisionList = try metadata(changed)
        StubURLProtocol.handler = { _, path in (200, path.hasSuffix("/revisions") ? revisionList : full) }
        await model.checkForUpdate()
        XCTAssertTrue(try model.adoptingUpdate().visited.isEmpty)
    }

    func testUnavailableModeCannotReplaceStartedRevision() async throws {
        let model = try start(revision(), mode: "bike")
        let failed = try revision(2) { raw in
            var snapshot = raw["snapshot"] as! [String: Any]
            var routes = snapshot["routes"] as! [String: [String: Any]]
            routes["bike"]?["status"] = "failed"
            snapshot["routes"] = routes; raw["snapshot"] = snapshot
        }
        let full = try JSONEncoder().encode(failed)
        let revisionList = try metadata(failed)
        StubURLProtocol.handler = { _, path in (200, path.hasSuffix("/revisions") ? revisionList : full) }
        await model.checkForUpdate()
        XCTAssertNotNil(model.updateError)
        XCTAssertThrowsError(try model.adoptingUpdate())
        XCTAssertEqual(model.revision?.number, 1)
    }

    func testKnownRevocationClearsSnapshotAndDisablesArrivals() async throws {
        let first = try revision()
        let model = try start(first)
        StubURLProtocol.handler = { _, _ in (404, fixtureData("error_not_found")) }
        await model.loadRoute()
        XCTAssertTrue(model.revoked)
        XCTAssertNil(model.nextStop)
        XCTAssertNil(model.route)
        XCTAssertFalse(model.isComplete)
        XCTAssertFalse(cache.has(ActiveTourModel.revisionKey(api: api, accountID: "alice", revisionID: first.id)))
        XCTAssertNil(ActiveTourModel.restore(api: api, cache: cache, isSignedIn: { false }, accountID: "alice"))
        model.markArrivedAtNextStop()
        XCTAssertTrue(model.visited.isEmpty)
    }

    func testNavigationUsesReviewedEntranceInsteadOfSiteCoordinates() throws {
        let model = try start(revision())
        let stop = try XCTUnwrap(model.nextStop)
        let coordinate = try XCTUnwrap(model.coordinate(for: stop))
        XCTAssertEqual(coordinate.latitude, stop.place.latitude + 0.0002, accuracy: 0.0000001)
        XCTAssertEqual(coordinate.longitude, stop.place.longitude + 0.0002, accuracy: 0.0000001)
        XCTAssertEqual(model.mapItemForNextStop()?.placemark.coordinate.latitude, coordinate.latitude)
    }

    func testReviewedNoticesStayBoundToTheSelectedModeAndSegments() throws {
        let raw = Data("""
        {"intent":{"source":{"note":"Retained operator guidance"},"segments":[
          {"id":"selected","access":[{"mode":"walk","notes":"Walking access notice"},{"mode":"bike","notes":"Biking access notice"}]},
          {"id":"other","access":[{"mode":"walk","notes":"Unselected branch notice"}]}],
          "routes":[{"mode":"walk","path":[{"segment_id":"selected"}]},{"mode":"bike","path":[{"segment_id":"selected"}]}]}}
        """.utf8)
        let corridor = try JSONDecoder().decode(ReviewedCorridor.self, from: raw)
        XCTAssertEqual(corridor.notices(mode: "walk"), ["Retained operator guidance", "Walking access notice"])
        XCTAssertEqual(corridor.notices(mode: "bike"), ["Retained operator guidance", "Biking access notice"])
    }
}
