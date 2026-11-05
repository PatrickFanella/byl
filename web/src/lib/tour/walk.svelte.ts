// The walk in progress: tour and route (cached for offline), the next stop, arrival by GPS or
// by hand, progress, and the legs split for the map. Pages render this; it owns no DOM.
import type { TourDetail, TourRoute, TourStop } from '#lib/visitor.ts';
import { SvelteSet } from 'svelte/reactivity';
import { ArrivalMonitor, type Fix } from './arrival';
import { ProgressStore, type ProgressApi } from './progress';

export interface WalkDeps {
	fetchTour(ref: string): Promise<TourDetail>;
	fetchRoute(tourID: string, mode: string): Promise<TourRoute>;
	storage: Storage;
	progressApi: ProgressApi;
	signedIn: () => boolean;
	accountID?: () => string | null;
}

export type Leg = TourRoute['legs'][number];

export const tourCacheKey = (ref: string) => `invisible-museum.tour.${ref}`;
export const routeCacheKey = (tourID: string, mode: string) =>
	`invisible-museum.route.${tourID}.${mode}`;

function readJSON<T>(storage: Storage, key: string): T | null {
	try {
		const raw = storage.getItem(key);
		return raw ? (JSON.parse(raw) as T) : null;
	} catch {
		return null;
	}
}

function writeJSON(storage: Storage, key: string, value: unknown): void {
	try {
		storage.setItem(key, JSON.stringify(value));
	} catch {
		// storage full or unavailable
	}
}

export class ActiveWalk {
	readonly ref: string;
	readonly mode: string;

	status: 'loading' | 'ready' | 'error' = $state('loading');
	message = $state('');
	tour = $state<TourDetail | null>(null);
	route = $state<TourRoute | null>(null);
	routeMessage = $state('');
	fromCache = $state(false);
	visited = new SvelteSet<number>();
	nextStop = $state<TourStop | null>(null);
	arrivedStop = $state<TourStop | null>(null);
	distanceToNextM = $state<number | null>(null);
	lastFix = $state<Fix | null>(null);
	syncError = $state<string | null>(null);

	private deps: WalkDeps;
	private progress: ProgressStore | null = null;
	private monitor: ArrivalMonitor | null = null;

	constructor(ref: string, mode: string, deps: WalkDeps) {
		this.ref = ref;
		this.mode = mode;
		this.deps = deps;
	}

	/** Loads tour and route, caching both; falls back to the cache when the network fails. */
	async load(): Promise<void> {
		this.status = 'loading';
		const { storage } = this.deps;
		let tour: TourDetail | null;
		let cached = false;
		try {
			tour = await this.deps.fetchTour(this.ref);
			writeJSON(storage, tourCacheKey(this.ref), tour);
		} catch (e) {
			tour = readJSON<TourDetail>(storage, tourCacheKey(this.ref));
			cached = tour != null;
			if (!tour) {
				this.message = e instanceof Error ? e.message : 'Could not load the tour.';
				this.status = 'error';
				return;
			}
		}
		this.tour = tour;
		const routeKey = routeCacheKey(tour.id, this.mode);
		try {
			const r = await this.deps.fetchRoute(tour.id, this.mode);
			this.route = r;
			this.routeMessage =
				r.status === 'ready'
					? ''
					: r.status === 'failed'
						? 'Route unavailable'
						: 'Route being computed';
			if (r.status === 'ready') writeJSON(storage, routeKey, r);
		} catch (e) {
			const r = readJSON<TourRoute>(storage, routeKey);
			this.route = r;
			if (r) cached = true;
			else this.routeMessage = e instanceof Error ? e.message : 'Route unavailable';
		}
		this.fromCache = cached;
		this.progress = new ProgressStore(tour.id, this.mode, tour.stops.length, {
			storage,
			api: this.deps.progressApi,
			signedIn: this.deps.signedIn,
			accountID: this.deps.accountID
		});
		this.replaceVisited(this.progress.visited);
		this.advance();
		this.status = 'ready';
		// A fix that came in while loading is evaluated now instead of waiting for the next one.
		if (this.lastFix) this.handleFix(this.lastFix);
		void this.progress.mergeRemote().then(() => this.refreshFromProgress());
	}

