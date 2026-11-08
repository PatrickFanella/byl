import Foundation
import Observation
import CoreLocation
import MapKit
import CryptoKit

/// Progress on one tour kept on the device, so a walk continues without signal and resumes later.
public struct LocalTourProgress: Codable, Equatable {
    public var tourID: String
    public var mode: String
    public var visitedPositions: [Int]
    public var startedAt: Date
    public var completedAt: Date?
    /// Last local change; compared with `syncedAt` to know whether the server is behind.
    public var updatedAt: Date = Date()
    /// When the server last accepted this progress; nil until a PUT succeeds.
    public var syncedAt: Date?
    public var resetPending: Bool?
    public var revisionID: String?
    public var visitedStopIDs: [String]?

    public var needsSync: Bool {
        if resetPending == true { return true }
        guard !visitedPositions.isEmpty else { return false }
        guard let syncedAt else { return true }
        return syncedAt < updatedAt
    }

    public static func key(tourID: String, mode: String, accountID: String? = nil) -> String { "progress.v2.\(accountID ?? "anonymous").\(tourID)" }
}

/// Which tour is being walked, kept on disk so a relaunch can resume it.
public struct ActiveTourRecord: Codable, Equatable {
    public var tourID: String
    public var mode: String
    public var accountID: String?
    public var revisionID: String?
    public var origin: String?
    public static let key = "active"
}

/// Drives a tour being walked or ridden: the route, the next stop, arrival, progress, and the
/// cache that lets all of it survive losing signal.
@MainActor
@Observable
public final class ActiveTourModel: Identifiable {
    nonisolated public let id: String

    public static func routeKey(tourID: String, mode: String) -> String { "route.\(tourID).\(mode)" }
    public static func tourKey(tourID: String) -> String { "tour.\(tourID)" }

    public let api: Endpoints
    public let tour: TourDetail
    public let mode: String
    public let cache: DiskCache
    public let isSignedIn: () -> Bool
    public let revision: TourRevision?

    public private(set) var route: TourRoute?
    public private(set) var routeState: LoadState = .idle
    public private(set) var routeFromCache = false
    public private(set) var routePending = false
    public private(set) var visited: Set<Int> = []
    public private(set) var nextStop: TourStop?
    /// The stop just reached, for the moment sheet; the view clears it when dismissed.
    public var arrivedStop: TourStop?
    public private(set) var distanceToNextM: Double?
    public private(set) var lastLocation: CLLocation?
    public private(set) var syncError: String?
    public var audioEnabled = false
    /// Called after every arrival (the audio guide hooks in here).
    public var onArrive: ((TourStop) -> Void)?
    public private(set) var availableUpdate: TourRevision?
    public private(set) var updateError: String?
    public private(set) var checkingUpdate = false
    public private(set) var revoked = false
    public private(set) var legacyResume = false

    private let progressKey: String
    private let accountID: String?
    private var progress: LocalTourProgress
    private var monitor: ArrivalMonitor?
    private var pendingSync: Task<Void, Never>?
    private let origin: String
    private let snapshotKey: String
    public var owningAccountID: String? { accountID }

    private static func scope(api: Endpoints, accountID: String?) -> String {
        let origin = api.client.baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let hash = SHA256.hash(data: Data(origin.utf8)).map { String(format: "%02x", $0) }.joined()
        return "\(hash).\(accountID ?? "anonymous")"
    }

    public static func revisionKey(api: Endpoints, accountID: String?, revisionID: String) -> String {
        "revision.v1.\(scope(api: api, accountID: accountID)).\(revisionID)"
    }

    /// Older cached trips remain readable, but new app starts use the checked revision initializer.
    public convenience init(api: Endpoints, tour: TourDetail, mode: String, cache: DiskCache, isSignedIn: @escaping () -> Bool, accountID: String? = nil) {
        self.init(api: api, tour: tour, revision: nil, mode: mode, cache: cache, isSignedIn: isSignedIn, accountID: accountID)
    }

