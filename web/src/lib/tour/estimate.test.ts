import { describe, expect, it } from 'vitest';
import { distanceText, estimateText, modeVerb, tripEstimateText } from './estimate';

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

// These canonical response fixtures are also decoded by Swift and Kotlin.
import returnRoute from '../../../../backend/api/fixtures/route-journeys-v1/route-return.json';
import failedRoute from '../../../../backend/api/fixtures/route-journeys-v1/route-failed.json';
import partialTour from '../../../../backend/api/fixtures/route-journeys-v1/tour-partial.json';
import type { RouteJourney } from './estimate';

describe('shared journey contract', () => {
	it('keeps legacy travel separate from the complete itinerary', () => {
		expect(estimateText(partialTour.routes.walk)).toBe('29 min walk');
		expect(tripEstimateText(returnRoute.journey as RouteJourney)).toBe('49 min including stops');
		expect(returnRoute.journey.return_plan.duration_s).toBe(300);
		expect(returnRoute.journey.round_trip_duration_s).toBe(
			returnRoute.journey.itinerary_duration_s
		);
	});
	it('does not invent an estimate for a failed mode', () => {
		expect(tripEstimateText(failedRoute.journey as RouteJourney)).toBeNull();
		expect(partialTour.journeys.bike.usable).toBe(false);
		expect(partialTour.journeys.walk.usable).toBe(true);
	});
	it('states when the return has no plan', () => {
		const journey = {
			...returnRoute.journey,
			return_plan: { status: 'unplanned', distance_m: null, duration_s: null },
			round_trip_duration_s: null
		} as RouteJourney;
		expect(tripEstimateText(journey)).toBe('49 min including stops; return not planned');
	});
});
