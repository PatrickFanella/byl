import Foundation
import Observation
import CoreLocation
import MapKit

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

    private let progressKey: String
    private let accountID: String?
    private var progress: LocalTourProgress
    private var monitor: ArrivalMonitor?
    private var pendingSync: Task<Void, Never>?

    public init(api: Endpoints, tour: TourDetail, mode: String, cache: DiskCache, isSignedIn: @escaping () -> Bool, accountID: String? = nil) {
        self.api = api
        self.tour = tour
        self.mode = mode
        self.cache = cache
        self.isSignedIn = isSignedIn
        self.id = "\(tour.tour.id).\(mode)"
        self.accountID = accountID
        let key = LocalTourProgress.key(tourID: tour.tour.id, mode: mode, accountID: accountID)
        self.progressKey = key
        progress = cache.load(LocalTourProgress.self, key: key)
            ?? LocalTourProgress(tourID: tour.tour.id, mode: mode, visitedPositions: [], startedAt: Date())
        visited = Set(progress.visitedPositions)
        cache.save(tour, key: Self.tourKey(tourID: tour.tour.id))
        cache.save(ActiveTourRecord(tourID: tour.tour.id, mode: mode, accountID: accountID), key: ActiveTourRecord.key)
        advance()
    }

    /// Rebuilds the tour that was active when the app last ran, from the cached detail alone,
    /// so it comes back even without signal (Review Focus 2). Nil when no tour was active.
    public static func restore(api: Endpoints, cache: DiskCache, isSignedIn: @escaping () -> Bool, accountID: String? = nil) -> ActiveTourModel? {
        guard let record = cache.load(ActiveTourRecord.self, key: ActiveTourRecord.key), record.accountID == accountID,
              let tour = cache.load(TourDetail.self, key: tourKey(tourID: record.tourID)) else { return nil }
        return ActiveTourModel(api: api, tour: tour, mode: record.mode, cache: cache, isSignedIn: isSignedIn, accountID: accountID)
    }

    /// Ends the walk: the tour is no longer resumed on launch. Progress stays.
    public func end() {
        if let record = cache.load(ActiveTourRecord.self, key: ActiveTourRecord.key),
           record.tourID == tour.tour.id, record.mode == mode, record.accountID == accountID {
            cache.remove(ActiveTourRecord.key)
        }
    }

    // MARK: Route

    /// Loads the route from the API, falling back to the cached copy when the request fails.
    public func loadRoute() async {
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
        return legs.filter { $0.toPosition < next.position }
    }

    public var futureLegs: [RouteLeg] {
        guard let legs = route?.legs, let next = nextStop else { return [] }
        return legs.filter { $0.toPosition > next.position }
    }

    // MARK: Stops and arrival

    public var isComplete: Bool { nextStop == nil && !tour.stops.isEmpty }

    public var progressText: String {
        "\(visited.count) of \(tour.stops.count) stops"
    }

    public func isVisited(_ stop: TourStop) -> Bool { visited.contains(stop.position) }

    /// Feeds a location fix: updates the distance label and lets the monitor decide on arrival.
    public func handle(location: CLLocation) {
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
        guard !visited.contains(stop.position) else { return }
        visited.insert(stop.position)
        arrivedStop = stop
        recordVisited()
        onArrive?(stop)
        sync()
    }

    /// Writes `visited` into the local record and advances to the next stop.
    private func recordVisited() {
        progress.visitedPositions = visited.sorted()
        progress.updatedAt = Date()
        advance()
        if isComplete { progress.completedAt = progress.completedAt ?? Date() }
        cache.save(progress, key: progressKey)
    }

    /// Points `nextStop` and the monitor at the first unvisited stop.
    private func advance() {
        nextStop = tour.stops.sorted { $0.position < $1.position }.first { !visited.contains($0.position) }
        if let next = nextStop {
            let coordinate = CLLocationCoordinate2D(latitude: next.place.latitude, longitude: next.place.longitude)
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
        guard isSignedIn() else { return }
        if progress.resetPending == true {
            await syncIfNeeded()
            return
        }
        let generation = progress.startedAt
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
        guard isSignedIn(), progress.needsSync else { return }
        sync()
        await flush()
    }

    /// PUTs progress when signed in, one request at a time and in order; local progress is
    /// already saved. Failures are kept in syncError and retried by syncIfNeeded.
    private func sync() {
        guard isSignedIn() else { return }
        let tourID = tour.tour.id
        let previous = pendingSync
        pendingSync = Task { [weak self] in
            await previous?.value
            guard let self, self.isSignedIn(), self.progress.needsSync else { return }
            let snapshot = self.progress
            do {
                if snapshot.resetPending == true {
                    _ = try await self.api.putProgress(tour: tourID, ProgressRequest(mode: self.mode, visitedPositions: [], completed: false, reset: true))
                    if self.progress.startedAt == snapshot.startedAt { self.progress.resetPending = false }
                }
                guard self.isSignedIn(), self.progress.startedAt == snapshot.startedAt else { return }
                if !snapshot.visitedPositions.isEmpty {
                    _ = try await self.api.putProgress(tour: tourID, ProgressRequest(mode: self.mode, visitedPositions: snapshot.visitedPositions, completed: snapshot.completedAt != nil))
                }
                self.progress.syncedAt = snapshot.updatedAt
                self.syncError = nil
                self.cache.save(self.progress, key: self.progressKey)
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
        visited = []
        progress = LocalTourProgress(tourID: tour.tour.id, mode: mode, visitedPositions: [], startedAt: Date())
        progress.resetPending = isSignedIn()
        cache.save(progress, key: progressKey)
        advance()
        sync()
    }

    // MARK: Maps handoff

    public static func mapsLaunchOptions(mode: String) -> [String: Any] {
        [MKLaunchOptionsDirectionsModeKey: mode == "bike" ? MKLaunchOptionsDirectionsModeCycling : MKLaunchOptionsDirectionsModeWalking]
    }

    public func mapItemForNextStop() -> MKMapItem? {
        guard let next = nextStop else { return nil }
        let placemark = MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: next.place.latitude, longitude: next.place.longitude))
        let item = MKMapItem(placemark: placemark)
        item.name = next.place.name
        return item
    }

    /// Hands the current leg to Apple Maps with walking or cycling directions.
    public func openNextStopInMaps() {
        mapItemForNextStop()?.openInMaps(launchOptions: Self.mapsLaunchOptions(mode: mode))
    }
}
