import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ActiveWalk, activeRevisionKey, revisionCacheKey, type WalkDeps } from './walk.svelte';
import type { TourDetail, TourRoute, TourRevision } from '#lib/visitor.ts';
import { ApiError } from '#lib/api/index.ts';

class MemoryStorage implements Storage {
	private m = new Map<string, string>();
	get length() {
		return this.m.size;
	}
	clear() {
		this.m.clear();
	}
	getItem(k: string) {
		return this.m.get(k) ?? null;
	}
	key(i: number) {
		return [...this.m.keys()][i] ?? null;
	}
	removeItem(k: string) {
		this.m.delete(k);
	}
	setItem(k: string, v: string) {
		this.m.set(k, v);
	}
}

const TOUR = 't-1';
const stops = [
	[1, 'Main Street Station', 41.796, -88.0093],
	[2, 'Tivoli Theatre', 41.7943, -88.0107],
	[3, 'Fishel Park', 41.7931, -88.0121],
	[4, 'Downers Grove Museum', 41.8052, -88.0169]
] as const;

const tour = {
	id: TOUR,
	slug: 'loop',
	title: 'Loop',
	modes: ['walk', 'bike'],
	routes: {
		walk: {
			mode: 'walk',
			status: 'ready',
			distance_m: 1800,
			duration_s: 1400,
			estimated_minutes: 24
		}
	},
	stop_count: 4,
	stops: stops.map(([position, name, latitude, longitude]) => ({
		id: `s-${position}`,
		position,
		note: '',
		featured_moment_id: null,
		featured_moment: null,
		place: { id: `p-${position}`, name, latitude, longitude, arrival_radius_m: 40, moment_count: 1 }
	}))
} as unknown as TourDetail;

const route = {
	id: 'r-1',
	tour_id: TOUR,
	mode: 'walk',
	status: 'ready',
	legs: [1, 2, 3].map((from) => ({
		from_position: from,
		to_position: from + 1,
		distance_m: 100,
		duration_s: 80,
		geometry: [
			[stops[from - 1][3], stops[from - 1][2]],
			[stops[from][3], stops[from][2]]
		]
	})),
	geometry: { type: 'LineString', coordinates: [] }
} as unknown as TourRoute;

function deps(storage: Storage, fail = false): WalkDeps & { puts: number[][] } {
	const puts: number[][] = [];
	return {
		puts,
		fetchTour: async () => {
			if (fail) throw new Error('offline');
			return tour;
		},
		fetchRoute: async () => {
			if (fail) throw new Error('offline');
			return route;
		},
		storage,
		progressApi: {
			get: async () => [],
			put: async (b) => {
				puts.push(b.visited_positions);
				return {};
			}
		},
		signedIn: () => false
	};
}

const at = (i: number, accuracy = 10) => ({ lat: stops[i][2], lng: stops[i][3], accuracy });

