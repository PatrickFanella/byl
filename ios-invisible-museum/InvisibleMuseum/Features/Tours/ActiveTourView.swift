import SwiftUI
import InvisibleMuseumKit

/// Walking or riding mode: the map, the next stop with distance, arrival by GPS or by hand,
/// Apple Maps handoff, the audio toggle, and progress.
struct ActiveTourView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.scenePhase) private var scenePhase
    @State private var showContributions = false
    let active: ActiveTourModel
    @State private var showEndConfirm = false
    @State private var path = NavigationPath()
    @State private var showTripReview = false
    @State private var confirmUpdate = false
    @State private var updateBusy = false
    @State private var updateError: String?
    @State private var showReport = false
    @State private var reviewAfterReport = false

    var body: some View {
        NavigationStack(path: $path) {
            Group {
                if active.revoked {
                    MuseumScreen {
                        MuseumInlineHint(text: "This revision is no longer available to this account. Its cached itinerary has been removed.", tint: Museum.danger, isError: true)
                        Button("End trip") { model.endTour(resetProgress: false) }
                            .buttonStyle(.museum(.primary))
                    }
                } else {
                    currentTrip
                }
            }
            .background(Museum.background)
            .navigationTitle(active.tour.tour.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("End") { showEndConfirm = true }
                        .foregroundStyle(Museum.text)
                }
                ToolbarItemGroup(placement: .topBarTrailing) {
                    if active.revision != nil && model.isSignedIn {
                        Button("Report") { showReport = true }.disabled(active.revoked)
                    }
                    if active.revision != nil {
                        Button("Trip") { showTripReview = true }.disabled(active.revoked)
                    }
                    Button {
                        model.setAudioGuide(!active.audioEnabled)
                    } label: {
                        Image(systemName: active.audioEnabled ? "speaker.wave.2.fill" : "speaker.slash")
                    }
                    .foregroundStyle(Museum.text)
                    .accessibilityLabel(active.audioEnabled ? "Audio guide on" : "Audio guide off")
                    .disabled(active.revoked)
                }
            }
            .museumDestinations()
            .sheet(item: Binding(get: { active.arrivedStop }, set: { active.arrivedStop = $0 })) { stop in
                StopMomentSheet(stop: stop, active: active, isLast: active.isComplete)
                    .environment(model)
                    .presentationDetents([.medium, .large])
                    .presentationBackground(Museum.background)
            }
            .sheet(isPresented: $showContributions) {
                NavigationStack {
                    MuseumScreen {
                        if let controller = contributionController { ContributionControls(controller: controller, allowOptIn: true) }
                        if let error = model.contributionError { MuseumInlineHint(text: error, tint: Museum.danger, isError: true) }
                    }
                    .navigationTitle("Route improvement")
                    .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Done") { showContributions = false } } }
                }
            }
            .sheet(isPresented: $showTripReview) { tripReview }
            .sheet(isPresented: $showReport, onDismiss: {
                if reviewAfterReport { reviewAfterReport = false; showTripReview = true }
            }) {
                NavigationStack {
                    if let revision = active.revision, !active.revoked {
                        RouteReportEntryView(tourID: revision.tourID, revisionID: revision.id, mode: active.mode,
                            cachedRevision: revision, initialTarget: active.nextStop.map { "entrance:\($0.id)" }) {
                            reviewAfterReport = true
                            showReport = false
                            Task { await active.checkForUpdate() }
                        }
                        .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Done") { showReport = false } } }
                    }
                }.environment(model)
            }
            .confirmationDialog("End this tour?", isPresented: $showEndConfirm, titleVisibility: .visible) {
                Button("Keep my progress and end") { model.endTour(resetProgress: false) }
                Button("Start over next time", role: .destructive) { model.endTour(resetProgress: true) }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text(active.progressText)
            }
            .task {
                for _ in 0..<30 {
                    await active.loadRoute()
                    if !active.routePending || Task.isCancelled { break }
                    do { try await Task.sleep(for: .seconds(2)) } catch { break }
                }
                await active.checkForUpdate()
            }
            .task(id: ObjectIdentifier(active)) {
                guard let controller = contributionController else { return }
                updateContributionVisibility()
                while !Task.isCancelled && model.active === active {
                    if scenePhase == .active { await controller.refresh() }
                    do { try await Task.sleep(for: .seconds(30)) } catch { return }
                }
            }
            .onChange(of: scenePhase) { _, _ in updateContributionVisibility() }
            .onChange(of: showTripReview) { _, _ in updateContributionVisibility() }
            .onChange(of: showReport) { _, _ in updateContributionVisibility() }
            .onChange(of: showEndConfirm) { _, _ in updateContributionVisibility() }
            .onChange(of: active.arrivedStop?.id) { _, id in
                contributionController?.lifecycle(id == nil ? .resume : .manualArrival)
                updateContributionVisibility()
            }
            .onDisappear { contributionController?.setForeground(false) }
            .onChange(of: active.revoked) { _, revoked in
                if revoked { contributionController?.lifecycle(.end); contributionController?.setForeground(false); model.narration.stop(); model.location.stop(); showTripReview = false; showReport = false }
            }
            .onAppear {
                model.location.requestWhenInUseAuthorization()
                model.location.start()
            }
        }
    }

    private var currentTrip: some View {
        ZStack(alignment: .bottom) {
            TourMapView(model: active)
                .ignoresSafeArea(edges: .bottom)
            VStack(spacing: 10) {
                if active.legacyResume {
                        Text("Older saved trip · Cached stops and route; progress stays on this device. End it to start a reviewed revision.")
                        .font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
                }
                if active.availableUpdate != nil {
                    Button("A new revision is available · Review update") { showTripReview = true }
                        .buttonStyle(.museum(.secondary))
                }
                if case .failed(let message) = active.routeState {
                    routeNotice(message)
                } else if active.routeFromCache {
                        Label("Using the saved route", systemImage: "archivebox")
                        .font(Museum.meta(11))
                        .foregroundStyle(Museum.muted)
                        .padding(.horizontal, 12).padding(.vertical, 6)
                        .museumGlassCard(radius: Museum.Radius.pill)
                }
                if let message = active.syncError {
                        Text(message)
                        .font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
                }
                if active.routePending {
                    Text("Route being computed. Stops still work.")
                        .font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
                }
                Button("Route improvement · \(contributionController?.optedIn == true ? (contributionController?.paused == true ? "Paused" : "On") : "Off")") { showContributions = true }
                    .buttonStyle(.museum(.secondary, size: .small))
                bottomCard
            }
            .padding(.horizontal, Museum.Spacing.pageHorizontal)
            .padding(.bottom, Museum.Spacing.md)
        }
    }

    private var tripReview: some View {
        NavigationStack {
            MuseumScreen(bottomInset: 24) {
                if let revision = active.revision {
                    Text("Started revision \(revision.number)")
                        .font(Museum.title(24)).foregroundStyle(Museum.text)
                    Text("This itinerary remains in use until you explicitly choose a different revision.")
                        .font(Museum.bodyFont(14)).foregroundStyle(Museum.muted)
                    TripJourneyView(tour: active.tour, mode: active.mode, journey: active.route?.journey ?? revision.snapshot?.routes[active.mode]?.journey)
                    Button(active.checkingUpdate ? "Checking…" : "Check for an update") { Task { await active.checkForUpdate() } }
                        .buttonStyle(.museum(.secondary))
                        .disabled(active.checkingUpdate || updateBusy)
                    if let error = active.updateError { MuseumInlineHint(text: error, tint: Museum.danger, isError: true) }
                    if let candidate = active.availableUpdate, let snapshot = candidate.snapshot {
                        Text("Available revision \(candidate.number)").font(Museum.title(22)).foregroundStyle(Museum.text)
                        TripJourneyView(tour: snapshot.tour, mode: active.mode, journey: snapshot.routes[active.mode]?.journey)
                        Text("Only visits to unchanged stops carry forward by stop ID. New or changed stops remain unvisited. Your old revision's progress stays separate.")
                            .font(Museum.bodyFont(14)).foregroundStyle(Museum.muted)
                        Button(updateBusy ? "Switching…" : "Use revision \(candidate.number)") { confirmUpdate = true }
                            .buttonStyle(.museum(.primary, size: .large))
                            .disabled(updateBusy || candidate.unavailableReason(mode: active.mode) != nil)
                    }
                    if let updateError { MuseumInlineHint(text: updateError, tint: Museum.danger, isError: true) }
                }
            }
            .navigationTitle("Reviewed trip")
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Done") { showTripReview = false } } }
            .confirmationDialog("Switch to the reviewed update?", isPresented: $confirmUpdate, titleVisibility: .visible) {
                Button("Switch revision") {
                    updateBusy = true
                    updateError = nil
                    Task {
                        defer { updateBusy = false }
                        do { try await model.adoptUpdate(for: active); showTripReview = false }
                        catch { updateError = error.museumMessage }
                    }
                }
            } message: {
                Text("The route and stop order may change. Only unchanged stop identities keep their visited state. Your old revision remains saved separately.")
            }
        }
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
            MuseumCard(fill: Museum.surface) {
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
                        Button { contributionController?.lifecycle(.manualArrival); active.markArrivedAtNextStop() } label: { Label("I'm here", systemImage: "checkmark.circle") }
                            .buttonStyle(.museum(.primary, size: .small))
                        Button { active.openNextStopInMaps() } label: { Label("Open in Maps", systemImage: "map") }
                            .buttonStyle(.museum(.secondary, size: .small))
                        if model.narration.isPlaying {
                            Button { model.narration.stop() } label: { Image(systemName: "stop.fill").frame(width: 36, height: 36) }
                                .buttonStyle(.museum(.secondary, size: .icon))
                                .accessibilityLabel("Stop narration")
                        }
                    }
                }
            }
        } else {
            MuseumCard(fill: Museum.surface) {
                VStack(alignment: .leading, spacing: 10) {
                    Overline(text: "Tour complete", color: Museum.success)
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

    private var contributionController: TripContributions? { model.active === active ? model.contributions : nil }

    private func updateContributionVisibility() {
        guard let controller = contributionController else { return }
        let visible = scenePhase == .active && !active.revoked && !active.isComplete && active.arrivedStop == nil && !showTripReview && !showReport && !showEndConfirm
        controller.setForeground(visible)
        if visible { Task { await controller.refresh() } }
    }

    private var distanceLabel: String {
        guard let d = active.distanceToNextM else { return "—" }
        return GeoMath.formatDistance(d)
    }
}
