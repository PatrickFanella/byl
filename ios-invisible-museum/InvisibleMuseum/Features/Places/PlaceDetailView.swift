import SwiftUI
import InvisibleMuseumKit

/// A place and every moment that happened there. When opened from a tour stop, the moment that
/// tour features comes first; the rest stay browsable underneath.
struct PlaceDetailView: View {
    @Environment(AppModel.self) private var model
    @State private var state: Loadable<PlaceDetail> = .idle

    let ref: String
    var featuredMomentID: String?

    var body: some View {
        MuseumScreen {
            switch state {
            case .idle, .loading:
                MuseumLoadingView()
            case .failed(let message):
                MuseumErrorView(message: message) { Task { await load() } }
            case .loaded(let place):
                header(place)
                moments(place)
            }
        }
        .task(id: ref) { await load() }
        .navigationBarTitleDisplayMode(.inline)
    }

    private func header(_ detail: PlaceDetail) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Overline(text: detail.place.kind.replacingOccurrences(of: "_", with: " "))
            Text(detail.place.name)
                .font(Museum.title(28))
                .foregroundStyle(Museum.text)
            if !detail.place.address.isEmpty {
                Label(detail.place.address, systemImage: "mappin.and.ellipse")
                    .font(Museum.bodyFont(13))
                    .foregroundStyle(Museum.muted)
            }
            if !detail.place.summary.isEmpty {
                Text(detail.place.summary)
                    .font(Museum.bodyFont(15))
                    .foregroundStyle(Museum.text.opacity(0.85))
            }
        }
    }

    @ViewBuilder
    private func moments(_ detail: PlaceDetail) -> some View {
        let featured = detail.moments.first { $0.id == featuredMomentID } ?? detail.primaryMoment
        let others = detail.moments.filter { $0.id != featured?.id }
        if let featured {
            VStack(alignment: .leading, spacing: Museum.Spacing.md) {
                MuseumSectionHeader(title: featured.title, eyebrow: featuredMomentID != nil ? "Featured on this tour" : "Primary moment")
                NavigationLink(value: MomentRoute(id: featured.id)) { MomentCard(moment: featured, prominent: true) }
                    .buttonStyle(.plain)
            }
        }
        if !others.isEmpty {
            VStack(alignment: .leading, spacing: Museum.Spacing.md) {
                MuseumSectionHeader(title: featured == nil ? "Moments here" : "Also at this place", eyebrow: "\(others.count) more")
                ForEach(others) { moment in
                    NavigationLink(value: MomentRoute(id: moment.id)) { MomentCard(moment: moment, prominent: false) }
                        .buttonStyle(.plain)
                }
            }
        }
        if featured == nil && others.isEmpty {
            MuseumEmptyView(title: "Nothing published here yet", message: "Curators are still writing this place's moments.", symbol: "book.closed")
        }
    }

    private func load() async {
        state = .loading
        do {
            state = .loaded(try await model.api.place(ref))
        } catch {
            state = .failed(error.museumMessage)
        }
    }
}

struct MomentCard: View {
    let moment: MomentSummary
    var prominent = false

    var body: some View {
        if prominent {
            MuseumHeroCard(imageURL: moment.heroMedia?.url, symbol: "photo.on.rectangle.angled", height: 200) {
                VStack(alignment: .leading, spacing: 6) {
                    if !moment.dateDisplay.isEmpty { Overline(text: moment.dateDisplay) }
                    Text(moment.title)
                        .font(Museum.title(21))
                        .foregroundStyle(Museum.text)
                    Text(moment.excerpt)
                        .font(Museum.bodyFont(13))
                        .foregroundStyle(Museum.text.opacity(0.85))
                        .lineLimit(3)
                    if moment.hasNarration {
                        Label("Audio guide", systemImage: "waveform")
                            .font(Museum.meta(10))
                            .foregroundStyle(Museum.text.opacity(0.75))
                    }
                }
            }
        } else {
            MuseumCard(padding: Museum.Spacing.md) {
                VStack(alignment: .leading, spacing: 4) {
                    if !moment.dateDisplay.isEmpty {
                        Text(moment.dateDisplay)
                            .font(Museum.meta(11))
                            .foregroundStyle(Museum.action)
                    }
                    Text(moment.title)
                        .font(Museum.bodyFont(16, weight: .semibold))
                        .foregroundStyle(Museum.text)
                    Text(moment.excerpt)
                        .font(Museum.bodyFont(13))
                        .foregroundStyle(Museum.muted)
                        .lineLimit(2)
                }
            }
        }
    }
}
