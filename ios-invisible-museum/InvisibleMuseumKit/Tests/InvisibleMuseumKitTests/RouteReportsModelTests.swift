import XCTest
@testable import InvisibleMuseumKit

private final class ReportTestIdentity: @unchecked Sendable {
    private let lock = NSLock()
    private var value = true
    var current: Bool { lock.withLock { value } }
    func expire() { lock.withLock { value = false } }
}

@MainActor
final class RouteReportsModelTests: XCTestCase {
    private let owner = "50000000-0000-4000-8000-000000000001"
    private var cache: DiskCache!
    override func setUp() async throws {
        try await super.setUp()
        StubURLProtocol.reset()
        cache = try DiskCache(directory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    }
    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: cache.directory)
        try await super.tearDown()
    }
    private func context(mode: String = "walk", change: ((inout [String: Any]) -> Void)? = nil) throws -> RouteReportContext {
        var raw = try XCTUnwrap(JSONSerialization.jsonObject(with: fixtureData("tour-revision-1")) as? [String: Any])
        change?(&raw)
        return RouteReportContext(revision: try JSONDecoder().decode(TourRevision.self, from: JSONSerialization.data(withJSONObject: raw)), mode: mode)
    }
    private func flow(_ context: RouteReportContext? = nil, owner: String? = nil, client: APIClient? = nil, current: @escaping () -> Bool = { true }) -> RouteReportsModel {
        RouteReportsModel(api: Endpoints(client: client ?? stubbedClient(token: "opening-token")), cache: cache, accountID: owner ?? self.owner, context: context, isCurrent: current)
    }
    private func response(_ input: RouteReportInput? = nil, account: String? = nil, status: String = "review") throws -> Data {
        var raw = try XCTUnwrap(JSONSerialization.jsonObject(with: fixtureData("route_report")) as? [String: Any])
        if let input { raw["submitted"] = try JSONSerialization.jsonObject(with: JSONEncoder().encode(input)) }
        raw["owner_account_id"] = account ?? owner; raw["status"] = status
        return try JSONSerialization.data(withJSONObject: raw)
    }
    private func header(_ request: StubURLProtocol.Recorded, _ name: String) -> String? {
        request.headers.first { $0.key.lowercased() == name.lowercased() }?.value
    }

    func testCanonicalPrivateReportFixtureAndOutcomeDecode() throws {
        let report = try JSONDecoder().decode(RouteReport.self, from: fixtureData("route_report"))
        XCTAssertEqual(report.ownerAccountID, owner)
        XCTAssertEqual(report.submitted.affected.kind, "entrance")
        XCTAssertNil(report.submitted.annotation); XCTAssertNil(report.submitted.alternative)
        XCTAssertEqual(report.reason, "Please confirm the operator notice and closure dates.")
        let outcome = try JSONDecoder().decode(RouteCorrectionOutcome.self, from: Data("{\"revision_id\":\"new\",\"previous_revision_id\":\"old\",\"temporary\":true,\"review_on\":\"2026-12-01\",\"reassessment_required\":false}".utf8))
        XCTAssertEqual(outcome.reviewOn, "2026-12-01"); XCTAssertTrue(outcome.temporary)
    }
    func testTargetsRetainStopsEntrancesAndActualLegsWithoutRequiringKnownAccess() throws {
        let context = try context()
        XCTAssertNil(context.unavailableReason)
        XCTAssertEqual(context.targets.filter { $0.affected.kind == "stop" }.count, 2)
        XCTAssertEqual(context.targets.filter { $0.affected.kind == "entrance" }.count, 2)
        XCTAssertEqual(context.targets.filter { $0.affected.kind == "segment" }.count, context.revision.snapshot?.routes["walk"]?.legs.count)
        let noLegs = try self.context { raw in
            var snapshot = raw["snapshot"] as! [String: Any]
            var routes = snapshot["routes"] as! [String: [String: Any]]
            routes["walk"]?["legs"] = []; snapshot["routes"] = routes; raw["snapshot"] = snapshot
        }
        XCTAssertTrue(noLegs.targets.allSatisfy { $0.affected.kind != "segment" })
    }
    func testMismatchedLegIDsAreRejectedAndClosingReturnIsRetained() throws {
        let context = try context { raw in
            var snapshot = raw["snapshot"] as! [String: Any]
            let stops = (snapshot["tour"] as! [String: Any])["stops"] as! [[String: Any]]
            var routes = snapshot["routes"] as! [String: [String: Any]]
            var legs = routes["walk"]!["legs"] as! [[String: Any]]
            legs[0]["from_stop_id"] = stops[1]["id"]
            var returning = legs[0]
            returning["from_position"] = 2; returning["to_position"] = 1
            returning["from_stop_id"] = stops[1]["id"]; returning["to_stop_id"] = stops[0]["id"]; returning["return"] = true
            routes["walk"]?["legs"] = [legs[0], returning]; snapshot["routes"] = routes; raw["snapshot"] = snapshot
        }
        let segments = context.targets.filter { $0.affected.kind == "segment" }
        XCTAssertEqual(segments.count, 1); XCTAssertTrue(segments[0].label.hasSuffix("(return)"))
        XCTAssertEqual(segments[0].affected.fromStopID, context.revision.snapshot!.tour.stops[1].id)
    }
    func testUnsupportedModeAndNestedRevisionMismatchCannotSubmit() throws {
        let unsupported = flow(try context(mode: "drive"))
        XCTAssertFalse(unsupported.canSubmit)
        let mismatch = try context { raw in
            var snapshot = raw["snapshot"] as! [String: Any]
            var tour = snapshot["tour"] as! [String: Any]
            tour["revision_id"] = "different"; snapshot["tour"] = tour; raw["snapshot"] = snapshot
        }
        XCTAssertFalse(flow(mismatch).canSubmit); XCTAssertTrue(mismatch.targets.isEmpty)
    }
    func testReportOnlySubmissionPinsRevisionModeAndHasNoJourneyOrAutomaticLocation() async throws {
        let context = try context()
        let model = flow(context)
        var fields = model.fields; fields.target = context.targets.first { $0.affected.kind == "segment" }!.id
        fields.details = " Report only "; model.edit(fields)
        let input = try fields.input(context: context)
        let saved = try response(input)
        StubURLProtocol.handler = { _, _ in (201, saved) }
        let report = await model.submit()
        XCTAssertNotNil(report); XCTAssertNil(model.pending)
        let request = try XCTUnwrap(StubURLProtocol.requests.last)
        XCTAssertEqual(request.path, "/api/v1/me/route-reports")
        XCTAssertEqual(header(request, "Authorization"), "Bearer opening-token")
        XCTAssertNotNil(UUID(uuidString: try XCTUnwrap(header(request, "Idempotency-Key"))))
        let posted = try JSONDecoder().decode(RouteReportInput.self, from: XCTUnwrap(request.body))
        XCTAssertEqual(posted.revisionID, context.revision.id); XCTAssertEqual(posted.mode, "walk")
        XCTAssertEqual(posted.details, "Report only"); XCTAssertNil(posted.annotation); XCTAssertNil(posted.alternative)
        let keys = (try JSONSerialization.jsonObject(with: XCTUnwrap(request.body)) as! [String: Any]).keys
        XCTAssertFalse(keys.contains("visited_stop_ids")); XCTAssertFalse(keys.contains("started_at")); XCTAssertFalse(keys.contains("trace"))
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(atPath: cache.directory.path).isEmpty)
    }
    func testOptionalPointAndAlternativeUseExplicitFiniteCoordinates() throws {
        let context = try context()
        var fields = ReportFields(); fields.target = context.targets[1].id
        fields.latitude = "41.8"; fields.longitude = "-88.0"; fields.alternativePoints = "-88.0, 41.8\n-88.0001, 41.8001"
        let input = try fields.input(context: context)
        XCTAssertEqual(input.annotation, ReportPoint(lat: 41.8, lng: -88.0))
        XCTAssertEqual(input.alternative?.coordinates.count, 2)
        fields.alternativePoints = "-88,41,9\n-88,41"
        XCTAssertThrowsError(try fields.input(context: context))
        fields.alternativePoints = ""; fields.latitude = "nan"
        XCTAssertThrowsError(try fields.input(context: context))
        fields.latitude = "41"; fields.longitude = ""
        XCTAssertThrowsError(try fields.input(context: context))
    }
    func testUncertainRetrySurvivesCloseAndKeepsExactBodyAndKey() async throws {
        let context = try context()
        let first = flow(context)
        var fields = first.fields; fields.details = " Keep this observation "; first.edit(fields)
        StubURLProtocol.handler = { _, _ in (503, Data()) }
        let failed = await first.submit(); XCTAssertNil(failed)
        let original = try XCTUnwrap(StubURLProtocol.requests.last)
        let pending = try XCTUnwrap(first.pending)
        first.close()
        let resumed = flow(context)
        XCTAssertEqual(resumed.fields.details, fields.details); XCTAssertEqual(resumed.pending, pending)
        XCTAssertFalse(resumed.canEdit)
        let saved = try response(pending.input)
        StubURLProtocol.handler = { _, _ in (201, saved) }
        let retried = await resumed.submit(); XCTAssertNotNil(retried)
        let retry = try XCTUnwrap(StubURLProtocol.requests.last)
        XCTAssertEqual(retry.body, original.body); XCTAssertEqual(header(retry, "Idempotency-Key"), header(original, "Idempotency-Key"))
    }
    func testDiskFailureBlocksPostAndKeepsInMemoryAttempt() async throws {
        let model = flow(try context())
        try FileManager.default.removeItem(at: cache.directory)
        try Data("blocked".utf8).write(to: cache.directory)
        let result = await model.submit(); XCTAssertNil(result)
        XCTAssertTrue(StubURLProtocol.requests.isEmpty)
        XCTAssertTrue(model.message?.contains("No report was sent") == true)
        let pending = try XCTUnwrap(model.pending)
        try FileManager.default.removeItem(at: cache.directory)
        try FileManager.default.createDirectory(at: cache.directory, withIntermediateDirectories: true)
        let saved = try response(pending.input)
        StubURLProtocol.handler = { _, _ in (201, saved) }
        let retried = await model.submit(); XCTAssertNotNil(retried)
        XCTAssertEqual(header(try XCTUnwrap(StubURLProtocol.requests.last), "Idempotency-Key"), pending.key)
    }
    func testDraftsAreScopedToAccountOriginRevisionAndModeAndExcludedFromBackup() throws {
        let context = try context()
        let model = flow(context)
        var fields = model.fields; fields.details = "Private unfinished observation"; model.edit(fields)
        XCTAssertEqual(flow(context).fields.details, fields.details)
        XCTAssertEqual(flow(context, owner: "another-owner").fields.details, "")
        XCTAssertEqual(flow(try self.context(mode: "bike")).fields.details, "")
        let client = APIClient(baseURL: URL(string: "https://another.test")!, tokenProvider: { "other" })
        XCTAssertEqual(flow(context, client: client).fields.details, "")
        let files = try FileManager.default.contentsOfDirectory(at: cache.directory, includingPropertiesForKeys: [.isExcludedFromBackupKey])
        XCTAssertEqual(files.count, 1); XCTAssertEqual(try files[0].resourceValues(forKeys: [.isExcludedFromBackupKey]).isExcludedFromBackup, true)
    }
    func testValidationRejectionKeepsFieldsAndUnlocksOnlyAfterSavingClearedAttempt() async throws {
        let model = flow(try context())
        var fields = model.fields; fields.details = "Keep editing"; model.edit(fields)
        StubURLProtocol.handler = { _, _ in (422, Data("{\"error\":{\"code\":\"validation\",\"message\":\"Invalid\",\"fields\":{\"source_url\":\"Use a public HTTP source\"}}}".utf8)) }
        let result = await model.submit(); XCTAssertNil(result)
        XCTAssertNil(model.pending); XCTAssertTrue(model.canEdit); XCTAssertEqual(model.fields.details, fields.details)
        XCTAssertEqual(model.fieldErrors["source_url"], "Use a public HTTP source")
    }
    func testForeignReportAndExpiredIdentityCannotPopulatePrivateState() async throws {
        let context = try context()
        let foreign = try response(try flow(context).fields.input(context: context), account: "another-owner")
        let model = flow(context)
        StubURLProtocol.handler = { _, _ in (201, foreign) }
        let result = await model.submit(); XCTAssertNil(result); XCTAssertNil(model.selected); XCTAssertNotNil(model.pending)
        let identity = ReportTestIdentity()
        let expired = flow(context, current: { identity.current })
        StubURLProtocol.handler = { _, _ in identity.expire(); return (200, foreign) }
        await expired.loadPage()
        XCTAssertTrue(expired.items.isEmpty)
        XCTAssertFalse(expired.canSubmit)
    }
    func testOwnerDecisionAndCorrectionAreReadWithoutAdoptingTripRevision() async throws {
        let fixture = try JSONDecoder().decode(RouteReport.self, from: fixtureData("route_report"))
        let model = flow()
        StubURLProtocol.handler = { _, path in
            if path.hasSuffix("/correction") {
                return (200, Data("{\"revision_id\":\"new-review\",\"previous_revision_id\":\"\(fixture.submitted.revisionID)\",\"temporary\":true,\"review_on\":\"2026-12-01\",\"reassessment_required\":true}".utf8))
            }
            return (200, fixtureData("route_report"))
        }
        await model.loadDetail(fixture.id)
        XCTAssertEqual(model.selected?.reason, fixture.reason)
        XCTAssertEqual(model.outcome?.revisionID, "new-review"); XCTAssertTrue(model.outcome?.reassessmentRequired == true)
        XCTAssertTrue(StubURLProtocol.requests.allSatisfy { $0.method == "GET" && $0.path.hasPrefix("/api/v1/me/route-reports/") })
    }
    func testNoCorrectionYetIsNormalAndForeignDetailIsRefused() async throws {
        let fixture = try JSONDecoder().decode(RouteReport.self, from: fixtureData("route_report"))
        let model = flow()
        StubURLProtocol.handler = { _, path in path.hasSuffix("/correction") ? (404, Data()) : (200, fixtureData("route_report")) }
        await model.loadDetail(fixture.id)
        XCTAssertNotNil(model.selected); XCTAssertNil(model.outcome); XCTAssertNil(model.outcomeError)
        let foreign = try response(account: "another-owner")
        StubURLProtocol.handler = { _, _ in (200, foreign) }
        await model.loadDetail(fixture.id)
        XCTAssertNil(model.selected); XCTAssertNotNil(model.detailError); XCTAssertNil(model.outcome)
    }

    func testFailedDiscardAndFailedValidationCleanupKeepThePendingIdentity() async throws {
        let context = try context()
        let model = flow(context)
        var fields = model.fields; fields.details = "Keep private input"; model.edit(fields)
        StubURLProtocol.handler = { _, _ in (503, Data()) }
        _ = await model.submit()
        let pending = try XCTUnwrap(model.pending)
        let files = try FileManager.default.contentsOfDirectory(at: cache.directory, includingPropertiesForKeys: nil)
        try FileManager.default.setAttributes([.posixPermissions: 0o500], ofItemAtPath: cache.directory.path)
        model.discardLocalDraft()
        XCTAssertEqual(model.pending, pending); XCTAssertEqual(model.fields.details, fields.details)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: cache.directory.path)
        // The initial durable write succeeds, then storage fails while persisting
        // the confirmed validation rejection. The old retry must remain locked.
        StubURLProtocol.handler = { _, _ in
            try! FileManager.default.removeItem(at: files[0])
            try! FileManager.default.createDirectory(at: files[0], withIntermediateDirectories: true)
            try! Data("retain".utf8).write(to: files[0].appendingPathComponent("blocker"))
            return (422, Data("{\"error\":{\"code\":\"validation\",\"message\":\"Invalid\"}}".utf8))
        }
        _ = await model.submit()
        XCTAssertEqual(model.pending, pending); XCTAssertEqual(model.fields.details, fields.details)
        XCTAssertFalse(model.canEdit)
    }

    func testUnreadableDraftAndUnauthorizedResponseKeepInputPrivateForSameAccount() async throws {
        let context = try context()
        let model = flow(context)
        var fields = model.fields; fields.details = "Account-scoped input"; model.edit(fields)
        StubURLProtocol.handler = { _, _ in (401, Data("{\"error\":{\"code\":\"unauthorized\",\"message\":\"Expired\"}}".utf8)) }
        _ = await model.submit()
        XCTAssertTrue(model.needsSignIn); XCTAssertNotNil(model.pending); XCTAssertFalse(model.canSubmit)
        let files = try FileManager.default.contentsOfDirectory(at: cache.directory, includingPropertiesForKeys: nil)
        try Data("corrupt retained retry".utf8).write(to: files[0])
        let restored = flow(context)
        XCTAssertFalse(restored.canEdit); XCTAssertFalse(restored.canSubmit)
        XCTAssertTrue(restored.message?.contains("could not be read") == true)
        XCTAssertEqual(flow(context, owner: "foreign-account").fields.details, "")
        restored.discardLocalDraft()
        XCTAssertTrue(restored.canEdit)
    }

    func testBoundedPageOffsetAndWrongCorrectionRevisionAreHandledWithoutLeakingPreviousDetail() async throws {
        var raw = try XCTUnwrap(JSONSerialization.jsonObject(with: fixtureData("route_report")) as? [String: Any])
        let reports = (0..<21).map { index -> [String: Any] in var row = raw; row["id"] = "report-\(index)"; return row }
        let page = try JSONSerialization.data(withJSONObject: ["items": reports, "next_cursor": ""])
        let model = flow()
        StubURLProtocol.handler = { _, _ in (200, page) }
        await model.loadPage(offset: 40)
        XCTAssertEqual(model.items.count, 20); XCTAssertTrue(model.hasMore); XCTAssertEqual(model.offset, 40)
        let query = URLComponents(string: "https://stub.test/?\(StubURLProtocol.requests.last!.query ?? "")")?.queryItems
        XCTAssertTrue(query?.contains(URLQueryItem(name: "limit", value: "21")) == true)
        XCTAssertTrue(query?.contains(URLQueryItem(name: "offset", value: "40")) == true)
        raw["owner_account_id"] = "another-account"
        let foreign = try JSONSerialization.data(withJSONObject: ["items": [raw], "next_cursor": ""])
        StubURLProtocol.handler = { _, _ in (200, foreign) }
        await model.loadPage()
        XCTAssertTrue(model.items.isEmpty); XCTAssertNotNil(model.listError)
        let fixture = try JSONDecoder().decode(RouteReport.self, from: fixtureData("route_report"))
        StubURLProtocol.handler = { _, path in
            path.hasSuffix("/correction") ? (200, Data("{\"revision_id\":\"new\",\"previous_revision_id\":\"wrong-source\",\"temporary\":false,\"review_on\":null,\"reassessment_required\":false}".utf8)) : (200, fixtureData("route_report"))
        }
        await model.loadDetail(fixture.id)
        XCTAssertNotNil(model.selected); XCTAssertNil(model.outcome); XCTAssertNotNil(model.outcomeError)
    }
}
