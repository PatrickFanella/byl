import SwiftUI
import InvisibleMuseumKit

/// A moment in full: date, media, body, topics, and the sources it rests on.
struct MomentView: View {
    @Environment(AppModel.self) private var model
    @State private var state: Loadable<MomentDetail> = .idle

    let id: String

    var body: some View {
        MuseumScreen {
            switch state {
            case .idle, .loading:
                MuseumLoadingView()
            case .failed(let message):
                MuseumErrorView(message: message) { Task { await load() } }
            case .loaded(let moment):
                header(moment)
                if !moment.media.isEmpty {
                    MediaGalleryView(media: moment.media)
                }
                bodyText(moment)
                if !moment.topics.isEmpty {
                    topics(moment)
                }
                sources(moment)
            }
        }
        .task(id: id) { await load() }
        .navigationBarTitleDisplayMode(.inline)
    }

    private func header(_ moment: MomentDetail) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            if !moment.summary.dateDisplay.isEmpty {
                Overline(text: moment.summary.dateDisplay)
            }
            Text(moment.summary.title)
                .font(Museum.title(28))
                .foregroundStyle(Museum.text)
            if moment.summary.hasNarration || moment.narration != nil {
                Label("Audio guide available on the tour", systemImage: "waveform")
                    .font(Museum.meta(11))
                    .foregroundStyle(Museum.muted)
            }
        }
    }

    private func bodyText(_ moment: MomentDetail) -> some View {
        MuseumCard {
            Text(markdown(moment.body.isEmpty ? moment.summary.excerpt : moment.body))
                .font(Museum.bodyFont(16))
                .foregroundStyle(Museum.text.opacity(0.9))
                .lineSpacing(4)
                .textSelection(.enabled)
        }
    }

    private func topics(_ moment: MomentDetail) -> some View {
        MuseumFlowLayout {
            ForEach(moment.topics) { topic in
                MuseumChip(title: topic.name, icon: "tag")
            }
        }
    }

    private func sources(_ moment: MomentDetail) -> some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.md) {
            MuseumSectionHeader(title: "Sources", eyebrow: "Where this comes from")
            if moment.sources.isEmpty {
                MuseumEmptyView(title: "No sources listed", message: "This moment was written without a cited source.", symbol: "questionmark.circle")
            }
            ForEach(Array(moment.sources.enumerated()), id: \.offset) { _, source in
                SourceRow(source: source)
            }
        }
    }

    private func markdown(_ text: String) -> AttributedString {
        (try? AttributedString(markdown: text, options: .init(interpretedSyntax: .full))) ?? AttributedString(text)
    }

    private func load() async {
        state = .loading
        do {
            state = .loaded(try await model.api.moment(id))
        } catch {
            state = .failed(error.museumMessage)
        }
    }
}

struct SourceRow: View {
    let source: Source

    var body: some View {
        MuseumCard(padding: Museum.Spacing.md) {
            VStack(alignment: .leading, spacing: 4) {
                if let url = URL(string: source.url), !source.url.isEmpty {
                    Link(destination: url) {
                        Label(source.title.isEmpty ? source.url : source.title, systemImage: "arrow.up.right.square")
                            .font(Museum.bodyFont(14, weight: .semibold))
                            .foregroundStyle(Museum.action)
                    }
                } else {
                    Text(source.title)
                        .font(Museum.bodyFont(14, weight: .semibold))
                        .foregroundStyle(Museum.text)
                }
                Text([source.publisher, source.license, source.retrievedAt.map { "retrieved \($0)" }].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · "))
                    .font(Museum.bodyFont(12))
                    .foregroundStyle(Museum.muted)
                if !source.note.isEmpty {
                    Text(source.note)
                        .font(Museum.bodyFont(12))
                        .italic()
                        .foregroundStyle(Museum.faint)
                }
            }
        }
    }
}
