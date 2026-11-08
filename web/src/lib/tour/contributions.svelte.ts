import { ApiError } from '#lib/api/client.ts';
import type { TourRevision } from '#lib/visitor.ts';
import { DeviationDetector, type DeviationFix, type DeviationInput } from './deviation';
import { contributionContexts } from './contribution-context';
import {
	ContributionStore,
	ContributionStorageError,
	contributionDay,
	minimalSummary,
	type ContributionRecord
} from './contribution-store';
import type { ContributionAPI } from './contribution-api';

export interface ContributionDeps {
	store: ContributionStore;
	api: ContributionAPI;
	owner: string | null;
	sameIdentity: () => boolean;
	now?: () => number;
	key?: () => string;
}

/** Owns only active foreground observation. Navigation never depends on this class. */
export class TripContributions {
	message = $state('Route improvement is off. Navigation does not contribute location data.');
	admitted = $state(false);
	eligible = $state(false);
	optedIn = $state(false);
	paused = $state(false);
	busy = $state(false);
	pending = $state(0);
	private now: () => number;
	private key: () => string;
	private revision: TourRevision | null = null;
	private mode: 'walk' | 'bike' = 'walk';
	private id = '';
	private detectors: DeviationDetector[] = [];
	private visible = true;
	private suspended = false;
	private ended = false;
	private stopped = false;
	private storageBroken = false;
	private gateChecked = -Infinity;
	private sampleDay = '';
	private flushing: Promise<void> | null = null;

