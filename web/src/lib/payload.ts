// Maps a candidate payload (shaped like the create request for its kind) to editable form
// fields and back, without losing keys the form does not show.

export type FieldKind = 'text' | 'textarea' | 'number' | 'select' | 'date';

export interface Field {
	key: string;
	label: string;
	kind: FieldKind;
	value: string;
	options?: string[];
	help?: string;
}

type Payload = Record<string, unknown>;

interface Spec {
	key: string;
	label: string;
	kind: FieldKind;
	options?: string[];
	help?: string;
}

const momentSpec: Spec[] = [
	{ key: 'title', label: 'Title', kind: 'text' },
	{
		key: 'body',
		label: 'Body (Markdown)',
		kind: 'textarea',
		help: 'Aim for 300–500 sourced words in at least three paragraphs: event, context, and significance. Keep separate events at the same place as separate moments. Leave unsupported details in draft notes.'
	},

	{
		key: 'date_display',
		label: 'Date as shown',
		kind: 'text',
		help: 'e.g. "December 25, 1928" or "Summer 1923"'
	},
	{ key: 'date_start', label: 'Date start', kind: 'date' },
	{ key: 'date_end', label: 'Date end', kind: 'date' },
	{
		key: 'date_precision',
		label: 'Date precision',
		kind: 'select',
		options: ['day', 'month', 'year', 'circa']
	},
	{
		key: 'hero_media_id',
		label: 'Picture asset id',
		kind: 'text',
		help: 'Attach a reviewed image from Media, with caption, creator credit, source and reusable license.'
	},
	{ key: 'slug', label: 'Slug', kind: 'text', help: 'Leave empty to derive from the title.' },
	{
		key: 'draft_notes',
		label: 'Draft notes',
		kind: 'textarea',
		help: 'What the model could not confirm. Not published.'
	}
];

const placeSpec: Spec[] = [
	{ key: 'name', label: 'Name', kind: 'text' },
	{ key: 'summary', label: 'Summary', kind: 'textarea' },
	{
		key: 'kind',
		label: 'Kind',
		kind: 'select',
		options: ['building', 'park', 'street', 'station', 'monument', 'site']
	},
	{ key: 'latitude', label: 'Latitude', kind: 'number' },
	{ key: 'longitude', label: 'Longitude', kind: 'number' },
	{ key: 'arrival_radius_m', label: 'Arrival radius (m)', kind: 'number' },
	{ key: 'address', label: 'Address', kind: 'text' },
	{ key: 'slug', label: 'Slug', kind: 'text' }
];

const mediaSpec: Spec[] = [
	{ key: 'caption', label: 'Caption', kind: 'textarea' },
	{ key: 'credit', label: 'Credit', kind: 'text' },
	{
		key: 'license',
		label: 'Licence',
		kind: 'text',
		help: 'Required before anything using this asset can be published, e.g. CC-BY-SA-4.0, CC0-1.0, PD.'
	},
	{ key: 'license_url', label: 'Licence URL', kind: 'text' },
	{ key: 'source_url', label: 'Source URL', kind: 'text' }
];

const specs: Record<string, Spec[]> = { moment: momentSpec, place: placeSpec, media: mediaSpec };

function asString(v: unknown): string {
	if (v === undefined || v === null) return '';
	if (typeof v === 'string') return v;
	if (typeof v === 'number' || typeof v === 'boolean') return String(v);
	return '';
}

/** fieldsFor returns the editable fields for a candidate kind, pre-filled from the payload. */
export function fieldsFor(kind: string, payload: Payload): Field[] {
	const spec = specs[kind] ?? [];
	return spec.map((s) => ({
		key: s.key,
		label: s.label,
		kind: s.kind,
		options: s.options,
		help: s.help,
		value: asString(payload[s.key])
	}));
}

/** applyFields writes the fields back onto a copy of the payload, coercing numbers. */
export function applyFields(payload: Payload, fields: Field[]): Payload {
	const out: Payload = { ...payload };
	for (const f of fields) {
		if (f.kind === 'number') {
			const trimmed = f.value.trim();
			if (trimmed === '') {
				if (!(f.key in out)) continue;
				delete out[f.key];
			} else {
				const n = Number(trimmed);
				if (!Number.isNaN(n)) out[f.key] = n;
			}
		} else {
			out[f.key] = f.value;
		}
	}
	return out;
}

/** readOnlyEntries lists payload keys the form does not edit, for display. */
export function readOnlyEntries(kind: string, payload: Payload): [string, unknown][] {
	const shown = new Set((specs[kind] ?? []).map((s) => s.key));
	return Object.entries(payload).filter(([k]) => !shown.has(k) && k !== 'raw');
}
