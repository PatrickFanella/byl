import CoreLocation
import Foundation

enum UserProximity: Equatable {
    case unknown
    case tooFar(distanceMeters: Double, bearingDegrees: Double)
    case nearby(distanceMeters: Double, bearingDegrees: Double)
    case inside(distanceMeters: Double)
}

enum GeoMath {
    static let arrivalRadiusMeters = 50.0
    static let nearbyMultiplier = 2.0

    static func distanceMeters(from: CLLocationCoordinate2D, to: CLLocationCoordinate2D) -> Double {
        CLLocation(latitude: from.latitude, longitude: from.longitude)
            .distance(from: CLLocation(latitude: to.latitude, longitude: to.longitude))
    }

    static func bearingDegrees(from: CLLocationCoordinate2D, to: CLLocationCoordinate2D) -> Double {
        let lat1 = from.latitude * .pi / 180
        let lat2 = to.latitude * .pi / 180
        let deltaLon = (to.longitude - from.longitude) * .pi / 180
        let y = sin(deltaLon) * cos(lat2)
        let x = cos(lat1) * sin(lat2) - sin(lat1) * cos(lat2) * cos(deltaLon)
        return normalizeDegrees((atan2(y, x) * 180 / .pi) + 360)
    }

    static func relativeBearingDegrees(targetBearing: Double, heading: Double?) -> Double {
        guard let heading else { return normalizeDegrees(targetBearing) }
        return normalizeDegrees((targetBearing - heading) + 360)
    }

    static func classify(user: CLLocationCoordinate2D?, target: CLLocationCoordinate2D, radiusMeters: Double) -> UserProximity {
        guard let user else { return .unknown }
        let distance = distanceMeters(from: user, to: target)
        let bearing = bearingDegrees(from: user, to: target)
        if distance <= radiusMeters { return .inside(distanceMeters: distance) }
        if distance <= radiusMeters * nearbyMultiplier { return .nearby(distanceMeters: distance, bearingDegrees: bearing) }
        return .tooFar(distanceMeters: distance, bearingDegrees: bearing)
    }

    static func coordinate(from origin: CLLocationCoordinate2D, eastMeters: Double, northMeters: Double) -> CLLocationCoordinate2D {
        let earthRadiusMeters = 6_378_137.0
        let latitudeDelta = northMeters / earthRadiusMeters
        let longitudeDelta = eastMeters / (earthRadiusMeters * cos(origin.latitude * .pi / 180))
        return CLLocationCoordinate2D(
            latitude: origin.latitude + latitudeDelta * 180 / .pi,
            longitude: origin.longitude + longitudeDelta * 180 / .pi
        )
    }

    static func compassDirection(for bearingDegrees: Double) -> String {
        let directions = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"]
        let normalized = normalizeDegrees(bearingDegrees)
        let index = Int((normalized + 22.5) / 45.0) % directions.count
        return directions[index]
    }

    static func formatDistance(_ distanceMeters: Double) -> String {
        if distanceMeters >= 1000 {
            return String(format: "%.1f km", distanceMeters / 1000)
        }
        return String(format: "%.0f m", distanceMeters)
    }

    private static func normalizeDegrees(_ value: Double) -> Double {
        let normalized = value.truncatingRemainder(dividingBy: 360)
        return normalized >= 0 ? normalized : normalized + 360
    }
}
