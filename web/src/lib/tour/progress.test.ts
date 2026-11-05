import { beforeEach, describe, expect, it } from 'vitest';
import { ProgressStore, progressKey, type ProgressApi } from './progress';

class MemoryStorage implements Storage {
	private m = new Map<string, string>();
	get length() {
		return this.m.size;
	}
	clear() {
		this.m.clear();
	}
	getItem(k: string) {
		return this.m.get(k) ?? null;
	}
	key(i: number) {
		return [...this.m.keys()][i] ?? null;
	}
	removeItem(k: string) {
		this.m.delete(k);
	}
	setItem(k: string, v: string) {
		this.m.set(k, v);
	}
}

const TOUR = '00000000-0000-4000-8000-00000000e0e1';

function fakeApi(opts: { remote?: number[]; fail?: boolean } = {}) {
	const puts: { mode: string; visited_positions: number[]; completed: boolean }[] = [];
	const state = { fail: opts.fail ?? false };
	const api: ProgressApi = {
		async get() {
			return opts.remote
				? [
						{
							account_id: 'a',
							tour_id: TOUR,
							mode: 'bike',
							visited_positions: opts.remote,
							started_at: '2026-11-04T00:00:00Z',
							completed_at: null
						}
					]
				: [];
		},
		async put(body) {
			if (state.fail) throw new Error('down');
			puts.push(body);
			return {};
		}
	};
	return { api, puts, state };
}

