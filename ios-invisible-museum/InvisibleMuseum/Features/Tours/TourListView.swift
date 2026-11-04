import SwiftUI
import InvisibleMuseumKit

/// Every tour in the selected city, filtered by topic and travel mode. Also the landing
/// point for shared tour links (`model.pendingTourSlug`).
struct TourListView: View {
    @Environment(AppModel.self) private var model
    @State private var tours: Loadable<[TourSummary]> = .idle
    @State private var topic: Topic?
    @State private var mode: String?
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            MuseumScreen {
                VStack(alignment: .leading, spacing: 10) {
                    Overline(text: model.selectedCity?.name ?? "Tours")
                    Text("Tours")
                        .font(Museum.title(30))
                        .foregroundStyle(Museum.text)
                    Text("Each tour is a gallery: one topic, a handful of places, and the moment at each that belongs to the story.")
                        .font(Museum.bodyFont(15))
                        .foregroundStyle(Museum.muted)
                }
                filters
                content
            }
            .museumDestinations()
            .task(id: filterKey) { await load() }
            .onChange(of: model.pendingTourSlug, initial: true) { _, slug in
                guard let slug else { return }
                path.append(TourRef(ref: slug))
                model.pendingTourSlug = nil
            }
        }
    }

    private var filters: some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.sm) {
            if let topics = model.selectedCity?.topics, !topics.isEmpty {
                ScrollView(.horizontal) {
                    HStack(spacing: 8) {
                        Button { topic = nil } label: { MuseumChip(title: "All topics", isSelected: topic == nil) }
                        ForEach(topics) { t in
                            Button { topic = topic?.id == t.id ? nil : t } label: { MuseumChip(title: t.name, isSelected: topic?.id == t.id) }
                        }
                    }
                }
                .scrollIndicators(.hidden)
                .buttonStyle(.plain)
            }
            HStack(spacing: 8) {
                Button { mode = nil } label: { MuseumChip(title: "Any", isSelected: mode == nil) }
                Button { mode = mode == "walk" ? nil : "walk" } label: { MuseumChip(title: "Walking", isSelected: mode == "walk", icon: "figure.walk") }
                Button { mode = mode == "bike" ? nil : "bike" } label: { MuseumChip(title: "Biking", isSelected: mode == "bike", icon: "bicycle") }
            }
            .buttonStyle(.plain)
        }
    }

    @ViewBuilder
    private var content: some View {
        switch tours {
        case .idle, .loading:
            MuseumLoadingView()
        case .failed(let message):
            MuseumErrorView(message: message) { Task { await load() } }
        case .loaded(let items) where items.isEmpty:
            MuseumEmptyView(title: "No tours match", message: topic == nil && mode == nil ? "Curated tours for this city are still being written." : "Try another topic or travel mode.", symbol: "figure.walk")
        case .loaded(let items):
            ForEach(items) { tour in
                NavigationLink(value: TourRef(ref: tour.slug)) { TourCard(tour: tour) }
                    .buttonStyle(.plain)
            }
        }
    }

    private var filterKey: String { "\(model.selectedCity?.slug ?? "")|\(topic?.slug ?? "")|\(mode ?? "")" }

    private func load() async {
        guard let city = model.selectedCity else { return }
        tours = .loading
        do {
            tours = .loaded(try await model.api.tours(city: city.slug, topic: topic?.slug, mode: mode))
        } catch {
            tours = .failed(error.museumMessage)
        }
    }
}
