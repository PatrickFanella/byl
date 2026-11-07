import SwiftUI
import InvisibleMuseumKit

/// Pick published places, order them, name the route, choose modes and visibility, create or
/// update; sharing becomes available after publication.
struct RouteBuilderView: View {
    @Environment(AppModel.self) private var model
    @State private var builder: RouteBuilderModel?
    @State private var created: TourDetail?

    var editing: TourDetail?
    var onDone: (TourDetail) -> Void

    var body: some View {
        MuseumScreen {
            if let builder {
                if let created {
                    success(created)
                } else {
                    BuilderForm(builder: builder) { tour in created = tour }
                }
            } else {
                MuseumLoadingView()
            }
        }
        .navigationTitle(editing == nil ? "New route" : "Edit route")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            // A route being edited keeps its own city even if the picker moved on.
            let owning = editing.flatMap { e in model.cities.first { $0.id == e.tour.cityID } }
            guard builder == nil, let city = owning ?? model.selectedCity else { return }
            let b = RouteBuilderModel(api: model.api, city: city, editing: editing)
            if let here = model.location.currentLocation { b.sortNear(latitude: here.coordinate.latitude, longitude: here.coordinate.longitude) }
            builder = b
            await b.loadPlaces()
        }
    }

    private func success(_ tour: TourDetail) -> some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.lg) {
            MuseumCard {
                VStack(alignment: .leading, spacing: 10) {
                    Overline(text: editing == nil ? "Route created" : "Route updated", color: Museum.success)
                    Text(tour.tour.title)
                        .font(Museum.title(24))
                        .foregroundStyle(Museum.text)
                    Text(tour.tour.status != "published" ? "Saved as a draft that only you can open. The path needs review before publication or sharing." : tour.tour.visibility == "private" ? "Only you can open it." : "Anyone with the link can open it; it is not listed publicly.")
                        .font(Museum.bodyFont(14))
                        .foregroundStyle(Museum.muted)
                    Text("Routes are computed in the background; distances appear in a minute.")
                        .font(Museum.bodyFont(12))
                        .foregroundStyle(Museum.faint)
                }
            }
            if tour.tour.status == "published" {
                ShareLink(item: RouteBuilderModel.shareURL(slug: tour.tour.slug, base: model.client.baseURL), subject: Text(tour.tour.title)) {
                    Label("Share the link", systemImage: "square.and.arrow.up")
                }
                .buttonStyle(.museum(.primary, size: .large))
            }
            Button("Done") { onDone(tour) }
                .buttonStyle(.museum(.secondary))
        }
    }
}

private struct BuilderForm: View {
    @Bindable var builder: RouteBuilderModel
    var onCreated: (TourDetail) -> Void

    var body: some View {
        MuseumTextField(label: "Name", placeholder: "e.g. Depot to the river", text: $builder.title)
        MuseumTextEditor(label: "Summary", placeholder: "Optional: what ties these places together?", text: $builder.summary, minHeight: 90)

        MuseumCard(padding: Museum.Spacing.md) {
            VStack(alignment: .leading, spacing: 10) {
                Overline(text: "Travel")
                HStack(spacing: 8) {
                    Button { builder.walk.toggle() } label: { MuseumChip(title: "Walking", isSelected: builder.walk, icon: "figure.walk") }
                    Button { builder.bike.toggle() } label: { MuseumChip(title: "Biking", isSelected: builder.bike, icon: "bicycle") }
                }
                .buttonStyle(.plain)
                if builder.modes.isEmpty {
                    MuseumInlineHint(text: "Pick at least one way to travel.", tint: Museum.danger, isError: true)
                }
                Overline(text: "Who can open it after publication")
                HStack(spacing: 8) {
                    Button { builder.visibility = "unlisted" } label: { MuseumChip(title: "Anyone with the link", isSelected: builder.visibility == "unlisted", icon: "link") }
                    Button { builder.visibility = "private" } label: { MuseumChip(title: "Only me", isSelected: builder.visibility == "private", icon: "lock") }
                }
                .buttonStyle(.plain)
            }
        }

        stopsSection
        pickerSection

        if let error = builder.error, builder.stopsError == nil {
            MuseumInlineHint(text: error, tint: Museum.danger, isError: true)
        }
        Button {
            Task { if let tour = await builder.submit() { onCreated(tour) } }
        } label: {
            Text(builder.busy ? "Saving…" : (builder.isEditing ? "Save changes" : "Create route"))
        }
        .buttonStyle(.museum(.primary, size: .large))
        .disabled(!builder.canSubmit)
    }

