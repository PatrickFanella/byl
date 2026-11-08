import Foundation
import CryptoKit
import Observation

public struct SuggestionIntake: Decodable, Sendable {
    public let enabled: Bool
}

public struct RouteSuggestionInput: Codable, Equatable, Sendable {
    public var topic: String
    public var area: String
    public var notes: String
    public var placeLeads: [String]
    public var eventLeads: [String]
    public var sourceLinks: [String]
    public var modes: [String]
    public var tripShape: String
    public var durationMinutes: Int?
    public var startPreference: String
    public var endPreference: String
    public var pilotID: String

    enum CodingKeys: String, CodingKey {
        case topic, area, notes, modes
        case placeLeads = "place_leads", eventLeads = "event_leads", sourceLinks = "source_links"
        case tripShape = "trip_shape", durationMinutes = "duration_minutes"
        case startPreference = "start_preference", endPreference = "end_preference", pilotID = "pilot_id"
    }

    public init(topic: String, area: String, notes: String = "", placeLeads: [String] = [], eventLeads: [String] = [], sourceLinks: [String] = []) {
        self.topic = topic
        self.area = area
        self.notes = notes
        self.placeLeads = placeLeads
        self.eventLeads = eventLeads
        self.sourceLinks = sourceLinks
        self.modes = ["walk", "bike"]
        self.tripShape = "flexible"
        self.durationMinutes = nil
        self.startPreference = ""
        self.endPreference = ""
        self.pilotID = ""
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        topic = try values.decode(String.self, forKey: .topic)
        area = try values.decode(String.self, forKey: .area)
        notes = try values.decodeIfPresent(String.self, forKey: .notes) ?? ""
        placeLeads = try values.decodeIfPresent([String].self, forKey: .placeLeads) ?? []
        eventLeads = try values.decodeIfPresent([String].self, forKey: .eventLeads) ?? []
        sourceLinks = try values.decodeIfPresent([String].self, forKey: .sourceLinks) ?? []
        modes = try values.decodeIfPresent([String].self, forKey: .modes) ?? ["walk", "bike"]
        tripShape = try values.decodeIfPresent(String.self, forKey: .tripShape) ?? "flexible"
        durationMinutes = try values.decodeIfPresent(Int.self, forKey: .durationMinutes)
        startPreference = try values.decodeIfPresent(String.self, forKey: .startPreference) ?? ""
        endPreference = try values.decodeIfPresent(String.self, forKey: .endPreference) ?? ""
        pilotID = try values.decodeIfPresent(String.self, forKey: .pilotID) ?? ""
    }
}

public struct RouteSuggestion: Decodable, Identifiable, Equatable, Sendable {
    public let id: String
    public let ownerAccountID: String
    public let submitted: RouteSuggestionInput
    public let status: String
    public let reason: String?
    public let version: Int
    public let createdAt: String
    public let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id, submitted, status, reason, version
        case ownerAccountID = "owner_account_id", createdAt = "created_at", updatedAt = "updated_at"
    }
}

public struct SuggestionStatus: Equatable, Sendable {
    public let label: String
    public let detail: String

    public init(_ status: String) {
        switch status {
        case "draft":
            (label, detail) = ("Received", "Saved privately for curator triage. Research has not started yet.")
        case "processing":
            (label, detail) = ("Research in progress", "Research is being prepared. Check back for an update.")
        case "needs_research":
            (label, detail) = ("More research needed", "Some questions or sources still need investigation.")
        case "review":
            (label, detail) = ("Curator review", "The proposal is waiting for a curator decision.")
        case "accepted":
            (label, detail) = ("Accepted", "Only content approved through curator publication appears as a public tour.")
        case "declined":
            (label, detail) = ("Declined", "This suggestion is not being taken forward. Read the decision below.")
        default:
            (label, detail) = ("Status unavailable", "Refresh to check the current submission status.")
        }
    }
}

public struct SuggestionFields: Codable, Equatable, Sendable {
    public var topic = ""
    public var area = ""
    public var notes = ""
    public var places = ""
    public var events = ""
    public var sources = ""

    public init() {}

