import Foundation
import Observation

/// Foreground-only contribution state. Navigation has no dependency on its IO.
@MainActor
@Observable
public final class TripContributions {
    public private(set) var message = "Route improvement is off. Navigation does not contribute location data."
    public private(set) var admitted = false
    public private(set) var optedIn = false
    public private(set) var paused = true
    public private(set) var busy = false
    public private(set) var pending = 0
    public private(set) var retainedCount = 0
    public private(set) var deletionPending = 0
    public let eligible: Bool
    public var identityAvailable: Bool { owner != nil && sameIdentity() }
    private let store: ContributionStore
    private let api: any ContributionAPI
    private let owner: String?
    private let origin: String
    private let revision: TourRevision?
    private let mode: String
    private let contexts: [DeviationContext]
    private let sameIdentity: () -> Bool
    private let now: () -> Date
    private let key: () -> String
    private let recordID: String?
    private var detectors: [DeviationDetector] = []
    private var foreground = false
    private var ended = false
    private var stopped = false
    private var uploadsAuthorized = false
    private var storageBroken = false
    private var manualSuspended = false
    private var permissionSuspended = false
    private var gateChecked: Date?
    private var confirmedConsent: (key: String, at: Date)?
    private var sampleDay = ""
    private var samplingSince = Double.infinity
    private var flushing: Task<Void, Never>?
    private var actions = 0
    private var gateRequest = 0
    private var choiceEpoch = 0
    private var acceptedGeneration: Int

    public init(store: ContributionStore, api: any ContributionAPI, owner: String?, origin: URL, revision: TourRevision?, mode: String, sameIdentity: @escaping () -> Bool, now: @escaping () -> Date = Date.init, key: @escaping () -> String = { UUID().uuidString }) {
        let originString = origin.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        self.store = store; acceptedGeneration = store.choiceGeneration; self.api = api; self.owner = owner; self.origin = originString
        self.revision = revision; self.mode = mode; self.sameIdentity = sameIdentity; self.now = now; self.key = key
        contexts = contributionContexts(revision: revision, mode: mode); eligible = !contexts.isEmpty
        recordID = owner.flatMap { owner in revision.map { [originString, owner, $0.tourID, $0.id, mode].joined(separator: "|") } }
        do { try store.prune(); try restore() } catch { storageFailed() }
        // Every newly attached trip requires an explicit choice or Resume. A
        // restart cannot silently undo an opt-out that failed to reach disk.
    }

    private var freshGate: Bool {
        guard admitted, let checked = gateChecked else { return false }
        return (0...60).contains(now().timeIntervalSince(checked))
    }
    private var mayResume: Bool { foreground && !ended && !stopped && acceptedGeneration == store.choiceGeneration && !paused && !manualSuspended && !permissionSuspended && freshGate && sameIdentity() && optedIn }
    private func restore() throws {
        let records = try store.read()
        pending = records.reduce(0) { $0 + $1.queue.count }; retainedCount = records.count
        deletionPending = records.filter(\.withdrawing).count
        let current = records.first { $0.id == recordID }
        optedIn = !stopped && acceptedGeneration == store.choiceGeneration && sameIdentity() && current?.enabled == true && current?.withdrawing == false && current?.consent != nil && (current?.consent?.expiresOn ?? "") > ContributionDay.string(now())
    }
    private func storageFailed() {
        storageBroken = true; admitted = false; optedIn = false; stopped = true; confirmedConsent = nil
        detectors = []; samplingSince = .infinity
        message = "Contribution storage is unavailable. Collection and uploads stopped; navigation still works. Retry storage, then make an explicit choice."
    }
    private func pauseEpisode() {
        for detector in detectors { _ = detector.push(.pause) }
        samplingSince = .infinity
    }
    private func resumeSampling() {
        guard mayResume, let confirmed = confirmedConsent, (0...60).contains(now().timeIntervalSince(confirmed.at)) else { return }
        if detectors.isEmpty { detectors = contexts.map(DeviationDetector.init(context:)) }
        for detector in detectors { _ = detector.push(.resume) }
        samplingSince = now().timeIntervalSince1970 * 1000; sampleDay = ContributionDay.string(now())
        message = "Route improvement is on for this revision and mode. Only qualified day-level counts can be sent."
    }
    public func setForeground(_ value: Bool) {
        guard value != foreground else { return }
        foreground = value
        if value { confirmedConsent = nil; samplingSince = .infinity }
        else { pauseEpisode() }
    }
    public func identityChanged() {
        guard !sameIdentity() else { return }
        choiceEpoch += 1
        stopped = true; uploadsAuthorized = false; optedIn = false; confirmedConsent = nil; detectors = []; samplingSince = .infinity
        message = "Reopen this trip after changing accounts or servers before choosing to contribute. Saved deletion details remain available in Profile."
    }
    public func lifecycle(_ event: DeviationDetector.Input) {
        switch event {
        case .fix: return
        case .permissionLost: permissionSuspended = true
        case .manualArrival: manualSuspended = true
        case .resume: manualSuspended = false
        case .end: ended = true; uploadsAuthorized = false; choiceEpoch += 1
        case .pause: manualSuspended = true
        }
        for detector in detectors { _ = detector.push(event) }
        if case .resume = event { resumeSampling() }
        else { samplingSince = .infinity }
        if ended { detectors = []; confirmedConsent = nil }
    }
    public func setPaused(_ value: Bool) async {
        if value { choiceEpoch += 1; uploadsAuthorized = false; paused = true; pauseEpisode(); return }
        guard !ended, !storageBroken, !stopped, acceptedGeneration == store.choiceGeneration, sameIdentity() else { return }
        uploadsAuthorized = true
        await refresh()
        guard optedIn, freshGate, !stopped else { return }
        paused = false; resumeSampling()
    }