    public convenience init(api: Endpoints, revision: TourRevision, mode: String, cache: DiskCache, isSignedIn: @escaping () -> Bool, accountID: String? = nil) throws {
        if let reason = revision.unavailableReason(mode: mode) {
            throw APIError(status: 0, code: "revision_unavailable", message: reason)
        }
        guard let snapshot = revision.snapshot else {
            throw APIError(status: 0, code: "revision_unavailable", message: "The reviewed itinerary is unavailable.")
        }
        let key = Self.revisionKey(api: api, accountID: accountID, revisionID: revision.id)
        try cache.savePrivate(revision, key: key)
        try cache.savePrivate(ActiveTourRecord(tourID: revision.tourID, mode: mode, accountID: accountID, revisionID: revision.id, origin: api.client.baseURL.absoluteString), key: ActiveTourRecord.key)
        self.init(api: api, tour: snapshot.tour, revision: revision, mode: mode, cache: cache, isSignedIn: isSignedIn, accountID: accountID)
    }

    private init(api: Endpoints, tour: TourDetail, revision: TourRevision?, mode: String, cache: DiskCache, isSignedIn: @escaping () -> Bool, accountID: String?) {
        self.api = api
        self.tour = tour
        self.mode = mode
        self.cache = cache
        self.isSignedIn = isSignedIn
        self.revision = revision
        self.origin = api.client.baseURL.absoluteString
        self.snapshotKey = Self.revisionKey(api: api, accountID: accountID, revisionID: revision?.id ?? "legacy")
        self.id = "\(tour.tour.id).\(revision?.id ?? "legacy").\(mode)"
        self.accountID = accountID
        let key = revision.map { "progress.v3.\(Self.scope(api: api, accountID: accountID)).\($0.id)" }
            ?? LocalTourProgress.key(tourID: tour.tour.id, mode: mode, accountID: accountID)
        self.progressKey = key
        progress = cache.load(LocalTourProgress.self, key: key)
            ?? LocalTourProgress(tourID: tour.tour.id, mode: mode, visitedPositions: [], startedAt: Date())
        if let revision, progress.tourID != tour.id || (progress.revisionID != nil && progress.revisionID != revision.id) {
            progress = LocalTourProgress(tourID: tour.id, mode: mode, visitedPositions: [], startedAt: Date())
        }
        if let revision {
            let ids = progress.revisionID == revision.id ? Set(progress.visitedStopIDs ?? []) : []
            visited = Set(tour.stops.filter { ids.contains($0.id) }.map(\.position))
            progress.revisionID = revision.id
            progress.visitedStopIDs = tour.stops.filter { visited.contains($0.position) }.map(\.id)
            progress.visitedPositions = visited.sorted()
            if visited.count != tour.stops.count { progress.completedAt = nil }
            route = revision.snapshot?.routes[mode]
            routeState = .loaded
            routeFromCache = true
        } else {
            visited = Set(progress.visitedPositions)
            cache.save(tour, key: Self.tourKey(tourID: tour.tour.id))
            cache.save(ActiveTourRecord(tourID: tour.tour.id, mode: mode, accountID: accountID), key: ActiveTourRecord.key)
        }
        advance()
    }

    /// Rebuilds the tour that was active when the app last ran, from the cached detail alone,
    /// so it comes back even without signal (Review Focus 2). Nil when no tour was active.
    public static func restore(api: Endpoints, cache: DiskCache, isSignedIn: @escaping () -> Bool, accountID: String? = nil) -> ActiveTourModel? {
        guard let record = cache.load(ActiveTourRecord.self, key: ActiveTourRecord.key), record.accountID == accountID else { return nil }
        if let revisionID = record.revisionID {
            guard record.origin == api.client.baseURL.absoluteString,
                  let revision = cache.load(TourRevision.self, key: revisionKey(api: api, accountID: accountID, revisionID: revisionID)),
                  revision.id == revisionID, revision.tourID == record.tourID else { return nil }
            let restored = try? ActiveTourModel(api: api, revision: revision, mode: record.mode, cache: cache, isSignedIn: isSignedIn, accountID: accountID)
            restored?.routeFromCache = true
            return restored
        }
        guard let tour = cache.load(TourDetail.self, key: tourKey(tourID: record.tourID)) else { return nil }
        let restored = ActiveTourModel(api: api, tour: tour, mode: record.mode, cache: cache, isSignedIn: isSignedIn, accountID: accountID)
        restored.legacyResume = true
        return restored
    }

