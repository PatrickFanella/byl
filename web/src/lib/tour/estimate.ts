// Per-mode route estimates as shown on tour cards and detail pages.

export interface RouteLike {
	mode: string;
	status: string;
	distance_m: number;
	duration_s: number;
	estimated_minutes?: number | null;
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
