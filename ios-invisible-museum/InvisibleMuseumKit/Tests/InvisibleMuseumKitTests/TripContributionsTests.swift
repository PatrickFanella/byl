import XCTest
@testable import InvisibleMuseumKit

@MainActor
private final class ContributionTestAPI: ContributionAPI {
    var enabled = true
    var gateError: Error?, grantError: Error?, submitError: Error?, withdrawalError: Error?
    var choices: [(ContributionChoice, String)] = []
    var submissions: [(DeviationSummary, String)] = []
    var withdrawals: [(ContributionConsent, URL)] = []
    var onGrant: ((ContributionChoice, String) async throws -> Void)?
    var onGate: (() async -> Void)?
    var onWithdrawal: (() throws -> Void)?
    func gate() async throws -> ContributionGate {
        await onGate?()
        if let gateError { throw gateError }
        return ContributionGate(enabled: enabled, calibrated: false, detectorVersion: "uncalibrated-v1")
    }
    func grant(_ choice: ContributionChoice, key: String) async throws -> ContributionConsent {
        choices.append((choice, key)); try await onGrant?(choice, key)
        if let grantError { throw grantError }
        return ContributionConsent(id: "60000000-0000-4000-8000-000000000001", deletionHandle: String(repeating: "a", count: 64), expiresOn: ContributionDay.string(ContributionDay.adding(30, to: choice.requestedOn)!))
    }
    func submit(consent: ContributionConsent, summary: DeviationSummary, key: String) async throws {
        submissions.append((summary, key)); if let submitError { throw submitError }
    }
    func withdraw(_ consent: ContributionConsent, origin: URL) async throws {
        try onWithdrawal?(); withdrawals.append((consent, origin))
        if let withdrawalError { throw withdrawalError }
    }
}

@MainActor
final class TripContributionsTests: XCTestCase {
    private var directory: URL!
    private var store: ContributionStore!
    private var api: ContributionTestAPI!
    private var date = ISO8601DateFormatter().date(from: "2026-11-07T12:00:00Z")!
    private var identity = true
    private let owner = "50000000-0000-4000-8000-000000000001"
    override func setUp() async throws {
        try await super.setUp()
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("byl-trip-contributions-\(UUID().uuidString)")
        store = try ContributionStore(directory: directory, now: { [unowned self] in self.date }); api = ContributionTestAPI()
    }
    override func tearDown() async throws { try? FileManager.default.removeItem(at: directory); try await super.tearDown() }
    private func model(revision: TourRevision? = nil, settings: Bool = false) throws -> TripContributions {
        let selected = try (settings ? nil : revision ?? contributionRevisionFixture())
        let result = TripContributions(store: store, api: api, owner: owner, origin: URL(string: "https://stub.test")!, revision: selected, mode: "walk", sameIdentity: { [unowned self] in self.identity }, now: { [unowned self] in self.date })
        result.setForeground(true); return result
    }
    private func fix(_ x: Double, time: Date? = nil) -> DeviationFix {
        DeviationFix(point: DeviationPoint(lat: 41.8, lng: -88 + x / (6_371_000 * cos(41.8 * .pi / 180)) * 180 / .pi), accuracyM: 3, timestampMs: (time ?? date).timeIntervalSince1970 * 1000)
    }
    private func passage(_ model: TripContributions, start: Date? = nil, authorized: Bool = true) async {
        let start = start ?? date
        for i in 0...20 {
            date = start.addingTimeInterval(Double(i) * 10)
            if i % 3 == 0 { await model.refresh() }
            model.observe(fix(Double(i) * 20), authorizationGranted: authorized)
        }
    }

    func testEraseWhileGrantIsInFlightRetainsThenUsesLateCapability() async throws {
        let model = try model()
        let granted = expectation(description: "grant started")
        var reply: CheckedContinuation<Void, Never>?
        api.onGrant = { _, _ in
            granted.fulfill()
            await withCheckedContinuation { reply = $0 }
        }
        let choice = Task { await model.optIn() }
        await fulfillment(of: [granted], timeout: 2)
        XCTAssertEqual(model.retainedCount, 1)
        let stopped = expectation(description: "local erasure before network")
        let erase = Task {
            stopped.fulfill()
            await model.eraseAll()
        }
        await fulfillment(of: [stopped], timeout: 2)
        // Let eraseAll mark the durable record while the grant is suspended.
        await Task.yield()
        XCTAssertTrue(try XCTUnwrap(store.read().first).withdrawing)
        reply?.resume()
        await choice.value; await erase.value
        XCTAssertFalse(model.optedIn); XCTAssertTrue(model.paused)
        XCTAssertTrue(try store.read().isEmpty)
        XCTAssertEqual(api.withdrawals.count, 1)
        XCTAssertTrue(api.submissions.isEmpty)
    }

