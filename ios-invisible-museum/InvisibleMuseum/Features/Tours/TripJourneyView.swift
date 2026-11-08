import SwiftUI
import InvisibleMuseumKit

/// The public reviewed itinerary projection; estimates are not field measurements.
struct TripJourneyView: View {
    let tour: TourDetail
    let mode: String
    let journey: RouteJourney?

    var body: some View {
        MuseumCard {
            VStack(alignment: .leading, spacing: 12) {
                Overline(text: mode == "bike" ? "Biking itinerary" : "Walking itinerary")
                ForEach(tour.corridor?.notices(mode: mode) ?? [], id: \.self) { note in
                    MuseumInlineHint(text: note)
                }
                if let journey {
                    Text(journey.usable ? "Reviewed path ready" : "Path \(journey.state.replacingOccurrences(of: "_", with: " "))")
                        .font(Museum.bodyFont(17, weight: .semibold))
                    Text("Computation: \(journey.computation) · Quality: \(journey.quality)")
                        .font(Museum.bodyFont(13))
                    Text("Travel \(minutes(journey.travelDurationS)) · Stops \(minutes(journey.dwellDurationS)) · Story allowance \(minutes(journey.narrationDurationS))")
                    Text("Itinerary total: \(minutes(journey.itineraryDurationS))")
                    if let distance = journey.travelDistanceM {
                        Text("Travel distance: \(TourDetailModel.distanceText(meters: distance))")
                    }
                    Text(journey.returnPlan.status == "included" ? "Return included · Round trip \(minutes(journey.roundTripDurationS))" : "Return \(journey.returnPlan.status.replacingOccurrences(of: "_", with: " ")) · No round-trip total")
                    ForEach(Array(journey.segments.enumerated()), id: \.offset) { _, segment in
                        VStack(alignment: .leading, spacing: 4) {
                            Text("\(segment.kind.capitalized)\(segment.reverse ? " · Reverse traversal" : "")")
                                .font(Museum.bodyFont(14, weight: .semibold))
                            Text("Surface: \(segment.surface ?? "unknown") · Modes: \(segment.modes?.joined(separator: ", ") ?? "unknown") · Access: \(segment.access)")
                            Text("Approved reference: \(segment.source.title) · \(segment.source.publisher)")
                            if !segment.source.url.isEmpty { Text(segment.source.url).textSelection(.enabled) }
                        }
                        .font(Museum.bodyFont(12))
                    }
                    ForEach(journey.assumptions, id: \.self) { assumption in
                        Text(assumption).font(Museum.bodyFont(12)).foregroundStyle(Museum.muted)
                    }
                } else {
                    Text("Reviewed path details are unavailable. A computed distance alone is not a reviewed trip.")
                }
                ForEach(tour.stops) { stop in
                    VStack(alignment: .leading, spacing: 4) {
                        Text("\(stop.position). \(stop.place.name)").font(Museum.bodyFont(14, weight: .semibold))
                        if let arrival = stop.place.arrivalPoint {
                            Text("\(arrival.point.kind.capitalized): \(arrival.point.name)")
                            Text(arrival.point.notes)
                            ForEach(Array(arrival.point.access.filter { $0.mode == mode }.enumerated()), id: \.offset) { _, access in
                                Text("\(mode): \(access.status) · Surface: \(access.surface)")
                                if !access.notes.isEmpty { Text(access.notes) }
                                if !access.sourceTitle.isEmpty { Text("\(access.sourceTitle) · Retrieved \(access.retrievedAt)") }
                            }
                        } else { Text("Reviewed arrival access is unknown for this mode.") }
                        if !stop.note.isEmpty { Text(stop.note) }
                    }
                    .font(Museum.bodyFont(12))
                }
            }
            .foregroundStyle(Museum.text)
        }
    }

    private func minutes(_ seconds: Int?) -> String {
        guard let seconds else { return "unavailable" }
        return "\(max(0, Int(ceil(Double(seconds) / 60)))) min"
    }
}