	get stops(): TourStop[] {
		return [...(this.tour?.stops ?? [])].sort((a, b) => a.position - b.position);
	}

	get isComplete(): boolean {
		return this.stops.length > 0 && this.nextStop === null;
	}

	get progressText(): string {
		return `${this.visited.size} of ${this.stops.length} stops`;
	}

	isVisited(stop: TourStop): boolean {
		return this.visited.has(stop.position);
	}

	/** The leg being travelled now: from the last visited stop to the next one. */
	get activeLeg(): Leg | null {
		const next = this.nextStop;
		const legs = this.route?.legs ?? [];
		if (!next) return null;
		return (
			legs.find((l) => l.to_position === next.position && this.visited.has(l.from_position)) ?? null
		);
	}

	get pastLegs(): Leg[] {
		const legs = this.route?.legs ?? [];
		const next = this.nextStop;
		return next ? legs.filter((l) => l.to_position < next.position) : legs;
	}

	get futureLegs(): Leg[] {
		const next = this.nextStop;
		if (!next) return [];
		return (this.route?.legs ?? []).filter((l) => l.to_position > next.position);
	}

	/** A new GPS fix: updates the distance and lets the monitor decide on arrival. */
	handleFix(fix: Fix): void {
		this.lastFix = fix;
		if (!this.monitor) return;
		this.distanceToNextM = this.monitor.distance(fix);
		if (this.monitor.update(fix) && this.nextStop) this.arrive(this.nextStop);
	}

	/** "I'm here": arrival by hand when GPS cannot confirm it. */
	markArrived(): void {
		if (this.nextStop) this.arrive(this.nextStop);
	}

	/** Another tab visited stops: pick them up (storage event). */
	refreshFromStorage(): void {
		if (!this.progress) return;
		this.progress.reload();
		this.refreshFromProgress();
	}

	async syncIfNeeded(): Promise<void> {
		await this.progress?.syncIfNeeded();
		this.syncError = this.progress?.syncError ?? null;
	}

	flush(): Promise<void> {
		return this.progress?.flush() ?? Promise.resolve();
	}

	/** Ends the walk; with `reset` the next visit starts from stop 1. */
	end(reset: boolean): void {
		if (reset) {
			this.progress?.reset();
			this.visited.clear();
			this.advance();
		}
	}

	private arrive(stop: TourStop): void {
		if (this.visited.has(stop.position)) return;
		this.progress?.markVisited(stop.position);
		this.visited.add(stop.position);
		this.arrivedStop = stop;
		this.advance();
		this.syncError = this.progress?.syncError ?? null;
	}

	private refreshFromProgress(): void {
		if (!this.progress) return;
		if (this.progress.visited.size !== this.visited.size) {
			this.replaceVisited(this.progress.visited);
			this.advance();
		}
	}

	private replaceVisited(positions: Iterable<number>): void {
		this.visited.clear();
		for (const p of positions) this.visited.add(p);
	}

	private advance(): void {
		const next = this.stops.find((s) => !this.visited.has(s.position)) ?? null;
		this.nextStop = next;
		if (!next) {
			this.monitor = null;
			this.distanceToNextM = null;
			return;
		}
		const stopPoint = { lat: next.place.latitude, lng: next.place.longitude };
		if (this.monitor) this.monitor.retarget(stopPoint, next.place.arrival_radius_m);
		else this.monitor = new ArrivalMonitor(stopPoint, next.place.arrival_radius_m);
		if (this.lastFix) this.distanceToNextM = this.monitor.distance(this.lastFix);
	}
}
