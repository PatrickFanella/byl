import Foundation
import CoreLocation

/// Decides when the visitor has reached a stop. `evaluate` is a pure check; `update` wraps it
/// with the once-per-stop callback the active tour uses.
public final class ArrivalMonitor {
    /// Fixes less accurate than this are ignored; a 200 m fix in a car park must never count.
    public static let maxAccuracyM: Double = 65

    public private(set) var stop: CLLocationCoordinate2D
    public private(set) var radiusM: Double
    public private(set) var hasArrived = false
    public var onArrival: (() -> Void)?

    public init(stop: CLLocationCoordinate2D, radiusM: Double) {
        self.stop = stop
        self.radiusM = radiusM
    }

    /// True when the fix is usable and within the stop's radius plus the fix's own uncertainty.
    public func evaluate(location: CLLocation) -> Bool {
        Self.hasArrived(location: location, stop: stop, radiusM: radiusM)
    }

    public static func hasArrived(location: CLLocation, stop: CLLocationCoordinate2D, radiusM: Double) -> Bool {
        guard abs(location.timestamp.timeIntervalSinceNow) <= 30 else { return false }
        let accuracy = location.horizontalAccuracy
        guard accuracy >= 0, accuracy <= maxAccuracyM else { return false }
        let distance = location.distance(from: CLLocation(latitude: stop.latitude, longitude: stop.longitude))
        return distance <= radiusM + accuracy
    }

    /// Metres from the fix to the stop, for "120 m to go" labels.
    public func distance(from location: CLLocation) -> Double {
        location.distance(from: CLLocation(latitude: stop.latitude, longitude: stop.longitude))
    }

    /// Feeds a new fix; fires `onArrival` the first time the stop is reached and not again
    /// until `retarget` moves the monitor to another stop.
    public func update(location: CLLocation) {
        guard !hasArrived, evaluate(location: location) else { return }
        hasArrived = true
        onArrival?()
    }

    /// Points the monitor at the next stop and arms the callback again.
    public func retarget(stop: CLLocationCoordinate2D, radiusM: Double) {
        self.stop = stop
        self.radiusM = radiusM
        hasArrived = false
    }
}
