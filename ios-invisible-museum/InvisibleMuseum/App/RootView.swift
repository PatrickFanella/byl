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
        .fullScreenCover(item: Binding(get: { model.active }, set: { if $0 == nil { model.endTour(resetProgress: false) } })) { active in
            ActiveTourView(active: active)
                .environment(model)
        }
        .sheet(isPresented: $model.presentSignIn) {
            SignInView()
                .environment(model)
        }
        .onChange(of: model.pendingTourSlug) { _, slug in
            if slug != nil { tab = .tours }
        }
    }
}
