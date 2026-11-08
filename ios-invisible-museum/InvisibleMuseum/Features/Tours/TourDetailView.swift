import SwiftUI
import InvisibleMuseumKit

/// One tour: summary, per-mode estimates, save, start, and the stops with their featured moments.
struct TourDetailView: View {
    @Environment(AppModel.self) private var model
    @State private var detail: TourDetailModel?

    let ref: String

    var body: some View {
        MuseumScreen {
            if let detail {
                TourDetailContent(detail: detail)
            } else {
                MuseumLoadingView()
            }
        }
        .task(id: ref) {
            let m = TourDetailModel(api: model.api, ref: ref, isSaved: model.savedTourIDs.contains(ref))
            detail = m
            await m.load()
            if let id = m.tour?.tour.id { m.markSaved(model.savedTourIDs.contains(id)) }
        }
        .navigationBarTitleDisplayMode(.inline)
    }
}

private struct TourDetailContent: View {
    @Environment(AppModel.self) private var model
    let detail: TourDetailModel
    @State private var startBusy = false
    @State private var startError: String?

    var body: some View {
        switch detail.state {
        case .idle, .loading:
            MuseumLoadingView(text: "Opening the gallery…")
        case .failed(let message):
            MuseumErrorView(message: message) { Task { await detail.load() } }
        case .loaded:
            if let tour = detail.tour {
                header(tour)
                modes(tour)
                ForEach(tour.tour.modes, id: \.self) { mode in
                    TripJourneyView(tour: tour, mode: mode, journey: tour.journeys?[mode])
                }
                actions(tour)
                stops(tour)
            }
        }
    }

