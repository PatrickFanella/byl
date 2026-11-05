import XCTest
@testable import InvisibleMuseumKit

final class APIClientTests: XCTestCase {
    func testPlaceBrowsingFollowsEveryCursorForTheRouteBuilder() async throws {
        let data = fixtureData("places_near")
        var calls = 0
        StubURLProtocol.handler = { _, _ in
            calls += 1
            if calls == 1 {
                let text = String(decoding: data, as: UTF8.self).replacingOccurrences(of: "\"next_cursor\":\"\"", with: "\"next_cursor\":\"page-2\"")
                return (200, Data(text.utf8))
            }
            return (200, data)
        }
        let places = try await Endpoints(client: stubbedClient()).places(city: "downers-grove")
        let firstPage = try JSONDecoder().decode(ListPage<PlaceSummary>.self, from: data)
        XCTAssertEqual(places.count, firstPage.items.count * 2)
        XCTAssertEqual(calls, 2)
        XCTAssertTrue(StubURLProtocol.requests.last?.query?.contains("cursor=page-2") == true)
    }
    override func setUp() {
        super.setUp()
        StubURLProtocol.reset()
    }

    func testUnauthorizedCallsTheHookWithTheTokenUsedAndThrows() async {
        // Review Focus 3: a 401 must reach the app with the rejected token, so a stale request
        // that lands after a fresh sign-in cannot clear the new session.
        StubURLProtocol.handler = { _, _ in (401, Data("{\"error\":{\"code\":\"unauthorized\",\"message\":\"sign in required\"}}".utf8)) }
        let client = stubbedClient(token: "old-token")
        var rejected: [String?] = []
        client.onUnauthorized = { rejected.append($0) }
        do {
            let _: Account = try await client.get("/api/v1/me")
            XCTFail("expected an error")
        } catch let error as APIError {
            XCTAssertTrue(error.isUnauthorized)
            XCTAssertEqual(error.message, "sign in required")
        } catch {
            XCTFail("unexpected \(error)")
        }
        XCTAssertEqual(rejected, ["old-token"])
        XCTAssertEqual(StubURLProtocol.requests.first?.path, "/api/v1/me")
    }

    func testOtherErrorsDoNotCallTheHookAndCarryFields() async {
        StubURLProtocol.handler = { _, _ in (422, fixtureData("error_validation")) }
        let client = stubbedClient(token: "tok")
        var hookCalls = 0
        client.onUnauthorized = { _ in hookCalls += 1 }
        do {
            let _: TourDetail = try await client.post("/api/v1/me/tours", body: ["title": "x"])
            XCTFail("expected an error")
        } catch let error as APIError {
            XCTAssertEqual(error.status, 422)
            XCTAssertEqual(error.fields["stops"], "must have between 2 and 30 stops")
        } catch {
            XCTFail("unexpected \(error)")
        }
        XCTAssertEqual(hookCalls, 0)
    }

    func testNoContentResponsesDecodeAsEmpty() async throws {
        StubURLProtocol.handler = { _, _ in (204, Data()) }
        let client = stubbedClient(token: "tok")
        try await client.delete("/api/v1/me/saved/abc")
        XCTAssertEqual(StubURLProtocol.requests.first?.method, "DELETE")
    }
}
