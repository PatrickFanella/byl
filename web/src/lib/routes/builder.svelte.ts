// Builds or edits a visitor route: ordered published places in one city, a name, modes, and
// visibility. Validation mirrors the API (2–30 stops, one city) so Create stays disabled until
// the request can succeed; a 422 still maps its field messages next to the stops.
import { ApiError } from '#lib/api/index.ts';
import type { City, PlaceSummary, TourDetail } from '#lib/visitor.ts';

export const MIN_STOPS = 2;
export const MAX_STOPS = 30;

export interface RouteApi {
	create(body: Record<string, unknown>): Promise<TourDetail>;
	update(id: string, body: Record<string, unknown>): Promise<TourDetail>;
}

export function shareURL(origin: string, slug: string): string {
	return `${origin.replace(/\/$/, '')}/t/${slug}`;
}

export class RouteBuilder {
	readonly city: City;
	readonly editing: TourDetail | null;

	title = $state('');
	summary = $state('');
	walk = $state(true);
	bike = $state(true);
	visibility = $state<'unlisted' | 'private'>('unlisted');
	stops = $state<PlaceSummary[]>([]);
	busy = $state(false);
	error = $state<string | null>(null);
	fieldErrors = $state<Record<string, string>>({});

	constructor(city: City, editing: TourDetail | null = null) {
		this.city = city;
		this.editing = editing;
		if (editing) {
			this.title = editing.title;
			this.summary = editing.summary ?? '';
			this.walk = editing.modes.includes('walk');
			this.bike = editing.modes.includes('bike');
			this.visibility = editing.visibility === 'private' ? 'private' : 'unlisted';
			this.stops = [...editing.stops].sort((a, b) => a.position - b.position).map((s) => s.place);
		}
	}

	get isEditing(): boolean {
		return this.editing !== null;
	}

	contains(place: PlaceSummary): boolean {
		return this.stops.some((s) => s.id === place.id);
	}

	add(place: PlaceSummary): void {
		if (this.contains(place) || this.stops.length >= MAX_STOPS) return;
		this.stops = [...this.stops, place];
		if (this.fieldErrors.stops) {
			const { stops: _omit, ...rest } = this.fieldErrors;
			void _omit;
			this.fieldErrors = rest;
		}
	}

	remove(placeID: string): void {
		this.stops = this.stops.filter((s) => s.id !== placeID);
	}

	move(from: number, to: number): void {
		if (from === to || from < 0 || to < 0 || from >= this.stops.length || to >= this.stops.length)
			return;
		const next = [...this.stops];
		const [item] = next.splice(from, 1);
		next.splice(to, 0, item);
		this.stops = next;
	}

	get modes(): string[] {
		const out: string[] = [];
		if (this.walk) out.push('walk');
		if (this.bike) out.push('bike');
		return out;
	}

	/** Stops from another city; the API would answer 422. */
	get foreignStops(): PlaceSummary[] {
		return this.stops.filter((s) => s.city_id !== this.city.id);
	}

	get stopsHint(): string | null {
		if (this.foreignStops.length) return `Every stop must be in ${this.city.name}`;
		if (this.stops.length < MIN_STOPS)
			return `Add at least ${MIN_STOPS} stops (${this.stops.length} so far)`;
		if (this.stops.length >= MAX_STOPS) return `${MAX_STOPS} stops is the maximum`;
		return null;
	}

	get stopsError(): string | null {
		return this.fieldErrors.stops ?? null;
	}

	get canSubmit(): boolean {
		return (
			!this.busy &&
			this.title.trim().length > 0 &&
			this.modes.length > 0 &&
			this.stops.length >= MIN_STOPS &&
			this.stops.length <= MAX_STOPS &&
			this.foreignStops.length === 0
		);
	}

	async submit(api: RouteApi): Promise<TourDetail | null> {
		if (!this.canSubmit) return null;
		this.busy = true;
		this.error = null;
		this.fieldErrors = {};
		const body = {
			title: this.title.trim(),
			summary: this.summary,
			modes: this.modes,
			visibility: this.visibility,
			stops: this.stops.map((s) => ({ place_id: s.id, note: '' }))
		};
		try {
			if (this.editing) return await api.update(this.editing.id, body);
			return await api.create({ city: this.city.slug, ...body });
		} catch (e) {
			if (e instanceof ApiError) {
				this.error = e.message;
				this.fieldErrors = e.fields ?? {};
			} else {
				this.error = e instanceof Error ? e.message : 'Could not save the route.';
			}
			return null;
		} finally {
			this.busy = false;
		}
	}
}
