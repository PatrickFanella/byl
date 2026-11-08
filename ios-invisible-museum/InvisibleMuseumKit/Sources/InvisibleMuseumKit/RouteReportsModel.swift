import Foundation
import CryptoKit
import Observation

public struct ReportAttempt: Codable, Equatable, Sendable {
    public let key: String
    public let input: RouteReportInput
}
private struct StoredReportDraft: Codable {
    let fields: ReportFields
    let pending: ReportAttempt?
}

/// A controller is bound to its opening account, credentials and server. Drafts are
/// private and revision/mode scoped; server decisions are always read afresh.
@MainActor
@Observable
public final class RouteReportsModel {
    public let context: RouteReportContext?
    public private(set) var fields = ReportFields()
    public private(set) var pending: ReportAttempt?
    public private(set) var busy = false
    public private(set) var message: String?
    public private(set) var fieldErrors: [String: String] = [:]
    public private(set) var needsSignIn = false
    public private(set) var items: [RouteReport] = []
    public private(set) var listLoading = false
    public private(set) var listError: String?
    public private(set) var offset = 0
    public private(set) var hasMore = false
    public private(set) var selectedID: String?
    public private(set) var selected: RouteReport?
    public private(set) var outcome: RouteCorrectionOutcome?
    public private(set) var outcomeError: String?
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
    private var current: Bool { active && isCurrent() }

    public init(api: Endpoints, cache: DiskCache, accountID: String, context: RouteReportContext? = nil, initialTarget: String? = nil, isCurrent: @escaping () -> Bool) {
        precondition(!accountID.isEmpty)
        self.api = api; self.cache = cache; self.accountID = accountID
        self.context = context; self.isCurrent = isCurrent
        let origin = api.client.baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let hash = SHA256.hash(data: Data(origin.utf8)).map { String(format: "%02x", $0) }.joined()
        key = "route-report.v1.\(hash).\(accountID).\(context?.revision.id ?? "inbox").\(context?.mode ?? "")"
        if let context, isCurrent() {
            fields.target = context.targets.first(where: { $0.id == initialTarget })?.id ?? context.targets.first?.id ?? ""
            if let draft = cache.load(StoredReportDraft.self, key: key) {
                fields = draft.fields; pending = draft.pending
                if let attempt = pending, attempt.input.tourID != context.revision.tourID || attempt.input.revisionID != context.revision.id || attempt.input.mode != context.mode || UUID(uuidString: attempt.key) == nil {
                    unreadableDraft = true
                }
            } else if cache.has(key) { unreadableDraft = true }
            if unreadableDraft { message = "The saved report could not be read. Check Your reports before discarding the local draft." }
        }
    }
    public var canEdit: Bool { current && context != nil && !busy && pending == nil && !unreadableDraft && selected == nil }
    public var canSubmit: Bool { current && context?.unavailableReason == nil && context != nil && !busy && !needsSignIn && !unreadableDraft && selected == nil }