    public func refresh() async {
        gateRequest += 1
        let request = gateRequest
        do {
            try store.prune(); storageBroken = false; try restore()
            let wasFresh = freshGate
            let gate = try await api.gate()
            guard request == gateRequest else { return }
            admitted = gate.enabled && gate.detectorVersion == DeviationDetector.version && !gate.calibrated
            gateChecked = now()
            if !admitted {
                confirmedConsent = nil; pauseEpisode()
                message = "Route improvement collection is unavailable. Navigation still works."
            }
            await flush()
            if admitted && (!wasFresh || samplingSince == .infinity) { resumeSampling() }
        } catch {
            guard request == gateRequest else { return }
            if error is ContributionFailure { storageFailed(); return }
            admitted = false; confirmedConsent = nil; pauseEpisode()
            message = "Collection could not be checked. Observation stopped; saved summaries wait for a retry."
        }
    }

    public func optIn() async {
        guard !busy, !ended, eligible, let revision, let owner, let recordID, foreground, sameIdentity() else { return }
        actions += 1; busy = true
        let epoch = choiceEpoch
        let generation = store.choiceGeneration
        defer { actions -= 1; busy = actions > 0 }
        do {
            try store.prune(); storageBroken = false
            await refresh()
            guard epoch == choiceEpoch, generation == store.choiceGeneration, !ended, freshGate, sameIdentity(), !storageBroken else { return }
            var records = try store.read()
            if let i = records.firstIndex(where: { $0.id == recordID }) {
                guard !records[i].withdrawing else { message = "Finish the pending deletion before making another choice for this revision."; return }
                records[i].enabled = true
            } else {
                records.append(ContributionRecord(origin: origin, owner: owner, choice: ContributionChoice(tourID: revision.tourID, revisionID: revision.id, mode: mode, requestedOn: ContributionDay.string(now())), key: key(), enabled: true, withdrawing: false, consent: nil, queue: []))
            }
            // The original choice and retry key must be durable before POST.
            try store.write(records); acceptedGeneration = generation; stopped = false; uploadsAuthorized = true; try restore()
            await flush(); try restore()
            guard epoch == choiceEpoch, generation == store.choiceGeneration, !ended, optedIn, freshGate, !stopped, sameIdentity() else { return }
            paused = false; resumeSampling()
        } catch { storageFailed() }
    }

    public func observe(_ fix: DeviationFix, authorizationGranted: Bool) {
        guard acceptedGeneration == store.choiceGeneration else {
            stopped = true; uploadsAuthorized = false; optedIn = false; paused = true; confirmedConsent = nil; detectors = []; samplingSince = .infinity
            return
        }
        guard authorizationGranted else { lifecycle(.permissionLost); return }
        if admitted && !freshGate {
            admitted = false; confirmedConsent = nil; pauseEpisode()
            message = "The collection check expired. Observation stopped; retry when online. Navigation still works."
            return
        }
        let milliseconds = now().timeIntervalSince1970 * 1000
        guard fix.timestampMs.isFinite, fix.timestampMs <= milliseconds + 1000, milliseconds - fix.timestampMs <= 30_000 else { pauseEpisode(); resumeSampling(); return }
        if permissionSuspended { permissionSuspended = false; resumeSampling() }
        guard mayResume, let confirmation = confirmedConsent, (0...60).contains(now().timeIntervalSince(confirmation.at)) else { pauseEpisode(); return }
        guard fix.timestampMs >= samplingSince else { return }
        do {
            guard let record = try store.read().first(where: { $0.id == recordID }), record.enabled, !record.withdrawing,
                  record.key == confirmation.key, let consent = record.consent, consent.expiresOn > ContributionDay.string(now()) else {
                optedIn = false; detectors = []; confirmedConsent = nil; return
            }
            if sampleDay != ContributionDay.string(now()) { detectors = []; resumeSampling() }
            for detector in detectors {
                for summary in detector.push(.fix(fix)) {
                    guard ContributionStore.validSummary(summary) else { storageFailed(); return }
                    let current = try store.read().first { $0.id == record.id && $0.key == record.key }
                    guard let current, current.enabled, !current.withdrawing else { pauseEpisode(); return }
                    let duplicate = current.queue.contains { $0.summary.segmentID == summary.segmentID && $0.summary.direction == summary.direction && $0.summary.observationPeriod == summary.observationPeriod }
                    if duplicate { continue }
                    guard current.queue.count < 32 else { paused = true; pauseEpisode(); message = "Saved summary limit reached. Retry uploads or erase saved contributions before resuming."; return }
                    let pending = PendingDeviation(key: key(), summary: summary)
                    try store.update(current.id, key: current.key) { $0.queue.append(pending) }
                }
            }
            try restore()
        } catch { storageFailed() }
    }

