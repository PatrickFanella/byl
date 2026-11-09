import Foundation
import Observation
import UIKit
import InvisibleMuseumKit

/// App-wide state: API access, the session, the selected city, and the tour being walked.
@MainActor
@Observable
final class AppModel {
    static let defaultBaseURL = "https://byl.subcult.tv"
    static let baseURLDefaultsKey = "InvisibleMuseum.apiBaseURL"
    static let citySlugDefaultsKey = "InvisibleMuseum.citySlug"

    let client: APIClient
    let api: Endpoints
    let session: SessionController
    let location = MuseumLocationService()
    /// Application Support cache for the last city, tours, and the active tour's detail and route.
    let cache: DiskCache
    /// The audio guide: recorded narration or the body read aloud.
    let narration = NarrationPlayer(session: SpokenAudioSession(), synthesizer: SystemSpeechSynthesizer())
    /// Remembered across tours.
    var audioGuideEnabled: Bool {
        get { UserDefaults.standard.bool(forKey: "InvisibleMuseum.audioGuide") }
        set { UserDefaults.standard.set(newValue, forKey: "InvisibleMuseum.audioGuide") }
    }

    var account: Account?
    var cities: [City] = []
    var selectedCity: City?
    /// The tour being walked or ridden, if any.
    var active: ActiveTourModel?
    var contributions: TripContributions?
    var contributionError: String?
    var contributionSessionVersion = 0
    var appIsActive = true
    private var privateContributionStore: ContributionStore?
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
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.account = self.session.account
                self.contributionSessionVersion += 1
                self.contributions?.identityChanged()
            }
        }
        location.onAuthorizationChange = { [weak self] state in
            if state != .authorized { self?.contributions?.lifecycle(.permissionLost) }
        }
        client.onUnauthorized = { [weak self] token in
            Task { @MainActor [weak self] in self?.handleUnauthorized(token: token) }
        }
    }

    static func resolveBaseURL() -> String {
        if let override = UserDefaults.standard.string(forKey: baseURLDefaultsKey), !override.isEmpty { return override }
        if let env = ProcessInfo.processInfo.environment[baseURLDefaultsKey], !env.isEmpty { return env }
        if let plist = Bundle.main.object(forInfoDictionaryKey: baseURLDefaultsKey) as? String, !plist.isEmpty { return plist }
        return defaultBaseURL
    }

    /// Tracked through `account`, so views switch between signed-in and signed-out states.
    var isSignedIn: Bool { account != nil }
    /// Set when a 401 arrives during an active tour; sign-in is shown once the tour screen closes.
    private var signInAfterTour = false

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
        resumeActiveTour()
        #if DEBUG
        devSignInAvailable = true
        #endif
    }

    /// Brings back the tour that was being walked when the app was last closed (Review Focus 2).
    func resumeActiveTour() {
        let accountID = session.account?.id
        let token = session.token
        let origin = client.baseURL
        guard active == nil, let restored = ActiveTourModel.restore(api: activeTourAPI(), cache: cache, isSignedIn: { [weak self] in
            self?.isSignedIn == true && self?.session.account?.id == accountID && self?.session.token == token && self?.client.baseURL == origin
        }, accountID: accountID, isCurrentSession: { [weak self] in
            self?.session.account?.id == accountID && self?.session.token == token && self?.client.baseURL == origin
        }) else { return }
        attach(restored)
    }

    /// Foreground: push any progress the server has not seen yet.
    func didBecomeActive() {
        appIsActive = true
        if let contributions { Task { await contributions.refresh() } }
        guard let active else { return }
        Task {
            await active.loadRoute()
            if active.revoked { narration.stop() }
            await active.syncIfNeeded()
        }
    }

    func didLeaveForeground() {
        appIsActive = false
        contributions?.setForeground(false)
    }

    func contributionModel(for trip: ActiveTourModel? = nil) -> TripContributions? {
        do {
            let store: ContributionStore
            if let existing = privateContributionStore { store = existing }
            else {
                // Do not use navigation's temporary-cache fallback for erasure capabilities.
                let support = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
                store = try ContributionStore(directory: support.appendingPathComponent("InvisibleMuseum/RouteContributions", isDirectory: true))
                privateContributionStore = store
            }
            let origin = trip?.api.client.baseURL ?? client.baseURL
            let owner = trip == nil ? session.account?.id : trip?.owningAccountID
            let token: String?
            if let trip { token = trip.api.client.tokenProvider() }
            else { token = session.token }
            let controller = TripContributions(store: store, api: ContributionHTTPAPI(origin: origin, token: token), owner: owner, origin: origin, revision: trip?.revision, mode: trip?.mode ?? "walk", sameIdentity: { [weak self] in
                guard let self, owner != nil else { return false }
                return self.session.account?.id == owner && self.session.token == token && self.client.baseURL == origin
            })
            contributionError = nil
            return controller
        } catch {
            contributionError = "Private contribution storage is unavailable. Route improvement is off; navigation still works."
            return nil
        }
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
        active?.transit?.invalidate()
        try? await api.signOut()
        session.clear()
        account = nil
        savedTourIDs = []
    }

    /// A 401 clears the session (only when the rejected token is still the current one) and
    /// shows sign-in, but never touches the tour being walked (Review Focus 3).
    func handleUnauthorized(token: String?) {
        guard session.isSignedIn, token == nil || token == session.token else { return }
        active?.transit?.invalidate()
        session.clear()
        account = nil
        savedTourIDs = []
        if active == nil {
            presentSignIn = true
        } else {
            signInAfterTour = true
        }
    }

    func refreshSaved() async {
        guard session.isSignedIn, let saved = try? await api.saved() else { return }
        savedTourIDs = Set(saved.flatMap { [$0.id, $0.slug] })
    }

    func setSaved(_ tourID: String, _ saved: Bool) {
        if saved { savedTourIDs.insert(tourID) } else { savedTourIDs.remove(tourID) }
    }

    /// Makes a tour the active one; RootView presents ActiveTourView over everything.
    func startTour(_ tour: TourDetail, mode: String) async throws {
        guard let revisionID = tour.tour.revisionID else {
            throw APIError(status: 0, code: "revision_unavailable", message: "This tour has no reviewed revision to start.")
        }
        let accountID = session.account?.id
        let token = session.token
        let origin = client.baseURL
        let api = activeTourAPI()
        let revision = try await api.revision(tour: tour.id, revision: revisionID)
        guard session.account?.id == accountID, session.token == token, client.baseURL == origin, active == nil,
              revision.id == revisionID, revision.tourID == tour.id else {
            throw APIError(status: 0, code: "trip_changed", message: "The trip or session changed. Open the tour again before starting.")
        }
        let transitEnabled = revision.isMixed(mode: mode) ? try await api.itineraryCapabilities().scheduledTransitV1 : nil
        guard session.account?.id == accountID, session.token == token, client.baseURL == origin, active == nil else {
            throw APIError(status: 0, code: "trip_changed", message: "The session changed. Open the tour again.")
        }
        let model = try ActiveTourModel(api: api, revision: revision, mode: mode, cache: cache, isSignedIn: { [weak self] in
            self?.isSignedIn == true && self?.session.account?.id == accountID && self?.session.token == token && self?.client.baseURL == origin
        }, accountID: accountID, transitEnabled: transitEnabled, isCurrentSession: { [weak self] in
            self?.session.account?.id == accountID && self?.session.token == token && self?.client.baseURL == origin
        })
        attach(model)
    }

    func adoptUpdate(for expected: ActiveTourModel) async throws {
        guard let candidate = expected.availableUpdate else {
            throw APIError(status: 0, code: "revision_unavailable", message: "Check for a reviewed update first.")
        }
        let confirmed = try await expected.api.revision(tour: expected.tour.id, revision: candidate.id)
        await expected.flush()
        guard active === expected, expected.api.client.baseURL == client.baseURL,
              expected.owningAccountID == session.account?.id,
              expected.api.client.tokenProvider() == session.token,
              expected.availableUpdate?.id == candidate.id, confirmed.id == candidate.id,
              confirmed.tourID == expected.tour.id, confirmed.unavailableReason(mode: expected.mode) == nil else {
            throw APIError(status: 0, code: "trip_changed", message: "The active trip or session changed. Check the trip again.")
        }
        let transitEnabled = candidate.isMixed(mode: expected.mode) ? try await expected.api.itineraryCapabilities().scheduledTransitV1 : nil
        guard active === expected, expected.owningAccountID == session.account?.id,
              expected.availableUpdate?.id == candidate.id,
              expected.api.client.tokenProvider() == session.token, expected.api.client.baseURL == client.baseURL else {
            throw APIError(status: 0, code: "trip_changed", message: "The session changed. Check the trip again.")
        }
        let replacement = try expected.adoptingUpdate(transitEnabled: transitEnabled)
        expected.end()
        narration.stop()
        attach(replacement)
    }

    // A queued progress request must keep the original account token and server.
    private func activeTourAPI() -> Endpoints {
        let token = session.token
        let origin = client.baseURL
        let scoped = APIClient(baseURL: origin, tokenProvider: { token })
        scoped.onUnauthorized = { [weak self] rejected in
            Task { @MainActor [weak self] in
                guard let self, self.client.baseURL == origin else { return }
                self.handleUnauthorized(token: rejected)
            }
        }
        return Endpoints(client: scoped)
    }

    func routeSuggestions(accountID: String) -> RouteSuggestionsModel {
        let token = session.token
        let origin = client.baseURL
        let scoped = APIClient(baseURL: origin, tokenProvider: { token })
        scoped.onUnauthorized = { [weak self] rejected in
            Task { @MainActor [weak self] in
                guard let self, self.client.baseURL == origin else { return }
                self.handleUnauthorized(token: rejected)
            }
        }
        return RouteSuggestionsModel(api: Endpoints(client: scoped), cache: cache, accountID: accountID) { [weak self] in
            guard let self else { return false }
            return self.account?.id == accountID && self.session.token == token && self.client.baseURL == origin
        }
    }

    func routeReports(accountID: String, context: RouteReportContext? = nil, initialTarget: String? = nil) -> RouteReportsModel {
        let token = session.token
        let origin = client.baseURL
        return RouteReportsModel(api: activeTourAPI(), cache: cache, accountID: accountID, context: context, initialTarget: initialTarget) { [weak self] in
            guard let self else { return false }
            return self.account?.id == accountID && self.session.token == token && self.client.baseURL == origin
        }
    }

    private func attach(_ model: ActiveTourModel) {
        contributions?.lifecycle(.end)
        contributions?.setForeground(false)
        contributions = contributionModel(for: model)
        model.audioEnabled = audioGuideEnabled
        model.onArrive = { [weak self, weak model] stop in
            guard let self, let model, model.audioEnabled else { return }
            Task { await self.playNarration(for: stop, requireGuide: true) }
        }
        location.onLocation = { [weak self, weak model] location in
            guard let self, let model, self.active === model else { return }
            model.handle(location: location)
            guard let contributions = self.contributions else { return }
            if model.revoked || model.isComplete { contributions.lifecycle(.end); return }
            if model.arrivedStop != nil { contributions.lifecycle(.manualArrival); return }
            guard UIApplication.shared.applicationState == .active else { contributions.setForeground(false); return }
            contributions.observe(DeviationFix(point: DeviationPoint(lat: location.coordinate.latitude, lng: location.coordinate.longitude), accuracyM: location.horizontalAccuracy, timestampMs: (location.timestamp.timeIntervalSince1970 * 1000).rounded(.down)), authorizationGranted: self.location.authorizationState == .authorized)
        }
        if let here = location.currentLocation { model.handle(location: here) }
        active = model
        Task {
            await model.loadRemoteProgress()
            await model.syncIfNeeded()
        }
    }

    func endTour(resetProgress: Bool) {
        contributions?.lifecycle(.end)
        contributions?.setForeground(false)
        contributions = nil
        let ending = active
        if resetProgress { active?.resetProgress() }
        active?.end()
        narration.stop()
        location.onLocation = nil
        location.stop()
        active = nil
        Task { await ending?.flush() }
        if signInAfterTour {
            signInAfterTour = false
            presentSignIn = true
        }
    }

    /// Plays the featured moment at a stop: its recorded narration, else the body read aloud.
    /// The summary already says whether narration exists; the detail carries the URL and body.
    func playNarration(for stop: TourStop, requireGuide: Bool = false) async {
        guard let expected = active, !expected.revoked, let featured = stop.featuredMoment else { return }
        let cacheKey = "moment.\(featured.id)"
        var detail: MomentDetail?
        if expected.revision != nil || expected.legacyResume {
            detail = expected.retainedMoment(featured.id)
        } else {
            detail = try? await api.moment(featured.id)
            if let detail { cache.save(detail, key: cacheKey) } else { detail = cache.load(MomentDetail.self, key: cacheKey) }
        }
        guard active === expected, !expected.revoked, (!requireGuide || expected.audioEnabled), expected.arrivedStop?.id == stop.id else { return }
        let source = NarrationPlayer.plan(narrationURL: detail?.narration?.url, body: detail?.body ?? featured.excerpt)
        narration.play(source)
    }

    func setAudioGuide(_ on: Bool) {
        audioGuideEnabled = on
        active?.audioEnabled = on
        if !on {
            narration.stop()
        } else if let stop = active?.arrivedStop {
            Task { await playNarration(for: stop, requireGuide: true) }
        }
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
        contributionSessionVersion += 1
        contributions?.identityChanged()
    }
}
