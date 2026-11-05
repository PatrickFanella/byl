import { describe, expect, it } from 'vitest';
import { RouteBuilder, shareURL } from './builder.svelte';
import { ApiError } from '#lib/api/index.ts';
import type { City, PlaceSummary, TourDetail } from '#lib/visitor.ts';

const city = { id: 'c1', slug: 'downers-grove', name: 'Downers Grove' } as City;
const place = (n: number, cityID = 'c1') =>
	({
		id: `p${n}`,
		city_id: cityID,
		name: `Place ${n}`,
		address: '',
		latitude: 41.79 + n / 1000,
		longitude: -88.01
	}) as PlaceSummary;

function fakeApi(result: TourDetail | Error) {
	const calls: { method: string; id?: string; body: Record<string, unknown> }[] = [];
	return {
		calls,
		create: async (body: Record<string, unknown>) => {
			calls.push({ method: 'POST', body });
			if (result instanceof Error) throw result;
			return result;
		},
		update: async (id: string, body: Record<string, unknown>) => {
			calls.push({ method: 'PATCH', id, body });
			if (result instanceof Error) throw result;
			return result;
		}
	};
}

const created = {
	id: 't9',
	slug: 'abc123XYZ0',
	title: 'Two stops',
	owner_account_id: 'a',
	stops: []
} as unknown as TourDetail;

describe('RouteBuilder', () => {
	it('stays disabled under two stops and without a title or mode', () => {
		const b = new RouteBuilder(city);
		b.title = 'Depot loop';
		expect(b.canSubmit).toBe(false);
		b.add(place(1));
		expect(b.canSubmit).toBe(false);
		expect(b.stopsHint).toBe('Add at least 2 stops (1 so far)');
		b.add(place(2));
		expect(b.canSubmit).toBe(true);
		b.title = '  ';
		expect(b.canSubmit).toBe(false);
		b.title = 'Depot loop';
		b.walk = false;
		b.bike = false;
		expect(b.canSubmit).toBe(false);
	});

	it('ignores duplicates and caps at 30', () => {
		const b = new RouteBuilder(city);
		b.add(place(1));
		b.add(place(1));
		expect(b.stops).toHaveLength(1);
		for (let i = 2; i <= 40; i++) b.add(place(i));
		expect(b.stops).toHaveLength(30);
		expect(b.stopsHint).toBe('30 stops is the maximum');
	});

	it('keeps Create disabled for stops in another city (Review Focus 5)', () => {
		const b = new RouteBuilder(city);
		b.title = 'Two towns';
		b.add(place(1));
		b.add(place(2, 'c2'));
		expect(b.canSubmit).toBe(false);
		expect(b.stopsHint).toBe('Every stop must be in Downers Grove');
		b.remove('p2');
		b.add(place(2));
		expect(b.canSubmit).toBe(true);
	});

	it('reorders and removes', () => {
		const b = new RouteBuilder(city);
		b.add(place(1));
		b.add(place(2));
		b.move(1, 0);
		expect(b.stops.map((s) => s.id)).toEqual(['p2', 'p1']);
		b.remove('p2');
		expect(b.stops.map((s) => s.id)).toEqual(['p1']);
		expect(b.contains(place(1))).toBe(true);
	});

	it('posts the route and returns it', async () => {
		const api = fakeApi(created);
		const b = new RouteBuilder(city);
		b.title = ' Two stops ';
		b.summary = 'x';
		b.bike = false;
		b.visibility = 'private';
		b.add(place(1));
		b.add(place(2));
		const tour = await b.submit(api);
		expect(tour?.slug).toBe('abc123XYZ0');
		expect(api.calls[0]).toEqual({
			method: 'POST',
			body: {
				city: 'downers-grove',
				title: 'Two stops',
				summary: 'x',
				modes: ['walk'],
				visibility: 'private',
				stops: [
					{ place_id: 'p1', note: '' },
					{ place_id: 'p2', note: '' }
				]
			}
		});
	});

	it('surfaces 422 fields next to the stops', async () => {
		const api = fakeApi(
			new ApiError(422, 'validation_failed', 'request failed validation', {
				stops: 'must have between 2 and 30 stops'
			})
		);
		const b = new RouteBuilder(city);
		b.title = 'x';
		b.add(place(1));
		b.add(place(2));
		expect(await b.submit(api)).toBeNull();
		expect(b.stopsError).toBe('must have between 2 and 30 stops');
		expect(b.error).toBe('request failed validation');
		expect(b.busy).toBe(false);
	});

	it('prefills from a route being edited and PATCHes it', async () => {
		const editing = {
			id: 't9',
			slug: 'abc',
			title: 'Mine',
			summary: 's',
			modes: ['bike'],
			visibility: 'unlisted',
			city_id: 'c1',
			stops: [
				{ id: 's2', position: 2, note: '', place: place(2) },
				{ id: 's1', position: 1, note: '', place: place(1) }
			]
		} as unknown as TourDetail;
		const api = fakeApi(created);
		const b = new RouteBuilder(city, editing);
		expect(b.isEditing).toBe(true);
		expect(b.title).toBe('Mine');
		expect(b.walk).toBe(false);
		expect(b.bike).toBe(true);
		expect(b.stops.map((s) => s.id)).toEqual(['p1', 'p2']);
		b.title = 'Renamed';
		await b.submit(api);
		expect(api.calls[0].method).toBe('PATCH');
		expect(api.calls[0].id).toBe('t9');
		expect(api.calls[0].body.title).toBe('Renamed');
	});

	it('builds the share link from the origin and slug', () => {
		expect(shareURL('https://museum.example', 'abc123XYZ0')).toBe(
			'https://museum.example/t/abc123XYZ0'
		);
	});
});
