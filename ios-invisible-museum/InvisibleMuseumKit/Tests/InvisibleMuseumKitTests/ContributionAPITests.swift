import XCTest
@testable import InvisibleMuseumKit

@MainActor
final class ContributionAPITests: XCTestCase {
    private func api() -> ContributionHTTPAPI {
        let config = URLSessionConfiguration.ephemeral; config.protocolClasses = [StubURLProtocol.self]
        return ContributionHTTPAPI(origin: URL(string: "https://stub.test")!, token: "captured-token", session: URLSession(configuration: config))
    }
    override func setUp() { super.setUp(); StubURLProtocol.reset() }
    private func header(_ request: StubURLProtocol.Recorded, _ name: String) -> String? { request.headers.first { $0.key.lowercased() == name.lowercased() }?.value }

    func testGateAndSignedOutWithdrawalAreAnonymousAndCapabilitiesAreNotInURLs() async throws {
        let client = api(), record = contributionRecordFixture()
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("withdraw") { return (204, Data()) }
            return (200, Data("{\"enabled\":false,\"detector_version\":\"uncalibrated-v1\",\"calibrated\":false}".utf8))
        }
        let gate = try await client.gate(); XCTAssertFalse(gate.enabled)
        try await client.withdraw(record.consent!, origin: URL(string: "https://original-server.test")!)
        XCTAssertEqual(StubURLProtocol.requests.map(\.path), ["/api/v1/route-contribution-intake", "/api/v1/route-contributions/withdraw"])
        for request in StubURLProtocol.requests { XCTAssertNil(header(request, "Authorization")); XCTAssertNil(request.query) }
        XCTAssertEqual(StubURLProtocol.requests.last?.host, "original-server.test")
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(StubURLProtocol.requests.last?.body)) as! [String: Any]
        XCTAssertEqual(Set(body.keys), Set(["consent_id", "deletion_handle"]))
    }

    func testGrantAndMinimalUploadKeepCapturedTokenBodyAndRetryKey() async throws {
        let client = api(), record = contributionRecordFixture(), summary = contributionSummaryFixture()
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("consent") { return (200, try! JSONEncoder().encode(record.consent)) }
            return (201, Data("{\"id\":\"70000000-0000-4000-8000-000000000001\",\"expires_on\":\"2026-12-07\"}".utf8))
        }
        let consent = try await client.grant(record.choice, key: record.key)
        try await client.submit(consent: consent, summary: summary, key: record.key)
        for request in StubURLProtocol.requests {
            XCTAssertEqual(header(request, "Authorization"), "Bearer captured-token")
            XCTAssertEqual(header(request, "Idempotency-Key"), record.key)
        }
        let body = try JSONSerialization.jsonObject(with: XCTUnwrap(StubURLProtocol.requests.last?.body)) as! [String: Any]
        XCTAssertEqual(Set(body.keys), Set(["consent_id", "summary"]))
        XCTAssertEqual((body["summary"] as! [String: Any]).count, 10)
    }

    func testMalformedConsentReceiptCannotEnableCollection() async {
        StubURLProtocol.handler = { _, _ in (200, Data("{\"id\":\"60000000-0000-4000-8000-000000000001\",\"deletion_handle\":\"invalid\",\"expires_on\":\"9999-01-01\"}".utf8)) }
        do { _ = try await api().grant(contributionRecordFixture().choice, key: UUID().uuidString); XCTFail("Expected receipt rejection") }
        catch { /* Exact original request remains available for recovery. */ }
    }
}
