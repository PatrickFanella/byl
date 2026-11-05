import SwiftUI

struct MuseumChip: View {
    let title: String
    var isSelected = false
    var icon: String?

    var body: some View {
        HStack(spacing: 6) {
            if let icon {
                Image(systemName: icon)
                    .font(.system(size: 11, weight: .bold))
            }
            Text(title)
        }
        .font(.system(size: 13, weight: .semibold))
        .foregroundStyle(isSelected ? .black : Museum.text.opacity(0.78))
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(isSelected ? Color.white : Museum.surfaceElevated, in: Capsule())
        .overlay(Capsule().stroke(isSelected ? Color.clear : Museum.border, lineWidth: 1))
    }
}

struct MuseumFlowLayout: Layout {
    var spacing: CGFloat = Museum.Spacing.sm

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? 0
        var currentX: CGFloat = 0
        var currentY: CGFloat = 0
        var rowHeight: CGFloat = 0
        var maxX: CGFloat = 0

        for subview in subviews {
            let size = subview.sizeThatFits(.unspecified)
            if currentX > 0, currentX + size.width > width {
                currentY += rowHeight + spacing
                currentX = 0
                rowHeight = 0
            }

            if currentX > 0 { currentX += spacing }
            currentX += size.width
            rowHeight = max(rowHeight, size.height)
            maxX = max(maxX, currentX)
        }

        return CGSize(width: width, height: currentY + rowHeight)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var x = bounds.minX
        var y = bounds.minY
        var rowHeight: CGFloat = 0

        for subview in subviews {
            let size = subview.sizeThatFits(.unspecified)
            if x > bounds.minX, x + size.width > bounds.maxX {
                y += rowHeight + spacing
                x = bounds.minX
                rowHeight = 0
            }

            subview.place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
            x += size.width + spacing
            rowHeight = max(rowHeight, size.height)
        }
    }
}

typealias Chip = MuseumChip
