// A tiny async loader: one value, four states, a retry. Pages render LoadState from it.
import { ApiError } from '#lib/api/index.ts';

export type LoadStatus = 'loading' | 'ready' | 'empty' | 'error';

export class Loader<T> {
	status: LoadStatus = $state('loading');
	data: T | null = $state(null);
	message = $state('');
	#fn: () => Promise<T>;
	#isEmpty: (v: T) => boolean;
	#seq = 0;

	constructor(fn: () => Promise<T>, isEmpty: (v: T) => boolean = () => false) {
		this.#fn = fn;
		this.#isEmpty = isEmpty;
	}

	async run(): Promise<void> {
		const seq = ++this.#seq;
		this.status = 'loading';
		try {
			const v = await this.#fn();
			if (seq !== this.#seq) return;
			this.data = v;
			this.status = this.#isEmpty(v) ? 'empty' : 'ready';
		} catch (e) {
			if (seq !== this.#seq) return;
			this.message = e instanceof ApiError ? e.message : 'Something went wrong.';
			this.status = 'error';
		}
	}
}
