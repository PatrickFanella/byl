import SwiftUI
import InvisibleMuseumKit

private struct SuggestionSessionIdentity: Hashable {
    let accountID: String?
    let credential: String?
    let origin: URL
}

struct RouteSuggestionsView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Group {
            if let owner = model.account?.id {
                SuggestionAccountView(accountID: owner)
                    .id(SuggestionSessionIdentity(accountID: owner, credential: model.session.token, origin: model.client.baseURL))
            } else {
                MuseumScreen {
                    introduction
                    SignInPrompt(reason: "Sign in to suggest a route and read its private review status. An unfinished draft returns when you sign back into the same account on this device.")
                }
            }
        }
        .navigationTitle("Route suggestions")
        .navigationBarTitleDisplayMode(.inline)
    }
}

private var introduction: some View {
    VStack(alignment: .leading, spacing: 10) {
        Overline(text: "Curator review")
        Text("Suggest a route")
            .font(Museum.title(28))
            .foregroundStyle(Museum.text)
        Text("Send a topic and area for curators to investigate. Suggestions stay private until reviewed content is approved for publication. Use New route in Your routes to plan your own trip.")
            .font(Museum.bodyFont(15))
            .foregroundStyle(Museum.muted)
    }
}

private struct SuggestionAccountView: View {
    @Environment(AppModel.self) private var app
    let accountID: String
    @State private var controller: RouteSuggestionsModel?
    @State private var confirmDiscard = false

    var body: some View {
        Group {
            if let controller {
                MuseumScreen {
                    if let message = controller.message {
                        MuseumInlineHint(text: message)
                            .accessibilityAddTraits(.updatesFrequently)
                    }
                    if let id = controller.selectedID {
                        detail(controller, id: id)
                    } else {
                        introduction
                        intake(controller)
                        SuggestionForm(controller: controller)
                        Button("Discard local draft", role: .destructive) { confirmDiscard = true }
                            .buttonStyle(.museum(.ghost))
                            .disabled(controller.busy)
                        submissions(controller)
                    }
                }
                .id(controller.selectedID ?? "suggestion-list")
                .refreshable { await refresh(controller) }
                .confirmationDialog("Discard the draft on this device?", isPresented: $confirmDiscard, titleVisibility: .visible) {
                    Button("Discard local draft", role: .destructive) { controller.discardLocalDraft() }
                } message: {
                    Text("This removes unfinished input and its retry key. It does not delete a submission already saved on the server. Check Your submissions before starting another.")
                }
            } else { MuseumLoadingView() }
        }
        .task {
            let flow = app.routeSuggestions(accountID: accountID)
            controller = flow
            await refresh(flow)
        }
        .onDisappear {
            controller?.close()
            controller = nil
        }
    }

    private func refresh(_ flow: RouteSuggestionsModel) async {
        if let id = flow.selectedID { await flow.loadDetail(id) }
        else {
            await flow.loadIntake()
            await flow.loadPage(offset: flow.offset)
        }
    }

    @ViewBuilder
    private func intake(_ flow: RouteSuggestionsModel) -> some View {
        if flow.needsSignIn {
            SignInPrompt(reason: "Sign in again to continue. Your unfinished suggestion is kept on this device.")
        }
        if flow.intakeLoading {
            ProgressView("Checking whether suggestions are open…")
        } else if let error = flow.intakeError {
            MuseumInlineHint(text: error, tint: Museum.danger, isError: true)
            Button("Check again") { Task { await flow.loadIntake() } }
                .buttonStyle(.museum(.secondary))
        } else if flow.intake == false {
            MuseumInlineHint(text: "New suggestions are currently closed. You can keep a draft and read your previous submissions.")
            Button("Check again") { Task { await flow.loadIntake() } }
                .buttonStyle(.museum(.secondary))
        }
    }

