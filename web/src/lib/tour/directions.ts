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
