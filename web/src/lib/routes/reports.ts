import type { Schemas } from '#lib/api/index.ts';
import type { RouteReport, RouteReportInput, TourRevision } from '#lib/visitor.ts';

export const reportReasons: { value: RouteReportInput['reason']; label: string }[] = [
	{ value: 'blocked', label: 'Cannot pass' },
	{ value: 'closed', label: 'Closure' },
	{ value: 'unsafe', label: 'Unsafe crossing or path' },
	{ value: 'access', label: 'Entrance or access problem' },
	{ value: 'surface', label: 'Surface problem' },
	{ value: 'improvement', label: 'A better option' },
	{ value: 'other', label: 'Something else' }
];
export const reportStatus: Record<RouteReport['status'], string> = {
	draft: 'Received',
	processing: 'Investigation in progress',
	needs_research: 'More information or research needed',
	review: 'Curator review',
	accepted: 'Accepted',
	declined: 'Declined'
};

export interface ReportTarget {
	key: string;
	label: string;
	affected: Schemas['ReportLocation'];
}

/** Reference actual retained stop/leg identities, including a closing return. */
export function reportTargets(revision: TourRevision, mode: 'walk' | 'bike'): ReportTarget[] {
	const tour = revision.snapshot?.tour;
	if (!tour || !tour.modes.includes(mode)) return [];
	const result: ReportTarget[] = [];
	for (const stop of tour.stops) {
		result.push({
			key: `stop:${stop.id}`,
			label: `Stop ${stop.position}: ${stop.place.name}`,
			affected: { kind: 'stop', stop_id: stop.id }
		});
		result.push({
			key: `entrance:${stop.id}`,
			label: `Entrance / access at ${stop.place.name}`,
			affected: {
				kind: 'entrance',
				stop_id: stop.id,
				...(stop.place.arrival_point?.id ? { arrival_id: stop.place.arrival_point.id } : {})
			}
		});
	}
	for (const leg of revision.snapshot?.routes[mode]?.legs ?? []) {
		const from = tour.stops.find((s) =>
			leg.from_stop_id ? s.id === leg.from_stop_id : s.position === leg.from_position
		);
		const to = tour.stops.find((s) =>
			leg.to_stop_id ? s.id === leg.to_stop_id : s.position === leg.to_position
		);
		if (
			!from ||
			!to ||
			from.id === to.id ||
			from.position !== leg.from_position ||
			to.position !== leg.to_position
		)
			continue;
		result.push({
			key: `segment:${from.id}:${to.id}`,
			label: `Path from ${from.place.name} to ${to.place.name}${leg.return ? ' (return)' : ''}`,
			affected: { kind: 'segment', from_stop_id: from.id, to_stop_id: to.id }
		});
	}
	return result;
}

export function reportTargetKey(affected: Schemas['ReportLocation']): string {
	return affected.kind === 'segment'
		? `segment:${affected.from_stop_id}:${affected.to_stop_id}`
		: `${affected.kind}:${affected.stop_id}`;
}
