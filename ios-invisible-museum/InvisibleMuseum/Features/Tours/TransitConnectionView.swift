import SwiftUI
import InvisibleMuseumKit

struct TransitConnectionView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL
    @Environment(\.scenePhase) private var scenePhase
    let trip: TransitTripModel
    @State private var departure = Date().addingTimeInterval(3600)
    @State private var error: String?
    @State private var showHandoff = false
    @State private var handoffBoarding = true

    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { context in
            MuseumScreen(bottomInset: 24) {
                if let choice = trip.revision.snapshot?.tour.transit {
                    Text("Scheduled CTA connection").font(Museum.title(24)).foregroundStyle(Museum.text)
                    Text("Revision \(trip.revision.number) · timetable estimates, not live arrivals. Byl does not detect boarding, alighting or train movement.")
                    Text("Board \(choice.board.name) · platform \(choice.board.id)")
                    Text("Alight \(choice.alight.name) · platform \(choice.alight.id)")
                    Text("Walking estimates: \(choice.intent.walkToStopSeconds / 60) min to the platform and \(choice.intent.walkFromStopSeconds / 60) min after alighting, plus \(choice.intent.boardingBufferSeconds / 60) min boarding buffer.")
                    Text(choice.intent.walkingAssumptions)
                    Link("Check CTA Blue Line alerts", destination: URL(string: "https://www.transitchicago.com/blueline/")!)
                    if let selected = trip.selected {
                        Text("Retained choice").font(Museum.title(20))
                        plan(selected)
                        if let reason = selected.unavailableReason(now: context.date) { MuseumInlineHint(text: reason) }
                        if trip.guidanceBlocked { MuseumInlineHint(text: "This connection needs review. Retained details remain readable; directions are suspended until you explicitly accept a fresh valid plan.") }
                        if trip.canGuide(at: context.date) {
                            Button("Walking directions to boarding platform") { handoffBoarding = true; showHandoff = true }
                                .buttonStyle(.museum(.secondary))
                            Button("Walking directions after alighting") { handoffBoarding = false; showHandoff = true }
                                .buttonStyle(.museum(.secondary))
                        }
                        Button("Remove retained transit choice", role: .destructive) {
                            do { try trip.remove(); error = nil } catch { self.error = error.localizedDescription }
                        }.buttonStyle(.museum(.ghost))
                    }
                    Text("Choose a departure").font(Museum.title(20))
                    Text("Choose when you expect to leave \(departureStopName) after its stories. Earlier walking and story time are outside this query. The picker uses your device timezone; results below use Chicago time.")
                    DatePicker("Leave connector story stop", selection: $departure, displayedComponents: [.date, .hourAndMinute])
                    Button(trip.busy ? "Requesting…" : "Request a plan for review") {
                        error = nil
                        Task { await trip.request(departure: departure) }
                    }.buttonStyle(.museum(.primary)).disabled(trip.busy)
                    if let proposed = trip.proposed {
                        Text("Proposed choice · review before retaining").font(Museum.title(20))
                        plan(proposed)
                        if let reason = proposed.unavailableReason(now: context.date) { MuseumInlineHint(text: reason) }
                        else {
                            Button("Accept and retain this plan") {
                                do { try trip.accept(); error = nil } catch { self.error = error.localizedDescription }
                            }.buttonStyle(.museum(.primary)).disabled(trip.busy || trip.enabled != true)
                        }
                    }
                    if trip.enabled != true { MuseumInlineHint(text: "Scheduled transit availability is unconfirmed or disabled. Directions are suspended.") }
                    Button("Refresh availability") { Task { await trip.refreshGate() } }.buttonStyle(.museum(.secondary))
                    if let message = trip.message { MuseumInlineHint(text: message) }
                    if let error { MuseumInlineHint(text: error, tint: Museum.danger, isError: true) }
                }
            }
            .font(Museum.bodyFont(14)).foregroundStyle(Museum.text)
        }
        .navigationTitle("Transit plan")
        .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Done") { dismiss() } } }
        .task {
            while !Task.isCancelled {
                if scenePhase == .active { await trip.refreshGate() }
                do { try await Task.sleep(for: .seconds(30)) } catch { return }
            }
        }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { Task { await trip.refreshGate() } }
        }
        .confirmationDialog("Open walking directions in Apple Maps?", isPresented: $showHandoff, titleVisibility: .visible) {
            Button("Open Apple Maps") {
                if let url = trip.walkingURL(boarding: handoffBoarding, at: Date()) { openURL(url) }
                showHandoff = false
            }
            Button("Cancel", role: .cancel) { showHandoff = false }
        } message: {
            Text("Apple Maps takes over directions between public story/platform references. Timetable points are not reviewed street entrances or proof of accessibility. Check station signs and current service. No device location is included in this link.")
        }
    }

    private var departureStopName: String {
        let tour = trip.revision.snapshot?.tour
        return tour?.stops.first { $0.id == tour?.transit?.intent.fromStopID }?.place.name ?? "the first connector story stop"
    }

    private func plan(_ plan: ScheduledTransitPlan) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Result: \(plan.state.replacingOccurrences(of: "_", with: " "))")
            Text("Leave story stop: \(time(plan.requestedDepartureAt))")
            if let source = plan.service.source {
                Text("CTA timetable retrieved \(time(source.retrievedAt)) · America/Chicago")
            }
            if let headsign = plan.service.headsign { Text("Blue Line toward \(headsign)") }
            ForEach(Array(plan.segments.enumerated()), id: \.offset) { _, segment in
                VStack(alignment: .leading, spacing: 3) {
                    Text(segment.label).font(Museum.bodyFont(15, weight: .semibold))
                    Text("\(segment.durationS / 60) min · \(segment.basis.replacingOccurrences(of: "_", with: " "))")
                    if let departure = segment.departureAt, let arrival = segment.arrivalAt {
                        Text("\(time(departure)) → \(time(arrival))")
                    }
                }
            }
            ForEach(plan.assumptions, id: \.self) { Text($0) }
        }
    }

    private func time(_ value: String) -> String {
        guard let date = transitDate(value) else { return "Unavailable" }
        let formatter = DateFormatter(); formatter.timeZone = TimeZone(identifier: "America/Chicago")
        formatter.dateStyle = .medium; formatter.timeStyle = .short
        return formatter.string(from: date) + " Chicago"
    }
}
