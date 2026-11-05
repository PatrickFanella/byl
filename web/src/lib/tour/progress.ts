// Tour progress kept in the browser per tour and mode, merged with the account's server record
// when signed in. PUTs go out one at a time, in order, and are retried when the server is behind.

export interface ProgressRecord {
	visited: number[];
	/** Milliseconds since the epoch, strictly increasing per change. */
	updatedAt: number;
	/** The updatedAt value the server last accepted; null until a PUT succeeds. */
	syncedAt: number | null;
	completedAt: string | null;
	startedAt: string;
}

export interface RemoteProgress {
	tour_id: string;
	mode: string;
	visited_positions: number[];
	completed_at?: string | null;
}

export interface ProgressApi {
	get(): Promise<RemoteProgress[]>;
	put(body: { mode: string; visited_positions: number[]; completed: boolean }): Promise<unknown>;
}

export interface ProgressOptions {
	storage: Storage;
	api: ProgressApi;
	signedIn: () => boolean;
}

export const progressKey = (tourID: string, mode: string) =>
	`invisible-museum.progress.${tourID}.${mode}`;

export class ProgressStore {
	readonly tourID: string;
	readonly mode: string;
	readonly stopCount: number;
	visited: Set<number>;
	syncError: string | null = null;

	private record: ProgressRecord;
	private opts: ProgressOptions;
	private chain: Promise<void> = Promise.resolve();

	constructor(tourID: string, mode: string, stopCount: number, opts: ProgressOptions) {
		this.tourID = tourID;
		this.mode = mode;
		this.stopCount = stopCount;
		this.opts = opts;
		this.record = this.load() ?? {
			visited: [],
			updatedAt: 0,
			syncedAt: null,
			completedAt: null,
			startedAt: new Date().toISOString()
		};
		this.visited = new Set(this.record.visited);
	}

	get isComplete(): boolean {
		return this.stopCount > 0 && this.visited.size >= this.stopCount;
	}

	/** True when the server has not seen the latest local change. */
	get needsSync(): boolean {
		if (this.visited.size === 0) return false;
		if (this.record.syncedAt == null) return true;
		return this.record.syncedAt < this.record.updatedAt;
	}

	markVisited(position: number): void {
		if (this.visited.has(position)) return;
		this.visited.add(position);
		this.commit();
		this.sync();
	}

	/** Re-reads storage: another tab may have visited stops since this one loaded. */
	reload(): void {
		const stored = this.load();
		if (!stored) return;
		for (const p of stored.visited) this.visited.add(p);
		if (stored.updatedAt > this.record.updatedAt) {
			this.record = { ...stored, visited: [...this.visited].sort((a, b) => a - b) };
		}
	}

	/** Union with the account's record for this tour (any mode); pushes when local is ahead. */
	async mergeRemote(): Promise<void> {
		if (!this.opts.signedIn()) return;
		let records: RemoteProgress[];
		try {
			records = await this.opts.api.get();
		} catch {
			return;
		}
		const remote = records.find((r) => r.tour_id === this.tourID);
		if (!remote) return;
		const before = this.visited.size;
		for (const p of remote.visited_positions) this.visited.add(p);
		const remoteSet = new Set(remote.visited_positions);
		const equal =
			remoteSet.size === this.visited.size && [...this.visited].every((p) => remoteSet.has(p));
		if (this.visited.size !== before) this.commit();
		if (equal) {
			this.record.syncedAt = this.record.updatedAt;
			this.save();
		} else {
			this.sync();
		}
	}

	async syncIfNeeded(): Promise<void> {
		if (!this.opts.signedIn() || !this.needsSync) return;
		this.sync();
		await this.flush();
	}

	flush(): Promise<void> {
		return this.chain;
	}

	/** Forgets local progress and, when signed in, clears the server record too. */
	reset(): void {
		this.visited = new Set();
		this.record = {
			visited: [],
			updatedAt: Math.max(Date.now(), this.record.updatedAt + 1),
			syncedAt: null,
			completedAt: null,
			startedAt: new Date().toISOString()
		};
		try {
			this.opts.storage.removeItem(progressKey(this.tourID, this.mode));
		} catch {
			// nothing stored
		}
		if (this.opts.signedIn()) {
			const body = { mode: this.mode, visited_positions: [] as number[], completed: false };
			const changedAt = this.record.updatedAt;
			this.chain = this.chain.then(async () => {
				try {
					await this.opts.api.put(body);
					this.record.syncedAt = changedAt;
					this.syncError = null;
				} catch (e) {
					this.syncError = e instanceof Error ? e.message : 'Could not save progress.';
				}
			});
		}
	}

	private commit(): void {
		// Union with whatever another tab wrote to the same key, so no stop is ever dropped.
		const stored = this.load();
		if (stored) for (const p of stored.visited) this.visited.add(p);
		this.record.visited = [...this.visited].sort((a, b) => a - b);
		// Strictly after the previous change so needsSync compares correctly within one millisecond.
		this.record.updatedAt = Math.max(Date.now(), this.record.updatedAt + 1);
		if (this.isComplete && !this.record.completedAt)
			this.record.completedAt = new Date().toISOString();
		this.save();
	}

	private sync(): void {
		if (!this.opts.signedIn()) return;
		this.chain = this.chain.then(async () => {
			// Build the body at send time so a PUT queued behind another carries every stop known now.
			this.reload();
			const body = {
				mode: this.mode,
				visited_positions: [...this.record.visited],
				completed: this.record.completedAt != null
			};
			const changedAt = this.record.updatedAt;
			try {
				await this.opts.api.put(body);
				this.record.syncedAt = changedAt;
				this.syncError = null;
				this.save();
			} catch (e) {
				this.syncError = e instanceof Error ? e.message : 'Could not save progress.';
			}
		});
	}

	private load(): ProgressRecord | null {
		try {
			const raw = this.opts.storage.getItem(progressKey(this.tourID, this.mode));
			return raw ? (JSON.parse(raw) as ProgressRecord) : null;
		} catch {
			return null;
		}
	}

	private save(): void {
		try {
			this.opts.storage.setItem(progressKey(this.tourID, this.mode), JSON.stringify(this.record));
		} catch {
			// private mode or full storage: progress is still in memory
		}
	}
}