    public func eraseAll() async {
        actions += 1; busy = true
        defer { actions -= 1; busy = actions > 0 }
        stopped = true; uploadsAuthorized = false; optedIn = false; paused = true; confirmedConsent = nil
        choiceEpoch += 1
        detectors = []; samplingSince = .infinity
        do {
            // Drop every local pending summary before sending any deletion.
            try store.stopAll(); try restore()
            if let work = flushing { await work.value }
            await flush()
        } catch { storageFailed() }
    }
    public func flush() async {
        if let work = flushing { await work.value; return }
        let work = Task { [weak self] in
            guard let self else { return }
            await self.upload()
            self.flushing = nil
        }
        flushing = work; await work.value
    }

    private func upload() async {
        do {
            try store.prune()
            for initial in try store.read() {
                guard var record = try store.read().first(where: { $0.id == initial.id && $0.key == initial.key }) else { continue }
                var freshlyConfirmed = false
                if record.consent == nil {
                    guard record.enabled || record.withdrawing, record.withdrawing || acceptedGeneration == store.choiceGeneration, foreground, freshGate, sameIdentity(), record.owner == owner, record.origin == origin else { continue }
                    let consent = try await api.grant(record.choice, key: record.key)
                    guard ContributionStore.validConsent(consent, choice: record.choice) else { throw ContributionFailure.invalidContract }
                    // Keep this capability even if opt-out/account loss happened
                    // during the request; it is needed for anonymous erasure.
                    try store.update(record.id, key: record.key) { $0.consent = consent }
                    freshlyConfirmed = true
                }
                guard let latest = try store.read().first(where: { $0.id == record.id && $0.key == record.key }), let consent = latest.consent else { continue }
                record = latest
                if record.withdrawing {
                    guard let original = URL(string: record.origin) else { throw ContributionFailure.invalidContract }
                    // Only a successful withdrawal confirms removal. A 404 is
                    // kept pending instead of assuming a capability was valid.
                    try await api.withdraw(consent, origin: original)
                    try store.write(store.read().filter { $0.id != record.id || $0.key != record.key })
                    continue
                }
                guard !stopped, uploadsAuthorized, acceptedGeneration == store.choiceGeneration, foreground, record.enabled, freshGate, sameIdentity(), record.owner == owner, record.origin == origin else { continue }
                if !freshlyConfirmed {
                    let checked = try await api.grant(record.choice, key: record.key)
                    guard checked == consent else { throw ContributionFailure.invalidContract }
                }
                guard let current = try store.read().first(where: { $0.id == record.id && $0.key == record.key }), current.enabled, !current.withdrawing, sameIdentity(), foreground, !stopped, uploadsAuthorized, acceptedGeneration == store.choiceGeneration, freshGate else { continue }
                if current.id == recordID { confirmedConsent = (current.key, now()) }
                for pending in current.queue {
                    try store.prune()
                    guard foreground, !stopped, uploadsAuthorized, acceptedGeneration == store.choiceGeneration, freshGate, sameIdentity(),
                          let before = try store.read().first(where: { $0.id == current.id && $0.key == current.key }), before.enabled, !before.withdrawing,
                          before.queue.contains(where: { $0.key == pending.key }), let receipt = before.consent else { break }
                    do { try await api.submit(consent: receipt, summary: pending.summary, key: pending.key) }
                    catch let error as APIError where error.status == 409 && error.code == "already_contributed" { /* Scoped daily duplicate is acknowledged. */ }
                    try store.update(before.id, key: before.key) { $0.queue.removeAll { $0.key == pending.key } }
                }
            }
            try restore()
            if deletionPending > 0 {
                message = "Deletion is pending. Unknown consent receipts need the original account and server. Collection is off; keep the saved deletion capabilities until confirmation or expiry."
            } else if stopped {
                message = retainedCount == 0
                    ? "Route improvement is off. No saved contribution work remains on this device."
                    : "Collection is off. Saved choices and deletion details remain available in Profile; reopen the trip before making another choice."
            } else if optedIn {
                message = paused ? "Route improvement is paused. Resume explicitly for this trip." : "Route improvement is on for this revision and mode. Only qualified day-level counts can be sent."
            } else if admitted { message = "Route improvement is off. Navigation does not contribute location data." }
        } catch {
            if case ContributionFailure.storageUnavailable = error { storageFailed(); return }
            admitted = false; confirmedConsent = nil; pauseEpisode()
            try? restore()
            message = deletionPending > 0
                ? "Deletion could not be confirmed. Local queues are cleared; keep the original deletion capabilities and retry. Server records retain their original expiry."
                : "Contribution work is pending. Collection stopped; retry with the original choice and keys, or erase saved contributions."
        }
    }
}
