import { describe, expect, it } from 'vitest';
import { ArrivalMonitor, distanceM, hasArrived } from './arrival';

const station = { lat: 41.796, lng: -88.0093 };
// A fix `meters` east of the station.
const fix = (meters: number, accuracy: number) => ({
	lat: station.lat,
	lng: station.lng + meters / (111_320 * Math.cos((station.lat * Math.PI) / 180)),
	accuracy
});

describe('hasArrived', () => {
	it('is true inside the radius with a good fix', () => {
		expect(hasArrived(fix(20, 10), station, 40)).toBe(true);
	});
	it('ignores poor fixes (a 200 m fix in a car park)', () => {
		expect(hasArrived(fix(5, 200), station, 40)).toBe(false);
	});
	it('is false outside the radius', () => {
		expect(hasArrived(fix(120, 10), station, 40)).toBe(false);
	});
	it('lets the distance use up the accuracy margin', () => {
		expect(hasArrived(fix(50, 15), station, 40)).toBe(true);
		expect(hasArrived(fix(60, 15), station, 40)).toBe(false);
	});
	it('never arrives on an invalid accuracy', () => {
		expect(hasArrived(fix(0, -1), station, 40)).toBe(false);
	});
	it('caps accuracy at 65 m', () => {
		expect(hasArrived(fix(0, 65), station, 40)).toBe(true);
		expect(hasArrived(fix(0, 66), station, 40)).toBe(false);
	});
});

describe('ArrivalMonitor', () => {
	it('fires once per stop until retargeted', () => {
		const m = new ArrivalMonitor(station, 40);
		expect(m.update(fix(10, 10))).toBe(true);
		expect(m.update(fix(12, 10))).toBe(false);
		expect(m.update(fix(300, 10))).toBe(false);
		expect(m.update(fix(8, 10))).toBe(false);
		expect(m.hasArrived).toBe(true);
		const tivoli = { lat: 41.7943, lng: -88.0107 };
		m.retarget(tivoli, 30);
		expect(m.hasArrived).toBe(false);
		expect(m.update({ ...tivoli, accuracy: 8 })).toBe(true);
	});
	it('exposes the distance to the stop', () => {
		const m = new ArrivalMonitor(station, 40);
		expect(m.distance(fix(100, 10))).toBeCloseTo(100, -1);
		expect(distanceM(station, station)).toBe(0);
	});
});
