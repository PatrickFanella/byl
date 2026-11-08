import type { TourRevision } from '#lib/visitor.ts';
import { DeviationDetector, type DeviationContext } from './deviation';

/** Use only frozen, explicitly reviewed logical references; never computed GPS legs. */
export function contributionContexts(
	revision: TourRevision | null,
	mode: 'walk' | 'bike'
): DeviationContext[] {
	try {
		return reviewedContexts(revision, mode);
	} catch {
		return [];
	}
}

function reviewedContexts(
	revision: TourRevision | null,
	mode: 'walk' | 'bike'
): DeviationContext[] {
	const snapshot = revision?.snapshot;
	const tour = snapshot?.tour;
	const route = snapshot?.routes?.[mode];
	const intent = tour?.corridor?.intent;
	if (
		!revision ||
		!tour ||
		tour.transit ||
		!route?.journey?.usable ||
		!intent ||
		!tour.modes.includes(mode)
	)
		return [];
	const selected = intent.routes.find((r) => r.mode === mode);
	if (!selected || selected.path.length > 64) return [];
	const stopAreas = tour.stops.map((s) => ({
		center: {
			lat: s.place.arrival_point?.point?.latitude ?? s.place.latitude,
			lng: s.place.arrival_point?.point?.longitude ?? s.place.longitude
		},
		radiusM: s.place.arrival_radius_m
	}));
	const contexts: DeviationContext[] = [];
	for (const traversal of selected.path) {
		const segment = intent.segments.find((s) => s.id === traversal.segment_id);
		const ref = segment?.signal_references?.[mode];
		if (
			!segment ||
			!ref ||
			ref.version !== 1 ||
			ref.ambiguity_reviewed !== true ||
			!ref.source?.version ||
			!Array.isArray(ref.competing_paths) ||
			!Array.isArray(ref.stop_areas) ||
			!route.journey.segments.some(
				(s) => s.id === segment.id && !!s.reverse === !!traversal.reverse && s.access === 'allowed'
			)
		)
			continue;
		if (
			segment.geometry.type !== 'LineString' ||
			segment.geometry.coordinates.length > 512 ||
			segment.geometry.coordinates.some((p) => !Array.isArray(p) || p.length !== 2) ||
			ref.competing_paths.length > 8 ||
			ref.competing_paths.some(
				(line) =>
					line.type !== 'LineString' ||
					line.coordinates.length > 512 ||
					line.coordinates.some((p) => !Array.isArray(p) || p.length !== 2)
			) ||
			stopAreas.length + ref.stop_areas.length > 32
		)
			continue;
		const context: DeviationContext = {
			revisionID: revision.id,
			segmentID: segment.id,
			mode,
			direction: traversal.reverse ? 'reverse' : 'forward',
			eligibility: 'reviewed',
			ambiguityReviewed: true,
			path: segment.geometry.coordinates.map(([lng, lat]) => ({ lat, lng })),
			competingPaths: ref.competing_paths.map((line) =>
				line.coordinates.map(([lng, lat]) => ({ lat, lng }))
			),
			stopAreas: [
				...stopAreas,
				...ref.stop_areas.map((a) => ({
					center: { lat: a.latitude, lng: a.longitude },
					radiusM: a.radius_m
				}))
			]
		};
		if (
			!contexts.some(
				(c) => c.segmentID === context.segmentID && c.direction === context.direction
			) &&
			new DeviationDetector(context).supported
		)
			contexts.push(context);
	}
	return contexts;
}
