import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api/client.ts';
import type { TourRevision } from '#lib/visitor.ts';
import fixtures from './fixtures/deviation-v1.json';
import { ContributionStore, contributionDay, minimalSummary } from './contribution-store';
import { contributionContexts } from './contribution-context';
import { TripContributions } from './contributions.svelte';
import type { ContributionAPI } from './contribution-api';

const epoch = Date.parse(fixtures.epoch);
const DAY = 86400000;
const point = (x: number, y: number) => ({
	lat: fixtures.origin.lat + ((y / 6371000) * 180) / Math.PI,
	lng:
		fixtures.origin.lng +
		((x / (6371000 * Math.cos((fixtures.origin.lat * Math.PI) / 180))) * 180) / Math.PI
});
const line = [
	[0, 0],
	[400, 0]
].map(([x, y]) => {
	const p = point(x, y);
	return [p.lng, p.lat];
});
export function signalRevision(): TourRevision {
	return {
		id: 'rev-1',
		tour_id: 'tour-1',
		snapshot: {
			tour: {
				id: 'tour-1',
				modes: ['walk'],
				stops: [],
				corridor: {
					intent: {
						routes: [{ mode: 'walk', path: [{ segment_id: 'segment-1', reverse: false }] }],
						segments: [
							{
								id: 'segment-1',
								geometry: { type: 'LineString', coordinates: line },
								signal_references: {
									walk: {
										version: 1,
										ambiguity_reviewed: true,
										source: { version: 'recorded-test-reference' },
										competing_paths: [],
										stop_areas: []
									}
								}
							}
						]
					}
				}
			},
			routes: {
				walk: {
					journey: {
						usable: true,
						segments: [{ id: 'segment-1', reverse: false, access: 'allowed' }]
					}
				}
			}
		}
	} as unknown as TourRevision;
}
class MemoryStorage implements Storage {
	data = new Map<string, string>();
	fail = false;
	get length() {
		return this.data.size;
	}
	clear() {
		this.data.clear();
	}
	getItem(k: string) {
		return this.data.get(k) ?? null;
	}
	setItem(k: string, v: string) {
		if (this.fail) throw new Error('full');
		this.data.set(k, v);
	}
	removeItem(k: string) {
		this.data.delete(k);
	}
	key(i: number) {
		return [...this.data.keys()][i] ?? null;
	}
}
function fixture() {
	let now = epoch;
	let owner = true;
	let enabled = true;
	let sequence = 0;
	const storage = new MemoryStorage();
	const store = new ContributionStore(storage, 'https://fixture.test', () => now);
	const consent = {
		id: 'consent-1',
		deletion_handle: 'random-erasure-capability',
		expires_on: contributionDay(epoch + 30 * DAY)
	};
	const api: ContributionAPI = {
		gate: vi.fn(async () => ({ enabled, detector_version: 'uncalibrated-v1', calibrated: false })),
		grant: vi.fn(async () => consent),
		submit: vi.fn(async () => ({})),
		withdraw: vi.fn(async () => {})
	};
	const controller = new TripContributions({
		store,
		api,
		owner: 'account-1',
		sameIdentity: () => owner,
		now: () => now,
		key: () => `unique-request-key-${++sequence}`
	});
	controller.setRevision(signalRevision(), 'walk');
	return {
		controller,
		api,
		store,
		storage,
		consent,
		setNow: (v: number) => {
			now = v;
		},
		identity: (v: boolean) => {
			owner = v;
		},
		gate: (v: boolean) => {
			enabled = v;
		}
	};
}
async function passage(f: ReturnType<typeof fixture>, after?: (index: number) => void) {
	const example = fixtures.cases[0];
	for (const [i, row] of example.samples.entries()) {
		if (!Array.isArray(row)) continue;
		const [t, x, y, a] = row;
		f.setNow(epoch + t * 1000);
		if (i > 0 && i % 4 === 0) await f.controller.refresh();
		after?.(i);
		f.controller.observe({ ...point(x, y), timestampMs: epoch + t * 1000, accuracyM: a });
	}
	await f.controller.flush();
}

