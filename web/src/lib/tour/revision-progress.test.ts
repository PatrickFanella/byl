import { describe, expect, it } from 'vitest';
import { RevisionProgressStore, revisionProgressKey } from './revision-progress';
import type { RevisionProgress } from '#lib/visitor.ts';

const storage = () => {
	const data = new Map<string, string>();
	return {
		getItem: (k: string) => data.get(k) ?? null,
		setItem: (k: string, v: string) => data.set(k, v),
		removeItem: (k: string) => data.delete(k),
		clear: () => data.clear(),
		key: (n: number) => [...data.keys()][n] ?? null,
		get length() {
			return data.size;
		}
	} as Storage;
};
describe('stable revision progress', () => {
	it('keeps stable IDs separate across revisions and accounts, ignoring stale remote IDs', async () => {
		const disk = storage();
		const sent: unknown[] = [];
		const opts = {
			storage: disk,
			accountID: () => 'account-a',
			signedIn: () => true,
			api: {
				get: async () =>
					({
						tour_id: 'tour',
						revision_id: 'old',
						visited_stop_ids: ['stop-b']
					}) as RevisionProgress,
				put: async (body: unknown) => {
					sent.push(body);
				}
			}
		};
		const p = new RevisionProgressStore('tour', 'one', 'walk', ['stop-a', 'stop-b'], opts);
		p.markVisited('stop-a');
		await p.flush();
		await p.mergeRemote();
		expect([...p.visited]).toEqual(['stop-a']);
		expect(sent).toEqual([{ mode: 'walk', visited_stop_ids: ['stop-a'], completed: false }]);
		const next = new RevisionProgressStore('tour', 'two', 'walk', ['stop-b', 'stop-a'], opts);
		expect(next.visited.size).toBe(0);
		const other = new RevisionProgressStore('tour', 'one', 'walk', ['stop-a', 'stop-b'], {
			...opts,
			accountID: () => 'account-b'
		});
		expect(other.visited.size).toBe(0);
		expect(
			JSON.parse(disk.getItem(revisionProgressKey('tour', 'one', 'account-a'))!).visited
		).toEqual(['stop-a']);
	});
	it('filters unknown visits and merges only proof scoped to this revision', async () => {
		const opts = {
			storage: storage(),
			accountID: () => 'a',
			signedIn: () => true,
			api: {
				get: async () =>
					({
						tour_id: 'tour',
						revision_id: 'rev',
						visited_stop_ids: ['stop-b', 'unrelated']
					}) as RevisionProgress,
				put: async () => ({})
			}
		};
		const p = new RevisionProgressStore('tour', 'rev', 'walk', ['stop-a', 'stop-b'], opts);
		p.markVisited('stop-a');
		await p.mergeRemote();
		await p.flush();
		expect([...p.visited].sort()).toEqual(['stop-a', 'stop-b']);
	});
	it('an in-flight read cannot resurrect progress after reset', async () => {
		let resolve: (value: RevisionProgress) => void = () => {};
		const pending = new Promise<RevisionProgress>((r) => (resolve = r));
		const p = new RevisionProgressStore('tour', 'rev', 'walk', ['stop-a'], {
			storage: storage(),
			accountID: () => 'a',
			signedIn: () => true,
			api: { get: () => pending, put: async () => ({}) }
		});
		const merging = p.mergeRemote();
		p.reset();
		resolve({
			tour_id: 'tour',
			revision_id: 'rev',
			visited_stop_ids: ['stop-a']
		} as RevisionProgress);
		await merging;
		await p.flush();
		expect(p.visited.size).toBe(0);
	});
	it('a reset waits behind an earlier PUT and failed reset retries before later visits', async () => {
		let fail = true;
		const bodies: unknown[] = [];
		const disk = storage();
		const opts = {
			storage: disk,
			accountID: () => 'a',
			signedIn: () => true,
			api: {
				get: async () => null,
				put: async (body: unknown) => {
					if (fail) throw new Error('offline');
					bodies.push(body);
				}
			}
		};
		const p = new RevisionProgressStore('tour', 'rev', 'walk', ['stop-a', 'stop-b'], opts);
		p.markVisited('stop-a');
		p.reset();
		await p.flush();
		expect(p.needsSync).toBe(true);
		const resumed = new RevisionProgressStore('tour', 'rev', 'walk', ['stop-a', 'stop-b'], opts);
		resumed.markVisited('stop-b');
		fail = false;
		await resumed.syncIfNeeded();
		expect(bodies[0]).toEqual({
			mode: 'walk',
			visited_stop_ids: [],
			completed: false,
			reset: true
		});
		expect((bodies.at(-1) as { visited_stop_ids: string[] }).visited_stop_ids).toEqual(['stop-b']);
		expect(resumed.needsSync).toBe(false);
	});
	it('stops syncing when the active account changes', async () => {
		let account = 'a';
		const bodies: unknown[] = [];
		const p = new RevisionProgressStore('tour', 'rev', 'walk', ['stop'], {
			storage: storage(),
			accountID: () => account,
			signedIn: () => true,
			api: {
				get: async () => null,
				put: async (body) => {
					bodies.push(body);
				}
			}
		});
		account = 'b';
		p.markVisited('stop');
		await p.flush();
		expect(bodies).toEqual([]);
		expect(p.visited.size).toBe(0);
	});
});
