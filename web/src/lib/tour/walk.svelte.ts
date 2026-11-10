// The walk in progress: tour and route (cached for offline), the next stop, arrival by GPS or
// by hand, progress, and the legs split for the map. Pages render this; it owns no DOM.
import type { TourDetail, TourRoute, TourStop, TourRevision, MomentDetail } from '#lib/visitor.ts';
import { validTransitSelection } from './transit';
import { TransitTrip, type TransitDeps } from './transit-trip.svelte';
import { ApiError } from '#lib/api/index.ts';
import { SvelteSet } from 'svelte/reactivity';
import { ArrivalMonitor, type Fix } from './arrival';
import { ProgressStore, type ProgressApi } from './progress';
import { RevisionProgressStore, type RevisionProgressApi } from './revision-progress';

export interface WalkDeps {
	transit?: TransitDeps;
	fetchTour(ref: string): Promise<TourDetail>;
	fetchRoute(tourID: string, mode: string): Promise<TourRoute>;
	storage: Storage;
	progressApi: ProgressApi;
	signedIn: () => boolean;
	accountID?: () => string | null;
	fetchRevision?: (tourID: string, revisionID: string) => Promise<TourRevision>;
	listRevisions?: (tourID: string) => Promise<TourRevision[]>;
	revisionProgressApi?: (tourID: string, revisionID: string) => RevisionProgressApi;
}

export type Leg = TourRoute['legs'][number];

export const tourCacheKey = (ref: string) => `invisible-museum.tour.${ref}`;
export const routeCacheKey = (tourID: string, mode: string) =>
	`invisible-museum.route.${tourID}.${mode}`;
export const activeRevisionKey = (ref: string, account = 'anonymous') =>
	`invisible-museum.active-revision.${account}.${ref}`;