    /// Ends the walk: the tour is no longer resumed on launch. Progress stays.
    public func end() {
        if let record = cache.load(ActiveTourRecord.self, key: ActiveTourRecord.key),
           record.tourID == tour.tour.id, record.mode == mode, record.accountID == accountID, record.revisionID == revision?.id,
           record.origin == nil || record.origin == origin {
            cache.remove(ActiveTourRecord.key)
        }
    }

    // MARK: Route

    public var visitedStopIDs: Set<String> {
        Set(tour.stops.filter { visited.contains($0.position) }.map(\.id))
    }

    public func retainedMoment(_ id: String) -> MomentDetail? {
        revoked ? nil : revision?.snapshot?.moments[id]
    }

    public func coordinate(for stop: TourStop) -> CLLocationCoordinate2D? {
        guard !revoked else { return nil }
        if revision != nil {
            guard let arrival = stop.place.arrivalPoint, arrival.allows(mode),
                  let lat = arrival.point.latitude, let lng = arrival.point.longitude else { return nil }
            return CLLocationCoordinate2D(latitude: lat, longitude: lng)
        }
        return CLLocationCoordinate2D(latitude: stop.place.latitude, longitude: stop.place.longitude)
    }

    private func revoke() {
        revoked = true
        route = nil
        routeState = .failed("This revision is no longer available to this account.")
        nextStop = nil
        arrivedStop = nil
        monitor = nil
        availableUpdate = nil
        cache.remove(snapshotKey)
        cache.remove(progressKey)
        end()
    }

    public func checkForUpdate() async {
        guard let revision, !revoked, !checkingUpdate else { return }
        checkingUpdate = true
        updateError = nil
        defer { checkingUpdate = false }
        do {
            let latest = try await api.tour(tour.id)
            guard latest.id == tour.id, let id = latest.tour.revisionID else {
                throw APIError(status: 0, code: "revision_unavailable", message: "The latest reviewed revision could not be read.")
            }
            if id == revision.id { availableUpdate = nil; return }
            let candidate = try await api.revision(tour: tour.id, revision: id)
            guard candidate.id == id, candidate.tourID == tour.id, candidate.number > revision.number, !revoked else {
                throw APIError(status: 0, code: "revision_unavailable", message: "The update could not be confirmed.")
            }
            availableUpdate = candidate
            updateError = candidate.unavailableReason(mode: mode)
        } catch {
            updateError = "Could not check for an update. Your started itinerary is unchanged."
        }
    }

