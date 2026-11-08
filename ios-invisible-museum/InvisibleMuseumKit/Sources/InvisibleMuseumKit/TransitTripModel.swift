import Foundation
import Observation
import CryptoKit

/// A plan is retained only by explicit acceptance. Opening or refreshing never chooses a train.
@MainActor @Observable
public final class TransitTripModel {
    public let revision: TourRevision
    public private(set) var selected: ScheduledTransitPlan?
    public private(set) var proposed: ScheduledTransitPlan?
    public private(set) var guidanceBlocked = false
    public private(set) var enabled: Bool?
    public private(set) var busy = false
    public private(set) var message: String?
    private struct Retained: Codable, Equatable {
        let version: Int
        let plan: ScheduledTransitPlan
        let guidanceBlocked: Bool
    }
    private let api: Endpoints
    private let cache: DiskCache
    private let key: String
    private let current: () -> Bool
    private var generation = 0
    private var gateGeneration = 0
    private var closed = false
    private var storageBlocked = false
    private var admittedAt: Date?
    private let now: () -> Date

    public init(api: Endpoints, revision: TourRevision, cache: DiskCache, accountID: String?, isCurrent: @escaping () -> Bool = { true }, now: @escaping () -> Date = Date.init) {
        self.api = api; self.revision = revision; self.cache = cache; self.now = now
        let origin = api.client.baseURL, token = api.client.tokenProvider()
        self.current = { api.client.baseURL == origin && api.client.tokenProvider() == token && isCurrent() }
        let identity = [api.client.baseURL.absoluteString, accountID ?? "anonymous", revision.tourID, revision.id].joined(separator: "\n")
        key = "transit-plan.v1." + SHA256.hash(data: Data(identity.utf8)).map { String(format: "%02x", $0) }.joined()
        if let saved = cache.load(Retained.self, key: key), saved.version == 1, saved.plan.valid(for: revision) {
            selected = saved.plan; guidanceBlocked = saved.guidanceBlocked
        } else if cache.has(key) {
            storageBlocked = true
            message = "The saved transit choice could not be read. Remove it explicitly before choosing another plan."
        }
    }

    public func refreshGate() async {
        guard !closed, current() else { invalidate(); return }
        gateGeneration += 1; let request = gateGeneration
        do {
            let gate = try await api.itineraryCapabilities()
            guard !closed, current(), request == gateGeneration else { return }
            enabled = gate.scheduledTransitV1
            admittedAt = enabled == true ? now() : nil
            if enabled == false { message = "Scheduled transit is disabled. The retained trip is readable; directions are suspended." }
        } catch {
            guard !closed, current(), request == gateGeneration else { return }
            enabled = nil; admittedAt = nil
            message = "Could not confirm scheduled transit availability. Directions are suspended."
        }
    }

    public func request(departure: Date) async {
        guard !closed, current(), !busy, !storageBlocked else { return }
        let instant = now()
        guard departure > instant, departure.timeIntervalSince(instant) <= 7 * 86400 else {
            message = "Choose a future departure from the first connector story stop within seven days."; return
        }
        generation += 1; let request = generation
        busy = true; proposed = nil; message = nil
        defer { if request == generation { busy = false } }
        await refreshGate()
        guard !closed, current(), request == generation, enabled == true else { return }
        let formatter = ISO8601DateFormatter(); formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let departureText = formatter.string(from: departure)
        do {
            let plan = try await api.transitPlan(tour: revision.tourID, revision: revision.id, departure: departureText)
            guard !closed, current(), request == generation else { return }
            guard plan.valid(for: revision), let resultDate = transitDate(plan.requestedDepartureAt),
                  abs(resultDate.timeIntervalSince(departure)) < 0.01 else {
                message = "The timetable response did not match this revision and departure. Your retained choice is unchanged."; return
            }
            proposed = plan
            if plan.state == "needs_review" || plan.state == "unsupported" ||
                (plan.state != "scheduled" && selected.map { transitDate($0.requestedDepartureAt) == resultDate } == true) {
                suspendRetainedGuidance()
            }
        } catch {
            guard !closed, current(), request == generation else { return }
            if let error = error as? APIError, [403, 404].contains(error.status) { enabled = false; admittedAt = nil }
            message = "Could not plan this departure. Your retained choice is unchanged."
        }
    }

