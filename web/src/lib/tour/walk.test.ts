import { beforeEach, describe, expect, it } from 'vitest';
import { ActiveWalk, type WalkDeps } from './walk.svelte';
import type { TourDetail, TourRoute } from '#lib/visitor.ts';

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
