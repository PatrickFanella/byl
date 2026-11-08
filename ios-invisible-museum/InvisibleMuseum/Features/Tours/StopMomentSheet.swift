import SwiftUI
import InvisibleMuseumKit

/// Shown on arrival: the moment this tour features at the stop, with the place's other moments
/// one tap away.
struct StopMomentSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var place: Loadable<PlaceDetail> = .idle

    let stop: TourStop
    var active: ActiveTourModel? = nil
    var isLast = false

    var body: some View {
        NavigationStack {
            MuseumScreen(bottomInset: 24) {
                VStack(alignment: .leading, spacing: 8) {
                    Overline(text: "You have arrived · stop \(stop.position)", color: Museum.success)
                    Text(stop.place.name)
                        .font(Museum.title(26))
                        .foregroundStyle(Museum.text)
                    if !stop.note.isEmpty {
                        Text(stop.note)
                            .font(Museum.bodyFont(14))
                            .italic()
                            .foregroundStyle(Museum.muted)
                    }
                }
                if let featured = stop.featuredMoment {
                    storyLink(featured) { MomentCard(moment: featured, prominent: true) }
                        .buttonStyle(.plain)
                    HStack(spacing: 10) {
                        storyLink(featured) {
                            Label("Read the full story", systemImage: "book")
                        }
                        .buttonStyle(.museum(.secondary))
                        Button {
                            if model.narration.isPlaying { model.narration.stop() } else { Task { await model.playNarration(for: stop) } }
                        } label: {
                            Label(model.narration.isPlaying ? "Stop" : "Listen", systemImage: model.narration.isPlaying ? "stop.fill" : "waveform")
                        }
                        .buttonStyle(.museum(.secondary))
                    }
                } else {
                    MuseumEmptyView(title: "No moment featured here", message: "This stop is a waypoint; look around and carry on.", symbol: "mappin")
                }
                HappeningsSection(placeID: stop.place.id, origin: active?.api.client.baseURL ?? model.client.baseURL, tripContext: true)
                    .id("\((active?.api.client.baseURL ?? model.client.baseURL).absoluteString)|\(stop.id)")
                others
                Button(isLast ? "Finish" : "Continue to the next stop") { dismiss() }
                    .buttonStyle(.museum(.primary, size: .large))
            }
            .museumDestinations()
            .task { await loadPlace() }
        }
    }

    @ViewBuilder
    private var others: some View {
        switch place {
        case .idle, .loading:
            if stop.place.momentCount > 1 { MuseumLoadingView(text: "Looking up the other moments here…") }
        case .failed:
            EmptyView()
        case .loaded(let detail):
            let rest = detail.moments.filter { $0.id != stop.featuredMoment?.id }
            if !rest.isEmpty {
                VStack(alignment: .leading, spacing: Museum.Spacing.md) {
                    MuseumSectionHeader(title: "Also at this place", eyebrow: "\(rest.count) more")
                    ForEach(rest) { moment in
                        NavigationLink(value: MomentRoute(id: moment.id)) { MomentCard(moment: moment) }
                            .buttonStyle(.plain)
                    }
                }
            }
        }
    }

    private func loadPlace() async {
        guard active?.revision == nil, active?.legacyResume != true, stop.place.momentCount > 1 else { return }
        place = .loading
        do {
            place = .loaded(try await model.api.place(stop.place.id))
        } catch {
            place = .failed(error.museumMessage)
        }
    }

    @ViewBuilder
    private func storyLink<Label: View>(_ moment: MomentSummary, @ViewBuilder label: () -> Label) -> some View {
        if let active, active.revision != nil || active.legacyResume {
            NavigationLink { MomentView(id: moment.id, retained: active.retainedMoment(moment.id), revisionOnly: true) } label: { label() }
        } else {
            NavigationLink(value: MomentRoute(id: moment.id)) { label() }
        }
    }
}
