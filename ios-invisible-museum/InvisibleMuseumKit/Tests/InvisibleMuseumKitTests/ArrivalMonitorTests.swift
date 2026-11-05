import XCTest
import CoreLocation
@testable import InvisibleMuseumKit

final class ArrivalMonitorTests: XCTestCase {
    // Main Street Station, Downers Grove.
    private let station = CLLocationCoordinate2D(latitude: 41.7960, longitude: -88.0093)

    /// A fix `meters` east of the station with the given horizontal accuracy.
    private func fix(eastMeters meters: Double, accuracy: Double) -> CLLocation {
        let lng = station.longitude + meters / (111_320 * cos(station.latitude * .pi / 180))
        return CLLocation(coordinate: CLLocationCoordinate2D(latitude: station.latitude, longitude: lng),
                          altitude: 0, horizontalAccuracy: accuracy, verticalAccuracy: 0, timestamp: Date())
    }

    func testInsideRadiusWithGoodAccuracyArrives() {
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        XCTAssertTrue(monitor.evaluate(location: fix(eastMeters: 20, accuracy: 10)))
    }

    func testStaleAndFutureLocationsDoNotTriggerArrival() {
        for age in [-60.0, 60.0] {
            let location = CLLocation(coordinate: station, altitude: 0, horizontalAccuracy: 10,
                                      verticalAccuracy: 0, timestamp: Date(timeIntervalSinceNow: age))
            XCTAssertFalse(ArrivalMonitor.hasArrived(location: location, stop: station, radiusM: 40))
        }
    }

    func testInsideRadiusWithPoorAccuracyDoesNotArrive() {
        // Review Focus 1: a 200 m fix in a car park must not count as arrived.
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        XCTAssertFalse(monitor.evaluate(location: fix(eastMeters: 5, accuracy: 200)))
    }

    func testOutsideRadiusDoesNotArrive() {
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        XCTAssertFalse(monitor.evaluate(location: fix(eastMeters: 120, accuracy: 10)))
    }

    func testDistanceMayUseUpTheAccuracyMargin() {
        // 50 m away with a 15 m fix: within radius (40) + accuracy (15).
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        XCTAssertTrue(monitor.evaluate(location: fix(eastMeters: 50, accuracy: 15)))
        XCTAssertFalse(monitor.evaluate(location: fix(eastMeters: 60, accuracy: 15)))
    }

    func testInvalidAccuracyNeverArrives() {
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        XCTAssertFalse(monitor.evaluate(location: fix(eastMeters: 0, accuracy: -1)))
    }

    func testAccuracyCeilingIsSixtyFiveMetres() {
        XCTAssertEqual(ArrivalMonitor.maxAccuracyM, 65)
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        XCTAssertTrue(monitor.evaluate(location: fix(eastMeters: 0, accuracy: 65)))
        XCTAssertFalse(monitor.evaluate(location: fix(eastMeters: 0, accuracy: 66)))
    }

    func testArrivalFiresOncePerStopUntilTheStopChanges() {
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        var arrivals = 0
        monitor.onArrival = { arrivals += 1 }
        monitor.update(location: fix(eastMeters: 10, accuracy: 10))
        monitor.update(location: fix(eastMeters: 12, accuracy: 10))
        monitor.update(location: fix(eastMeters: 300, accuracy: 10))
        monitor.update(location: fix(eastMeters: 8, accuracy: 10))
        XCTAssertEqual(arrivals, 1, "re-entering the same stop must not fire again")
        XCTAssertTrue(monitor.hasArrived)

        let tivoli = CLLocationCoordinate2D(latitude: 41.7943, longitude: -88.0107)
        monitor.retarget(stop: tivoli, radiusM: 30)
        XCTAssertFalse(monitor.hasArrived)
        monitor.update(location: CLLocation(coordinate: tivoli, altitude: 0, horizontalAccuracy: 8, verticalAccuracy: 0, timestamp: Date()))
        XCTAssertEqual(arrivals, 2)
    }

    func testDistanceToStopIsExposedForTheUI() {
        let monitor = ArrivalMonitor(stop: station, radiusM: 40)
        let d = monitor.distance(from: fix(eastMeters: 100, accuracy: 10))
        XCTAssertEqual(d, 100, accuracy: 2)
    }
}