    public var input: RouteSuggestionInput {
        RouteSuggestionInput(topic: topic.trimmingCharacters(in: .whitespacesAndNewlines), area: area.trimmingCharacters(in: .whitespacesAndNewlines), notes: notes, placeLeads: lines(places), eventLeads: lines(events), sourceLinks: lines(sources))
    }

    private func lines(_ text: String) -> [String] {
        text.components(separatedBy: .newlines).map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
    }
}

public struct SuggestionAttempt: Codable, Equatable, Sendable {
    public let key: String
    public let input: RouteSuggestionInput
}

private struct StoredSuggestionDraft: Codable {
    let fields: SuggestionFields
    let pending: SuggestionAttempt?
}

/// A screen owns one account, credential, and server. Only its unfinished input and exact
/// retry identity are stored locally; lifecycle and decisions always come from owner reads.
@MainActor
@Observable
public final class RouteSuggestionsModel {
    public private(set) var fields = SuggestionFields()
    public private(set) var pending: SuggestionAttempt?
    public private(set) var busy = false
    public private(set) var message: String?
    public private(set) var fieldErrors: [String: String] = [:]
    public private(set) var needsSignIn = false
    public private(set) var intake: Bool?
    public private(set) var intakeLoading = false
    public private(set) var intakeError: String?
    public private(set) var items: [RouteSuggestion] = []
    public private(set) var listLoading = false
    public private(set) var listError: String?
    public private(set) var offset = 0
    public private(set) var hasMore = false
    public private(set) var selectedID: String?
    public private(set) var selected: RouteSuggestion?
    public private(set) var detailLoading = false
    public private(set) var detailError: String?

    private let api: Endpoints
    private let cache: DiskCache
    private let accountID: String
    private let isCurrent: () -> Bool
    private let key: String
    private var active = true
    private var unreadableDraft = false
    private var pageRequest = 0
    private var detailRequest = 0
    private var intakeRequest = 0
    private var current: Bool { active && isCurrent() }

    public init(api: Endpoints, cache: DiskCache, accountID: String, isCurrent: @escaping () -> Bool) {
        precondition(!accountID.isEmpty)
        self.api = api
        self.cache = cache
        self.accountID = accountID
        self.isCurrent = isCurrent
        let origin = api.client.baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let hash = SHA256.hash(data: Data(origin.utf8)).map { String(format: "%02x", $0) }.joined()
        self.key = "suggestion.v1.\(hash).\(accountID)"
        if isCurrent() {
            if let draft = cache.load(StoredSuggestionDraft.self, key: key) {
                fields = draft.fields
                pending = draft.pending
            } else if cache.has(key) {
                unreadableDraft = true
                message = "The saved draft could not be read. Check your submissions before discarding it and starting another."
            }
        }
    }

    public var canEdit: Bool { current && !busy && pending == nil && !unreadableDraft }
    public var canSubmit: Bool {
        current && !busy && !unreadableDraft && !needsSignIn && intake == true &&
            (pending != nil || (!fields.input.topic.isEmpty && !fields.input.area.isEmpty))
    }

    public func edit(_ fields: SuggestionFields) {
        guard canEdit else { return }
        self.fields = fields
        fieldErrors = [:]
        message = nil
        do { try persist() }
        catch { message = "Could not save this draft on the device. Keep this screen open and try again." }
    }

    public func discardLocalDraft() {
        guard current && !busy else { return }
        do {
            try cache.removeChecked(key)
            fields = SuggestionFields()
            pending = nil
            unreadableDraft = false
            fieldErrors = [:]
            message = nil
        } catch { message = "Could not remove the saved draft. Try again." }
    }

    private func persist(pending attempt: SuggestionAttempt? = nil) throws {
        try cache.savePrivate(StoredSuggestionDraft(fields: fields, pending: attempt ?? pending), key: key)
    }

    public func loadIntake() async {
        guard current else { return }
        intakeRequest += 1
        let request = intakeRequest
        intake = nil
        intakeLoading = true
        intakeError = nil
        defer { if current && request == intakeRequest { intakeLoading = false } }
        do {
            let result = try await api.suggestionIntake()
            guard current && request == intakeRequest else { return }
            intake = result.enabled
        } catch {
            guard current && request == intakeRequest else { return }
            intakeError = "Could not check whether new suggestions are open."
        }
    }

