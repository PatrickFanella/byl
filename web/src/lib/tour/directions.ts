// Directions handoff: Apple Maps on Apple devices, Google Maps elsewhere.

export function isAppleDevice(userAgent: string): boolean {
	return /iPhone|iPad|Macintosh/.test(userAgent);
}

export function mapsURL(
	lat: number,
	lng: number,
	mode: 'walk' | 'bike' | string,
	userAgent: string
): string {
	if (isAppleDevice(userAgent)) {
		return `https://maps.apple.com/?daddr=${lat},${lng}&dirflg=${mode === 'bike' ? 'b' : 'w'}`;
	}
	return `https://www.google.com/maps/dir/?api=1&destination=${lat},${lng}&travelmode=${mode === 'bike' ? 'bicycling' : 'walking'}`;
}

/** Station walks transfer control to another app; only public reference points are sent. */
export function walkingHandoffURL(
	destination: { lat: number; lng: number },
	userAgent: string,
	origin?: { lat: number; lng: number }
) {
	const url = new URL(mapsURL(destination.lat, destination.lng, 'walk', userAgent));
	if (origin)
		url.searchParams.set(
			isAppleDevice(userAgent) ? 'saddr' : 'origin',
			`${origin.lat},${origin.lng}`
		);
	return url.href;
}
