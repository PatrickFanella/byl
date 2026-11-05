// Arrival detection, the same rule as the iOS app: a fix counts only when its accuracy is
// 65 m or better and it lies within the stop's radius plus that accuracy.

export interface Fix {
	lat: number;
	lng: number;
	/** Horizontal accuracy in metres; negative means unknown. */
	accuracy: number;
}

export interface Point {
	lat: number;
	lng: number;
}

export const MAX_ACCURACY_M = 65;

/** Haversine distance in metres. */
export function distanceM(a: Point, b: Point): number {
	const R = 6_371_000;
	const toRad = (d: number) => (d * Math.PI) / 180;
	const dLat = toRad(b.lat - a.lat);
	const dLng = toRad(b.lng - a.lng);
	const s =
		Math.sin(dLat / 2) ** 2 +
		Math.cos(toRad(a.lat)) * Math.cos(toRad(b.lat)) * Math.sin(dLng / 2) ** 2;
	return 2 * R * Math.asin(Math.min(1, Math.sqrt(s)));
}

export function hasArrived(fix: Fix, stop: Point, radiusM: number): boolean {
	if (!(fix.accuracy >= 0) || fix.accuracy > MAX_ACCURACY_M) return false;
	return distanceM(fix, stop) <= radiusM + fix.accuracy;
}

/** Fires once per stop; `retarget` arms it for the next one. */
export class ArrivalMonitor {
	stop: Point;
	radiusM: number;
	hasArrived = false;

	constructor(stop: Point, radiusM: number) {
		this.stop = stop;
		this.radiusM = radiusM;
	}

	evaluate(fix: Fix): boolean {
		return hasArrived(fix, this.stop, this.radiusM);
	}

	distance(fix: Point): number {
		return distanceM(fix, this.stop);
	}

	/** Returns true the first time the stop is reached. */
	update(fix: Fix): boolean {
		if (this.hasArrived || !this.evaluate(fix)) return false;
		this.hasArrived = true;
		return true;
	}

	retarget(stop: Point, radiusM: number): void {
		this.stop = stop;
		this.radiusM = radiusM;
		this.hasArrived = false;
	}
}
