import XCTest
@testable import InvisibleMuseumKit

final class APIModelsTests: XCTestCase {
    private func fixture(_ name: String) throws -> Data {
        let url = try XCTUnwrap(Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures"))
        return try Data(contentsOf: url)
    }

    private let decoder = JSONDecoder()

    func testCityDecodesWithTopics() throws {
        let city = try decoder.decode(City.self, from: fixture("city"))
        XCTAssertEqual(city.slug, "downers-grove")
        XCTAssertEqual(city.topics?.count, 4)
        XCTAssertEqual(city.bbox.minLat, 41.76, accuracy: 0.001)
    }

    func testTourDecodesStopsAndRoutes() throws {
        let tour = try decoder.decode(TourDetail.self, from: fixture("tour"))
        XCTAssertEqual(tour.tour.slug, "downtown-memory-loop")
        XCTAssertEqual(tour.stops.count, 4)
        XCTAssertEqual(tour.tour.modes, ["walk", "bike"])
        XCTAssertEqual(tour.tour.routes["walk"]?.status, "ready")
        XCTAssertNotNil(tour.tour.routes["walk"]?.estimatedMinutes)
        // Stop 3 has no explicit featured moment and falls back to the place's primary moment.
        let fishel = tour.stops[2]
        XCTAssertNil(fishel.featuredMomentID)
        XCTAssertEqual(fishel.featuredMoment?.title, "Summer Concert Lawn")
        XCTAssertFalse(tour.tour.isUserRoute)
    }

    func testTourListDecodes() throws {
        let page = try decoder.decode(ListPage<TourSummary>.self, from: fixture("tours"))
        XCTAssertEqual(page.items.count, 1)
        XCTAssertEqual(page.nextCursor, "")
    }

    func testRouteDecodesGeometryAndLegs() throws {
        let route = try decoder.decode(TourRoute.self, from: fixture("route"))
        XCTAssertEqual(route.status, "ready")
        XCTAssertEqual(route.geometry?.coordinates.count, 4)
        XCTAssertEqual(route.legs.count, 3)
        XCTAssertEqual(route.legs[0].fromPosition, 1)
        XCTAssertEqual(route.legs[0].toPosition, 2)
    }

    func testPlaceDecodesMomentsAndPrimary() throws {
        let place = try decoder.decode(PlaceDetail.self, from: fixture("place"))
        XCTAssertEqual(place.place.slug, "downers-grove-museum")
        XCTAssertEqual(place.moments.count, 1)
        XCTAssertEqual(place.primaryMoment?.id, place.place.primaryMomentID)
    }

    func testNearbyPlacesCarryDistance() throws {
        let page = try decoder.decode(ListPage<PlaceSummary>.self, from: fixture("places_near"))
        XCTAssertFalse(page.items.isEmpty)
        XCTAssertNotNil(page.items[0].distanceM)
        XCTAssertLessThan(page.items[0].distanceM ?? 1, 1)
    }

    func testMomentDecodesSourcesAndTopics() throws {
        let moment = try decoder.decode(MomentDetail.self, from: fixture("moment"))
        XCTAssertEqual(moment.summary.title, "Built for the Talkies")
        XCTAssertEqual(moment.sources.count, 2)
        XCTAssertEqual(moment.topics.count, 2)
        XCTAssertFalse(moment.body.isEmpty)
        XCTAssertNil(moment.narration)
    }

    func testSessionAndUserTourDecode() throws {
        let session = try decoder.decode(Session.self, from: fixture("session"))
        XCTAssertEqual(session.account.role, "visitor")
        let mine = try decoder.decode(TourDetail.self, from: fixture("user_tour"))
        XCTAssertTrue(mine.tour.isUserRoute)
        XCTAssertEqual(mine.tour.visibility, "unlisted")
        XCTAssertEqual(mine.stops.count, 2)
        XCTAssertEqual(mine.tour.slug.count, 10)
    }

    func testProgressDecodes() throws {
        let progress = try decoder.decode(TourProgress.self, from: fixture("progress"))
        XCTAssertEqual(progress.visitedPositions, [1, 2])
        XCTAssertNil(progress.completedAt)
    }

    func testErrorEnvelopeMaps() throws {
        let notFound = APIClient.parseError(status: 404, data: try fixture("error_not_found"))
        XCTAssertEqual(notFound.code, "not_found")
        XCTAssertTrue(notFound.isNotFound)
        let invalid = APIClient.parseError(status: 422, data: try fixture("error_validation"))
        XCTAssertEqual(invalid.code, "validation_failed")
        XCTAssertNotNil(invalid.fields["stops"])
        let garbage = APIClient.parseError(status: 502, data: Data("<html>".utf8))
        XCTAssertEqual(garbage.code, "http_error")
    }

    func testRequestBodiesUseSnakeCase() throws {
        let body = CreateUserTourRequest(city: "downers-grove", title: "T", stops: [StopInput(placeID: "p1")])
        let json = String(decoding: try JSONEncoder().encode(body), as: UTF8.self)
        XCTAssertTrue(json.contains("\"place_id\":\"p1\""))
        XCTAssertTrue(json.contains("\"visibility\":\"unlisted\""))
        let progress = String(decoding: try JSONEncoder().encode(ProgressRequest(mode: "walk", visitedPositions: [1], completed: false)), as: UTF8.self)
        XCTAssertTrue(progress.contains("\"visited_positions\":[1]"))
    }
}

extension APIModelsTests {
    /// The active tour's detail and route are cached as JSON, so decode → encode → decode must be lossless.
    func testTourAndRouteRoundTripThroughJSON() throws {
        let tour = try decoder.decode(TourDetail.self, from: fixture("tour"))
        let again = try decoder.decode(TourDetail.self, from: JSONEncoder().encode(tour))
        XCTAssertEqual(again, tour)
        let route = try decoder.decode(TourRoute.self, from: fixture("route"))
        XCTAssertEqual(try decoder.decode(TourRoute.self, from: JSONEncoder().encode(route)), route)
        let moment = try decoder.decode(MomentDetail.self, from: fixture("moment"))
        XCTAssertEqual(try decoder.decode(MomentDetail.self, from: JSONEncoder().encode(moment)), moment)
    }
}

extension APIModelsTests {
    func testSharedJourneyFixturesAndLegacyCompatibility() throws {
        let route = try decoder.decode(TourRoute.self, from: fixture("journey-route-return"))
        let tour = try decoder.decode(TourDetail.self, from: fixture("journey-tour-partial"))
        let journey = try XCTUnwrap(route.journey)
        XCTAssertTrue(journey.usable)
        XCTAssertEqual(journey.itineraryDurationS, 2904)
        XCTAssertEqual(journey.roundTripDurationS, 2904)
        XCTAssertEqual(journey.returnPlan.durationS, 300)
        XCTAssertEqual(journey.segments.first?.surface, "asphalt")
        XCTAssertNil(journey.segments.last?.surface)
        XCTAssertEqual(tour.journeys?["bike"]?.state, "failed")
        XCTAssertNil(tour.journeys?["bike"]?.travelDurationS)
        XCTAssertEqual(tour.tour.routes["walk"]?.estimatedMinutes, 29)
        XCTAssertEqual(try decoder.decode(TourRoute.self, from: JSONEncoder().encode(route)), route)
        XCTAssertEqual(try decoder.decode(TourDetail.self, from: JSONEncoder().encode(tour)), tour)
        for name in ["journey-route-pending", "journey-route-failed"] {
            let unavailable = try decoder.decode(TourRoute.self, from: fixture(name))
            XCTAssertNil(unavailable.journey?.itineraryDurationS)
            XCTAssertFalse(unavailable.journey?.usable ?? true)
        }
        let unreviewed = try decoder.decode(TourRoute.self, from: fixture("journey-route-unreviewed"))
        XCTAssertFalse(unreviewed.journey?.usable ?? true)
        XCTAssertEqual(unreviewed.journey?.state, "unreviewed")
        XCTAssertNil(try decoder.decode(TourRoute.self, from: fixture("route")).journey)
        XCTAssertNil(try decoder.decode(TourDetail.self, from: fixture("tour")).journeys)
    }
}
