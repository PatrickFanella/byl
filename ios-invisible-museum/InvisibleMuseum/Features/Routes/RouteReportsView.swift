import SwiftUI
import InvisibleMuseumKit

private struct ReportIdentity: Hashable {
    let account: String
    let credential: String?
    let origin: URL
    var revision = ""
}

/// Uses an exact retained revision, including the active trip's cached revision offline.
struct RouteReportEntryView: View {
    @Environment(AppModel.self) private var app
    let tourID: String
    let revisionID: String
    let mode: String
    var cachedRevision: TourRevision? = nil
    var initialTarget: String? = nil
    var onReviewUpdate: (() -> Void)? = nil
    @State private var revision: TourRevision?
    @State private var error: String?

    var body: some View {
        Group {
            if let account = app.account?.id {
                Group {
                    if let revision {
                        let context = RouteReportContext(revision: revision, mode: mode)
                        if let reason = context.unavailableReason {
                            MuseumScreen { MuseumInlineHint(text: reason, tint: Museum.danger, isError: true) }
                        } else {
                            ReportAccountView(accountID: account, context: context, initialTarget: initialTarget, onReviewUpdate: onReviewUpdate)
                        }
                    } else if let error {
                        MuseumScreen { MuseumErrorView(message: error) { Task { await load() } } }
                    } else { MuseumLoadingView(text: "Opening the reported revision…") }
                }
                .id(ReportIdentity(account: account, credential: app.session.token, origin: app.client.baseURL))
            } else {
                MuseumScreen { SignInPrompt(reason: "Sign in to send a private route report and read its decision. Your unfinished input returns for the same account on this device.") }
            }
        }
        .navigationTitle("Suggest a route change")
        .navigationBarTitleDisplayMode(.inline)
        .task(id: ReportIdentity(account: app.account?.id ?? "", credential: app.session.token, origin: app.client.baseURL, revision: revisionID)) { await load() }
    }
    private func load() async {
        let account = app.account?.id, credential = app.session.token, origin = app.client.baseURL
        error = nil; revision = nil
        do {
            let retained: TourRevision
            if let cachedRevision { retained = cachedRevision }
            else { retained = try await app.api.revision(tour: tourID, revision: revisionID) }
            guard !Task.isCancelled, app.account?.id == account, app.session.token == credential, app.client.baseURL == origin else { return }
            guard retained.id == revisionID && retained.tourID == tourID else {
                error = "The selected revision could not be confirmed."; return
            }
            revision = retained
        } catch {
            guard !Task.isCancelled, app.account?.id == account, app.session.token == credential, app.client.baseURL == origin else { return }
            self.error = "Could not read this retained revision. Connect and try again; the selected revision will not be replaced."
        }
    }
}

struct RouteReportsView: View {
    @Environment(AppModel.self) private var app
    var body: some View {
        Group {
            if let account = app.account?.id {
                ReportAccountView(accountID: account)
                    .id(ReportIdentity(account: account, credential: app.session.token, origin: app.client.baseURL))
            } else { MuseumScreen { SignInPrompt(reason: "Sign in to read your private route reports and curator decisions.") } }
        }
        .navigationTitle("Your route reports")
        .navigationBarTitleDisplayMode(.inline)
    }
}

private struct ReportAccountView: View {
    @Environment(AppModel.self) private var app
    let accountID: String
    var context: RouteReportContext? = nil
    var initialTarget: String? = nil
    var onReviewUpdate: (() -> Void)? = nil
    @State private var controller: RouteReportsModel?
    @State private var confirmDiscard = false

