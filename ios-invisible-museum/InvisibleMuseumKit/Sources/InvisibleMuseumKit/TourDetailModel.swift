import Foundation
import Observation

/// Shared async load state for the view models.
public enum LoadState: Equatable {
    case idle
    case loading
    case loaded
    case failed(String)
}

/// Loads one tour and owns the save toggle, so the detail view body stays free of network code.
@MainActor
@Observable
public final class TourDetailModel {
    public typealias State = LoadState

    public let api: Endpoints
    public let ref: String

    public private(set) var state: State = .idle
    public private(set) var tour: TourDetail?
    public private(set) var isSaved: Bool
    public private(set) var saveBusy = false
    public private(set) var saveError: String?

    public init(api: Endpoints, ref: String, isSaved: Bool = false) {
        self.api = api
        self.ref = ref
        self.isSaved = isSaved
    }

    public func load() async {
        state = .loading
        do {
            tour = try await api.tour(ref)
            state = .loaded
        } catch {
            tour = nil
            state = .failed(Self.message(for: error))
        }
    }

    /// Modes the tour offers, in the tour's own order.
    public var modes: [String] { tour?.tour.modes ?? [] }

    public func canStart(mode: String) -> Bool {
        guard let tour, tour.tour.revisionID != nil, tour.tour.status == "published",
              tour.journeys?[mode]?.usable == true, !tour.stops.isEmpty else { return false }
        return tour.stops.allSatisfy { $0.place.arrivalPoint?.allows(mode) == true }
    }

    /// "24 min walk" / "7 min ride" for a ready route, nil otherwise.
    public func estimateText(mode: String) -> String? {
        guard let route = tour?.tour.routes[mode] else { return nil }
        return Self.estimateText(for: route)
    }

    /// "1.8 km" / "850 m" for a ready route, nil otherwise.
    public func distanceText(mode: String) -> String? {
        guard let route = tour?.tour.routes[mode], route.status == "ready" else { return nil }
        return Self.distanceText(meters: route.distanceM)
    }

    public static func estimateText(for route: RouteSummary) -> String? {
        guard route.status == "ready" else { return nil }
        let minutes = route.estimatedMinutes ?? Int((Double(route.durationS) / 60).rounded())
        return "\(max(minutes, 1)) min \(modeVerb(route.mode))"
    }

    public static func distanceText(meters: Double) -> String {
        if meters < 1000 {
            return "\(Int(meters.rounded())) m"
        }
        return String(format: "%.1f km", meters / 1000)
    }

    /// walk → "walk", bike → "ride".
    public static func modeVerb(_ mode: String) -> String {
        mode == "bike" ? "ride" : "walk"
    }

    /// Saves or unsaves the tour for the signed-in account. On failure the previous state stays
    /// and `saveError` carries the API message.
    public func toggleSave() async {
        guard let id = tour?.tour.id, !saveBusy else { return }
        saveBusy = true
        saveError = nil
        defer { saveBusy = false }
        do {
            if isSaved {
                try await api.unsave(tour: id)
                isSaved = false
            } else {
                try await api.save(tour: id)
                isSaved = true
            }
        } catch {
            saveError = Self.message(for: error)
        }
    }

    public func markSaved(_ saved: Bool) {
        isSaved = saved
    }

    static func message(for error: Error) -> String {
        if let api = error as? APIError { return api.message }
        return error.localizedDescription
    }
}
