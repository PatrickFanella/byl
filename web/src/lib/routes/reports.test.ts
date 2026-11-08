import { describe, expect, it } from 'vitest';
import capture from '../../../../backend/api/fixtures/tour-revisions-v1/revision-1.json';
import type { TourRevision } from '#lib/visitor.ts';
import { reportTargets, reportTargetKey } from './reports';

function fixture() {
	// JSON fixtures retain API nulls which the generated optional fields omit.
	return structuredClone(capture) as unknown as TourRevision;
}
describe('report references from retained snapshots', () => {
	it('keeps stable entrance and stop identity without inventing legs', () => {
		const r = fixture();
		r.snapshot!.routes.walk!.legs = [];
		const stop = r.snapshot!.tour.stops[0];
		const targets = reportTargets(r, 'walk');
		expect(targets.find((t) => t.key === `entrance:${stop.id}`)?.affected).toMatchObject({
			kind: 'entrance',
			stop_id: stop.id
		});
		expect(targets.filter((t) => t.affected.kind === 'segment')).toEqual([]);
		r.snapshot!.tour.modes = ['walk'];
		expect(reportTargets(r, 'bike')).toEqual([]);
	});
	it('retains a closing return and rejects mismatched position/id pairs', () => {
		const r = fixture();
		const stops = r.snapshot!.tour.stops;
		const from = stops.at(-1)!;
		const to = stops[0];
		r.snapshot!.routes.walk!.legs = [
			{
				from_stop_id: from.id,
				to_stop_id: to.id,
				from_position: from.position,
				to_position: to.position,
				return: true,
				distance_m: 1,
				duration_s: 1,
				geometry: [
					[from.place.longitude, from.place.latitude],
					[to.place.longitude, to.place.latitude]
				]
			}
		];
		const returnTarget = reportTargets(r, 'walk').find((t) => t.affected.kind === 'segment')!;
		expect(returnTarget.label).toContain('(return)');
		expect(returnTarget.affected).toEqual({
			kind: 'segment',
			from_stop_id: from.id,
			to_stop_id: to.id
		});
		expect(reportTargetKey(returnTarget.affected)).toBe(returnTarget.key);
		r.snapshot!.routes.walk!.legs[0].from_position = to.position;
		expect(reportTargets(r, 'walk').filter((t) => t.affected.kind === 'segment')).toEqual([]);
	});
});
