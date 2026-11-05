// Tour progress kept in the browser per account and tour, merged with the server record
// when signed in. PUTs go out one at a time, in order, and are retried when the server is behind.

export interface ProgressRecord {
	resetPending?: boolean;
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
	put(body: {
		mode: string;
		visited_positions: number[];
		completed: boolean;
		reset?: boolean;
	}): Promise<unknown>;
}

export interface ProgressOptions {
	storage: Storage;
	api: ProgressApi;
	signedIn: () => boolean;
	accountID?: () => string | null;
}

export const progressKey = (tourID: string, _mode: string, accountID = 'anonymous') =>
	`invisible-museum.progress.v2.${accountID}.${tourID}`;

export class ProgressStore {
	readonly tourID: string;
	readonly mode: string;
	readonly stopCount: number;
	visited: Set<number>;
	syncError: string | null = null;

	private record: ProgressRecord;
	private opts: ProgressOptions;
	private chain: Promise<void> = Promise.resolve();
	private readonly accountID: string;

	constructor(tourID: string, mode: string, stopCount: number, opts: ProgressOptions) {
		this.tourID = tourID;
		this.mode = mode;
		this.stopCount = stopCount;
		this.opts = opts;
		this.accountID = opts.accountID?.() ?? 'anonymous';
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
		if (this.record.resetPending) return true;
		if (this.visited.size === 0) return false;
		if (this.record.syncedAt == null) return true;
		return this.record.syncedAt < this.record.updatedAt;
	}

	markVisited(position: number): void {
		this.reload();
		if (this.visited.has(position)) return;
		this.visited.add(position);
		this.commit();
		this.sync();
	}

	/** Re-reads storage: another tab may have visited stops since this one loaded. */
	reload(): void {
		const stored = this.load();
		if (!stored) return;
		if (stored.startedAt !== this.record.startedAt) {
			if (stored.updatedAt > this.record.updatedAt) {
				this.record = stored;
				this.visited = new Set(stored.visited);
			}
			return;
		}
		for (const p of stored.visited) this.visited.add(p);
		if (stored.updatedAt > this.record.updatedAt) {
			this.record = { ...stored, visited: [...this.visited].sort((a, b) => a - b) };
		}
	}

	/** Union with the account's record for this tour (any mode); pushes when local is ahead. */
	async mergeRemote(): Promise<void> {
		if (!this.canSync()) return;
		if (this.record.resetPending) {
			await this.syncIfNeeded();
			return;
		}
		const generation = this.record.startedAt;
		let records: RemoteProgress[];
		try {
			records = await this.opts.api.get();
		} catch {
			return;
		}
		if (!this.canSync() || this.record.startedAt !== generation) return;
		if (this.record.resetPending) {
			await this.syncIfNeeded();
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
		if (!this.canSync() || !this.needsSync) return;
		this.sync();
		await this.flush();
	}

	flush(): Promise<void> {
		return this.chain;
	}

	/** Forgets local progress and, when signed in, clears the server record too. */
	reset(): void {
		const startedAt = new Date(
			Math.max(Date.now(), Date.parse(this.record.startedAt) + 1)
		).toISOString();
		this.visited = new Set();
		this.record = {
			visited: [],
			updatedAt: Math.max(Date.now(), this.record.updatedAt + 1),
			syncedAt: null,
			completedAt: null,
			startedAt
		};
		this.record.resetPending = this.canSync();
		this.save();
		this.sync();
	}

	private canSync(): boolean {
		return this.opts.signedIn() && (this.opts.accountID?.() ?? 'anonymous') === this.accountID;
	}

	private commit(): void {
		// Union with whatever another tab wrote to the same key, so no stop is ever dropped.
		const stored = this.load();
		if (stored?.startedAt === this.record.startedAt)
			for (const p of stored.visited) this.visited.add(p);
		this.record.visited = [...this.visited].sort((a, b) => a - b);
		// Strictly after the previous change so needsSync compares correctly within one millisecond.
		this.record.updatedAt = Math.max(Date.now(), this.record.updatedAt + 1);
		if (this.isComplete && !this.record.completedAt)
			this.record.completedAt = new Date().toISOString();
		this.save();
	}

	private sync(): void {
		if (!this.canSync()) return;
		this.chain = this.chain.then(async () => {
			if (!this.canSync()) return;
			// Build the body at send time so a PUT queued behind another carries every stop known now.
			this.reload();
			const body = {
				mode: this.mode,
				visited_positions: [...this.record.visited],
				completed: this.record.completedAt != null
			};
			const changedAt = this.record.updatedAt;
			const generation = this.record.startedAt;
			try {
				if (this.record.resetPending) {
					await this.opts.api.put({
						mode: this.mode,
						visited_positions: [],
						completed: false,
						reset: true
					});
					if (!this.canSync()) return;
					if (this.record.startedAt !== generation) return;
					this.record.resetPending = false;
					this.save();
				}
				if (body.visited_positions.length > 0) await this.opts.api.put(body);
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
			const raw = this.opts.storage.getItem(progressKey(this.tourID, this.mode, this.accountID));
			return raw ? (JSON.parse(raw) as ProgressRecord) : null;
		} catch {
			return null;
		}
	}

	private save(): void {
		try {
			this.opts.storage.setItem(
				progressKey(this.tourID, this.mode, this.accountID),
				JSON.stringify(this.record)
			);
		} catch {
			// private mode or full storage: progress is still in memory
		}
	}
}
