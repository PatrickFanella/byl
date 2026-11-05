import SwiftUI

enum MuseumTab: String, CaseIterable {
    case home = "Home"
    case tours = "Tours"
    case routes = "Routes"
    case saved = "Saved"
    case profile = "Profile"

    var iconName: String {
        switch self {
        case .home: "building.columns.fill"
        case .tours: "figure.walk"
        case .routes: "point.topleft.down.to.point.bottomright.curvepath.fill"
        case .saved: "heart.fill"
        case .profile: "person.crop.circle.fill"
        }
    }
}

struct MuseumBottomBar: View {
    @Binding var selection: MuseumTab

    var body: some View {
        HStack(spacing: 8) {
            ForEach(MuseumTab.allCases, id: \.self) { item in
                tabButton(item)
            }
        }
        .padding(8)
        .background(.black.opacity(0.78), in: Capsule())
        .overlay(Capsule().stroke(Museum.border, lineWidth: 1))
        .shadow(color: .black.opacity(0.35), radius: 20, x: 0, y: 10)
        .padding(.horizontal, Museum.Spacing.pageHorizontal)
        .padding(.vertical, 8)
    }

    private func tabButton(_ item: MuseumTab) -> some View {
        Button {
            withAnimation(.snappy) {
                selection = item
            }
        } label: {
            HStack(spacing: 7) {
                Image(systemName: item.iconName)
                if selection == item {
                    Text(item.rawValue)
                        .lineLimit(1)
                        .minimumScaleFactor(0.78)
                }
            }
            .font(.system(size: 12, weight: .bold))
            .foregroundStyle(selection == item ? .black : Color.white.opacity(0.62))
            .frame(maxWidth: .infinity)
            .padding(.vertical, 11)
            .padding(.horizontal, selection == item ? 10 : 8)
            .background(selection == item ? Museum.blue : Color.clear, in: Capsule())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(item.rawValue)
    }
}
