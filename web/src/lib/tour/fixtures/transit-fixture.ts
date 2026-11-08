// Synthetic software fixture only. No station-access or field-timing claim.
import { readFileSync } from 'node:fs';
import type { TourRevision, ScheduledTransitPlan, ApprovedTransit } from '#lib/visitor.ts';
const original = JSON.parse(
	readFileSync(
		new URL(
			'../../../../../backend/api/fixtures/tour-revisions-v1/revision-1.json',
			import.meta.url
		),
		'utf8'
	)
) as TourRevision;
export class TransitMemoryStorage implements Storage {
	private values = new Map<string, string>();
	get length() {
		return this.values.size;
	}
	clear() {
		this.values.clear();
	}
	key(i: number) {
		return [...this.values.keys()][i] ?? null;
	}
	getItem(k: string) {
		return this.values.get(k) ?? null;
	}
	setItem(k: string, v: string) {
		this.values.set(k, v);
	}
	removeItem(k: string) {
		this.values.delete(k);
	}
}
export function transitFixture(now = Date.now(), departure = now + 3600000) {
	const revision = structuredClone(original),
		tour = revision.snapshot!.tour;
	const third = structuredClone(tour.stops[1]);
	third.id = '40000000-0000-4000-8000-000000009999';
	third.position = 3;
	third.featured_moment = undefined;
	third.featured_moment_id = null;
	third.place.name = 'Synthetic final story';
	third.place.latitude += 0.001;
	tour.stops.push(third);
	for (const [i, stop] of tour.stops.entries())
		Object.assign(stop.place, {
			arrival_point: {
				id: `40000000-0000-4000-8000-00000000f00${i}`,
				version: 1,
				point: {
					name: 'Synthetic reviewed arrival',
					kind: 'entrance',
					latitude: stop.place.latitude,
					longitude: stop.place.longitude,
					access: [
						{
							mode: 'walk',
							status: 'allowed',
							surface: 'unknown',
							source_url: 'https://example.org/synthetic-arrival',
							source_title: 'Synthetic test fixture',
							retrieved_at: new Date(now).toISOString(),
							notes: 'Not field evidence.'
						}
					],
					notes: 'Synthetic fixture only.'
				},
				reviewed_at: new Date(now).toISOString()
			}
		});

	tour.stop_count = 3;
	tour.modes = ['walk'];
	tour.routes = {};
	Object.assign(tour, { journeys: undefined });
	const [from, to] = tour.stops;
	const source = {
		url: 'https://www.transitchicago.com/downloads/sch_data/google_transit.zip',
		publisher: 'Synthetic CTA fixture',
		sha256: 'a'.repeat(64),
		retrieved_at: new Date(now - 3600000).toISOString(),
		license_url: 'https://www.transitchicago.com/developers/terms/',
		timezone: 'America/Chicago' as const
	};
	const board = {
		id: '30374',
		name: 'Synthetic Clark/Lake platform',
		parent_id: '40380',
		latitude: 41.885737,
		longitude: -87.630886
	};
	const alight = {
		id: '30077',
		name: 'Synthetic Forest Park platform',
		parent_id: '40390',
		latitude: 41.874257,
		longitude: -87.817318
	};
	const leg = {
		from_stop_id: to.id,
		to_stop_id: third.id,
		from_position: 2,
		to_position: 3,
		geometry: [
			[to.place.longitude, to.place.latitude],
			[third.place.longitude, third.place.latitude]
		],
		duration_s: 120,
		distance_m: 100
	};
	const selection: ApprovedTransit = {
		version: 1,
		intent: {
			version: 1,
			provider: 'cta_gtfs_blue_v1',
			mode: 'walk',
			from_stop_id: from.id,
			to_stop_id: to.id,
			board_stop_id: board.id,
			alight_stop_id: alight.id,
			walk_to_stop_seconds: 300,
			walk_from_stop_seconds: 600,
			boarding_buffer_seconds: 180,
			walking_assumptions: 'Synthetic estimates for software testing only.'
		},
		board,
		alight,
		source,
		reviewed_at: new Date(now).toISOString(),
		walking_legs: [leg]
	};
	Object.assign(tour, { transit: selection });
	revision.snapshot!.routes = {};
	const iso = (ms: number) => new Date(ms).toISOString();
	const walkEnd = departure + 300000,
		dep = departure + 900000,
		arr = dep + 2700000;
	const plan: ScheduledTransitPlan = {
		version: 1,
		tour_id: tour.id,
		revision_id: revision.id,
		connector_version: 1,
		state: 'scheduled',
		requested_departure_at: iso(departure),
		departure_context_stop_id: from.id,
		service: {
			state: 'scheduled',
			scheduled: true,
			live: false,
			source,
			board,
			alight,
			trip_id: 'synthetic-train-v1',
			route_id: 'Blue',
			headsign: 'Synthetic westbound',
			departure_at: iso(dep),
			arrival_at: iso(arr),
			issues: ['scheduled_not_live']
		},
		assumptions: ['Synthetic timetable and walking estimates.'],
		segments: [
			{
				kind: 'walk_to_stop',
				from_stop_id: from.id,
				to_platform_id: board.id,
				duration_s: 300,
				departure_at: iso(departure),
				arrival_at: iso(walkEnd),
				basis: 'curator_walking_estimate',
				navigation: 'external_directions_required'
			},
			{
				kind: 'boarding_wait',
				from_platform_id: board.id,
				to_platform_id: board.id,
				duration_s: 600,
				departure_at: iso(walkEnd),
				arrival_at: iso(dep),
				basis: 'schedule_and_curator_buffer',
				navigation: 'check_station_signs_and_alerts'
			},
			{
				kind: 'transit',
				from_platform_id: board.id,
				to_platform_id: alight.id,
				duration_s: 2700,
				departure_at: iso(dep),
				arrival_at: iso(arr),
				basis: 'cta_static_schedule',
				navigation: 'scheduled_rail'
			},
			{
				kind: 'walk_from_stop',
				from_platform_id: alight.id,
				to_stop_id: to.id,
				duration_s: 600,
				departure_at: iso(arr),
				arrival_at: iso(arr + 600000),
				basis: 'curator_walking_estimate',
				navigation: 'external_directions_required'
			},
			{
				kind: 'walk',
				from_stop_id: to.id,
				to_stop_id: third.id,
				geometry: { type: 'LineString', coordinates: leg.geometry },
				duration_s: 120,
				basis: 'retained_qualified_walking_route',
				navigation: 'in_app_walking'
			}
		]
	};
	return { revision, plan: structuredClone(plan) };
}
