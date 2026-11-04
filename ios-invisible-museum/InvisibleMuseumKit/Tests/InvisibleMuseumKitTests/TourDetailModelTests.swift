import XCTest
@testable import InvisibleMuseumKit

@MainActor
final class TourDetailModelTests: XCTestCase {
    override func setUp() {
        super.setUp()
        StubURLProtocol.reset()
    }

    func testLoadExposesStopsAndPerModeEstimates() async {
        StubURLProtocol.handler = { method, path in
            if method == "GET", path == "/api/v1/tours/downtown-memory-loop" { return (200, fixtureData("tour")) }
            return (404, fixtureData("error_not_found"))
        }
        let model = TourDetailModel(api: Endpoints(client: stubbedClient()), ref: "downtown-memory-loop")
        XCTAssertEqual(model.state, .idle)
        await model.load()
        XCTAssertEqual(model.state, .loaded)
        XCTAssertEqual(model.tour?.stops.count, 4)
        XCTAssertEqual(model.estimateText(mode: "walk"), "24 min walk")
        XCTAssertEqual(model.estimateText(mode: "bike"), "7 min ride")
        XCTAssertEqual(model.distanceText(mode: "walk"), "1.8 km")
        XCTAssertNil(model.estimateText(mode: "swim"))
        XCTAssertEqual(model.modes, ["walk", "bike"])
    }

    func testEstimateFallsBackToDurationAndHidesUnreadyRoutes() throws {
        let json = """
        {"mode":"walk","status":"ready","distance_m":900,"duration_s":650}
        """
        let ready = try JSONDecoder().decode(RouteSummary.self, from: Data(json.utf8))
        XCTAssertEqual(TourDetailModel.estimateText(for: ready), "11 min walk")
        let pending = try JSONDecoder().decode(RouteSummary.self, from: Data("{\"mode\":\"bike\",\"status\":\"pending\",\"distance_m\":0,\"duration_s\":0}".utf8))
        XCTAssertNil(TourDetailModel.estimateText(for: pending))
    }

    func testLoadFailureShowsTheAPIMessage() async {
        StubURLProtocol.handler = { _, _ in (404, fixtureData("error_not_found")) }
        let model = TourDetailModel(api: Endpoints(client: stubbedClient()), ref: "missing")
        await model.load()
        XCTAssertEqual(model.state, .failed("not found"))
        XCTAssertNil(model.tour)
    }

    func testToggleSaveCallsPutThenDelete() async {
        StubURLProtocol.handler = { method, path in
            switch (method, path) {
            case ("GET", "/api/v1/tours/downtown-memory-loop"): return (200, fixtureData("tour"))
            case ("PUT", "/api/v1/me/saved/00000000-0000-4000-8000-00000000e0e1"): return (204, Data())
            case ("DELETE", "/api/v1/me/saved/00000000-0000-4000-8000-00000000e0e1"): return (204, Data())
            default: return (404, fixtureData("error_not_found"))
            }
        }
        let model = TourDetailModel(api: Endpoints(client: stubbedClient(token: "tok")), ref: "downtown-memory-loop")
        await model.load()
        XCTAssertFalse(model.isSaved)
        await model.toggleSave()
        XCTAssertTrue(model.isSaved)
        await model.toggleSave()
        XCTAssertFalse(model.isSaved)
        let calls = StubURLProtocol.requests.map { "\($0.method) \($0.path)" }
        XCTAssertEqual(calls, [
            "GET /api/v1/tours/downtown-memory-loop",
            "PUT /api/v1/me/saved/00000000-0000-4000-8000-00000000e0e1",
            "DELETE /api/v1/me/saved/00000000-0000-4000-8000-00000000e0e1"
        ])
        XCTAssertNil(model.saveError)
    }

    func testFailedSaveKeepsPreviousStateAndReportsError() async {
        StubURLProtocol.handler = { method, path in
            if method == "GET" { return (200, fixtureData("tour")) }
            return (401, Data("{\"error\":{\"code\":\"unauthorized\",\"message\":\"sign in required\"}}".utf8))
        }
        let model = TourDetailModel(api: Endpoints(client: stubbedClient()), ref: "downtown-memory-loop")
        await model.load()
        await model.toggleSave()
        XCTAssertFalse(model.isSaved)
        XCTAssertEqual(model.saveError, "sign in required")
    }
}
