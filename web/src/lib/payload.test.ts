import { describe, expect, it } from 'vitest';
import { fieldsFor, applyFields, type Field } from './payload';

describe('payload form mapping', () => {
	const payload = {
		place_id: 'p1',
		title: 'Station Fire',
		raw_text: 'A fire damaged the depot.',
		body: '',
		date_start: '1910-01-01',
		date_precision: 'year',
		topic_ids: ['t1'],
		sources: [{ title: 'Wikipedia', url: 'https://w' }],
		raw: { pageid: 1 },
		draft_notes: 'Check the year.'
	};

	it('exposes editable moment fields in a stable order and keeps the rest', () => {
		const fields = fieldsFor('moment', payload);
		const keys = fields.map((f: Field) => f.key);
		expect(keys.slice(0, 4)).toEqual(['title', 'body', 'date_display', 'date_start']);
		expect(keys).not.toContain('raw');
		expect(keys).not.toContain('raw_text');
		const body = fields.find((f) => f.key === 'body');
		expect(body?.kind).toBe('textarea');
		const precision = fields.find((f) => f.key === 'date_precision');
		expect(precision?.kind).toBe('select');
		expect(precision?.options).toEqual(['day', 'month', 'year', 'circa']);
	});

	it('round-trips edits without losing unknown keys', () => {
		const fields = fieldsFor('moment', payload);
		const edited = applyFields(
			payload,
			fields.map((f) => (f.key === 'title' ? { ...f, value: 'Station Fire of 1910' } : f))
		);
		expect(edited.title).toBe('Station Fire of 1910');
		expect(edited.raw).toEqual({ pageid: 1 });
		expect(edited.raw_text).toBe('A fire damaged the depot.');
		expect(edited.sources).toEqual([{ title: 'Wikipedia', url: 'https://w' }]);
	});

	it('maps place and media payloads to their own fields', () => {
		const place = fieldsFor('place', {
			city_id: 'c',
			name: 'X',
			latitude: 41.8,
			longitude: -88,
			source_url: 'u'
		});
		expect(place.map((f) => f.key)).toEqual(
			expect.arrayContaining([
				'name',
				'summary',
				'kind',
				'latitude',
				'longitude',
				'arrival_radius_m',
				'address'
			])
		);
		const lat = place.find((f) => f.key === 'latitude');
		expect(lat?.kind).toBe('number');
		const media = fieldsFor('media', { kind: 'image', media_url: 'https://x/a.jpg', license: '' });
		expect(media.map((f) => f.key)).toEqual(
			expect.arrayContaining(['caption', 'credit', 'license', 'license_url', 'source_url'])
		);
		expect(
			applyFields(
				{ kind: 'image' },
				media.map((f) => (f.key === 'license' ? { ...f, value: 'CC0-1.0' } : f))
			).license
		).toBe('CC0-1.0');
	});

	it('coerces numbers and drops empty optional strings to empty', () => {
		const fields = fieldsFor('place', { name: 'X' });
		const out = applyFields(
			{ name: 'X' },
			fields.map((f) => (f.key === 'latitude' ? { ...f, value: '41.5' } : f))
		);
		expect(out.latitude).toBe(41.5);
		expect(out.summary).toBe('');
	});
});
