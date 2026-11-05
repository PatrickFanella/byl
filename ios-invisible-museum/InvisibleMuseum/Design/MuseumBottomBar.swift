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
        HStack(spacing: 0) {
            ForEach(MuseumTab.allCases, id: \.self) { item in
                tabButton(item)
            }
        }
        .padding(8)
        .background(Museum.background)
        .overlay(alignment: .top) { Rectangle().fill(Museum.border).frame(height: 1) }
        .padding(.horizontal, Museum.Spacing.pageHorizontal)
        .padding(.vertical, 8)
    }

    private func tabButton(_ item: MuseumTab) -> some View {
        Button {
            withAnimation(.snappy) {
                selection = item
            }
        } label: {
            VStack(spacing: 4) {
                Image(systemName: item.iconName)
                Text(item.rawValue)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .font(Museum.meta(12))
            .foregroundStyle(selection == item ? Museum.action : Museum.muted)
            .frame(maxWidth: .infinity)
            .padding(.vertical, 11)
            .padding(.horizontal, 4)
            .frame(minHeight: 44)
            .background(selection == item ? Museum.surface : Color.clear, in: RoundedRectangle(cornerRadius: 4))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(item.rawValue)
        .accessibilityAddTraits(selection == item ? [.isSelected] : [])
    }
}