describe('ActiveWalk', () => {
	let storage: MemoryStorage;
	beforeEach(() => {
		storage = new MemoryStorage();
	});

	it('loads the tour and route and caches them', async () => {
		const w = new ActiveWalk('loop', 'walk', deps(storage));
		await w.load();
		expect(w.status).toBe('ready');
		expect(w.fromCache).toBe(false);
		expect(w.nextStop?.position).toBe(1);
		expect(storage.getItem('invisible-museum.tour.loop')).not.toBeNull();
		expect(storage.getItem(`invisible-museum.route.${TOUR}.walk`)).not.toBeNull();
	});

	it('comes back from the cache when the network fails (Review Focus 2)', async () => {
		await new ActiveWalk('loop', 'walk', deps(storage)).load();
		const offline = new ActiveWalk('loop', 'walk', deps(storage, true));
		await offline.load();
		expect(offline.status).toBe('ready');
		expect(offline.fromCache).toBe(true);
		expect(offline.route?.legs).toHaveLength(3);
	});

	it('does not start an unpinned cached walk when the server forbids an unsupported itinerary', async () => {
		await new ActiveWalk('loop', 'walk', deps(storage)).load();
		const d = deps(storage);
		d.fetchTour = async () => {
			throw new ApiError(403, 'unsupported_itinerary', 'Client support required');
		};
		d.fetchRoute = vi.fn(async () => route);
		const blocked = new ActiveWalk('loop', 'walk', d);
		await blocked.load();
		expect(blocked.status).toBe('error');
		expect(blocked.tour).toBeNull();
		expect(blocked.route).toBeNull();
		expect(d.fetchRoute).not.toHaveBeenCalled();
		expect(storage.getItem('invisible-museum.tour.loop')).not.toBeNull();
	});

	it('reports an error when there is neither network nor cache', async () => {
		const w = new ActiveWalk('loop', 'walk', deps(storage, true));
		await w.load();
		expect(w.status).toBe('error');
		expect(w.message).toBe('offline');
	});

	it('advances on arrival and splits the legs', async () => {
		const w = new ActiveWalk('loop', 'walk', deps(storage));
		await w.load();
		expect(w.activeLeg).toBeNull();
		expect(w.futureLegs.map((l) => l.to_position)).toEqual([2, 3, 4]);
		w.handleFix(at(0));
		expect(w.nextStop?.position).toBe(2);
		expect(w.arrivedStop?.position).toBe(1);
		expect(w.activeLeg?.to_position).toBe(2);
		expect(w.futureLegs.map((l) => l.to_position)).toEqual([3, 4]);
		expect(w.pastLegs).toEqual([]);
		expect(w.progressText).toBe('1 of 4 stops');
		w.handleFix(at(1));
		expect(w.pastLegs.map((l) => l.to_position)).toEqual([2]);
		expect(w.activeLeg?.to_position).toBe(3);
	});

	it('keeps closing travel ahead while story stops remain', async () => {
		const d = deps(storage);
		const closing = { ...route.legs[0], from_position: 4, to_position: 1, return: true };
		d.fetchRoute = async () => ({ ...route, legs: [...route.legs, closing] });
		const w = new ActiveWalk('loop', 'walk', d);
		await w.load();
		expect(w.futureLegs).toContainEqual(closing);
		for (const i of [0, 1, 2]) w.handleFix(at(i));
		expect(w.nextStop?.position).toBe(4);
		expect(w.pastLegs.map((l) => l.to_position)).toEqual([2, 3]);
		expect(w.activeLeg?.to_position).toBe(4);
		expect(w.futureLegs).toEqual([closing]);
	});

	it('ignores a poor fix and tracks distance', async () => {
		const w = new ActiveWalk('loop', 'walk', deps(storage));
		await w.load();
		w.handleFix(at(0, 200));
		expect(w.nextStop?.position).toBe(1);
		expect(w.distanceToNextM).toBeLessThan(5);
	});

	it('marks arrival by hand and completes', async () => {
		const w = new ActiveWalk('loop', 'walk', deps(storage));
		await w.load();
		for (let i = 0; i < 4; i++) w.markArrived();
		expect(w.isComplete).toBe(true);
		expect(w.nextStop).toBeNull();
		expect(w.pastLegs).toHaveLength(3);
		const again = new ActiveWalk('loop', 'walk', deps(storage));
		await again.load();
		expect(again.isComplete).toBe(true);
		again.end(true);
		const fresh = new ActiveWalk('loop', 'walk', deps(storage));
		await fresh.load();
		expect(fresh.nextStop?.position).toBe(1);
	});

	it('evaluates a fix that arrived before the tour finished loading', async () => {
		const w = new ActiveWalk('loop', 'walk', deps(storage));
		w.handleFix(at(0));
		await w.load();
		expect(w.arrivedStop?.position).toBe(1);
		expect(w.nextStop?.position).toBe(2);
	});
});

