import Foundation

public struct ContributionGate: Decodable {
    public let enabled, calibrated: Bool
    public let detectorVersion: String
    enum CodingKeys: String, CodingKey { case enabled, calibrated; case detectorVersion = "detector_version" }
}

@MainActor
public protocol ContributionAPI {
    func gate() async throws -> ContributionGate
    func grant(_ choice: ContributionChoice, key: String) async throws -> ContributionConsent
    func submit(consent: ContributionConsent, summary: DeviationSummary, key: String) async throws
    func withdraw(_ consent: ContributionConsent, origin: URL) async throws
}

/// Captures one token and API origin. Optional failures never call app auth hooks.
@MainActor
public final class ContributionHTTPAPI: ContributionAPI {
    private let client: APIClient
    private let publicClient: APIClient
    private let session: URLSession
    public init(origin: URL, token: String?, session: URLSession = .shared) {
        self.session = session
        client = APIClient(baseURL: origin, session: session, tokenProvider: { token })
        publicClient = APIClient(baseURL: origin, session: session)
    }
    public func gate() async throws -> ContributionGate { try await publicClient.get("/api/v1/route-contribution-intake") }
    public func grant(_ choice: ContributionChoice, key: String) async throws -> ContributionConsent {
        let consent: ContributionConsent = try await client.post("/api/v1/me/route-contributions/consent", body: choice, headers: ["Idempotency-Key": key])
        guard ContributionStore.validConsent(consent, choice: choice) else { throw ContributionFailure.invalidContract }
        return consent
    }
    private struct Submission: Encodable {
        let consentID: String
        let summary: DeviationSummary
        enum CodingKeys: String, CodingKey { case consentID = "consent_id", summary }
    }
    private struct Receipt: Decodable {
        let id, expiresOn: String
        enum CodingKeys: String, CodingKey { case id; case expiresOn = "expires_on" }
    }
    public func submit(consent: ContributionConsent, summary: DeviationSummary, key: String) async throws {
        guard ContributionStore.validSummary(summary) else { throw ContributionFailure.invalidContract }
        let receipt: Receipt = try await client.post("/api/v1/me/route-contributions", body: Submission(consentID: consent.id, summary: summary), headers: ["Idempotency-Key": key])
        guard UUID(uuidString: receipt.id) != nil, ContributionDay.parse(receipt.expiresOn) != nil, receipt.expiresOn <= consent.expiresOn else { throw ContributionFailure.invalidContract }
    }
    private struct Withdrawal: Encodable {
        let consentID, deletionHandle: String
        enum CodingKeys: String, CodingKey { case consentID = "consent_id", deletionHandle = "deletion_handle" }
    }
    public func withdraw(_ consent: ContributionConsent, origin: URL) async throws {
        let anonymous = APIClient(baseURL: origin, session: session)
        // No bearer, including for handles retained from another original server.
        try await anonymous.send("POST", "/api/v1/route-contributions/withdraw", body: Withdrawal(consentID: consent.id, deletionHandle: consent.deletionHandle))
    }
}
