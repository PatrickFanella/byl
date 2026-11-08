import XCTest
@testable import InvisibleMuseumKit

final class HappeningsTests: XCTestCase {
    private struct Fixture: Decodable {
        let now: String
        let cases: [Example]
    }
    private struct Example: Decodable {
        let name: String
        let response: ListPage<Happening>
        let expected_statuses: [String]
    }
    override func setUp() { super.setUp(); StubURLProtocol.reset() }

    func testSharedResponseStatesAndClockFallbacks() throws {
        let fixture = try JSONDecoder().decode(Fixture.self, from: fixtureData("happenings-v1"))
        let now = try XCTUnwrap(ISO8601DateFormatter().date(from: fixture.now))
        XCTAssertEqual(fixture.cases.count, 17)
        for example in fixture.cases {
            XCTAssertEqual(example.response.items.map { $0.presentationStatus(now: now).rawValue }, example.expected_statuses, example.name)
        }
        let notice = try XCTUnwrap(fixture.cases.first?.response.items.first)
        XCTAssertEqual(notice.occurrenceText, "Nov 8, 2026 · America/Chicago · dates only")
        XCTAssertEqual(notice.checkedText, "Last source check: 2026-11-08 15:00 UTC")
        XCTAssertEqual(notice.presentationStatus(now: try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-11-10T06:00:00Z"))), .expired)
        XCTAssertNil(Happening.sourceURL("javascript:alert(1)"))
        XCTAssertNil(Happening.sourceURL("https://user:secret@example.com/"))
        XCTAssertNotNil(Happening.sourceURL("https://organizer.example/announcement"))
    }

    func testPublicOptionalEndpointIsBoundedAndAnonymous() async throws {
        let fixture = try JSONSerialization.jsonObject(with: fixtureData("happenings-v1")) as! [String: Any]
        let response = (fixture["cases"] as! [[String: Any]])[0]["response"]!
        let data = try JSONSerialization.data(withJSONObject: response)
        StubURLProtocol.handler = { _, _ in (200, data) }
        let page = try await Endpoints(client: stubbedClient()).happenings(place: "place-id", cursor: "20")
        XCTAssertEqual(page.items.count, 1)
        let request = try XCTUnwrap(StubURLProtocol.requests.last)
        XCTAssertEqual(request.method, "GET")
        XCTAssertEqual(request.path, "/api/v1/places/place-id/happenings")
        XCTAssertTrue(request.query?.contains("limit=20") == true)
        XCTAssertTrue(request.query?.contains("cursor=20") == true)
        XCTAssertFalse(request.headers.keys.contains { $0.lowercased() == "authorization" })
        StubURLProtocol.handler = { _, _ in (404, fixtureData("error_not_found")) }
        do { _ = try await Endpoints(client: stubbedClient()).happenings(place: "place-id"); XCTFail("Expected optional endpoint failure") }
        catch let error as APIError { XCTAssertEqual(error.status, 404) }
    }
}