    public func accept() throws {
        guard !closed, current(), !busy, !storageBlocked, let proposed,
              proposed.valid(for: revision), proposed.unavailableReason(now: now()) == nil, gateIsCurrent else {
            throw APIError(status: 0, code: "transit_unavailable", message: "Refresh availability and review a usable future plan before accepting it.")
        }
        try persist(Retained(version: 1, plan: proposed, guidanceBlocked: false))
        selected = proposed; self.proposed = nil; guidanceBlocked = false; message = nil
    }

    public func remove() throws {
        guard !closed, current() else { return }
        try cache.removeChecked(key)
        generation += 1; busy = false; selected = nil; proposed = nil
        guidanceBlocked = false; storageBlocked = false; message = nil
    }

    public func invalidate() {
        closed = true; generation += 1; gateGeneration += 1
        enabled = nil; admittedAt = nil; proposed = nil; busy = false
    }

    private var gateIsCurrent: Bool {
        guard enabled == true, let admittedAt else { return false }
        return (0...60).contains(now().timeIntervalSince(admittedAt))
    }

    public func canGuide(at date: Date) -> Bool {
        !closed && current() && !guidanceBlocked && gateIsCurrent && selected?.unavailableReason(now: date) == nil && selected != nil
    }

    private func suspendRetainedGuidance() {
        guard let selected else { return }
        guidanceBlocked = true
        do { try persist(Retained(version: 1, plan: selected, guidanceBlocked: true)) }
        catch {
            // If blocking cannot be persisted, remove the record so a relaunch cannot resume it as usable.
            try? cache.removeChecked(key)
            message = "Guidance is suspended. The suspension could not be saved; check device storage before reopening."
        }
    }

    private func persist(_ value: Retained) throws {
        let data = try JSONEncoder().encode(value)
        guard data.count <= 256 * 1024 else {
            throw APIError(status: 0, code: "transit_storage", message: "The timetable choice is too large to retain.")
        }
        let previous = cache.load(Retained.self, key: key)
        do {
            try cache.savePrivate(value, key: key)
            guard cache.load(Retained.self, key: key) == value else {
                throw APIError(status: 0, code: "transit_storage", message: "The choice could not be read back from device storage.")
            }
        } catch {
            if let previous { try? cache.savePrivate(previous, key: key) } else { try? cache.removeChecked(key) }
            throw error
        }
    }

    /// Both endpoints are public references. No device location or account data enters the handoff URL.
    public func walkingURL(boarding: Bool, at date: Date) -> URL? {
        guard canGuide(at: date), let tour = revision.snapshot?.tour, let choice = tour.transit else { return nil }
        let target = boarding ? nil : tour.stops.first { $0.id == choice.intent.toStopID }?.place.arrivalPoint?.point
        let start = boarding ? tour.stops.first { $0.id == choice.intent.fromStopID }?.place.arrivalPoint?.point : nil
        guard boarding || (target?.latitude != nil && target?.longitude != nil) else { return nil }
        let destination = boarding ? "\(choice.board.latitude),\(choice.board.longitude)" : "\(target!.latitude!),\(target!.longitude!)"
        let origin = boarding ? start.flatMap { point in
            point.latitude.flatMap { lat in point.longitude.map { "\(lat),\($0)" } }
        } : "\(choice.alight.latitude),\(choice.alight.longitude)"
        var c = URLComponents(string: "https://maps.apple.com/")!
        c.queryItems = [URLQueryItem(name: "daddr", value: destination), URLQueryItem(name: "dirflg", value: "w")]
        if let origin { c.queryItems?.append(URLQueryItem(name: "saddr", value: origin)) }
        return c.url
    }
}