    var body: some View {
        Group {
            if let controller {
                MuseumScreen {
                    if let message = controller.message { MuseumInlineHint(text: message).accessibilityAddTraits(.updatesFrequently) }
                    if controller.needsSignIn { SignInPrompt(reason: "Sign in again. Your report input and retry key remain on this device.") }
                    if controller.selectedID != nil {
                        if context == nil {
                            Button("Back to your reports") { controller.showList(); Task { await controller.loadPage(offset: controller.offset) } }
                                .buttonStyle(.museum(.secondary))
                        }
                        ReportStatusContent(controller: controller, onReviewUpdate: onReviewUpdate)
                    } else if let context {
                        Overline(text: "Private observation")
                        Text(context.revision.snapshot?.tour.tour.title ?? "Reported route").font(Museum.title(26))
                        Text("Revision \(context.revision.number) · \(context.mode == "bike" ? "Cycling" : "Walking"). A report needs no replacement path. Curators review observations before a route changes.")
                            .foregroundStyle(Museum.muted)
                        ReportForm(controller: controller, context: context)
                        Button("Discard local draft", role: .destructive) { confirmDiscard = true }
                            .buttonStyle(.museum(.ghost)).disabled(controller.busy)
                        NavigationLink("Your route reports") { RouteReportsView() }.buttonStyle(.museum(.secondary))
                    } else {
                        reportList(controller)
                    }
                }
                .refreshable { await refresh(controller) }
                .confirmationDialog("Discard this local report?", isPresented: $confirmDiscard, titleVisibility: .visible) {
                    Button("Discard local draft", role: .destructive) { controller.discardLocalDraft() }
                } message: { Text("This removes unfinished input and its retry identity on this device. It does not cancel a report already saved on the server. Check Your reports before starting another.") }
            } else { MuseumLoadingView() }
        }
        .task {
            let flow = app.routeReports(accountID: accountID, context: context, initialTarget: initialTarget)
            controller = flow
            if context == nil { await flow.loadPage() }
        }
        .onDisappear { controller?.close(); controller = nil }
    }
    private func refresh(_ flow: RouteReportsModel) async {
        if let id = flow.selectedID { await flow.loadDetail(id) }
        else if context == nil { await flow.loadPage(offset: flow.offset) }
    }
    @ViewBuilder private func reportList(_ flow: RouteReportsModel) -> some View {
        MuseumSectionHeader(title: "Your route reports", eyebrow: "Private to your account")
        if flow.listLoading { MuseumLoadingView() }
        else if let error = flow.listError { MuseumErrorView(message: error) { Task { await flow.loadPage(offset: flow.offset) } } }
        else if flow.items.isEmpty { Text("No reports on this page.").foregroundStyle(Museum.muted) }
        else {
            ForEach(flow.items) { report in
                Button { Task { await flow.loadDetail(report.id) } } label: {
                    MuseumCard {
                        VStack(alignment: .leading, spacing: 8) {
                            Text(report.submitted.details.isEmpty ? report.submitted.reason.capitalized : report.submitted.details).lineLimit(3)
                            Text("\(SuggestionStatus(report.status).label) · \(report.submitted.mode == "bike" ? "Cycling" : "Walking")").font(Museum.meta(12))
                        }.foregroundStyle(Museum.text).frame(maxWidth: .infinity, alignment: .leading)
                    }
                }.buttonStyle(.plain).accessibilityHint("Open private report and curator decision")
            }
        }
        if flow.offset > 0 { Button("Previous reports") { Task { await flow.loadPage(offset: max(0, flow.offset - 20)) } }.buttonStyle(.museum(.secondary)).disabled(flow.listLoading) }
        if flow.hasMore { Button("More reports") { Task { await flow.loadPage(offset: flow.offset + 20) } }.buttonStyle(.museum(.secondary)).disabled(flow.listLoading) }
        Button("Refresh reports") { Task { await flow.loadPage(offset: flow.offset) } }.buttonStyle(.museum(.secondary)).disabled(flow.listLoading)
    }
}

private struct ReportForm: View {
    let controller: RouteReportsModel
    let context: RouteReportContext
    private let reasons = [("blocked", "Cannot pass"), ("closed", "Closure"), ("unsafe", "Unsafe crossing or path"), ("access", "Entrance or access problem"), ("surface", "Surface problem"), ("improvement", "A better option"), ("other", "Something else")]
    private func field<T>(_ path: WritableKeyPath<ReportFields, T>) -> Binding<T> {
        Binding(get: { controller.fields[keyPath: path] }, set: { value in
            var fields = controller.fields; fields[keyPath: path] = value; controller.edit(fields)
        })
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Group {
                Picker("Affected stop, entrance or path", selection: field(\.target)) {
                    ForEach(context.targets) { target in Text(target.label).tag(target.id) }
                }
                Picker("What is the problem?", selection: field(\.reason)) {
                    ForEach(Array(reasons.enumerated()), id: \.offset) { _, reason in Text(reason.1).tag(reason.0) }
                }
                labeledEditor("What did you observe?", value: field(\.details))
                if controller.fields.reason == "improvement" { labeledEditor("What would be better?", value: field(\.better)) }
                TextField("Source link, if available", text: field(\.sourceURL)).keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                Picker("Travel direction", selection: field(\.direction)) {
                    Text("Not sure").tag("unknown"); Text("Following displayed itinerary").tag("forward"); Text("Opposite displayed itinerary").tag("reverse")
                }
                Picker("How long might it last?", selection: field(\.duration)) {
                    Text("Not sure").tag("unknown"); Text("Temporary").tag("temporary"); Text("Permanent").tag("permanent")
                }
                TextField("Date observed, if known (YYYY-MM-DD)", text: field(\.observedOn)).textInputAutocapitalization(.never).autocorrectionDisabled()
                DisclosureGroup("Optional location or local alternative") {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Enter only the problem area. No current location or background journey is attached. Leave these fields empty for a report only.").font(Museum.bodyFont(13)).foregroundStyle(Museum.muted)
                        TextField("Problem latitude", text: field(\.latitude)).keyboardType(.numbersAndPunctuation)
                        TextField("Problem longitude", text: field(\.longitude)).keyboardType(.numbersAndPunctuation)
                        labeledEditor("Alternative path: longitude, latitude pairs, one per line", value: field(\.alternativePoints))
                        Text("Use 2 to 64 points in travel order, close to the selected area. Path alternatives must connect to the selected segment's endpoints. These are unverified suggestions.").font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
                    }
                }
            }.disabled(!controller.canEdit)
            ForEach(controller.fieldErrors.sorted(by: { $0.key < $1.key }), id: \.key) { field, message in
                MuseumInlineHint(text: "\(field): \(message)", tint: Museum.danger, isError: true)
            }
            if controller.pending != nil { Text("Input is locked while retrying. The exact observation, revision and retry key are retained.").font(Museum.bodyFont(13)).foregroundStyle(Museum.muted) }
            Button(controller.busy ? "Sending…" : controller.pending == nil ? "Send private report" : "Retry exact report") {
                Task { if let saved = await controller.submit() { await controller.loadDetail(saved.id) } }
            }.buttonStyle(.museum(.primary, size: .large)).disabled(!controller.canSubmit)
        }.textFieldStyle(.roundedBorder)
    }
    private func labeledEditor(_ label: String, value: Binding<String>) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label).font(Museum.bodyFont(14, weight: .semibold))
            TextEditor(text: value).frame(minHeight: 88).accessibilityLabel(label).overlay(RoundedRectangle(cornerRadius: 6).stroke(Museum.faint))
        }
    }
}

