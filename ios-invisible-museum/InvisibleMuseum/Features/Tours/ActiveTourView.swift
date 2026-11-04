import SwiftUI
import InvisibleMuseumKit

/// Walking or riding mode: the map, the next stop with distance, arrival by GPS or by hand,
/// Apple Maps handoff, the audio toggle, and progress.
struct ActiveTourView: View {
    @Environment(AppModel.self) private var model
    let active: ActiveTourModel
    @State private var showEndConfirm = false
    @State private var path = NavigationPath()

    var body: some View {
        NavigationStack(path: $path) {
            ZStack(alignment: .bottom) {
                TourMapView(model: active)
                    .ignoresSafeArea(edges: .bottom)
                VStack(spacing: 10) {
                    if case .failed(let message) = active.routeState {
                        routeNotice(message)
                    } else if active.routeFromCache {
                        Label("Offline: using the saved route", systemImage: "wifi.slash")
                            .font(Museum.meta(11))
                            .foregroundStyle(Museum.muted)
                            .padding(.horizontal, 12).padding(.vertical, 6)
                            .museumGlassCard(radius: Museum.Radius.pill)
                    }
                    bottomCard
                }
                .padding(.horizontal, Museum.Spacing.pageHorizontal)
                .padding(.bottom, Museum.Spacing.md)
            }
            .background(Museum.background)
            .navigationTitle(active.tour.tour.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("End") { showEndConfirm = true }
                        .foregroundStyle(Museum.text)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        active.audioEnabled.toggle()
                    } label: {
                        Image(systemName: active.audioEnabled ? "speaker.wave.2.fill" : "speaker.slash")
                    }
                    .foregroundStyle(Museum.text)
                    .accessibilityLabel(active.audioEnabled ? "Audio guide on" : "Audio guide off")
                }
            }
            .museumDestinations()
            .sheet(item: Binding(get: { active.arrivedStop }, set: { active.arrivedStop = $0 })) { stop in
                StopMomentSheet(stop: stop, isLast: active.isComplete)
                    .environment(model)
                    .presentationDetents([.medium, .large])
                    .presentationBackground(Museum.background)
            }
            .confirmationDialog("End this tour?", isPresented: $showEndConfirm, titleVisibility: .visible) {
                Button("Keep my progress and end") { model.endTour(resetProgress: false) }
                Button("Start over next time", role: .destructive) { model.endTour(resetProgress: true) }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text(active.progressText)
            }
            .task { await active.loadRoute() }
            .onAppear {
                model.location.requestWhenInUseAuthorization()
                model.location.start()
            }
        }
        .preferredColorScheme(.dark)
    }

    private func routeNotice(_ message: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle")
            Text("Route unavailable: \(message). Stops still work; use Open in Maps for directions.")
                .lineLimit(2)
            Spacer(minLength: 0)
            Button("Retry") { Task { await active.loadRoute() } }
        }
        .font(Museum.bodyFont(12))
        .foregroundStyle(Museum.text)
        .padding(12)
        .museumGlassCard(radius: Museum.Radius.md)
    }

    @ViewBuilder
    private var bottomCard: some View {
        if let next = active.nextStop {
            MuseumCard(fill: Museum.surface.opacity(0.96)) {
                VStack(alignment: .leading, spacing: 12) {
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 4) {
                            Overline(text: "Next · stop \(next.position) of \(active.tour.stops.count)")
                            Text(next.place.name)
                                .font(Museum.title(20))
                                .foregroundStyle(Museum.text)
                            if let moment = next.featuredMoment {
                                Text(moment.title)
                                    .font(Museum.bodyFont(13))
                                    .foregroundStyle(Museum.muted)
                                    .lineLimit(1)
                            }
                        }
                        Spacer()
                        VStack(alignment: .trailing, spacing: 2) {
                            Text(distanceLabel)
                                .font(Museum.title(18))
                                .foregroundStyle(Museum.text)
                            Text(active.progressText)
                                .font(Museum.meta(10))
                                .foregroundStyle(Museum.faint)
                        }
                    }
                    if model.location.authorizationState == .denied {
                        MuseumInlineHint(text: "Location is off. Tap I'm here at each stop, or allow location in Settings.", tint: Museum.muted)
                    }
                    HStack(spacing: 10) {
                        Button { active.markArrivedAtNextStop() } label: { Label("I'm here", systemImage: "checkmark.circle") }
                            .buttonStyle(.museum(.primary, size: .small))
                        Button { active.openNextStopInMaps() } label: { Label("Open in Maps", systemImage: "map") }
                            .buttonStyle(.museum(.secondary, size: .small))
                    }
                }
            }
        } else {
            MuseumCard(fill: Museum.surface.opacity(0.96)) {
                VStack(alignment: .leading, spacing: 10) {
                    Overline(text: "Tour complete", color: Museum.green)
                    Text("You walked every stop of \(active.tour.tour.title).")
                        .font(Museum.title(18))
                        .foregroundStyle(Museum.text)
                    if !model.isSignedIn {
                        Text("Sign in to keep this on your account.")
                            .font(Museum.bodyFont(13))
                            .foregroundStyle(Museum.muted)
                    }
                    Button("Done") { model.endTour(resetProgress: false) }
                        .buttonStyle(.museum(.primary, size: .small))
                }
            }
        }
    }

    private var distanceLabel: String {
        guard let d = active.distanceToNextM else { return "—" }
        return GeoMath.formatDistance(d)
    }
}
