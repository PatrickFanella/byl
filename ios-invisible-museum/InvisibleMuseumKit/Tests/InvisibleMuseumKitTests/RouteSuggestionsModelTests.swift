import XCTest
@testable import InvisibleMuseumKit

private final class SuggestionIdentity: @unchecked Sendable {
    private let lock = NSLock()
    private var value = true
    var current: Bool { lock.withLock { value } }
    func expire() { lock.withLock { value = false } }
}

@MainActor
final class RouteSuggestionsModelTests: XCTestCase {
    private let owner = "50000000-0000-4000-8000-000000000001"
    private var cache: DiskCache!

    override func setUp() async throws {
        try await super.setUp()
        StubURLProtocol.reset()
        cache = try DiskCache(directory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
        StubURLProtocol.handler = { method, path in
            if path == "/api/v1/route-brief-intake" { return (200, Data("{\"enabled\":true}".utf8)) }
            if method == "POST" { return (201, fixtureData("route_suggestion")) }
            return (200, Data("{\"items\":[],\"next_cursor\":\"\"}".utf8))
        }
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: cache.directory)
        try await super.tearDown()
    }

    private func flow(owner: String? = nil, client: APIClient? = nil, current: @escaping () -> Bool = { true }) -> RouteSuggestionsModel {
        RouteSuggestionsModel(api: Endpoints(client: client ?? stubbedClient(token: "opening-token")), cache: cache, accountID: owner ?? self.owner, isCurrent: current)
    }

    private func fill(_ model: RouteSuggestionsModel) {
        var fields = SuggestionFields()
        fields.topic = " Rail workers "
        fields.area = " Downers Grove "
        fields.places = " Old depot \n\n Main Street "
        fields.sources = "https://example.org/lead"
        model.edit(fields)
    }

    private func header(_ request: StubURLProtocol.Recorded, _ name: String) -> String? {
        request.headers.first { $0.key.lowercased() == name.lowercased() }?.value
    }

    func testCanonicalFixtureAndMinimalInputDefaults() throws {
        let result = try JSONDecoder().decode(RouteSuggestion.self, from: fixtureData("route_suggestion"))
        XCTAssertEqual(result.ownerAccountID, owner)
        XCTAssertEqual(result.status, "needs_research")
        XCTAssertEqual(result.reason, "An operator source is still needed.")
        XCTAssertEqual(result.version, 3)
        XCTAssertEqual(result.submitted.placeLeads, ["Old depot, address unknown"])
        XCTAssertNil(result.submitted.durationMinutes)
        let input = try JSONDecoder().decode(RouteSuggestionInput.self, from: Data("{\"topic\":\"Rail\",\"area\":\"Chicago\"}".utf8))
        XCTAssertEqual(input.modes, ["walk", "bike"])
        XCTAssertEqual(input.tripShape, "flexible")
        XCTAssertEqual(input.sourceLinks, [])
    }

    func testEveryLifecycleAndUnknownStatusHaveSafeLabels() {
        let statuses = ["draft", "processing", "needs_research", "review", "accepted", "declined"]
        XCTAssertEqual(statuses.map { SuggestionStatus($0).label }, ["Received", "Research in progress", "More research needed", "Curator review", "Accepted", "Declined"])
        XCTAssertEqual(SuggestionStatus("future-status").label, "Status unavailable")
        XCTAssertTrue(SuggestionStatus("accepted").detail.contains("curator publication"))
    }

