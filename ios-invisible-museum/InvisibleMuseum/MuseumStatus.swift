import SwiftUI
import InvisibleMuseumKit

/// Loading, empty, and error states shared by every screen.
struct MuseumLoadingView: View {
    var text = "Loading…"

    var body: some View {
        HStack(spacing: 12) {
            ProgressView().tint(Museum.text)
            Text(text)
                .font(Museum.bodyFont(14))
                .foregroundStyle(Museum.muted)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 18)
    }
}

struct MuseumEmptyView: View {
    let title: String
    var message: String?
    var symbol = "sparkles"

    var body: some View {
        MuseumCard {
            VStack(alignment: .leading, spacing: 8) {
                Image(systemName: symbol)
                    .font(.system(size: 22, weight: .light))
                    .foregroundStyle(Museum.muted)
                Text(title)
                    .font(Museum.title(17))
                    .foregroundStyle(Museum.text)
                if let message {
                    Text(message)
                        .font(Museum.bodyFont(14))
                        .foregroundStyle(Museum.muted)
                }
            }
        }
    }
}

struct MuseumErrorView: View {
    let message: String
    var retry: (() -> Void)?

    var body: some View {
        MuseumCard(stroke: Museum.danger.opacity(0.5)) {
            VStack(alignment: .leading, spacing: 10) {
                Overline(text: "Something went wrong", color: Museum.danger)
                Text(message)
                    .font(Museum.bodyFont(14))
                    .foregroundStyle(Museum.text)
                if let retry {
                    Button("Try again", action: retry)
                        .buttonStyle(.museum(.secondary, size: .small))
                }
            }
        }
    }
}

/// A generic async load state for screens that fetch one value.
enum Loadable<Value> {
    case idle
    case loading
    case loaded(Value)
    case failed(String)

    var value: Value? {
        if case .loaded(let v) = self { return v }
        return nil
    }

    var isLoading: Bool {
        if case .loading = self { return true }
        return false
    }
}

extension Error {
    /// The API's own message when there is one, else the system description.
    var museumMessage: String {
        if let api = self as? APIError { return api.message }
        return localizedDescription
    }
}
