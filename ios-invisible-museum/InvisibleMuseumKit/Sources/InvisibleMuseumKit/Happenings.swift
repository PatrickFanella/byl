import Foundation

/// Optional public announcements, never part of a retained historical trip.
public struct Happening: Decodable, Equatable, Identifiable {
    public let id, revisionID, placeID, title, summary, organizer: String
    public let startsOn, endsOn, timezone, declaredState, state: String
    public let reviewOn, expiresOn, lastCheckedAt, sourceHealth: String
    public let sources: [HappeningSource]
    public let issues: [String]
    enum CodingKeys: String, CodingKey {
        case id, title, summary, organizer, timezone, state, sources, issues
        case revisionID = "revision_id", placeID = "place_id", startsOn = "starts_on", endsOn = "ends_on"
        case declaredState = "declared_state", reviewOn = "review_on", expiresOn = "expires_on"
        case lastCheckedAt = "last_checked_at", sourceHealth = "source_health"
    }

    public func presentationStatus(now: Date) -> HappeningStatus {
        guard ["upcoming", "active", "ended", "expired", "needs_review", "cancelled", "date_conflict", "source_unavailable"].contains(state),
              ["scheduled", "cancelled", "date_conflict", "source_unavailable"].contains(declaredState),
              [startsOn, endsOn, reviewOn, expiresOn].allSatisfy({ Self.day($0) != nil }),
              endsOn >= startsOn, let zone = TimeZone(identifier: timezone), let checked = Self.instant(lastCheckedAt) else { return .unavailable }
        var calendar = Calendar(identifier: .gregorian); calendar.timeZone = zone
        let parts = calendar.dateComponents([.year, .month, .day], from: now)
        let today = String(format: "%04d-%02d-%02d", parts.year ?? 0, parts.month ?? 0, parts.day ?? 0)
        if declaredState == "cancelled" || state == "cancelled" { return .cancelled }
        if declaredState == "date_conflict" || state == "date_conflict" { return .dateConflict }
        if state == "expired" || today >= expiresOn { return .expired }
        if sourceHealth == "unavailable" || declaredState == "source_unavailable" || state == "source_unavailable" { return .sourceUnavailable }
        if sourceHealth != "unchanged" || !issues.isEmpty || state == "needs_review" || checked > now || now.timeIntervalSince(checked) >= 7 * 86_400 || today >= reviewOn || !sources.contains(where: { Self.sourceURL($0.url) != nil }) { return .needsReview }
        if state == "ended" || today > endsOn { return .ended }
        return today < startsOn ? .upcoming : .active
    }

    public var occurrenceText: String {
        guard let start = Self.day(startsOn), let end = Self.day(endsOn) else { return "Occurrence dates unavailable" }
        let f = DateFormatter(); f.locale = Locale(identifier: "en_US_POSIX"); f.timeZone = TimeZone(secondsFromGMT: 0); f.dateFormat = "MMM d, yyyy"
        return "\(f.string(from: start))\(startsOn == endsOn ? "" : " – \(f.string(from: end))") · \(timezone) · dates only"
    }
    public var checkedText: String {
        guard let date = Self.instant(lastCheckedAt) else { return "Source check unavailable" }
        let f = DateFormatter(); f.locale = Locale(identifier: "en_US_POSIX"); f.timeZone = TimeZone(secondsFromGMT: 0); f.dateFormat = "yyyy-MM-dd HH:mm 'UTC'"
        return "Last source check: \(f.string(from: date))"
    }
    public static func sourceURL(_ value: String) -> URL? {
        guard let c = URLComponents(string: value), ["https", "http"].contains(c.scheme?.lowercased() ?? ""), c.host != nil, c.user == nil, c.password == nil else { return nil }
        return c.url
    }
    private static func day(_ value: String) -> Date? {
        guard value.count == 10 else { return nil }
        let f = DateFormatter(); f.locale = Locale(identifier: "en_US_POSIX"); f.calendar = Calendar(identifier: .gregorian); f.timeZone = TimeZone(secondsFromGMT: 0); f.dateFormat = "yyyy-MM-dd"; f.isLenient = false
        guard let date = f.date(from: value), f.string(from: date) == value else { return nil }; return date
    }
    private static func instant(_ value: String) -> Date? {
        guard Self.day(String(value.prefix(10))) != nil else { return nil }
        let f = ISO8601DateFormatter(); f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = f.date(from: value) { return date }; f.formatOptions = [.withInternetDateTime]; return f.date(from: value)
    }
}

public struct HappeningSource: Decodable, Equatable {
    public let url, title, publisher, version, retrievedOn, license, licenseURL: String
    enum CodingKeys: String, CodingKey {
        case url, title, publisher, version, license
        case retrievedOn = "retrieved_on", licenseURL = "license_url"
    }
}

public enum HappeningStatus: String {
    case active, upcoming, ended, expired, cancelled, unavailable
    case dateConflict = "date_conflict", needsReview = "needs_review", sourceUnavailable = "source_unavailable"
    public var label: String {
        switch self {
        case .active: return "Scheduled for these dates"
        case .upcoming: return "Upcoming"
        case .ended: return "Dates passed"
        case .expired: return "Expired notice"
        case .cancelled: return "Cancelled"
        case .dateConflict: return "Dates conflict"
        case .needsReview: return "Needs review"
        case .sourceUnavailable: return "Source unavailable"
        case .unavailable: return "Notice unavailable"
        }
    }
}
