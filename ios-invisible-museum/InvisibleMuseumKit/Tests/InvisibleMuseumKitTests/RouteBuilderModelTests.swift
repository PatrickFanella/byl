import XCTest
@testable import InvisibleMuseumKit

@MainActor
final class RouteBuilderModelTests: XCTestCase {
    private var city: City!
    private var places: [PlaceSummary]!

    override func setUp() async throws {
        try await super.setUp()
        StubURLProtocol.reset()
        city = try JSONDecoder().decode(City.self, from: fixtureData("city"))
        places = try JSONDecoder().decode(ListPage<PlaceSummary>.self, from: fixtureData("places_near")).items
        XCTAssertGreaterThanOrEqual(places.count, 2)
    }

    private func builder(signedIn: Bool = true, editing: TourDetail? = nil) -> RouteBuilderModel {
        RouteBuilderModel(api: Endpoints(client: stubbedClient(token: signedIn ? "tok" : nil)), city: city, editing: editing)
    }

    func testCreateStaysDisabledUnderTwoStops() {
        let b = builder()
        b.title = "Depot loop"
        XCTAssertFalse(b.canSubmit)
        b.add(places[0])
        XCTAssertFalse(b.canSubmit)
        XCTAssertEqual(b.stopsHint, "Add at least 2 stops (1 so far)")
        b.add(places[1])
        XCTAssertTrue(b.canSubmit)
        b.title = "   "
        XCTAssertFalse(b.canSubmit, "a title is required")
        b.title = "Depot loop"
        b.walk = false
        b.bike = false
        XCTAssertFalse(b.canSubmit, "at least one mode is required")
    }

    func testDuplicatesAreIgnoredAndThirtyIsTheCap() throws {
        let b = builder()
        b.add(places[0])
        b.add(places[0])
        XCTAssertEqual(b.stops.count, 1)
        let template = places[1]
        for i in 0..<40 {
            let json = try JSONEncoder().encode(template)
            var dict = try XCTUnwrap(JSONSerialization.jsonObject(with: json) as? [String: Any])
            dict["id"] = "p-\(i)"
            let p = try JSONDecoder().decode(PlaceSummary.self, from: JSONSerialization.data(withJSONObject: dict))
            b.add(p)
        }
        XCTAssertEqual(b.stops.count, RouteBuilderModel.maxStops)
        XCTAssertEqual(b.stopsHint, "30 stops is the maximum")
    }

    func testReorderAndRemove() {
        let b = builder()
        b.add(places[0]); b.add(places[1])
        b.move(from: 1, to: 0)
        XCTAssertEqual(b.stops.map(\.id), [places[1].id, places[0].id])
        b.remove(places[1].id)
        XCTAssertEqual(b.stops.map(\.id), [places[0].id])
        XCTAssertTrue(b.contains(places[0]))
        XCTAssertFalse(b.contains(places[1]))
    }