	constructor(private d: ContributionDeps) {
		this.now = d.now ?? Date.now;
		this.key = d.key ?? (() => crypto.randomUUID());
		try {
			d.store.prune();
			this.count();
		} catch {
			this.storageFailed();
		}
	}
	setRevision(revision: TourRevision | null, mode: 'walk' | 'bike'): void {
		this.lifecycle('end');
		this.revision = revision;
		this.mode = mode;
		this.ended = false;
		this.suspended = false;
		this.id =
			revision && this.d.owner ? `${this.d.owner}:${revision.tour_id}:${revision.id}:${mode}` : '';
		this.eligible = contributionContexts(revision, mode).length > 0;
		this.restore();
		this.rebuild();
	}
	private restore(): void {
		try {
			const r = this.d.store.read().find((r) => r.id === this.id);
			this.optedIn =
				!!r?.enabled && !!r.consent && !r.withdrawing && !this.stopped && this.d.sameIdentity();
			this.count();
		} catch {
			this.storageFailed();
		}
	}
	private count(): void {
		this.pending = this.d.store.read().reduce((n, r) => n + r.queue.length, 0);
	}
	private storageFailed(): void {
		this.storageBroken = true;
		this.admitted = false;
		this.stopped = true;
		this.optedIn = false;
		this.detectors = [];
		this.message =
			'Contribution storage is unavailable. Observation and uploads are stopped; navigation still works.';
	}
	private rebuild(): void {
		this.detectors =
			this.eligible && this.optedIn && !this.stopped
				? contributionContexts(this.revision, this.mode).map((c) => new DeviationDetector(c))
				: [];
		this.sampleDay = contributionDay(this.now());
	}
	lifecycle(type: Exclude<DeviationInput['type'], 'fix'>): void {
		if (type === 'permission_lost' || type === 'manual_arrival') this.suspended = true;
		if (type === 'resume') this.suspended = false;
		for (const detector of this.detectors) detector.push({ type });
		if (type === 'end') {
			this.detectors = [];
			this.ended = true;
		}
	}
	setVisible(visible: boolean): void {
		this.visible = visible;
		if (visible) this.resumeSampling();
		else this.lifecycle('pause');
	}
	private resumeSampling(): void {
		if (!this.suspended && !this.ended && this.visible && !this.paused)
			for (const detector of this.detectors) detector.push({ type: 'resume' });
	}
	async storageChanged(): Promise<void> {
		this.lifecycle('pause');
		await this.refresh();
		if (this.admitted && this.optedIn) this.resumeSampling();
	}
	togglePause(): void {
		this.paused = !this.paused;
		this.lifecycle(this.paused ? 'pause' : 'resume');
	}
	async refresh(): Promise<void> {
		try {
			this.d.store.prune();
			this.storageBroken = false;
			this.restore();
			const gate = await this.d.api.gate();
			const resume = !this.admitted || this.now() - this.gateChecked > 60_000;
			this.admitted =
				gate.enabled === true &&
				gate.detector_version === 'uncalibrated-v1' &&
				gate.calibrated === false;
			this.gateChecked = this.now();
			if (!this.admitted) {
				this.lifecycle('pause');
				this.message = 'Route improvement collection is unavailable. Navigation still works.';
			} else if (resume) this.resumeSampling();
		} catch (e) {
			if (e instanceof ContributionStorageError) {
				this.storageFailed();
				return;
			}
			this.admitted = false;
			this.lifecycle('pause');
			this.message =
				'Collection could not be checked. Observation is paused; saved summaries wait for a retry.';
		}
		await this.flush();
		if (this.optedIn && !this.ended && this.detectors.length === 0) this.rebuild();
	}
	async optIn(): Promise<void> {
		if (!this.d.sameIdentity()) {
			this.message = 'Reopen this trip after changing accounts before choosing to contribute.';
			return;
		}
		if (
			this.busy ||
			this.storageBroken ||
			!this.eligible ||
			!this.revision ||
			!this.d.owner ||
			!this.d.sameIdentity()
		)
			return;
		this.busy = true;
		this.stopped = false;
		this.ended = false;
		try {
			await this.refresh();
			if (!this.admitted || !this.d.sameIdentity()) return;
			const records = this.d.store.read();
			const previous = records.find((r) => r.id === this.id);
			if (previous) {
				if (previous.withdrawing) {
					this.message =
						'Finish the pending deletion before making a new choice for this revision.';
					return;
				}
				previous.enabled = true;
			} else
				records.push({
					id: this.id,
					owner: this.d.owner,
					choice: {
						version: 1,
						accepted: true,
						tour_id: this.revision.tour_id,
						revision_id: this.revision.id,
						mode: this.mode,
						requested_on: contributionDay(this.now())
					},
					key: this.key(),
					enabled: true,
					withdrawing: false,
					queue: []
				});
			// Save the exact choice and retry key before any request can create consent.
			this.d.store.write(records);
			await this.flush();
			this.rebuild();
		} catch {
			this.storageFailed();
		} finally {
			this.busy = false;
		}
	}
	observe(fix: DeviationFix): void {
		if (this.now() - this.gateChecked > 60_000) this.admitted = false;
		if (
			this.ended ||
			this.stopped ||
			!this.visible ||
			this.paused ||
			this.suspended ||
			!this.d.sameIdentity() ||
			!this.admitted
		) {
			this.lifecycle('pause');
			return;
		}
		if (fix.timestampMs > this.now() + 1000 || this.now() - fix.timestampMs > 30_000) {
			this.lifecycle('pause');
			this.lifecycle('resume');
			return;
		}
		try {
			const r = this.d.store.read().find((r) => r.id === this.id);
			if (
				!r?.enabled ||
				r.withdrawing ||
				!r.consent ||
				r.consent.expires_on <= contributionDay(this.now())
			) {
				this.optedIn = false;
				this.detectors = [];
				return;
			}
			if (this.sampleDay !== contributionDay(this.now())) this.rebuild();
			for (const detector of this.detectors)
				for (const summary of detector.push({ type: 'fix', fix })) {
					this.d.store.update(this.id, (current) => {
						if (
							!current.enabled ||
							current.withdrawing ||
							current.key !== r.key ||
							current.queue.length >= 32
						)
							return;
						const minimal = minimalSummary(summary);
						if (
							!current.queue.some(
								(p) =>
									p.summary.segment_id === minimal.segment_id &&
									p.summary.direction === minimal.direction &&
									p.summary.observation_period === minimal.observation_period
							)
						)
							current.queue.push({ key: this.key(), summary: minimal });
					});
				}
			this.count();
			if (this.pending) void this.flush();
		} catch {
			this.storageFailed();
		}
	}
	async withdrawAll(): Promise<void> {
		this.stopped = true;
		this.optedIn = false;
		this.lifecycle('end');
		try {
			this.d.store.stopAll();
			this.count();
			this.message =
				'Observation stopped and queued summaries cleared. Deleting retained contributions…';
			await this.flush();
			await this.flush();
		} catch {
			this.storageFailed();
		}
	}
	flush(): Promise<void> {
		if (this.flushing) return this.flushing;
		this.flushing = this.upload().finally(() => {
			this.flushing = null;
		});
		return this.flushing;
	}
	private async upload(): Promise<void> {
		try {
			this.d.store.prune();
			for (const record of this.d.store.read()) {
				let r: ContributionRecord | undefined = record;
				if (!r.consent) {
					if (
						!this.admitted ||
						!this.d.sameIdentity() ||
						r.owner !== this.d.owner ||
						(!r.enabled && !r.withdrawing)
					)
						continue;
					const consent = await this.d.api.grant(r.choice, r.key);
					// Retain erasure capability even if sign-out/withdrawal happened in flight.
					this.d.store.update(r.id, (current) => {
						if (current.key === r!.key) current.consent = consent;
					});
				}
				r = this.d.store.read().find((x) => x.id === record.id && x.key === record.key);
				if (!r?.consent) continue;
				if (r.withdrawing) {
					try {
						await this.d.api.withdraw(r.consent);
					} catch (e) {
						if (!(e instanceof ApiError && e.status === 404)) throw e;
					}
					this.d.store.write(this.d.store.read().filter((x) => x.id !== r!.id || x.key !== r!.key));
					continue;
				}
				if (
					this.stopped ||
					!r.enabled ||
					!this.admitted ||
					this.now() - this.gateChecked > 60_000 ||
					!this.d.sameIdentity() ||
					r.owner !== this.d.owner ||
					r.queue.length === 0
				)
					continue;
				// Revalidate the original choice, never silently renew it with a new day/key.
				const checked = await this.d.api.grant(r.choice, r.key);
				if (checked.id !== r.consent.id || checked.expires_on !== r.consent.expires_on)
					throw new Error('Consent changed');
				for (const pending of r.queue) {
					this.d.store.prune();
					const current = this.d.store.read().find((x) => x.id === r!.id && x.key === r!.key);
					if (
						this.stopped ||
						!this.d.sameIdentity() ||
						!current?.enabled ||
						current.withdrawing ||
						!current.queue.some((p) => p.key === pending.key)
					)
						break;
					try {
						await this.d.api.submit(
							current.consent!.id,
							minimalSummary(pending.summary),
							pending.key
						);
					} catch (e) {
						if (!(e instanceof ApiError && e.status === 409 && e.code === 'already_contributed'))
							throw e;
					}
					this.d.store.update(current.id, (x) => {
						x.queue = x.queue.filter((p) => p.key !== pending.key);
					});
				}
			}
			this.count();
			this.restore();
			if (!this.d.store.read().some((r) => r.withdrawing))
				this.message =
					this.admitted && this.optedIn
						? 'Route improvement is on for this revision and travel mode. Only qualified day-level counts can be sent.'
						: this.admitted
							? 'Route improvement is off. Navigation does not contribute location data.'
							: 'Collection is unavailable. Observation is stopped; navigation still works.';
			else
				this.message =
					'Deletion is pending. Unknown consent receipts need the original account and an available collection service; retained server data expires within 30 days.';
		} catch (e) {
			if (e instanceof ContributionStorageError) {
				this.storageFailed();
				return;
			}
			this.lifecycle('pause');
			if (e instanceof ApiError && [401, 403, 404, 422].includes(e.status)) {
				this.admitted = false;
				this.message =
					'Contribution permission is unavailable. Observation stopped; retry or delete retained contributions.';
			} else
				this.message =
					'Contribution work is pending. Retry when online; no new consent or summary key will be created.';
		}
	}
}
