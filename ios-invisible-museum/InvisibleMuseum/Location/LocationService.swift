import CoreLocation
import Observation

/// Thin CLLocationManager wrapper: authorization state, the latest fix, and a callback the
/// active tour feeds into its ArrivalMonitor. Heading is not tracked; the map rotates itself.
@MainActor
@Observable
final class MuseumLocationService: NSObject, CLLocationManagerDelegate {
    enum AuthorizationState: Equatable {
        case unknown
        case denied
        case authorized
    }

    private let manager = CLLocationManager()

    var authorizationState: AuthorizationState = .unknown
    var currentLocation: CLLocation?
    var lastErrorMessage: String?
    /// Called on the main actor for every new fix.
    var onLocation: ((CLLocation) -> Void)?

    private var wantsUpdates = false

    override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
        manager.distanceFilter = 5
        manager.activityType = .fitness
        manager.pausesLocationUpdatesAutomatically = true
        updateAuthorizationState(manager.authorizationStatus)
    }

    func requestWhenInUseAuthorization() {
        guard authorizationState == .unknown else { return }
        manager.requestWhenInUseAuthorization()
    }

    /// Starts updates now if allowed, or as soon as the visitor grants access.
    func start() {
        wantsUpdates = true
        guard authorizationState == .authorized else { return }
        manager.startUpdatingLocation()
    }

    func stop() {
        wantsUpdates = false
        manager.stopUpdatingLocation()
    }

    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        Task { @MainActor in
            updateAuthorizationState(status)
            if authorizationState == .authorized, wantsUpdates {
                self.manager.startUpdatingLocation()
            }
        }
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let location = locations.last else { return }
        Task { @MainActor in
            currentLocation = location
            onLocation?(location)
        }
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        Task { @MainActor in
            lastErrorMessage = error.localizedDescription
        }
    }

    private func updateAuthorizationState(_ status: CLAuthorizationStatus) {
        switch status {
        case .authorizedAlways, .authorizedWhenInUse:
            authorizationState = .authorized
        case .denied, .restricted:
            authorizationState = .denied
        case .notDetermined:
            authorizationState = .unknown
        @unknown default:
            authorizationState = .unknown
        }
    }
}
