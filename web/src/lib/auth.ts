// Session state: the bearer token and the signed-in account, persisted per browser.
import { writable, get, type Readable } from 'svelte/store';

export interface Account {
	id: string;
	email: string;
	display_name: string;
	role: 'visitor' | 'curator' | 'admin';
}

export interface SessionData {
	token: string;
	account: Account;
}

const KEY = 'invisible-museum.session';

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

function defaultStorage(): Storage {
	try {
		if (typeof localStorage !== 'undefined') return localStorage;
	} catch {
		// private mode or blocked storage
	}
	return new MemoryStorage();
}

class Session {
	private storage: Storage = defaultStorage();
	private data = writable<SessionData | null>(null);

	/** Readable store of the current session, for components. */
	get store(): Readable<SessionData | null> {
		return { subscribe: this.data.subscribe };
	}

	useStorage(s: Storage) {
		this.storage = s;
	}

	restore(): SessionData | null {
		try {
			const raw = this.storage.getItem(KEY);
			const parsed = raw ? (JSON.parse(raw) as SessionData) : null;
			this.data.set(parsed && parsed.token ? parsed : null);
		} catch {
			this.data.set(null);
		}
		return get(this.data);
	}

	set(d: SessionData) {
		this.data.set(d);
		try {
			this.storage.setItem(KEY, JSON.stringify(d));
		} catch {
			// storage unavailable; the in-memory copy still works for this page
		}
	}

	updateAccount(account: Account) {
		const current = get(this.data);
		if (current) this.set({ ...current, account });
	}

	clear() {
		this.data.set(null);
		try {
			this.storage.removeItem(KEY);
		} catch {
			// ignore
		}
	}

	token(): string | null {
		return get(this.data)?.token ?? null;
	}

	account(): Account | null {
		return get(this.data)?.account ?? null;
	}
}

export const session = new Session();

export function canCurate(account: Account | null): boolean {
	return account?.role === 'curator' || account?.role === 'admin';
}

export function isAdmin(account: Account | null): boolean {
	return account?.role === 'admin';
}
