import type { RevisionProgress } from '#lib/visitor.ts';

export interface RevisionProgressApi {
	get(): Promise<RevisionProgress | null>;
	put(body: {
		mode: 'walk' | 'bike';
		visited_stop_ids: string[];
		completed: boolean;
		reset?: boolean;
	}): Promise<unknown>;
}

interface Record {
	visited: string[];
	generation: number;
	changed: number;
	synced: number | null;
	resetPending: boolean;
}

export const revisionProgressKey = (tour: string, revision: string, account: string) =>
	`invisible-museum.revision-progress.${account}.${tour}.${revision}`;

/** Local and remote visits are confined to one account and immutable stop set. */
export class RevisionProgressStore {
	visited = new Set<string>();
	syncError: string | null = null;
	private record: Record;
	private chain = Promise.resolve();
	private key: string;
	private ids: Set<string>;
	constructor(
		readonly tourID: string,
		readonly revisionID: string,
		readonly mode: 'walk' | 'bike',
		stopIDs: string[],
		private opts: {
			storage: Storage;
			api: RevisionProgressApi;
			signedIn: () => boolean;
			accountID: () => string | null;
		}
	) {
		this.account = opts.accountID() ?? 'anonymous';
		this.key = revisionProgressKey(tourID, revisionID, this.account);
		this.ids = new Set(stopIDs);
		this.record = this.read() ?? {
			visited: [],
			generation: Date.now(),
			changed: 0,
			synced: null,
			resetPending: false
		};
		this.visited = new Set(this.record.visited);
	}
	private account: string;
	get isComplete() {
		return this.ids.size > 0 && this.visited.size === this.ids.size;
	}
	get needsSync() {
		return (
			this.record.resetPending ||
			(this.visited.size > 0 && this.record.synced !== this.record.changed)
		);
	}
	private canSync() {
		return this.opts.signedIn() && (this.opts.accountID() ?? 'anonymous') === this.account;
	}
	private read(): Record | null {
		try {
			const r = JSON.parse(this.opts.storage.getItem(this.key) ?? 'null') as Record | null;
			if (
				!r ||
				!Array.isArray(r.visited) ||
				!Number.isFinite(r.generation) ||
				!Number.isFinite(r.changed)
			)
				return null;
			return { ...r, visited: [...new Set(r.visited.filter((id) => this.ids.has(id)))] };
		} catch {
			return null;
		}
	}
	private save() {
		this.record.visited = [...this.visited].sort();
		try {
			this.opts.storage.setItem(this.key, JSON.stringify(this.record));
		} catch {
			/* Keep in-memory progress if storage is unavailable. */
		}
	}
	reload() {
		const stored = this.read();
		if (!stored) return;
		if (stored.generation > this.record.generation) {
			this.record = stored;
			this.visited = new Set(stored.visited);
		} else if (stored.generation === this.record.generation) {
			for (const id of stored.visited) this.visited.add(id);
			if (stored.changed > this.record.changed) this.record = stored;
		}
	}
	markVisited(id: string) {
		if ((this.opts.accountID() ?? 'anonymous') !== this.account) return;
		this.reload();
		if (!this.ids.has(id) || this.visited.has(id)) return;
		this.visited.add(id);
		this.record.changed = Math.max(Date.now(), this.record.changed + 1);
		this.save();
		this.sync();
	}
	reset() {
		if ((this.opts.accountID() ?? 'anonymous') !== this.account) return;
		this.reload();
		this.visited.clear();
		this.record = {
			visited: [],
			generation: Math.max(Date.now(), this.record.generation + 1),
			changed: Math.max(Date.now(), this.record.changed + 1),
			synced: null,
			resetPending: this.canSync()
		};
		this.save();
		this.sync();
	}
	async mergeRemote() {
		if (!this.canSync()) return;
		this.reload();
		if (this.record.resetPending) {
			await this.syncIfNeeded();
			return;
		}
		const generation = this.record.generation;
		try {
			const remote = await this.opts.api.get();
			this.reload();
			if (
				!this.canSync() ||
				generation !== this.record.generation ||
				this.record.resetPending ||
				!remote
			)
				return;
			if (remote.tour_id !== this.tourID || remote.revision_id !== this.revisionID) return;
			const remoteIDs = new Set((remote.visited_stop_ids ?? []).filter((id) => this.ids.has(id)));
			for (const id of remoteIDs) this.visited.add(id);
			this.record.changed = Math.max(Date.now(), this.record.changed + 1);
			this.record.synced = remoteIDs.size === this.visited.size ? this.record.changed : null;
			this.save();
			if (this.needsSync) this.sync();
		} catch (e) {
			this.syncError = e instanceof Error ? e.message : 'Could not read saved progress.';
		}
	}
	private sync() {
		if (!this.canSync()) return;
		this.chain = this.chain.then(async () => {
			if (!this.canSync()) return;
			this.reload();
			const generation = this.record.generation;
			const changed = this.record.changed;
			const ids = [...this.visited];
			try {
				if (this.record.resetPending) {
					await this.opts.api.put({
						mode: this.mode,
						visited_stop_ids: [],
						completed: false,
						reset: true
					});
					this.reload();
					if (!this.canSync() || generation !== this.record.generation) return;
					this.record.resetPending = false;
				}
				if (ids.length)
					await this.opts.api.put({
						mode: this.mode,
						visited_stop_ids: ids,
						completed: ids.length === this.ids.size
					});
				this.reload();
				if (!this.canSync() || generation !== this.record.generation) return;
				this.record.synced = changed;
				this.syncError = null;
				this.save();
			} catch (e) {
				this.syncError = e instanceof Error ? e.message : 'Could not save progress.';
			}
		});
	}
	async syncIfNeeded() {
		if (this.needsSync) this.sync();
		await this.flush();
	}
	flush() {
		return this.chain;
	}
}
