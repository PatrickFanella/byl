import SwiftUI
import InvisibleMuseumKit

@main
struct InvisibleMuseumApp: App {
    @AppStorage("byl.appearance") private var appearance: BylAppearance = .system
    @State private var model = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .preferredColorScheme(appearance.colorScheme)
                .tint(Museum.action)
                .font(Museum.bodyFont())
                .task { await model.bootstrap() }
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active { model.didBecomeActive() }
                    else { model.didLeaveForeground() }
                }
                .onOpenURL { url in
                    // invisiblemuseum://tour/{slug} and https://museum-api.subcult.tv/t/{slug}
                    let parts = url.pathComponents.filter { $0 != "/" }
                    if url.scheme == "invisiblemuseum", url.host == "tour", let slug = parts.first {
                        model.pendingTourSlug = slug
                    } else if parts.count >= 2, parts[0] == "t" {
                        model.pendingTourSlug = parts[1]
                    }
                }
        }
    }
}
