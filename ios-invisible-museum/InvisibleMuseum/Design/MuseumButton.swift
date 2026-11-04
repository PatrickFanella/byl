import SwiftUI

struct MuseumButtonStyle: ButtonStyle {
    enum Variant { case primary, secondary, ghost, destructive }
    enum Size { case small, regular, large, icon }

    var variant: Variant = .primary
    var size: Size = .regular

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(font)
            .foregroundStyle(foregroundStyle)
            .frame(maxWidth: size == .icon ? nil : .infinity)
            .padding(.horizontal, horizontalPadding)
            .padding(.vertical, verticalPadding)
            .background(backgroundStyle(configuration.isPressed), in: shape)
            .overlay(shape.stroke(borderStyle, lineWidth: 1))
            .contentShape(shape)
            .scaleEffect(configuration.isPressed ? 0.985 : 1)
            .opacity(configuration.isPressed ? 0.92 : 1)
            .animation(.snappy(duration: 0.18), value: configuration.isPressed)
    }

    private var shape: RoundedRectangle { RoundedRectangle(cornerRadius: Museum.Radius.md, style: .continuous) }

    private var font: Font {
        switch size {
        case .small: .system(size: 13, weight: .semibold)
        case .regular: .system(size: 15, weight: .semibold)
        case .large: .system(size: 16, weight: .bold)
        case .icon: .system(size: 15, weight: .bold)
        }
    }

    private var horizontalPadding: CGFloat {
        switch size {
        case .small: 12
        case .regular: 14
        case .large: 16
        case .icon: 0
        }
    }

    private var verticalPadding: CGFloat {
        switch size {
        case .small: 8
        case .regular: 12
        case .large: 15
        case .icon: 0
        }
    }

    private var foregroundStyle: Color {
        switch variant {
        case .primary: .black
        case .secondary, .ghost: Museum.text
        case .destructive: .white
        }
    }

    private func backgroundStyle(_ pressed: Bool) -> AnyShapeStyle {
        switch variant {
        case .primary:
            return AnyShapeStyle(Color.white.opacity(pressed ? 0.9 : 1))
        case .secondary:
            return AnyShapeStyle(Museum.surfaceElevated.opacity(pressed ? 0.92 : 1))
        case .ghost:
            return AnyShapeStyle(Color.clear)
        case .destructive:
            return AnyShapeStyle(Museum.danger.opacity(pressed ? 0.88 : 1))
        }
    }

    private var borderStyle: Color {
        switch variant {
        case .primary, .destructive:
            return .clear
        case .secondary, .ghost:
            return Museum.border
        }
    }
}

extension ButtonStyle where Self == MuseumButtonStyle {
    static func museum(_ variant: MuseumButtonStyle.Variant = .primary, size: MuseumButtonStyle.Size = .regular) -> MuseumButtonStyle {
        MuseumButtonStyle(variant: variant, size: size)
    }
}