    func testSubmitPostsTheRouteAndReturnsIt() async throws {
        StubURLProtocol.handler = { method, path in
            if method == "POST", path == "/api/v1/me/tours" { return (201, fixtureData("user_tour")) }
            return (404, fixtureData("error_not_found"))
        }
        let b = builder()
        b.title = "Depot loop"
        b.summary = "Two stops"
        b.bike = false
        b.visibility = "private"
        b.add(places[0]); b.add(places[1])
        let created = await b.submit()
        XCTAssertNotNil(created)
        XCTAssertTrue(created?.tour.isUserRoute ?? false)
        let post = try XCTUnwrap(StubURLProtocol.requests.first { $0.method == "POST" })
        let body = try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(post.body)) as? [String: Any])
        XCTAssertEqual(body["city"] as? String, city.slug)
        XCTAssertEqual(body["title"] as? String, "Depot loop")
        XCTAssertEqual(body["modes"] as? [String], ["walk"])
        XCTAssertEqual(body["visibility"] as? String, "private")
        let stops = try XCTUnwrap(body["stops"] as? [[String: Any]])
        XCTAssertEqual(stops.map { $0["place_id"] as? String }, [places[0].id, places[1].id])
        XCTAssertNil(b.error)
    }

    func testValidationFieldsSurfaceOnStops() async {
        // Review Focus 5: the API's 422 field message is shown next to the stops list.
        StubURLProtocol.handler = { method, _ in
            method == "POST" ? (422, fixtureData("error_validation")) : (404, fixtureData("error_not_found"))
        }
        let b = builder()
        b.title = "Depot loop"
        b.add(places[0]); b.add(places[1])
        let created = await b.submit()
        XCTAssertNil(created)
        XCTAssertEqual(b.fieldErrors["stops"], "must have between 2 and 30 stops")
        XCTAssertEqual(b.stopsError, "must have between 2 and 30 stops")
        XCTAssertEqual(b.error, "request failed validation")
        XCTAssertFalse(b.busy)
    }

    func testEditingPrefillsAndPatches() async throws {
        let mine = try JSONDecoder().decode(TourDetail.self, from: fixtureData("user_tour"))
        StubURLProtocol.handler = { method, path in
            if method == "PATCH", path == "/api/v1/me/tours/\(mine.tour.id)" { return (200, fixtureData("user_tour")) }
            return (404, fixtureData("error_not_found"))
        }
        let b = builder(editing: mine)
        XCTAssertEqual(b.title, mine.tour.title)
        XCTAssertEqual(b.stops.map(\.id), mine.stops.map(\.place.id))
        XCTAssertEqual(b.visibility, mine.tour.visibility)
        XCTAssertTrue(b.isEditing)
        b.title = "Renamed"
        let updated = await b.submit()
        XCTAssertNotNil(updated)
        let patch = try XCTUnwrap(StubURLProtocol.requests.first { $0.method == "PATCH" })
        let body = try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(patch.body)) as? [String: Any])
        XCTAssertEqual(body["title"] as? String, "Renamed")
        XCTAssertEqual((body["stops"] as? [[String: Any]])?.count, mine.stops.count)
    }

    func testShareURLIsBuiltFromTheSlug() {
        let url = RouteBuilderModel.shareURL(slug: "abc123XYZ0", base: URL(string: "https://museum-api.subcult.tv")!)
        XCTAssertEqual(url.absoluteString, "https://museum-api.subcult.tv/t/abc123XYZ0")
    }

    func testPlacesLoadSearchAndSortByDistance() async {
        StubURLProtocol.handler = { method, path in
            if method == "GET", path == "/api/v1/cities/\(self.city.slug)/places" { return (200, fixtureData("places_near")) }
            return (404, fixtureData("error_not_found"))
        }
        let b = builder()
        await b.loadPlaces()
        XCTAssertEqual(b.placesState, .loaded)
        XCTAssertEqual(b.filteredPlaces.count, places.count)
        // Search matches name or address; the fixture addresses all contain the city name.
        let target = places.last!
        b.search = target.name.lowercased()
        XCTAssertTrue(b.filteredPlaces.contains { $0.id == target.id })
        XCTAssertTrue(b.filteredPlaces.allSatisfy { $0.name.lowercased().contains(target.name.lowercased()) || $0.address.lowercased().contains(target.name.lowercased()) })
        b.search = "zzz-no-such-place"
        XCTAssertTrue(b.filteredPlaces.isEmpty)
        b.search = ""
        let far = places.last!
        b.sortNear(latitude: far.latitude, longitude: far.longitude)
        XCTAssertEqual(b.filteredPlaces.first?.id, far.id)
    }

    func testStopsFromAnotherCityBlockSubmit() throws {
        // Review Focus 5: cross-city stops keep Create disabled instead of waiting for the 422.
        let b = builder()
        b.title = "Two towns"
        b.add(places[0])
        let json = try JSONEncoder().encode(places[1])
        var dict = try XCTUnwrap(JSONSerialization.jsonObject(with: json) as? [String: Any])
        dict["id"] = "foreign-1"
        dict["city_id"] = "00000000-0000-4000-8000-0000000000c2"
        let foreign = try JSONDecoder().decode(PlaceSummary.self, from: JSONSerialization.data(withJSONObject: dict))
        b.add(foreign)
        XCTAssertEqual(b.stops.count, 2)
        XCTAssertFalse(b.canSubmit)
        XCTAssertEqual(b.stopsHint, "Every stop must be in \(city.name)")
        b.remove(foreign.id)
        b.add(places[1])
        XCTAssertTrue(b.canSubmit)
    }
}
