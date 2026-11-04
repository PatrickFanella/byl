import CoreLocation
import MapKit
import SwiftUI

struct MapListView: View {
    @Environment(MuseumStore.self) private var store
    private let onClose: (() -> Void)?
    private let persistence = MuseumLocalPersistence()
    @State private var path: [MuseumSpace] = []
    @State private var activeKindFilter: SpaceKind?
    @State private var selectedSpace: MuseumSpace?
    @State private var cameraPosition: MapCameraPosition = .automatic
    @State private var locationService = MuseumLocationService()
    @State private var routeService = WalkingRouteService()
    @State private var walkingRouteState: WalkingRouteState = .idle
    @State private var activeWalkingSpaceID: String?
    @State private var activeTourProgress: WalkingTourProgress?
    @State private var selectedTourID: String?
    @State private var restoredTourProgressTourIDs: Set<String> = []

    init(initialSelectedSpace: MuseumSpace? = nil, onClose: (() -> Void)? = nil) {
        self.onClose = onClose
        _selectedSpace = State(initialValue: initialSelectedSpace)
    }

    private var filteredSpaces: [MuseumSpace] {
        guard let activeKindFilter else { return store.spaces }
        return store.spaces.filter { $0.kind == activeKindFilter }
    }

    private var filterTitle: String {
        activeKindFilter?.label ?? "All"
    }

    private var mapSpaces: [MuseumSpace] {
        filteredSpaces.isEmpty ? store.spaces : filteredSpaces
    }

    var body: some View {
        NavigationStack(path: $path) {
            MuseumScreen {
                header
                statusBanner
                availableToursSection
                tourPreview
                mapPanel
                filterBar
                if let selectedSpace {
                    selectedSpaceCard(selectedSpace)
                }
                listHeader
                ForEach(filteredSpaces) { space in
                    NavigationLink(value: space) {
                        SpaceRow(space: space, isDownloaded: store.isDownloaded(space))
                    }
                    .buttonStyle(.plain)
                }
                if filteredSpaces.isEmpty {
                    emptyState
                }
            }
            .toolbar(.hidden, for: .navigationBar)
            .navigationDestination(for: MuseumSpace.self) { MuseumDetailView(space: $0) }
            .overlay(alignment: .topTrailing) {
                if let onClose {
                    Button(action: onClose) {
                        Image(systemName: "xmark")
                            .font(.system(size: 14, weight: .bold))
                            .foregroundStyle(Museum.text)
                            .frame(width: 42, height: 42)
                            .background(.black.opacity(0.42), in: Circle())
                            .overlay(Circle().stroke(Museum.border, lineWidth: 1))
                    }
                    .padding(.trailing, 16)
                    .padding(.top, 54)
                }
            }
            .onAppear {
                if locationService.authorizationState == .unknown {
                    locationService.requestWhenInUseAuthorization()
                }
                locationService.start()
                restoreTourProgressIfNeeded()
                updateCameraPosition()
            }
            .onDisappear {
                locationService.stop()
                routeService.cancel()
            }
            .onChange(of: selectedSpace?.id ?? "none") { _, _ in
                updateCameraPosition()
            }
            .onChange(of: activeKindFilter) { _, _ in
                if let selectedSpace, !filteredSpaces.contains(selectedSpace) {
                    self.selectedSpace = nil
                }
                updateCameraPosition()
            }
            .onChange(of: filteredSpaces.map(\.id)) { _, _ in
                if let selectedSpace, !filteredSpaces.contains(selectedSpace) {
                    self.selectedSpace = nil
                }
                updateCameraPosition()
            }
            .onChange(of: currentTour.id) { _, _ in
                restoreTourProgressIfNeeded()
                updateCameraPosition()
            }
            .onChange(of: locationService.currentLocation?.coordinate.latitude ?? 0) { _, _ in
                if selectedSpace == nil {
                    updateCameraPosition()
                }
            }
            .onChange(of: locationService.currentLocation?.coordinate.longitude ?? 0) { _, _ in
                if selectedSpace == nil {
                    updateCameraPosition()
                }
            }
            .task(id: routeTaskKey) {
                await updateWalkingRoute()
            }
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 8) {
            Overline(text: "Map view")
            Text("Downers Grove and nearby")
                .font(Museum.title(30))
                .foregroundStyle(Museum.text)
            Text("Browse the area map, then jump into any nearby space.")
                .font(Museum.bodyFont(14))
                .foregroundStyle(Museum.muted)
        }
    }