describe('revision-pinned trips', () => {
	it('clears the displayed trip instead of writing visits after an account switch', async () => {
		const f = revisions();
		let account = 'account-a';
		f.d.accountID = () => account;
		const w = new ActiveWalk('loop', 'walk', f.d);
		await w.load();
		account = 'account-b';
		w.markArrived();
		expect(w.status).toBe('error');
		expect(w.message).toContain('account changed');
		expect(w.tour).toBeNull();
		expect(w.visited.size).toBe(0);
	});
	function revisions() {
		const storage = new MemoryStorage();
		let latest = 1;
		const snapshot = (n: number): TourRevision => {
			const t = structuredClone(tour);
			if (n > 1)
				t.stops = [t.stops[1], t.stops[0], ...t.stops.slice(2)].map((s, i) => ({
					...s,
					position: i + 1
				}));
			t.revision_id = `rev-${n}`;
			t.revision_number = n;
			if (n > 1) t.stops[0].note = 'Urgent reviewed access update: use the east entrance.';
			return {
				id: `rev-${n}`,
				tour_id: TOUR,
				number: n,
				draft_version: n,
				published_at: '2026-11-07T00:00:00Z',
				snapshot: {
					tour: t,
					routes: { walk: { ...structuredClone(route), id: `route-${n}` } },
					moments: { 'm-1': { id: 'm-1', body: `Reviewed narrative ${n}` } },
					routing_intent: {}
				}
			} as unknown as TourRevision;
		};
		const d: WalkDeps = {
			...deps(storage),
			fetchTour: async () => snapshot(latest).snapshot!.tour,
			fetchRoute: vi.fn(async () => {
				throw new Error('must not fetch mutable geometry');
			}),
			fetchRevision: async (_t, id) => snapshot(Number(id.split('-')[1])),
			listRevisions: async () => [snapshot(latest)],
			revisionProgressApi: () => ({ get: async () => null, put: async () => ({}) })
		};
		return { storage, d, snapshot, publish: () => latest++ };
	}
	it('arrival uses the retained public entrance and online revocation clears the visible trip', async () => {
		const f = revisions();
		f.d.fetchRevision = async () => {
			const r = f.snapshot(1);
			r.snapshot!.tour.stops[0].place.arrival_point = {
				point: { latitude: 41.9, longitude: -88.1 }
			} as TourDetail['stops'][number]['place']['arrival_point'];
			return r;
		};
		const w = new ActiveWalk('loop', 'walk', f.d);
		await w.load();
		await vi.waitFor(() => expect(w.updateBusy).toBe(false));
		w.handleFix(at(0));
		expect(w.visited.size).toBe(0);
		w.handleFix({ lat: 41.9, lng: -88.1, accuracy: 5 });
		expect([...w.visited]).toEqual(['s-1']);
		f.d.listRevisions = async () => {
			throw new ApiError(404, 'not_found', 'revoked');
		};
		await w.checkForUpdate();
		expect(w.status).toBe('error');
		expect(w.tour).toBeNull();
		expect(w.route).toBeNull();
		expect(w.arrivedStop).toBeNull();
	});
	it('loads a coherent snapshot and retains it through publication and explicit reordered adoption', async () => {
		const f = revisions();
		const w = new ActiveWalk('loop', 'walk', f.d);
		await w.load();
		await vi.waitFor(() => expect(w.updateBusy).toBe(false));
		w.markArrived();
		f.publish();
		await w.checkForUpdate();
		expect(w.revision?.id).toBe('rev-1');
		expect(w.route?.id).toBe('route-1');
		expect([...w.visited]).toEqual(['s-1']);
		expect(w.pinnedMoment('m-1')?.body).toBe('Reviewed narrative 1');
		expect(w.updatedGuidance[0].note).toContain('Urgent reviewed access update');
		await w.adoptLatest();
		expect(w.revision?.id).toBe('rev-2');
		expect(w.route?.id).toBe('route-2');
		expect(w.nextStop?.id).toBe('s-2');
		expect(w.visited.size).toBe(0);
		w.markArrived();
		const resumed = new ActiveWalk('loop', 'walk', f.d);
		await resumed.load();
		expect([...resumed.visited]).toEqual(['s-2']);
		expect(resumed.nextStop?.id).toBe('s-1');
		expect(f.d.fetchRoute).not.toHaveBeenCalled();
	});
	it('resumes the same cached revision offline and refuses an online revocation', async () => {
		const f = revisions();
		const first = new ActiveWalk('loop', 'walk', f.d);
		await first.load();
		first.markArrived();
		f.publish();
		f.d.fetchRevision = async () => {
			throw new TypeError('offline');
		};
		const cached = new ActiveWalk('loop', 'walk', f.d);
		await cached.load();
		expect(cached.status).toBe('ready');
		expect(cached.fromCache).toBe(true);
		expect(cached.revision?.id).toBe('rev-1');
		expect([...cached.visited]).toEqual(['s-1']);
		f.d.fetchRevision = async () => {
			throw new ApiError(404, 'not_found', 'revoked');
		};
		const revoked = new ActiveWalk('loop', 'walk', f.d);
		await revoked.load();
		expect(revoked.status).toBe('error');
		expect(revoked.message).toContain('no longer available');
		expect(revoked.revision).toBeNull();
		expect(JSON.parse(f.storage.getItem(activeRevisionKey('loop'))!).revisionID).toBe('rev-1');
	});
	it('does not substitute live geometry when the pinned revision has no saved copy', async () => {
		const f = revisions();
		f.storage.setItem(
			activeRevisionKey('loop'),
			JSON.stringify({ tourID: TOUR, revisionID: 'rev-1' })
		);
		f.d.fetchRevision = async () => {
			throw new TypeError('offline');
		};
		const w = new ActiveWalk('loop', 'walk', f.d);
		await w.load();
		expect(w.status).toBe('error');
		expect(w.message).toContain('no saved copy');
		expect(f.d.fetchRoute).not.toHaveBeenCalled();
	});
	it('refuses unsupported modes and cannot pair a returned path with another tour', async () => {
		const f = revisions();
		f.d.fetchRevision = async () => {
			const r = f.snapshot(1);
			r.snapshot!.tour.modes = ['walk'];
			return r;
		};
		const bike = new ActiveWalk('loop', 'bike', f.d);
		await bike.load();
		expect(bike.status).toBe('error');
		expect(bike.message).toContain('not available');
		f.d.fetchRevision = async () => {
			const r = f.snapshot(1);
			r.snapshot!.routes.walk.tour_id = 'another';
			return r;
		};
		const bad = new ActiveWalk('loop', 'walk', f.d);
		await bad.load();
		expect(bad.status).toBe('error');
		expect(bad.message).toContain('does not match');
		expect(f.storage.getItem(revisionCacheKey(TOUR, 'rev-1'))).toBeNull();
	});
});
