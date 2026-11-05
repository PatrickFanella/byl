import SwiftUI
import InvisibleMuseumKit

/// Tours the visitor saved, with how far along each one they are.
struct SavedView: View {
    @Environment(AppModel.self) private var model
    @State private var saved: Loadable<[TourSummary]> = .idle
    @State private var progress: [String: TourProgress] = [:]

    var body: some View {
        NavigationStack {
            MuseumScreen {
                VStack(alignment: .leading, spacing: 10) {
                    Overline(text: "Saved")
                    Text("Tours to come back to")
                        .font(Museum.title(30))
                        .foregroundStyle(Museum.text)
                }
                if !model.isSignedIn {
                    SignInPrompt(reason: "Sign in to save tours and keep your progress on every device.")
                } else {
                    content
                }
            }
            .museumDestinations()
            .task(id: "\(model.isSignedIn)|\(model.savedTourIDs.count)") { await load() }
            .refreshable { await load() }
        }
    }

    @ViewBuilder
    private var content: some View {
        switch saved {
        case .idle, .loading:
            MuseumLoadingView()
        case .failed(let message):
            MuseumErrorView(message: message) { Task { await load() } }
        case .loaded(let items) where items.isEmpty:
            MuseumEmptyView(title: "Nothing saved yet", message: "Tap Save for later on any tour.", symbol: "heart")
        case .loaded(let items):
            ForEach(items) { tour in
                NavigationLink(value: TourRef(ref: tour.slug)) {
                    VStack(alignment: .leading, spacing: 6) {
                        TourCard(tour: tour)
                        if let p = progress[tour.id] {
                            HStack(spacing: 8) {
                                ProgressView(value: Double(p.visitedPositions.count), total: Double(max(tour.stopCount, 1)))
                                    .tint(p.completedAt == nil ? Museum.blue : Museum.green)
                                Text(p.completedAt == nil ? "\(p.visitedPositions.count) of \(tour.stopCount) stops" : "Completed")
                                    .font(Museum.meta(11))
                                    .foregroundStyle(Museum.muted)
                                    .fixedSize()
                            }
                            .padding(.horizontal, 4)
                        }
                    }
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func load() async {
        guard model.isSignedIn else { saved = .idle; return }
        saved = .loading
        do {
            async let tours = model.api.saved()
            async let records = model.api.progress()
            let (t, r) = try await (tours, records)
            // Keep the most advanced record per tour across modes.
            var best: [String: TourProgress] = [:]
            for rec in r where (best[rec.tourID]?.visitedPositions.count ?? -1) < rec.visitedPositions.count {
                best[rec.tourID] = rec
            }
            progress = best
            saved = .loaded(t)
        } catch {
            saved = .failed(error.museumMessage)
        }
    }
}
