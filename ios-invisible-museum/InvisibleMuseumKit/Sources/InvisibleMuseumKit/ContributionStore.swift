import Foundation

public struct ContributionChoice: Codable, Equatable {
    public let version: Int
    public let accepted: Bool
    public let tourID, revisionID, mode, requestedOn: String
    enum CodingKeys: String, CodingKey {
        case version, accepted, mode
        case tourID = "tour_id", revisionID = "revision_id", requestedOn = "requested_on"
    }
    init(tourID: String, revisionID: String, mode: String, requestedOn: String) {
        version = 1; accepted = true; self.tourID = tourID; self.revisionID = revisionID
        self.mode = mode; self.requestedOn = requestedOn
    }
}

public struct ContributionConsent: Codable, Equatable {
    public let id, deletionHandle, expiresOn: String
    enum CodingKeys: String, CodingKey { case id; case deletionHandle = "deletion_handle", expiresOn = "expires_on" }
}

struct PendingDeviation: Codable, Equatable {
    let key: String
    let summary: DeviationSummary
}

struct ContributionRecord: Codable, Equatable {
    let origin, owner: String
    let choice: ContributionChoice
    let key: String
    var enabled: Bool
    var withdrawing: Bool
    var consent: ContributionConsent?
    var queue: [PendingDeviation]
    var id: String { [origin, owner, choice.tourID, choice.revisionID, choice.mode].joined(separator: "|") }
}

enum ContributionFailure: Error { case storageUnavailable, invalidContract }

@MainActor
enum ContributionDay {
    private static let formatter: DateFormatter = {
        let f = DateFormatter(); f.calendar = Calendar(identifier: .gregorian); f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(secondsFromGMT: 0); f.dateFormat = "yyyy-MM-dd"; f.isLenient = false
        return f
    }()
    static func string(_ date: Date) -> String { formatter.string(from: date) }
    static func parse(_ value: String) -> Date? {
        guard value.count == 10, value.range(of: "^[0-9]{4}-[0-9]{2}-[0-9]{2}$", options: .regularExpression) != nil,
              let date = formatter.date(from: value), date.timeIntervalSince1970 >= 0, string(date) == value else { return nil }
        return date
    }
    static func adding(_ days: Int, to value: String) -> Date? { parse(value)?.addingTimeInterval(Double(days) * 86400) }
}

/// One private, backup-excluded file; no raw-fix type is encodable by this store.
/// Every write/read is bounded and checked before any consent or upload request.
@MainActor
public final class ContributionStore {
    private var erasureWritePending = false
    private(set) var choiceGeneration = 0
    public let directory: URL
    private let file: URL
    private let now: () -> Date
    private let encoder: JSONEncoder = {
        let e = JSONEncoder(); e.outputFormatting = [.sortedKeys]; return e
    }()
    public init(directory: URL, now: @escaping () -> Date = Date.init) throws {
        self.directory = directory; self.file = directory.appendingPathComponent("contributions-v1.json"); self.now = now
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        var location = directory
        var attributes = URLResourceValues(); attributes.isExcludedFromBackup = true
        try location.setResourceValues(attributes)
        guard try location.resourceValues(forKeys: [.isExcludedFromBackupKey]).isExcludedFromBackup == true else { throw ContributionFailure.storageUnavailable }
    }