describe('reviewed contribution context', () => {
	it('requires explicit frozen ambiguity metadata, selected access and mode; handles malformed/legacy/transit safely', () => {
		expect(contributionContexts(signalRevision(), 'walk')).toHaveLength(1);
		expect(contributionContexts(signalRevision(), 'bike')).toEqual([]);
		for (const change of [
			(r: TourRevision) => {
				delete r.snapshot!.tour.corridor!.intent!.segments[0].signal_references;
			},
			(r: TourRevision) => {
				r.snapshot!.tour.corridor!.intent!.segments[0].signal_references!.walk!.ambiguity_reviewed =
					false as true;
			},
			(r: TourRevision) => {
				r.snapshot!.routes.walk.journey!.segments[0].access = 'closed';
			},
			(r: TourRevision) => {
				Object.assign(r.snapshot!.tour, { transit: {} });
			},
			(r: TourRevision) => {
				r.snapshot!.tour.corridor!.intent!.segments[0].geometry.coordinates = null as never;
			}
		]) {
			const r = signalRevision();
			change(r);
			expect(contributionContexts(r, 'walk')).toEqual([]);
		}
	});
});

describe('opt-in contribution lifecycle and private queue', () => {
	it('defaults off and never grants or uploads without a separate explicit choice', async () => {
		const f = fixture();
		await f.controller.refresh();
		await passage(f);
		expect(f.api.grant).not.toHaveBeenCalled();
		expect(f.api.submit).not.toHaveBeenCalled();
	});
	it('disabled collection cannot create consent or queue observations', async () => {
		const f = fixture();
		f.gate(false);
		await f.controller.optIn();
		await passage(f);
		expect(f.api.grant).not.toHaveBeenCalled();
		expect(f.store.read()).toEqual([]);
	});
	it('saves exact consent and deletion capability before sampling, sends only the minimal schema', async () => {
		const f = fixture();
		await f.controller.optIn();
		await passage(f);
		expect(f.api.submit).toHaveBeenCalledTimes(1);
		const [id, summary] = vi.mocked(f.api.submit).mock.calls[0];
		expect(id).toBe(f.consent.id);
		expect(summary).toEqual(minimalSummary(summary));
		expect(Object.keys(summary)).toHaveLength(10);
		expect(f.storage.getItem(f.store.key)).not.toMatch(
			/"(lat|lng|latitude|longitude|accuracyM|timestampMs|token|fixes|trace)"/
		);
		expect(f.store.read()[0].queue).toEqual([]);
	});
	it('a failed durable choice prevents the first POST', async () => {
		const f = fixture();
		await f.controller.refresh();
		f.storage.fail = true;
		await f.controller.optIn();
		expect(f.api.grant).not.toHaveBeenCalled();
		expect(f.controller.optedIn).toBe(false);
	});
	it('lost grant responses recover the same body/key and do not silently create another choice', async () => {
		const f = fixture();
		vi.mocked(f.api.grant).mockRejectedValueOnce(new Error('response lost'));
		await f.controller.optIn();
		const first = vi.mocked(f.api.grant).mock.calls[0];
		await f.controller.refresh();
		expect(vi.mocked(f.api.grant).mock.calls[1]).toEqual(first);
		expect(f.store.read()).toHaveLength(1);
		expect(f.store.read()[0].consent?.id).toBe(f.consent.id);
	});
	it('an uncertain summary keeps its exact retry key; a server dedup receipt clears it', async () => {
		const f = fixture();
		vi.mocked(f.api.submit).mockRejectedValue(new Error('offline'));
		await f.controller.optIn();
		await passage(f);
		const first = vi.mocked(f.api.submit).mock.calls[0];
		expect(f.store.read()[0].queue).toHaveLength(1);
		vi.mocked(f.api.submit).mockRejectedValueOnce(
			new ApiError(409, 'already_contributed', 'Already counted')
		);
		await f.controller.refresh();
		expect(vi.mocked(f.api.submit).mock.calls.at(-1)).toEqual(first);
		expect(f.store.read()[0].queue).toEqual([]);
	});
	it('expiry removes queued summaries before seven UTC days and consent before 30', async () => {
		const f = fixture();
		vi.mocked(f.api.submit).mockRejectedValue(new Error('offline'));
		await f.controller.optIn();
		await passage(f);
		f.setNow(epoch + 7 * DAY);
		f.store.prune();
		expect(f.store.read()[0].queue).toEqual([]);
		f.setNow(epoch + 30 * DAY);
		await f.controller.refresh();
		expect(f.store.read()).toEqual([]);
	});
	it('withdrawal clears pending data first and erases retained contributions while signed out and collection is off', async () => {
		const f = fixture();
		vi.mocked(f.api.submit).mockRejectedValue(new Error('offline'));
		await f.controller.optIn();
		await passage(f);
		const sent = vi.mocked(f.api.submit).mock.calls.length;
		f.identity(false);
		f.gate(false);
		await f.controller.refresh();
		await f.controller.withdrawAll();
		expect(f.api.withdraw).toHaveBeenCalledWith(f.consent);
		expect(f.store.read()).toEqual([]);
		expect(f.api.submit).toHaveBeenCalledTimes(sent);
	});
	it('identity loss prevents queued uploads and sampling', async () => {
		const f = fixture();
		await f.controller.optIn();
		f.identity(false);
		await passage(f);
		await f.controller.refresh();
		expect(f.api.submit).not.toHaveBeenCalled();
	});
	it('withdrawal during an unknown grant recovers the handle and deletes instead of sampling', async () => {
		const f = fixture();
		let finish!: (v: typeof f.consent) => void;
		vi.mocked(f.api.grant).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finish = resolve;
				})
		);
		const opting = f.controller.optIn();
		await vi.waitFor(() => expect(f.api.grant).toHaveBeenCalledTimes(1));
		const withdrawing = f.controller.withdrawAll();
		expect(f.controller.optedIn).toBe(false);
		expect(f.store.read()[0].withdrawing).toBe(true);
		expect(f.store.read()[0].queue).toEqual([]);
		f.identity(false);
		finish(f.consent);
		await opting;
		await withdrawing;
		expect(f.api.withdraw).toHaveBeenCalledWith(f.consent);
		expect(f.api.submit).not.toHaveBeenCalled();
		expect(f.store.read()).toEqual([]);
	});
	it('a sign-out during receipt revalidation prevents the queued summary POST', async () => {
		const f = fixture();
		vi.mocked(f.api.submit).mockRejectedValue(new Error('offline'));
		await f.controller.optIn();
		await passage(f);
		const sent = vi.mocked(f.api.submit).mock.calls.length;
		vi.mocked(f.api.grant).mockImplementationOnce(async () => {
			f.identity(false);
			return f.consent;
		});
		await f.controller.refresh();
		expect(f.api.submit).toHaveBeenCalledTimes(sent);
		expect(f.store.read()[0].queue).toHaveLength(1);
	});
	it('a fresh revision does not inherit another revision consent', async () => {
		const f = fixture();
		await f.controller.optIn();
		const revised = signalRevision();
		revised.id = 'rev-2';
		f.controller.setRevision(revised, 'walk');
		await passage(f);
		expect(f.controller.optedIn).toBe(false);
		expect(f.api.submit).not.toHaveBeenCalled();
		expect(f.store.read()).toHaveLength(1);
	});
	it('collection expires in memory when its admission check is stale', async () => {
		const f = fixture();
		await f.controller.optIn();
		f.setNow(epoch + 61000);
		f.controller.observe({ ...point(0, 0), timestampMs: epoch + 61000, accuracyM: 3 });
		expect(f.controller.admitted).toBe(false);
		expect(f.api.submit).not.toHaveBeenCalled();
	});
	it('a late admission response resumes fresh observation after the previous check expired', async () => {
		const f = fixture();
		await f.controller.optIn();
		let finish!: (v: { enabled: boolean; detector_version: string; calibrated: boolean }) => void;
		vi.mocked(f.api.gate).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finish = resolve;
				})
		);
		const checking = f.controller.refresh();
		f.setNow(epoch + 61000);
		f.controller.observe({ ...point(0, 0), timestampMs: epoch + 61000, accuracyM: 3 });
		expect(f.controller.admitted).toBe(false);
		finish({ enabled: true, detector_version: 'uncalibrated-v1', calibrated: false });
		await checking;
		for (const row of fixtures.cases[0].samples) {
			if (!Array.isArray(row)) continue;
			const [t, x, y, a] = row;
			f.setNow(epoch + 61000 + t * 1000);
			if (t > 0 && t % 40 === 0) await f.controller.refresh();
			f.controller.observe({ ...point(x, y), timestampMs: epoch + 61000 + t * 1000, accuracyM: a });
		}
		await f.controller.flush();
		expect(f.api.submit).toHaveBeenCalledTimes(1);
	});
	it('unknown grant handles retain their deletion work through the latest possible expiry', async () => {
		const f = fixture();
		vi.mocked(f.api.grant).mockRejectedValue(new Error('unknown receipt'));
		await f.controller.optIn();
		f.identity(false);
		await f.controller.withdrawAll();
		f.setNow(epoch + 30 * DAY);
		f.store.prune();
		expect(f.store.read()).toHaveLength(1);
		f.setNow(epoch + 31 * DAY);
		f.store.prune();
		expect(f.store.read()).toEqual([]);
	});
	for (const event of ['manual_arrival', 'permission_lost'] as const)
		it(`${event} cannot be resumed by visibility or admission refresh`, async () => {
			const f = fixture();
			await f.controller.optIn();
			f.controller.lifecycle(event);
			f.controller.setVisible(false);
			f.controller.setVisible(true);
			await f.controller.refresh();
			await passage(f);
			expect(f.api.submit).not.toHaveBeenCalled();
			f.controller.lifecycle('resume');
			f.setNow(epoch);
			await f.controller.refresh();
			await passage(f);
			expect(f.api.submit).toHaveBeenCalledTimes(1);
		});
	it('a storage event starts fresh only while the saved choice is still enabled', async () => {
		const f = fixture();
		await f.controller.optIn();
		await f.controller.storageChanged();
		await passage(f);
		expect(f.api.submit).toHaveBeenCalledTimes(1);
		f.store.stopAll();
		await f.controller.storageChanged();
		f.setNow(epoch);
		await passage(f);
		expect(f.api.submit).toHaveBeenCalledTimes(1);
		expect(f.api.withdraw).toHaveBeenCalledWith(f.consent);
	});
	for (const event of ['pause', 'end', 'permission_lost', 'manual_arrival', 'hidden'] as const)
		it(`${event} excludes incomplete passages`, async () => {
			const f = fixture();
			await f.controller.optIn();
			await passage(f, (i) => {
				if (i === 10) {
					if (event === 'hidden') f.controller.setVisible(false);
					else if (event === 'pause') f.controller.togglePause();
					else f.controller.lifecycle(event);
				}
			});
			expect(f.api.submit).not.toHaveBeenCalled();
		});
	it('a gate refresh does not reset a continuously observed passage', async () => {
		const f = fixture();
		await f.controller.optIn();
		await passage(f);
		expect(f.api.submit).toHaveBeenCalledTimes(1);
	});
	it('rejects extra precise fields even if a typed caller is bypassed', () => {
		expect(() => minimalSummary({ timestampMs: epoch } as never)).toThrow();
	});
});