    /// A deliberate transition carries visits only for unchanged stable stop identities.
    /// The old revision's progress remains in its own record.
    public func adoptingUpdate() throws -> ActiveTourModel {
        guard !revoked, let target = availableUpdate, target.tourID == tour.id,
              target.unavailableReason(mode: mode) == nil, let snapshot = target.snapshot else {
            throw APIError(status: 0, code: "revision_unavailable", message: "This update is unavailable for the selected mode.")
        }
        let oldStops = Dictionary(uniqueKeysWithValues: tour.stops.map { ($0.id, $0) })
        let carry = Set(snapshot.tour.stops.filter { stop in
            guard visitedStopIDs.contains(stop.id), let old = oldStops[stop.id] else { return false }
            return old.place.id == stop.place.id && old.place.arrivalPoint == stop.place.arrivalPoint && old.featuredMoment?.id == stop.featuredMoment?.id
        }.map(\.id))
        let targetKey = "progress.v3.\(Self.scope(api: api, accountID: accountID)).\(target.id)"
        var next = cache.load(LocalTourProgress.self, key: targetKey)
            ?? LocalTourProgress(tourID: tour.id, mode: mode, visitedPositions: [], startedAt: Date())
        let known = Set(snapshot.tour.stops.map(\.id))
        let previous = next.revisionID == target.id && next.resetPending != true ? Set(next.visitedStopIDs ?? []).intersection(known) : []
        next.revisionID = target.id
        next.visitedStopIDs = Array(previous.union(carry)).sorted()
        next.visitedPositions = snapshot.tour.stops.filter { previous.union(carry).contains($0.id) }.map(\.position).sorted()
        next.updatedAt = Date()
        next.syncedAt = nil
        next.completedAt = previous.union(carry) == known ? Date() : nil
        try cache.savePrivate(next, key: targetKey)
        return try ActiveTourModel(api: api, revision: target, mode: mode, cache: cache, isSignedIn: isSignedIn, accountID: accountID)
    }

    /// Loads the route from the API, falling back to the cached copy when the request fails.
    public func loadRoute() async {
        guard !revoked else { return }
        if let revision {
            route = revision.snapshot?.routes[mode]
            routeState = .loaded
            routePending = false
            do {
                let retained = try await api.revision(tour: tour.id, revision: revision.id)
                guard retained.id == revision.id, retained.tourID == tour.id else {
                    throw APIError(status: 0, code: "invalid_revision", message: "The saved revision could not be confirmed.")
                }
                routeFromCache = false
            } catch {
                if let apiError = error as? APIError, [403, 404].contains(apiError.status) {
                    revoke()
                } else { routeFromCache = true }
            }
            return
        }
        if legacyResume {
            route = cache.load(TourRoute.self, key: Self.routeKey(tourID: tour.id, mode: mode))
            routeFromCache = route != nil
            routeState = route == nil ? .failed("This older trip has no saved reviewed route. End it and open a reviewed revision when connected.") : .loaded
            return
        }
        routeState = .loading
        let key = Self.routeKey(tourID: tour.tour.id, mode: mode)
        do {
            let fresh = try await api.route(tour: tour.tour.id, mode: mode)
            routePending = fresh.status == "pending"
            let cached = cache.load(TourRoute.self, key: key)
            if fresh.status != "ready", cached?.status == "ready" {
                route = cached
                routeFromCache = true
            } else {
                route = fresh
                routeFromCache = false
            }
            routeState = .loaded
            if fresh.status == "ready" { cache.save(fresh, key: key) }
        } catch {
            if let cached = cache.load(TourRoute.self, key: key) {
                route = cached
                routeFromCache = true
                routeState = .loaded
            } else {
                route = nil
                routeState = .failed((error as? APIError)?.message ?? error.localizedDescription)
            }
        }
    }

    /// The leg being travelled now: from the last visited stop to the next one.
    public var activeLeg: RouteLeg? {
        guard let next = nextStop, let legs = route?.legs else { return nil }
        return legs.first { $0.toPosition == next.position && visited.contains($0.fromPosition) }
    }

    public var pastLegs: [RouteLeg] {
        guard let legs = route?.legs else { return [] }
        guard let next = nextStop else { return legs }
        return legs.filter { $0.fromPosition < $0.toPosition && $0.toPosition < next.position }
    }

    public var futureLegs: [RouteLeg] {
        guard let legs = route?.legs, let next = nextStop else { return [] }
        return legs.filter { $0.toPosition > next.position || $0.toPosition < $0.fromPosition }
    }

    // MARK: Stops and arrival

    public var isComplete: Bool { !revoked && nextStop == nil && !tour.stops.isEmpty }

    public var progressText: String {
        "\(visited.count) of \(tour.stops.count) stops"
    }