    @ViewBuilder
    private var statusBanner: some View {
        switch store.nearbySpacesState {
        case .loading:
            compactStatusRow(
                icon: nil,
                title: "Syncing live spaces",
                subtitle: "Loading backend map data…",
                tint: Museum.blue,
                trailing: AnyView(ProgressView().tint(Museum.blue))
            )
        case .loadedFromBackend:
            compactStatusRow(
                icon: "checkmark.seal.fill",
                title: "Live backend",
                subtitle: "Nearby spaces synced.",
                tint: Museum.green
            )
        case .sampleFallback(let reason):
            compactStatusRow(
                icon: "exclamationmark.triangle.fill",
                title: "Offline sample mode",
                subtitle: reason,
                tint: Museum.purple
            )
        }
    }

    private func compactStatusRow(icon: String?, title: String, subtitle: String, tint: Color, trailing: AnyView = AnyView(EmptyView())) -> some View {
        HStack(spacing: 12) {
            if let icon {
                Image(systemName: icon)
                    .font(.system(size: 14, weight: .bold))
                    .foregroundStyle(tint)
                    .frame(width: 24, height: 24)
                    .background(tint.opacity(0.14), in: Circle())
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(Museum.text)
                Text(subtitle)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundStyle(Museum.muted)
                    .lineLimit(1)
            }
            Spacer(minLength: 8)
            trailing
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .museumGlassCard(radius: Museum.Radius.md, fill: Museum.surface.opacity(0.72))
    }

    private var availableToursSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Overline(text: "Walking tours")
                Spacer()
                Text("\(store.tours.count)")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(Museum.muted)
            }

