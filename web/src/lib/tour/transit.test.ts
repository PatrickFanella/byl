import apiPlan from './fixtures/backend-transit-plan.json' with { type: 'json' };
import apiRevision from './fixtures/backend-transit-revision.json' with { type: 'json' };
import type { TourRevision, ScheduledTransitPlan } from '#lib/visitor.ts';
import { describe, it, expect, vi } from 'vitest';
import { ActiveWalk, type WalkDeps } from './walk.svelte';
import { TransitTrip } from './transit-trip.svelte';
import {
	validTransitPlan,
	saveTransitPlan,
	readTransitPlan,
	transitPlanKey,
	transitPlanStatus,
	planCanGuide
} from './transit';
import { transitFixture, TransitMemoryStorage } from './fixtures/transit-fixture';
const now = Date.parse('2026-11-08T12:00:00Z');
function setup() {
	const { revision, plan } = transitFixture(now),
		disk = new TransitMemoryStorage();
	let identity = true;
	const api = {
		base: 'https://api.test',
		enabled: vi.fn(async () => true),
		plan: vi.fn(async () => structuredClone(plan)),
		now: () => now
	};
	const trip = new TransitTrip(revision, api, disk, 'owner', () => identity, true);
	return { revision, plan, disk, api, trip, setIdentity: (v: boolean) => (identity = v) };
}
describe('retained scheduled transit', () => {
	it('accepts the actual backend HTTP contract with a synthetic provider and isolated PostgreSQL', () => {
		expect(
			validTransitPlan(apiPlan as ScheduledTransitPlan, apiRevision as unknown as TourRevision)
		).toBe(true);
	});
	it('validates the mixed contract without making a continuous walking line', () => {
		const { revision, plan } = transitFixture(now);
		expect(validTransitPlan(plan, revision)).toBe(true);
		expect(plan.segments.filter((s) => s.geometry).map((s) => s.kind)).toEqual(['walk']);
	});
	it('matches backend position-derived stop IDs and integer projection of fractional walking seconds', () => {
		const { revision, plan } = transitFixture(now);
		Object.assign(revision.snapshot!.tour.transit!.walking_legs![0], {
			from_stop_id: undefined,
			to_stop_id: undefined,
			duration_s: 120.75
		});
		expect(validTransitPlan(plan, revision)).toBe(true);
	});

	it.each([
		'revision',
		'connector',
		'platform',
		'live',
		'geometry',
		'order',
		'walk',
		'raw',
		'nested raw',
		'source'
	])('refuses altered %s responses', (kind) => {
		const { revision, plan } = transitFixture(now);
		if (kind === 'revision') plan.revision_id = 'other';
		if (kind === 'connector') plan.connector_version++;
		if (kind === 'platform') plan.service.board!.id = 'other';
		if (kind === 'live') Object.assign(plan.service, { live: true });
		if (kind === 'geometry')
			plan.segments[2].geometry = {
				type: 'LineString',
				coordinates: [
					[0, 0],
					[1, 1]
				]
			};
		if (kind === 'order')
			[plan.segments[1], plan.segments[2]] = [plan.segments[2], plan.segments[1]];
		if (kind === 'walk') plan.segments[4].geometry!.coordinates[0][0] = 0;
		if (kind === 'raw') Object.assign(plan, { raw_fixes: [{ lat: 1, lng: 2 }] });
		if (kind === 'nested raw')
			Object.assign(plan.segments[4], { from_platform_id: { lat: 1, lng: 2 } });
		if (kind === 'source') Object.assign(plan.service.source!, { license_url: { lat: 1, lng: 2 } });
		expect(validTransitPlan(plan, revision)).toBe(false);
	});
	it('accepts unavailable service only without segments and never adopts it', async () => {
		const x = setup();
		x.api.plan.mockResolvedValue({
			...x.plan,
			state: 'no_service',
			service: { state: 'no_service', scheduled: true, live: false, issues: [] },
			segments: []
		});
		await x.trip.request(x.plan.requested_departure_at);
		expect(x.trip.proposed?.state).toBe('no_service');
		expect(x.trip.accept()).toBe(false);
		expect(x.trip.selected).toBeNull();
		expect(x.disk.length).toBe(0);
	});
	it('requests and explicit durable adoption are separate; resume never requests another train', async () => {
		const x = setup();
		await x.trip.request(x.plan.requested_departure_at);
		expect(x.trip.selected).toBeNull();
		expect(x.disk.length).toBe(0);
		expect(x.trip.accept()).toBe(true);
		expect(x.api.plan).toHaveBeenCalledWith(
			x.revision.tour_id,
			x.revision.id,
			x.plan.requested_departure_at
		);
		const reopened = new TransitTrip(x.revision, x.api, x.disk, 'owner', () => true, true);
		expect(reopened.selected).toEqual(x.plan);
		expect(x.api.plan).toHaveBeenCalledTimes(1);
		expect(
			new TransitTrip(x.revision, x.api, x.disk, 'other', () => true, true).selected
		).toBeNull();
	});
	it('storage failure prevents adoption and unknown fields never reach disk', async () => {
		const x = setup();
		await x.trip.request(x.plan.requested_departure_at);
		x.disk.setItem = () => {
			throw Error('full');
		};
		expect(x.trip.accept()).toBe(false);
		expect(x.trip.selected).toBeNull();
		const key = transitPlanKey('server', 'owner', x.revision.tour_id);
		Object.assign(x.plan, { trace: [] });
		expect(() => saveTransitPlan(x.disk, key, x.plan, x.revision)).toThrow();
	});
	it('preserves the original plan on failed replacement and refuses stale or passed proposals', async () => {
		const x = setup();
		await x.trip.request(x.plan.requested_departure_at);
		x.trip.accept();
		x.api.plan.mockRejectedValue(Error('offline'));
		await x.trip.request(x.plan.requested_departure_at);
		expect(x.trip.selected).toEqual(x.plan);
		expect(planCanGuide(x.plan, now + 25 * 3600000)).toBe(false);
		expect(planCanGuide(x.plan, Date.parse(x.plan.service.departure_at!))).toBe(false);
		expect(transitPlanStatus(x.plan, now + 25 * 3600000)).toContain('stale');
	});
	it('authoritative platform review blocks old guidance across reload without replacing the chosen train', async () => {
		const x = setup();
		await x.trip.request(x.plan.requested_departure_at);
		x.trip.accept();
		x.api.plan.mockResolvedValue({
			...x.plan,
			state: 'needs_review',
			service: {
				state: 'needs_review',
				scheduled: true,
				live: false,
				issues: ['selection_requires_review']
			},
			segments: []
		});
		await x.trip.request(x.plan.requested_departure_at);
		expect(x.trip.selected).toEqual(x.plan);
		expect(x.trip.guidanceBlocked).toBe(true);
		const restored = new TransitTrip(x.revision, x.api, x.disk, 'owner', () => true, true);
		expect(restored.selected).toEqual(x.plan);
		expect(restored.guidanceBlocked).toBe(true);
		x.api.plan.mockResolvedValue(x.plan);
		await restored.request(x.plan.requested_departure_at);
		restored.accept();
		expect(restored.guidanceBlocked).toBe(false);
	});

	it('late responses after identity loss, removal or end cannot replace a retained plan', async () => {
		for (const action of ['identity', 'remove', 'end']) {
			const x = setup();
			let done!: (v: typeof x.plan) => void;
			x.api.plan.mockImplementation(() => new Promise((resolve) => (done = resolve)));
			const request = x.trip.request(x.plan.requested_departure_at);
			await vi.waitFor(() => expect(x.api.plan).toHaveBeenCalledTimes(1));
			if (action === 'identity') x.setIdentity(false);
			if (action === 'remove') x.trip.remove();
			if (action === 'end') x.trip.end();
			done(x.plan);
			await request;
			expect(x.trip.proposed).toBeNull();
			expect(x.trip.accept()).toBe(false);
			expect(x.disk.length).toBe(0);
		}
	});
	it('a different revision cannot reuse an old plan and corrupt saved data stays explicit', () => {
		const x = setup(),
			key = transitPlanKey(x.api.base, 'owner', x.revision.tour_id);
		saveTransitPlan(x.disk, key, x.plan, x.revision);
		const next = structuredClone(x.revision);
		next.id = 'next';
		expect(readTransitPlan(x.disk, key, next)).toBeNull();
		x.disk.setItem(key, 'bad json');
		const restored = new TransitTrip(x.revision, x.api, x.disk, 'owner', () => true, true);
		expect(restored.message).toContain('could not be read');
		expect(x.disk.getItem(key)).toBe('bad json');
	});
	it('keeps story progress across mixed sections and maps only ordinary retained walking legs', async () => {
		const x = setup();
		const deps: WalkDeps = {
			fetchTour: async () => x.revision.snapshot!.tour,
			fetchRoute: vi.fn(async () => {
				throw Error('must not fetch a continuous route');
			}),
			fetchRevision: async () => x.revision,
			listRevisions: async () => [],
			revisionProgressApi: () => ({ get: async () => null, put: async () => {} }),
			storage: x.disk,
			progressApi: { get: async () => [], put: async () => {} },
			accountID: () => null,
			signedIn: () => false,
			transit: x.api
		};
		const walk = new ActiveWalk('mixed', 'walk', deps);
		await walk.load();
		expect(walk.status).toBe('ready');
		expect(walk.route).toBeNull();
		expect(walk.walkingLegs).toHaveLength(1);
		walk.markArrived();
		expect(walk.transitInterval).toBe(true);
		walk.markArrived();
		expect(walk.transitInterval).toBe(false);
		expect(walk.visited.size).toBe(2);
		expect(deps.fetchRoute).not.toHaveBeenCalled();
	});
	it('an ordinary bike alternative stays separate from the walking-only transit selection', async () => {
		const x = setup();
		x.revision.snapshot!.tour.modes = ['walk', 'bike'];
		const route = {
			id: 'bike',
			tour_id: x.revision.tour_id,
			mode: 'bike',
			status: 'ready',
			provider: 'synthetic',
			distance_m: 1000,
			duration_s: 300,
			geometry: {
				type: 'LineString',
				coordinates: [
					[0, 0],
					[1, 1]
				]
			},
			legs: [],
			error: '',
			computed_at: null
		} as const;
		x.revision.snapshot!.routes.bike = structuredClone(route) as unknown as NonNullable<
			typeof x.revision.snapshot
		>['routes']['bike'];
		const deps: WalkDeps = {
			fetchTour: async () => x.revision.snapshot!.tour,
			fetchRoute: async () => {
				throw Error();
			},
			fetchRevision: async () => x.revision,
			revisionProgressApi: () => ({ get: async () => null, put: async () => {} }),
			storage: x.disk,
			progressApi: { get: async () => [], put: async () => {} },
			signedIn: () => false,
			transit: x.api
		};
		const walk = new ActiveWalk('bike', 'bike', deps);
		await walk.load();
		expect(walk.status).toBe('ready');
		expect(walk.isMixed).toBe(false);
		expect(walk.transit).toBeNull();
		expect(walk.route?.mode).toBe('bike');
		expect(x.api.enabled).not.toHaveBeenCalled();
	});

	it('a missing or disabled capability rejects mixed trips without breaking ordinary fixtures', async () => {
		const x = setup();
		const deps: WalkDeps = {
			fetchTour: async () => x.revision.snapshot!.tour,
			fetchRoute: async () => {
				throw Error();
			},
			fetchRevision: async () => x.revision,
			revisionProgressApi: () => ({ get: async () => null, put: async () => {} }),
			storage: x.disk,
			progressApi: { get: async () => [], put: async () => {} },
			signedIn: () => false
		};
		const old = new ActiveWalk('mixed', 'walk', deps);
		await old.load();
		expect(old.status).toBe('error');
		x.api.enabled.mockResolvedValue(false);
		const disabled = new ActiveWalk('mixed', 'walk', { ...deps, transit: x.api });
		await disabled.load();
		expect(disabled.status).toBe('error');
		expect(disabled.route).toBeNull();
	});
});