    private func persist(_ fields: ReportFields, pending: ReportAttempt?) throws {
        try cache.savePrivate(StoredReportDraft(fields: fields, pending: pending), key: key)
    }
    public func edit(_ fields: ReportFields) {
        guard canEdit else { return }
        self.fields = fields; fieldErrors = [:]; message = nil
        do { try persist(fields, pending: nil) }
        catch { message = "Could not save the draft on this device. Keep this screen open and try again." }
    }
    public func discardLocalDraft() {
        guard current && !busy && context != nil else { return }
        do {
            try cache.removeChecked(key)
            fields = ReportFields(); fields.target = context?.targets.first?.id ?? ""
            pending = nil; unreadableDraft = false; fieldErrors = [:]; message = nil
        } catch { message = "Could not clear the saved report. Your input and retry identity are kept." }
    }
    @discardableResult
    public func submit() async -> RouteReport? {
        guard canSubmit, let context else { return nil }
        busy = true; message = nil; fieldErrors = [:]
        defer { if current { busy = false } }
        let attempt: ReportAttempt
        do {
            attempt = try pending ?? ReportAttempt(key: UUID().uuidString.lowercased(), input: fields.input(context: context))
        } catch { message = error.localizedDescription; return nil }
        pending = attempt
        do { try persist(fields, pending: attempt) }
        catch { message = "Could not save the report retry on this device. No report was sent."; return nil }
        guard current else { return nil }
        do {
            let saved = try await api.submitReport(attempt.input, key: attempt.key)
            guard current else { return nil }
            guard saved.ownerAccountID == accountID, saved.submitted == attempt.input else {
                throw APIError(status: 0, code: "unconfirmed", message: "Report identity was not confirmed.")
            }
            selectedID = saved.id; selected = saved
            do { try cache.removeChecked(key); pending = nil }
            catch { message = "Report saved privately. Its local retry could not be cleared; it still identifies this report." }
            message = message ?? "Report saved privately. Curators review it before a route changes."
            return saved
        } catch {
            guard current else { return nil }
            let apiError = error as? APIError
            if apiError?.status == 401 {
                needsSignIn = true; message = "Sign in again. The original report and retry identity are kept on this device."
            } else if let apiError, [400, 403, 404, 422, 429].contains(apiError.status) {
                do {
                    try persist(fields, pending: nil)
                    pending = nil; fieldErrors = apiError.fields
                    message = fieldErrors.isEmpty ? apiError.message : "Check the report fields and submit again."
                } catch { message = "The report was rejected, but the saved retry could not be updated. Your input and retry identity are kept." }
            } else { message = "Could not confirm the report. Retry sends the same observation; check Your reports before discarding it." }
            return nil
        }
    }
    public func loadPage(offset: Int = 0) async {
        guard current else { return }
        pageRequest += 1; let request = pageRequest
        self.offset = max(0, offset); items = []; hasMore = false; listLoading = true; listError = nil
        defer { if current && request == pageRequest { listLoading = false } }
        do {
            let saved = try await api.reports(offset: self.offset)
            guard current && request == pageRequest else { return }
            guard saved.allSatisfy({ $0.ownerAccountID == accountID }) else {
                listError = "Private reports are unavailable for this account."; return
            }
            items = Array(saved.prefix(20)); hasMore = saved.count > 20
        } catch {
            guard current && request == pageRequest else { return }
            listError = "Could not read your private reports. Try again when connected."
            if (error as? APIError)?.status == 401 { needsSignIn = true }
        }
    }
    public func loadDetail(_ id: String) async {
        guard current else { return }
        detailRequest += 1; let request = detailRequest
        selectedID = id; selected = nil; outcome = nil; outcomeError = nil; detailLoading = true; detailError = nil
        defer { if current && request == detailRequest { detailLoading = false } }
        do {
            let saved = try await api.report(id)
            guard current && request == detailRequest else { return }
            guard saved.ownerAccountID == accountID && saved.id == id else {
                detailError = "This report is not available for this account."; return
            }
            selected = saved
            do {
                let correction = try await api.reportCorrection(id)
                guard current && request == detailRequest else { return }
                guard correction.previousRevisionID == saved.submitted.revisionID, !correction.revisionID.isEmpty else {
                    outcomeError = "The approved revision could not be confirmed. Refresh to check again."; return
                }
                outcome = correction
            } catch {
                guard current && request == detailRequest else { return }
                if (error as? APIError)?.status != 404 { outcomeError = "The report is available, but its approved-update status could not be read." }
            }
        } catch {
            guard current && request == detailRequest else { return }
            let status = (error as? APIError)?.status
            detailError = [403, 404].contains(status ?? 0) ? "This report is not available for this account." : "Could not load this report. Try again when connected."
            if status == 401 { needsSignIn = true }
        }
    }
    public func showList() {
        guard current else { return }
        detailRequest += 1; selectedID = nil; selected = nil; outcome = nil; detailLoading = false; detailError = nil; outcomeError = nil; message = nil
    }
    public func close() {
        active = false; fields = ReportFields(); pending = nil; selected = nil; selectedID = nil
        items = []; outcome = nil; fieldErrors = [:]; message = nil
    }
}
