import XCTest
import CoreLocation
import MapKit
@testable import InvisibleMuseumKit

@MainActor
final class ActiveTourModelTests: XCTestCase {
    private var cache: DiskCache!
    private var tour: TourDetail!
    private let tourID = "00000000-0000-4000-8000-00000000e0e1"

    override func setUp() async throws {
        try await super.setUp()
        StubURLProtocol.reset()
        cache = try DiskCache(directory: FileManager.default.temporaryDirectory.appendingPathComponent("imk-active-\(UUID().uuidString)"))
        tour = try JSONDecoder().decode(TourDetail.self, from: fixtureData("tour"))
    }

    private func model(signedIn: Bool) -> ActiveTourModel {
        ActiveTourModel(api: Endpoints(client: stubbedClient(token: signedIn ? "tok" : nil)), tour: tour, mode: "walk", cache: cache, isSignedIn: { signedIn })
    }

    private func fix(at stop: TourStop, accuracy: Double = 10) -> CLLocation {
        CLLocation(coordinate: CLLocationCoordinate2D(latitude: stop.place.latitude, longitude: stop.place.longitude),
                   altitude: 0, horizontalAccuracy: accuracy, verticalAccuracy: 0, timestamp: Date())
    }

    private func stubRouteAndProgress(routeStatus: Int = 200) {
        StubURLProtocol.handler = { method, path in
            switch (method, path) {
            case ("GET", "/api/v1/tours/\(self.tourID)/route"):
                return routeStatus == 200 ? (200, fixtureData("route")) : (routeStatus, Data("{\"error\":{\"code\":\"internal\",\"message\":\"down\"}}".utf8))
            case ("PUT", "/api/v1/me/progress/\(self.tourID)"):
                return (200, fixtureData("progress"))
            default:
                return (404, fixtureData("error_not_found"))
            }
        }
    }

    func testNextStopAdvancesOnArrival() async {
        stubRouteAndProgress()
        let m = model(signedIn: false)
        XCTAssertEqual(m.nextStop?.position, 1)
        XCTAssertEqual(m.progressText, "0 of 4 stops")
        m.handle(location: fix(at: tour.stops[0]))
        await m.flush()
        XCTAssertEqual(m.visited, [1])
        XCTAssertEqual(m.nextStop?.position, 2)
        XCTAssertEqual(m.arrivedStop?.position, 1)
        XCTAssertEqual(m.progressText, "1 of 4 stops")
        XCTAssertFalse(m.isComplete)
    }

    func testPoorFixDoesNotAdvance() async {
        stubRouteAndProgress()
        let m = model(signedIn: false)
        m.handle(location: fix(at: tour.stops[0], accuracy: 200))
        await m.flush()
        XCTAssertEqual(m.nextStop?.position, 1)
        XCTAssertTrue(m.visited.isEmpty)
    }

    func testProgressPersistsLocallyAndPostsWhenSignedIn() async throws {
        stubRouteAndProgress()
        let m = model(signedIn: true)
        m.handle(location: fix(at: tour.stops[0]))
        await m.flush()
        let puts = StubURLProtocol.requests.filter { $0.method == "PUT" }
        XCTAssertEqual(puts.map(\.path), ["/api/v1/me/progress/\(tourID)"])
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(puts.first?.body)) as? [String: Any]
        XCTAssertEqual(body?["mode"] as? String, "walk")
        XCTAssertEqual(body?["visited_positions"] as? [Int], [1])
        XCTAssertEqual(body?["completed"] as? Bool, false)