    private func header(_ tour: TourDetail) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            TourCard(tour: tour.tour)
            if !tour.tour.summary.isEmpty {
                Text(tour.tour.summary)
                    .font(Museum.bodyFont(15))
                    .foregroundStyle(Museum.muted)
            }
            if tour.tour.isUserRoute {
                Label("A visitor-made route", systemImage: "person.crop.circle")
                    .font(Museum.meta(11))
                    .foregroundStyle(Museum.faint)
            }
        }
    }

    private func modes(_ tour: TourDetail) -> some View {
        MuseumCard(padding: Museum.Spacing.md) {
            VStack(alignment: .leading, spacing: 10) {
                Overline(text: "Getting around")
                ForEach(tour.tour.modes, id: \.self) { mode in
                    HStack {
                        Label(mode == "bike" ? "Biking" : "Walking", systemImage: mode == "bike" ? "bicycle" : "figure.walk")
                            .font(Museum.bodyFont(15, weight: .semibold))
                            .foregroundStyle(Museum.text)
                        Spacer()
                        if let estimate = detail.estimateText(mode: mode) {
                            Text([estimate, detail.distanceText(mode: mode)].compactMap { $0 }.joined(separator: " · "))
                                .font(Museum.bodyFont(13))
                                .foregroundStyle(Museum.muted)
                        } else if tour.tour.routes[mode]?.status == "failed" {
                            Text("Route unavailable")
                                .font(Museum.bodyFont(13))
                                .foregroundStyle(Museum.danger)
                        } else {
                            Text("Route being computed")
                                .font(Museum.bodyFont(13))
                                .foregroundStyle(Museum.faint)
                        }
                    }
                }
            }
        }
    }

    private func actions(_ tour: TourDetail) -> some View {
        VStack(spacing: 10) {
            if tour.tour.status != "published" {
                Text("Draft · Only you can open this route. Its path needs review before publication or sharing.")
                    .font(Museum.bodyFont(14))
                    .foregroundStyle(Museum.muted)
            } else {
                ForEach(tour.tour.modes, id: \.self) { mode in
                    Button {
                        startBusy = true
                        startError = nil
                        Task {
                            defer { startBusy = false }
                            do { try await model.startTour(tour, mode: mode) }
                            catch { startError = error.museumMessage }
                        }
                    } label: {
                        Label(mode == "bike" ? "Start riding" : "Start walking", systemImage: mode == "bike" ? "bicycle" : "figure.walk")
                    }
                    .buttonStyle(.museum(mode == tour.tour.modes.first ? .primary : .secondary, size: .large))
                    .disabled(startBusy || !detail.canStart(mode: mode))
                }
                if tour.tour.revisionID == nil {
                    MuseumInlineHint(text: "A reviewed revision is required before starting this tour.")
                }
                if let revisionID = tour.tour.revisionID {
                    ForEach(tour.tour.modes, id: \.self) { mode in
                        NavigationLink(mode == "bike" ? "Suggest a cycling route change" : "Suggest a walking route change") {
                            RouteReportEntryView(tourID: tour.id, revisionID: revisionID, mode: mode)
                        }.buttonStyle(.museum(.secondary))
                    }
                }
                if let startError { MuseumInlineHint(text: startError, tint: Museum.danger, isError: true) }
                Button {
                    if model.isSignedIn {
                        Task {
                            await detail.toggleSave()
                            if detail.saveError == nil, let id = detail.tour?.tour.id { model.setSaved(id, detail.isSaved) }
                        }
                    } else {
                        model.presentSignIn = true
                    }
                } label: {
                    Label(detail.isSaved ? "Saved" : "Save for later", systemImage: detail.isSaved ? "heart.fill" : "heart")
                }
                .buttonStyle(.museum(.ghost))
                .disabled(detail.saveBusy)
            }
            if let error = detail.saveError {
                MuseumInlineHint(text: error, tint: Museum.danger, isError: true)
            }
        }
    }

    private func stops(_ tour: TourDetail) -> some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.md) {
            MuseumSectionHeader(title: "Stops", eyebrow: "\(tour.stops.count) places")
            if tour.stops.isEmpty {
                MuseumEmptyView(title: "No stops yet", message: "This tour has no published places.", symbol: "mappin.slash")
            }
            ForEach(tour.stops) { stop in
                NavigationLink(value: PlaceRoute(id: stop.place.id, featuredMomentID: stop.featuredMoment?.id ?? stop.place.primaryMomentID)) {
                    StopRow(stop: stop)
                }
                .buttonStyle(.plain)
            }
        }
    }
}

struct StopRow: View {
    let stop: TourStop

    var body: some View {
        MuseumCard(padding: Museum.Spacing.md) {
            HStack(alignment: .top, spacing: 14) {
                Text("\(stop.position)")
                    .font(Museum.meta(13))
                    .foregroundStyle(Museum.actionText)
                    .frame(width: 30, height: 30)
                    .background(Museum.action, in: Circle())
                VStack(alignment: .leading, spacing: 4) {
                    Text(stop.place.name)
                        .font(Museum.bodyFont(16, weight: .semibold))
                        .foregroundStyle(Museum.text)
                    if let moment = stop.featuredMoment {
                        Text(moment.title)
                            .font(Museum.bodyFont(14))
                            .foregroundStyle(Museum.text.opacity(0.85))
                        if !moment.dateDisplay.isEmpty {
                            Text(moment.dateDisplay)
                                .font(Museum.meta(11))
                                .foregroundStyle(Museum.action)
                        }
                    } else {
                        Text("No moment published here yet")
                            .font(Museum.bodyFont(13))
                            .foregroundStyle(Museum.faint)
                    }
                    if stop.place.momentCount > 1 {
                        Text("+\(stop.place.momentCount - 1) more at this place")
                            .font(Museum.bodyFont(12))
                            .foregroundStyle(Museum.muted)
                    }
                    if !stop.note.isEmpty {
                        Text(stop.note)
                            .font(Museum.bodyFont(12))
                            .italic()
                            .foregroundStyle(Museum.muted)
                    }
                }
                Spacer(minLength: 0)
            }
        }
    }
}
