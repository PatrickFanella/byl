import type { Happening } from '#lib/visitor.ts';

export const noticeLabels = {
	active: 'Scheduled for these dates',
	upcoming: 'Upcoming',
	ended: 'Dates passed',
	expired: 'Expired notice',
	cancelled: 'Cancelled',
	date_conflict: 'Dates conflict',
	needs_review: 'Needs review',
	source_unavailable: 'Source unavailable',
	unavailable: 'Notice unavailable'
} as const;
export type NoticeStatus = keyof typeof noticeLabels;
const knownStates = [
	'upcoming',
	'active',
	'ended',
	'expired',
	'needs_review',
	'cancelled',
	'date_conflict',
	'source_unavailable'
];
const knownDeclarations = ['scheduled', 'cancelled', 'date_conflict', 'source_unavailable'];
const validDate = (s: string) =>
	/^\d{4}-\d{2}-\d{2}$/.test(s) &&
	Number.isFinite(Date.parse(s)) &&
	new Date(s).toISOString().slice(0, 10) === s;
const checkedInstant = (s: string) =>
	validDate(s.slice(0, 10)) &&
	/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(s)
		? Date.parse(s)
		: NaN;
export function noticeShape(value: unknown, placeID: string): value is Happening {
	if (!value || typeof value !== 'object') return false;
	const h = value as Record<string, unknown>;
	const fields = [
		'id',
		'revision_id',
		'place_id',
		'title',
		'summary',
		'organizer',
		'starts_on',
		'ends_on',
		'timezone',
		'declared_state',
		'state',
		'review_on',
		'expires_on',
		'last_checked_at',
		'source_health'
	];
	return (
		fields.every((k) => typeof h[k] === 'string' && h[k].length <= 8000) &&
		h.place_id === placeID &&
		!!h.id &&
		!!h.revision_id &&
		Array.isArray(h.issues) &&
		h.issues.length <= 32 &&
		h.issues.every((s) => typeof s === 'string') &&
		Array.isArray(h.sources) &&
		h.sources.length <= 5 &&
		h.sources.every(
			(source) =>
				source &&
				typeof source === 'object' &&
				['url', 'title', 'publisher', 'version', 'retrieved_on', 'license', 'license_url'].every(
					(k) => typeof source[k] === 'string' && source[k].length <= 8000
				)
		)
	);
}
export function noticeSourceURL(value: string): string | null {
	try {
		const u = new URL(value);
		return ['https:', 'http:'].includes(u.protocol) && !u.username && !u.password ? u.href : null;
	} catch {
		return null;
	}
}
export function cityDay(now: number, timezone: string): string {
	const parts = new Intl.DateTimeFormat('en-US', {
		timeZone: timezone,
		year: 'numeric',
		month: '2-digit',
		day: '2-digit'
	}).formatToParts(now);
	return ['year', 'month', 'day']
		.map((type) => parts.find((p) => p.type === type)!.value)
		.join('-');
}

/** Defensive rendering of a no-store response, with no navigation side effect. */
export function noticeStatus(h: Happening, now: number): NoticeStatus {
	try {
		if (
			!knownStates.includes(h.state) ||
			!knownDeclarations.includes(h.declared_state) ||
			![h.starts_on, h.ends_on, h.review_on, h.expires_on].every(validDate) ||
			h.ends_on < h.starts_on
		)
			return 'unavailable';
		const today = cityDay(now, h.timezone);
		const checked = checkedInstant(h.last_checked_at);
		if (!Number.isFinite(checked)) return 'unavailable';
		if (h.declared_state === 'cancelled' || h.state === 'cancelled') return 'cancelled';
		if (h.declared_state === 'date_conflict' || h.state === 'date_conflict') return 'date_conflict';
		if (h.state === 'expired' || today >= h.expires_on) return 'expired';
		if (
			h.source_health === 'unavailable' ||
			h.declared_state === 'source_unavailable' ||
			h.state === 'source_unavailable'
		)
			return 'source_unavailable';
		if (
			h.source_health !== 'unchanged' ||
			h.issues.length > 0 ||
			h.state === 'needs_review' ||
			checked > now ||
			now - checked >= 7 * 86400000 ||
			today >= h.review_on ||
			!h.sources.some((s) => noticeSourceURL(s.url))
		)
			return 'needs_review';
		if (h.state === 'ended' || today > h.ends_on) return 'ended';
		return today < h.starts_on ? 'upcoming' : 'active';
	} catch {
		return 'unavailable';
	}
}

export function noticeDates(h: Happening): string {
	if (![h.starts_on, h.ends_on].every(validDate)) return 'Occurrence dates unavailable';
	const format = (s: string) =>
		new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeZone: 'UTC' }).format(new Date(s));
	return `${format(h.starts_on)}${h.starts_on === h.ends_on ? '' : ` – ${format(h.ends_on)}`} · ${h.timezone} · dates only`;
}
export function noticeChecked(h: Happening): string {
	const value = checkedInstant(h.last_checked_at);
	if (!Number.isFinite(value)) return 'Source check unavailable';
	return `Last source check: ${new Date(value).toISOString().slice(0, 16).replace('T', ' ')} UTC`;
}