        // A second model on the same cache picks up where the first left off (Review Focus 2).
        let again = model(signedIn: true)
        XCTAssertEqual(again.visited, [1])
        XCTAssertEqual(again.nextStop?.position, 2)
    }

    func testProgressStaysLocalWhenSignedOut() async {
        stubRouteAndProgress()
        let m = model(signedIn: false)
        m.handle(location: fix(at: tour.stops[0]))
        await m.flush()
        XCTAssertTrue(StubURLProtocol.requests.filter { $0.method == "PUT" }.isEmpty)
        XCTAssertEqual(cache.load(LocalTourProgress.self, key: LocalTourProgress.key(tourID: tourID, mode: "walk"))?.visitedPositions, [1])
    }

    func testCompletionPostsCompletedTrue() async throws {
        stubRouteAndProgress()
        let m = model(signedIn: true)
        for stop in tour.stops {
            m.handle(location: fix(at: stop))
            await m.flush()
        }
        XCTAssertTrue(m.isComplete)
        XCTAssertNil(m.nextStop)
        let last = StubURLProtocol.requests.last { $0.method == "PUT" }
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(last?.body)) as? [String: Any]
        XCTAssertEqual(body?["visited_positions"] as? [Int], [1, 2, 3, 4])
        XCTAssertEqual(body?["completed"] as? Bool, true)
    }

    func testManualArrivalWorksWithoutAFix() async {
        stubRouteAndProgress()
        let m = model(signedIn: false)
        m.markArrivedAtNextStop()
        await m.flush()
        XCTAssertEqual(m.visited, [1])
        XCTAssertEqual(m.nextStop?.position, 2)
    }

    func testLegsSplitPastActiveAndFuture() async {
        stubRouteAndProgress()
        let m = model(signedIn: false)
        await m.loadRoute()
        XCTAssertEqual(m.routeState, .loaded)
        XCTAssertNil(m.activeLeg, "before the first stop no route leg is being walked")
        XCTAssertEqual(m.futureLegs.map(\.toPosition), [2, 3, 4])

        m.handle(location: fix(at: tour.stops[0]))
        await m.flush()
        XCTAssertEqual(m.activeLeg?.fromPosition, 1)
        XCTAssertEqual(m.activeLeg?.toPosition, 2)
        XCTAssertEqual(m.futureLegs.map(\.toPosition), [3, 4])
        XCTAssertTrue(m.pastLegs.isEmpty)

        m.handle(location: fix(at: tour.stops[1]))
        await m.flush()
        XCTAssertEqual(m.pastLegs.map(\.toPosition), [2])
        XCTAssertEqual(m.activeLeg?.toPosition, 3)
    }

    func testClosingTravelStaysAheadWhileStoryStopsRemain() async throws {
        var body = try XCTUnwrap(JSONSerialization.jsonObject(with: fixtureData("route")) as? [String: Any])
        var legs = try XCTUnwrap(body["legs"] as? [[String: Any]])
        var closing = try XCTUnwrap(legs.last)
        closing["from_position"] = 4
        closing["to_position"] = 1
        closing["return"] = true
        closing["navigation_source"] = "approved_navigation"
        legs.append(closing)
        body["legs"] = legs
        let data = try JSONSerialization.data(withJSONObject: body)
        StubURLProtocol.handler = { method, path in
            method == "GET" && path == "/api/v1/tours/\(self.tourID)/route"
                ? (200, data) : (404, fixtureData("error_not_found"))
        }
        let m = model(signedIn: false)
        await m.loadRoute()
        XCTAssertEqual(m.futureLegs.map(\.toPosition), [2, 3, 4, 1])
        for stop in tour.stops.prefix(3) { m.handle(location: fix(at: stop)) }
        await m.flush()
        XCTAssertEqual(m.nextStop?.position, 4)
        XCTAssertEqual(m.pastLegs.map(\.toPosition), [2, 3])
        XCTAssertEqual(m.activeLeg?.toPosition, 4)
        XCTAssertEqual(m.futureLegs.map(\.toPosition), [1])
    }

    func testRouteFallsBackToCacheWhenTheAPIIsDown() async {
        stubRouteAndProgress()
        await model(signedIn: false).loadRoute()
        stubRouteAndProgress(routeStatus: 500)
        let offline = model(signedIn: false)
        await offline.loadRoute()
        XCTAssertEqual(offline.routeState, .loaded)
        XCTAssertTrue(offline.routeFromCache)
        XCTAssertEqual(offline.route?.legs.count, 3)
    }

    func testAuthoritativeUnavailableRouteCannotFallBackToCachedGeometry() async {
        for status in [403, 404] {
            stubRouteAndProgress()
            await model(signedIn: false).loadRoute()
            XCTAssertNotNil(cache.load(TourRoute.self, key: ActiveTourModel.routeKey(tourID: tourID, mode: "walk")))
            stubRouteAndProgress(routeStatus: status)
            let denied = model(signedIn: false)
            await denied.loadRoute()
            XCTAssertTrue(denied.revoked, "HTTP \(status)")
            XCTAssertNil(denied.route)
            XCTAssertNil(denied.nextStop)
            XCTAssertNil(cache.load(TourRoute.self, key: ActiveTourModel.routeKey(tourID: tourID, mode: "walk")))
            denied.handle(location: fix(at: tour.stops[0]))
            XCTAssertNil(denied.arrivedStop)
        }
    }

    func testPendingRouteKeepsCachedGeometryUntilReplacementIsReady() async {
        stubRouteAndProgress()
        let m = model(signedIn: false)
        await m.loadRoute()
        let text = String(decoding: fixtureData("route"), as: UTF8.self).replacingOccurrences(of: "\"status\":\"ready\"", with: "\"status\":\"pending\"")
        StubURLProtocol.handler = { _, _ in (200, Data(text.utf8)) }
        await m.loadRoute()
        XCTAssertEqual(m.route?.status, "ready")
        XCTAssertTrue(m.routeFromCache)
        XCTAssertTrue(m.routePending)
    }

    func testRouteFailureWithoutCacheIsReported() async {
        stubRouteAndProgress(routeStatus: 500)
        let m = model(signedIn: false)
        await m.loadRoute()
        XCTAssertEqual(m.routeState, .failed("down"))
        XCTAssertNil(m.route)
    }

    func testMapsLaunchOptionsFollowTheMode() {
        XCTAssertEqual(ActiveTourModel.mapsLaunchOptions(mode: "walk")[MKLaunchOptionsDirectionsModeKey] as? String, MKLaunchOptionsDirectionsModeWalking)
        XCTAssertEqual(ActiveTourModel.mapsLaunchOptions(mode: "bike")[MKLaunchOptionsDirectionsModeKey] as? String, MKLaunchOptionsDirectionsModeCycling)
        let item = model(signedIn: false).mapItemForNextStop()
        XCTAssertEqual(item?.name, tour.stops[0].place.name)
    }

    func testDistanceToNextStopUpdatesWithEveryFix() async {
        stubRouteAndProgress()
        let m = model(signedIn: false)
        m.handle(location: fix(at: tour.stops[1]))
        await m.flush()
        XCTAssertNotNil(m.distanceToNextM)
        XCTAssertGreaterThan(m.distanceToNextM ?? 0, 50)
        XCTAssertTrue(m.visited.isEmpty, "being at stop 2 must not count as arriving at stop 1")
    }

    // MARK: Review fixes (cycle 3)

    private func stubWithRemoteProgress(putStatus: Int = 200) {
        StubURLProtocol.handler = { method, path in
            switch (method, path) {
            case ("GET", "/api/v1/tours/\(self.tourID)/route"): return (200, fixtureData("route"))
            case ("GET", "/api/v1/me/progress"):
                let one = String(decoding: fixtureData("progress"), as: UTF8.self)
                return (200, Data("{\"items\":[\(one)],\"next_cursor\":\"\"}".utf8))
            case ("PUT", "/api/v1/me/progress/\(self.tourID)"):
                return putStatus == 200 ? (200, fixtureData("progress")) : (putStatus, Data("{\"error\":{\"code\":\"internal\",\"message\":\"down\"}}".utf8))
            default: return (404, fixtureData("error_not_found"))
            }
        }
    }

    func testServerProgressIsMergedWhenTheTourStarts() async {
        // Review Focus 2: a second device (or the other mode) must not lose stops.
        stubWithRemoteProgress()
        let m = model(signedIn: true)
        await m.loadRemoteProgress()
        XCTAssertEqual(m.visited, [1, 2])
        XCTAssertEqual(m.nextStop?.position, 3)
        XCTAssertFalse(m.needsSync, "nothing local was ahead of the server")
        XCTAssertTrue(StubURLProtocol.requests.filter { $0.method == "PUT" }.isEmpty)
    }

    func testLocalStopsAheadOfTheServerArePushedAfterTheMerge() async throws {
        stubWithRemoteProgress()
        let m = model(signedIn: true)
        m.markArrivedAtNextStop() // stop 1 locally
        await m.flush()
        m.markArrivedAtNextStop() // stop 2 locally
        await m.flush()
        m.markArrivedAtNextStop() // stop 3: ahead of the server's [1, 2]
        await m.flush()
        await m.loadRemoteProgress()
        await m.flush()
        let last = try XCTUnwrap(StubURLProtocol.requests.last { $0.method == "PUT" })
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(last.body)) as? [String: Any]
        XCTAssertEqual(body?["visited_positions"] as? [Int], [1, 2, 3])
    }

    func testStaleProgressResyncsWhenTheServerIsBack() async {
        stubWithRemoteProgress(putStatus: 500)
        let m = model(signedIn: true)
        m.markArrivedAtNextStop()
        await m.flush()
        XCTAssertEqual(m.syncError, "down")
        XCTAssertTrue(m.needsSync)
        stubWithRemoteProgress(putStatus: 200)
        await m.syncIfNeeded()
        XCTAssertNil(m.syncError)
        XCTAssertFalse(m.needsSync)
        XCTAssertEqual(StubURLProtocol.requests.filter { $0.method == "PUT" }.count, 2)
        await m.syncIfNeeded()
        XCTAssertEqual(StubURLProtocol.requests.filter { $0.method == "PUT" }.count, 2, "nothing new to send")
    }

    func testProgressPutsAreSentInOrder() async throws {
        stubRouteAndProgress()
        let m = model(signedIn: true)
        m.markArrivedAtNextStop()
        m.markArrivedAtNextStop()
        await m.flush()
        let bodies = try StubURLProtocol.requests.filter { $0.method == "PUT" }.map { rec -> [Int] in
            let body = try JSONSerialization.jsonObject(with: XCTUnwrap(rec.body)) as? [String: Any]
            return body?["visited_positions"] as? [Int] ?? []
        }
        var previous: Set<Int> = []
        for positions in bodies {
            let current = Set(positions)
            XCTAssertTrue(previous.isSubset(of: current), "queued updates must not drop stops")
            previous = current
        }
        XCTAssertEqual(previous, [1, 2])
    }

    func testActiveTourIsRestoredFromTheCacheAfterARelaunch() {
        // Review Focus 2: the walk comes back after the app is killed, even without signal.
        stubRouteAndProgress()
        let first = model(signedIn: false)
        first.markArrivedAtNextStop()
        let restored = ActiveTourModel.restore(api: Endpoints(client: stubbedClient()), cache: cache, isSignedIn: { false })
        XCTAssertEqual(restored?.tour.tour.id, tourID)
        XCTAssertEqual(restored?.mode, "walk")
        XCTAssertEqual(restored?.visited, [1])
        XCTAssertEqual(restored?.nextStop?.position, 2)
        first.end()
        XCTAssertNil(ActiveTourModel.restore(api: Endpoints(client: stubbedClient()), cache: cache, isSignedIn: { false }))
    }

    func testResettingASignedInWalkClearsTheServerRecord() async throws {
        stubRouteAndProgress()
        let m = model(signedIn: true)
        m.markArrivedAtNextStop()
        await m.flush()
        m.resetProgress()
        await m.flush()
        let last = try XCTUnwrap(StubURLProtocol.requests.last { $0.method == "PUT" })
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(last.body)) as? [String: Any]
        XCTAssertEqual(body?["visited_positions"] as? [Int], [])
        XCTAssertEqual(body?["completed"] as? Bool, false)
        XCTAssertEqual(body?["reset"] as? Bool, true)
        XCTAssertTrue(m.visited.isEmpty)
        XCTAssertEqual(m.nextStop?.position, 1)
    }

    func testOfflineResetIsRetriedAfterRelaunchBeforeMerging() async throws {
        stubWithRemoteProgress(putStatus: 500)
        let m = model(signedIn: true)
        m.resetProgress()
        await m.flush()
        XCTAssertTrue(m.needsSync)
        let restored = model(signedIn: true)
        stubWithRemoteProgress(putStatus: 200)
        await restored.loadRemoteProgress()
        await restored.flush()
        XCTAssertTrue(restored.visited.isEmpty)
        XCTAssertFalse(restored.needsSync)
        let last = try XCTUnwrap(StubURLProtocol.requests.last { $0.method == "PUT" })
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(last.body)) as? [String: Any]
        XCTAssertEqual(body?["reset"] as? Bool, true)
    }

    func testProgressIsScopedByAccountAndSharedAcrossTravelModes() {
        let original = model(signedIn: false)
        let alice = ActiveTourModel(api: original.api, tour: original.tour, mode: "walk", cache: cache, isSignedIn: { false }, accountID: "alice")
        alice.markArrivedAtNextStop()
        let bike = ActiveTourModel(api: original.api, tour: original.tour, mode: "bike", cache: cache, isSignedIn: { false }, accountID: "alice")
        XCTAssertEqual(bike.visited, [1])
        XCTAssertNil(ActiveTourModel.restore(api: original.api, cache: cache, isSignedIn: { false }, accountID: "bob"))
        let bob = ActiveTourModel(api: original.api, tour: original.tour, mode: "walk", cache: cache, isSignedIn: { false }, accountID: "bob")
        XCTAssertTrue(bob.visited.isEmpty)
    }
}
