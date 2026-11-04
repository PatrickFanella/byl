import Foundation
import Observation
import InvisibleMuseumKit

/// App-wide state: API access, the session, the selected city, and the tour being walked.
@MainActor
@Observable
final class AppModel {
    static let defaultBaseURL = "https://museum-api.subcult.tv"
    static let baseURLDefaultsKey = "InvisibleMuseum.apiBaseURL"
    static let citySlugDefaultsKey = "InvisibleMuseum.citySlug"

    let client: APIClient
    let api: Endpoints
    let session: SessionController
    let location = MuseumLocationService()
    /// Application Support cache for the last city, tours, and the active tour's detail and route.
    let cache: DiskCache

    var account: Account?
    var cities: [City] = []
    var selectedCity: City?
    /// The tour being walked or ridden, if any.
    var active: ActiveTourModel?
    var activeTour: TourDetail? { active?.tour }
    var activeMode: String? { active?.mode }
    var activeTourID: String? { active?.tour.tour.id }
    var pendingTourSlug: String?
    /// Ids and slugs of the tours the signed-in account saved; refreshed on sign-in.
    var savedTourIDs: Set<String> = []
    var presentSignIn = false
    var bootstrapError: String?
    var devSignInAvailable = false

    init(storage: TokenStorage = KeychainTokenStorage(), cache: DiskCache? = nil) {
        let base = URL(string: Self.resolveBaseURL()) ?? URL(string: Self.defaultBaseURL)!
        let session = SessionController(storage: storage)
        let client = APIClient(baseURL: base, tokenProvider: { [session] in session.token })
        self.client = client
        self.api = Endpoints(client: client)
        self.session = session
        self.cache = cache ?? (try? DiskCache.applicationSupport())
            ?? (try! DiskCache(directory: FileManager.default.temporaryDirectory.appendingPathComponent("InvisibleMuseum")))
        self.account = nil
        session.onChange = { [weak self] in
            Task { @MainActor [weak self] in self?.account = self?.session.account }
        }
        client.onUnauthorized = { [weak self] in
            Task { @MainActor [weak self] in self?.handleUnauthorized() }
        }
    }

    static func resolveBaseURL() -> String {
        if let override = UserDefaults.standard.string(forKey: baseURLDefaultsKey), !override.isEmpty { return override }
        if let env = ProcessInfo.processInfo.environment[baseURLDefaultsKey], !env.isEmpty { return env }
        if let plist = Bundle.main.object(forInfoDictionaryKey: baseURLDefaultsKey) as? String, !plist.isEmpty { return plist }
        return defaultBaseURL
    }

    var isSignedIn: Bool { session.isSignedIn }

    func bootstrap() async {
        session.restore()
        account = session.account
        if session.isSignedIn {
            if let me = try? await api.me() {
                session.update(account: me)
            }
            await refreshSaved()
        }
        await loadCities()
        #if DEBUG
        devSignInAvailable = true
        #endif
    }

    func loadCities() async {
        do {
            cities = try await api.cities()
            cache.save(cities, key: "cities")
            bootstrapError = nil
            let remembered = UserDefaults.standard.string(forKey: Self.citySlugDefaultsKey)
            if let remembered, let match = cities.first(where: { $0.slug == remembered }) {
                selectedCity = match
            } else if selectedCity == nil {
                selectedCity = cities.first
            }
        } catch {
            // Offline: fall back to the last cities seen so an active tour can still be resumed.
            if cities.isEmpty, let cached = cache.load([City].self, key: "cities"), !cached.isEmpty {
                cities = cached
                let remembered = UserDefaults.standard.string(forKey: Self.citySlugDefaultsKey)
                selectedCity = cities.first { $0.slug == remembered } ?? cities.first
            }
            bootstrapError = (error as? APIError)?.message ?? error.localizedDescription
        }
    }

    func select(city: City) {
        selectedCity = city
        UserDefaults.standard.set(city.slug, forKey: Self.citySlugDefaultsKey)
    }

    func signIn(with session: Session) {
        self.session.set(session: session)
        account = session.account
        presentSignIn = false
        Task { await refreshSaved() }
    }

    func signOut() async {
        try? await api.signOut()
        session.clear()
        account = nil
        savedTourIDs = []
    }

    /// A 401 clears the session but never the tour being walked.
    func handleUnauthorized() {
        guard session.isSignedIn else { return }
        session.clear()
        account = nil
        savedTourIDs = []
    }

    func refreshSaved() async {
        guard session.isSignedIn, let saved = try? await api.saved() else { return }
        savedTourIDs = Set(saved.flatMap { [$0.id, $0.slug] })
    }

    func setSaved(_ tourID: String, _ saved: Bool) {
        if saved { savedTourIDs.insert(tourID) } else { savedTourIDs.remove(tourID) }
    }

    /// Makes a tour the active one; RootView presents ActiveTourView over everything.
    func startTour(_ tour: TourDetail, mode: String) {
        let model = ActiveTourModel(api: api, tour: tour, mode: mode, cache: cache, isSignedIn: { [weak self] in self?.isSignedIn ?? false })
        location.onLocation = { [weak model] location in model?.handle(location: location) }
        if let here = location.currentLocation { model.handle(location: here) }
        active = model
    }

    func endTour(resetProgress: Bool) {
        if resetProgress { active?.resetProgress() }
        location.onLocation = nil
        active = nil
    }

    func setBaseURL(_ value: String) {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty {
            UserDefaults.standard.removeObject(forKey: Self.baseURLDefaultsKey)
        } else {
            UserDefaults.standard.set(trimmed, forKey: Self.baseURLDefaultsKey)
        }
        if let url = URL(string: Self.resolveBaseURL()) {
            client.baseURL = url
        }
    }
}