    func testEraseDuringAdmissionCheckCancelsThePendingChoice() async throws {
        let model = try model()
        let checked = expectation(description: "admission started")
        var reply: CheckedContinuation<Void, Never>?
        api.onGate = {
            checked.fulfill()
            await withCheckedContinuation { reply = $0 }
        }
        let choice = Task { await model.optIn() }
        await fulfillment(of: [checked], timeout: 2)
        // Profile owns a separate controller over the same private store.
        let profile = try self.model(settings: true)
        await profile.eraseAll()
        reply?.resume(); await choice.value
        XCTAssertTrue(api.choices.isEmpty)
        XCTAssertTrue(try store.read().isEmpty)
        XCTAssertFalse(model.optedIn)
    }

    func testNavigationHasNoImplicitConsentAndClosedGateCannotGrant() async throws {
        let model = try model()
        XCTAssertTrue(model.eligible); XCTAssertFalse(model.optedIn); XCTAssertTrue(model.paused)
        await passage(model); XCTAssertTrue(api.choices.isEmpty); XCTAssertTrue(try store.read().isEmpty)
        api.enabled = false; await model.optIn()
        XCTAssertTrue(api.choices.isEmpty); XCTAssertFalse(model.optedIn)
    }

    func testChoiceAndCapabilityAreDurableBeforeOnlyMinimalSummaryIsQueued() async throws {
        let model = try model()
        api.onGrant = { [unowned self] choice, key in
            let record = try XCTUnwrap(self.store.read().first)
            XCTAssertEqual(record.choice, choice); XCTAssertEqual(record.key, key)
        }
        await model.optIn(); XCTAssertTrue(model.optedIn); XCTAssertFalse(model.paused)
        XCTAssertNotNil(try store.read().first?.consent)
        await passage(model)
        let queued = try XCTUnwrap(store.read().first?.queue.first)
        XCTAssertEqual(model.pending, 1)
        let raw = try JSONSerialization.jsonObject(with: JSONEncoder().encode(queued.summary)) as! [String: Any]
        XCTAssertEqual(raw.count, 10)
        await model.flush(); XCTAssertEqual(api.submissions.count, 1); XCTAssertEqual(api.submissions.first?.1, queued.key)
        XCTAssertEqual(model.pending, 0)
    }

    func testLostGrantResponseRecoversTheExactChoiceAndKeyWithoutAutomaticResume() async throws {
        let first = try model()
        api.grantError = URLError(.networkConnectionLost)
        await first.optIn()
        let retained = try XCTUnwrap(store.read().first)
        XCTAssertNil(retained.consent); XCTAssertFalse(first.optedIn)
        api.grantError = nil; date = date.addingTimeInterval(86400)
        let restored = try model()
        await restored.refresh()
        XCTAssertEqual(api.choices.first?.0, api.choices.last?.0); XCTAssertEqual(api.choices.first?.1, api.choices.last?.1)
        XCTAssertEqual(try store.read().first?.key, retained.key)
        XCTAssertTrue(restored.paused)
        XCTAssertEqual(try store.read().first?.consent?.expiresOn, "2026-12-07")
    }

    func testLostSummaryResponseRetainsItsBodyAndKeyAndDailyDuplicateIsAcknowledged() async throws {
        let model = try model(); await model.optIn(); await passage(model)
        api.submitError = URLError(.networkConnectionLost); await model.flush()
        let pending = try XCTUnwrap(store.read().first?.queue.first)
        XCTAssertEqual(model.pending, 1); XCTAssertFalse(model.admitted)
        api.submitError = APIError(status: 409, code: "already_contributed", message: "Already contributed")
        await model.refresh()
        XCTAssertEqual(api.submissions.count, 2)
        XCTAssertEqual(api.submissions.first?.0, api.submissions.last?.0)
        XCTAssertEqual(api.submissions.last?.1, pending.key)
        XCTAssertEqual(model.pending, 0)
    }

    func testSevenDayExpiryRemovesOfflineSummariesBeforeAnyUpload() async throws {
        let model = try model(); await model.optIn(); await passage(model)
        XCTAssertEqual(model.pending, 1)
        date = ISO8601DateFormatter().date(from: "2026-11-14T00:00:00Z")!
        await model.refresh()
        XCTAssertTrue(api.submissions.isEmpty); XCTAssertEqual(model.pending, 0)
        XCTAssertEqual(try store.read().first?.queue.count, 0)
    }

