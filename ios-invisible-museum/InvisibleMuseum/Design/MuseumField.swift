import SwiftUI

struct MuseumTextField: View {
    let label: String
    let placeholder: String
    @Binding var text: String
    var helperText: String? = nil
    var helperTint: Color = Museum.muted
    var helperIsError: Bool = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Overline(text: label)
            TextField(placeholder, text: $text)
                .textFieldStyle(.plain)
                .foregroundStyle(Museum.text)
                .padding(15)
                .museumGlassCard(radius: Museum.Radius.md)
            if let helperText {
                MuseumInlineHint(text: helperText, tint: helperTint, isError: helperIsError)
            }
        }
    }
}

struct MuseumTextEditor: View {
    let label: String
    let placeholder: String
    @Binding var text: String
    var minHeight: CGFloat = 140
    var helperText: String? = nil
    var helperTint: Color = Museum.muted
    var helperIsError: Bool = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Overline(text: label)
            TextEditor(text: $text)
                .frame(minHeight: minHeight)
                .scrollContentBackground(.hidden)
                .foregroundStyle(Museum.text)
                .padding(10)
                .overlay(alignment: .topLeading) {
                    if text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        Text(placeholder)
                            .font(.system(size: 15))
                            .foregroundStyle(Museum.faint)
                            .padding(.top, 18)
                            .padding(.leading, 18)
                            .allowsHitTesting(false)
                    }
                }
                .museumGlassCard(radius: Museum.Radius.md)
            if let helperText {
                MuseumInlineHint(text: helperText, tint: helperTint, isError: helperIsError)
            }
        }
    }
}

struct MuseumInlineHint: View {
    let text: String
    var tint: Color = Museum.muted
    var isError: Bool = false

    var body: some View {
        HStack(alignment: .top, spacing: 6) {
            Image(systemName: isError ? "exclamationmark.circle.fill" : "info.circle")
                .font(.system(size: 11, weight: .semibold))
                .foregroundStyle(tint)
                .padding(.top, 1)
            Text(text)
                .font(.system(size: 11, weight: .medium))
                .foregroundStyle(tint)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 2)
    }
}
