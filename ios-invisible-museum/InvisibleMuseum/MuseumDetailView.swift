import CoreLocation
import MapKit
import SwiftUI

struct MuseumDetailView: View {
    let space: MuseumSpace
    @Environment(MuseumStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    @State private var showingMapList = false
    @State private var locationService = MuseumLocationService()
    @State private var readerExhibit: Exhibit?

    var body: some View {
        MuseumScreen(bottomInset: 36, topInset: 0) {
            banner
            summary
            actions
            exhibits
            contributors
        }
        .toolbar(.hidden, for: .navigationBar)
        .overlay(alignment: .topLeading) { backButton }
        .task { locationService.start() }
        .fullScreenCover(isPresented: $showingMapList) {
            MapListView(initialSelectedSpace: space, onClose: { showingMapList = false })
        }
        .sheet(item: $readerExhibit) { ExhibitDetailView(space: space, exhibit: $0).presentationDetents([.medium, .large]).presentationDragIndicator(.visible).presentationBackground(.ultraThinMaterial) }
    }

    private var banner: some View {
        ZStack(alignment: .bottomLeading) {
            RoundedRectangle(cornerRadius: Museum.Radius.xl, style: .continuous)
                .fill(Museum.accentGradient.opacity(0.56))
                .overlay(alignment: .center) {
                    Image(systemName: space.kind.symbol)
                        .font(.system(size: 132, weight: .ultraLight))
                        .foregroundStyle(.white.opacity(0.20))
                }
                .overlay(alignment: .bottom) {
                    LinearGradient(colors: [.clear, Museum.ink], startPoint: .top, endPoint: .bottom)
                        .frame(height: 160)
                }

            VStack(alignment: .leading, spacing: 8) {
                Overline(text: space.kind.label, color: .white.opacity(0.82))
                Text(space.name)
                    .font(Museum.title(34))
                    .foregroundStyle(Museum.text)
                Text(space.tagline)
                    .font(Museum.bodyFont(15))
                    .foregroundStyle(Museum.text.opacity(0.72))
            }
            .padding(22)
        }
        .frame(height: 320)
        .overlay(RoundedRectangle(cornerRadius: Museum.Radius.xl, style: .continuous).stroke(Museum.border, lineWidth: 1))
    }

    private var backButton: some View {
        Button { dismiss() } label: {
            Image(systemName: "chevron.left")
                .font(.system(size: 16, weight: .bold))
                .foregroundStyle(Museum.text)
                .frame(width: 42, height: 42)
                .background(.black.opacity(0.42), in: Circle())
                .overlay(Circle().stroke(Museum.border, lineWidth: 1))
        }
        .padding(.leading, 16)
        .padding(.top, 54)
    }

    private var summary: some View {
        let liveDistanceText: String
        switch currentProximity {
        case .tooFar(let distanceMeters, _), .nearby(let distanceMeters, _), .inside(let distanceMeters):
            liveDistanceText = GeoMath.formatDistance(distanceMeters)
        case .unknown:
            liveDistanceText = GeoMath.formatDistance(space.distanceMeters)
        }

        return VStack(alignment: .leading, spacing: 12) {
            Text(space.summary)
                .font(Museum.bodyFont(15))
                .lineSpacing(4)
                .foregroundStyle(Museum.text.opacity(0.78))
            HStack(spacing: 10) {
                stat("\(space.exhibitCount)", "Exhibits", color: Museum.blue)
                stat("\(space.contributorCount)", "Contributors", color: Museum.purple)
                stat(liveDistanceText, "Away", color: Museum.green)
            }
        }
    }

    private func stat(_ value: String, _ label: String, color: Color) -> some View {
        VStack(spacing: 4) {
            Text(value)
                .font(.system(size: 22, weight: .bold))
                .foregroundStyle(color)
            Text(label.uppercased())
                .font(.system(size: 10, weight: .bold))
                .tracking(1)
                .foregroundStyle(Museum.faint)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 14)
        .museumGlassCard(radius: Museum.Radius.md)
    }

    private var actions: some View {
        VStack(spacing: 10) {
            Button { showingMapList = true } label: {
                Label("Get Walking Directions", systemImage: "figure.walk")
            }
            .buttonStyle(.museum(.primary, size: .large))

            Button { openInAppleMaps() } label: {
                Label("Open in Apple Maps", systemImage: "map")
            }
            .buttonStyle(.museum(.secondary, size: .regular))

            Button { store.toggleDownloaded(space) } label: {
                Label(store.isDownloaded(space) ? "Available Offline" : "Download for Offline", systemImage: store.isDownloaded(space) ? "checkmark.circle.fill" : "arrow.down.circle")
            }
            .buttonStyle(.museum(.secondary, size: .regular))
        }
    }

    private var currentProximity: UserProximity {
        let userCoordinate = locationService.currentLocation?.coordinate
        let targetCoordinate = CLLocationCoordinate2D(latitude: space.latitude, longitude: space.longitude)
        return GeoMath.classify(user: userCoordinate, target: targetCoordinate, radiusMeters: GeoMath.arrivalRadiusMeters)
    }

    private func openInAppleMaps() {
        let placemark = MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: space.latitude, longitude: space.longitude))
        let item = MKMapItem(placemark: placemark)
        item.name = space.name
        item.openInMaps(launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeWalking])
    }

    private var exhibits: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Exhibits")
                .font(.system(size: 20, weight: .bold))
                .foregroundStyle(Museum.text)
            ForEach(space.exhibits) { exhibit in
                Button { readerExhibit = exhibit } label: {
                    HStack(spacing: 12) {
                        Image(systemName: exhibit.kind.symbol)
                            .foregroundStyle(space.accent)
                            .frame(width: 40, height: 40)
                            .background(Museum.surfaceElevated, in: Circle())
                        VStack(alignment: .leading, spacing: 3) {
                            Overline(text: exhibit.kind.label, color: space.accent)
                            Text(exhibit.title)
                                .font(.system(size: 15, weight: .semibold))
                                .foregroundStyle(Museum.text)
                            Text(exhibit.body)
                                .font(Museum.bodyFont(12))
                                .foregroundStyle(Museum.muted)
                                .lineLimit(2)
                        }
                        Spacer()
                    }
                    .padding(14)
                    .museumGlassCard(radius: Museum.Radius.md)
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var contributors: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(Museum.accentGradient)
                .frame(width: 44, height: 44)
                .overlay(Image(systemName: "person.2.fill").foregroundStyle(.white))
            VStack(alignment: .leading, spacing: 3) {
                Text("Community Contributions")
                    .font(.system(size: 15, weight: .bold))
                    .foregroundStyle(Museum.text)
                Text("Local residents keep this museum alive with memories and sources.")
                    .font(Museum.bodyFont(12))
                    .foregroundStyle(Museum.muted)
            }
        }
        .padding(16)
        .background(LinearGradient(colors: [Museum.blue.opacity(0.25), Museum.purple.opacity(0.20)], startPoint: .leading, endPoint: .trailing), in: RoundedRectangle(cornerRadius: Museum.Radius.lg, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Museum.Radius.lg).stroke(Museum.blue.opacity(0.22), lineWidth: 1))
    }
}
