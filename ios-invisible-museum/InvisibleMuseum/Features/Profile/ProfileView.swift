import SwiftUI
import InvisibleMuseumKit

/// Account, audio guide preference, API base URL override, and sign out.
struct ProfileView: View {
    @AppStorage("byl.appearance") private var appearance: BylAppearance = .system
    @Environment(AppModel.self) private var model
    @State private var baseURL = AppModel.resolveBaseURL()
    @State private var audioGuide = false
    @State private var signingOut = false

    var body: some View {
        NavigationStack {
            MuseumScreen {
                VStack(alignment: .leading, spacing: 10) {
                    Overline(text: "Profile")
                    Text(model.account?.displayName.isEmpty == false ? model.account!.displayName : "Visitor")
                        .font(Museum.title(30))
                        .foregroundStyle(Museum.text)
                }
                account
                MuseumCard {
                    Picker("Appearance", selection: $appearance) {
                        ForEach(BylAppearance.allCases) { item in
                            Text(item.label).tag(item)
                        }
                    }
                    .font(Museum.bodyFont())
                    .foregroundStyle(Museum.text)
                }
                preferences
                developer
                about
            }
            .onAppear { audioGuide = model.audioGuideEnabled }
        }
    }

    private var account: some View {
        MuseumCard {
            VStack(alignment: .leading, spacing: 12) {
                Overline(text: "Account")
                if let account = model.account {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(account.email.isEmpty ? "Signed in with \(account.provider.capitalized)" : account.email)
                            .font(Museum.bodyFont(15))
                            .foregroundStyle(Museum.text)
                        Text("\(account.provider.capitalized) · \(account.role)")
                            .font(Museum.meta(11))
                            .foregroundStyle(Museum.muted)
                    }
                    Button(signingOut ? "Signing out…" : "Sign out") {
                        signingOut = true
                        Task { await model.signOut(); signingOut = false }
                    }
                    .buttonStyle(.museum(.secondary))
                    .disabled(signingOut)
                } else {
                    Text("Sign in to save tours, keep progress, and build routes. Browsing and walking tours work without an account.")
                        .font(Museum.bodyFont(14))
                        .foregroundStyle(Museum.muted)
                    Button("Sign in") { model.presentSignIn = true }
                        .buttonStyle(.museum(.primary))
                }
            }
        }
    }

    private var preferences: some View {
        MuseumCard {
            VStack(alignment: .leading, spacing: 12) {
                Overline(text: "On the tour")
                Toggle(isOn: $audioGuide) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Audio guide")
                            .font(Museum.bodyFont(15, weight: .semibold))
                            .foregroundStyle(Museum.text)
                        Text("Play each stop's story when you arrive. Recorded narration when there is one, otherwise read aloud.")
                            .font(Museum.bodyFont(12))
                            .foregroundStyle(Museum.muted)
                    }
                }
                .tint(Museum.action)
                .onChange(of: audioGuide) { _, on in model.setAudioGuide(on) }
                HStack {
                    Text("Location")
                        .font(Museum.bodyFont(15, weight: .semibold))
                        .foregroundStyle(Museum.text)
                    Spacer()
                    Text(locationLabel)
                        .font(Museum.bodyFont(13))
                        .foregroundStyle(Museum.muted)
                }
                if model.location.authorizationState != .authorized, let url = URL(string: UIApplication.openSettingsURLString) {
                    Link("Open Settings", destination: url)
                        .font(Museum.bodyFont(13, weight: .semibold))
                        .foregroundStyle(Museum.action)
                }
            }
        }
    }

    private var developer: some View {
        MuseumCard {
            VStack(alignment: .leading, spacing: 12) {
                Overline(text: "Server", color: Museum.info)
                MuseumTextField(label: "API base URL", placeholder: AppModel.defaultBaseURL, text: $baseURL,
                                helperText: "Leave empty for \(AppModel.defaultBaseURL). Changing it signs you out.")
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .keyboardType(.URL)
                HStack(spacing: 8) {
                    Button("Apply") {
                        Task {
                            await model.signOut()
                            model.setBaseURL(baseURL)
                            baseURL = AppModel.resolveBaseURL()
                            await model.loadCities()
                        }
                    }
                    .buttonStyle(.museum(.secondary, size: .small))
                    .disabled(baseURL.trimmingCharacters(in: .whitespaces) == AppModel.resolveBaseURL())
                    Button("Reset") {
                        baseURL = ""
                        Task {
                            await model.signOut()
                            model.setBaseURL("")
                            baseURL = AppModel.resolveBaseURL()
                            await model.loadCities()
                        }
                    }
                    .buttonStyle(.museum(.ghost, size: .small))
                }
            }
        }
    }

    private var about: some View {
        MuseumCard(padding: Museum.Spacing.md) {
            VStack(alignment: .leading, spacing: 6) {
                Overline(text: "About")
                Text("Byl \(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "")")
                    .font(Museum.bodyFont(14, weight: .semibold))
                    .foregroundStyle(Museum.text)
                Text("Stories are drawn from open archives and reviewed by curators; each moment lists its sources and media credits.")
                    .font(Museum.bodyFont(12))
                    .foregroundStyle(Museum.muted)
            }
        }
    }

    private var locationLabel: String {
        switch model.location.authorizationState {
        case .authorized: return "Allowed while using"
        case .denied: return "Off"
        case .unknown: return "Not asked yet"
        }
    }
}
