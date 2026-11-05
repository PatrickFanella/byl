import SwiftUI
import MapKit
import InvisibleMuseumKit

/// The route on a map: past legs faint, the active leg in the accent colour, future legs muted,
/// a numbered pin per stop, and the visitor's own position.
struct TourMapView: View {
    let model: ActiveTourModel
    @State private var camera: MapCameraPosition = .automatic

    var body: some View {
        Map(position: $camera) {
            UserAnnotation()
            ForEach(model.pastLegs, id: \.fromPosition) { leg in
                MapPolyline(coordinates: coordinates(leg))
                    .stroke(Museum.faint, style: StrokeStyle(lineWidth: 4, lineCap: .round, lineJoin: .round))
            }
            ForEach(model.futureLegs, id: \.fromPosition) { leg in
                MapPolyline(coordinates: coordinates(leg))
                    .stroke(Museum.muted, style: StrokeStyle(lineWidth: 4, lineCap: .round, lineJoin: .round, dash: [6, 8]))
            }
            if let leg = model.activeLeg {
                MapPolyline(coordinates: coordinates(leg))
                    .stroke(Museum.routeHalo, style: StrokeStyle(lineWidth: 8, lineCap: .round, lineJoin: .round))
                MapPolyline(coordinates: coordinates(leg))
                    .stroke(Museum.action, style: StrokeStyle(lineWidth: 4, lineCap: .round, lineJoin: .round))
            }
            ForEach(model.tour.stops) { stop in
                Annotation(stop.place.name, coordinate: CLLocationCoordinate2D(latitude: stop.place.latitude, longitude: stop.place.longitude)) {
                    StopPin(number: stop.position, visited: model.isVisited(stop), isNext: model.nextStop?.id == stop.id)
                }
                .annotationTitles(.hidden)
            }
        }
        .mapStyle(.standard(elevation: .flat, pointsOfInterest: .excludingAll))
        .mapControls {
            MapUserLocationButton()
            MapCompass()
        }
        .onChange(of: model.nextStop?.id, initial: true) { _, _ in
            frameRoute()
        }
    }

    private func coordinates(_ leg: RouteLeg) -> [CLLocationCoordinate2D] {
        leg.geometry.compactMap { pair in
            pair.count >= 2 ? CLLocationCoordinate2D(latitude: pair[1], longitude: pair[0]) : nil
        }
    }

    /// Frames the stops still ahead (and the visitor when known); the whole route when done.
    private func frameRoute() {
        var points = model.tour.stops
            .filter { model.nextStop == nil || $0.position >= (model.nextStop?.position ?? 0) }
            .map { CLLocationCoordinate2D(latitude: $0.place.latitude, longitude: $0.place.longitude) }
        if let here = model.lastLocation?.coordinate { points.append(here) }
        guard let first = points.first else { return }
        var minLat = first.latitude, maxLat = first.latitude, minLng = first.longitude, maxLng = first.longitude
        for p in points {
            minLat = min(minLat, p.latitude); maxLat = max(maxLat, p.latitude)
            minLng = min(minLng, p.longitude); maxLng = max(maxLng, p.longitude)
        }
        let span = MKCoordinateSpan(latitudeDelta: max((maxLat - minLat) * 1.6, 0.004), longitudeDelta: max((maxLng - minLng) * 1.6, 0.004))
        let center = CLLocationCoordinate2D(latitude: (minLat + maxLat) / 2, longitude: (minLng + maxLng) / 2)
        withAnimation(.easeInOut(duration: 0.6)) {
            camera = .region(MKCoordinateRegion(center: center, span: span))
        }
    }
}

struct StopPin: View {
    let number: Int
    let visited: Bool
    let isNext: Bool

    var body: some View {
        ZStack {
            Circle()
                .fill(visited ? Museum.successSurface : (isNext ? Museum.action : Museum.surfaceElevated))
                .frame(width: isNext ? 36 : 32, height: isNext ? 36 : 32)
                .overlay(Circle().stroke(Museum.borderStrong, lineWidth: 2))
                .shadow(color: .black.opacity(0.4), radius: 6, y: 3)
            if visited {
                Image(systemName: "checkmark")
                    .font(.system(size: 13, weight: .bold))
                    .foregroundStyle(Museum.success)
            } else {
                Text("\(number)")
                    .font(Museum.meta(13))
                    .foregroundStyle(isNext ? Museum.actionText : Museum.text)
            }
        }
        .accessibilityLabel(visited ? "Stop \(number), visited" : "Stop \(number)")
    }
}