    @ViewBuilder
    private func submissions(_ flow: RouteSuggestionsModel) -> some View {
        MuseumSectionHeader(title: "Your submissions", eyebrow: "Private to your account")
        if flow.listLoading {
            MuseumLoadingView()
        } else if let error = flow.listError {
            MuseumErrorView(message: error) { Task { await flow.loadPage(offset: flow.offset) } }
        } else if flow.items.isEmpty {
            Text(flow.offset == 0 ? "You have no submissions yet." : "No more submissions on this page.")
                .foregroundStyle(Museum.muted)
        } else {
            ForEach(flow.items) { item in
                Button { Task { await flow.loadDetail(item.id) } } label: {
                    MuseumCard {
                        VStack(alignment: .leading, spacing: 8) {
                            Text(item.submitted.topic).font(Museum.bodyFont(17, weight: .semibold))
                            Text(item.submitted.area).font(Museum.bodyFont(14))
                            Text(SuggestionStatus(item.status).label).font(Museum.meta(12))
                        }
                        .foregroundStyle(Museum.text)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                .buttonStyle(.plain)
                .accessibilityHint("Open private submission status and decision")
            }
        }
        if flow.offset > 0 {
            Button("Previous submissions") { Task { await flow.loadPage(offset: max(0, flow.offset - 20)) } }
                .buttonStyle(.museum(.secondary))
                .disabled(flow.listLoading)
        }
        if flow.hasMore {
            Button("More submissions") { Task { await flow.loadPage(offset: flow.offset + 20) } }
                .buttonStyle(.museum(.secondary))
                .disabled(flow.listLoading)
        }
    }

    @ViewBuilder
    private func detail(_ flow: RouteSuggestionsModel, id: String) -> some View {
        Button("Back to your submissions") {
            flow.showList()
            Task { await flow.loadPage(offset: flow.offset) }
        }
        .buttonStyle(.museum(.secondary))
        if flow.detailLoading { MuseumLoadingView() }
        else if let error = flow.detailError {
            MuseumErrorView(message: error) { Task { await flow.loadDetail(id) } }
            if flow.needsSignIn { SignInPrompt(reason: "Sign in again to read your private submission.") }
        } else if let item = flow.selected {
            let status = SuggestionStatus(item.status)
            Overline(text: "Private submission")
            Text(item.submitted.topic).font(Museum.title(26)).foregroundStyle(Museum.text)
            Text(item.submitted.area).font(Museum.bodyFont(17)).foregroundStyle(Museum.muted)
            MuseumCard {
                VStack(alignment: .leading, spacing: 10) {
                    Text(status.label).font(Museum.bodyFont(18, weight: .semibold))
                    Text(status.detail).font(Museum.bodyFont(15))
                    if let reason = item.reason, !reason.isEmpty {
                        Overline(text: "Decision or research note")
                        Text(reason).font(Museum.bodyFont(15)).textSelection(.enabled)
                    }
                }
                .foregroundStyle(Museum.text)
            }
            if !item.submitted.notes.isEmpty { Text(item.submitted.notes).textSelection(.enabled) }
            leads("Place leads", item.submitted.placeLeads)
            leads("Event leads", item.submitted.eventLeads)
            leads("Source leads", item.submitted.sourceLinks)
            Text("Updated \(item.updatedAt) · Version \(item.version)")
                .font(Museum.bodyFont(12)).foregroundStyle(Museum.faint)
            Button("Refresh status") { Task { await flow.loadDetail(id) } }
                .buttonStyle(.museum(.secondary))
        }
    }

    @ViewBuilder
    private func leads(_ title: String, _ values: [String]) -> some View {
        if !values.isEmpty {
            Overline(text: title)
            ForEach(Array(values.enumerated()), id: \.offset) { _, value in
                Text(value).font(Museum.bodyFont(14)).foregroundStyle(Museum.text).textSelection(.enabled)
            }
        }
    }
}

private struct SuggestionForm: View {
    let controller: RouteSuggestionsModel
    @State private var optionalLeads = false

    private func binding(_ key: WritableKeyPath<SuggestionFields, String>) -> Binding<String> {
        Binding(get: { controller.fields[keyPath: key] }, set: { value in
            var fields = controller.fields
            fields[keyPath: key] = value
            controller.edit(fields)
        })
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.md) {
            MuseumTextField(label: "Topic", placeholder: "e.g. Rail workers", text: binding(\.topic), helperText: controller.fieldErrors["topic"], helperTint: Museum.danger, helperIsError: true)
            MuseumTextField(label: "Area", placeholder: "e.g. Downers Grove", text: binding(\.area), helperText: controller.fieldErrors["area"], helperTint: Museum.danger, helperIsError: true)
            DisclosureGroup("Optional notes and leads", isExpanded: $optionalLeads) {
                VStack(spacing: Museum.Spacing.md) {
                    editor("Notes", "What would you like investigated?", \.notes, errorKey: "notes")
                    editor("Place leads", "Names or addresses, one per line", \.places, errorKey: "place_leads")
                    editor("Event leads", "Names or dates, one per line", \.events, errorKey: "event_leads")
                    editor("Source links", "https://…, one per line", \.sources, errorKey: "source_links")
                }
                .padding(.top, 12)
            }
            .foregroundStyle(Museum.text)
        }
        .disabled(!controller.canEdit)
        .onAppear {
            optionalLeads = [controller.fields.notes, controller.fields.places, controller.fields.events, controller.fields.sources].contains { !$0.isEmpty }
        }
        if controller.pending != nil {
            MuseumInlineHint(text: "The original request is saved for retry. Check Your submissions before discarding it or starting another.")
        }
        Button {
            Task {
                await controller.submit()
                if controller.fieldErrors.keys.contains(where: { !["topic", "area"].contains($0) }) { optionalLeads = true }
            }
        } label: {
            Text(controller.busy ? "Sending…" : controller.pending == nil ? "Submit for review" : "Retry original suggestion")
        }
        .buttonStyle(.museum(.primary, size: .large))
        .disabled(!controller.canSubmit)
        Text("Unfinished input is saved on this device for this account and server. No location tracking is collected by this form.")
            .font(Museum.bodyFont(12)).foregroundStyle(Museum.faint)
    }

    private func editor(_ label: String, _ placeholder: String, _ key: WritableKeyPath<SuggestionFields, String>, errorKey: String) -> some View {
        MuseumTextEditor(label: label, placeholder: placeholder, text: binding(key), minHeight: 85, helperText: controller.fieldErrors[errorKey], helperTint: Museum.danger, helperIsError: true)
    }
}