    func testManualArrivalAndExplicitPauseCannotBeResumedByAdmissionRefresh() async throws {
        for event: DeviationDetector.Input in [.manualArrival, .pause] {
            let model = try model(); await model.optIn()
            model.lifecycle(event)
            await passage(model)
            XCTAssertEqual(model.pending, 0)
            model.lifecycle(.resume)
            await passage(model, start: date.addingTimeInterval(30))
            XCTAssertEqual(model.pending, 1)
            await model.eraseAll()
        }
    }

    func testBackgroundClearsEpisodeAndForegroundRechecksConsentBeforeFreshSamples() async throws {
        let model = try model(); await model.optIn()
        model.observe(fix(0), authorizationGranted: true)
        model.setForeground(false)
        await passage(model)
        XCTAssertEqual(model.pending, 0)
        model.setForeground(true)
        await passage(model, start: date.addingTimeInterval(30))
        XCTAssertEqual(model.pending, 1)
    }

    func testPermissionLossRequiresActualFreshAuthorizedFixAndEndStaysTerminal() async throws {
        let model = try model(); await model.optIn()
        model.lifecycle(.permissionLost)
        await passage(model, authorized: false); XCTAssertEqual(model.pending, 0)
        await passage(model, start: date.addingTimeInterval(30), authorized: true)
        XCTAssertEqual(model.pending, 1)
        await model.flush()
        model.lifecycle(.end); model.lifecycle(.resume)
        await passage(model, start: date.addingTimeInterval(30))
        XCTAssertEqual(api.submissions.count, 1)
        XCTAssertEqual(model.pending, 0)
    }

    func testStaleAdmissionAndCachedPreConsentFixesCannotCompleteAPassage() async throws {
        let model = try model(); await model.optIn()
        let start = date
        date = date.addingTimeInterval(1)
        model.observe(fix(0, time: start.addingTimeInterval(-1)), authorizationGranted: true)
        for i in 1...20 { date = start.addingTimeInterval(Double(i) * 10); model.observe(fix(Double(i) * 20), authorizationGranted: true) }
        XCTAssertEqual(model.pending, 0)
    }

    func testACachedFixBeforeConsentCannotSupplyTheRequiredPassageStart() async throws {
        let model = try model(); await model.optIn()
        let start = date
        model.observe(fix(0, time: start.addingTimeInterval(-1)), authorizationGranted: true)
        for i in 3...20 {
            date = start.addingTimeInterval(15 + Double(i - 3) * 10)
            if i % 3 == 0 { await model.refresh() }
            model.observe(fix(Double(i) * 20), authorizationGranted: true)
        }
        XCTAssertEqual(model.pending, 0)
    }

    func testAccountLossStopsUploadsAndSignedOutSettingsCanStillEraseKnownHandles() async throws {
        let model = try model(); await model.optIn(); await passage(model)
        identity = false; await model.flush()
        XCTAssertTrue(api.submissions.isEmpty)
        let settings = TripContributions(store: store, api: api, owner: nil, origin: URL(string: "https://stub.test")!, revision: nil, mode: "walk", sameIdentity: { false }, now: { [unowned self] in self.date })
        settings.setForeground(true)
        api.onWithdrawal = { [unowned self] in XCTAssertTrue(try self.store.read().allSatisfy { !$0.enabled && $0.withdrawing && $0.queue.isEmpty }) }
        await settings.eraseAll()
        XCTAssertEqual(api.withdrawals.count, 1); XCTAssertTrue(try store.read().isEmpty)
    }

    func testWrongOrMissingCapabilityResponseKeepsDeletionExplicitlyPending() async throws {
        let model = try model(); await model.optIn()
        api.withdrawalError = APIError(status: 404, code: "not_found", message: "Not found")
        await model.eraseAll()
        XCTAssertEqual(model.deletionPending, 1)
        XCTAssertTrue(try store.read().allSatisfy { !$0.enabled && $0.withdrawing && $0.queue.isEmpty })
        XCTAssertTrue(model.message.contains("could not be confirmed"))
    }

    func testStorageFailureBeforeConsentSendsNoGrant() async throws {
        let model = try model()
        try FileManager.default.removeItem(at: directory); try Data("blocked".utf8).write(to: directory)
        await model.optIn()
        XCTAssertTrue(api.choices.isEmpty); XCTAssertFalse(model.optedIn)
        XCTAssertTrue(model.message.contains("storage"))
    }
}