    public func loadPage(offset: Int = 0) async {
        guard current else { return }
        pageRequest += 1
        let request = pageRequest
        self.offset = max(0, offset)
        items = []
        hasMore = false
        listLoading = true
        listError = nil
        defer { if current && request == pageRequest { listLoading = false } }
        do {
            let result = try await api.suggestions(offset: self.offset)
            guard current && request == pageRequest else { return }
            guard result.allSatisfy({ $0.ownerAccountID == accountID }) else {
                listError = "Private submissions are unavailable for this account."
                return
            }
            items = Array(result.prefix(20))
            hasMore = result.count > 20
        } catch {
            guard current && request == pageRequest else { return }
            listError = "Could not load your private submissions. Try again when connected."
            if (error as? APIError)?.isUnauthorized == true { needsSignIn = true }
        }
    }

    public func loadDetail(_ id: String) async {
        guard current else { return }
        detailRequest += 1
        let request = detailRequest
        selectedID = id
        selected = nil
        detailLoading = true
        detailError = nil
        message = nil
        defer { if current && request == detailRequest { detailLoading = false } }
        do {
            let result = try await api.suggestion(id)
            guard current && request == detailRequest else { return }
            if result.ownerAccountID == accountID && result.id == id {
                selected = result
            } else { detailError = "This suggestion is not available for this account." }
        } catch {
            guard current && request == detailRequest else { return }
            let status = (error as? APIError)?.status
            detailError = status == 403 || status == 404 ? "This suggestion is not available for this account." : "Could not load this suggestion. Try again when connected."
            if status == 401 { needsSignIn = true }
        }
    }

    public func showList() {
        guard current else { return }
        detailRequest += 1
        selectedID = nil
        selected = nil
        detailLoading = false
        detailError = nil
        message = nil
    }

    @discardableResult
    public func submit() async -> RouteSuggestion? {
        guard canSubmit else { return nil }
        busy = true
        message = nil
        fieldErrors = [:]
        defer { if current { busy = false } }
        let attempt = pending ?? SuggestionAttempt(key: UUID().uuidString.lowercased(), input: fields.input)
        do { try persist(pending: attempt) }
        catch {
            message = "Could not save the draft on this device. No submission was sent."
            return nil
        }
        pending = attempt
        guard current else { return nil }
        do {
            let result = try await api.submitSuggestion(attempt.input, key: attempt.key)
            guard current else { return nil }
            guard result.ownerAccountID == accountID else {
                throw APIError(status: 0, code: "unconfirmed", message: "Submission not confirmed.")
            }
            // If local cleanup fails, retain the attempt; a retry still resolves to this row.
            do {
                try cache.removeChecked(key)
                fields = SuggestionFields()
                pending = nil
            } catch {
                message = "Saved on the server. The local draft could not be cleared; retrying uses the same suggestion."
            }
            selectedID = result.id
            selected = result
            detailLoading = false
            detailError = nil
            if message == nil { message = "Suggestion saved privately. Curators review it before any publication." }
            return result
        } catch {
            guard current else { return nil }
            let apiError = error as? APIError
            if apiError?.code == "intake_disabled" {
                intake = false
                message = "New suggestions are closed. Your draft is kept on this device for retrying later."
            } else if apiError?.status == 401 {
                needsSignIn = true
                message = "Sign in again to continue. Your original suggestion is kept on this device."
            } else if let apiError, [400, 404, 422, 429].contains(apiError.status) {
                pending = nil
                fieldErrors = apiError.fields
                message = fieldErrors.isEmpty ? apiError.message : "Check the marked fields and submit again."
                do { try persist() }
                catch { message = "Could not update the saved draft. Check your submissions before trying again." }
            } else {
                message = "Could not confirm the submission. Retry sends the same suggestion; check your submissions before starting another."
            }
            return nil
        }
    }

    /// Leaving or changing credentials clears screen memory without deleting the local draft.
    public func close() {
        active = false
        fields = SuggestionFields()
        pending = nil
        items = []
        selected = nil
        selectedID = nil
        fieldErrors = [:]
        message = nil
    }
}
