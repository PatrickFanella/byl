import SwiftUI

struct MuseumCard<Content: View>: View {
    var radius: CGFloat = Museum.Radius.lg
    var fill: Color = Museum.surface
    var stroke: Color = Museum.border
    var padding: CGFloat = Museum.Spacing.lg
    let content: Content

    init(
        radius: CGFloat = Museum.Radius.lg,
        fill: Color = Museum.surface,
        stroke: Color = Museum.border,
        padding: CGFloat = Museum.Spacing.lg,
        @ViewBuilder content: () -> Content
    ) {
        self.radius = radius
        self.fill = fill
        self.stroke = stroke
        self.padding = padding
        self.content = content()
    }

    var body: some View {
        content
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(padding)
            .background(fill, in: RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).stroke(stroke, lineWidth: 1))
    }
}

struct MuseumMediaCard<Overlay: View>: View {
    var radius: CGFloat = Museum.Radius.xl
    var gradient: LinearGradient = Museum.accentGradient
    var symbol: String?
    var symbolSize: CGFloat = 62
    var height: CGFloat = 190
    let overlay: Overlay

    init(
        radius: CGFloat = Museum.Radius.xl,
        gradient: LinearGradient = Museum.accentGradient,
        symbol: String? = nil,
        symbolSize: CGFloat = 62,
        height: CGFloat = 190,
        @ViewBuilder overlay: () -> Overlay
    ) {
        self.radius = radius
        self.gradient = gradient
        self.symbol = symbol
        self.symbolSize = symbolSize
        self.height = height
        self.overlay = overlay()
    }

    var body: some View {
        ZStack(alignment: .bottomLeading) {
            RoundedRectangle(cornerRadius: radius, style: .continuous)
                .fill(gradient)
                .overlay(alignment: .center) {
                    if let symbol {
                        Image(systemName: symbol)
                            .font(.system(size: symbolSize, weight: .ultraLight))
                            .foregroundStyle(Museum.muted)
                    }
                }
                .overlay(alignment: .bottom) {
                    Museum.background
                        .frame(height: height * 0.45)
                }

            overlay
                .padding(Museum.Spacing.lg)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Museum.background)
        }
        .frame(maxWidth: .infinity)
        .frame(minHeight: height)
        .clipShape(RoundedRectangle(cornerRadius: radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).stroke(Museum.border, lineWidth: 1))
    }
}

/// A media card that shows a remote image when there is one and the gradient-and-symbol
/// treatment when there is not, with the overlay content pinned to the bottom.
struct MuseumHeroCard<Overlay: View>: View {
    var imageURL: URL?
    var symbol: String = "building.columns"
    var height: CGFloat = 190
    var radius: CGFloat = Museum.Radius.xl
    let overlay: Overlay

    init(imageURL: URL?, symbol: String = "building.columns", height: CGFloat = 190, radius: CGFloat = Museum.Radius.xl, @ViewBuilder overlay: () -> Overlay) {
        self.imageURL = imageURL
        self.symbol = symbol
        self.height = height
        self.radius = radius
        self.overlay = overlay()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let imageURL {
                AsyncImage(url: imageURL) { phase in
                    if case .success(let image) = phase {
                        image.resizable().scaledToFill()
                    } else {
                        Museum.surface
                    }
                }
                .frame(height: height)
                .clipped()
            }
            overlay
                .padding(Museum.Spacing.lg)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Museum.background)
        }
        .frame(maxWidth: .infinity)
        .frame(minHeight: height)
        .clipShape(RoundedRectangle(cornerRadius: radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).stroke(Museum.border, lineWidth: 1))
    }
}
