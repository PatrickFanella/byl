import SwiftUI

enum Museum {
    static let background = Color(red: 0.035, green: 0.035, blue: 0.043)
    static let ink = background
    static let surface = Color(red: 0.070, green: 0.070, blue: 0.082)
    static let surfaceElevated = Color(red: 0.105, green: 0.105, blue: 0.120)
    static let border = Color.white.opacity(0.09)
    static let borderStrong = Color.white.opacity(0.18)
    static let text = Color.white
    static let muted = Color.white.opacity(0.54)
    static let faint = Color.white.opacity(0.34)
    static let blue = Color(red: 0.231, green: 0.510, blue: 0.965)
    static let purple = Color(red: 0.659, green: 0.333, blue: 0.969)
    static let green = Color(red: 0.290, green: 0.871, blue: 0.502)
    static let danger = Color(red: 0.865, green: 0.220, blue: 0.343)

    enum Radius {
        static let sm: CGFloat = 14
        static let md: CGFloat = 18
        static let lg: CGFloat = 22
        static let xl: CGFloat = 28
        static let pill: CGFloat = 999
    }

    enum Spacing {
        static let xs: CGFloat = 6
        static let sm: CGFloat = 10
        static let md: CGFloat = 14
        static let lg: CGFloat = 18
        static let xl: CGFloat = 24
        static let pageHorizontal: CGFloat = 18
        static let pageTop: CGFloat = 12
        static let section: CGFloat = 22
        static let bottomNavClearance: CGFloat = 124
    }

    // Back-compat aliases for older views/components.
    static let slate = surface
    static let stone = surfaceElevated
    static let brass = blue
    static let brassGlow = Color(red: 0.576, green: 0.773, blue: 1.0)
    static let ghost = blue
    static let parchment = text

    static var galleryBackground: some View {
        ZStack {
            LinearGradient(
                colors: [Color(red: 0.050, green: 0.050, blue: 0.060), background],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
            RadialGradient(
                colors: [blue.opacity(0.18), .clear],
                center: .topLeading,
                startRadius: 0,
                endRadius: 460
            )
            RadialGradient(
                colors: [purple.opacity(0.10), .clear],
                center: .bottomTrailing,
                startRadius: 0,
                endRadius: 380
            )
        }
    }

    static var accentGradient: LinearGradient {
        LinearGradient(colors: [blue, purple], startPoint: .topLeading, endPoint: .bottomTrailing)
    }

    static var brassSheen: LinearGradient { accentGradient }

    static func accent(for hue: Double) -> Color {
        hue > 0.45 ? purple : blue
    }

    static func title(_ size: CGFloat, weight: Font.Weight = .bold) -> Font {
        .system(size: size, weight: weight, design: .default)
    }

    static func bodyFont(_ size: CGFloat = 15, weight: Font.Weight = .regular) -> Font {
        .system(size: size, weight: weight, design: .default)
    }

    static func meta(_ size: CGFloat = 10) -> Font {
        .system(size: size, weight: .bold, design: .rounded)
    }

    static func serifTitle(_ size: CGFloat) -> Font { title(size, weight: .bold) }
    static let label: Font = .system(size: 11, weight: .bold, design: .rounded)
}

struct Overline: View {
    let text: String
    var color: Color = Museum.blue

    var body: some View {
        Text(text.uppercased())
            .font(Museum.label)
            .tracking(1.8)
            .foregroundStyle(color)
    }
}

struct GlassCardModifier: ViewModifier {
    var radius: CGFloat = 22

    func body(content: Content) -> some View {
        content
            .background(Museum.surface.opacity(0.92), in: RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).stroke(Museum.border, lineWidth: 1))
    }
}

extension View {
    func glassCard(radius: CGFloat = 22) -> some View { modifier(GlassCardModifier(radius: radius)) }
}

extension View {
    func museumSurface(radius: CGFloat = Museum.Radius.lg, fill: Color = Museum.surface) -> some View {
        background(fill, in: RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).stroke(Museum.border, lineWidth: 1))
    }

    func museumGlassCard(radius: CGFloat = Museum.Radius.lg, fill: Color = Museum.surface.opacity(0.94)) -> some View {
        background(fill, in: RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous).stroke(Museum.border, lineWidth: 1))
    }

    func blueGlow(_ opacity: Double = 0.35) -> some View {
        shadow(color: Museum.blue.opacity(opacity), radius: 18, x: 0, y: 8)
    }

    func brassHalo() -> some View { blueGlow() }
}
