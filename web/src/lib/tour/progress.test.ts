import { beforeEach, describe, expect, it } from 'vitest';
import { ProgressStore, type ProgressApi } from './progress';

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
		expect(JSON.parse(storage.getItem(`invisible-museum.progress.${TOUR}.walk`)!).visited).toEqual([
			1
		]);
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

	it('sends PUTs in order', async () => {
		const f = fakeApi();
		const s = make(true, f);
		s.markVisited(1);
		s.markVisited(2);
		await s.flush();
		expect(f.puts.map((p) => p.visited_positions)).toEqual([[1], [1, 2]]);
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
});
