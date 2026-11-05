import { describe, expect, it } from 'vitest';
import { distanceText, estimateText, modeVerb } from './estimate';

const route = (over: Record<string, unknown>) => ({
	mode: 'walk',
	status: 'ready',
	distance_m: 1824.68,
	duration_s: 1404,
	estimated_minutes: 24,
	...over
});

describe('estimateText', () => {
	it('reads the server estimate for a ready route', () => {
		expect(estimateText(route({}))).toBe('24 min walk');
		expect(estimateText(route({ mode: 'bike', duration_s: 405, estimated_minutes: 7 }))).toBe(
			'7 min ride'
		);
	});
	it('falls back to the duration when there is no estimate', () => {
		expect(estimateText(route({ duration_s: 650, estimated_minutes: undefined }))).toBe(
			'11 min walk'
		);
	});
	it('is null unless the route is ready', () => {
		expect(estimateText(route({ status: 'pending' }))).toBeNull();
		expect(estimateText(undefined)).toBeNull();
	});
});

describe('distanceText', () => {
	it('uses metres under a kilometre and one decimal above', () => {
		expect(distanceText(850)).toBe('850 m');
		expect(distanceText(1824.68)).toBe('1.8 km');
	});
});

describe('modeVerb', () => {
	it('names the travel mode', () => {
		expect(modeVerb('walk')).toBe('walk');
		expect(modeVerb('bike')).toBe('ride');
	});
});