    public func isVisited(_ stop: TourStop) -> Bool { visited.contains(stop.position) }

    /// Feeds a location fix: updates the distance label and lets the monitor decide on arrival.
    public func handle(location: CLLocation) {
        guard !revoked else { return }
        lastLocation = location
        guard let monitor else { return }
        distanceToNextM = monitor.distance(from: location)
        monitor.update(location: location)
    }

    /// "I'm here": arrival by hand when GPS cannot confirm it.
    public func markArrivedAtNextStop() {
        guard let next = nextStop else { return }
        arrive(at: next)
    }

    private func arrive(at stop: TourStop) {
        guard !revoked, !visited.contains(stop.position) else { return }
        visited.insert(stop.position)
        arrivedStop = stop
        recordVisited()
        onArrive?(stop)
        sync()
    }

    /// Writes `visited` into the local record and advances to the next stop.
    private func recordVisited() {
        progress.visitedPositions = visited.sorted()
        if revision != nil { progress.visitedStopIDs = visitedStopIDs.sorted() }
        progress.updatedAt = Date()
        advance()
        if isComplete { progress.completedAt = progress.completedAt ?? Date() }
        saveProgress()
    }

    /// Points `nextStop` and the monitor at the first unvisited stop.
    private func advance() {
        nextStop = tour.stops.sorted { $0.position < $1.position }.first { !visited.contains($0.position) }
        if let next = nextStop, let coordinate = coordinate(for: next) {
            if let monitor {
                monitor.retarget(stop: coordinate, radiusM: next.place.arrivalRadiusM)
            } else {
                let m = ArrivalMonitor(stop: coordinate, radiusM: next.place.arrivalRadiusM)
                m.onArrival = { [weak self] in
                    guard let self, let stop = self.nextStop else { return }
                    self.arrive(at: stop)
                }
                monitor = m
            }
            if let lastLocation { distanceToNextM = monitor?.distance(from: lastLocation) }
        } else {
            monitor = nil
            distanceToNextM = nil
        }
    }

    // MARK: Progress sync

    /// True when the server has not seen the latest local progress.
    public var needsSync: Bool { progress.needsSync }

    /// Merges the server's record for this tour into the local one, so a second device or the
    /// other travel mode never loses stops. The server keeps one record per tour.
    public func loadRemoteProgress() async {
        guard !revoked, !legacyResume, isSignedIn() else { return }
        if progress.resetPending == true {
            await syncIfNeeded()
            return
        }
        let generation = progress.startedAt
        if let revision {
            do {
                guard let remote = try await api.revisionProgress(tour: tour.id, revision: revision.id),
                      !revoked, isSignedIn(), progress.startedAt == generation, progress.resetPending != true,
                      remote.accountID == accountID, remote.tourID == tour.id, remote.revisionID == revision.id else { return }
                let remoteIDs = Set(remote.visitedStopIDs)
                let positions = Set(tour.stops.filter { remoteIDs.contains($0.id) }.map(\.position))
                if !positions.isSubset(of: visited) { visited.formUnion(positions); recordVisited() }
                if positions == visited { progress.syncedAt = Date(); saveProgress() }
                else { sync() }
            } catch {
                if !revoked && isSignedIn() { syncError = "Could not read progress for this revision." }
            }
            return
        }
        guard let records = try? await api.progress(), isSignedIn(), progress.startedAt == generation,
              let remote = records.first(where: { $0.tourID == tour.tour.id }) else { return }
        let merged = visited.union(remote.visitedPositions)
        if merged != visited {
            visited = merged
            recordVisited()
        }
        if Set(remote.visitedPositions) == merged {
            progress.syncedAt = Date()
            cache.save(progress, key: progressKey)
        } else {
            sync()
        }
    }

    /// Sends progress the server is missing (after an offline stretch, on foreground).
    public func syncIfNeeded() async {
        guard !revoked, !legacyResume, isSignedIn(), progress.needsSync else { return }
        sync()
        await flush()
    }

