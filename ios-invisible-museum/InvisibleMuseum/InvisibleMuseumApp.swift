import SwiftUI
import InvisibleMuseumKit

@main
struct InvisibleMuseumApp: App {
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .preferredColorScheme(.dark)
                .task { await model.bootstrap() }
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
