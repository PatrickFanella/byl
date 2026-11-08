import { distanceM } from './arrival';
import type { TourRevision, ScheduledTransitPlan, ApprovedTransit } from '#lib/visitor.ts';
export const CTA_SOURCE = 'https://www.transitchicago.com/downloads/sch_data/google_transit.zip';
export type TransitPlan = ScheduledTransitPlan;
const states = [
	'scheduled',
	'no_service',
	'stale',
	'provider_unavailable',
	'unavailable',
	'unsupported',
	'needs_review'
];
const instant = (value: unknown) =>
	typeof value === 'string' &&
	/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value)
		? Date.parse(value)
		: NaN;
const point = (lat: number, lng: number) =>
	Number.isFinite(lat) && Number.isFinite(lng) && Math.abs(lat) <= 90 && Math.abs(lng) <= 180;
function platform(
	actual: ApprovedTransit['board'] | undefined,
	expected: ApprovedTransit['board']
) {
	return (
		actual &&
		['id', 'name', 'parent_id'].every(
			(k) =>
				typeof actual[k as keyof typeof actual] === 'string' &&
				String(actual[k as keyof typeof actual]).length <= 4096
		) &&
		actual.id.length > 0 &&
		actual.name.length > 0 &&
		actual.id === expected.id &&
		actual.name === expected.name &&
		actual.parent_id === expected.parent_id &&
		point(actual.latitude, actual.longitude) &&
		distanceM(
			{ lat: actual.latitude, lng: actual.longitude },
			{ lat: expected.latitude, lng: expected.longitude }
		) <= 25
	);
}
/** A v1-capable client never interprets an unknown mixed selection as a walk. */
export function validTransitSelection(revision: TourRevision): boolean {
	try {
		const tour = revision.snapshot?.tour,
			choice = tour?.transit,
			intent = choice?.intent;
		if (
			!tour ||
			!choice ||
			!intent ||
			!Number.isInteger(choice.version) ||
			choice.version < 1 ||
			intent.version !== 1 ||
			intent.provider !== 'cta_gtfs_blue_v1' ||
			intent.mode !== 'walk' ||
			intent.from_stop_id === intent.to_stop_id ||
			!platform(choice.board, choice.board) ||
			!platform(choice.alight, choice.alight) ||
			choice.board.id !== intent.board_stop_id ||
			choice.alight.id !== intent.alight_stop_id
		)
			return false;
		const from = tour.stops.find((s) => s.id === intent.from_stop_id),
			to = tour.stops.find((s) => s.id === intent.to_stop_id);
		return (
			!!from &&
			!!to &&
			to.position === from.position + 1 &&
			[
				intent.walk_to_stop_seconds,
				intent.walk_from_stop_seconds,
				intent.boarding_buffer_seconds
			].every((v) => Number.isInteger(v) && v >= 60 && v <= 3600) &&
			intent.boarding_buffer_seconds <= 1800 &&
			typeof intent.walking_assumptions === 'string' &&
			intent.walking_assumptions.length > 0
		);
	} catch {
		return false;
	}
}
/** Validate retained identity and segment kinds before caching or rendering times. */
export function validTransitPlan(plan: TransitPlan, revision: TourRevision): boolean {
	try {
		const only = (value: object, keys: string[]) =>
			Object.keys(value).every((k) => keys.includes(k));
		if (
			!only(plan, [
				'version',
				'tour_id',
				'revision_id',
				'draft_version',
				'connector_version',
				'state',
				'requested_departure_at',
				'departure_context_stop_id',
				'service',
				'segments',
				'assumptions'
			]) ||
			!only(plan.service, [
				'state',
				'scheduled',
				'live',
				'source',
				'board',
				'alight',
				'trip_id',
				'route_id',
				'headsign',
				'departure_at',
				'arrival_at',
				'issues'
			])
		)
			return false;
		for (const p of [plan.service.board, plan.service.alight])
			if (p && !only(p, ['id', 'name', 'parent_id', 'latitude', 'longitude'])) return false;
		if (
			plan.service.source &&
			!only(plan.service.source, [
				'url',
				'publisher',
				'sha256',
				'retrieved_at',
				'license_url',
				'timezone'
			])
		)
			return false;
		if (!validTransitSelection(revision)) return false;
		const tour = revision.snapshot?.tour,
			selected = tour?.transit;
		if (
			!tour ||
			!selected ||
			plan.version !== 1 ||
			plan.tour_id !== revision.tour_id ||
			plan.revision_id !== revision.id ||
			plan.draft_version !== undefined ||
			plan.connector_version !== selected.version ||
			plan.departure_context_stop_id !== selected.intent.from_stop_id ||
			!Number.isFinite(instant(plan.requested_departure_at)) ||
			!states.includes(plan.state) ||
			!Array.isArray(plan.segments) ||
			!Array.isArray(plan.assumptions) ||
			plan.assumptions.some((v) => typeof v !== 'string' || v.length > 4000) ||
			plan.assumptions.length > 16
		)
			return false;
		if (
			plan.service.state !== plan.state ||
			!Array.isArray(plan.service.issues) ||
			plan.service.issues.length > 16 ||
			plan.service.issues.some((v) => typeof v !== 'string' || v.length > 4096)
		)
			return false;
		if (plan.state !== 'scheduled')
			return (
				plan.segments.length === 0 && plan.service.live === false && plan.service.scheduled === true
			);
		const service = plan.service,
			source = service.source;
		if (
			service.state !== 'scheduled' ||
			!service.scheduled ||
			service.live !== false ||
			service.route_id !== 'Blue' ||
			typeof service.trip_id !== 'string' ||
			service.trip_id.length === 0 ||
			service.trip_id.length > 4096 ||
			(service.headsign !== undefined &&
				(typeof service.headsign !== 'string' || service.headsign.length > 4096)) ||
			!source ||
			typeof source.publisher !== 'string' ||
			source.publisher.length > 4096 ||
			typeof source.sha256 !== 'string' ||
			source.url !== CTA_SOURCE ||
			source.timezone !== 'America/Chicago' ||
			!Number.isFinite(instant(source.retrieved_at)) ||
			!/^([0-9a-f]{64})$/.test(source.sha256) ||
			!platform(service.board, selected.board) ||
			!platform(service.alight, selected.alight)
		)
			return false;
		const dep = instant(service.departure_at),
			arr = instant(service.arrival_at),
			requested = instant(plan.requested_departure_at);
		if (
			!Number.isFinite(dep) ||
			!Number.isFinite(arr) ||
			arr <= dep ||
			arr - dep > 3 * 3600000 ||
			dep <
				requested +
					(selected.intent.walk_to_stop_seconds + selected.intent.boarding_buffer_seconds) * 1000 ||
			dep >
				requested +
					(selected.intent.walk_to_stop_seconds + selected.intent.boarding_buffer_seconds) * 1000 +
					6 * 3600000
		)
			return false;
		const stops = tour.stops;
		const position = stops.find((s) => s.id === selected.intent.from_stop_id)?.position;
		if (
			position === undefined ||
			!stops.some((s) => s.id === selected.intent.to_stop_id && s.position === position + 1)
		)
			return false;
		let inserted = false;
		const expected: string[] = [];
		const connectorKeys = ['walk_to_stop', 'boarding_wait', 'transit', 'walk_from_stop'];
		for (const leg of selected.walking_legs ?? []) {
			if (!inserted && leg.from_position >= position) {
				expected.push(...connectorKeys);
				inserted = true;
			}
			expected.push(`walk:${leg.from_stop_id}:${leg.to_stop_id}`);
		}
		if (!inserted) expected.push(...connectorKeys);
		if (
			JSON.stringify(expected) !==
			JSON.stringify(
				plan.segments.map((s) =>
					s.kind === 'walk' ? `walk:${s.from_stop_id}:${s.to_stop_id}` : s.kind
				)
			)
		)
			return false;
		const kinds = plan.segments
			.filter((s) => s.kind !== 'walk')
			.map((s) => s.kind)
			.join(',');
		if (kinds !== 'walk_to_stop,boarding_wait,transit,walk_from_stop' || plan.segments.length > 34)
			return false;
		const connector = plan.segments.filter((s) => s.kind !== 'walk');
		const [to, wait, ride, from] = connector;
		if (
			to.duration_s !== selected.intent.walk_to_stop_seconds ||
			from.duration_s !== selected.intent.walk_from_stop_seconds ||
			wait.duration_s < selected.intent.boarding_buffer_seconds ||
			to.from_stop_id !== selected.intent.from_stop_id ||
			to.to_platform_id !== selected.board.id ||
			wait.from_platform_id !== selected.board.id ||
			wait.to_platform_id !== selected.board.id ||
			ride.from_platform_id !== selected.board.id ||
			ride.to_platform_id !== selected.alight.id ||
			from.from_platform_id !== selected.alight.id ||
			from.to_stop_id !== selected.intent.to_stop_id
		)
			return false;
		const bases = {
			walk: ['retained_qualified_walking_route', 'in_app_walking'],
			walk_to_stop: ['curator_walking_estimate', 'external_directions_required'],
			boarding_wait: ['schedule_and_curator_buffer', 'check_station_signs_and_alerts'],
			transit: ['cta_static_schedule', 'scheduled_rail'],
			walk_from_stop: ['curator_walking_estimate', 'external_directions_required']
		} as const;
		for (const segment of plan.segments) {
			if (
				!only(segment, [
					'kind',
					'from_stop_id',
					'to_stop_id',
					'from_platform_id',
					'to_platform_id',
					'geometry',
					'duration_s',
					'departure_at',
					'arrival_at',
					'basis',
					'navigation'
				])
			)
				return false;
			const expected = bases[segment.kind];
			if (
				!expected ||
				segment.basis !== expected[0] ||
				segment.navigation !== expected[1] ||
				!Number.isInteger(segment.duration_s) ||
				segment.duration_s < 0 ||
				segment.duration_s > 3 * 3600
			)
				return false;
			if (segment.kind === 'walk') {
				if (
					!segment.geometry ||
					!only(segment.geometry, ['type', 'coordinates']) ||
					!Array.isArray(segment.geometry.coordinates) ||
					segment.geometry.coordinates.length < 2 ||
					segment.geometry.coordinates.some(
						(p) => !Array.isArray(p) || p.length !== 2 || !point(p[1], p[0])
					)
				)
					return false;
				const leg = selected.walking_legs?.find(
					(l) => l.from_stop_id === segment.from_stop_id && l.to_stop_id === segment.to_stop_id
				);
				if (
					!leg ||
					segment.duration_s !== leg.duration_s ||
					JSON.stringify(segment.geometry?.coordinates) !== JSON.stringify(leg.geometry) ||
					segment.geometry?.type !== 'LineString'
				)
					return false;
			} else if (
				segment.geometry !== undefined ||
				!Number.isFinite(instant(segment.departure_at)) ||
				!Number.isFinite(instant(segment.arrival_at)) ||
				Math.abs(
					(instant(segment.arrival_at) - instant(segment.departure_at)) / 1000 - segment.duration_s
				) >= 1
			)
				return false;
		}
		return (
			instant(to.departure_at) === requested &&
			instant(to.arrival_at) === instant(wait.departure_at) &&
			instant(wait.arrival_at) === dep &&
			instant(ride.departure_at) === dep &&
			instant(ride.arrival_at) === arr &&
			instant(from.departure_at) === arr
		);
	} catch {
		return false;
	}
}
export function transitPlanStatus(plan: TransitPlan | null, now = Date.now()): string {
	if (!plan)
		return 'No timetable selected. Choose when you expect to leave the connector’s first story stop.';
	if (plan.state !== 'scheduled')
		return (
			(
				{
					no_service: 'No scheduled service found.',
					stale: 'The timetable source is stale.',
					provider_unavailable: 'The timetable provider is unavailable.',
					unavailable: 'This connection is unavailable.',
					unsupported: 'This connection is unsupported.',
					needs_review: 'The retained platforms or connection need review.'
				} as Record<string, string>
			)[plan.state] ?? 'This connection is unavailable.'
		);
	const retrieved = instant(plan.service.source?.retrieved_at);
	if (!Number.isFinite(retrieved) || now - retrieved > 86400000 || retrieved > now + 300000)
		return 'Stored timetable is stale. Request and review a fresh plan before relying on it.';
	if (now >= instant(plan.service.departure_at))
		return 'The scheduled departure has passed. This retained plan cannot confirm whether you boarded or whether the train ran.';
	return 'Scheduled plan retained. Check current CTA arrivals, station signs and service alerts before boarding.';
}
export function planCanGuide(plan: TransitPlan | null, now = Date.now()) {
	return (
		!!plan &&
		plan.state === 'scheduled' &&
		Number.isFinite(instant(plan.service.source?.retrieved_at)) &&
		now - instant(plan.service.source?.retrieved_at) <= 86400000 &&
		instant(plan.service.source?.retrieved_at) <= now + 300000 &&
		now < instant(plan.service.departure_at)
	);
}
export function transitPlanKey(base: string, owner: string, tour: string) {
	return `byl.transit-plan.v1.${encodeURIComponent(base)}.${encodeURIComponent(owner)}.${encodeURIComponent(tour)}`;
}
export function readTransitPlan(
	storage: Storage,
	key: string,
	revision: TourRevision
): TransitPlan | null {
	const raw = storage.getItem(key);
	if (raw === null) return null;
	if (new TextEncoder().encode(raw).byteLength > 262144)
		throw Error(
			'The saved timetable is too large to read. Remove it explicitly before planning again.'
		);
	const plan = JSON.parse(raw) as TransitPlan;
	if (
		!plan ||
		typeof plan !== 'object' ||
		Array.isArray(plan) ||
		typeof plan.revision_id !== 'string'
	)
		throw Error(
			'The saved timetable could not be validated. Remove it explicitly before planning again.'
		);
	if (plan.revision_id !== revision.id) return null;
	if (!validTransitPlan(plan, revision))
		throw Error(
			'The saved timetable could not be validated. Remove it explicitly before planning again.'
		);
	return plan;
}
export function saveTransitPlan(
	storage: Storage,
	key: string,
	plan: TransitPlan,
	revision: TourRevision
) {
	if (!validTransitPlan(plan, revision))
		throw Error('The timetable does not match this retained revision.');
	const raw = JSON.stringify(plan);
	if (new TextEncoder().encode(raw).byteLength > 262144)
		throw Error('The timetable could not be retained on this device.');
	storage.setItem(key, raw);
	if (storage.getItem(key) !== raw)
		throw Error('The timetable could not be retained on this device.');
}