private struct ReportStatusContent: View {
    let controller: RouteReportsModel
    var onReviewUpdate: (() -> Void)? = nil
    var body: some View {
        if controller.detailLoading { MuseumLoadingView() }
        else if let error = controller.detailError { MuseumErrorView(message: error) { refresh() } }
        else if let report = controller.selected {
            MuseumSectionHeader(title: SuggestionStatus(report.status).label, eyebrow: "Private report")
            Text(report.submitted.reason.capitalized).font(Museum.bodyFont(17, weight: .semibold))
            Text(report.submitted.details.isEmpty ? "No additional description." : report.submitted.details).textSelection(.enabled)
            if !report.submitted.better.isEmpty { Text("Suggested improvement: \(report.submitted.better)") }
            Text("\(report.submitted.mode == "bike" ? "Cycling" : "Walking") · \(report.submitted.affected.kind) · Direction: \(report.submitted.direction) · Duration: \(report.submitted.duration)").font(Museum.bodyFont(13)).foregroundStyle(Museum.muted)
            if let reason = report.reason, !reason.isEmpty {
                MuseumSectionHeader(title: "Curator decision reason", eyebrow: "Review update")
                Text(reason).textSelection(.enabled)
            } else { Text("No curator decision has been recorded yet.").foregroundStyle(Museum.muted) }
            if !report.submitted.sourceURL.isEmpty { Text("Submitted source lead: \(report.submitted.sourceURL)").textSelection(.enabled) }
            if let point = report.submitted.annotation { Text("Reported location: \(point.lat), \(point.lng)").font(Museum.bodyFont(13)) }
            if let alternative = report.submitted.alternative { Text("Unverified local alternative: \(alternative.coordinates.count) points.").font(Museum.bodyFont(13)) }
            if let outcome = controller.outcome {
                MuseumInlineHint(text: "An approved route update is available. A started trip keeps its original revision until you review and choose an update.")
                if let onReviewUpdate { Button("Review route update", action: onReviewUpdate).buttonStyle(.museum(.secondary)) }
                else { NavigationLink("Open tour to review the update") { TourDetailView(ref: report.submitted.tourID) }.buttonStyle(.museum(.secondary)) }
                Text("In a started trip, open Trip, check for updates, then choose whether to use the newer revision.").font(Museum.bodyFont(13)).foregroundStyle(Museum.muted)
                if outcome.temporary { Text("Temporary detour · review due \(outcome.reviewOn ?? "date unavailable").\(outcome.reassessmentRequired ? " Reassessment is required; reopening is not inferred." : "")").font(Museum.bodyFont(13)) }
            }
            if let error = controller.outcomeError { MuseumInlineHint(text: error, tint: Museum.danger, isError: true) }
            Button("Refresh report status") { refresh() }.buttonStyle(.museum(.secondary))
        }
    }
    private func refresh() { if let id = controller.selectedID { Task { await controller.loadDetail(id) } } }
}
