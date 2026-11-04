import Foundation
import Observation
import CoreLocation

/// Builds or edits a visitor route: pick published places in one city, order them, name the
/// route, choose modes and visibility, and create it. Validation mirrors the API (2–30 stops).
@MainActor
@Observable
public final class RouteBuilderModel {
    public static let minStops = 2
    public static let maxStops = 30

    public let api: Endpoints
    public let city: City
    public let editing: TourDetail?

    public var title = ""
    public var summary = ""
    public var walk = true
    public var bike = true
    public var visibility = "unlisted"
    public private(set) var stops: [PlaceSummary] = []

    public private(set) var places: [PlaceSummary] = []
    public private(set) var placesState: LoadState = .idle
    public var search = ""
    private var sortOrigin: CLLocation?

    public private(set) var busy = false
    public private(set) var error: String?
    public private(set) var fieldErrors: [String: String] = [:]

    public init(api: Endpoints, city: City, editing: TourDetail? = nil) {
        self.api = api
        self.city = city
        self.editing = editing
        if let editing {
            title = editing.tour.title
            summary = editing.tour.summary
            walk = editing.tour.modes.contains("walk")
            bike = editing.tour.modes.contains("bike")
            visibility = editing.tour.visibility
            stops = editing.stops.sorted { $0.position < $1.position }.map(\.place)
        }
    }

    public var isEditing: Bool { editing != nil }

    // MARK: Stops

    public func contains(_ place: PlaceSummary) -> Bool { stops.contains { $0.id == place.id } }

    public func add(_ place: PlaceSummary) {
        guard !contains(place), stops.count < Self.maxStops else { return }
        stops.append(place)
        fieldErrors["stops"] = nil
    }

    public func remove(_ placeID: String) {
        stops.removeAll { $0.id == placeID }
    }

    public func move(from source: Int, to destination: Int) {
        guard stops.indices.contains(source), stops.indices.contains(destination), source != destination else { return }
        let item = stops.remove(at: source)
        stops.insert(item, at: destination)
    }

    /// Guidance under the stops list: how many more are needed, or that the cap is reached.
    public var stopsHint: String? {
        if stops.count < Self.minStops {
            return "Add at least \(Self.minStops) stops (\(stops.count) so far)"
        }
        if stops.count >= Self.maxStops {
            return "\(Self.maxStops) stops is the maximum"
        }
        return nil
    }

    /// The API's own message for the stops field after a 422.
    public var stopsError: String? { fieldErrors["stops"] }

    public var modes: [String] {
        var out: [String] = []
        if walk { out.append("walk") }
        if bike { out.append("bike") }
        return out
    }

    public var canSubmit: Bool {
        !busy && !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !modes.isEmpty
            && (Self.minStops...Self.maxStops).contains(stops.count)
    }

    // MARK: Places

    public func loadPlaces() async {
        placesState = .loading
        do {
            places = try await api.places(city: city.slug)
            placesState = .loaded
        } catch {
            placesState = .failed((error as? APIError)?.message ?? error.localizedDescription)
        }
    }

    /// Orders the picker by distance from here.
    public func sortNear(latitude: Double, longitude: Double) {
        sortOrigin = CLLocation(latitude: latitude, longitude: longitude)
    }

    public var filteredPlaces: [PlaceSummary] {
        let needle = search.trimmingCharacters(in: .whitespaces).lowercased()
        var out = needle.isEmpty ? places : places.filter { $0.name.lowercased().contains(needle) || $0.address.lowercased().contains(needle) }
        if let origin = sortOrigin {
            out.sort { distance($0, origin) < distance($1, origin) }
        }
        return out
    }

    private func distance(_ place: PlaceSummary, _ origin: CLLocation) -> Double {
        CLLocation(latitude: place.latitude, longitude: place.longitude).distance(from: origin)
    }

    // MARK: Submit

    /// Creates (POST) or updates (PATCH) the route. Returns nil on failure with `error` and
    /// `fieldErrors` set from the API envelope.
    public func submit() async -> TourDetail? {
        guard canSubmit else { return nil }
        busy = true
        error = nil
        fieldErrors = [:]
        defer { busy = false }
        let stopInputs = stops.map { StopInput(placeID: $0.id) }
        do {
            if let editing {
                let body = Endpoints.UpdateUserTour(title: title.trimmingCharacters(in: .whitespacesAndNewlines), summary: summary, modes: modes, visibility: visibility, stops: stopInputs)
                return try await api.updateTour(editing.tour.id, body)
            }
            let body = CreateUserTourRequest(city: city.slug, title: title.trimmingCharacters(in: .whitespacesAndNewlines), summary: summary, modes: modes, visibility: visibility, stops: stopInputs)
            return try await api.createTour(body)
        } catch let apiError as APIError {
            error = apiError.message
            fieldErrors = apiError.fields
            return nil
        } catch {
            self.error = error.localizedDescription
            return nil
        }
    }

    /// The link a visitor shares: `{base}/t/{slug}`; the app opens it through its URL handler.
    public static func shareURL(slug: String, base: URL) -> URL {
        base.appendingPathComponent("t").appendingPathComponent(slug)
    }
}
