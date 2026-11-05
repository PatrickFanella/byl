import SwiftUI

struct MuseumSectionHeader: View {
    let title: String
    var actionTitle: String?
    var eyebrow: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let eyebrow {
                Overline(text: eyebrow)
            }

            HStack {
                Text(title)
                    .font(.system(size: 18, weight: .bold))
                    .foregroundStyle(Museum.text)
                Spacer()
                if let actionTitle {
                    Text(actionTitle)
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundStyle(Museum.muted)
                }
            }
        }
    }
}