    private func bytes() throws -> Data {
        let handle = try FileHandle(forReadingFrom: file); defer { try? handle.close() }
        let data = try handle.read(upToCount: 131073) ?? Data()
        guard data.count <= 131072 else { throw ContributionFailure.storageUnavailable }
        return data
    }
    func read() throws -> [ContributionRecord] {
        guard FileManager.default.fileExists(atPath: file.path) else { return [] }
        do {
            let data = try bytes()
            guard let raw = try JSONSerialization.jsonObject(with: data) as? [[String: Any]], raw.count <= 16,
                  raw.allSatisfy(strictKeys) else { throw ContributionFailure.storageUnavailable }
            let records = try JSONDecoder().decode([ContributionRecord].self, from: data)
            guard records.allSatisfy(valid), Set(records.map(\.id)).count == records.count else { throw ContributionFailure.storageUnavailable }
            return records
        } catch { throw ContributionFailure.storageUnavailable }
    }
    func write(_ records: [ContributionRecord]) throws {
        do {
            guard records.count <= 16, records.allSatisfy(valid), Set(records.map(\.id)).count == records.count else { throw ContributionFailure.storageUnavailable }
            let data = try encoder.encode(records)
            guard data.count <= 131072 else { throw ContributionFailure.storageUnavailable }
            // Directory exclusion is set before the first capability can be written.
            guard try directory.resourceValues(forKeys: [.isExcludedFromBackupKey]).isExcludedFromBackup == true else { throw ContributionFailure.storageUnavailable }
            #if os(iOS)
            try data.write(to: file, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
            #else
            try data.write(to: file, options: .atomic)
            #endif
            guard try bytes() == data else { throw ContributionFailure.storageUnavailable }
        } catch { throw ContributionFailure.storageUnavailable }
    }
    func update(_ id: String, key: String, _ change: (inout ContributionRecord) -> Void) throws {
        var records = try read()
        guard let i = records.firstIndex(where: { $0.id == id && $0.key == key }) else { throw ContributionFailure.storageUnavailable }
        change(&records[i]); try write(records)
    }
    func prune() throws {
        if erasureWritePending { try stopAll() }
        let date = now(), today = ContributionDay.string(date)
        var records = try read().filter { r in
            if let consent = r.consent { return consent.expiresOn > today }
            return ContributionDay.adding(31, to: r.choice.requestedOn).map { date < $0 } == true
        }
        for i in records.indices {
            records[i].queue.removeAll { p in
                p.summary.observationPeriod > today || ContributionDay.adding(7, to: p.summary.observationPeriod).map { date >= $0 } != false
            }
        }
        try write(records)
    }
    func stopAll() throws {
        choiceGeneration += 1
        erasureWritePending = true
        var records = try read()
        for i in records.indices { records[i].enabled = false; records[i].withdrawing = true; records[i].queue = [] }
        try write(records)
        erasureWritePending = false
    }
    static func validSummary(_ s: DeviationSummary) -> Bool {
        s.schemaVersion == 1 && s.detectorVersion == DeviationDetector.version && !s.calibrated &&
        UUID(uuidString: s.revisionID) != nil && !s.segmentID.isEmpty && s.segmentID.utf8.count <= 120 &&
        ["walk", "bike"].contains(s.mode) && ["forward", "reverse"].contains(s.direction) &&
        ContributionDay.parse(s.observationPeriod) != nil && s.qualifiedPassages == 1 && [0, 1].contains(s.sustainedDepartureRejoins)
    }
    static func validConsent(_ consent: ContributionConsent, choice: ContributionChoice) -> Bool {
        guard UUID(uuidString: consent.id) != nil, consent.deletionHandle.count == 64,
              consent.deletionHandle.allSatisfy({ $0.isASCII && $0.isHexDigit }),
              let expiry = ContributionDay.parse(consent.expiresOn), let first = ContributionDay.adding(30, to: choice.requestedOn),
              let last = ContributionDay.adding(31, to: choice.requestedOn) else { return false }
        return expiry >= first && expiry <= last
    }
    private func valid(_ r: ContributionRecord) -> Bool {
        guard let url = URLComponents(string: r.origin), ["http", "https"].contains(url.scheme?.lowercased() ?? ""),
              url.host?.isEmpty == false, url.user == nil, url.password == nil, url.query == nil, url.fragment == nil,
              r.origin.utf8.count <= 2048, UUID(uuidString: r.owner) != nil,
              r.choice.version == 1, r.choice.accepted, UUID(uuidString: r.choice.tourID) != nil,
              UUID(uuidString: r.choice.revisionID) != nil, ["walk", "bike"].contains(r.choice.mode),
              ContributionDay.parse(r.choice.requestedOn) != nil, validKey(r.key), r.queue.count <= 32,
              r.consent.map({ Self.validConsent($0, choice: r.choice) }) ?? true,
              r.consent != nil || r.queue.isEmpty,
              !r.withdrawing || (!r.enabled && r.queue.isEmpty),
              Set(r.queue.map(\.key)).count == r.queue.count else { return false }
        return r.queue.allSatisfy { validKey($0.key) && Self.validSummary($0.summary) && $0.summary.revisionID == r.choice.revisionID && $0.summary.mode == r.choice.mode }
    }
    private func validKey(_ value: String) -> Bool { (16...128).contains(value.utf8.count) && value.utf8.allSatisfy { (33...126).contains($0) } }
    private func strictKeys(_ r: [String: Any]) -> Bool {
        func only(_ value: [String: Any], _ keys: [String]) -> Bool { Set(value.keys).isSubset(of: Set(keys)) }
        guard only(r, ["origin", "owner", "choice", "key", "enabled", "withdrawing", "consent", "queue"]),
              let choice = r["choice"] as? [String: Any], only(choice, ["version", "accepted", "tour_id", "revision_id", "mode", "requested_on"]),
              let queue = r["queue"] as? [[String: Any]] else { return false }
        if let consent = r["consent"], !(consent is NSNull) {
            guard let fields = consent as? [String: Any], only(fields, ["id", "deletion_handle", "expires_on"]) else { return false }
        }
        return queue.allSatisfy { p in
            guard only(p, ["key", "summary"]), let summary = p["summary"] as? [String: Any] else { return false }
            return only(summary, ["schema_version", "detector_version", "calibrated", "revision_id", "segment_id", "mode", "direction", "observation_period", "qualified_passages", "sustained_departure_rejoins"])
        }
    }
}
