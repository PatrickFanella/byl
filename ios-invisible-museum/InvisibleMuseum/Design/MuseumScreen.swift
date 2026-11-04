import SwiftUI

struct MuseumScreen<Content: View>: View {
    var bottomInset: CGFloat = Museum.Spacing.bottomNavClearance
    var topInset: CGFloat = Museum.Spacing.pageTop
    var horizontalInset: CGFloat = Museum.Spacing.pageHorizontal
    var verticalSpacing: CGFloat = Museum.Spacing.section
    let content: Content

    init(
        bottomInset: CGFloat = Museum.Spacing.bottomNavClearance,
        topInset: CGFloat = Museum.Spacing.pageTop,
        horizontalInset: CGFloat = Museum.Spacing.pageHorizontal,
        verticalSpacing: CGFloat = Museum.Spacing.section,
        @ViewBuilder content: () -> Content
    ) {
        self.bottomInset = bottomInset
        self.topInset = topInset
        self.horizontalInset = horizontalInset
        self.verticalSpacing = verticalSpacing
        self.content = content()
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: verticalSpacing) {
                content
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, horizontalInset)
            .padding(.top, topInset)
            .padding(.bottom, bottomInset)
        }
        .scrollIndicators(.hidden)
        .background(Museum.galleryBackground.ignoresSafeArea())
    }
}
