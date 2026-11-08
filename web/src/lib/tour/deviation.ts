import { distanceM, type Point } from './arrival';

// Candidate engineering values only. F7 field calibration must precede collection.
export const DEVIATION_RULES = Object.freeze({
	version: 'uncalibrated-v1',
	calibrated: false,
	maxAccuracyM: 20,
	insideM: 20,
	outsideM: 35,
	ambiguityMarginM: 10,
	maxGapMs: 20_000,
	maxEpisodeMs: 15 * 60_000,
	minOutsideMs: 20_000,
	minOutsideSamples: 3,
	minMovementM: 30,
	minRejoinAdvanceM: 30,
	minPassageMs: 30_000,
	minPassageSamples: 5,
	maxSpeedMps: Object.freeze({ walk: 4, bike: 15 }),
	maxPathVertices: 512,
	maxCompetingPaths: 8,
	maxStopAreas: 32,
	minPathLengthM: 50,
	maxPathLengthM: 5_000
} as const);

/** One reviewed logical traversal, not an inferred match to an arbitrary GPS leg. */
export interface DeviationContext {
	revisionID: string;
	segmentID: string;
	mode: 'walk' | 'bike';
	direction: 'forward' | 'reverse';
	eligibility: 'reviewed' | 'unsupported';
	/** Must be explicit; missing parallel-road/overlap analysis is not a clean result. */
	ambiguityReviewed: boolean;
	path: Point[];
	competingPaths: Point[][];
	stopAreas: { center: Point; radiusM: number }[];
}

export interface DeviationFix extends Point {
	accuracyM: number;
	/** Epoch milliseconds, monotonically increasing within a traversal. */
	timestampMs: number;
}

export type DeviationInput =
	| { type: 'fix'; fix: DeviationFix }
	| { type: 'pause' | 'resume' | 'end' | 'permission_lost' | 'manual_arrival' };

/** No coordinate, exact timestamp, account ID, trace, or persistent trip identifier. */
export interface DeviationSummary {
	schema_version: 1;
	detector_version: 'uncalibrated-v1';
	calibrated: false;
	revision_id: string;
	segment_id: string;
	mode: 'walk' | 'bike';
	direction: 'forward' | 'reverse';
	observation_period: string;
	qualified_passages: 1;
	sustained_departure_rejoins: 0 | 1;
}

type XY = { x: number; y: number };
type Projection = { distance: number; along: number };
const R = 6_371_000;
const rad = Math.PI / 180;
const pointValid = (p: Point) =>
	Number.isFinite(p.lat) &&
	Number.isFinite(p.lng) &&
	Math.abs(p.lat) <= 85 &&
	Math.abs(p.lng) <= 180;

/** Pure in-memory reference. It never subscribes to GPS, stores data or uploads. */
export class DeviationDetector {
	readonly supported: boolean;
	private context: DeviationContext;
	private path: XY[] = [];
	private competing: XY[][] = [];
	private origin: Point;
	private length = 0;
	private paused = false;
	private ended = false;
	private reported = false;
	private lastSeen = -Infinity;
	private previous: DeviationFix | null = null;
	private start: { fix: DeviationFix } | null = null;
	private trusted: { fix: DeviationFix; along: number } | null = null;
	private nearSamples = 0;
	private armed = false;
	private uncertainSince: number | null = null;
	private samples = 0;
	private deviated = false;
	private outside: {
		first: DeviationFix;
		departureAlong: number;
		count: number;
		qualified: boolean;
	} | null = null;

