import SwiftUI
import InvisibleMuseumKit

struct RootView: View {
    @Environment(AppModel.self) private var model
    @State private var tab: MuseumTab = .home

    var body: some View {
        @Bindable var model = model
        ZStack {
            Group {
                switch tab {
                case .home: HomeView()
                case .tours: TourListView()
                case .routes: MyRoutesView()
                case .saved: SavedView()
                case .profile: ProfileView()
                }
            }
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            MuseumBottomBar(selection: $tab)
        }
        .sheet(isPresented: $model.presentSignIn) {
            SignInView()
                .environment(model)
                .preferredColorScheme(.dark)
        }
        .onChange(of: model.pendingTourSlug) { _, slug in
            if slug != nil { tab = .tours }
        }
    }
}

/// Placeholder used until a feature screen exists; replaced in later tasks.
struct PlaceholderScreen: View {
    let title: String

    var body: some View {
        MuseumScreen {
            Text(title)
                .font(Museum.title(28))
                .foregroundStyle(Museum.text)
            Text("Coming up in this rebuild.")
                .font(Museum.bodyFont())
                .foregroundStyle(Museum.muted)
        }
    }
}

struct MyRoutesView: View { var body: some View { PlaceholderScreen(title: "Your routes") } }
struct SavedView: View { var body: some View { PlaceholderScreen(title: "Saved") } }
struct ProfileView: View { var body: some View { PlaceholderScreen(title: "Profile") } }
