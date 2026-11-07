import SwiftUI
import InvisibleMuseumKit

/// The visitor's own routes: build new ones, share, edit, delete. Signed-in only.
struct MyRoutesView: View {
    @Environment(AppModel.self) private var model
    @State private var routes: Loadable<[TourSummary]> = .idle
    @State private var path = NavigationPath()
    @State private var pendingDelete: TourSummary?
    @State private var actionError: String?

    var body: some View {
        NavigationStack(path: $path) {
            MuseumScreen {
                VStack(alignment: .leading, spacing: 10) {
                    Overline(text: "Your routes")
                    Text("Curate your own gallery")
                        .font(Museum.title(30))
                        .foregroundStyle(Museum.text)
                    Text("Order the places you love into a walk or ride and share it by link.")
                        .font(Museum.bodyFont(15))
                        .foregroundStyle(Museum.muted)
                }
                if !model.isSignedIn {
                    SignInPrompt(reason: "Sign in to build routes that follow you to every device.")
                } else {
                    Button {
                        path.append(BuilderRoute(editing: nil))
                    } label: {
                        Label("New route", systemImage: "plus")
                    }
                    .buttonStyle(.museum(.primary, size: .large))
                    .disabled(model.selectedCity == nil)
                    if let actionError { MuseumInlineHint(text: actionError, tint: Museum.muted) }
                    content
                }
            }
            .museumDestinations()
            .navigationDestination(for: BuilderRoute.self) { route in
                RouteBuilderView(editing: route.editing) { _ in
                    path.removeLast()
                    Task { await load() }
                }
            }
            .task(id: model.isSignedIn) { await load() }
            .refreshable { await load() }
            .confirmationDialog("Delete this route?", isPresented: Binding(get: { pendingDelete != nil }, set: { if !$0 { pendingDelete = nil } }), titleVisibility: .visible) {
                Button("Delete", role: .destructive) {
                    if let tour = pendingDelete { Task { await delete(tour) } }
                }
                Button("Cancel", role: .cancel) { pendingDelete = nil }
            } message: {
                Text(pendingDelete?.title ?? "")
            }
        }
    }

    @ViewBuilder
    private var content: some View {
        switch routes {
        case .idle, .loading:
            MuseumLoadingView()
        case .failed(let message):
            MuseumErrorView(message: message) { Task { await load() } }
        case .loaded(let items) where items.isEmpty:
            MuseumEmptyView(title: "No routes yet", message: "Tap New route to pick your first two places.", symbol: "point.topleft.down.to.point.bottomright.curvepath")
        case .loaded(let items):
            ForEach(items) { tour in
                VStack(alignment: .leading, spacing: 8) {
                    NavigationLink(value: TourRef(ref: tour.slug)) { TourCard(tour: tour) }
                        .buttonStyle(.plain)
                    HStack(spacing: 8) {
                        if tour.status == "published" {
                            ShareLink(item: RouteBuilderModel.shareURL(slug: tour.slug, base: model.client.baseURL), subject: Text(tour.title)) {
                                Label("Share", systemImage: "square.and.arrow.up")
                            }
                            .buttonStyle(.museum(.secondary, size: .small))
                        } else {
                            Text("Draft · Path review required before sharing")
                                .font(Museum.bodyFont(12))
                                .foregroundStyle(Museum.muted)
                        }
                        Button {
                            Task {
                                actionError = nil
                                do { path.append(BuilderRoute(editing: try await model.api.tour(tour.slug))) }
                                catch { actionError = error.museumMessage }
                            }
                        } label: { Label("Edit", systemImage: "pencil") }
                            .buttonStyle(.museum(.secondary, size: .small))
                        Button(role: .destructive) { pendingDelete = tour } label: { Image(systemName: "trash").frame(width: 36, height: 36) }
                            .buttonStyle(.museum(.ghost, size: .icon))
                            .accessibilityLabel("Delete \(tour.title)")
                        Text(tour.visibility.capitalized)
                            .font(Museum.meta(10))
                            .foregroundStyle(Museum.faint)
                    }
                }
            }
        }
    }

    private func load() async {
        guard model.isSignedIn else { routes = .idle; return }
        routes = .loading
        do {
            routes = .loaded(try await model.api.myTours())
        } catch {
            routes = .failed(error.museumMessage)
        }
    }

    private func delete(_ tour: TourSummary) async {
        pendingDelete = nil
        actionError = nil
        do {
            try await model.api.deleteTour(tour.id)
            await load()
        } catch {
            actionError = error.museumMessage
        }
    }
}

/// Navigation value for the builder; `editing` carries the route being changed.
struct BuilderRoute: Hashable {
    let editing: TourDetail?

    static func == (lhs: BuilderRoute, rhs: BuilderRoute) -> Bool { lhs.editing?.tour.id == rhs.editing?.tour.id }
    func hash(into hasher: inout Hasher) { hasher.combine(editing?.tour.id ?? "new") }
}

struct SignInPrompt: View {
    @Environment(AppModel.self) private var model
    let reason: String

    var body: some View {
        MuseumCard {
            VStack(alignment: .leading, spacing: 12) {
                Overline(text: "Sign in")
                Text(reason)
                    .font(Museum.bodyFont(15))
                    .foregroundStyle(Museum.text)
                Button("Sign in") { model.presentSignIn = true }
                    .buttonStyle(.museum(.primary))
            }
        }
    }
}
