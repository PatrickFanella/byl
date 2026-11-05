import SwiftUI
import AuthenticationServices
import InvisibleMuseumKit

struct SignInView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var error: String?
    @State private var busy = false
    @State private var devEmail = ""
    @State private var devName = ""

    var body: some View {
        MuseumScreen(bottomInset: 40) {
            VStack(alignment: .leading, spacing: 8) {
                Overline(text: "Sign in")
                Text("Keep your routes and progress")
                    .font(Museum.title(28))
                    .foregroundStyle(Museum.text)
                Text("Sign in to build and share your own routes, save tours, and pick up where you left off on any device.")
                    .font(Museum.bodyFont(15))
                    .foregroundStyle(Museum.muted)
            }

            SignInWithAppleButton(.signIn) { request in
                request.requestedScopes = [.fullName, .email]
            } onCompletion: { result in
                handleApple(result)
            }
            .signInWithAppleButtonStyle(.white)
            .frame(height: 50)
            .clipShape(RoundedRectangle(cornerRadius: Museum.Radius.md, style: .continuous))

            if model.devSignInAvailable {
                MuseumCard {
                    VStack(alignment: .leading, spacing: 12) {
                        Overline(text: "Development sign-in", color: Museum.info)
                        MuseumTextField(label: "Email", placeholder: "you@example.com", text: $devEmail)
                        MuseumTextField(label: "Name", placeholder: "Optional", text: $devName)
                        Button {
                            Task { await devSignIn() }
                        } label: {
                            Text(busy ? "Signing in…" : "Sign in with email")
                        }
                        .buttonStyle(.museum(.secondary))
                        .disabled(busy || !devEmail.contains("@"))
                        MuseumInlineHint(text: "Works only against an API running with DEV_AUTH_ENABLED.")
                    }
                }
            }

            if let error {
                MuseumInlineHint(text: error, tint: Museum.danger, isError: true)
            }

            Button("Not now") { dismiss() }
                .buttonStyle(.museum(.ghost))
        }
    }

    private func handleApple(_ result: Result<ASAuthorization, Error>) {
        switch result {
        case .failure(let err):
            if (err as? ASAuthorizationError)?.code != .canceled {
                error = err.localizedDescription
            }
        case .success(let auth):
            guard let credential = auth.credential as? ASAuthorizationAppleIDCredential,
                  let tokenData = credential.identityToken,
                  let token = String(data: tokenData, encoding: .utf8) else {
                error = "Apple did not return an identity token."
                return
            }
            let name = [credential.fullName?.givenName, credential.fullName?.familyName].compactMap { $0 }.joined(separator: " ")
            Task { await exchange { try await model.api.signInWithApple(identityToken: token, displayName: name) } }
        }
    }

    private func devSignIn() async {
        await exchange { try await model.api.signInDev(email: devEmail, displayName: devName) }
    }

    private func exchange(_ call: () async throws -> Session) async {
        busy = true
        defer { busy = false }
        do {
            let session = try await call()
            model.signIn(with: session)
            dismiss()
        } catch let apiError as APIError {
            error = apiError.status == 404 ? "Development sign-in is disabled on this server." : apiError.message
        } catch {
            self.error = error.localizedDescription
        }
    }
}