    private var stopsSection: some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.sm) {
            MuseumSectionHeader(title: "Stops", eyebrow: "\(builder.stops.count) of \(RouteBuilderModel.maxStops)")
            if builder.stops.isEmpty {
                MuseumEmptyView(title: "No stops yet", message: "Add places from the list below; the first is where the route starts.", symbol: "mappin")
            }
            ForEach(Array(builder.stops.enumerated()), id: \.element.id) { index, place in
                MuseumCard(padding: Museum.Spacing.sm) {
                    HStack(spacing: 10) {
                        Text("\(index + 1)")
                            .font(Museum.meta(12))
                            .foregroundStyle(Museum.actionText)
                            .frame(width: 26, height: 26)
                            .background(Museum.action, in: Circle())
                        Text(place.name)
                            .font(Museum.bodyFont(14, weight: .semibold))
                            .foregroundStyle(Museum.text)
                            .lineLimit(1)
                        Spacer()
                        Button { builder.move(from: index, to: index - 1) } label: { Image(systemName: "chevron.up").frame(width: 30, height: 30) }
                            .disabled(index == 0)
                            .accessibilityLabel("Move \(place.name) up")
                        Button { builder.move(from: index, to: index + 1) } label: { Image(systemName: "chevron.down").frame(width: 30, height: 30) }
                            .disabled(index == builder.stops.count - 1)
                            .accessibilityLabel("Move \(place.name) down")
                        Button(role: .destructive) { builder.remove(place.id) } label: { Image(systemName: "minus.circle").frame(width: 30, height: 30) }
                            .accessibilityLabel("Remove \(place.name)")
                    }
                    .foregroundStyle(Museum.text)
                    .buttonStyle(.plain)
                }
            }
            // Review Focus 5: the API's 422 field message sits next to the stops list.
            if let error = builder.stopsError {
                MuseumInlineHint(text: error, tint: Museum.danger, isError: true)
            } else if let hint = builder.stopsHint {
                MuseumInlineHint(text: hint, tint: Museum.muted)
            }
        }
    }

    private var pickerSection: some View {
        VStack(alignment: .leading, spacing: Museum.Spacing.sm) {
            MuseumSectionHeader(title: "Places in \(builder.city.name)", eyebrow: "Published places")
            MuseumTextField(label: "Search", placeholder: "Name or street", text: $builder.search)
            switch builder.placesState {
            case .idle, .loading:
                MuseumLoadingView()
            case .failed(let message):
                MuseumErrorView(message: message) { Task { await builder.loadPlaces() } }
            case .loaded:
                let items = builder.filteredPlaces
                if items.isEmpty {
                    MuseumEmptyView(title: "No places match", symbol: "magnifyingglass")
                }
                ForEach(items.prefix(40)) { place in
                    let added = builder.contains(place)
                    MuseumCard(padding: Museum.Spacing.sm) {
                        HStack(spacing: 10) {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(place.name)
                                    .font(Museum.bodyFont(14, weight: .semibold))
                                    .foregroundStyle(Museum.text)
                                if !place.address.isEmpty {
                                    Text(place.address)
                                        .font(Museum.bodyFont(12))
                                        .foregroundStyle(Museum.muted)
                                        .lineLimit(1)
                                }
                            }
                            Spacer()
                            Button {
                                if added { builder.remove(place.id) } else { builder.add(place) }
                            } label: {
                                Image(systemName: added ? "checkmark.circle.fill" : "plus.circle")
                                    .font(.system(size: 22))
                                    .foregroundStyle(added ? Museum.success : Museum.action)
                            }
                            .buttonStyle(.plain)
                            .disabled(!added && builder.stops.count >= RouteBuilderModel.maxStops)
                            .accessibilityLabel(added ? "Remove \(place.name)" : "Add \(place.name)")
                        }
                    }
                }
            }
        }
    }
}