	constructor(context: DeviationContext) {
		// Snapshot caller-owned geometry; later mutations must not retarget a traversal.
		this.context = structuredClone(context);
		this.origin = { ...(context.path[0] ?? { lat: 0, lng: 0 }) };
		const validPath = (points: Point[]) =>
			points.length >= 2 &&
			points.length <= DEVIATION_RULES.maxPathVertices &&
			points.every(
				(p) =>
					pointValid(p) &&
					Math.abs(p.lat - this.origin.lat) <= 0.25 &&
					Math.abs(p.lng - this.origin.lng) <= 0.25
			);
		this.supported =
			context.eligibility === 'reviewed' &&
			context.ambiguityReviewed === true &&
			!!context.revisionID &&
			!!context.segmentID &&
			(context.mode === 'walk' || context.mode === 'bike') &&
			(context.direction === 'forward' || context.direction === 'reverse') &&
			validPath(context.path) &&
			context.competingPaths.length <= DEVIATION_RULES.maxCompetingPaths &&
			context.competingPaths.every(validPath) &&
			context.stopAreas.length <= DEVIATION_RULES.maxStopAreas &&
			context.stopAreas.every(
				(area) =>
					pointValid(area.center) &&
					Number.isFinite(area.radiusM) &&
					area.radiusM >= 0 &&
					area.radiusM <= 500
			);
		if (!this.supported) return;
		const points = context.direction === 'reverse' ? [...context.path].reverse() : context.path;
		this.path = points.map((p) => this.xy(p));
		this.competing = context.competingPaths.map((line) => line.map((p) => this.xy(p)));
		this.length = this.path
			.slice(1)
			.reduce(
				(sum, point, i) => sum + Math.hypot(point.x - this.path[i].x, point.y - this.path[i].y),
				0
			);
		this.supported =
			this.length >= DEVIATION_RULES.minPathLengthM &&
			this.length <= DEVIATION_RULES.maxPathLengthM;
	}