    /// PUTs progress when signed in, one request at a time and in order; local progress is
    /// already saved. Failures are kept in syncError and retried by syncIfNeeded.
    private func sync() {
        guard !revoked, !legacyResume, isSignedIn() else { return }
        let tourID = tour.tour.id
        let previous = pendingSync
        pendingSync = Task { [weak self] in
            await previous?.value
            guard let self, !self.revoked, self.isSignedIn(), self.progress.needsSync else { return }
            let snapshot = self.progress
            do {
                if snapshot.resetPending == true {
                    if let revision = self.revision {
                        _ = try await self.api.putRevisionProgress(tour: tourID, revision: revision.id, RevisionProgressRequest(mode: self.mode, visitedStopIDs: [], completed: false, reset: true))
                    } else {
                        _ = try await self.api.putProgress(tour: tourID, ProgressRequest(mode: self.mode, visitedPositions: [], completed: false, reset: true))
                    }
                    guard !self.revoked, self.isSignedIn(), self.progress.startedAt == snapshot.startedAt else { return }
                    self.progress.resetPending = false
                }
                guard !self.revoked, self.isSignedIn(), self.progress.startedAt == snapshot.startedAt else { return }
                if !snapshot.visitedPositions.isEmpty {
                    if let revision = self.revision {
                        _ = try await self.api.putRevisionProgress(tour: tourID, revision: revision.id, RevisionProgressRequest(mode: self.mode, visitedStopIDs: snapshot.visitedStopIDs ?? [], completed: snapshot.completedAt != nil))
                    } else {
                        _ = try await self.api.putProgress(tour: tourID, ProgressRequest(mode: self.mode, visitedPositions: snapshot.visitedPositions, completed: snapshot.completedAt != nil))
                    }
                }
                guard !self.revoked, self.isSignedIn(), self.progress.startedAt == snapshot.startedAt else { return }
                self.progress.syncedAt = snapshot.updatedAt
                self.syncError = nil
                self.saveProgress()
            } catch {
                self.syncError = (error as? APIError)?.message ?? error.localizedDescription
            }
        }
    }

    /// Waits for in-flight progress posts (tests, and the end-tour button).
    public func flush() async {
        await pendingSync?.value
    }

    /// Forgets local progress so the tour starts from stop 1 next time; when signed in the
    /// server record is cleared too, otherwise the next start would merge the old stops back.
    public func resetProgress() {
        guard !revoked else { return }
        visited = []
        progress = LocalTourProgress(tourID: tour.tour.id, mode: mode, visitedPositions: [], startedAt: Date())
        progress.resetPending = isSignedIn()
        progress.revisionID = revision?.id
        progress.visitedStopIDs = revision == nil ? nil : []
        saveProgress()
        advance()
        sync()
    }

    // MARK: Maps handoff

    public static func mapsLaunchOptions(mode: String) -> [String: Any] {
        [MKLaunchOptionsDirectionsModeKey: mode == "bike" ? MKLaunchOptionsDirectionsModeCycling : MKLaunchOptionsDirectionsModeWalking]
    }

    public func mapItemForNextStop() -> MKMapItem? {
        guard let next = nextStop, let coordinate = coordinate(for: next) else { return nil }
        let placemark = MKPlacemark(coordinate: coordinate)
        let item = MKMapItem(placemark: placemark)
        item.name = next.place.name
        return item
    }

    private func saveProgress() {
        if revision != nil {
            do { try cache.savePrivate(progress, key: progressKey) }
            catch { syncError = "Progress could not be saved on this device. Keep the trip open and try syncing when connected." }
        } else { cache.save(progress, key: progressKey) }
    }

    /// Hands the current leg to Apple Maps with walking or cycling directions.
    public func openNextStopInMaps() {
        mapItemForNextStop()?.openInMaps(launchOptions: Self.mapsLaunchOptions(mode: mode))
    }
}
