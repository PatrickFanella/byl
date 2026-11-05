import { describe, expect, it } from 'vitest';
import { mapsURL } from './directions';

const chrome = 'Mozilla/5.0 (X11; Linux x86_64) Chrome/130.0';
const iphone = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Safari/604.1';

describe('mapsURL', () => {
	it('sends walkers to Google Maps walking directions', () => {
		expect(mapsURL(41.796, -88.0093, 'walk', chrome)).toBe(
			'https://www.google.com/maps/dir/?api=1&destination=41.796,-88.0093&travelmode=walking'
		);
	});
	it('sends Apple devices to Apple Maps with the cycling flag', () => {
		expect(mapsURL(41.796, -88.0093, 'bike', iphone)).toBe(
			'https://maps.apple.com/?daddr=41.796,-88.0093&dirflg=b'
		);
	});
	it('uses bicycling on Google for riders', () => {
		expect(mapsURL(1, 2, 'bike', chrome)).toContain('travelmode=bicycling');
	});
});
