import SwiftUI

/// Semantic roles from Studio's Classic solid / P3 Oxblood specification.
enum Museum {
    static let background = Color("BylBackground")
    static let ink = background
    static let surface = Color("BylSurface")
    static let surfaceElevated = surface
    static let border = Color("BylBorderSubtle")
    static let borderStrong = Color("BylBorderControl")
    static let text = Color("BylText")
    static let muted = Color("BylTextSecondary")
    static let faint = muted
    static let action = Color("BylAction")
    static let actionText = Color("BylActionText")
    static let success = Color("BylSuccess")
    static let successSurface = Color("BylSuccessSurface")
    static let danger = Color("BylDanger")
    static let dangerSurface = Color("BylDangerSurface")
    static let info = Color("BylInfo")
    static let warning = Color("BylWarning")
    static let routeHalo = Color("BylRouteHalo")

    enum Radius {
        static let sm: CGFloat = 4
        static let md: CGFloat = 4
        static let lg: CGFloat = 8
        static let xl: CGFloat = 0
        static let pill: CGFloat = 999
    }

    enum Spacing {
        static let xs: CGFloat = 4
        static let sm: CGFloat = 8
        static let md: CGFloat = 12
        static let lg: CGFloat = 16
        static let xl: CGFloat = 24
        static let pageHorizontal: CGFloat = 16
        static let pageTop: CGFloat = 16
        static let section: CGFloat = 24
        static let bottomNavClearance: CGFloat = 32
    }

    static var galleryBackground: some View { background }
    // Kept as a shape style for existing image placeholders; no decorative gradient.
    static var accentGradient: LinearGradient {
        LinearGradient(colors: [surface, surface], startPoint: .top, endPoint: .bottom)
    }

    static func accent(for hue: Double) -> Color { action }

    static func title(_ size: CGFloat, weight: Font.Weight = .bold) -> Font {
        .custom("LibreBaskerville-Regular", size: size, relativeTo: size >= 28 ? .largeTitle : .title2)
    }

    static func bodyFont(_ size: CGFloat = 16, weight: Font.Weight = .regular) -> Font {
        .custom(weight == .regular ? "PublicSans-Regular" : "PublicSans-Bold", size: size, relativeTo: .body)
    }

    static func meta(_ size: CGFloat = 12) -> Font {
        .custom("PublicSans-Bold", size: max(size, 12), relativeTo: .caption)
    }

    static func serifTitle(_ size: CGFloat) -> Font { title(size) }
    static let label: Font = .custom("PublicSans-Bold", size: 12, relativeTo: .caption)
}

enum BylAppearance: String, CaseIterable, Identifiable {
    case system, light, dark
    var id: String { rawValue }
    var label: String { rawValue.capitalized }
    var colorScheme: ColorScheme? {
        switch self {
        case .system: nil
        case .light: .light
        case .dark: .dark
        }
    }
}

struct Overline: View {
    let text: String
    var color: Color = Museum.action
    var body: some View {
        Text(text)
            .font(Museum.label)
            .foregroundStyle(color)
    }
}

extension View {
    func museumSurface(radius: CGFloat = Museum.Radius.lg, fill: Color = Museum.surface) -> some View {
        background(fill, in: RoundedRectangle(cornerRadius: radius))
            .overlay(RoundedRectangle(cornerRadius: radius).stroke(Museum.border, lineWidth: 1))
    }

    func museumGlassCard(radius: CGFloat = Museum.Radius.lg, fill: Color = Museum.surface) -> some View {
        museumSurface(radius: radius, fill: fill)
    }
}
