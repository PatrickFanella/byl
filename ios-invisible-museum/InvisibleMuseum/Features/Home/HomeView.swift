import SwiftUI
import CoreLocation
import InvisibleMuseumKit

/// City picker, featured tours, and places near the visitor.
struct HomeView: View {
    @Environment(AppModel.self) private var model
    @State private var tours: Loadable<[TourSummary]> = .idle
    @State private var nearby: Loadable<[PlaceSummary]> = .idle
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            MuseumScreen {
                header
                if let error = model.bootstrapError, model.cities.isEmpty {
                    MuseumErrorView(message: error) { Task { await model.loadCities() } }
                } else if model.selectedCity == nil {
                    MuseumLoadingView(text: "Finding your city…")
                } else {
                    toursSection
                    nearbySection
                }
            }
            .museumDestinations()
            .task(id: model.selectedCity?.id) { await loadTours() }
            .task(id: nearbyKey) { await loadNearby() }
            .onAppear { model.location.requestWhenInUseAuthorization(); model.location.start() }
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 10) {
            Image("BylLockup")
                .resizable().scaledToFit().frame(width: 134, height: 48)
                .padding(6)
                .accessibilityLabel("Byl")
            Text(model.selectedCity.map { "\($0.name) on foot" } ?? "Local history, on foot or by bike.")
                .font(Museum.title(30))
                .foregroundStyle(Museum.text)
            Text("Walk or ride through the stories that happened right where you are standing.")
                .font(Museum.bodyFont(15))
                .foregroundStyle(Museum.muted)
            if model.cities.count > 1 {
                CityPicker()
            }
        }
    }

    private var toursSection: some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.md) {
            MuseumSectionHeader(title: "Tours", eyebrow: "By topic")
            switch tours {
            case .idle, .loading:
                MuseumLoadingView()
            case .failed(let message):
                MuseumErrorView(message: message) { Task { await loadTours() } }
            case .loaded(let items) where items.isEmpty:
                MuseumEmptyView(title: "No tours yet", message: "Curated tours for this city are still being written.", symbol: "figure.walk")
            case .loaded(let items):
                ForEach(items) { tour in
                    NavigationLink(value: TourRef(ref: tour.slug)) { TourCard(tour: tour) }
                        .buttonStyle(.plain)
                }
            }
        }
    }

    private var nearbySection: some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.md) {
            MuseumSectionHeader(title: "Near you", eyebrow: "Places")
            switch nearby {
            case .idle:
                if model.location.authorizationState == .denied {
                    MuseumEmptyView(title: "Location is off", message: "Allow location access in Settings to see the places around you.", symbol: "location.slash")
                } else {
                    MuseumLoadingView(text: "Waiting for your location…")
                }
            case .loading:
                MuseumLoadingView()
            case .failed(let message):
                MuseumErrorView(message: message) { Task { await loadNearby() } }
            case .loaded(let items) where items.isEmpty:
                MuseumEmptyView(title: "Nothing within 2 km", message: "Pick a tour to travel to its first stop.", symbol: "mappin.slash")
            case .loaded(let items):
                ForEach(items.prefix(8)) { place in
                    NavigationLink(value: PlaceRoute(id: place.id, featuredMomentID: nil)) { PlaceRow(place: place) }
                        .buttonStyle(.plain)
                }
            }
        }
    }

    /// Re-run the nearby query when the city changes or the user moves more than ~100 m.
    private var nearbyKey: String {
        let city = model.selectedCity?.slug ?? ""
        guard let loc = model.location.currentLocation else { return city }
        return "\(city)|\(Int(loc.coordinate.latitude * 1000))|\(Int(loc.coordinate.longitude * 1000))"
    }

    private func loadTours() async {
        guard let city = model.selectedCity else { return }
        tours = .loading
        do {
            tours = .loaded(try await model.api.tours(city: city.slug))
        } catch {
            tours = .failed(error.museumMessage)
        }
    }

    private func loadNearby() async {
        guard let city = model.selectedCity, let loc = model.location.currentLocation else {
            nearby = .idle
            return
        }
        nearby = .loading
        do {
            let places = try await model.api.places(city: city.slug, near: (loc.coordinate.latitude, loc.coordinate.longitude), radiusM: 2000)
            nearby = .loaded(places)
        } catch {
            nearby = .failed(error.museumMessage)
        }
    }
}

struct CityPicker: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Menu {
            ForEach(model.cities) { city in
                Button(city.name) { model.select(city: city) }
            }
        } label: {
            MuseumChip(title: model.selectedCity?.name ?? "Choose a city", isSelected: true, icon: "building.2")
        }
    }
}

/// Navigation targets shared by every stack: tours, places (optionally with the moment a tour features), moments.
struct TourRef: Hashable {
    let ref: String
}

struct PlaceRoute: Hashable {
    let id: String
    let featuredMomentID: String?
}

struct MomentRoute: Hashable {
    let id: String
}

extension View {
    func museumDestinations() -> some View {
        self
            .navigationDestination(for: TourRef.self) { TourDetailView(ref: $0.ref) }
            .navigationDestination(for: PlaceRoute.self) { PlaceDetailView(ref: $0.id, featuredMomentID: $0.featuredMomentID) }
            .navigationDestination(for: MomentRoute.self) { MomentView(id: $0.id) }
            .toolbarBackground(.hidden, for: .navigationBar)
    }
}

struct TourCard: View {
    let tour: TourSummary

    var body: some View {
        MuseumHeroCard(imageURL: tour.heroMedia?.url, symbol: tour.isUserRoute ? "point.topleft.down.to.point.bottomright.curvepath" : "building.columns", height: 180) {
            VStack(alignment: .leading, spacing: 6) {
                if let topic = tour.topic {
                    Overline(text: topic.name)
                }
                Text(tour.title)
                    .font(Museum.title(21))
                    .foregroundStyle(Museum.text)
                    .lineLimit(2)
                HStack(spacing: 10) {
                    Label("\(tour.stopCount) stops", systemImage: "mappin")
                    ForEach(tour.modes, id: \.self) { mode in
                        if let route = tour.routes[mode], let text = TourDetailModel.estimateText(for: route) {
                            Label(text, systemImage: mode == "bike" ? "bicycle" : "figure.walk")
                        }
                    }
                }
                .font(Museum.meta(11))
                .foregroundStyle(Museum.text.opacity(0.8))
            }
        }
    }
}

struct PlaceRow: View {
    let place: PlaceSummary

    var body: some View {
        MuseumCard(padding: Museum.Spacing.md) {
            HStack(spacing: 12) {
                Image(systemName: "mappin.circle.fill")
                    .font(.system(size: 26))
                    .foregroundStyle(Museum.action)
                VStack(alignment: .leading, spacing: 3) {
                    Text(place.name)
                        .font(Museum.bodyFont(15, weight: .semibold))
                        .foregroundStyle(Museum.text)
                    Text(placeMeta)
                        .font(Museum.bodyFont(12))
                        .foregroundStyle(Museum.muted)
                }
                Spacer()
                Image(systemName: "chevron.right")
                    .font(.system(size: 12, weight: .bold))
                    .foregroundStyle(Museum.faint)
            }
        }
    }

    private var placeMeta: String {
        var parts: [String] = []
        if let d = place.distanceM { parts.append(GeoMath.formatDistance(d)) }
        parts.append(place.momentCount == 1 ? "1 moment" : "\(place.momentCount) moments")
        return parts.joined(separator: " · ")
    }
}
