import type { components } from '#lib/api/types.ts';
import type { DeviationSummary } from './deviation';

type Schemas = components['schemas'];
export type ConsentChoice = Schemas['ContributionConsentRequest'];
export type Consent = Schemas['ContributionConsent'];
export interface PendingSummary {
	key: string;
	summary: DeviationSummary;
}
export interface ContributionRecord {
	id: string;
	owner: string;
	choice: ConsentChoice;
	key: string;
	enabled: boolean;
	withdrawing: boolean;
	consent?: Consent;
	queue: PendingSummary[];
}
export class ContributionStorageError extends Error {}
const DAY = 86_400_000;
export const contributionDay = (now: number) => new Date(now).toISOString().slice(0, 10);
const date = (s: unknown): s is string =>
	typeof s === 'string' &&
	/^\d{4}-\d{2}-\d{2}$/.test(s) &&
	Number.isFinite(Date.parse(s)) &&
	contributionDay(Date.parse(s)) === s;
const text = (s: unknown, max = 128): s is string =>
	typeof s === 'string' && s.length > 0 && s.length <= max;
const keys = (v: object, allowed: string[]) => Object.keys(v).every((k) => allowed.includes(k));

/** Whitelist before persistence or transport, even if callers supply extra properties. */
export function minimalSummary(value: DeviationSummary): DeviationSummary {
	if (
		!value ||
		!keys(value, [
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
		]) ||
		value.schema_version !== 1 ||
		value.detector_version !== 'uncalibrated-v1' ||
		value.calibrated !== false ||
		!text(value.revision_id) ||
		!text(value.segment_id, 120) ||
		!['walk', 'bike'].includes(value.mode) ||
		!['forward', 'reverse'].includes(value.direction) ||
		!date(value.observation_period) ||
		value.qualified_passages !== 1 ||
		![0, 1].includes(value.sustained_departure_rejoins)
	)
		throw new Error('Invalid minimal summary');
	return {
		schema_version: 1,
		detector_version: 'uncalibrated-v1',
		calibrated: false,
		revision_id: value.revision_id,
		segment_id: value.segment_id,
		mode: value.mode,
		direction: value.direction,
		observation_period: value.observation_period,
		qualified_passages: 1,
		sustained_departure_rejoins: value.sustained_departure_rejoins
	};
}

function validRecord(r: ContributionRecord): boolean {
	const c = r?.choice;
	return (
		!!r &&
		keys(r, ['id', 'owner', 'choice', 'key', 'enabled', 'withdrawing', 'consent', 'queue']) &&
		text(r.id, 400) &&
		text(r.owner) &&
		text(r.key) &&
		typeof r.enabled === 'boolean' &&
		typeof r.withdrawing === 'boolean' &&
		!!c &&
		keys(c, ['version', 'accepted', 'tour_id', 'revision_id', 'mode', 'requested_on']) &&
		c.version === 1 &&
		c.accepted === true &&
		text(c.tour_id) &&
		text(c.revision_id) &&
		['walk', 'bike'].includes(c.mode) &&
		date(c.requested_on) &&
		(!r.consent ||
			(keys(r.consent, ['id', 'deletion_handle', 'expires_on']) &&
				text(r.consent.id) &&
				text(r.consent.deletion_handle, 256) &&
				date(r.consent.expires_on))) &&
		Array.isArray(r.queue) &&
		r.queue.length <= 32 &&
		r.queue.every(
			(p) =>
				keys(p, ['key', 'summary']) &&
				text(p.key) &&
				!!minimalSummary(p.summary) &&
				p.summary.revision_id === c.revision_id &&
				p.summary.mode === c.mode
		)
	);
}

/** Browser-local scoped choices/erasure handles and bounded minimal summaries only. */
export class ContributionStore {
	readonly key: string;
	constructor(
		private storage: Storage,
		apiBase: string,
		private now = Date.now
	) {
		this.key = `byl.contributions.v1.${encodeURIComponent(apiBase)}`;
	}
	read(): ContributionRecord[] {
		try {
			return this.readRecords();
		} catch {
			throw new ContributionStorageError('Contribution storage is unavailable');
		}
	}
	private readRecords(): ContributionRecord[] {
		const raw = this.storage.getItem(this.key);
		if (!raw) return [];
		if (raw.length > 131072) throw new Error('Contribution storage exceeds its bound');
		const records = JSON.parse(raw) as ContributionRecord[];
		if (!Array.isArray(records) || records.length > 16 || !records.every(validRecord))
			throw new Error('Contribution storage is unreadable; collection stopped');
		return records;
	}
	write(records: ContributionRecord[]): void {
		try {
			this.writeRecords(records);
		} catch {
			throw new ContributionStorageError('Contribution storage could not be saved');
		}
	}
	private writeRecords(records: ContributionRecord[]): void {
		if (records.length > 16 || !records.every(validRecord))
			throw new Error('Contribution storage limit reached');
		const raw = JSON.stringify(records);
		if (raw.length > 131072) throw new Error('Contribution storage limit reached');
		this.storage.setItem(this.key, raw);
		if (this.storage.getItem(this.key) !== raw)
			throw new Error('Contribution choice could not be saved');
	}
	update(id: string, change: (r: ContributionRecord) => void): void {
		const records = this.read();
		const r = records.find((r) => r.id === id);
		if (!r) throw new Error('Contribution choice no longer exists');
		change(r);
		this.write(records);
	}
	prune(): void {
		const today = contributionDay(this.now());
		const records = this.read().filter(
			(r) =>
				// New grants can be created on the request day or the next UTC day.
				(r.consent?.expires_on ?? contributionDay(Date.parse(r.choice.requested_on) + 31 * DAY)) >
				today
		);
		for (const r of records)
			r.queue = r.queue.filter(
				(p) =>
					p.summary.observation_period <= today &&
					this.now() < Date.parse(p.summary.observation_period) + 7 * DAY
			);
		this.write(records);
	}
	stopAll(): void {
		const records = this.read();
		for (const r of records) {
			r.enabled = false;
			r.withdrawing = true;
			r.queue = [];
		}
		this.write(records);
	}
}
