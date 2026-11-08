import { describe, expect, it } from 'vitest';
import fixtures from './fixtures/deviation-v1.json';
import {
	DeviationDetector,
	DEVIATION_RULES,
	type DeviationContext,
	type DeviationInput,
	type DeviationSummary
} from './deviation';

const epoch = Date.parse(fixtures.epoch);
function point(x: number, y: number) {
	return {
		lat: fixtures.origin.lat + ((y / 6_371_000) * 180) / Math.PI,
		lng:
			fixtures.origin.lng +
			((x / (6_371_000 * Math.cos((fixtures.origin.lat * Math.PI) / 180))) * 180) / Math.PI
	};
}
const context = (): DeviationContext => ({
	revisionID: 'reviewed-revision',
	segmentID: 'reviewed-segment',
	mode: 'walk',
	direction: 'forward',
	eligibility: 'reviewed',
	ambiguityReviewed: true,
	path: [point(0, 0), point(400, 0)],
	competingPaths: [],
	stopAreas: []
});
function fix(t: number, x: number, y = 0, accuracyM = 3): DeviationInput {
	return { type: 'fix', fix: { ...point(x, y), timestampMs: epoch + t * 1000, accuracyM } };
}

describe('shared deviation reference fixtures', () => {
	for (const example of fixtures.cases)
		it(example.name, () => {
			if (example.mode !== 'walk' && example.mode !== 'bike')
				throw new Error('Invalid fixture mode');
			if (example.direction !== 'forward' && example.direction !== 'reverse')
				throw new Error('Invalid fixture direction');
			const detector = new DeviationDetector({
				...context(),
				mode: example.mode,
				direction: example.direction,
				eligibility: example.eligible ? 'reviewed' : 'unsupported',
				ambiguityReviewed: example.ambiguity_reviewed,
				path: example.path_m.map(([x, y]) => point(x, y)),
				competingPaths: example.competing_paths_m.map((line) => line.map(([x, y]) => point(x, y))),
				stopAreas: example.stop_areas_m.map(([x, y, radiusM]) => ({ center: point(x, y), radiusM }))
			});
			const reports: DeviationSummary[] = [];
			for (const row of example.samples) {
				if (Array.isArray(row)) {
					const [t, x, y, a] = row;
					if (row.length !== 4 || ![t, x, y, a].every(Number.isFinite))
						throw new Error('Invalid fixture fix');
					reports.push(...detector.push(fix(t, x, y, a)));
				} else {
					switch (row.action) {
						case 'pause':
							detector.push({ type: 'pause' });
							break;
						case 'resume':
							detector.push({ type: 'resume' });
							break;
						case 'end':
							detector.push({ type: 'end' });
							break;
						case 'permission_lost':
							detector.push({ type: 'permission_lost' });
							break;
						case 'manual_arrival':
							detector.push({ type: 'manual_arrival' });
							break;
						default:
							throw new Error('Unknown fixture action');
					}
				}
			}
			expect(reports).toEqual(
				example.expected_deviation === null
					? []
					: [
							{
								schema_version: 1,
								detector_version: 'uncalibrated-v1',
								calibrated: false,
								revision_id: 'reviewed-revision',
								segment_id: 'reviewed-segment',
								mode: example.mode,
								direction: example.direction,
								observation_period: '2026-11-07',
								qualified_passages: 1,
								sustained_departure_rejoins: example.expected_deviation ? 1 : 0
							}
						]
			);
		});
});

describe('detector boundaries', () => {
	it('does not join observations across UTC days or expired evidence', () => {
		const midnight = new DeviationDetector(context());
		const reports: DeviationSummary[] = [];
		for (let i = 0; i <= 20; i++)
			reports.push(
				...midnight.push({
					type: 'fix',
					fix: {
						...point(i * 20, 0),
						accuracyM: 3,
						timestampMs: Date.parse('2026-11-07T23:59:00Z') + i * 10_000
					}
				})
			);
		expect(reports).toEqual([]);
		const expired = new DeviationDetector(context());
		for (let i = 0; i <= 95; i++) expired.push(fix(i * 10, Math.min(i * 20, 80)));
		for (let i = 5; i <= 20; i++) reports.push(...expired.push(fix(960 + (i - 5) * 10, i * 20)));
		expect(reports).toEqual([]);
	});
	it('malformed and replayed timestamps cannot complete a passage on their own', () => {
		const d = new DeviationDetector(context());
		for (const timestampMs of [NaN, Infinity, -1, epoch + 0.5, 253402300800000])
			expect(d.push({ type: 'fix', fix: { ...point(0, 0), accuracyM: 3, timestampMs } })).toEqual(
				[]
			);
		d.push(fix(10, 0));
		expect(d.push(fix(10, 400))).toEqual([]);
		expect(d.push(fix(0, 400))).toEqual([]);
	});
	it('snapshots context and never changes navigation or emits coordinates, precise time, or identifiers for people', () => {
		const c = context(),
			detector = new DeviationDetector(c);
		c.path[0].lat = 0;
		c.mode = 'bike';
		const reports = [];
		for (let i = 0; i <= 20; i++) reports.push(...detector.push(fix(i * 10, i * 20)));
		expect(reports).toHaveLength(1);
		expect(reports[0].mode).toBe('walk');
		expect(Object.keys(reports[0]).sort()).toEqual(
			[
				'schema_version',
				'detector_version',
				'calibrated',
				'revision_id',
				'segment_id',
				'mode',
				'direction',
				'observation_period',
				'qualified_passages',
				'sustained_departure_rejoins'
			].sort()
		);
		expect(DEVIATION_RULES.calibrated).toBe(false);
	});
	it('explicit resume starts fresh evidence, whereas end is terminal', () => {
		const d = new DeviationDetector(context());
		d.push(fix(0, 0));
		d.push({ type: 'pause' });
		d.push({ type: 'resume' });
		const reports = [];
		for (let i = 0; i <= 20; i++) reports.push(...d.push(fix(20 + i * 10, i * 20)));
		expect(reports).toHaveLength(1);
		d.push({ type: 'end' });
		d.push({ type: 'resume' });
		expect(d.push(fix(500, 0))).toEqual([]);
	});
	it('invalid geometry cannot become a supported passage', () => {
		for (const path of [
			[],
			[point(0, 0)],
			[point(0, 0), point(1, 0)],
			[point(0, 0), { lat: NaN, lng: 0 }]
		]) {
			const d = new DeviationDetector({ ...context(), path });
			expect(d.supported).toBe(false);
			expect(d.push(fix(0, 0))).toEqual([]);
		}
	});
	it('invalid location quality or a nonlocal jump invalidates in-flight evidence', () => {
		for (const sample of [
			{ ...point(100, 0), accuracyM: 65, timestampMs: epoch + 50_000 },
			{ lat: Infinity, lng: 0, accuracyM: 3, timestampMs: epoch + 50_000 }
		]) {
			const d = new DeviationDetector(context());
			for (let i = 0; i < 5; i++) d.push(fix(i * 10, i * 20));
			d.push({ type: 'fix', fix: sample });
			const reports = [];
			for (let i = 6; i <= 20; i++) reports.push(...d.push(fix(i * 10, i * 20)));
			expect(reports).toEqual([]);
		}
	});
});
