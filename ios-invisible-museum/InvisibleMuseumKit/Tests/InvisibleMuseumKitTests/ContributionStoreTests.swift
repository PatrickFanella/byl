import XCTest
@testable import InvisibleMuseumKit

func contributionRecordFixture(owner: String = "50000000-0000-4000-8000-000000000001", day: String = "2026-11-07", receipt: Bool = true) -> ContributionRecord {
    let choice = ContributionChoice(tourID: "00000000-0000-4000-8000-00000000e0e1", revisionID: "00000000-0000-4000-8000-000000000051", mode: "walk", requestedOn: day)
    return ContributionRecord(origin: "https://stub.test", owner: owner, choice: choice, key: UUID().uuidString, enabled: true, withdrawing: false,
        consent: receipt ? ContributionConsent(id: "60000000-0000-4000-8000-000000000001", deletionHandle: String(repeating: "a", count: 64), expiresOn: "2026-12-07") : nil, queue: [])
}

func contributionSummaryFixture(revisionID: String = "00000000-0000-4000-8000-000000000051", day: String = "2026-11-07") -> DeviationSummary {
    DeviationSummary(context: DeviationContext(revisionID: revisionID, segmentID: "signal-fixture", mode: "walk", direction: "forward", reviewed: true, ambiguityReviewed: true, path: [], competingPaths: [], stopAreas: []), day: day, departures: 0)
}

@MainActor
final class ContributionStoreTests: XCTestCase {
    private var directory: URL!
    private var date = ISO8601DateFormatter().date(from: "2026-11-07T12:00:00Z")!
    private var store: ContributionStore!
    override func setUp() async throws {
        try await super.setUp()
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("byl-contribution-store-\(UUID().uuidString)")
        store = try ContributionStore(directory: directory, now: { [unowned self] in self.date })
    }
    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: directory)
        try await super.tearDown()
    }

    func testPrivateDirectoryIsExcludedBeforeChoiceAndCapabilityWrites() throws {
        XCTAssertEqual(try directory.resourceValues(forKeys: [.isExcludedFromBackupKey]).isExcludedFromBackup, true)
        let record = contributionRecordFixture()
        try store.write([record])
        XCTAssertEqual(try store.read(), [record])
        let saved = try Data(contentsOf: directory.appendingPathComponent("contributions-v1.json"))
        let raw = try XCTUnwrap(JSONSerialization.jsonObject(with: saved) as? [[String: Any]])
        XCTAssertEqual(Set(raw[0].keys), Set(["origin", "owner", "choice", "key", "enabled", "withdrawing", "consent", "queue"]))
    }

    func testSevenUTCDayQueueExpiryIsPhysicalAndNeverRenewsConsent() throws {
        var record = contributionRecordFixture()
        record.queue = [PendingDeviation(key: UUID().uuidString, summary: contributionSummaryFixture())]
        try store.write([record])
        date = ISO8601DateFormatter().date(from: "2026-11-13T23:59:59Z")!
        try store.prune(); XCTAssertEqual(try store.read().first?.queue.count, 1)
        date = ISO8601DateFormatter().date(from: "2026-11-14T00:00:00Z")!
        try store.prune(); XCTAssertEqual(try store.read().first?.queue.count, 0)
        XCTAssertEqual(try store.read().first?.consent, record.consent)
        date = ISO8601DateFormatter().date(from: "2026-12-07T00:00:00Z")!
        try store.prune(); XCTAssertTrue(try store.read().isEmpty)
        XCTAssertEqual(String(decoding: try Data(contentsOf: directory.appendingPathComponent("contributions-v1.json")), as: UTF8.self), "[]")
    }

    func testUnknownGrantReceiptRetainsOriginalChoiceAndKeyForAtMost31Days() throws {
        let record = contributionRecordFixture(receipt: false)
        try store.write([record])
        date = ISO8601DateFormatter().date(from: "2026-12-07T00:00:00Z")!
        try store.prune(); XCTAssertEqual(try store.read(), [record])
        date = ISO8601DateFormatter().date(from: "2026-12-08T00:00:00Z")!
        try store.prune(); XCTAssertTrue(try store.read().isEmpty)
    }

    func testGlobalStopClearsEveryQueueBeforeErasureAndRetainsCapabilities() throws {
        var first = contributionRecordFixture()
        first.queue = [PendingDeviation(key: UUID().uuidString, summary: contributionSummaryFixture())]
        let second = contributionRecordFixture(owner: "50000000-0000-4000-8000-000000000002", receipt: false)
        try store.write([first, second]); try store.stopAll()
        let saved = try store.read()
        XCTAssertEqual(saved.count, 2)
        XCTAssertTrue(saved.allSatisfy { !$0.enabled && $0.withdrawing && $0.queue.isEmpty })
        XCTAssertEqual(saved[0].consent, first.consent)
        XCTAssertEqual(saved[1].choice, second.choice); XCTAssertEqual(saved[1].key, second.key)
    }

    func testRecordQueueAndTotalByteBoundsDoNotReplaceThePriorFile() throws {
        let first = contributionRecordFixture()
        try store.write([first])
        let records = (0..<17).map { _ in contributionRecordFixture(owner: UUID().uuidString) }
        XCTAssertThrowsError(try store.write(records))
        var oversized = first
        oversized.queue = (0..<33).map { _ in PendingDeviation(key: UUID().uuidString, summary: contributionSummaryFixture()) }
        XCTAssertThrowsError(try store.write([oversized]))
        let huge = (0..<16).map { _ -> ContributionRecord in
            var r = contributionRecordFixture(owner: UUID().uuidString)
            r.queue = (0..<32).map { _ in PendingDeviation(key: UUID().uuidString, summary: contributionSummaryFixture()) }
            return r
        }
        XCTAssertThrowsError(try store.write(huge))
        XCTAssertEqual(try store.read(), [first])
    }

    func testMalformedOrRawFixFieldsFailClosedInsteadOfBeingSilentlyDecoded() throws {
        try store.write([contributionRecordFixture()])
        let file = directory.appendingPathComponent("contributions-v1.json")
        var raw = try JSONSerialization.jsonObject(with: Data(contentsOf: file)) as! [[String: Any]]
        raw[0]["raw_fixes"] = [["lat": 0, "lng": 0]] // Adversarial synthetic file, never visitor data.
        try JSONSerialization.data(withJSONObject: raw).write(to: file, options: .atomic)
        XCTAssertThrowsError(try store.read())
        try Data(repeating: 32, count: 131073).write(to: file, options: .atomic)
        XCTAssertThrowsError(try store.read())
    }

    func testMissingCapabilityAndChangedRevisionCannotStoreSummaries() {
        var noReceipt = contributionRecordFixture(receipt: false)
        noReceipt.queue = [PendingDeviation(key: UUID().uuidString, summary: contributionSummaryFixture())]
        XCTAssertThrowsError(try store.write([noReceipt]))
        var mismatched = contributionRecordFixture()
        mismatched.queue = [PendingDeviation(key: UUID().uuidString, summary: contributionSummaryFixture(revisionID: UUID().uuidString))]
        XCTAssertThrowsError(try store.write([mismatched]))
    }

    func testDiskWriteFailureIsVisibleAndDoesNotClaimDurableConsent() throws {
        try FileManager.default.removeItem(at: directory)
        try Data("blocked".utf8).write(to: directory)
        XCTAssertThrowsError(try store.write([contributionRecordFixture(receipt: false)]))
    }
}
