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
}
