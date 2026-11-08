import type { TourRevision } from '#lib/visitor.ts';
import { SvelteDate } from 'svelte/reactivity';
import { ApiError } from '#lib/api/index.ts';
import {
	readTransitPlan,
	saveTransitPlan,
	transitPlanKey,
	validTransitPlan,
	planCanGuide,
	type TransitPlan
} from './transit';
export interface TransitDeps {
	enabled(): Promise<boolean>;
	plan(tour: string, revision: string, departure: string): Promise<TransitPlan>;
	base: string;
	now?: () => number;
}
/** One pinned revision and captured identity. Planning and adoption are separate actions. */
export class TransitTrip {
	selected = $state<TransitPlan | null>(null);
	proposed = $state<TransitPlan | null>(null);
	busy = $state(false);
	message = $state('');
	gate = $state<boolean | null>(null);
	guidanceBlocked = $state(false);
	private key: string;
	private ended = false;
	private storageBlocked = false;
	private gateRequest = 0;
	private gateChecked = 0;
	private generation = 0;
	constructor(
		readonly revision: TourRevision,
		private deps: TransitDeps,
		private storage: Storage,
		owner: string,
		private sameIdentity: () => boolean,
		gate: boolean | null
	) {
		this.gate = gate;
		this.gateChecked = deps.now?.() ?? Date.now();
		this.key = transitPlanKey(deps.base, owner, revision.tour_id);
		try {
			const saved = readTransitPlan(storage, this.key, revision);
			this.selected = saved?.plan ?? null;
			this.guidanceBlocked = saved?.guidance_blocked ?? false;
		} catch {
			this.storageBlocked = true;
			this.message =
				'The saved timetable could not be read. Remove it explicitly before selecting a replacement.';
		}
	}
	end() {
		this.ended = true;
		this.generation++;
		this.proposed = null;
	}
	isAdmitted(now = Date.now()) {
		return this.gate === true && now - this.gateChecked >= 0 && now - this.gateChecked <= 60_000;
	}
	async refreshGate() {
		const request = ++this.gateRequest;
		try {
			const gate = await this.deps.enabled();
			if (!this.ended && this.sameIdentity() && request === this.gateRequest) {
				this.gate = gate;
				this.gateChecked = this.deps.now?.() ?? Date.now();
			}
		} catch {
			if (!this.ended && request === this.gateRequest) this.gate = null;
		}
	}
	async request(departure: string) {
		if (this.storageBlocked) {
			this.message = 'Remove the unreadable saved timetable explicitly before planning again.';
			return;
		}
		if (this.busy || this.ended || !this.sameIdentity()) return;
		const generation = ++this.generation;
		this.busy = true;
		this.message = '';
		this.proposed = null;
		try {
			await this.refreshGate();
			if (!this.current(generation) || !this.gate) {
				if (this.current(generation))
					this.message = 'Scheduled transit is unavailable. The retained plan has not changed.';
				return;
			}
			const at = Date.parse(departure),
				now = this.deps.now?.() ?? Date.now();
			if (!Number.isFinite(at) || at < now || at > now + 7 * 86400000)
				throw Error(
					'Choose a future departure within seven days from the connector’s first story stop.'
				);
			const plan = await this.deps.plan(
				this.revision.tour_id,
				this.revision.id,
				new SvelteDate(at).toISOString()
			);
			if (!this.current(generation)) return;
			if (!validTransitPlan(plan, this.revision) || Date.parse(plan.requested_departure_at) !== at)
				throw Error('The timetable does not match this retained revision and selected departure.');
			if (
				this.selected &&
				(['needs_review', 'unsupported'].includes(plan.state) ||
					(plan.state !== 'scheduled' &&
						Date.parse(plan.requested_departure_at) ===
							Date.parse(this.selected.requested_departure_at)))
			) {
				this.guidanceBlocked = true;
				try {
					saveTransitPlan(this.storage, this.key, this.selected, this.revision, true);
				} catch {
					try {
						this.storage.removeItem(this.key);
					} catch {
						/* The current trip remains blocked. */
					}
					this.message =
						'The connection needs review and that status could not be saved. Platform directions are stopped; remove the saved timetable before reopening.';
				}
			}
			this.proposed = plan;
		} catch (e) {
			if (this.current(generation)) {
				if (e instanceof ApiError && (e.status === 403 || e.status === 404)) this.gate = false;
				this.message =
					e instanceof Error
						? e.message
						: 'The timetable could not be checked. The retained plan is unchanged.';
			}
		} finally {
			if (generation === this.generation) this.busy = false;
		}
	}
	accept() {
		if (
			this.busy ||
			this.ended ||
			!this.sameIdentity() ||
			this.storageBlocked ||
			!this.isAdmitted(this.deps.now?.() ?? Date.now()) ||
			!this.proposed ||
			!planCanGuide(this.proposed, this.deps.now?.() ?? Date.now())
		)
			return false;
		try {
			saveTransitPlan(this.storage, this.key, this.proposed, this.revision);
			this.guidanceBlocked = false;
			this.selected = this.proposed;
			this.proposed = null;
			this.message =
				'This timetable is retained with the trip. Reopening does not select a different train.';
			return true;
		} catch {
			this.message =
				'This timetable could not be saved on this device. The retained plan has not changed.';
			return false;
		}
	}
	remove() {
		if (this.ended || !this.sameIdentity()) return;
		try {
			this.storage.removeItem(this.key);
			if (this.storage.getItem(this.key) !== null) throw Error();
			this.generation++;
			this.busy = false;
			this.storageBlocked = false;
			this.guidanceBlocked = false;
			this.selected = null;
			this.proposed = null;
			this.message = 'The saved timetable was removed. Stories and stop progress are unchanged.';
		} catch {
			this.message =
				'The saved timetable could not be removed. Retry when device storage is available.';
		}
	}
	private current(generation: number) {
		return !this.ended && generation === this.generation && this.sameIdentity();
	}
}