export const revisionCacheKey = (tour: string, revision: string, account = 'anonymous') =>
	`invisible-museum.revision.${account}.${tour}.${revision}`;

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
	visited = new SvelteSet<string>();
	transit = $state<TransitTrip | null>(null);
	revision = $state<TourRevision | null>(null);
	latestRevision = $state<TourRevision | null>(null);
	updateMessage = $state('');
	updateBusy = $state(false);
	nextStop = $state<TourStop | null>(null);
	arrivedStop = $state<TourStop | null>(null);
	distanceToNextM = $state<number | null>(null);
	lastFix = $state<Fix | null>(null);
	syncError = $state<string | null>(null);

	private deps: WalkDeps;
	private progress: ProgressStore | RevisionProgressStore | null = null;
	private monitor: ArrivalMonitor | null = null;
	private account = 'anonymous';

	constructor(ref: string, mode: string, deps: WalkDeps) {
		this.ref = ref;
		this.mode = mode;
		this.deps = deps;
	}

	/** Loads tour and route, caching both; falls back to the cache when the network fails. */
	async load(): Promise<void> {
		this.transit?.end();
		this.transit = null;
		this.status = 'loading';
		this.message = '';
		this.routeMessage = '';
		this.tour = null;
		this.route = null;
		this.revision = null;
		this.latestRevision = null;
		this.nextStop = null;
		this.arrivedStop = null;
		this.monitor = null;
		this.visited.clear();
		this.account = this.deps.accountID?.() ?? 'anonymous';
		const pin = readJSON<{ tourID: string; revisionID: string }>(
			this.deps.storage,
			activeRevisionKey(this.ref, this.account)
		);
		if (pin && this.deps.fetchRevision) {
			await this.loadRevision(pin.tourID, pin.revisionID);
			return;
		}
		const { storage } = this.deps;
		let tour: TourDetail | null;
		let cached = false;
		try {
			tour = await this.deps.fetchTour(this.ref);
			if (tour.revision_id && this.deps.fetchRevision) {
				await this.loadRevision(tour.id, tour.revision_id);
				return;
			}
			writeJSON(storage, tourCacheKey(this.ref), tour);
		} catch (e) {
			if (this.unavailable(e)) {
				this.message = 'This tour is no longer available online.';
				this.status = 'error';
				return;
			}
			tour = readJSON<TourDetail>(storage, tourCacheKey(this.ref));
			cached = tour != null;
			if (!tour) {
				this.message = e instanceof Error ? e.message : 'Could not load the tour.';
				this.status = 'error';
				return;
			}
		}
		if (tour.revision_id && this.deps.fetchRevision) {
			await this.loadRevision(tour.id, tour.revision_id);
			return;
		}
		if (!tour.modes.includes(this.mode as 'walk' | 'bike')) {
			this.status = 'error';
			this.message = 'This travel mode is not available for this tour.';
			return;
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

	private unavailable(e: unknown) {
		return e instanceof ApiError && (e.status === 403 || e.status === 404);
	}
	private async loadRevision(tourID: string, revisionID: string) {
		let revision: TourRevision;
		let cached = false;
		try {
			revision = await this.deps.fetchRevision!(tourID, revisionID);
		} catch (e) {
			const saved =
				!this.unavailable(e) &&
				readJSON<TourRevision>(
					this.deps.storage,
					revisionCacheKey(tourID, revisionID, this.account)
				);
			if (!saved) {
				this.message = this.unavailable(e)
					? 'This saved tour revision is no longer available. Your progress has been kept; no newer route was substituted.'
					: 'The pinned tour revision could not be loaded and has no saved copy. Reconnect to retry; no newer route was substituted.';
				this.status = 'error';
				return;
			}
			revision = saved;
			cached = true;
		}
		try {
			this.installRevision(
				revision,
				tourID,
				revisionID,
				false,
				await this.transitAdmission(revision)
			);
			this.fromCache = cached;
			void this.checkForUpdate();
		} catch (e) {
			this.message = e instanceof Error ? e.message : 'Invalid saved revision.';
			this.status = 'error';
		}
	}
	private installRevision(
		revision: TourRevision,
		tourID: string,
		revisionID: string,
		adopt = false,
		transitGate: boolean | null = null
	) {
		const snapshot = revision.snapshot;
		if (
			revision.id !== revisionID ||
			revision.tour_id !== tourID ||
			snapshot?.tour.id !== tourID ||
			!snapshot.tour.stops?.length
		)
			throw new Error('The returned revision does not match this trip. Reconnect to reload it.');
		const stopIDs = snapshot.tour.stops.map((stop) => stop.id);
		if (
			stopIDs.some((id, index) => typeof id !== 'string' || !id || stopIDs.indexOf(id) !== index) ||
			(snapshot.tour.revision_id && snapshot.tour.revision_id !== revisionID)
		)
			throw new Error(
				'The saved itinerary has invalid stop identities. Reconnect to reload this revision.'
			);
		if ((this.deps.accountID?.() ?? 'anonymous') !== this.account)
			throw new Error('Your account changed. Reopen this trip to continue.');
		if (!snapshot.tour.modes.includes(this.mode as 'walk' | 'bike'))
			throw new Error(
				'This travel mode is not available in the pinned revision. Choose a supported mode from the tour page.'
			);
		if (snapshot.tour.transit && !validTransitSelection(revision))
			throw Error(
				'This retained transit selection is unsupported or invalid. No walking path was substituted.'
			);
		if (
			snapshot.tour.transit?.intent.mode === this.mode &&
			(!this.deps.transit || this.mode !== 'walk' || transitGate === false)
		)
			throw Error(
				'Scheduled transit is unavailable in this client or server. No walking path was substituted.'
			);
		const route = snapshot.routes[this.mode];
		if (route && (route.tour_id !== tourID || route.mode !== this.mode))
			throw new Error('This saved path does not match the pinned itinerary.');
		if (route?.journey?.state === 'blocked')
			throw new Error(
				'Access for this mode is blocked in the reviewed path. Choose another supported mode.'
			);
		if (!this.deps.revisionProgressApi)
			throw new Error('Revision progress is unavailable in this client.');
		this.transit?.end();
		this.transit =
			snapshot.tour.transit?.intent.mode === this.mode && this.deps.transit
				? new TransitTrip(
						revision,
						this.deps.transit,
						this.deps.storage,
						this.account,
						() => this.sameAccount(),
						transitGate
					)
				: null;
		this.revision = revision;
		this.tour = snapshot.tour;
		this.route = snapshot.tour.transit?.intent.mode === this.mode ? null : (route ?? null);
		this.routeMessage =
			snapshot.tour.transit?.intent.mode === this.mode
				? ''
				: !route
					? 'No path retained for this mode'
					: route.journey && !route.journey.usable
						? `Path ${route.journey.state.replaceAll('_', ' ')}`
						: route.status === 'ready'
							? ''
							: 'Route unavailable';
		this.arrivedStop = null;
		this.progress = new RevisionProgressStore(
			tourID,
			revisionID,
			this.mode as 'walk' | 'bike',
			this.tour.stops.map((stop) => stop.id),
			{
				storage: this.deps.storage,
				api: this.deps.revisionProgressApi(tourID, revisionID),
				signedIn: this.deps.signedIn,
				accountID: () => this.deps.accountID?.() ?? null
			}
		);
		writeJSON(this.deps.storage, revisionCacheKey(tourID, revisionID, this.account), revision);
		for (const ref of [this.ref, tourID, this.tour.slug].filter(
			(ref, index, refs) => refs.indexOf(ref) === index
		)) {
			const key = activeRevisionKey(ref, this.account);
			const current = readJSON<{ revisionID: string }>(this.deps.storage, key);
			if (adopt || !current || current.revisionID === revisionID)
				writeJSON(this.deps.storage, key, { tourID, revisionID, mode: this.mode });
		}
		this.replaceVisited(this.progress.visited);
		this.monitor = null;
		this.advance();
		this.status = 'ready';
		if (this.lastFix) this.handleFix(this.lastFix);
		const progress = this.progress;
		void progress.mergeRemote().then(() => {
			if (this.progress === progress) this.refreshFromProgress();
		});
	}
	private async transitAdmission(revision: TourRevision): Promise<boolean | null> {
		if (revision.snapshot?.tour.transit && !validTransitSelection(revision))
			throw Error('This retained transit selection is unsupported or invalid.');
		if (revision.snapshot?.tour.transit?.intent.mode !== this.mode) return null;
		if (!this.deps.transit || this.mode !== 'walk')
			throw Error('This mixed itinerary requires scheduled-transit support.');
		try {
			return await this.deps.transit.enabled();
		} catch {
			return null;
		}
	}
	async checkForUpdate() {
		if (
			!this.revision ||
			!this.tour ||
			!this.deps.listRevisions ||
			!this.deps.fetchRevision ||
			this.updateBusy
		)
			return;
		this.updateBusy = true;
		const current = this.revision;
		try {
			const releases = await this.deps.listRevisions(this.tour.id);
			const next = releases
				.filter((r) => r.id && (r.number ?? 0) > (current.number ?? 0))
				.sort((a, b) => (b.number ?? 0) - (a.number ?? 0))[0];
			let latest: TourRevision | null = null;
			if (next) {
				try {
					latest = await this.deps.fetchRevision(this.tour.id, next.id!);
				} catch (e) {
					// Only the tour's own listing decides that it is gone; a newer release this
					// client cannot open (for example transit while transit is off) leaves the pin alone.
					if (!this.unavailable(e)) throw e;
					if (this.revision === current)
						this.updateMessage =
							'A newer revision exists but cannot be opened here. The pinned trip is unchanged.';
					return;
				}
			}
			if (
				latest &&
				(latest.id !== next?.id ||
					latest.tour_id !== this.tour.id ||
					latest.snapshot?.tour.id !== this.tour.id)
			)
				throw new Error('Invalid newer revision response.');
			if (this.revision === current) {
				this.latestRevision = latest;
				this.updateMessage = '';
			}
		} catch (e) {
			if (this.unavailable(e)) {
				this.transit?.end();
				this.transit = null;
				this.status = 'error';
				this.message = 'This tour is no longer available online. Your saved progress is retained.';
				this.tour = null;
				this.route = null;
				this.revision = null;
				this.latestRevision = null;
				this.nextStop = null;
				this.arrivedStop = null;
				this.monitor = null;
				this.visited.clear();
			} else
				this.updateMessage =
					'Could not check for a newer revision or updated guidance. The pinned trip is unchanged.';
		} finally {
			this.updateBusy = false;
		}
	}
	async adoptLatest() {
		const latest = this.latestRevision;
		if (!latest?.id || !this.tour || !this.deps.fetchRevision || this.updateBusy) return;
		this.updateBusy = true;
		try {
			await this.flush();
			const revision = await this.deps.fetchRevision(this.tour.id, latest.id);
			this.installRevision(
				revision,
				this.tour.id,
				latest.id,
				true,
				await this.transitAdmission(revision)
			);
			this.latestRevision = null;
			this.fromCache = false;
			this.updateMessage = '';
		} catch (e) {
			this.updateMessage =
				e instanceof Error
					? e.message
					: 'Could not apply this revision. Your pinned trip is unchanged.';
		} finally {
			this.updateBusy = false;
		}
	}
	get updatedGuidance() {
		const tour = this.latestRevision?.snapshot?.tour;
		return tour && !tour.owner_account_id
			? tour.stops.filter(
					(stop) => stop.note && stop.note !== this.stops.find((s) => s.id === stop.id)?.note
				)
			: [];
	}
	pinnedMoment(id: string): MomentDetail | null {
		return this.revision?.snapshot?.moments[id] ?? null;
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
		return this.visited.has(stop.id);
	}

	get isMixed(): boolean {
		return !!this.tour?.transit && this.tour.transit.intent.mode === this.mode;
	}
	get walkingLegs(): Leg[] {
		return this.isMixed ? (this.tour!.transit!.walking_legs ?? []) : (this.route?.legs ?? []);
	}
	get transitInterval(): boolean {
		const intent = this.tour?.transit?.intent;
		return (
			this.isMixed &&
			!!intent &&
			this.nextStop?.id === intent.to_stop_id &&
			this.visited.has(intent.from_stop_id)
		);
	}
	/** The leg being travelled now: from the last visited stop to the next one. */
	get activeLeg(): Leg | null {
		const next = this.nextStop;
		const legs = this.walkingLegs;
		if (!next) return null;
		return (
			legs.find(
				(l) =>
					l.to_position === next.position &&
					this.stops.some((s) => s.position === l.from_position && this.isVisited(s))
			) ?? null
		);
	}

	get pastLegs(): Leg[] {
		const legs = this.walkingLegs;
		const next = this.nextStop;
		return next
			? legs.filter((l) => l.from_position < l.to_position && l.to_position < next.position)
			: legs;
	}

	get futureLegs(): Leg[] {
		const next = this.nextStop;
		if (!next) return [];
		return this.walkingLegs.filter(
			(l) => l.to_position > next.position || l.to_position < l.from_position
		);
	}

	/** A new GPS fix: updates the distance and lets the monitor decide on arrival. */
	handleFix(fix: Fix): void {
		this.lastFix = fix;
		if (this.status === 'ready' && !this.sameAccount()) return;
		if (!this.monitor) return;
		this.distanceToNextM = this.monitor.distance(fix);
		if (this.monitor.update(fix) && this.nextStop) this.arrive(this.nextStop);
	}

	/** "I'm here": arrival by hand when GPS cannot confirm it. */
	markArrived(): void {
		if (!this.sameAccount()) return;
		if (this.nextStop) this.arrive(this.nextStop);
	}

	/** Another tab visited stops: pick them up (storage event). */
	refreshFromStorage(): void {
		if (!this.progress) return;
		this.progress.reload();
		this.refreshFromProgress();
	}

	async syncIfNeeded(): Promise<void> {
		if (!this.sameAccount()) return;
		await this.progress?.syncIfNeeded();
		this.syncError = this.progress?.syncError ?? null;
		await this.checkForUpdate();
	}

	flush(): Promise<void> {
		return this.progress?.flush() ?? Promise.resolve();
	}

	/** Ends the walk; with `reset` the next visit starts from stop 1. */
	end(reset: boolean): void {
		this.transit?.end();
		if (reset) {
			this.progress?.reset();
			this.visited.clear();
			this.advance();
			if (this.revision && this.tour)
				for (const ref of [this.ref, this.tour.id, this.tour.slug].filter(
					(ref, index, refs) => refs.indexOf(ref) === index
				)) {
					const key = activeRevisionKey(ref, this.account);
					const pin = readJSON<{ revisionID: string }>(this.deps.storage, key);
					if (pin?.revisionID === this.revision.id)
						try {
							this.deps.storage.removeItem(key);
						} catch {
							/* Storage can be unavailable. */
						}
				}
		}
	}

	private arrive(stop: TourStop): void {
		if (this.visited.has(stop.id)) return;
		if (this.progress instanceof RevisionProgressStore) this.progress.markVisited(stop.id);
		else this.progress?.markVisited(stop.position);
		this.visited.add(stop.id);
		this.arrivedStop = stop;
		this.advance();
		this.syncError = this.progress?.syncError ?? null;
	}
	private sameAccount(): boolean {
		if ((this.deps.accountID?.() ?? 'anonymous') === this.account) return true;
		this.status = 'error';
		this.message = 'Your account changed. Reopen this trip to continue.';
		this.transit?.end();
		this.transit = null;
		this.tour = null;
		this.route = null;
		this.revision = null;
		this.latestRevision = null;
		this.nextStop = null;
		this.arrivedStop = null;
		this.monitor = null;
		this.visited.clear();
		return false;
	}

	private refreshFromProgress(): void {
		if (!this.progress) return;
		this.replaceVisited(this.progress.visited);
		this.advance();
		this.syncError = this.progress.syncError;
	}

	private replaceVisited(visits: Iterable<number | string>): void {
		this.visited.clear();
		for (const id of visits) {
			const stop = this.stops.find((s) =>
				typeof id === 'number' ? s.position === id : s.id === id
			);
			if (stop) this.visited.add(stop.id);
		}
	}

	private advance(): void {
		const next = this.stops.find((s) => !this.isVisited(s)) ?? null;
		this.nextStop = next;
		if (!next) {
			this.monitor = null;
			this.distanceToNextM = null;
			return;
		}
		const stopPoint = {
			lat: next.place.arrival_point?.point?.latitude ?? next.place.latitude,
			lng: next.place.arrival_point?.point?.longitude ?? next.place.longitude
		};
		if (this.monitor) this.monitor.retarget(stopPoint, next.place.arrival_radius_m);
		else this.monitor = new ArrivalMonitor(stopPoint, next.place.arrival_radius_m);
		if (this.lastFix) this.distanceToNextM = this.monitor.distance(this.lastFix);
	}
}
