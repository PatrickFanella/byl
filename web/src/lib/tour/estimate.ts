import type { components } from '../api/types';

export type RouteJourney = components['schemas']['RouteJourney'];

// Per-mode route estimates as shown on tour cards and detail pages.

export interface RouteLike {
	mode: string;
	status: string;
	distance_m: number;
	duration_s: number;
	estimated_minutes?: number | null;
	journey?: RouteJourney;
}

/** walk → "walk", bike → "ride". */
export function modeVerb(mode: string): string {
	return mode === 'bike' ? 'ride' : 'walk';
}

/** "24 min walk" / "7 min ride" for a ready route, null otherwise. */
export function estimateText(route: RouteLike | undefined | null): string | null {
	if (!route || route.status !== 'ready') return null;
	const minutes = route.estimated_minutes ?? Math.round(route.duration_s / 60);
	return `${Math.max(minutes, 1)} min ${modeVerb(route.mode)}`;
}

/** "850 m" under a kilometre, "1.8 km" above. */
export function distanceText(meters: number): string {
	if (meters < 1000) return `${Math.round(meters)} m`;
	return `${(meters / 1000).toFixed(1)} km`;
}

export function modeLabel(mode: string): string {
	return mode === 'bike' ? 'Biking' : 'Walking';
}

/** Includes stated stop assumptions; an unplanned return remains explicit. */
export function tripEstimateText(journey: RouteJourney | undefined | null): string | null {
	if (journey?.itinerary_duration_s == null) return null;
	const minutes = Math.max(1, Math.ceil(journey.itinerary_duration_s / 60));
	const returnNote = journey.return_plan.status === 'unplanned' ? '; return not planned' : '';
	return `${minutes} min including stops${returnNote}`;
}