    func testMinimalSubmissionHeadersAndSuccessfulCleanup() async throws {
        let model = flow()
        var fields = SuggestionFields()
        fields.topic = " Rail "
        fields.area = " Chicago "
        model.edit(fields)
        XCTAssertFalse(model.canSubmit)
        await model.loadIntake()
        XCTAssertTrue(model.canSubmit)
        let result = await model.submit()
        XCTAssertEqual(result?.ownerAccountID, owner)
        let post = try XCTUnwrap(StubURLProtocol.requests.first { $0.method == "POST" })
        XCTAssertEqual(post.path, "/api/v1/me/route-briefs")
        XCTAssertEqual(header(post, "Authorization"), "Bearer opening-token")
        XCTAssertNotNil(UUID(uuidString: try XCTUnwrap(header(post, "Idempotency-Key"))))
        let input = try JSONDecoder().decode(RouteSuggestionInput.self, from: XCTUnwrap(post.body))
        XCTAssertEqual(input.topic, "Rail")
        XCTAssertEqual(input.area, "Chicago")
        XCTAssertNil(model.pending)
        XCTAssertEqual(model.fields.topic, "")
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(atPath: cache.directory.path).isEmpty)
    }

    func testUncertainResponseSurvivesRecreationWithIdenticalBodyAndKey() async throws {
        let first = flow()
        fill(first)
        await first.loadIntake()
        StubURLProtocol.handler = { _, _ in (503, Data()) }
        let result = await first.submit()
        XCTAssertNil(result)
        let original = try XCTUnwrap(StubURLProtocol.requests.last)
        let pending = try XCTUnwrap(first.pending)
        XCTAssertFalse(first.canEdit)
        first.close()
        let resumed = flow()
        XCTAssertEqual(resumed.pending, pending)
        XCTAssertEqual(resumed.fields.places, " Old depot \n\n Main Street ")
        StubURLProtocol.handler = { method, _ in method == "POST" ? (201, fixtureData("route_suggestion")) : (200, Data("{\"enabled\":true}".utf8)) }
        await resumed.loadIntake()
        let retried = await resumed.submit()
        XCTAssertNotNil(retried)
        let retry = try XCTUnwrap(StubURLProtocol.requests.last)
        XCTAssertEqual(original.body, retry.body)
        XCTAssertEqual(header(original, "Idempotency-Key"), header(retry, "Idempotency-Key"))
        XCTAssertEqual(pending.input.placeLeads, ["Old depot", "Main Street"])
    }

    func testMalformedSuccessAndConflictKeepOriginalAttempt() async {
        for status in [201, 409] {
            let model = flow()
            fill(model)
            await model.loadIntake()
            StubURLProtocol.handler = { _, _ in (status, Data("{}".utf8)) }
            let result = await model.submit()
            XCTAssertNil(result)
            XCTAssertNotNil(model.pending)
            XCTAssertFalse(model.canEdit)
            model.discardLocalDraft()
            StubURLProtocol.handler = { _, _ in (200, Data("{\"enabled\":true}".utf8)) }
        }
    }

    func testValidationAndRateLimitAllowEditingAfterKnownRejection() async {
        for status in [422, 429] {
            let model = flow()
            fill(model)
            await model.loadIntake()
            StubURLProtocol.handler = { _, _ in (status, Data("{\"error\":{\"code\":\"invalid\",\"message\":\"Try later\",\"fields\":{\"source_links\":\"Use an https URL\"}}}".utf8)) }
            let result = await model.submit()
            XCTAssertNil(result)
            XCTAssertNil(model.pending)
            XCTAssertTrue(model.canEdit)
            XCTAssertEqual(model.fieldErrors["source_links"], "Use an https URL")
            model.discardLocalDraft()
            StubURLProtocol.handler = { _, _ in (200, Data("{\"enabled\":true}".utf8)) }
        }
    }

    func testClosedAndUnavailableIntakePreventPostButAllowOwnerReads() async {
        for data in [Data("{\"enabled\":false}".utf8), Data("{}".utf8)] {
            StubURLProtocol.handler = { _, path in
                if path == "/api/v1/route-brief-intake" { return (200, data) }
                return (200, Data("{\"items\":[],\"next_cursor\":\"\"}".utf8))
            }
            let model = flow()
            fill(model)
            await model.loadIntake()
            XCTAssertFalse(model.canSubmit)
            let result = await model.submit()
            XCTAssertNil(result)
            await model.loadPage()
            XCTAssertNil(model.listError)
        }
        XCTAssertFalse(StubURLProtocol.requests.contains { $0.method == "POST" })
    }

    func testGateRaceAndUnauthorizedKeepPendingDraft() async {
        for (status, code) in [(403, "intake_disabled"), (401, "unauthorized")] {
            let model = flow()
            fill(model)
            await model.loadIntake()
            StubURLProtocol.handler = { _, _ in (status, Data("{\"error\":{\"code\":\"\(code)\",\"message\":\"closed\"}}".utf8)) }
            let result = await model.submit()
            XCTAssertNil(result)
            XCTAssertNotNil(model.pending)
            XCTAssertFalse(model.canSubmit)
            XCTAssertEqual(model.needsSignIn, status == 401)
            model.discardLocalDraft()
            StubURLProtocol.handler = { _, _ in (200, Data("{\"enabled\":true}".utf8)) }
        }
    }

    func testAccountAndServerNamespacesDoNotShareDrafts() throws {
        let model = flow()
        fill(model)
        XCTAssertEqual(flow().fields.topic, " Rail workers ")
        XCTAssertEqual(flow(owner: "another-owner").fields.topic, "")
        let other = stubbedClient(token: "opening-token")
        other.baseURL = URL(string: "https://other.test")!
        XCTAssertEqual(flow(client: other).fields.topic, "")
        let file = try XCTUnwrap(FileManager.default.contentsOfDirectory(at: cache.directory, includingPropertiesForKeys: nil).first)
        let contents = try String(contentsOf: file, encoding: .utf8)
        XCTAssertFalse(contents.contains("opening-token"))
        XCTAssertTrue(try file.resourceValues(forKeys: [.isExcludedFromBackupKey]).isExcludedFromBackup == true)
    }

    func testDoubleTapSendsOnlyOneRequest() async {
        let model = flow()
        fill(model)
        await model.loadIntake()
        async let first = model.submit()
        async let second = model.submit()
        _ = await (first, second)
        XCTAssertEqual(StubURLProtocol.requests.filter { $0.method == "POST" }.count, 1)
    }

    func testAccountChangeDuringRequestCannotExposeResponseOrDeleteDraft() async {
        let identity = SuggestionIdentity()
        let model = flow(current: { identity.current })
        fill(model)
        await model.loadIntake()
        StubURLProtocol.handler = { _, _ in
            identity.expire()
            return (201, fixtureData("route_suggestion"))
        }
        let result = await model.submit()
        XCTAssertNil(result)
        XCTAssertNil(model.selected)
        model.close()
        XCTAssertEqual(model.fields.topic, "")
        XCTAssertTrue(model.items.isEmpty)
        XCTAssertNotNil(flow().pending)
    }

    func testOwnerMismatchAndNotFoundCannotPopulateDetailOrList() async {
        let wrongOwner = String(decoding: fixtureData("route_suggestion"), as: UTF8.self).replacingOccurrences(of: owner, with: "another-owner")
        StubURLProtocol.handler = { _, path in
            if path == "/api/v1/me/route-briefs" { return (200, Data("{\"items\":[\(wrongOwner)],\"next_cursor\":\"\"}".utf8)) }
            return (200, Data(wrongOwner.utf8))
        }
        let model = flow()
        await model.loadPage()
        XCTAssertTrue(model.items.isEmpty)
        XCTAssertNotNil(model.listError)
        await model.loadDetail("40000000-0000-4000-8000-000000000001")
        XCTAssertNil(model.selected)
        XCTAssertNotNil(model.detailError)
        StubURLProtocol.handler = { _, _ in (404, fixtureData("error_not_found")) }
        await model.loadDetail("missing")
        XCTAssertEqual(model.detailError, "This suggestion is not available for this account.")
    }

    func testPaginationUsesLookaheadAndDoesNotExposeExtraRow() async {
        let item = String(decoding: fixtureData("route_suggestion"), as: UTF8.self)
        let list = Data("{\"items\":[\(Array(repeating: item, count: 21).joined(separator: ","))],\"next_cursor\":\"\"}".utf8)
        StubURLProtocol.handler = { _, _ in (200, list) }
        let model = flow()
        await model.loadPage(offset: 20)
        XCTAssertEqual(model.items.count, 20)
        XCTAssertTrue(model.hasMore)
        XCTAssertEqual(model.offset, 20)
        XCTAssertTrue(StubURLProtocol.requests.last?.query?.contains("limit=21") == true)
        XCTAssertTrue(StubURLProtocol.requests.last?.query?.contains("offset=20") == true)
    }

    func testDiskFailurePreventsSubmission() async throws {
        let model = flow()
        fill(model)
        await model.loadIntake()
        try FileManager.default.removeItem(at: cache.directory)
        let result = await model.submit()
        XCTAssertNil(result)
        XCTAssertFalse(StubURLProtocol.requests.contains { $0.method == "POST" })
        XCTAssertTrue(model.message?.contains("No submission was sent") == true)
    }
}
