import { describe, expect, it, beforeEach } from 'vitest';
import { session, canCurate } from './auth';

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

describe('session', () => {
	beforeEach(() => {
		session.useStorage(new MemoryStorage());
		session.clear();
	});

	it('persists and restores the token and account', () => {
		session.set({
			token: 'abc',
			account: { id: '1', email: 'a@b.c', display_name: 'A', role: 'curator' }
		});
		expect(session.token()).toBe('abc');
		expect(session.account()?.role).toBe('curator');
		session.restore();
		expect(session.token()).toBe('abc');
	});

	it('clears everything', () => {
		session.set({
			token: 'abc',
			account: { id: '1', email: 'a@b.c', display_name: 'A', role: 'visitor' }
		});
		session.clear();
		expect(session.token()).toBeNull();
		expect(session.account()).toBeNull();
	});

	it('knows who can curate', () => {
		expect(canCurate(null)).toBe(false);
		expect(canCurate({ id: '1', email: '', display_name: '', role: 'visitor' })).toBe(false);
		expect(canCurate({ id: '1', email: '', display_name: '', role: 'curator' })).toBe(true);
		expect(canCurate({ id: '1', email: '', display_name: '', role: 'admin' })).toBe(true);
	});
});