	push(input: DeviationInput): DeviationSummary[] {
		if (input.type !== 'fix') {
			this.clear();
			this.previous = null;
			if (input.type === 'end') this.ended = true;
			this.paused = input.type !== 'resume';
			return [];
		}
		if (!this.supported || this.paused || this.ended || this.reported) return [];
		const fix = input.fix;
		if (
			!Number.isSafeInteger(fix.timestampMs) ||
			fix.timestampMs < 0 ||
			fix.timestampMs > 253402300799999 ||
			fix.timestampMs <= this.lastSeen
		)
			return [];
		this.lastSeen = fix.timestampMs;
		if (
			!pointValid(fix) ||
			!Number.isFinite(fix.accuracyM) ||
			fix.accuracyM < 0 ||
			fix.accuracyM > DEVIATION_RULES.maxAccuracyM
		) {
			this.clear();
			this.previous = null;
			return [];
		}
		const accuracy = Math.max(3, fix.accuracyM);
		const previous = this.previous;
		this.previous = { ...fix };
		if (previous) {
			const elapsed = fix.timestampMs - previous.timestampMs;
			const movement = Math.max(
				0,
				distanceM(previous, fix) - Math.max(3, previous.accuracyM) - accuracy
			);
			if (
				elapsed > DEVIATION_RULES.maxGapMs ||
				movement / (elapsed / 1000) > DEVIATION_RULES.maxSpeedMps[this.context.mode]
			) {
				this.clear();
				return [];
			}
		}
		if (
			this.start &&
			(fix.timestampMs - this.start.fix.timestampMs > DEVIATION_RULES.maxEpisodeMs ||
				this.day(fix) !== this.day(this.start.fix))
		)
			this.clear();
		const projections = this.project(fix, this.path).sort((a, b) => a.distance - b.distance);
		const closest = projections[0];
		if (!closest) {
			this.clear();
			return [];
		}
		const ambiguous = projections.some(
			(p) =>
				p.distance <= accuracy + DEVIATION_RULES.ambiguityMarginM &&
				Math.abs(p.along - closest.along) > Math.max(20, accuracy * 2)
		);
		const competitor = this.competing.some((line) =>
			this.project(fix, line).some((p) => p.distance <= accuracy + DEVIATION_RULES.ambiguityMarginM)
		);
		const stopAccess = this.context.stopAreas.some(
			(area) => distanceM(area.center, fix) <= area.radiusM + accuracy
		);
		if (ambiguous || competitor || stopAccess) {
			this.clear();
			return [];
		}
		if (this.trusted && closest.along < this.trusted.along - DEVIATION_RULES.minRejoinAdvanceM) {
			this.clear();
			return [];
		}
		const near = closest.distance + accuracy <= DEVIATION_RULES.insideM;
		const outside = closest.distance - accuracy >= DEVIATION_RULES.outsideM;
		if (!near && !outside) {
			this.nearSamples = 0;
			this.uncertainSince ??= fix.timestampMs;
			if (this.outside && !this.outside.qualified) this.outside = null;
			if (!this.trusted || fix.timestampMs - this.uncertainSince > DEVIATION_RULES.maxGapMs)
				this.clear();
			return [];
		}
		this.uncertainSince = null;
		this.samples++;
		if (near) {
			this.nearSamples++;
			if (this.nearSamples >= 2) this.armed = true;
			if (!this.start && closest.along <= this.length * 0.1) this.start = { fix: { ...fix } };
			if (this.outside && this.nearSamples >= 2) {
				if (
					this.outside.qualified &&
					closest.along - this.outside.departureAlong >= DEVIATION_RULES.minRejoinAdvanceM
				)
					this.deviated = true;
				this.outside = null;
			}
			this.trusted = { fix: { ...fix }, along: closest.along };
			if (
				this.start &&
				!this.outside &&
				this.nearSamples >= 2 &&
				closest.along >= this.length * 0.9 &&
				this.samples >= DEVIATION_RULES.minPassageSamples &&
				fix.timestampMs - this.start.fix.timestampMs >= DEVIATION_RULES.minPassageMs &&
				distanceM(this.start.fix, fix) - accuracy - Math.max(3, this.start.fix.accuracyM) >=
					DEVIATION_RULES.minMovementM
			) {
				this.reported = true;
				const departures = this.deviated ? 1 : 0;
				// Only the coarse summary and replay latch survive a completed passage.
				this.clear();
				this.previous = null;
				this.lastSeen = -Infinity;
				return [
					{
						schema_version: 1,
						detector_version: DEVIATION_RULES.version,
						calibrated: false,
						revision_id: this.context.revisionID,
						segment_id: this.context.segmentID,
						mode: this.context.mode,
						direction: this.context.direction,
						observation_period: this.day(fix),
						qualified_passages: 1,
						sustained_departure_rejoins: departures
					}
				];
			}
		} else {
			if (!this.start || !this.trusted) {
				this.clear();
				return [];
			}
			if (!this.outside && !this.armed) {
				this.clear();
				return [];
			}
			this.outside ??= {
				first: { ...fix },
				departureAlong: this.trusted.along,
				count: 0,
				qualified: false
			};
			this.nearSamples = 0;
			this.outside.count++;
			if (
				this.outside.count >= DEVIATION_RULES.minOutsideSamples &&
				fix.timestampMs - this.outside.first.timestampMs >= DEVIATION_RULES.minOutsideMs &&
				distanceM(this.outside.first, fix) - accuracy - Math.max(3, this.outside.first.accuracyM) >=
					DEVIATION_RULES.minMovementM
			)
				this.outside.qualified = true;
		}
		return [];
	}

	private clear() {
		this.start = null;
		this.trusted = null;
		this.nearSamples = 0;
		this.armed = false;
		this.uncertainSince = null;
		this.samples = 0;
		this.deviated = false;
		this.outside = null;
	}
	private day(fix: DeviationFix) {
		return new Date(fix.timestampMs).toISOString().slice(0, 10);
	}
	private xy(point: Point): XY {
		return {
			x: (point.lng - this.origin.lng) * rad * R * Math.cos(this.origin.lat * rad),
			y: (point.lat - this.origin.lat) * rad * R
		};
	}
	private project(point: Point, line: XY[]): Projection[] {
		const p = this.xy(point);
		let along = 0;
		const result: Projection[] = [];
		for (let i = 1; i < line.length; i++) {
			const a = line[i - 1],
				b = line[i],
				dx = b.x - a.x,
				dy = b.y - a.y,
				length = Math.hypot(dx, dy);
			if (length === 0) continue;
			const t = Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / (length * length)));
			result.push({
				distance: Math.hypot(p.x - a.x - t * dx, p.y - a.y - t * dy),
				along: along + t * length
			});
			along += length;
		}
		return result;
	}
}