describe('ProgressStore', () => {
	let storage: MemoryStorage;
	beforeEach(() => {
		storage = new MemoryStorage();
	});

	const make = (signedIn: boolean, f = fakeApi()) =>
		new ProgressStore(TOUR, 'walk', 4, { storage, api: f.api, signedIn: () => signedIn });

	it('persists visited stops and posts them when signed in', async () => {
		const f = fakeApi();
		const s = make(true, f);
		s.markVisited(1);
		await s.flush();
		expect(f.puts).toEqual([{ mode: 'walk', visited_positions: [1], completed: false }]);
		expect(JSON.parse(storage.getItem(progressKey(TOUR, 'walk'))!).visited).toEqual([1]);
		expect(s.needsSync).toBe(false);
	});

	it('stays local when signed out', async () => {
		const f = fakeApi();
		const s = make(false, f);
		s.markVisited(1);
		await s.flush();
		expect(f.puts).toEqual([]);
		expect([...s.visited]).toEqual([1]);
	});

	it('merges the server record without dropping local stops (Review Focus 3)', async () => {
		const f = fakeApi({ remote: [1, 2] });
		const s = make(true, f);
		s.markVisited(3);
		await s.flush();
		await s.mergeRemote();
		await s.flush();
		expect([...s.visited].sort()).toEqual([1, 2, 3]);
		expect(f.puts.at(-1)?.visited_positions).toEqual([1, 2, 3]);
	});

	it('does not post when the server already has everything', async () => {
		const f = fakeApi({ remote: [1, 2] });
		const s = make(true, f);
		await s.mergeRemote();
		await s.flush();
		expect([...s.visited].sort()).toEqual([1, 2]);
		expect(f.puts).toEqual([]);
		expect(s.needsSync).toBe(false);
	});

	it('re-sends when the server was down', async () => {
		const f = fakeApi({ fail: true });
		const s = make(true, f);
		s.markVisited(1);
		await s.flush();
		expect(s.needsSync).toBe(true);
		expect(s.syncError).toBe('down');
		f.state.fail = false;
		await s.syncIfNeeded();
		expect(f.puts).toHaveLength(1);
		expect(s.needsSync).toBe(false);
		await s.syncIfNeeded();
		expect(f.puts).toHaveLength(1);
	});

	it('sends PUTs in order, each carrying every stop known when it leaves', async () => {
		const f = fakeApi();
		const s = make(true, f);
		s.markVisited(1);
		s.markVisited(2);
		await s.flush();
		const bodies = f.puts.map((p) => p.visited_positions);
		expect(bodies).toHaveLength(2);
		expect(bodies.at(-1)).toEqual([1, 2]);
		expect(bodies[0].every((p) => bodies[1].includes(p))).toBe(true);
	});

	it('marks completion when every stop is visited', async () => {
		const f = fakeApi();
		const s = make(true, f);
		[1, 2, 3, 4].forEach((p) => s.markVisited(p));
		await s.flush();
		expect(s.isComplete).toBe(true);
		expect(f.puts.at(-1)?.completed).toBe(true);
	});

	it('restores from storage', async () => {
		make(false).markVisited(2);
		const again = make(false);
		expect([...again.visited]).toEqual([2]);
		again.reset();
		expect(again.visited.size).toBe(0);
		expect(make(false).visited.size).toBe(0);
	});

	it('unions with another tab writing the same storage key (Review Focus 3)', async () => {
		const fa = fakeApi();
		const a = make(true, fa);
		const b = make(true, fakeApi());
		a.markVisited(1);
		b.markVisited(2);
		a.markVisited(3);
		await a.flush();
		await b.flush();
		expect([...a.visited].sort()).toEqual([1, 2, 3]);
		expect(JSON.parse(storage.getItem(progressKey(TOUR, 'walk'))!).visited).toEqual([1, 2, 3]);
		expect(fa.puts.at(-1)?.visited_positions).toEqual([1, 2, 3]);
		b.reload();
		expect([...b.visited].sort()).toEqual([1, 2, 3]);
	});

	it('resetting a signed-in walk also clears the server record', async () => {
		const f = fakeApi();
		const s = make(true, f);
		s.markVisited(1);
		await s.flush();
		s.reset();
		await s.flush();
		expect(f.puts.at(-1)).toEqual({
			mode: 'walk',
			visited_positions: [],
			completed: false,
			reset: true
		});
		expect(s.visited.size).toBe(0);
	});

	it('persists an offline reset and retries it before merging old server stops', async () => {
		const f = fakeApi({ remote: [1, 2] });
		const s = make(true, f);
		s.markVisited(1);
		await s.flush();
		f.state.fail = true;
		s.reset();
		await s.flush();
		expect(s.needsSync).toBe(true);
		const restored = make(true, f);
		f.state.fail = false;
		await restored.mergeRemote();
		expect([...restored.visited]).toEqual([]);
		expect(f.puts.at(-1)).toMatchObject({ reset: true, visited_positions: [] });
		expect(restored.needsSync).toBe(false);
	});

	it('isolates accounts and refuses a queued write after switching accounts', async () => {
		const f = fakeApi();
		let account = 'alice';
		const a = new ProgressStore(TOUR, 'walk', 4, {
			storage,
			api: f.api,
			signedIn: () => true,
			accountID: () => account
		});
		a.markVisited(1);
		account = 'bob';
		await a.flush();
		expect(f.puts).toEqual([]);
		const b = new ProgressStore(TOUR, 'walk', 4, {
			storage,
			api: f.api,
			signedIn: () => true,
			accountID: () => account
		});
		expect([...b.visited]).toEqual([]);
	});

	it('shares stops across travel modes and observes another tab resetting', async () => {
		const f = fakeApi();
		const walk = make(true, f);
		walk.markVisited(1);
		await walk.flush();
		const bike = new ProgressStore(TOUR, 'bike', 4, { storage, api: f.api, signedIn: () => true });
		expect([...bike.visited]).toEqual([1]);
		bike.reset();
		await bike.flush();
		walk.reload();
		expect([...walk.visited]).toEqual([]);
	});

	it('ignores a server read that began before a local reset', async () => {
		const f = fakeApi();
		let finish: (value: Awaited<ReturnType<ProgressApi['get']>>) => void = () => {};
		f.api.get = () =>
			new Promise((resolve) => {
				finish = resolve;
			});
		const s = make(true, f);
		const merging = s.mergeRemote();
		s.reset();
		await s.flush();
		finish([{ tour_id: TOUR, mode: 'walk', visited_positions: [1, 2] }]);
		await merging;
		expect([...s.visited]).toEqual([]);
		expect(s.needsSync).toBe(false);
	});

	it('does not resurrect an older walk when storage is full during reset', async () => {
		const f = fakeApi();
		const s = make(true, f);
		s.markVisited(1);
		await s.flush();
		storage.setItem = () => {
			throw new Error('storage full');
		};
		s.reset();
		await s.flush();
		s.markVisited(2);
		await s.flush();
		expect([...s.visited]).toEqual([2]);
	});
});
