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
                HappeningsSection(placeID: place.place.id, origin: model.client.baseURL)
                    .id("\(model.client.baseURL.absoluteString)|\(place.place.id)")
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

/// User-opened current information with no dependency on historical loading or
/// active-trip state. No cache, background discovery, narration or progress writes.
@MainActor
struct HappeningsSection: View {
    let placeID: String
    let origin: URL
    var tripContext = false
    @State private var enabled = false
    @State private var busy = false
    @State private var items: [Happening] = []
    @State private var cursor = ""
    @State private var message = ""
    @State private var generation = 0
    @State private var request: Task<Void, Never>?

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button(enabled ? "Hide current notices" : "Show current notices") {
                enabled.toggle()
                if enabled { load() } else { stop() }
            }.buttonStyle(.museum(.secondary))
            if enabled {
                Text("Current announcements are separate from \(tripContext ? "this started historical itinerary" : "the historical stories"). Dates do not provide an exact start time. Confirm plans with the organizer.")
                    .font(Museum.bodyFont(13)).foregroundStyle(Museum.muted)
                if !message.isEmpty { Text(message).font(Museum.bodyFont(14)) }
                if !busy && message.isEmpty && items.isEmpty { Text("No current notices have been published here.").font(Museum.bodyFont(14)) }
                TimelineView(.periodic(from: .now, by: 60)) { context in
                    VStack(alignment: .leading, spacing: 16) {
                        ForEach(items) { notice in card(notice, now: context.date) }
                    }
                }
                if busy { ProgressView("Loading current notices…") }
                Button("Refresh current notices") { load() }.buttonStyle(.museum(.secondary)).disabled(busy)
                if !cursor.isEmpty { Button("Show more notices") { load(more: true) }.buttonStyle(.museum(.secondary)).disabled(busy) }
            }
        }
        .padding(16)
        .overlay(RoundedRectangle(cornerRadius: 4).stroke(Museum.border, lineWidth: 1))
        .onDisappear { stop() }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Optional current notices")
    }

    private func card(_ notice: Happening, now: Date) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Divider()
            Text(notice.presentationStatus(now: now).label).font(Museum.bodyFont(14, weight: .semibold))
            Text(notice.title).font(Museum.title(18))
            Text(notice.occurrenceText).font(Museum.bodyFont(13))
            Text(notice.summary).font(Museum.bodyFont(14))
            Text("Organizer: \(notice.organizer)").font(Museum.bodyFont(13))
            Text("\(notice.checkedText). Source status: \(notice.sourceHealth.replacingOccurrences(of: "_", with: " ")).").font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
            Text("Review by \(notice.reviewOn); expires \(notice.expiresOn).").font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
            if notice.sources.isEmpty { Text("Source attribution unavailable.").font(Museum.bodyFont(12)) }
            ForEach(Array(notice.sources.enumerated()), id: \.offset) { _, source in
                if let url = Happening.sourceURL(source.url) {
                    Link(source.title.isEmpty ? (source.publisher.isEmpty ? "Organizer announcement" : source.publisher) : source.title, destination: url)
                        .font(Museum.bodyFont(13)).foregroundStyle(Museum.action)
                } else { Text("Source link unavailable").font(Museum.bodyFont(13)) }
                Text("\(source.publisher) · captured \(source.retrievedOn) · media reuse \(source.license)").font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
            }
        }
    }

    private func stop() { generation += 1; request?.cancel(); request = nil; enabled = false; busy = false; items = []; cursor = ""; message = "" }
    private func load(more: Bool = false) {
        generation += 1
        let current = generation
        let next = more ? cursor : ""
        request?.cancel(); busy = true; message = ""
        if !more { items = []; cursor = "" }
        // Public notices never carry a bearer or call the app's auth handler.
        let api = Endpoints(client: APIClient(baseURL: origin))
        request = Task {
            do {
                let page = try await api.happenings(place: placeID, cursor: next)
                guard !Task.isCancelled, generation == current, enabled else { return }
                let n = Int(page.nextCursor)
                guard page.items.count <= 50, page.items.allSatisfy({ $0.placeID == placeID }), page.nextCursor.isEmpty || (page.nextCursor.allSatisfy(\.isNumber) && n != nil && n! > (Int(next) ?? 0) && n! <= 10000) else { throw NoticeFailure.invalidPage }
                var seen = Set<String>()
                items = ((more ? items : []) + page.items).filter { seen.insert($0.id).inserted }
                cursor = page.nextCursor
            } catch {
                guard !Task.isCancelled, generation == current, enabled else { return }
                items = []; cursor = ""; message = "Current notices are unavailable. The historical story and tour still work."
            }
            if generation == current { busy = false }
        }
    }
    private enum NoticeFailure: Error { case invalidPage }
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