            ForEach(store.tours) { tour in
                Button {
                    selectTour(tour)
                } label: {
                    MuseumCard(radius: Museum.Radius.md, fill: currentTour.id == tour.id ? Museum.surfaceElevated.opacity(0.72) : Museum.surfaceElevated.opacity(0.46), stroke: currentTour.id == tour.id ? Museum.blue.opacity(0.5) : Museum.border, padding: 14) {
                        HStack(spacing: 12) {
                            Image(systemName: "figure.walk.motion")
                                .font(.system(size: 16, weight: .bold))
                                .foregroundStyle(currentTour.id == tour.id ? Museum.blue : Museum.green)
                                .frame(width: 38, height: 38)
                                .background((currentTour.id == tour.id ? Museum.blue : Museum.green).opacity(0.14), in: Circle())

                            VStack(alignment: .leading, spacing: 3) {
                                Text(tour.title)
                                    .font(.system(size: 15, weight: .bold))
                                    .foregroundStyle(Museum.text)
                                Text(tourListSubtitle(for: tour))
                                    .font(Museum.bodyFont(12))
                                    .foregroundStyle(Museum.muted)
                                    .lineLimit(2)
                            }

                            Spacer(minLength: 8)

                            Text(tourListTrailingLabel(for: tour))
                                .font(.system(size: 11, weight: .bold))
                                .foregroundStyle(currentTour.id == tour.id ? Museum.blue : Museum.muted)
                        }
                    }
                }
                .buttonStyle(.plain)
            }
        }
    }

    @ViewBuilder
    private var tourPreview: some View {
        if activeTourProgress != nil {
            tourProgressCard
        } else {
            Button(action: activateDowntownLoopTour) {
                MuseumCard(radius: Museum.Radius.md, fill: Museum.surfaceElevated.opacity(0.58), stroke: Museum.border, padding: 14) {
                    HStack(spacing: 12) {
                        Image(systemName: "figure.walk.motion")
                            .font(.system(size: 16, weight: .bold))
                            .foregroundStyle(Museum.green)
                            .frame(width: 40, height: 40)
                            .background(Museum.green.opacity(0.14), in: Circle())

                        VStack(alignment: .leading, spacing: 3) {
                            Text(currentTour.title)
                                .font(.system(size: 15, weight: .bold))
                                .foregroundStyle(Museum.text)
                            Text(tourPreviewSubtitle)
                                .font(Museum.bodyFont(12))
                                .foregroundStyle(Museum.muted)
                                .lineLimit(1)
                        }

                        Spacer(minLength: 8)

                        Text(tourPreviewTrailingLabel)
                            .font(.system(size: 11, weight: .bold))
                            .foregroundStyle(tourPreviewIsReady ? Museum.blue : Museum.muted)
                    }
                }
            }
            .buttonStyle(.plain)
            .disabled(!tourPreviewIsReady)
        }
    }

    private var tourProgressCard: some View {
        return MuseumCard(radius: Museum.Radius.md, fill: Museum.surfaceElevated.opacity(0.58), stroke: Museum.border, padding: 14) {
            VStack(alignment: .leading, spacing: 12) {
                HStack(spacing: 12) {
                    Image(systemName: "figure.walk.circle.fill")
                        .font(.system(size: 16, weight: .bold))
                        .foregroundStyle(Museum.green)
                        .frame(width: 40, height: 40)
                        .background(Museum.green.opacity(0.14), in: Circle())

                    VStack(alignment: .leading, spacing: 3) {
                        Text(currentTour.title)
                            .font(.system(size: 15, weight: .bold))
                            .foregroundStyle(Museum.text)
                        Text(tourProgressSubtitle)
                            .font(Museum.bodyFont(12))
                            .foregroundStyle(Museum.muted)
                            .lineLimit(2)
                    }

                    Spacer(minLength: 8)

                    Button {
                        if isTourActive {
                            endWalkingMode()
                        } else {
                            activateDowntownLoopTour()
                        }
                    } label: {
                        Text(isTourActive ? "End" : "Resume")
                            .font(.system(size: 11, weight: .bold))
                    }
                    .buttonStyle(.museum(.secondary, size: .regular))
                }

                VStack(spacing: 8) {
                    ForEach(Array(currentTour.stops.enumerated()), id: \.element.id) { index, stop in
                        let step = index + 1
                        tourStopRow(stop: stop, step: step, isCurrent: stop.spaceID == activeTourProgress?.currentStopID, isVisited: activeTourProgress?.visitedStopIDs.contains(stop.spaceID) == true)
                    }
                }

                HStack(spacing: 10) {
                    Button(action: markCurrentTourStopVisitedAndAdvance) {
                        Text(markVisitedButtonTitle)
                            .font(.system(size: 12, weight: .bold))
                    }
                    .buttonStyle(.museum(.secondary, size: .regular))
                    .disabled(activeTourProgress?.currentStopID == nil)

                    Button(action: advanceTourStop) {
                        Text(advanceButtonTitle)
                            .font(.system(size: 12, weight: .bold))
                    }
                    .buttonStyle(.museum(.secondary, size: .regular))
                    .disabled(activeTourProgress?.currentStopID == nil)

                    Spacer(minLength: 0)
                }
            }
        }
    }

    private var currentTour: WalkingTour {
        if let activeTourProgress, let tour = store.tour(id: activeTourProgress.tourID) {
            return tour
        }

        if let selectedTourID, let tour = store.tour(id: selectedTourID) {
            return tour
        }

        return store.tours.first ?? SampleTours.downtownLoop
    }

    private var currentTourStopsLoaded: Bool {
        currentTour.stops.allSatisfy { store.space(id: $0.spaceID) != nil }
    }

    private var allCurrentTourStopsLoaded: Bool {
        currentTourStopsLoaded
    }

    private var isTourActive: Bool {
        activeTourProgress?.currentStopID != nil
    }

    private var activeTourCurrentSpace: MuseumSpace? {
        guard let currentStopID = activeTourProgress?.currentStopID else { return nil }
        return store.space(id: currentStopID)
    }

    private var activeTourCurrentStepIndex: Int? {
        guard let currentStopID = activeTourProgress?.currentStopID else { return nil }
        return currentTour.stopIDs.firstIndex(of: currentStopID)
    }

    private var activeWalkingSpace: MuseumSpace? {
        guard let activeWalkingSpaceID else { return nil }
        return store.space(id: activeWalkingSpaceID)
    }

    private var routeTargetSpace: MuseumSpace? {
        activeTourCurrentSpace ?? activeWalkingSpace ?? selectedSpace
    }

    private var tourPreviewIsReady: Bool {
        allCurrentTourStopsLoaded
    }

    private var tourPreviewSubtitle: String {
        if let currentSpace = activeTourCurrentSpace, let step = activeTourCurrentStepIndex.map({ $0 + 1 }) {
            return "Step \(step) of \(currentTour.stopIDs.count) · walking to \(currentSpace.name)"
        }
        if let progress = activeTourProgress, !progress.visitedStopIDs.isEmpty {
            return "\(progress.visitedStopIDs.count)/\(currentTour.stopIDs.count) visited · tap to resume"
        }
        if let firstStop = currentTour.stops.first {
            return "\(currentTour.estimatedMinutes) min · \(currentTour.stopIDs.count) stops · starts at \(firstStop.space.title)"
        }
        return "\(currentTour.estimatedMinutes) min · \(currentTour.stopIDs.count) stops · waiting for tour spaces"
    }

    private var tourPreviewTrailingLabel: String {
        if isTourActive { return "Walking" }
        if let progress = activeTourProgress, !progress.visitedStopIDs.isEmpty { return "Resume" }
        return tourProgressIsComplete ? "Restart" : "Start"
    }

    private var tourProgressSubtitle: String {
        guard let currentSpace = activeTourCurrentSpace else {
            return tourProgressIsComplete ? "All stops visited · tap End or restart the loop" : "Tour paused · tap the preview to continue"
        }

        let routeSummary = routeSummaryText(for: currentSpace, preferRoute: true)
        let visitedCount = activeTourProgress?.visitedStopIDs.count ?? 0
        return "Step \((activeTourCurrentStepIndex ?? 0) + 1) of \(currentTour.stopIDs.count) · \(visitedCount)/\(currentTour.stopIDs.count) visited · \(routeSummary)"
    }

    private var tourProgressIsComplete: Bool {
        activeTourProgress?.visitedStopIDs.count == currentTour.stopIDs.count && activeTourProgress?.currentStopID == nil
    }

    private var markVisitedButtonTitle: String {
        guard let currentStopID = activeTourProgress?.currentStopID,
              let currentIndex = currentTour.stopIDs.firstIndex(of: currentStopID),
              currentIndex + 1 < currentTour.stopIDs.count else {
            return "Finish tour"
        }
        return activeTourProgress?.visitedStopIDs.contains(currentStopID) == true ? "Advance to next" : "Mark visited & next"
    }

    private var advanceButtonTitle: String {
        guard let currentStopID = activeTourProgress?.currentStopID,
              let currentIndex = currentTour.stopIDs.firstIndex(of: currentStopID),
              currentIndex + 1 < currentTour.stopIDs.count else {
            return "End tour"
        }
        return "Next stop"
    }

    private var mapPanel: some View {
        ZStack(alignment: .topLeading) {
            Map(position: $cameraPosition, selection: $selectedSpace) {
                ForEach(filteredSpaces) { space in
                    Marker(space.name, systemImage: space.kind.symbol, coordinate: coordinate(for: space))
                        .tint(space.accent)
                        .tag(space)
                }

                UserAnnotation()

                if case .loaded(let walkingRoute) = walkingRouteState {
                    MapPolyline(walkingRoute.route.polyline)
                        .stroke(Museum.blue, style: StrokeStyle(lineWidth: 6, lineCap: .round, lineJoin: .round))
                        .mapOverlayLevel(level: .aboveRoads)
                }
            }
            .mapStyle(.standard(elevation: .flat))
                .frame(height: 360)
                .clipShape(RoundedRectangle(cornerRadius: Museum.Radius.xl, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: Museum.Radius.xl, style: .continuous).stroke(Museum.border, lineWidth: 1))

            VStack(alignment: .leading, spacing: 8) {
                MuseumChip(title: filterTitle, icon: "map")
                Text("Tap a marker or a space below to open details.")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundStyle(Museum.text.opacity(0.68))
            }
            .padding(14)

            if let destination = routeTargetSpace, isWalkingModeActive {
                VStack {
                    Spacer()
                    activeWalkingOverlay(for: destination)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottom)
            }
        }
    }

    private var filterBar: some View {
        MuseumFlowLayout(spacing: 10) {
            Button { activeKindFilter = nil } label: {
                MuseumChip(title: "All", isSelected: activeKindFilter == nil)
            }
            .buttonStyle(.plain)

            ForEach(SpaceKind.allCases, id: \.self) { kind in
                Button { activeKindFilter = kind } label: {
                    MuseumChip(title: kind.label, isSelected: activeKindFilter == kind, icon: kind.symbol)
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var listHeader: some View {
        HStack {
            Overline(text: "Nearby spaces")
            Spacer()
            Text("\(filteredSpaces.count)")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Museum.muted)
        }
    }

    private var emptyState: some View {
        MuseumCard(radius: Museum.Radius.lg, fill: Museum.surfaceElevated.opacity(0.52), stroke: Color.white.opacity(0.12), padding: 20) {
            VStack(alignment: .leading, spacing: 10) {
                Label("No spaces in this filter", systemImage: "magnifyingglass")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(Museum.text)
                Text("Clear the filter to bring the nearby list back.")
                    .font(Museum.bodyFont(13))
                    .foregroundStyle(Museum.muted)
                Button("Show all spaces") {
                    activeKindFilter = nil
                }
                .buttonStyle(.museum(.secondary, size: .regular))
            }
        }
    }

    private func selectedSpaceCard(_ space: MuseumSpace) -> some View {
        MuseumCard(radius: Museum.Radius.lg, fill: Museum.surfaceElevated.opacity(0.52), stroke: Color.white.opacity(0.12), padding: 14) {
            VStack(alignment: .leading, spacing: 12) {
                Button {
                    path.append(space)
                } label: {
                    HStack(spacing: 14) {
                        RoundedRectangle(cornerRadius: 16, style: .continuous)
                            .fill(Museum.accentGradient.opacity(0.85))
                            .frame(width: 58, height: 58)
                            .overlay(Image(systemName: space.kind.symbol).font(.system(size: 24, weight: .semibold)).foregroundStyle(.white))

                        VStack(alignment: .leading, spacing: 4) {
                            Overline(text: "Selected space", color: space.accent)
                            Text(space.name)
                                .font(.system(size: 16, weight: .bold))
                                .foregroundStyle(Museum.text)
                            Text("\(distanceSummaryText(for: space)) · \(space.exhibitCount) exhibits")
                                .font(Museum.bodyFont(12))
                                .foregroundStyle(Museum.muted)
                                .lineLimit(1)
                        }

                        Spacer()
                        Image(systemName: "chevron.right")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundStyle(Museum.muted)
                    }
                }
                .buttonStyle(.plain)

                routeSummary(for: space)

                if shouldShowStartWalkingButton(for: space) {
                    Button {
                        startWalking(to: space)
                    } label: {
                        Label("Start walking", systemImage: "figure.walk")
                    }
                    .buttonStyle(.museum(.secondary, size: .regular))
                }

                Button {
                    openInAppleMaps(space)
                } label: {
                    Label("Open in Apple Maps", systemImage: "map")
                }
                .buttonStyle(.museum(.secondary, size: .regular))
            }
        }
    }

    @ViewBuilder
    private func routeSummary(for space: MuseumSpace) -> some View {
        HStack(spacing: 8) {
            Image(systemName: "figure.walk")
                .foregroundStyle(Museum.green)

            switch walkingRouteState {
            case .idle:
                Text(routePromptText(for: space))
            case .loading:
                Text(routeLoadingText(for: space))
            case .loaded(let route):
                if route.destinationSpaceID == space.id {
                    Text("\(route.distanceText) · \(route.etaText)")
                } else if let liveDistance = liveDistanceMeters(to: space) {
                    Text("\(distanceText(for: liveDistance)) away")
                } else {
                    Text(routePromptText(for: space))
                }
            case .failed(let message):
                Text(message)
                    .lineLimit(2)
            }

            Spacer(minLength: 0)
        }
        .font(.system(size: 12, weight: .semibold))
        .foregroundStyle(Museum.muted)
    }

    private func activeWalkingOverlay(for destination: MuseumSpace) -> some View {
        MuseumCard(radius: Museum.Radius.lg, fill: Museum.surfaceElevated.opacity(0.84), stroke: Color.white.opacity(0.12), padding: 12) {
            VStack(alignment: .leading, spacing: 10) {
                HStack(alignment: .top, spacing: 10) {
                    VStack(alignment: .leading, spacing: 3) {
                        Overline(text: activeTourCurrentSpace == nil ? "Walking" : "Tour walk", color: Museum.blue)
                        Text(destination.name)
                            .font(.system(size: 15, weight: .bold))
                            .foregroundStyle(Museum.text)
                        Text(activeRouteSummaryText(for: destination))
                            .font(Museum.bodyFont(12))
                            .foregroundStyle(Museum.muted)
                    }

                    Spacer(minLength: 8)

                    Button(action: endWalkingMode) {
                        Text("End")
                            .font(.system(size: 11, weight: .bold))
                    }
                    .buttonStyle(.museum(.secondary, size: .regular))
                }

            }
        }
        .padding(.horizontal, 12)
        .padding(.bottom, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func tourStopRow(stop: TourStop, step: Int, isCurrent: Bool, isVisited: Bool) -> some View {
        Button {
            selectTourStop(stop)
        } label: {
            HStack(spacing: 10) {
                Text("\(step)")
                    .font(.system(size: 12, weight: .bold))
                    .foregroundStyle(isCurrent ? Museum.blue : Museum.muted)
                    .frame(width: 24, height: 24)
                    .background((isCurrent ? Museum.blue : Museum.border).opacity(0.18), in: Circle())

                VStack(alignment: .leading, spacing: 2) {
                    Text(stop.space.title)
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundStyle(Museum.text)
                    Text(isCurrent ? "Current stop" : (isVisited ? "Visited" : "Not visited yet"))
                        .font(.system(size: 11, weight: .medium))
                        .foregroundStyle(Museum.muted)
                }

                Spacer(minLength: 8)

                if isCurrent {
                    Image(systemName: "location.fill")
                        .foregroundStyle(Museum.blue)
                } else if isVisited {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundStyle(Museum.green)
                } else {
                    Image(systemName: "circle")
                        .foregroundStyle(Museum.muted)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background((isCurrent ? Museum.blue : Museum.surface).opacity(0.12), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(isCurrent ? Museum.blue.opacity(0.5) : Museum.border, lineWidth: 1))
        }
        .buttonStyle(.plain)
    }

    private func coordinate(for space: MuseumSpace) -> CLLocationCoordinate2D {
        CLLocationCoordinate2D(latitude: space.latitude, longitude: space.longitude)
    }

    private func selectTourStop(_ stop: TourStop) {
        if let space = store.space(id: stop.spaceID) {
            selectedSpace = space
        }
        if activeTourProgress != nil {
            activeTourProgress?.currentStopID = stop.spaceID
            persistence.saveWalkingTourProgress(activeTourProgress)
        }
        updateCameraPosition()
    }

    private var routeTaskKey: String {
        let selected = routeTargetSpace?.id ?? "none"
        let mode = isTourActive ? "tour" : (activeWalkingSpaceID != nil ? "walk" : "browse")
        let tourStop = activeTourProgress?.currentStopID ?? "none"
        let coordinateKey = quantizedLocationKey(locationService.currentLocation?.coordinate)
        return "\(mode)-\(tourStop)-\(selected)-\(coordinateKey)"
    }

    private func quantizedLocationKey(_ coordinate: CLLocationCoordinate2D?) -> String {
        guard let coordinate else { return "no-location" }
        let lat = Int((coordinate.latitude * 1000).rounded())
        let lng = Int((coordinate.longitude * 1000).rounded())
        return "\(lat)-\(lng)"
    }

    private func updateWalkingRoute() async {
        guard let targetSpace = routeTargetSpace else {
            walkingRouteState = .idle
            routeService.cancel()
            return
        }

        guard let userCoordinate = locationService.currentLocation?.coordinate else {
            walkingRouteState = .failed("Location is needed to show an in-app walking route.")
            routeService.cancel()
            return
        }

        walkingRouteState = .loading
        let state = await routeService.route(from: userCoordinate, to: targetSpace)
        guard !Task.isCancelled, targetSpace.id == routeTargetSpace?.id else { return }

        walkingRouteState = state
        if case .loaded(let route) = state {
            focusRoute(route.route)
        }
    }

    private func focusRoute(_ route: MKRoute) {
        cameraPosition = .rect(route.polyline.boundingMapRect)
    }

    private func routeSummaryText(for space: MuseumSpace, preferRoute: Bool) -> String {
        if preferRoute, case .loaded(let route) = walkingRouteState, route.destinationSpaceID == space.id {
            return "\(route.distanceText) · \(route.etaText)"
        }

        if let liveDistance = liveDistanceMeters(to: space) {
            return "\(distanceText(for: liveDistance)) away"
        }

        return "\(distanceText(for: space.distanceMeters)) away"
    }

    private func activeRouteSummaryText(for space: MuseumSpace) -> String {
        if case .loaded(let route) = walkingRouteState, route.destinationSpaceID == space.id {
            return "\(route.distanceText) · \(route.etaText)"
        }

        if let liveDistance = liveDistanceMeters(to: space) {
            return "\(distanceText(for: liveDistance)) away"
        }

        return "Updating route…"
    }

    private func routePromptText(for space: MuseumSpace) -> String {
        if activeTourCurrentSpace?.id == space.id {
            return "Tap Start walking to keep the route pinned"
        }
        if activeWalkingSpace?.id == space.id {
            return "Walking route is active"
        }
        if isWalkingModeActive {
            return "Active walk is pinned below"
        }
        return "Select a site to calculate walking route"
    }

    private func routeLoadingText(for space: MuseumSpace) -> String {
        if activeTourCurrentSpace?.id == space.id || activeWalkingSpace?.id == space.id {
            return "Calculating live walking route…"
        }
        if isWalkingModeActive {
            return "Updating the active walk…"
        }
        return "Calculating walking route…"
    }

    private func shouldShowStartWalkingButton(for space: MuseumSpace) -> Bool {
        guard !isWalkingModeActive else { return false }
        guard case .loaded(let route) = walkingRouteState else { return false }
        return route.destinationSpaceID == space.id
    }

    private func startWalking(to space: MuseumSpace) {
        activeWalkingSpaceID = space.id
        updateCameraPosition()
    }

    private func endWalkingMode() {
        activeWalkingSpaceID = nil
        if activeTourProgress?.currentStopID != nil {
            activeTourProgress?.currentStopID = nil
        }
        if activeTourProgress?.currentStopID == nil && activeTourProgress?.visitedStopIDs.isEmpty == true {
            activeTourProgress = nil
        }
        persistence.saveWalkingTourProgress(activeTourProgress)
        routeService.cancel()
        walkingRouteState = .idle
        updateCameraPosition()
    }

    private func openSpaceDetail(_ space: MuseumSpace) {
        selectedSpace = space
        if path.last?.id != space.id {
            path.append(space)
        }
    }

    private func selectTour(_ tour: WalkingTour) {
        selectedTourID = tour.id

        if activeTourProgress?.tourID == tour.id {
            return
        }

        restoreTourProgressIfNeeded()
        updateCameraPosition()
    }

    private func tourListSubtitle(for tour: WalkingTour) -> String {
        let stopCount = tour.stopIDs.count
        let firstStop = tour.stops.first?.space.title ?? "Loading stops"

        if let progress = persistence.loadWalkingTourProgress(tourID: tour.id), !progress.visitedStopIDs.isEmpty {
            return "\(tour.estimatedMinutes) min · \(stopCount) stops · \(progress.visitedStopIDs.count) visited · starts at \(firstStop)"
        }

        return "\(tour.estimatedMinutes) min · \(stopCount) stops · starts at \(firstStop)"
    }

    private func tourListTrailingLabel(for tour: WalkingTour) -> String {
        if activeTourProgress?.tourID == tour.id {
            return isTourActive ? "Walking" : "Resume"
        }

        if selectedTourID == tour.id {
            return "Selected"
        }

        if persistence.loadWalkingTourProgress(tourID: tour.id) != nil {
            return "Resume"
        }

        return "Start"
    }

    private func activateDowntownLoopTour() {
        guard allCurrentTourStopsLoaded else { return }

        let tour = currentTour
        selectedTourID = tour.id

        var progress = persistence.loadWalkingTourProgress(tourID: tour.id) ?? activeTourProgress ?? WalkingTourProgress(tourID: tour.id)
        let allVisited = progress.visitedStopIDs.count == tour.stopIDs.count

        if allVisited {
            progress = WalkingTourProgress(tourID: tour.id, currentStopID: tour.stopIDs.first, visitedStopIDs: [])
        } else if progress.currentStopID == nil {
            progress.currentStopID = nextTourStopID(after: nil, visitedStopIDs: progress.visitedStopIDs)
        }

        guard let currentStopID = progress.currentStopID, let currentSpace = store.space(id: currentStopID) else { return }
        activeTourProgress = progress
        activeWalkingSpaceID = nil
        selectedSpace = currentSpace
        persistence.saveWalkingTourProgress(progress)
        updateCameraPosition()
    }

    private func markCurrentTourStopVisitedAndAdvance() {
        guard var progress = activeTourProgress,
              let currentStopID = progress.currentStopID,
              let currentIndex = currentTour.stopIDs.firstIndex(of: currentStopID) else { return }

        progress.visitedStopIDs.insert(currentStopID)
        if currentIndex + 1 < currentTour.stopIDs.count {
            progress.currentStopID = currentTour.stopIDs[currentIndex + 1]
            if let nextSpace = store.space(id: progress.currentStopID ?? "") {
                selectedSpace = nextSpace
            }
        } else {
            progress.currentStopID = nil
        }

        activeTourProgress = progress
        persistence.saveWalkingTourProgress(progress)
        updateCameraPosition()
    }

    private func advanceTourStop() {
        guard var progress = activeTourProgress,
              let currentStopID = progress.currentStopID,
              let currentIndex = currentTour.stopIDs.firstIndex(of: currentStopID) else { return }

        if currentIndex + 1 < currentTour.stopIDs.count {
            progress.currentStopID = currentTour.stopIDs[currentIndex + 1]
            if let nextSpace = store.space(id: progress.currentStopID ?? "") {
                selectedSpace = nextSpace
            }
        } else {
            progress.currentStopID = nil
        }

        activeTourProgress = progress
        persistence.saveWalkingTourProgress(progress)
        updateCameraPosition()
    }

    private func nextTourStopID(after stopID: String?, visitedStopIDs: Set<String>) -> String? {
        let startIndex: Int
        if let stopID, let index = currentTour.stopIDs.firstIndex(of: stopID) {
            startIndex = index + 1
        } else {
            startIndex = 0
        }

        for index in startIndex..<currentTour.stopIDs.count {
            let candidate = currentTour.stopIDs[index]
            if !visitedStopIDs.contains(candidate) {
                return candidate
            }
        }

        return currentTour.stopIDs.first
    }

    private func restoreTourProgressIfNeeded() {
        let tour = currentTour
        guard !restoredTourProgressTourIDs.contains(tour.id) else { return }
        restoredTourProgressTourIDs.insert(tour.id)

        guard let progress = persistence.loadWalkingTourProgress(tourID: tour.id) else { return }
        activeTourProgress = progress
        selectedTourID = tour.id

        if let currentStopID = progress.currentStopID, let currentSpace = store.space(id: currentStopID) {
            selectedSpace = selectedSpace ?? currentSpace
        }
    }

    private func distanceSummaryText(for space: MuseumSpace) -> String {
        if let route = currentRouteIfMatches(space) {
            return route.distanceText
        }
        if let liveDistance = liveDistanceMeters(to: space) {
            return distanceText(for: liveDistance)
        }
        return distanceText(for: space.distanceMeters)
    }

    private func currentRouteIfMatches(_ space: MuseumSpace) -> WalkingRoute? {
        guard case .loaded(let route) = walkingRouteState, route.destinationSpaceID == space.id else { return nil }
        return route
    }

    private func liveDistanceMeters(to space: MuseumSpace) -> CLLocationDistance? {
        guard let userLocation = locationService.currentLocation else { return nil }
        return userLocation.distance(from: CLLocation(latitude: space.latitude, longitude: space.longitude))
    }

    private func destinationProximityMeters(for space: MuseumSpace) -> CLLocationDistance? {
        if let route = currentRouteIfMatches(space) {
            return route.distanceMeters
        }
        return liveDistanceMeters(to: space) ?? space.distanceMeters
    }

    private func distanceText(for meters: CLLocationDistance) -> String {
        if meters >= 1609.344 {
            return String(format: "%.1f mi", meters / 1609.344)
        }
        return String(format: "%.0f ft", meters * 3.28084)
    }

    private var isWalkingModeActive: Bool {
        activeTourProgress?.currentStopID != nil || activeWalkingSpaceID != nil
    }

    private func openInAppleMaps(_ space: MuseumSpace) {
        let item = MKMapItem(placemark: MKPlacemark(coordinate: coordinate(for: space)))
        item.name = space.name
        item.openInMaps(launchOptions: [MKLaunchOptionsDirectionsModeKey: MKLaunchOptionsDirectionsModeWalking])
    }

    private func updateCameraPosition() {
        if isWalkingModeActive, let targetSpace = routeTargetSpace {
            if case .loaded(let route) = walkingRouteState, route.destinationSpaceID == targetSpace.id {
                focusRoute(route.route)
                return
            }

            cameraPosition = .region(
                MKCoordinateRegion(
                    center: coordinate(for: targetSpace),
                    span: MKCoordinateSpan(latitudeDelta: 0.01, longitudeDelta: 0.01)
                )
            )
            return
        }

        if let selectedSpace {
            cameraPosition = .region(
                MKCoordinateRegion(
                    center: coordinate(for: selectedSpace),
                    span: MKCoordinateSpan(latitudeDelta: 0.01, longitudeDelta: 0.01)
                )
            )
            return
        }

        let coordinates = mapSpaces.map { coordinate(for: $0) }
        let userCoordinate = locationService.currentLocation?.coordinate
        let region = coordinateRegion(for: coordinates, userCoordinate: userCoordinate)
        cameraPosition = .region(region)
    }

    private func coordinateRegion(for coordinates: [CLLocationCoordinate2D], userCoordinate: CLLocationCoordinate2D?) -> MKCoordinateRegion {
        var points = coordinates
        if let userCoordinate {
            points.append(userCoordinate)
        }

        guard let first = points.first else {
            let fallbackCenter = CLLocationCoordinate2D(latitude: 41.8781, longitude: -87.6298)
            return MKCoordinateRegion(
                center: fallbackCenter,
                span: MKCoordinateSpan(latitudeDelta: 0.08, longitudeDelta: 0.08)
            )
        }

        let latitudes = points.map(\.latitude)
        let longitudes = points.map(\.longitude)
        let minLatitude = latitudes.min() ?? first.latitude
        let maxLatitude = latitudes.max() ?? first.latitude
        let minLongitude = longitudes.min() ?? first.longitude
        let maxLongitude = longitudes.max() ?? first.longitude

        let center = CLLocationCoordinate2D(
            latitude: (minLatitude + maxLatitude) / 2,
            longitude: (minLongitude + maxLongitude) / 2
        )

        let latitudeDelta = max((maxLatitude - minLatitude) * 1.5, 0.02)
        let longitudeDelta = max((maxLongitude - minLongitude) * 1.5, 0.02)

        return MKCoordinateRegion(
            center: center,
            span: MKCoordinateSpan(latitudeDelta: latitudeDelta, longitudeDelta: longitudeDelta)
        )
    }
}
